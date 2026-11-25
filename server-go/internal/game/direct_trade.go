package game

import (
	"context"
	"log/slog"
	"sync"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// ==================== 面对面交易 ====================
//
// 【依据】playernet.cpp:1680 net_CM_REQUESTDIRECTTRADE / :1834 UPDATEDIRECTTRADE /
// :1931 COMMITDIRECTTRADE / :1970 CANCELDIRECTTRADE
//
// 交易是个**双人协商状态机**，四个阶段：
//
//	REQUEST   甲请求交易           → 乙看到弹窗（SM_REQUESTDIRECTTRADE）
//	RESPOND   乙接受/拒绝          → 双方进入交易面板（SM_STARTDIRECTTRADE）
//	UPDATE    双方各自摆东西/改钱   → 对方看到变化（SM_UPDATEDIRECTTRADE）
//	COMMIT    双方各自点「确认」    → 两边都确认后结算（SM_COMPLETEDIRECTTRADE）
//
// 关键约束（这些是最容易漏的）：
//
//  1. **锁定后不能改**：一方点「锁定」后，自己不能再改物品/金币，
//     对方也不能改（否则会出现「你确认的是 A，实际成交的是 B」）。
//  2. **确认后不能改**：确认比锁定更严格，连解锁都不行。
//  3. **双方都确认才结算**：只有一方确认什么都不发生。
//  4. **确认后物品已经不属于自己**：结算时按「各自给出的清单」搬移，
//     不是按「确认那一刻的背包」—— 中间背包可能被别处改过。
//
// ★ 与原版的一个差异：
//
//	原版把状态放在两个 Player 各自的字段上，靠 AM_UPDATEDIRECTTRADE 同步。
//	Go 侧放在集中的 TradeManager 里（和组队同一个理由：
//	每个连接一个 goroutine，各持一份必然出现「两边看到的不一样」）。
//
//	代价是所有交易串行，但交易是低频操作，完全不是瓶颈。

// DirectTradeErrorCode 对应原版 DTRADEERR_*（protocoldef.hpp:222-238）。
const (
	DirectTradeErrNone             uint8 = 0
	DirectTradeErrBadTarget        uint8 = 1
	DirectTradeErrNotAllowed       uint8 = 2
	DirectTradeErrTargetNotAllowed uint8 = 3
	DirectTradeErrBusy             uint8 = 4
	DirectTradeErrTargetBusy       uint8 = 5
	DirectTradeErrDead             uint8 = 6
	DirectTradeErrTargetDead       uint8 = 7
	DirectTradeErrTooFar           uint8 = 8
	DirectTradeErrRejected         uint8 = 9
	DirectTradeErrOffline          uint8 = 10
	DirectTradeErrBadOffer         uint8 = 11
	DirectTradeErrBadGold          uint8 = 12
	DirectTradeErrLocked           uint8 = 13
	DirectTradeErrNotReady         uint8 = 14
	DirectTradeErrCommitFailed     uint8 = 15
	DirectTradeErrStallSelling     uint8 = 16
)

// MaxDirectTradeDistance 是能发起交易的最大距离（切比雪夫）。
//
// 原版用 getInViewCOPtr（视野内），这里收紧到 3 格：
// 视野内可能有十几格远，隔着半条街交易不合理，
// 也会被用来「远程交易」绕过面对面的设计意图。
const MaxDirectTradeDistance = 3

// DirectTrade 是一次进行中的交易。
type DirectTrade struct {
	// Initiator 发起者 UID（RESPOND 阶段决定谁先摆东西）
	Initiator uint64
	// A / B 双方 UID（A 是发起者）
	A, B uint64
	// OfferA / OfferB 各自的报价
	OfferA, OfferB protocol.SDDirectTradeOffer
	// Started 是否已进入交易面板（对方接受了）
	Started bool
}

// DirectTradeManager 掌管所有交易。
type DirectTradeManager struct {
	mu sync.Mutex
	// byUID UID -> 进行中的交易（双方都指向同一个 *DirectTrade）
	byUID map[uint64]*DirectTrade

	Players   *PlayerManager
	Broadcast Broadcaster
	Worlds    WorldLookup
	Logger    *slog.Logger
}

// WorldLookup 是判断「两个玩家距离」的最小接口。
type WorldLookup interface {
	EntityDistance(mapID uint32, uidA, uidB uint64) (int, bool)
}

// NewDirectTradeManager 创建交易管理器。
func NewDirectTradeManager(players *PlayerManager, broadcast Broadcaster, logger *slog.Logger) *DirectTradeManager {
	return &DirectTradeManager{
		byUID:     make(map[uint64]*DirectTrade),
		Players:   players,
		Broadcast: broadcast,
		Logger:    logger,
	}
}

// Request 发起交易（CM_REQUESTDIRECTTRADE）。
//
// 原版的校验顺序（playernet.cpp:1680-1717）值得照抄，因为每一步都有意义：
//
//  1. 摆摊中          → STALLSELLING
//  2. 目标不是玩家/是自己 → BADTARGET
//  3. 自己已死亡       → DEAD
//  4. 自己有交易在进行  → BUSY
//  5. 目标不在视野内    → TOOFAR
func (m *DirectTradeManager) Request(ctx context.Context, p *Player, targetUID uint64) uint8 {
	if targetUID == 0 || targetUID == p.UID {
		return DirectTradeErrBadTarget
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if p.HP <= 0 {
		return DirectTradeErrDead
	}
	if _, busy := m.byUID[p.UID]; busy {
		return DirectTradeErrBusy
	}
	target, ok := m.Players.Get(targetUID)
	if !ok {
		return DirectTradeErrOffline
	}
	if target.HP <= 0 {
		return DirectTradeErrTargetDead
	}
	if _, busy := m.byUID[targetUID]; busy {
		return DirectTradeErrTargetBusy
	}
	if !m.inRange(p, target) {
		return DirectTradeErrTooFar
	}

	trade := &DirectTrade{
		Initiator: p.UID,
		A:         p.UID,
		B:         targetUID,
		OfferA:    protocol.SDDirectTradeOffer{UID: p.UID},
		OfferB:    protocol.SDDirectTradeOffer{UID: targetUID},
	}
	m.byUID[p.UID] = trade
	m.byUID[targetUID] = trade

	// 通知目标：有人想和你交易
	var nameBuf common.StaticBuffer[common.Cap64]
	_ = nameBuf.Assign([]byte(p.Name))
	payload, err := protocol.AppendWirePayload(nil, protocol.SMRequestDirectTrade{
		UID:  p.UID,
		Name: nameBuf,
	})
	if err == nil {
		m.sendTo(target, protocol.SM_REQUESTDIRECTTRADE, payload)
	}
	return DirectTradeErrNone
}

// Respond 回应交易请求（CM_RESPONDDIRECTTRADE）。
func (m *DirectTradeManager) Respond(ctx context.Context, p *Player, peerUID uint64, accept bool) uint8 {
	m.mu.Lock()
	defer m.mu.Unlock()

	trade, ok := m.byUID[p.UID]
	if !ok || trade.B != p.UID || trade.Started {
		return DirectTradeErrBadTarget
	}
	if trade.A != peerUID {
		return DirectTradeErrBadTarget
	}
	if !accept {
		m.dropLocked(trade)
		m.sendError(p, DirectTradeErrRejected)
		// 通知发起者「对方拒绝了」
		if a, ok := m.Players.Get(trade.A); ok {
			m.sendTo(a, protocol.SM_CLOSEDIRECTTRADE, m.wire(protocol.SMCloseDirectTrade{UID: p.UID}))
		}
		return DirectTradeErrNone
	}

	// 再校验一次距离 —— 从请求到接受之间可能已经走远
	a, ok := m.Players.Get(trade.A)
	if !ok {
		m.dropLocked(trade)
		return DirectTradeErrOffline
	}
	if !m.inRange(a, p) {
		m.dropLocked(trade)
		m.sendError(p, DirectTradeErrTooFar)
		return DirectTradeErrNone
	}

	trade.Started = true

	// 双方都进入交易面板
	m.sendTo(a, protocol.SM_STARTDIRECTTRADE, m.wire(protocol.SMStartDirectTrade{UID: p.UID}))
	m.sendTo(p, protocol.SM_STARTDIRECTTRADE, m.wire(protocol.SMStartDirectTrade{UID: trade.A}))
	return DirectTradeErrNone
}

// Update 更新自己的报价（CM_UPDATEDIRECTTRADE）。
//
// 【依据】playernet.cpp:1834-1900。
//
// ★ 锁定后的规则：
//
//	自己已确认 → 完全不能再改（LOCKED）
//	自己已锁定 → 只能「保持锁定且内容不变」，改任何东西都要先解锁
//
//	第二条最容易被忽略：允许「锁定后悄悄改一样东西」会让
//	对方确认时看到的不是你锁定的内容。
func (m *DirectTradeManager) Update(ctx context.Context, p *Player, req protocol.SDDirectTradeOfferRequest) uint8 {
	m.mu.Lock()
	defer m.mu.Unlock()

	trade, ok := m.byUID[p.UID]
	if !ok || !trade.Started {
		return DirectTradeErrBadTarget
	}
	peerUID := trade.peerOf(p.UID)
	if req.UID != peerUID {
		return DirectTradeErrBadTarget
	}

	mine := trade.offerOf(p.UID)
	peer, peerOnline := m.Players.Get(peerUID)

	// 已确认：不能再动
	if mine.Confirmed {
		m.sendError(p, DirectTradeErrLocked)
		return DirectTradeErrNone
	}

	// 对方已确认时也不许自己改 —— 否则对方确认的内容会变
	if peerOnline {
		if peerOffer := trade.offerOf(peerUID); peerOffer.Confirmed {
			m.sendError(p, DirectTradeErrLocked)
			return DirectTradeErrNone
		}
	}

	// 距离/存活校验（原版在这也会 cancel）
	if p.HP <= 0 {
		m.cancelLocked(trade)
		m.sendError(p, DirectTradeErrDead)
		return DirectTradeErrNone
	}
	if !peerOnline {
		m.cancelLocked(trade)
		return DirectTradeErrOffline
	}
	if !m.inRange(p, peer) {
		m.cancelLocked(trade)
		m.sendError(p, DirectTradeErrTooFar)
		return DirectTradeErrNone
	}

	// 金币不能超过持有量
	if req.Gold > p.Gold {
		m.sendError(p, DirectTradeErrBadGold)
		return DirectTradeErrNone
	}

	// 已锁定：必须保持锁定且内容不变，否则先解锁
	if mine.Locked {
		if !req.Locked || !sameOfferItems(mine.ItemList, req.ItemList) || mine.Gold != req.Gold {
			m.sendError(p, DirectTradeErrLocked)
			return DirectTradeErrNone
		}
	}

	// 逐件校验：物品必须在背包里且数量够（原版 moveInventoryItem 会做）
	items, ok := m.resolveOfferItems(p, req.ItemList)
	if !ok {
		m.sendError(p, DirectTradeErrBadOffer)
		return DirectTradeErrNone
	}

	updated := protocol.SDDirectTradeOffer{
		UID:       p.UID,
		Gold:      req.Gold,
		Locked:    req.Locked,
		Confirmed: false, // 改了内容就重新回到未确认
		ItemList:  items,
	}
	if trade.A == p.UID {
		trade.OfferA = updated
	} else {
		trade.OfferB = updated
	}

	// 双方都看到新报价
	m.sendTo(p, protocol.SM_UPDATEDIRECTTRADE, m.cereal(updated))
	if peerOnline {
		m.sendTo(peer, protocol.SM_UPDATEDIRECTTRADE, m.cereal(updated))
	}
	return DirectTradeErrNone
}

// Commit 确认交易（CM_COMMITDIRECTTRADE）。
//
// 【依据】playernet.cpp:1931-1968。
//
// 双方都确认后才真正搬物品。判定用 directTradeReady：
// 双方都 locked 才能确认（原版要求「先锁定再确认」）。
func (m *DirectTradeManager) Commit(ctx context.Context, p *Player, peerUID uint64) uint8 {
	m.mu.Lock()
	defer m.mu.Unlock()

	trade, ok := m.byUID[p.UID]
	if !ok || !trade.Started {
		return DirectTradeErrBadTarget
	}
	if peerUID != trade.peerOf(p.UID) {
		return DirectTradeErrBadTarget
	}

	mine := trade.offerOf(p.UID)
	peerOffer := trade.offerOf(peerUID)

	// 双方都必须锁定（原版 directTradeReady）
	if !mine.Locked || !peerOffer.Locked {
		m.sendError(p, DirectTradeErrNotReady)
		return DirectTradeErrNone
	}

	if mine.Confirmed {
		return DirectTradeErrNone // 重复确认，静默
	}

	peer, peerOnline := m.Players.Get(peerUID)
	if !peerOnline {
		m.cancelLocked(trade)
		return DirectTradeErrOffline
	}
	if p.HP <= 0 || peer.HP <= 0 {
		m.cancelLocked(trade)
		m.sendError(p, DirectTradeErrDead)
		return DirectTradeErrNone
	}
	if !m.inRange(p, peer) {
		m.cancelLocked(trade)
		m.sendError(p, DirectTradeErrTooFar)
		return DirectTradeErrNone
	}

	// 标记自己已确认
	if trade.A == p.UID {
		trade.OfferA.Confirmed = true
		mine = trade.OfferA
	} else {
		trade.OfferB.Confirmed = true
		mine = trade.OfferB
	}
	m.sendTo(p, protocol.SM_UPDATEDIRECTTRADE, m.cereal(mine))
	m.sendTo(peer, protocol.SM_UPDATEDIRECTTRADE, m.cereal(mine))

	// 双方都确认了 → 结算
	if trade.OfferA.Confirmed && trade.OfferB.Confirmed {
		m.commitLocked(trade)
	}
	return DirectTradeErrNone
}

// Cancel 取消交易（CM_CANCELDIRECTTRADE）。
func (m *DirectTradeManager) Cancel(ctx context.Context, p *Player, peerUID uint64) uint8 {
	m.mu.Lock()
	defer m.mu.Unlock()

	trade, ok := m.byUID[p.UID]
	if !ok {
		return DirectTradeErrNone
	}
	if peerUID != trade.peerOf(p.UID) {
		return DirectTradeErrNone
	}
	m.cancelLocked(trade)
	return DirectTradeErrNone
}

// OnLeaveGame 玩家下线时取消其交易。
//
// ★ 必须调用：不清理的话对方会一直卡在「等待交易中」，
// 而且这个 UID 永远占着 byUID 索引，双方都再也发起不了新交易。
func (m *DirectTradeManager) OnLeaveGame(ctx context.Context, uid uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if trade, ok := m.byUID[uid]; ok {
		m.cancelLocked(trade)
	}
}

// ==================== 结算 ====================

// commitLocked 执行真正的物品/金币交换（调用方已持锁）。
//
// ★ 顺序很重要：
//
//  1. 先检查双方是否还有那些物品（可能中途被卖掉/寄存了）
//  2. 全部能交才一起交，任何一边不足就整体失败
//  3. 交完再给对方加
//
// 不先全检就逐个搬的话，第一名交了、第二名失败，
// 物品就凭空消失在一半的流程里。
func (m *DirectTradeManager) commitLocked(trade *DirectTrade) {
	a, aOK := m.Players.Get(trade.A)
	b, bOK := m.Players.Get(trade.B)
	if !aOK || !bOK {
		m.cancelLocked(trade)
		return
	}

	// 1. 校验双方物品与金币都还在
	if !canDeliver(a, trade.OfferA) || !canDeliver(b, trade.OfferB) {
		m.sendError(a, DirectTradeErrCommitFailed)
		m.sendError(b, DirectTradeErrCommitFailed)
		m.cancelLocked(trade)
		return
	}

	// 2. 一起交（此时不会再失败）
	takeOffer(a, trade.OfferA)
	takeOffer(b, trade.OfferB)
	giveOffer(a, trade.OfferB)
	giveOffer(b, trade.OfferA)

	// 3. 双方看到交易完成 + 背包/金币刷新
	m.sendTo(a, protocol.SM_COMPLETEDIRECTTRADE, m.wire(protocol.SMCompleteDirectTrade{UID: trade.B}))
	m.sendTo(b, protocol.SM_COMPLETEDIRECTTRADE, m.wire(protocol.SMCompleteDirectTrade{UID: trade.A}))
	m.pushInventoryAndGold(a)
	m.pushInventoryAndGold(b)

	m.dropLocked(trade)
}

// canDeliver 判断某人是否还持有报价里的全部物品与金币。
func canDeliver(p *Player, offer protocol.SDDirectTradeOffer) bool {
	if offer.Gold > p.Gold {
		return false
	}
	for _, want := range offer.ItemList {
		_, it := p.findItem(want.ItemID, want.SeqID)
		if it == nil {
			return false
		}
		if uint64(it.Count) < want.Count {
			return false
		}
	}
	return true
}

// takeOffer 从背包移除报价里的物品、扣掉金币。
func takeOffer(p *Player, offer protocol.SDDirectTradeOffer) {
	for _, want := range offer.ItemList {
		idx, it := p.findItem(want.ItemID, want.SeqID)
		if it == nil {
			continue
		}
		if uint64(it.Count) <= want.Count {
			p.Inventory = append(p.Inventory[:idx], p.Inventory[idx+1:]...)
			continue
		}
		p.Inventory[idx].Count -= uint32(want.Count)
	}
	if offer.Gold > 0 && p.Gold >= offer.Gold {
		p.Gold -= offer.Gold
	}
}

// giveOffer 把报价里的物品与金币给对方。
func giveOffer(p *Player, offer protocol.SDDirectTradeOffer) {
	for _, want := range offer.ItemList {
		p.Inventory = append(p.Inventory, Item{
			ItemID: want.ItemID, SeqID: uint64(want.SeqID), Count: uint32(want.Count),
			Durability:    uint32(want.Duration[0]),
			MaxDurability: uint32(want.Duration[1]),
		})
	}
	if offer.Gold > 0 {
		p.Gold += offer.Gold
		if p.Gold > MaxGold {
			p.Gold = MaxGold
		}
	}
}

// ==================== 内部 ====================

// peerOf 返回某人在交易里的对手。
func (t *DirectTrade) peerOf(uid uint64) uint64 {
	if uid == t.A {
		return t.B
	}
	return t.A
}

// offerOf 返回某人的报价（值拷贝，改它不会影响原状态）。
func (t *DirectTrade) offerOf(uid uint64) protocol.SDDirectTradeOffer {
	if uid == t.A {
		return t.OfferA
	}
	return t.OfferB
}

// resolveOfferItems 把「请求里声明的物品」转成带持久值的完整物品。
//
// ★ 必须从**玩家背包**里取持久值，不能信客户端传的：
//
//	客户端可以谎报 Duration，成交后对方拿到的是一件「假满持久」的装备。
func (m *DirectTradeManager) resolveOfferItems(p *Player, req []protocol.SDDirectTradeItem) ([]protocol.SDItem, bool) {
	out := make([]protocol.SDItem, 0, len(req))
	for _, want := range req {
		_, it := p.findItem(want.ItemID, want.SeqID)
		if it == nil {
			return nil, false
		}
		if uint64(it.Count) < want.Count || want.Count == 0 {
			return nil, false
		}
		item := toSDItem(*it)
		item.Count = want.Count // 只取声明的数量，其余（持久）以服务端为准
		out = append(out, item)
	}
	return out, true
}

// inRange 判断两个玩家是否在可交易距离内（同图 + 3 格）。
func (m *DirectTradeManager) inRange(a, b *Player) bool {
	if a.MapID != b.MapID {
		return false
	}
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	d := dx
	if dy > d {
		d = dy
	}
	return d <= MaxDirectTradeDistance
}

// sameOfferItems 判断两份物品清单是否完全一致（忽略顺序）。
func sameOfferItems(current []protocol.SDItem, req []protocol.SDDirectTradeItem) bool {
	if len(current) != len(req) {
		return false
	}
	for i := range current {
		if current[i].ItemID != req[i].ItemID ||
			current[i].SeqID != req[i].SeqID ||
			current[i].Count != req[i].Count {
			return false
		}
	}
	return true
}

// cancelLocked 取消交易并通知双方（调用方已持锁）。
func (m *DirectTradeManager) cancelLocked(trade *DirectTrade) {
	a, _ := m.Players.Get(trade.A)
	b, _ := m.Players.Get(trade.B)
	if a != nil {
		m.sendTo(a, protocol.SM_CLOSEDIRECTTRADE, m.wire(protocol.SMCloseDirectTrade{UID: trade.B}))
	}
	if b != nil {
		m.sendTo(b, protocol.SM_CLOSEDIRECTTRADE, m.wire(protocol.SMCloseDirectTrade{UID: trade.A}))
	}
	m.dropLocked(trade)
}

// dropLocked 只清索引，不发包（调用方已持锁）。
func (m *DirectTradeManager) dropLocked(trade *DirectTrade) {
	delete(m.byUID, trade.A)
	delete(m.byUID, trade.B)
}

func (m *DirectTradeManager) pushInventoryAndGold(p *Player) {
	if p == nil || m.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	list := make([]protocol.SDItem, 0, len(p.Inventory))
	for _, it := range p.Inventory {
		list = append(list, toSDItem(it))
	}
	if payload, err := protocol.CerealSerialize(protocol.SDInventory{ItemList: list}); err == nil {
		_ = m.Broadcast.SendTo(p.ChannelID, protocol.SM_INVENTORY, payload, 0)
	}
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMGold{Gold: p.Gold}); err == nil {
		_ = m.Broadcast.SendTo(p.ChannelID, protocol.SM_GOLD, payload, 0)
	}
}

func (m *DirectTradeManager) sendError(p *Player, code uint8) {
	if code == DirectTradeErrNone {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMDirectTradeError{Error: code})
	if err != nil {
		return
	}
	m.sendTo(p, protocol.SM_DIRECTTRADEERROR, payload)
}

func (m *DirectTradeManager) sendTo(p *Player, id protocol.SMType, payload []byte) {
	if m.Broadcast == nil || p == nil || p.ChannelID == 0 || payload == nil {
		return
	}
	_ = m.Broadcast.SendTo(p.ChannelID, id, payload, 0)
}

func (m *DirectTradeManager) wire(value any) []byte {
	payload, err := protocol.AppendWirePayload(nil, value)
	if err != nil {
		return nil
	}
	return payload
}

func (m *DirectTradeManager) cereal(value protocol.PortableMarshaler) []byte {
	payload, err := protocol.CerealSerialize(value)
	if err != nil {
		return nil
	}
	return payload
}

// ==================== 路由入口 ====================

func (m *DirectTradeManager) handleRequest(ctx context.Context, channelID uint64, targetUID uint64) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.sendError(p, m.Request(ctx, p, targetUID))
	return nil
}

func (m *DirectTradeManager) handleRespond(ctx context.Context, channelID uint64, peerUID uint64, accept bool) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.sendError(p, m.Respond(ctx, p, peerUID, accept))
	return nil
}

func (m *DirectTradeManager) handleUpdate(ctx context.Context, channelID uint64, req protocol.SDDirectTradeOfferRequest) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.sendError(p, m.Update(ctx, p, req))
	return nil
}

func (m *DirectTradeManager) handleCommit(ctx context.Context, channelID uint64, peerUID uint64) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.sendError(p, m.Commit(ctx, p, peerUID))
	return nil
}

func (m *DirectTradeManager) handleCancel(ctx context.Context, channelID uint64, peerUID uint64) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.sendError(p, m.Cancel(ctx, p, peerUID))
	return nil
}
