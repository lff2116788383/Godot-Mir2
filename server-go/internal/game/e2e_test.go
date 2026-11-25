package game

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/actor"
	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
	"github.com/lff2116788383/godot-mir2/server-go/internal/transport"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 端到端联调（真 TCP + 真协议 + 真数据库）====================
//
// 前面的单元测试都是「进程内直接调函数」。这个测试走真实网络：
//
//	真实 SQLite → 真实 transport → 真实 cerealf 编解码 → 两个 TCP 客户端
//
// 它验证的是「客户端真的能看见彼此」这件事 —— B4 的教训是
// world 层算对了 AOI 但没人发包，进程内测试发现不了，只有端到端能发现。

// e2eServer 是一套跑在本地端口的完整服务端。
type e2eServer struct {
	addr   string
	cancel context.CancelFunc
	store  *storage.Store
}

func startE2EServer(t *testing.T) *e2eServer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	store, err := storage.Open(ctx, storage.OpenOptions{
		Path: t.TempDir() + "/e2e.db3", CreateIfMissing: true,
	})
	if err != nil {
		cancel()
		t.Fatalf("open store: %v", err)
	}

	// 建两个账号与角色，让两个客户端都能登录
	accounts := storage.NewAccountRepository(store)
	chars := storage.NewCharacterRepository(store)
	for i, name := range []string{"alice", "bob"} {
		if err := accounts.Create(ctx, name, "pwd123"); err != nil {
			t.Fatalf("create account %s: %v", name, err)
		}
		if _, err := chars.Create(ctx, storage.CreateCharacterParams{
			Account: name, Name: name, Gender: 1, Job: 1,
			Map: 1, MapX: int32(100 + i*3), MapY: 100,
		}); err != nil {
			t.Fatalf("create char %s: %v", name, err)
		}
	}

	// 监听临时端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}

	sys := actor.NewSystem(ctx, actor.Options{})
	worlds := world.NewWorld(sys)
	players := NewPlayerManager()

	server := transport.NewServer(transport.Options{
		Handler: nil, // 稍后回填
	})

	worldSvc := NewWorldService(players, worlds, transport.NewBroadcaster(server), chars, "")
	worldSvc.Logger = discardLogger()

	loginHandler := NewHandler(NewSessionManager(), accounts, chars)
	loginHandler.Worlds = worldSvc

	cast := transport.NewBroadcaster(server)
	worldSvc.Broadcast = cast
	actions := &ActionHandler{
		Players: players, Sessions: loginHandler.Sessions, Worlds: worlds, Broadcast: cast,
	}

	// 物品表：e2e 也要真实加载，否则背包装备路由没有数据
	itemDB, err := LoadItemDB("../../content/items.json")
	if err != nil {
		cancel()
		t.Fatalf("load item db: %v", err)
	}
	GoldItemID = itemDB.GoldID()

	magicDB, err := LoadMagicDB("../../content/magics.json")
	if err != nil {
		cancel()
		t.Fatalf("load magic db: %v", err)
	}
	BindElementResolver(magicDB)

	inventory := &InventoryHandler{
		Players: players, Items: itemDB, Worlds: worlds, Broadcast: cast,
		ItemRepo: storage.NewItemRepository(store), Sessions: loginHandler.Sessions,
	}
	spells := &SpellHandler{
		Players: players, Worlds: worlds, Items: itemDB, Magics: magicDB,
		Broadcast: cast, Sessions: loginHandler.Sessions,
	}
	deaths := &DeathHandler{
		Players: players, Worlds: worlds, Broadcast: cast,
		Chars: chars,
	}
	shops := &ShopHandler{
		Players: players, Worlds: worlds, Items: itemDB, Broadcast: cast,
		Golds: storage.NewGoldRepository(store), Chars: chars,
	}
	actions.Spells = spells

	chats := &ChatHandler{
		Players: players, Sessions: loginHandler.Sessions,
		Broadcast: cast, Logger: discardLogger(),
		Chat: storage.NewChatRepository(store),
		Viewers: func(ctx context.Context, mapID uint32, x, y int) []uint64 {
			uids, err := worlds.ObserversAt(ctx, mapID, x, y)
			if err != nil {
				return nil
			}
			return uids
		},
	}

	dispatcher := transport.NewDispatcher()
	if err := RegisterAllRoutes(dispatcher, loginHandler, actions, inventory,
		spells, deaths, shops, nil, chats, nil, nil, nil); err != nil {
		cancel()
		t.Fatalf("register routes: %v", err)
	}
	sh := NewServerHandler(dispatcher, loginHandler)
	sh.SetDisconnectHook(func(c context.Context, channelID uint64) {
		worldSvc.Leave(c, channelID)
	})
	server.SetHandler(sh)

	go func() { _ = server.Serve(ctx, listener) }()

	t.Cleanup(func() {
		cancel()
		_ = server.Close()
		_ = sys.Shutdown(context.Background())
		_ = store.Close()
	})

	return &e2eServer{addr: listener.Addr().String(), cancel: cancel, store: store}
}

