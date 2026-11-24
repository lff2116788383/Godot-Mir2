package game

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/actor"
	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== B5 测试夹具 ====================

// captureBroadcaster 记录所有下发消息，用于断言「服务端到底发了什么」。
//
// 为什么这是必要的：B4 的问题是 world 算对了 AOI 事件但没人发出去，
// 所以测试必须断言**网络包**，不能只断言内存状态。
type captureBroadcaster struct {
	msgs []capturedMsg
}

type capturedMsg struct {
	ChannelID uint64
	ID        protocol.SMType
	Payload   []byte
}

func (c *captureBroadcaster) SendTo(channelID uint64, id protocol.SMType, payload []byte, respID uint64) error {
	c.msgs = append(c.msgs, capturedMsg{
		ChannelID: channelID, ID: id, Payload: append([]byte(nil), payload...),
	})
	return nil
}

func (c *captureBroadcaster) reset() { c.msgs = nil }

// of 返回发给某连接、某类型的消息。
func (c *captureBroadcaster) of(channel uint64, id protocol.SMType) []capturedMsg {
	var out []capturedMsg
	for _, m := range c.msgs {
		if m.ChannelID == channel && m.ID == id {
			out = append(out, m)
		}
	}
	return out
}

func (c *captureBroadcaster) count(id protocol.SMType) int {
	n := 0
	for _, m := range c.msgs {
		if m.ID == id {
			n++
		}
	}
	return n
}

// b5Fixture 装配一套完整的 B5 环境（真实 SQLite + world actor + 玩家）。
type b5Fixture struct {
	Handler   *Handler
	Actions   *ActionHandler
	Worlds    *WorldService
	Players   *PlayerManager
	Sessions  *SessionManager
	Cast      *captureBroadcaster
	Store     *storage.Store
	Chars     *storage.CharacterRepository
	actorSys  *actor.System
	ctx       context.Context
	cancelFns []func()
}

func newB5Fixture(t *testing.T) *b5Fixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	store, err := storage.Open(ctx, storage.OpenOptions{
		Path: filepath.Join(t.TempDir(), "b5.db3"), CreateIfMissing: true,
	})
	if err != nil {
		cancel()
		t.Fatalf("open store: %v", err)
	}

	sys := actor.NewSystem(ctx, actor.Options{})
	worlds := world.NewWorld(sys)
	players := NewPlayerManager()
	sessions := NewSessionManager()
	cast := &captureBroadcaster{}
	chars := storage.NewCharacterRepository(store)

	worldSvc := NewWorldService(players, worlds, cast, chars, "")
	worldSvc.Logger = discardLogger()

	handler := NewHandler(sessions,
		storage.NewAccountRepository(store), chars)
	handler.Worlds = worldSvc

	actions := &ActionHandler{
		Players: players, Sessions: sessions, Worlds: worlds, Broadcast: cast,
	}

	f := &b5Fixture{
		Handler: handler, Actions: actions, Worlds: worldSvc,
		Players: players, Sessions: sessions, Cast: cast,
		Store: store, Chars: chars, actorSys: sys, ctx: ctx,
	}
	f.cancelFns = append(f.cancelFns, cancel)
	t.Cleanup(func() {
		for _, fn := range f.cancelFns {
			fn()
		}
		_ = sys.Shutdown(context.Background())
		_ = store.Close()
	})
	return f
}

// makePlayer 建一个已登录、已进世界的玩家。
func (f *b5Fixture) makePlayer(t *testing.T, dbid uint32, name string, x, y int) *Player {
	t.Helper()
	p := &Player{
		ChannelID: uint64(dbid), DBID: dbid,
		UID:   common.GetPlayerUID(dbid),
		MapID: 1, MapUID: mustMapUID(t, 1),
		Name: name, Gender: 1, Job: 1,
		Level: 10, HP: 100, MaxHP: 100, MP: 50, MaxMP: 50,
		ViewRange: 10, Direction: common.DirectionDown,
		X: x, Y: y, Wear: make(map[int]*Item),
	}
	_ = f.Sessions.Login(p.ChannelID, name)
	_ = f.Sessions.MarkOnline(p.ChannelID, dbid)

	if err := f.Worlds.LoadMap(f.ctx, 1); err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	if _, err := f.Worlds.Enter(f.ctx, storage.Character{
		DBID: dbid, Name: name, Gender: 1, Job: 1,
		Map: 1, MapX: int32(x), MapY: int32(y), HP: 100, MP: 50, ViewRange: 10,
	}, p.ChannelID); err != nil {
		t.Fatalf("Enter: %v", err)
	}
	// Enter 内部会新建 Player 对象，取实际登记的那个
	actual, ok := f.Players.ByChannel(p.ChannelID)
	if !ok {
		t.Fatalf("player not registered after Enter")
	}
	return actual
}

