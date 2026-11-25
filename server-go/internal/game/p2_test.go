package game

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// p2Fixture 是 P2 三块（拍卖 / 投递 / 公会战）共用的测试环境。
//
// 全部走真 SQLite（t.TempDir），因为这三块的核心正确性
// 都在「原子性」和「顺序」上 —— 用内存假仓储测不出来。
type p2Fixture struct {
	ctx     context.Context
	Store   *storage.Store
	Items   *ItemDB
	Chars   *storage.CharacterRepository
	Guilds  *storage.GuildRepository
	Golds   *storage.GoldRepository
	Auction *storage.AuctionRepository
	Dels    *storage.DeliveryRepository
	Chat    *storage.ChatRepository
	ItemRepo *storage.ItemRepository
}

func newP2Fixture(t *testing.T) *p2Fixture {
	t.Helper()
	store, err := storage.Open(context.Background(), storage.OpenOptions{
		Path:            filepath.Join(t.TempDir(), "p2.db3"),
		CreateIfMissing: true,
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	items, err := LoadItemDB("../../content/items.json")
	if err != nil {
		t.Fatalf("load items: %v", err)
	}
	return &p2Fixture{
		ctx:     context.Background(),
		Store:   store,
		Items:   items,
		Chars:   storage.NewCharacterRepository(store),
		Guilds:  storage.NewGuildRepository(store),
		Golds:   storage.NewGoldRepository(store),
		Auction: storage.NewAuctionRepository(store),
		Dels:    storage.NewDeliveryRepository(store),
		Chat:    storage.NewChatRepository(store),
		ItemRepo: storage.NewItemRepository(store),
	}
}

// putInInventory 把一件物品放进角色背包（DB 层）。
//
// ★ 必须有这一步：AuctionRepository.Register 是从 tbl_inventory 里
// **原子删除**再插入拍卖行的（影响行数为 0 就返回 ErrInvalidItem）。
// 只在内存 Player.Inventory 里放一件是不够的 —— 那条 SQL 删不到行。
func (f *p2Fixture) putInInventory(t *testing.T, dbid, itemID, seqID uint32) {
	t.Helper()
	if err := f.ItemRepo.PutInventoryItem(f.ctx, dbid, protocol.SDItem{
		ItemID: itemID, SeqID: seqID, Count: 1, Duration: [2]uint64{10, 10},
	}); err != nil {
		t.Fatalf("put inventory item: %v", err)
	}
}

// mkChar 建一个账号 + 一个角色，返回角色 DBID。
func (f *p2Fixture) mkChar(t *testing.T, account, name string) uint32 {
	t.Helper()
	accounts := storage.NewAccountRepository(f.Store)
	if err := accounts.Create(f.ctx, account, "pw123456"); err != nil {
		t.Fatalf("create account %s: %v", account, err)
	}
	ch, err := f.Chars.Create(f.ctx, storage.CreateCharacterParams{
		Account: account, Name: name, Gender: 0, Job: 1, Map: 1, MapX: 100, MapY: 100,
	})
	if err != nil {
		t.Fatalf("create char %s: %v", name, err)
	}
	return ch.DBID
}

// ---------------- 拍卖：钱不够必须把物品放回去 ----------------

// TestAuctionBuyInsufficientKeepsItem 是这个测试的重点：
//
//	早期实现「先 Take 再判金币」，钱不够时物品已经被 delete 掉了、
//	没有任何补偿，**物品直接消失**。这是明确的物品丢失 bug。
//
// 现在按原版顺序（取走 → 条件扣金币 → 失败则 Reinstate 放回）。
func TestAuctionBuyInsufficientKeepsItem(t *testing.T) {
	f := newP2Fixture(t)
	sellerDBID := f.mkChar(t, "seller01", "卖家甲")
	buyerDBID := f.mkChar(t, "buyer001", "买家乙")

	weaponID, ok := f.Items.FindByName("铁剑")
	if !ok {
		t.Fatal("物品表里没有「铁剑」，测试数据变了")
	}

	// 上架一把剑，价格远超买家的钱
	f.putInInventory(t, sellerDBID, weaponID, 1)
	now := time.Now().Unix()
	auctionID, err := f.Auction.Register(f.ctx, sellerDBID,
		protocol.SDItem{ItemID: weaponID, SeqID: 1, Count: 1, Duration: [2]uint64{10, 10}},
		"测试备注", 999999, now+AuctionLifetimeSec)
	if err != nil {
		t.Fatalf("register auction: %v", err)
	}

	// 买家只有 1000 金币（schema 默认给 9900，显式设成 1000 更明确）
	if err := f.Golds.Set(f.ctx, buyerDBID, 1000); err != nil {
		t.Fatalf("set buyer gold: %v", err)
	}

	// 造一个在线买家（Buy 走 ByChannel）
	buyer := &Player{ChannelID: 7, DBID: buyerDBID, UID: playerUIDOf(buyerDBID), Name: "买家乙", Gold: 1000}
	players := NewPlayerManager()
	players.Add(buyer)
	broadcast := &captureBroadcaster{}

	h := &AuctionHandler{
		Players: players, Broadcast: broadcast, Logger: discardLogger(),
		Repo: f.Auction, Items: f.Items, Golds: f.Golds,
		Stalls: storage.NewStallRepository(f.Store),
		Now:    func() int64 { return now },
	}
	if err := h.Buy(f.ctx, buyer.ChannelID, auctionID); err != nil {
		t.Fatalf("buy: %v", err)
	}

	// 买家金币不能变
	if buyer.Gold != 1000 {
		t.Fatalf("钱不够时金币不能被扣，期望 1000 实际 %d", buyer.Gold)
	}
	if stored, _ := f.Golds.Load(f.ctx, buyerDBID); stored != 1000 {
		t.Fatalf("钱不够时库里的金币也不能变，期望 1000 实际 %d", stored)
	}
	// ★ 关键断言：物品必须还在拍卖行里
	list, err := f.Auction.Query(f.ctx, storage.AuctionCategoryAll, now)
	if err != nil {
		t.Fatalf("query auctions: %v", err)
	}
	if len(list.ItemList) != 1 {
		t.Fatalf("钱不够时物品必须放回拍卖行，实际剩 %d 件", len(list.ItemList))
	}
	if list.ItemList[0].Item.ItemID != weaponID {
		t.Fatalf("放回的不是原物品")
	}
}

// TestAuctionBuyDeliversBothSides 验证「买卖双方都走投递」。
//
// 原版不给任何人直接加背包/加金币（auctiondb.cpp:161-169）：
// 双方都是 dbCreateDeliveryInTransaction。
// 这样「卖家离线时东西被买走」才成立 —— 早期实现离线卖家货款直接丢失。
func TestAuctionBuyDeliversBothSides(t *testing.T) {
	f := newP2Fixture(t)
	sellerDBID := f.mkChar(t, "seller02", "卖家丙")
	buyerDBID := f.mkChar(t, "buyer002", "买家丁")

	weaponID, _ := f.Items.FindByName("铁剑")
	f.putInInventory(t, sellerDBID, weaponID, 1)
	now := time.Now().Unix()
	auctionID, err := f.Auction.Register(f.ctx, sellerDBID,
		protocol.SDItem{ItemID: weaponID, SeqID: 1, Count: 1, Duration: [2]uint64{10, 10}},
		"", 1000, now+AuctionLifetimeSec)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := f.Golds.Set(f.ctx, buyerDBID, 5000); err != nil {
		t.Fatalf("set gold: %v", err)
	}

	buyer := &Player{ChannelID: 8, DBID: buyerDBID, UID: playerUIDOf(buyerDBID), Name: "买家丁", Gold: 5000}
	players := NewPlayerManager()
	players.Add(buyer)
	broadcast := &captureBroadcaster{}

	deliverySvc := NewDeliveryService(f.Dels, f.Chat, f.Items, players, broadcast, discardLogger())

	h := &AuctionHandler{
		Players: players, Broadcast: broadcast, Logger: discardLogger(),
		Repo: f.Auction, Items: f.Items, Golds: f.Golds,
		Stalls: storage.NewStallRepository(f.Store), Deliveries: deliverySvc,
		Now: func() int64 { return now },
	}
	if err := h.Buy(f.ctx, buyer.ChannelID, auctionID); err != nil {
		t.Fatalf("buy: %v", err)
	}

	// 买家扣钱
	if buyer.Gold != 4000 {
		t.Fatalf("买家金币期望 4000（5000-1000）实际 %d", buyer.Gold)
	}
	// 卖家没有在线记录 → 只能靠投递。投递必须存在，且金币是税后价（2% → 980）
	sellerPending, err := f.Dels.ListUnclaimed(f.ctx, sellerDBID)
	if err != nil {
		t.Fatalf("list seller deliveries: %v", err)
	}
	if len(sellerPending) != 1 {
		t.Fatalf("卖家应该收到 1 份投递（离线也要能收到），实际 %d", len(sellerPending))
	}
	// 买家也要有一份投递（物品）
	buyerPending, err := f.Dels.ListUnclaimed(f.ctx, buyerDBID)
	if err != nil {
		t.Fatalf("list buyer deliveries: %v", err)
	}
	if len(buyerPending) != 1 {
		t.Fatalf("买家应该收到 1 份物品投递，实际 %d", len(buyerPending))
	}
}

// TestAuctionBuyOwnItem 自己买自己的东西必须被拒且物品放回。
func TestAuctionBuyOwnItem(t *testing.T) {
	f := newP2Fixture(t)
	dbid := f.mkChar(t, "solo0001", "独行侠")
	weaponID, _ := f.Items.FindByName("铁剑")
	f.putInInventory(t, dbid, weaponID, 1)
	now := time.Now().Unix()
	auctionID, err := f.Auction.Register(f.ctx, dbid,
		protocol.SDItem{ItemID: weaponID, SeqID: 1, Count: 1}, "", 100, now+AuctionLifetimeSec)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	_ = f.Golds.Set(f.ctx, dbid, 10000)

	self := &Player{ChannelID: 9, DBID: dbid, UID: playerUIDOf(dbid), Name: "独行侠", Gold: 10000}
	players := NewPlayerManager()
	players.Add(self)
	h := &AuctionHandler{
		Players: players, Broadcast: &captureBroadcaster{}, Logger: discardLogger(),
		Repo: f.Auction, Items: f.Items, Golds: f.Golds,
		Stalls: storage.NewStallRepository(f.Store),
		Now:    func() int64 { return now },
	}
	if err := h.Buy(f.ctx, self.ChannelID, auctionID); err != nil {
		t.Fatalf("buy: %v", err)
	}
	if self.Gold != 10000 {
		t.Fatalf("自己买自己不能扣钱，实际 %d", self.Gold)
	}
	list, _ := f.Auction.Query(f.ctx, storage.AuctionCategoryAll, now)
	if len(list.ItemList) != 1 {
		t.Fatalf("自己买自己时物品必须放回，实际剩 %d 件", len(list.ItemList))
	}
}

// ---------------- 投递：金币溢出必须整单回滚 ----------------

// TestDeliveryGoldFullRollsBack 金币溢出时投递必须保持未领取。
//
// 早期实现是 clamp 到 MaxGold + 标记已领取，
// 超出部分静默消失且再也领不回来。
func TestDeliveryGoldFullRollsBack(t *testing.T) {
	f := newP2Fixture(t)
	dbid := f.mkChar(t, "rich0001", "富人甲")

	players := NewPlayerManager()
	// 金币已到上限
	p := &Player{ChannelID: 11, DBID: dbid, UID: playerUIDOf(dbid), Name: "富人甲", Gold: MaxGold}
	players.Add(p)
	broadcast := &captureBroadcaster{}

	deliverySvc := NewDeliveryService(f.Dels, f.Chat, f.Items, players, broadcast, discardLogger())
	record, err := deliverySvc.DeliverGold(f.ctx, dbid, 1000, "测试投递：")
	if err != nil {
		t.Fatalf("deliver gold: %v", err)
	}

	h := &DeliveryHandler{
		Players: players, Broadcast: broadcast, Logger: discardLogger(),
		Repo: f.Dels, Items: f.Items, Golds: f.Golds,
	}
	if err := h.Claim(f.ctx, p.ChannelID, record); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// 金币不能变
	if p.Gold != MaxGold {
		t.Fatalf("溢出时金币不该变，实际 %d", p.Gold)
	}
	// ★ 关键：投递必须仍是未领取
	pending, err := f.Dels.ListUnclaimed(f.ctx, dbid)
	if err != nil {
		t.Fatalf("list unclaimed: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("溢出回滚后投递必须仍未领取，实际未领 %d 份", len(pending))
	}
}

// TestDeliveryCreatesSystemChatMessage 验证「邮箱 = 系统聊天消息」。
//
// 原版没有 SM_DELIVERYLIST，投递通知就是一条 SYS_CHATDBID_SYSTEM 的聊天。
func TestDeliveryCreatesSystemChatMessage(t *testing.T) {
	f := newP2Fixture(t)
	dbid := f.mkChar(t, "normal01", "普通人")

	players := NewPlayerManager()
	p := &Player{ChannelID: 12, DBID: dbid, UID: playerUIDOf(dbid), Name: "普通人"}
	players.Add(p)
	broadcast := &captureBroadcaster{}
	deliverySvc := NewDeliveryService(f.Dels, f.Chat, f.Items, players, broadcast, discardLogger())

	record, err := deliverySvc.DeliverGold(f.ctx, dbid, 100, "你收到测试金币：")
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}

	// 应该已经推了一条 SM_CHATMESSAGELIST
	if !broadcast.has(protocol.SM_CHATMESSAGELIST) {
		t.Fatal("投递必须推一条系统聊天消息（这就是原版的邮箱）")
	}

	// 聊天正文里必须带 record，客户端要靠它点「收取」
	body, ok := broadcast.lastBody(protocol.SM_CHATMESSAGELIST)
	if !ok {
		t.Fatal("找不到聊天消息包体")
	}
	var list protocol.SDChatMessageList
	if err := protocol.CerealDeserialize(body, &list); err != nil {
		t.Fatalf("decode chat list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("期望 1 条聊天消息，实际 %d", len(list))
	}
	msg := list[0].Message
	if !strings.Contains(msg, record) {
		t.Fatalf("聊天正文必须包含投递 record %q，实际正文：%s", record, msg)
	}
	if !strings.Contains(msg, SysEventDelivery) {
		t.Fatalf("聊天正文必须包含收取事件 id，实际：%s", msg)
	}
	if !strings.Contains(msg, "<event") {
		t.Fatalf("聊天正文必须是带 <event> 的 XML，实际：%s", msg)
	}
}

// ---------------- 公会战 ----------------

// TestGuildWarDeclareAndList 宣战后双方都能查到战列表，且金币被扣。
func TestGuildWarDeclareAndList(t *testing.T) {
	f := newP2Fixture(t)
	leaderDBID := f.mkChar(t, "leader01", "会长甲")
	enemyDBID := f.mkChar(t, "enemy001", "会长乙")

	// 建两个公会
	if _, code, err := f.Guilds.Create(f.ctx, leaderDBID, "战狼会"); err != nil || code != storage.GuildErrNone {
		t.Fatalf("create guild A: %v code=%d", err, code)
	}
	if _, code, err := f.Guilds.Create(f.ctx, enemyDBID, "雄鹰会"); err != nil || code != storage.GuildErrNone {
		t.Fatalf("create guild B: %v code=%d", err, code)
	}
	_ = f.Golds.Set(f.ctx, leaderDBID, 50000)

	leader := &Player{ChannelID: 21, DBID: leaderDBID, UID: playerUIDOf(leaderDBID), Name: "会长甲", Gold: 50000}
	players := NewPlayerManager()
	players.Add(leader)
	broadcast := &captureBroadcaster{}

	m := NewGuildWarManager(players, broadcast, f.Guilds, f.Golds)
	now := time.Now().Unix()

	code, err := m.DeclareWar(f.ctx, leaderDBID, "雄鹰会", 20000, now)
	if err != nil {
		t.Fatalf("declare war: %v", err)
	}
	if code != storage.GuildErrNone {
		t.Fatalf("宣战应该成功，错误码 %d", code)
	}
	// 金币被扣
	if leader.Gold != 30000 {
		t.Fatalf("宣战后金币期望 30000，实际 %d", leader.Gold)
	}

	// 战列表能查到
	mine, found, err := f.Guilds.LoadMembership(f.ctx, leaderDBID)
	if err != nil || !found {
		t.Fatalf("load membership: %v found=%v", err, found)
	}
	wars, err := f.Guilds.LoadWarList(f.ctx, mine.GuildID, now)
	if err != nil {
		t.Fatalf("load war list: %v", err)
	}
	if len(wars) != 1 {
		t.Fatalf("应该有 1 场战争，实际 %d", len(wars))
	}
	if wars[0].GuildName != "雄鹰会" {
		t.Fatalf("敌对会名期望「雄鹰会」，实际 %q", wars[0].GuildName)
	}
	// 时长是 1 小时
	if want := uint64(now + GuildWarDurationSec); wars[0].EndTime != want {
		t.Fatalf("结束时间期望 %d，实际 %d", want, wars[0].EndTime)
	}

	// AtWar 双向都成立
	if !m.AtWar(f.ctx, leaderDBID, enemyDBID) {
		t.Fatal("AtWar 应该返回 true")
	}
	if m.AtWar(f.ctx, leaderDBID, leaderDBID) {
		t.Fatal("AtWar 对自己应该返回 false")
	}

	// 推送的列表包必须发出去
	if !broadcast.has(protocol.SM_GUILDWARLIST) {
		t.Fatal("宣战后必须推 SM_GUILDWARLIST")
	}
}

// TestGuildWarDeclareRejectsBadCases 三种失败路径都不扣钱。
func TestGuildWarDeclareRejectsBadCases(t *testing.T) {
	f := newP2Fixture(t)
	leaderDBID := f.mkChar(t, "leader02", "会长丙")
	otherDBID := f.mkChar(t, "other001", "散人丁")
	if _, code, err := f.Guilds.Create(f.ctx, leaderDBID, "测试会"); err != nil || code != storage.GuildErrNone {
		t.Fatalf("create guild: %v", err)
	}

	leader := &Player{ChannelID: 22, DBID: leaderDBID, UID: playerUIDOf(leaderDBID), Name: "会长丙", Gold: 100}
	players := NewPlayerManager()
	players.Add(leader)
	broadcast := &captureBroadcaster{}
	m := NewGuildWarManager(players, broadcast, f.Guilds, f.Golds)
	now := time.Now().Unix()

	// 1. 金币不够（要 1000，只有 100）→ NOGOLD，不扣钱
	code, err := m.DeclareWar(f.ctx, leaderDBID, "不存在的会", 1000, now)
	if err != nil {
		t.Fatalf("declare: %v", err)
	}
	if code != storage.GuildErrNoGold {
		t.Fatalf("金币不够应该返回 NOGOLD，实际 %d", code)
	}
	if leader.Gold != 100 {
		t.Fatalf("失败时不该扣钱，实际 %d", leader.Gold)
	}

	// 2. 目标会不存在 → BADTARGET
	leader.Gold = 100000
	code, err = m.DeclareWar(f.ctx, leaderDBID, "不存在的会", 1000, now)
	if err != nil {
		t.Fatalf("declare: %v", err)
	}
	if code != storage.GuildErrBadTarget {
		t.Fatalf("目标会不存在应该返回 BADTARGET，实际 %d", code)
	}
	if leader.Gold != 100000 {
		t.Fatalf("失败时不该扣钱，实际 %d", leader.Gold)
	}

	// 3. 对自己宣战 → SELFWAR
	code, err = m.DeclareWar(f.ctx, leaderDBID, "测试会", 1000, now)
	if err != nil {
		t.Fatalf("declare: %v", err)
	}
	if code != storage.GuildErrSelfWar {
		t.Fatalf("对自己宣战应该返回 SELFWAR，实际 %d", code)
	}

	// 4. 非会长宣战 → NOTLEADER
	if code, err := f.Guilds.Invite(f.ctx, leaderDBID, otherDBID); err != nil || code != storage.GuildErrNone {
		t.Fatalf("invite: %v code=%d", err, code)
	}
	if code, err := f.Guilds.AcceptInvite(f.ctx, otherDBID, mustGuildID(t, f, leaderDBID)); err != nil || code != storage.GuildErrNone {
		t.Fatalf("accept invite: %v code=%d", err, code)
	}
	member := &Player{ChannelID: 23, DBID: otherDBID, UID: playerUIDOf(otherDBID), Name: "散人丁", Gold: 100000}
	players.Add(member)
	code, _ = m.DeclareWar(f.ctx, otherDBID, "测试会", 1000, now)
	if code != storage.GuildErrNotLeader {
		t.Fatalf("非会长宣战应该返回 NOTLEADER，实际 %d", code)
	}
}

// TestGuildWarNoPKPoint 公会战中杀敌不加 PK 点（playerop.cpp:671）。
func TestGuildWarNoPKPoint(t *testing.T) {
	f := newP2Fixture(t)
	aDBID := f.mkChar(t, "war00001", "战士甲")
	bDBID := f.mkChar(t, "war00002", "战士乙")
	if _, code, err := f.Guilds.Create(f.ctx, aDBID, "红方会"); err != nil || code != storage.GuildErrNone {
		t.Fatalf("create guild A: %v code=%d", err, code)
	}
	if _, code, err := f.Guilds.Create(f.ctx, bDBID, "蓝方会"); err != nil || code != storage.GuildErrNone {
		t.Fatalf("create guild B: %v code=%d", err, code)
	}

	players := NewPlayerManager()
	a := &Player{ChannelID: 31, DBID: aDBID, UID: playerUIDOf(aDBID), Name: "战士甲", Gold: 1 << 40}
	players.Add(a)
	broadcast := &captureBroadcaster{}
	m := NewGuildWarManager(players, broadcast, f.Guilds, f.Golds)
	now := time.Now().Unix()

	// 先不宣战 → AtWar 应为 false（会加 PK 点）
	if m.AtWar(f.ctx, aDBID, bDBID) {
		t.Fatal("还没宣战，AtWar 应该 false")
	}

	if code, err := m.DeclareWar(f.ctx, aDBID, "蓝方会", 0, now); err != nil || code != storage.GuildErrNone {
		t.Fatalf("declare war: %v code=%d", err, code)
	}
	// 宣战后 → AtWar 应为 true（不加 PK 点）
	if !m.AtWar(f.ctx, aDBID, bDBID) {
		t.Fatal("宣战后 AtWar 应该 true（公会战杀敌不算谋杀）")
	}
}

// ---------------- 公会邀请：走系统消息不是 SM_GUILDINFO ----------------

// TestGuildInviteSendsSystemMessage 邀请必须走系统聊天 XML，
// 而不是复用 SM_GUILDINFO（那个会让客户端以为「已在会里」）。
func TestGuildInviteSendsSystemMessage(t *testing.T) {
	f := newP2Fixture(t)
	leaderDBID := f.mkChar(t, "inv00001", "会长戊")
	targetDBID := f.mkChar(t, "inv00002", "路人己")
	guildID, code, err := f.Guilds.Create(f.ctx, leaderDBID, "邀请会")
	if err != nil || code != storage.GuildErrNone {
		t.Fatalf("create guild: %v code=%d", err, code)
	}

	players := NewPlayerManager()
	leader := &Player{ChannelID: 41, DBID: leaderDBID, UID: playerUIDOf(leaderDBID), Name: "会长戊"}
	target := &Player{ChannelID: 42, DBID: targetDBID, UID: playerUIDOf(targetDBID), Name: "路人己"}
	players.Add(leader)
	players.Add(target)
	broadcast := &captureBroadcaster{}

	h := &GuildHandler{
		Players: players, Broadcast: broadcast, Logger: discardLogger(),
		Repo: f.Guilds, Chat: f.Chat,
	}
	if err := h.Invite(f.ctx, leader.ChannelID, "路人己"); err != nil {
		t.Fatalf("invite: %v", err)
	}

	if broadcast.has(protocol.SM_GUILDINFO) {
		t.Fatal("邀请不该复用 SM_GUILDINFO（客户端会以为已在会里）")
	}
	if !broadcast.has(protocol.SM_CHATMESSAGELIST) {
		t.Fatal("邀请必须走系统聊天消息（原版 postSystemMessage）")
	}
	body, ok := broadcast.lastBody(protocol.SM_CHATMESSAGELIST)
	if !ok {
		t.Fatal("找不到聊天包体")
	}
	var list protocol.SDChatMessageList
	if err := protocol.CerealDeserialize(body, &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("期望 1 条消息，实际 %d", len(list))
	}
	msg := list[0].Message
	// 必须带 guildID（客户端回 CM_RESPONDGUILDINVITE 要用）
	if !strings.Contains(msg, "guild=\"") {
		t.Fatalf("邀请 XML 必须带 guild 属性，实际：%s", msg)
	}
	wantGuild := "guild=\"" + itoa(guildID) + "\""
	if !strings.Contains(msg, wantGuild) {
		t.Fatalf("邀请 XML 的 guild 应该是 %d，实际：%s", guildID, msg)
	}
	// 必须有同意/拒绝两个事件
	if strings.Count(msg, "<event") != 2 {
		t.Fatalf("应该有两个 <event>（同意/拒绝），实际：%s", msg)
	}
	if !strings.Contains(msg, "同意") || !strings.Contains(msg, "拒绝") {
		t.Fatalf("邀请 XML 缺少同意/拒绝，实际：%s", msg)
	}
}

// ---------------- 辅助 ----------------

// playerUIDOf 按 dbid 生成玩家 UID。
//
// ★★ 测试里必须给每个 Player 不同的 UID，否则会出现极其诡异的 bug。★★
//
//	PlayerManager 是 `byUID map[uint64]*Player` + `byChannel map[uint64]uint64`。
//	Add 时先写 byUID[p.UID]，再写 byChannel[ch] = p.UID。
//	两个玩家都用默认 UID=0 时：
//	  Add(A{ch:41, UID:0}) → byUID[0]=A, byChannel[41]=0
//	  Add(B{ch:42, UID:0}) → byUID[0]=B（**覆盖 A**）, byChannel[42]=0
//	于是 ByChannel(41) 返回的是 **B**，ByDBID(A) 也返回 B。
//
//	表现：邀请「路人己」时实际执行的是「路人己邀请自己」→ GUILDERR_BADTARGET（8）。
//	第一次排查这个测试时以为是 Invite 的业务逻辑错了，
//	实际是测试 fixture 自己把管理器搞脏了。
func playerUIDOf(dbid uint32) uint64 {
	return common.GetPlayerUID(dbid)
}

// has 判断是否发过某类型消息。
func (c *captureBroadcaster) has(id protocol.SMType) bool {
	for _, m := range c.msgs {
		if m.ID == id {
			return true
		}
	}
	return false
}

// lastBody 取某类型消息的最后一条包体（后发的覆盖前发的）。
//
// ★ 取「最后一条」而不是第一条：投递/邀请都可能先发别的再发目标消息，
// 断言最后一条才是「这次操作真正产生的结果」。
func (c *captureBroadcaster) lastBody(id protocol.SMType) ([]byte, bool) {
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if c.msgs[i].ID == id {
			return c.msgs[i].Payload, true
		}
	}
	return nil, false
}

func mustGuildID(t *testing.T, f *p2Fixture, dbid uint32) uint32 {
	t.Helper()
	member, found, err := f.Guilds.LoadMembership(f.ctx, dbid)
	if err != nil || !found {
		t.Fatalf("load membership for %d: %v found=%v", dbid, err, found)
	}
	return member.GuildID
}

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