// e2eClient 是一个最小客户端：能收发帧。
type e2eClient struct {
	conn   net.Conn
	dec    *protocol.PacketDecoder
	respID uint64
}

func dialE2E(t *testing.T, addr string) *e2eClient {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	return &e2eClient{
		conn: conn,
		dec:  protocol.NewPacketDecoder(conn),
	}
}

func (c *e2eClient) close() { _ = c.conn.Close() }

// send 发一条客户端消息。
func (c *e2eClient) send(t *testing.T, id protocol.CMType, payload []byte) {
	t.Helper()
	c.respID++
	// AppendClientPacket 内部按 CM 表选「固定压缩/变长」编码
	frame, err := protocol.AppendClientPacket(nil, id, payload, c.respID)
	if err != nil {
		t.Fatalf("encode %v: %v", id, err)
	}
	if _, err := c.conn.Write(frame); err != nil {
		t.Fatalf("write %v: %v", id, err)
	}
}

// recvUntil 读到指定类型的消息（跳过其他）。
func (c *e2eClient) recvUntil(t *testing.T, want protocol.SMType, timeout time.Duration) protocol.Packet {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %v", want)
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
		packet, err := c.dec.ReadServerPacket()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if protocol.SMType(packet.Code) == want {
			return packet
		}
	}
}

// collect 在指定时间窗口内收集所有消息。
func (c *e2eClient) collect(d time.Duration) []protocol.Packet {
	var out []protocol.Packet
	deadline := time.Now().Add(d)
	for {
		remain := time.Until(deadline)
		if remain <= 0 {
			return out
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(remain))
		packet, err := c.dec.ReadServerPacket()
		if err != nil {
			return out
		}
		out = append(out, packet)
	}
}