func mustMapUID(t *testing.T, mapID uint32) uint64 {
	t.Helper()
	uid, err := common.GetBaseMapUID(mapID, 0)
	if err != nil {
		t.Fatalf("GetBaseMapUID: %v", err)
	}
	return uid
}

// ---------- 进世界与 AOI 广播 ----------

func TestEnterWorldRegistersPlayer(t *testing.T) {
	f := newB5Fixture(t)
	p := f.makePlayer(t, 1, "alice", 100, 100)

	if p.UID != common.GetPlayerUID(1) {
		t.Fatalf("uid = %#x", p.UID)
	}
	if p.MapID != 1 {
		t.Fatalf("map = %d", p.MapID)
	}
	if p.MapUID != mustMapUID(t, 1) {
		t.Fatalf("map uid = %#x", p.MapUID)
	}
	if f.Players.Count() != 1 {
		t.Fatalf("players = %d, want 1", f.Players.Count())
	}

	// 地图里必须有对应实体
	e, ok, err := f.Worlds.Worlds.Entity(f.ctx, 1, p.UID)
	if err != nil || !ok {
		t.Fatalf("entity not in map: ok=%v err=%v", ok, err)
	}
	if e.X != 100 || e.Y != 100 {
		t.Fatalf("entity at (%d,%d), want (100,100)", e.X, e.Y)
	}
	if e.Type != world.EntityPlayer {
		t.Fatalf("entity type = %v, want player", e.Type)
	}
}

func TestSecondPlayerSeesFirstViaSnapshot(t *testing.T) {
	f := newB5Fixture(t)
	alice := f.makePlayer(t, 1, "alice", 100, 100)
	f.Cast.reset()

	// bob 出现在 alice 视野内
	bob := f.makePlayer(t, 2, "bob", 103, 100)

	// bob 应该收到 alice 的 CORECORD（进场快照）
	records := f.Cast.of(bob.ChannelID, protocol.SM_CORECORD)
	if len(records) == 0 {
		t.Fatalf("bob should receive SM_CORECORD for alice, got %d msgs total", len(f.Cast.msgs))
	}

	// alice 也应该收到 bob 出现的通知（COAppear 事件）
	aliceRecords := f.Cast.of(alice.ChannelID, protocol.SM_CORECORD)
	if len(aliceRecords) == 0 {
		t.Fatalf("alice should be told bob appeared, got %d msgs", len(f.Cast.msgs))
	}

	// 校验 CORECORD 内容：bob 看到的是 alice 的记录
	var sawAlice bool
	for _, m := range records {
		var rec protocol.SMCORecord
		if _, err := protocol.ReadWirePayload(&rec, m.Payload); err != nil {
			t.Fatalf("decode SMCORecord: %v", err)
		}
		if rec.UID == alice.UID {
			sawAlice = true
			if rec.Action.X != int16(100) || rec.Action.Y != int16(100) {
				t.Fatalf("alice position in record = (%d,%d)", rec.Action.X, rec.Action.Y)
			}
			// 玩家 union: dbid 在低 4 字节
			gotDBID := uint32(rec.Union[0]) | uint32(rec.Union[1])<<8 |
				uint32(rec.Union[2])<<16 | uint32(rec.Union[3])<<24
			if gotDBID != 1 {
				t.Fatalf("union dbid = %d, want 1", gotDBID)
			}
			if rec.Union[4] != uint8(alice.Level) {
				t.Fatalf("union level = %d, want %d", rec.Union[4], alice.Level)
			}
		}
	}
	if !sawAlice {
		t.Fatal("bob's snapshot should include alice")
	}
}