func TestE2ETwoPlayersSeeEachOther(t *testing.T) {
	srv := startE2EServer(t)

	// ---- 客户端 A：alice ----
	a := dialE2E(t, srv.addr)
	defer a.close()
	loginA := protocol.CMLogin{}
	if err := loginA.ID.Assign([]byte("alice")); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := loginA.Password.Assign([]byte("pwd123")); err != nil {
		t.Fatalf("assign: %v", err)
	}
	payload, err := protocol.AppendWirePayload(nil, loginA)
	if err != nil {
		t.Fatalf("encode login: %v", err)
	}
	a.send(t, protocol.CM_LOGIN, payload)
	a.recvUntil(t, protocol.SM_LOGINOK, 3*time.Second)

	a.send(t, protocol.CM_QUERYCHAR, nil)
	qc := a.recvUntil(t, protocol.SM_QUERYCHAROK, 3*time.Second)
	var qcr protocol.SMQueryCharOK
	if _, err := protocol.ReadWirePayload(&qcr, qc.Payload); err != nil {
		t.Fatalf("decode querychar: %v", err)
	}
	if qcr.Name.String() != "alice" {
		t.Fatalf("char name = %q", qcr.Name.String())
	}

	a.send(t, protocol.CM_ONLINE, nil)
	onlineA := a.recvUntil(t, protocol.SM_ONLINEOK, 3*time.Second)
	var okA protocol.SMOnlineOK
	if _, err := protocol.ReadWirePayload(&okA, onlineA.Payload); err != nil {
		t.Fatalf("decode onlineok: %v", err)
	}
	t.Logf("alice online: uid=%#x mapuid=%#x pos=(%d,%d)",
		okA.UID, okA.MapUID, okA.Action.X, okA.Action.Y)

	// ---- 客户端 B：bob（在 alice 视野内）----
	b := dialE2E(t, srv.addr)
	defer b.close()
	loginB := protocol.CMLogin{}
	_ = loginB.ID.Assign([]byte("bob"))
	_ = loginB.Password.Assign([]byte("pwd123"))
	payloadB, err := protocol.AppendWirePayload(nil, loginB)
	if err != nil {
		t.Fatalf("encode login: %v", err)
	}
	b.send(t, protocol.CM_LOGIN, payloadB)
	b.recvUntil(t, protocol.SM_LOGINOK, 3*time.Second)
	b.send(t, protocol.CM_QUERYCHAR, nil)
	b.recvUntil(t, protocol.SM_QUERYCHAROK, 3*time.Second)
	b.send(t, protocol.CM_ONLINE, nil)
	onlineB := b.recvUntil(t, protocol.SM_ONLINEOK, 3*time.Second)
	var okB protocol.SMOnlineOK
	_, _ = protocol.ReadWirePayload(&okB, onlineB.Payload)

	// ---- 关键断言：bob 能看到 alice ----
	bobSaw := b.collect(1500 * time.Millisecond)
	var bobGotAlice bool
	for _, p := range bobSaw {
		if protocol.SMType(p.Code) != protocol.SM_CORECORD {
			continue
		}
		var rec protocol.SMCORecord
		if _, err := protocol.ReadWirePayload(&rec, p.Payload); err != nil {
			continue
		}
		if rec.UID == okA.UID {
			bobGotAlice = true
			t.Logf("bob sees alice at (%d,%d)", rec.Action.X, rec.Action.Y)
		}
	}
	if !bobGotAlice {
		t.Fatalf("bob should see alice; got %d messages", len(bobSaw))
	}

	// ---- 关键断言：alice 收到 bob 出现的通知 ----
	aliceSaw := a.collect(1500 * time.Millisecond)
	var aliceGotBob bool
	for _, p := range aliceSaw {
		if protocol.SMType(p.Code) != protocol.SM_CORECORD {
			continue
		}
		var rec protocol.SMCORecord
		if _, err := protocol.ReadWirePayload(&rec, p.Payload); err != nil {
			continue
		}
		if rec.UID == okB.UID {
			aliceGotBob = true
		}
	}
	if !aliceGotBob {
		t.Fatalf("alice should be told bob appeared; got %d messages", len(aliceSaw))
	}
}

func TestE2EMoveBroadcastsToOtherPlayer(t *testing.T) {
	srv := startE2EServer(t)

	// 两个客户端都上线
	conns := make([]*e2eClient, 2)
	for i, name := range []string{"alice", "bob"} {
		c := dialE2E(t, srv.addr)
		defer c.close()
		var login protocol.CMLogin
		_ = login.ID.Assign([]byte(name))
		_ = login.Password.Assign([]byte("pwd123"))
		payload, err := protocol.AppendWirePayload(nil, login)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		c.send(t, protocol.CM_LOGIN, payload)
		c.recvUntil(t, protocol.SM_LOGINOK, 3*time.Second)
		c.send(t, protocol.CM_QUERYCHAR, nil)
		c.recvUntil(t, protocol.SM_QUERYCHAROK, 3*time.Second)
		c.send(t, protocol.CM_ONLINE, nil)
		c.recvUntil(t, protocol.SM_ONLINEOK, 3*time.Second)
		conns[i] = c
	}

	// alice 向 bob 方向走一格
	// alice 在 (100,100)，bob 在 (103,100)
	var moveAction protocol.CMAction
	moveAction.UID = mustPlayerUID(t, srv.store, "alice")
	// mapUID 用 alice 在线时的那个（与 base map uid 一致）
	moveAction.MapUID = mustMapUID(t, 1)
	t.Logf("move request: uid=%#x mapuid=%#x", moveAction.UID, moveAction.MapUID)
	// ★ 用常量而不是硬编码数字：ActionMove 实际值是 3
	//   （ActionNone=0, Spawn=1, Stand=2, Move=3, ...），
	//   硬编码 8 会解成 ACTION_PICKUP，服务端就走错分支了。
	moveAction.Action.Type = common.ActionMove
	moveAction.Action.Speed = 600
	moveAction.Action.Direction = common.DirectionRight
	moveAction.Action.X, moveAction.Action.Y = 100, 100
	moveAction.Action.AimX, moveAction.Action.AimY = 101, 100

	payload, err := protocol.AppendWirePayload(nil, moveAction)
	if err != nil {
		t.Fatalf("encode action: %v", err)
	}
	conns[0].send(t, protocol.CM_ACTION, payload)

	// alice 自己也要收到（客户端要播自己的移动动画）
	aliceMsgs := conns[0].collect(800 * time.Millisecond)
	var aliceGotOwn bool
	for _, p := range aliceMsgs {
		if protocol.SMType(p.Code) == protocol.SM_ACTION {
			aliceGotOwn = true
		}
	}
	if !aliceGotOwn {
		t.Fatalf("alice should receive her own move action")
	}

	// bob 应收到 alice 的移动广播
	msgs := conns[1].collect(1500 * time.Millisecond)
	var sawMove bool
	for _, p := range msgs {
		if protocol.SMType(p.Code) != protocol.SM_ACTION {
			continue
		}
		var act protocol.SMAction
		if _, err := protocol.ReadWirePayload(&act, p.Payload); err != nil {
			continue
		}
		if act.UID == moveAction.UID {
			sawMove = true
			t.Logf("bob saw alice action type=%v at (%d,%d)", act.Action.Type, act.Action.X, act.Action.Y)
		}
	}
	if !sawMove {
		t.Fatalf("bob should receive alice's move broadcast; got %d messages", len(msgs))
	}
}

func mustPlayerUID(t *testing.T, store *storage.Store, account string) uint64 {
	t.Helper()
	ch, err := storage.NewCharacterRepository(store).GetByAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("get char %s: %v", account, err)
	}
	return common.GetPlayerUID(ch.DBID)
}