func TestEnterWorldFallsBackToFlatMap(t *testing.T) {
	// assetsRoot 为空时应退回 200x200 平地，而不是失败
	f := newB5Fixture(t)
	p := f.makePlayer(t, 1, "alice", 50, 50)
	if p.X != 50 || p.Y != 50 {
		t.Fatalf("spawn = (%d,%d)", p.X, p.Y)
	}
}

// ---------- 动作处理 ----------

func TestActionRejectsForeignUID(t *testing.T) {
	f := newB5Fixture(t)
	p := f.makePlayer(t, 1, "alice", 100, 100)
	f.Cast.reset()

	// 冒充别人 UID 的动作必须被忽略（playernet.cpp:36-38）
	err := f.Actions.CMAction(f.ctx, p.ChannelID, protocol.CMAction{
		UID:    common.GetPlayerUID(999),
		MapUID: p.MapUID,
		Action: common.ActionNode{Type: common.ActionStand, X: 100, Y: 100},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}
	if len(f.Cast.msgs) != 0 {
		t.Fatalf("foreign action should be ignored, got %d msgs", len(f.Cast.msgs))
	}
}

func TestActionRejectsForeignMapUID(t *testing.T) {
	f := newB5Fixture(t)
	p := f.makePlayer(t, 1, "alice", 100, 100)
	f.Cast.reset()

	err := f.Actions.CMAction(f.ctx, p.ChannelID, protocol.CMAction{
		UID:    p.UID,
		MapUID: mustMapUID(t, 99), // 别的地图
		Action: common.ActionNode{Type: common.ActionStand, X: 100, Y: 100},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}
	if len(f.Cast.msgs) != 0 {
		t.Fatalf("foreign map action should be ignored, got %d msgs", len(f.Cast.msgs))
	}
}

func TestActionStandReportsBack(t *testing.T) {
	f := newB5Fixture(t)
	p := f.makePlayer(t, 1, "alice", 100, 100)
	f.Cast.reset()

	err := f.Actions.CMAction(f.ctx, p.ChannelID, protocol.CMAction{
		UID:    p.UID,
		MapUID: p.MapUID,
		Action: common.ActionNode{
			Type: common.ActionStand, Direction: common.DirectionDown,
			X: 100, Y: 100, AimX: 100, AimY: 100,
		},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}

	actions := f.Cast.of(p.ChannelID, protocol.SM_ACTION)
	if len(actions) != 1 {
		t.Fatalf("should send exactly one SM_ACTION, got %d", len(actions))
	}
	var act protocol.SMAction
	if _, err := protocol.ReadWirePayload(&act, actions[0].Payload); err != nil {
		t.Fatalf("decode SMAction: %v", err)
	}
	if act.UID != p.UID {
		t.Fatalf("action uid = %#x, want %#x", act.UID, p.UID)
	}
	if act.Action.Type != common.ActionStand {
		t.Fatalf("action type = %v, want STAND", act.Action.Type)
	}
}

func TestActionMoveSucceedsAndBroadcasts(t *testing.T) {
	f := newB5Fixture(t)
	p := f.makePlayer(t, 1, "alice", 100, 100)
	f.Cast.reset()

	// 客户端发「从 (100,100) 走到 (101,100)」
	err := f.Actions.CMAction(f.ctx, p.ChannelID, protocol.CMAction{
		UID:    p.UID,
		MapUID: p.MapUID,
		Action: common.ActionNode{
			Type: common.ActionMove, Speed: 600, Direction: common.DirectionRight,
			X: 100, Y: 100, AimX: 101, AimY: 100,
		},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}

	if p.X != 101 || p.Y != 100 {
		t.Fatalf("player at (%d,%d), want (101,100)", p.X, p.Y)
	}

	actions := f.Cast.of(p.ChannelID, protocol.SM_ACTION)
	if len(actions) == 0 {
		t.Fatal("move should broadcast SM_ACTION")
	}
	var act protocol.SMAction
	_, _ = protocol.ReadWirePayload(&act, actions[len(actions)-1].Payload)
	if act.Action.Type != common.ActionMove {
		t.Fatalf("broadcast type = %v, want MOVE", act.Action.Type)
	}

	// 世界侧实体也要跟着移动
	e, ok, _ := f.Worlds.Worlds.Entity(f.ctx, 1, p.UID)
	if !ok || e.X != 101 {
		t.Fatalf("world entity = (%d,%d) ok=%v", e.X, e.Y, ok)
	}
}

func TestActionMoveRejectedWhenBlocked(t *testing.T) {
	f := newB5Fixture(t)
	a := f.makePlayer(t, 1, "alice", 100, 100)
	b := f.makePlayer(t, 2, "bob", 101, 100)
	f.Cast.reset()

	// alice 想走到 bob 站着的位置
	err := f.Actions.CMAction(f.ctx, a.ChannelID, protocol.CMAction{
		UID:    a.UID,
		MapUID: a.MapUID,
		Action: common.ActionNode{
			Type: common.ActionMove, Speed: 600, Direction: common.DirectionRight,
			X: 100, Y: 100, AimX: 101, AimY: 100,
		},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}

	// 位置不能变
	if a.X != 100 || a.Y != 100 {
		t.Fatalf("alice should stay at (100,100), got (%d,%d)", a.X, a.Y)
	}
	// 必须回站立让客户端收敛（否则客户端卡在移动动画）
	var gotStand bool
	for _, m := range f.Cast.of(a.ChannelID, protocol.SM_ACTION) {
		var act protocol.SMAction
		if _, err := protocol.ReadWirePayload(&act, m.Payload); err != nil {
			continue
		}
		if act.Action.Type == common.ActionStand {
			gotStand = true
		}
	}
	if !gotStand {
		t.Fatalf("blocked move should report stand, msgs=%d", len(f.Cast.msgs))
	}
	_ = b
}

func TestActionMoveMultiHopRejected(t *testing.T) {
	// 服务端不做寻路：超过一跳的移动必须拒绝（player.cpp:1642 的 default 分支）
	f := newB5Fixture(t)
	p := f.makePlayer(t, 1, "alice", 100, 100)
	f.Cast.reset()

	err := f.Actions.CMAction(f.ctx, p.ChannelID, protocol.CMAction{
		UID:    p.UID,
		MapUID: p.MapUID,
		Action: common.ActionNode{
			Type: common.ActionMove, Speed: 600,
			X: 100, Y: 100, AimX: 105, AimY: 105, // 5 格远
		},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}
	if p.X != 100 || p.Y != 100 {
		t.Fatalf("multi-hop should be rejected, player at (%d,%d)", p.X, p.Y)
	}
}

func TestActionStandResyncsPosition(t *testing.T) {
	// 客户端报的坐标与服务器不一致时，服务端应该尝试把它挪过去
	f := newB5Fixture(t)
	p := f.makePlayer(t, 1, "alice", 100, 100)
	f.Cast.reset()

	err := f.Actions.CMAction(f.ctx, p.ChannelID, protocol.CMAction{
		UID:    p.UID,
		MapUID: p.MapUID,
		Action: common.ActionNode{
			Type: common.ActionStand, Direction: common.DirectionRight,
			X: 101, Y: 100, AimX: 101, AimY: 100, // 相邻格
		},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}
	if p.X != 101 {
		t.Fatalf("stand should resync to (101,100), got (%d,%d)", p.X, p.Y)
	}
	if p.Direction != common.DirectionRight {
		t.Fatalf("direction = %d, want RIGHT", p.Direction)
	}
}

// ---------- 战斗 ----------

func TestActionAttackDamagesTarget(t *testing.T) {
	f := newB5Fixture(t)
	a := f.makePlayer(t, 1, "alice", 100, 100)
	b := f.makePlayer(t, 2, "bob", 101, 100)
	// 1 级战士 DC={1,3} 会被 AC={0,1} 吃成 0，测不出掉血。
	// 调高等级让伤害稳定大于护甲 —— 这里测的是链路，不是数值平衡。
	bumpLevel(t, f, a.UID, 30)
	f.Cast.reset()

	err := f.Actions.CMAction(f.ctx, a.ChannelID, protocol.CMAction{
		UID:    a.UID,
		MapUID: a.MapUID,
		Action: common.ActionNode{
			Type: common.ActionAttack, Direction: common.DirectionRight,
			X: 100, Y: 100, AimX: 101, AimY: 100, AimUID: b.UID,
		},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}

	// bob 应收到血量变更（SM_HEALTH）
	healths := f.Cast.of(b.ChannelID, protocol.SM_HEALTH)
	if len(healths) == 0 {
		t.Fatalf("bob should receive SM_HEALTH, got %d msgs total", len(f.Cast.msgs))
	}
	// SM_HEALTH 是 MessageVariable，body 是 cerealf 序列化的 SDHealth
	var h protocol.SDHealth
	if err := protocol.CerealDeserialize(healths[len(healths)-1].Payload, &h); err != nil {
		t.Fatalf("decode SDHealth: %v", err)
	}
	if h.UID != b.UID {
		t.Fatalf("health uid = %#x, want %#x", h.UID, b.UID)
	}
	if h.HP >= b.MaxHP {
		t.Fatalf("bob HP should drop below %d, got %d", b.MaxHP, h.HP)
	}

	// 攻击动作应广播
	if len(f.Cast.of(a.ChannelID, protocol.SM_ACTION)) == 0 {
		t.Fatal("attack should broadcast SM_ACTION")
	}
}

func TestActionAttackOutOfRangeReportsStand(t *testing.T) {
	f := newB5Fixture(t)
	a := f.makePlayer(t, 1, "alice", 100, 100)
	b := f.makePlayer(t, 2, "bob", 110, 100) // 10 格远
	f.Cast.reset()

	err := f.Actions.CMAction(f.ctx, a.ChannelID, protocol.CMAction{
		UID:    a.UID,
		MapUID: a.MapUID,
		Action: common.ActionNode{
			Type: common.ActionAttack,
			X:    100, Y: 100, AimX: 110, AimY: 100, AimUID: b.UID,
		},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}

	// 不能造成伤害
	if len(f.Cast.of(b.ChannelID, protocol.SM_HEALTH)) != 0 {
		t.Fatal("out-of-range attack should not damage")
	}
	// 必须回站立
	var gotStand bool
	for _, m := range f.Cast.of(a.ChannelID, protocol.SM_ACTION) {
		var act protocol.SMAction
		if _, err := protocol.ReadWirePayload(&act, m.Payload); err != nil {
			continue
		}
		if act.Action.Type == common.ActionStand {
			gotStand = true
		}
	}
	if !gotStand {
		t.Fatal("out-of-range attack should report stand")
	}
}

// ---------- 拾取 ----------

func TestActionPickUpAddsToInventory(t *testing.T) {
	f := newB5Fixture(t)
	p := f.makePlayer(t, 1, "alice", 100, 100)
	f.Cast.reset()

	// 在地上放一件物品
	if err := dropItemAt(f.ctx, f.Worlds.Worlds, 1, 100, 100, []uint32{555}); err != nil {
		t.Fatalf("drop: %v", err)
	}

	err := f.Actions.CMAction(f.ctx, p.ChannelID, protocol.CMAction{
		UID:    p.UID,
		MapUID: p.MapUID,
		Action: common.ActionNode{
			Type: common.ActionPickUp,
			X:    100, Y: 100, AimX: 100, AimY: 100,
		},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}

	if len(p.Inventory) != 1 || p.Inventory[0].ItemID != 555 {
		t.Fatalf("inventory = %+v, want one item 555", p.Inventory)
	}

	// 应推送背包给玩家
	if len(f.Cast.of(p.ChannelID, protocol.SM_INVENTORY)) == 0 {
		t.Fatal("pickup should push SM_INVENTORY")
	}
}

func TestPickUpTooFarRejected(t *testing.T) {
	f := newB5Fixture(t)
	p := f.makePlayer(t, 1, "alice", 100, 100)
	f.Cast.reset()

	if err := dropItemAt(f.ctx, f.Worlds.Worlds, 1, 110, 110, []uint32{555}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	err := f.Actions.CMAction(f.ctx, p.ChannelID, protocol.CMAction{
		UID:    p.UID,
		MapUID: p.MapUID,
		Action: common.ActionNode{
			Type: common.ActionPickUp,
			X:    100, Y: 100, AimX: 110, AimY: 110,
		},
	})
	if err != nil {
		t.Fatalf("CMAction: %v", err)
	}
	if len(p.Inventory) != 0 {
		t.Fatalf("far pickup should be rejected, inventory = %+v", p.Inventory)
	}
}

// ---------- 离开世界 ----------

func TestLeaveWorldRemovesAndBroadcasts(t *testing.T) {
	f := newB5Fixture(t)
	a := f.makePlayer(t, 1, "alice", 100, 100)
	b := f.makePlayer(t, 2, "bob", 101, 100)
	f.Cast.reset()

	f.Worlds.Leave(f.ctx, b.ChannelID)

	// bob 应从管理器移除
	if _, ok := f.Players.ByChannel(b.ChannelID); ok {
		t.Fatal("bob should be removed from PlayerManager")
	}
	// 地图实体也应移除
	if _, ok, _ := f.Worlds.Worlds.Entity(f.ctx, 1, b.UID); ok {
		t.Fatal("bob should be removed from map")
	}
	// alice 应收到 bob 消失的通知
	if len(f.Cast.of(a.ChannelID, protocol.SM_CODISAPPEAR)) == 0 {
		t.Fatalf("alice should get SM_CODISAPPEAR for bob, msgs=%d", len(f.Cast.msgs))
	}
}

// ---------- 属性 ----------

func TestPlayerCombatStatsByJob(t *testing.T) {
	warrior := PlayerCombatStats(1, 10)
	if warrior.DC.Max <= 0 {
		t.Fatalf("warrior DC should be positive, got %+v", warrior.DC)
	}
	if warrior.MC.Max != 0 {
		t.Fatalf("warrior should have no magic, got %+v", warrior.MC)
	}

	wizard := PlayerCombatStats(2, 10)
	if wizard.MC.Max <= 0 {
		t.Fatalf("wizard MC should be positive, got %+v", wizard.MC)
	}
	if wizard.MC.Max <= wizard.DC.Max {
		t.Fatalf("wizard magic should exceed physical: MC=%+v DC=%+v", wizard.MC, wizard.DC)
	}

	taoist := PlayerCombatStats(3, 10)
	if taoist.MC.Max <= 0 || taoist.DC.Max <= 0 {
		t.Fatalf("taoist should have both, got %+v", taoist)
	}

	// 等级越高属性越强
	low := PlayerCombatStats(1, 1)
	high := PlayerCombatStats(1, 50)
	if high.DC.Max <= low.DC.Max {
		t.Fatalf("higher level should have bigger DC: %d vs %d", high.DC.Max, low.DC.Max)
	}
}

func TestDisplayLevel(t *testing.T) {
	if got := DisplayLevel(0); got != 1 {
		t.Fatalf("DisplayLevel(0) = %d, want 1", got)
	}
	// 递增性
	prev := 0
	for _, exp := range []uint64{0, 100, 500, 2000, 10000, 100000} {
		lvl := DisplayLevel(exp)
		if lvl < prev {
			t.Fatalf("DisplayLevel not monotonic at exp=%d", exp)
		}
		prev = lvl
	}
	if DisplayLevel(100000) < 2 {
		t.Fatal("large exp should give higher level")
	}
}

func TestEstimateHop(t *testing.T) {
	cases := []struct {
		dx, dy, want int
	}{
		{0, 0, 0},
		{1, 0, 1},
		{0, 1, 1},
		{1, 1, 1},
		{2, 0, 2},
		{0, 2, 2},
	}
	for _, c := range cases {
		if got := estimateHop(10, 10, 10+c.dx, 10+c.dy); got != c.want {
			t.Fatalf("estimateHop(delta %d,%d) = %d, want %d", c.dx, c.dy, got, c.want)
		}
	}
}

func TestBuildInventoryPayloadRoundTrip(t *testing.T) {
	p := &Player{Inventory: []Item{
		{ItemID: 100, SeqID: 1, Count: 5},
		{ItemID: 200, SeqID: 2, Count: 1},
	}}
	payload, err := BuildInventoryPayload(p)
	if err != nil {
		t.Fatalf("BuildInventoryPayload: %v", err)
	}
	var inv protocol.SDInventory
	if err := protocol.CerealDeserialize(payload, &inv); err != nil {
		t.Fatalf("CerealDeserialize: %v", err)
	}
	if len(inv.ItemList) != 2 {
		t.Fatalf("items = %d, want 2", len(inv.ItemList))
	}
	if inv.ItemList[0].ItemID != 100 || inv.ItemList[0].Count != 5 {
		t.Fatalf("item0 = %+v", inv.ItemList[0])
	}
	if inv.ItemList[1].ItemID != 200 {
		t.Fatalf("item1 = %+v", inv.ItemList[1])
	}
}