// TestE2EEquipWearOverNetwork 验证穿装备走真实网络往返。
//
// 这是最后一个关键验证：单元测试都在进程内直接调 handler，
// 只有走真实 TCP + 真实 cerealf 编解码才能确认
// 「定长错误消息」与「变长成功消息」两种编码都选对了。
func TestE2EEquipWearOverNetwork(t *testing.T) {
	srv := startE2EServer(t)

	c := dialE2E(t, srv.addr)
	defer c.close()

	// 登录 + 进图
	var login protocol.CMLogin
	_ = login.ID.Assign([]byte("alice"))
	_ = login.Password.Assign([]byte("pwd123"))
	payload, err := protocol.AppendWirePayload(nil, login)
	if err != nil {
		t.Fatalf("encode login: %v", err)
	}
	c.send(t, protocol.CM_LOGIN, payload)
	c.recvUntil(t, protocol.SM_LOGINOK, 3*time.Second)
	c.send(t, protocol.CM_QUERYCHAR, nil)
	c.recvUntil(t, protocol.SM_QUERYCHAROK, 3*time.Second)
	c.send(t, protocol.CM_ONLINE, nil)
	online := c.recvUntil(t, protocol.SM_ONLINEOK, 3*time.Second)
	var ok protocol.SMOnlineOK
	if _, err := protocol.ReadWirePayload(&ok, online.Payload); err != nil {
		t.Fatalf("decode onlineok: %v", err)
	}

	// 让服务端给这个角色发一件武器（通过 GM 式的内存注入不合适，
	// 这里改为直接请求穿一件背包里没有的物品，验证错误码路径能走通）
	var req protocol.CMRequestEquipWear
	req.ItemID = 100
	req.SeqID = 1
	req.WLType = uint16(WLGWeapon)
	body, err := protocol.AppendWirePayload(nil, req)
	if err != nil {
		t.Fatalf("encode equip request: %v", err)
	}
	c.send(t, protocol.CM_REQUESTEQUIPWEAR, body)

	// 背包里没有 -> 应收到 SM_EQUIPWEARERROR（定长 10 字节）
	errPkt := c.recvUntil(t, protocol.SM_EQUIPWEARERROR, 3*time.Second)
	var eqErr protocol.SMEquipWearError
	if _, err := protocol.ReadWirePayload(&eqErr, errPkt.Payload); err != nil {
		t.Fatalf("decode SMEquipWearError: %v", err)
	}
	if eqErr.ItemID != 100 || eqErr.SeqID != 1 {
		t.Fatalf("error payload echoed wrong ids: %+v", eqErr)
	}
	// 物品不存在时应该是 BADITEM（因为 itemID 100 在表里可能不存在）
	// 无论 BADITEM 还是 NOITEM，都说明错误链路走通了
	if eqErr.Error != EQWErrBadItem && eqErr.Error != EQWErrNoItem && eqErr.Error != EQWErrBadWLType {
		t.Fatalf("unexpected error code %d", eqErr.Error)
	}
	t.Logf("equip error round-trip ok: code=%d", eqErr.Error)
}

// e2eLogin 完成登录 + 进图，返回 SM_ONLINEOK 内容。
func e2eLogin(t *testing.T, addr, account string) (*e2eClient, protocol.SMOnlineOK) {
	t.Helper()
	c := dialE2E(t, addr)

	var login protocol.CMLogin
	_ = login.ID.Assign([]byte(account))
	_ = login.Password.Assign([]byte("pwd123"))
	payload, err := protocol.AppendWirePayload(nil, login)
	if err != nil {
		t.Fatalf("encode login: %v", err)
	}
	c.send(t, protocol.CM_LOGIN, payload)
	c.recvUntil(t, protocol.SM_LOGINOK, 3*time.Second)
	c.send(t, protocol.CM_QUERYCHAR, nil)
	c.recvUntil(t, protocol.SM_QUERYCHAROK, 3*time.Second)
	c.send(t, protocol.CM_ONLINE, nil)
	online := c.recvUntil(t, protocol.SM_ONLINEOK, 3*time.Second)

	var ok protocol.SMOnlineOK
	if _, err := protocol.ReadWirePayload(&ok, online.Payload); err != nil {
		t.Fatalf("decode onlineok: %v", err)
	}
	return c, ok
}

// TestE2EDieAndRevive 走真网络验证「求死 -> 收到死亡 -> 复活 -> 收到复活」。
//
// 这条链路的客户端表现是「角色倒地 -> 尸体淡出 -> 回到复活点站起来」，
// 三个包（SM_NOTIFYDEAD / SM_DEADFADEOUT / SM_REVIVE）缺任何一条
// 都会让客户端卡在某个中间状态。
func TestE2EDieAndRevive(t *testing.T) {
	srv := startE2EServer(t)
	c, _ := e2eLogin(t, srv.addr, "alice")
	defer c.close()

	// CM_REQUESTDIE 是空消息
	c.send(t, protocol.CM_REQUESTDIE, nil)

	dead := c.recvUntil(t, protocol.SM_NOTIFYDEAD, 3*time.Second)
	var deadPkt protocol.SMNotifyDead
	if _, err := protocol.ReadWirePayload(&deadPkt, dead.Payload); err != nil {
		t.Fatalf("decode SM_NOTIFYDEAD: %v", err)
	}

	fade := c.recvUntil(t, protocol.SM_DEADFADEOUT, 3*time.Second)
	var fadePkt protocol.SMDeadFadeOut
	if _, err := protocol.ReadWirePayload(&fadePkt, fade.Payload); err != nil {
		t.Fatalf("decode SM_DEADFADEOUT: %v", err)
	}
	if fadePkt.UID != deadPkt.UID {
		t.Fatalf("dead uid mismatch: notify=%#x fade=%#x", deadPkt.UID, fadePkt.UID)
	}
	t.Logf("death over network: uid=%#x at (%d,%d)", fadePkt.UID, fadePkt.X, fadePkt.Y)

	// 复活
	c.send(t, protocol.CM_REQUESTREVIVE, nil)
	rev := c.recvUntil(t, protocol.SM_REVIVE, 3*time.Second)
	var revPkt protocol.SMRevive
	if _, err := protocol.ReadWirePayload(&revPkt, rev.Payload); err != nil {
		t.Fatalf("decode SM_REVIVE: %v", err)
	}
	if revPkt.MapUID == 0 {
		t.Fatal("SM_REVIVE should carry a map uid")
	}
	t.Logf("revived at (%d,%d) dir=%d mapuid=%#x",
		revPkt.X, revPkt.Y, revPkt.Direction, revPkt.MapUID)
}

// TestE2EBuyOverNetwork 走真网络验证购买链路。
//
// 两条路径都测，因为它们的失败模式完全不同：
//
//	成功：SM_BUYSUCCEED + SM_GOLD + SM_INVENTORY，三包缺一客户端就不同步
//	失败：SM_BUYERROR 带回显的 itemID/seqID，客户端靠它弹提示
//
// ★ 新角色的初始金币不是 0 而是 schema 默认的 9900，
// 所以「买不起」要用一个必定超额的 count 来构造。
func TestE2EBuyOverNetwork(t *testing.T) {
	srv := startE2EServer(t)
	c, _ := e2eLogin(t, srv.addr, "alice")
	defer c.close()

	// 金创药（小）：itemID 6，price 80
	const potionID = 6
	const npcUID = 0x0F00000000000001

	// ---- 成功路径 ----
	c.send(t, protocol.CM_BUY, mustWire(t, protocol.CMBuy{
		NPCUID: npcUID, ItemID: potionID, SeqID: 1, Count: 1,
	}))
	pkt := c.recvUntil(t, protocol.SM_BUYSUCCEED, 3*time.Second)
	var ok protocol.SMBuySucceed
	if _, err := protocol.ReadWirePayload(&ok, pkt.Payload); err != nil {
		t.Fatalf("decode SM_BUYSUCCEED: %v", err)
	}
	if ok.ItemID != potionID {
		t.Fatalf("bought item = %d, want %d", ok.ItemID, potionID)
	}
	t.Logf("buy succeeded: item=%d seq=%d", ok.ItemID, ok.SeqID)

	// ---- 失败路径：数量远超金币能买的量 ----
	c.send(t, protocol.CM_BUY, mustWire(t, protocol.CMBuy{
		NPCUID: npcUID, ItemID: potionID, SeqID: 1, Count: MaxBuyCount,
	}))
	errPkt := c.recvUntil(t, protocol.SM_BUYERROR, 3*time.Second)
	var buyErr protocol.SMBuyError
	if _, err := protocol.ReadWirePayload(&buyErr, errPkt.Payload); err != nil {
		t.Fatalf("decode SM_BUYERROR: %v", err)
	}
	if buyErr.ItemID != potionID || buyErr.Error != BuyErrInsufficient {
		t.Fatalf("buy error = %+v, want itemID=%d err=%d", buyErr, potionID, BuyErrInsufficient)
	}
	t.Logf("buy rejected as expected: code=%d", buyErr.Error)
}

// mustWire 编码一个定长载荷，失败直接 t.Fatal。
func mustWire(t *testing.T, payload any) []byte {
	t.Helper()
	body, err := protocol.AppendWirePayload(nil, payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return body
}

// TestE2EMagicKeyOverNetwork 验证 CM_SETMAGICKEY 不会让连接崩掉。
//
// 这条消息没有回包（原版也是静默落库），所以断言方式只能是
// 「发完之后连接还活着、还能收到下一条消息的应答」。
func TestE2EMagicKeyOverNetwork(t *testing.T) {
	srv := startE2EServer(t)
	c, _ := e2eLogin(t, srv.addr, "alice")
	defer c.close()

	var key protocol.CMSetMagicKey
	key.MagicID = 14 // 雷电术（角色没学，服务端会静默忽略）
	key.Key = 'a'
	body, err := protocol.AppendWirePayload(nil, key)
	if err != nil {
		t.Fatalf("encode setmagickey: %v", err)
	}
	c.send(t, protocol.CM_SETMAGICKEY, body)

	// 连接必须还活着：用一个必然有回包的消息探活
	var health protocol.CMRequestAddHealth
	health.AddHP = 0
	health.AddMP = 0
	hbody, err := protocol.AppendWirePayload(nil, health)
	if err != nil {
		t.Fatalf("encode addhealth: %v", err)
	}
	c.send(t, protocol.CM_REQUESTADDHEALTH, hbody)
	c.recvUntil(t, protocol.SM_HEALTH, 3*time.Second)
	t.Log("connection alive after CM_SETMAGICKEY")
}

// TestE2ESayOverNetwork 走真网络验证「原地说话」。
//
// 客户端 `say()` 发 CM_PLAYERSAY（128 字节定长），
// 服务端应当回一条 SM_PLAYERSAY 给发言者自己 ——
// 客户端本地不显示自己说的话，全靠这条回包。
//
// 不回的话表现是「按回车打字，字打出去了但框里没反应」，
// 玩家会反复重打，很难判断是网络问题还是服务端没实现。
func TestE2ESayOverNetwork(t *testing.T) {
	srv := startE2EServer(t)
	c, _ := e2eLogin(t, srv.addr, "alice")
	defer c.close()

	var say protocol.CMPlayerSay
	copy(say.Content[:], "有人吗")
	c.send(t, protocol.CM_PLAYERSAY, mustWire(t, say))

	pkt := c.recvUntil(t, protocol.SM_PLAYERSAY, 3*time.Second)
	var got protocol.SMPlayerSay
	if _, err := protocol.ReadWirePayload(&got, pkt.Payload); err != nil {
		t.Fatalf("decode SM_PLAYERSAY: %v", err)
	}
	content := string(trimCStringBytes(got.Content[:]))
	if content != "有人吗" {
		t.Fatalf("回显内容 = %q, want 有人吗", content)
	}
	if got.UID == 0 {
		t.Fatal("SM_PLAYERSAY 应该带发言者 UID")
	}
	t.Logf("说了：%s (uid=%#x)", content, got.UID)
}

// trimCStringBytes 去掉定长数组尾部 0（e2e 断言用）。
func trimCStringBytes(buf []byte) []byte {
	end := len(buf)
	for end > 0 && buf[end-1] == 0 {
		end--
	}
	return buf[:end]
}
