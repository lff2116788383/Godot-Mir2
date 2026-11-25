package game

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 世界时钟驱动 ====================
//
// 原版 ServerMap 有自己的 clock tick，所有「按时间发生的事」都挂在这个
// 时钟上：技能冷却、buff 到期、地面物品过期、火墙伤害、怪物刷新。
//
// Go 侧的地图状态也带 NowMS（world.MapState.NowMS），但**它不会自己走**：
// NowMS 只在 World.Advance 被调用时推进。B4 写好了 Advance 的语义，
// 却一直没人调用它 —— 于是 NowMS 恒为 0，直接导致：
//
//	技能冷却：nowMS(0) - last(0) = 0 < coolDown(800) -> 永远「冷却中」
//	地面物品：expireMS = 0 + 120000 -> 永远不过期
//	火墙：攻击间隔判断拿 0 去比 -> 从不攻击
//
// 所以必须有这个东西。它做的事很简单：每 TickInterval 调一次
// World.Advance，把返回的 TickEvent 转成网络消息。
//
// ★ 为什么不在 World 内部开 goroutine：
//
//	世界层是纯状态机（actor 串行执行 + 可注入时钟），
//	测试里能手动把时钟拨到任意时刻断言行为。
//	要是在 World 里塞个 time.Ticker，测试就只能靠 sleep，
//	既慢又不确定。时钟的「谁推进」是上层的事。

// TickInterval 是世界时钟的推进间隔。
//
// 100ms 是「够细也够省」的折中：
//
//	技能冷却最小 100ms（魔法盾 coolDown=100），50ms 粒度能分辨
//	客户端 10 秒 100 次火墙的伤害节奏。
//	再细化（比如 16ms）会让每张图每秒 60 次 actor 消息，
//	人多时这些消息会和真实玩家动作抢同一个 actor 队列。
const TickInterval = 100 * time.Millisecond

// MapTicker 驱动所有已加载地图的世界时钟。
type MapTicker struct {
	Worlds    *world.World
	WorldSvc  *WorldService
	Broadcast Broadcaster
	Players   *PlayerManager
	Logger    *slog.Logger
	// Kills 击杀结算器。
	//
	// 守卫打死怪、怪互相打死也要掉东西 —— 但 killer 不是玩家，
	// 所以经验不给任何人，只有掉落落地（Owner=0，谁都能捡）。
	Kills *KillSettler

	// interval 推进间隔（测试可改小/改大）
	interval time.Duration
	// now 时钟源（测试可注入，避免依赖真实时间）
	now func() int64

	mu     sync.Mutex
	mapIDs []uint32

	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewMapTicker 创建时钟驱动器。
func NewMapTicker(w *world.World, svc *WorldService, broadcast Broadcaster,
	players *PlayerManager, logger *slog.Logger) *MapTicker {
	return &MapTicker{
		Worlds:    w,
		WorldSvc:  svc,
		Broadcast: broadcast,
		Players:   players,
		Logger:    logger,
		interval:  TickInterval,
		now:       func() int64 { return time.Now().UnixMilli() },
	}
}

// SetInterval 改推进间隔（测试用）。
func (t *MapTicker) SetInterval(d time.Duration) {
	if d > 0 {
		t.interval = d
	}
}

// SetClock 注入时钟源（测试用）。
func (t *MapTicker) SetClock(fn func() int64) {
	if fn != nil {
		t.now = fn
	}
}

// Track 让 ticker 开始驱动某张地图。
//
// 地图是懒加载的（玩家进场时才 CreateMap），所以这里不能一次性
// 枚举完，必须在地图创建后调用。
func (t *MapTicker) Track(mapID uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range t.mapIDs {
		if id == mapID {
			return
		}
	}
	t.mapIDs = append(t.mapIDs, mapID)
}

// Start 启动后台推进协程。
func (t *MapTicker) Start(ctx context.Context) {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running = true
	t.stopCh = make(chan struct{})
	t.doneCh = make(chan struct{})
	t.mu.Unlock()

	go t.loop(ctx)
}

// Stop 停止推进并等协程退出。
func (t *MapTicker) Stop() {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return
	}
	t.running = false
	close(t.stopCh)
	done := t.doneCh
	t.mu.Unlock()
	<-done
}

func (t *MapTicker) loop(ctx context.Context) {
	defer close(t.doneCh)
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.Tick(ctx)
		}
	}
}

// Tick 推进一帧（也被测试直接调用来做确定性断言）。
func (t *MapTicker) Tick(ctx context.Context) {
	nowMS := t.now()

	t.mu.Lock()
	ids := append([]uint32(nil), t.mapIDs...)
	t.mu.Unlock()

	for _, mapID := range ids {
		events, err := t.Worlds.Advance(ctx, mapID, nowMS)
		if err != nil {
			continue
		}
		t.dispatch(mapID, events)
	}

	// 推进地图脚本的刷怪协程
	//
	// ★ 必须放在 Advance 之后：
	//
	//	Advance 会把 HP=0 的实体标记成死亡，刷怪协程下一轮
	//	判断 uidAlive 才能正确看到「这只怪死了，可以补刷」。
	//	反过来（先刷怪后判死）会让刷新延迟一个 tick，
	//	怪物密集的图上会明显感觉「怪死了半天不补」。
	if t.WorldSvc != nil && t.WorldSvc.MapScripts != nil {
		t.WorldSvc.MapScripts.Tick(nowMS)
	}
}

// dispatch 把 tick 事件转成网络消息。
//
// TickEvent.Kind 是字符串（world/map_state.go:496）：
//
//	"buff_expired"        buff 到期（BuffSeq 是 buff 的 seqID）
//	"spawn"               该刷怪了（Monster + 坐标）
//	"ground_item_expire"  地面物品过期
//	"firewall_hit"        火墙灼烧（UID 受击者 + Amount 伤害）
//	"firewall_expire"     火墙到期
//
// 本批只接 buff 到期与地面物品过期；火墙命中要 SM_STRIKE 的完整
// 字段（需要伤害类型/特效 ID），留给下一批。
func (t *MapTicker) dispatch(mapID uint32, events []world.TickEvent) {
	for _, ev := range events {
		switch ev.Kind {
		case "ground_item_expire":
			t.logGroundExpired(mapID, ev)
		case "buff_trigger":
			// buff 的定时效果（治愈术回血 / 施毒术掉血）已经由 world 层
			// 落到 Entity.HP 上了，这里只负责把新血量同步给客户端。
			t.syncAfterBuffTick(mapID, ev)
		case "buff_expired":
			// ★ 先重算属性，再推 SM_BUFF off。
			//
			//	顺序反了的话，客户端收到「buff 消失」但服务端属性还没回落，
			//	中间那一刻的伤害结算用的是带 buff 的数值 ——
			//	表现为「buff 图标没了但还多抗了一下」。
			t.recalcStatsFor(mapID, ev.UID)
			t.broadcastBuffOff(mapID, ev)
		case "ai_move":
			t.broadcastAIMove(mapID, ev)
		case "ai_attack":
			t.broadcastAIAttack(mapID, ev)
		case "corpse_fade":
			t.broadcastCorpseFade(mapID, ev)

		default:
			// 刷新/火墙由后续批次接
		}
	}
}

func (t *MapTicker) logGroundExpired(mapID uint32, ev world.TickEvent) {
	// 地面物品过期不推包：客户端按同样的 120 秒规则自己清理。
	// 这里只留痕，方便排查「东西哪去了」。
	if t.Logger != nil {
		t.Logger.Debug("ground item expired", "map", mapID, "x", ev.X, "y", ev.Y)
	}
}

func (t *MapTicker) broadcastBuffOff(mapID uint32, ev world.TickEvent) {
	if t.Broadcast == nil || ev.UID == 0 {
		return
	}
	// SMBuff.Type 是 buff 的 ID，TickEvent 里只有 BuffSeq（seqID）。
	// 两者的区别：同一个 buff ID 可以叠加多层，每层有独立 seqID。
	// 要按 seqID 找到 buff ID，得回地图查 —— 这一步留到接 buffrecord 时做，
	// 当前先用 seqID 低 16 位兜底（客户端按它匹配不到就不画 OFF 动画，
	// 但 buff 时长在客户端本地也会自然走完，不会卡住）。
	payload, err := protocol.AppendWirePayload(nil, protocol.SMBuff{
		UID: ev.UID, Type: uint32(ev.BuffSeq & 0xFFFF), State: 0, // 0 = OFF
	})
	if err != nil {
		return
	}
	for _, p := range t.Players.Snapshot() {
		if p.MapID != mapID {
			continue
		}
		_ = t.Broadcast.SendTo(p.ChannelID, protocol.SM_BUFF, payload, 0)
	}
}

// broadcastAIMove 广播怪物移动（SM_ACTION + ACTION_MOVE）。
//
// ★ 怪物移动必须走 SM_ACTION，不能只改服务端坐标：
//
//	客户端是靠 SM_ACTION 驱动角色动画的。只改坐标不发包，
//	地图上的怪会「瞬移」或者根本不动（客户端不知道它走过了）。
func (t *MapTicker) broadcastAIMove(mapID uint32, ev world.TickEvent) {
	entity, ok, err := t.Worlds.Entity(context.Background(), mapID, ev.UID)
	if err != nil || !ok {
		return
	}
	// 服务端已经把位置改好了，这里只需要通知
	payload, err2 := protocol.AppendWirePayload(nil, protocol.SMAction{
		UID:    ev.UID,
		MapUID: t.mapUID(mapID),
		Action: common.ActionNode{
			Type:      common.ActionMove,
			Direction: uint8(entity.Direction),
			X:         int16(entity.X),
			Y:         int16(entity.Y),
			AimX:      int16(entity.X),
			AimY:      int16(entity.Y),
		},
	})
	if err2 != nil {
		return
	}
	t.sendToMapObservers(mapID, ev.X, ev.Y, protocol.SM_ACTION, payload)
}

// broadcastAIAttack 广播怪物攻击（SM_ACTION + ACTION_ATTACK，命中则加 SM_STRIKE）。
//
// 顺序照原版：先动作、再伤害。
//
//	客户端看到攻击动作后才播打击特效；
//	反过来的话，玩家会先看到自己掉血、再看到怪抬手。
func (t *MapTicker) broadcastAIAttack(mapID uint32, ev world.TickEvent) {
	attacker, ok, err := t.Worlds.Entity(context.Background(), mapID, ev.UID)
	if err != nil || !ok {
		return
	}

	// 1. 攻击动作
	payload, err2 := protocol.AppendWirePayload(nil, protocol.SMAction{
		UID:    ev.UID,
		MapUID: t.mapUID(mapID),
		Action: common.ActionNode{
			Type:      common.ActionAttack,
			Direction: uint8(attacker.Direction),
			X:         int16(attacker.X),
			Y:         int16(attacker.Y),
			AimX:      int16(ev.X),
			AimY:      int16(ev.Y),
		},
	})
	if err2 == nil {
		t.sendToMapObservers(mapID, attacker.X, attacker.Y, protocol.SM_ACTION, payload)
	}

	// 2. 伤害（未命中不发 STRIKE，但要发 MISS）
	targetUID := parseUint64(ev.Monster)
	if targetUID == 0 {
		return
	}
	if ev.Amount > 0 {
		target, found, terr := t.Worlds.Entity(context.Background(), mapID, targetUID)
		if terr == nil && found {
			strike, serr := protocol.AppendWirePayload(nil, protocol.SMStrikeGrid{
				X: uint32(target.X), Y: uint32(target.Y),
			})
			if serr == nil {
				t.broadcastMap(mapID, protocol.SM_STRIKEGRID, strike)
			}
		}
	}

	// 3. 玩家被怪打，要同步血量到客户端
	if p, exists := t.Players.Get(targetUID); exists {
		t.syncPlayerHealth(mapID, p)
	}

	// 4. 被打死：广播死亡 + 结算掉落
	//
	// ★ 不结算的话，「守卫帮你打死半兽人」时地上的东西会凭空消失，
	//   玩家会以为被系统吞了。killer 不是玩家，所以走 SettleNoKiller：
	//   掉落落地（无归属保护），经验不给任何人。
	if ev.Dead {
		t.broadcastMap(mapID, protocol.SM_NOTIFYDEAD, encodeWire(protocol.SMNotifyDead{UID: targetUID}))
		t.broadcastMap(mapID, protocol.SM_DEADFADEOUT, encodeWire(protocol.SMDeadFadeOut{
			UID:    targetUID,
			MapUID: t.mapUID(mapID),
			X:      uint32(ev.X),
			Y:      uint32(ev.Y),
		}))
		if t.Kills != nil {
			t.Kills.SettleNoKiller(context.Background(), mapID, targetUID)
		}
	}
}

// encodeWire 编码定长载荷，失败返回 nil。
//
// ★ 不叫 mustWire：e2e_test.go 里已有同名辅助函数，
// 同包内重复定义会编译失败。
func encodeWire(value any) []byte {
	payload, err := protocol.AppendWirePayload(nil, value)
	if err != nil {
		return nil
	}
	return payload
}

// sendToMapObservers 只发给能看到这一格的玩家（AOI 精确投递）。
func (t *MapTicker) sendToMapObservers(mapID uint32, x, y int, id protocol.SMType, payload []byte) {
	if t.Broadcast == nil {
		return
	}
	observers, err := t.Worlds.ObserversAt(context.Background(), mapID, x, y)
	if err != nil {
		return
	}
	for _, uid := range observers {
		if p, ok := t.Players.Get(uid); ok && p.ChannelID != 0 {
			_ = t.Broadcast.SendTo(p.ChannelID, id, payload, 0)
		}
	}
}

// broadcastMap 发给地图上所有玩家（用于全局可见的事件）。
//
// ★ 为什么怪物攻击的 STRIKE 走这个而不是 AOI 精确投递：
//
//	打击特效在原版里是「谁在视野内谁看到」，但我们的 ObserversAt
//	返回的是「观察这一格的实体」，怪物与玩家不共格，容易漏发。
//	怪攻击的频率是每秒级（attackWait >= 1000），全图广播的量可控。
func (t *MapTicker) broadcastMap(mapID uint32, id protocol.SMType, payload []byte) {
	if t.Broadcast == nil {
		return
	}
	for _, p := range t.Players.Snapshot() {
		if p.MapID != mapID || p.ChannelID == 0 {
			continue
		}
		_ = t.Broadcast.SendTo(p.ChannelID, id, payload, 0)
	}
}

// syncPlayerHealth 把服务端血量推给玩家（被怪打后调用）。
func (t *MapTicker) syncPlayerHealth(mapID uint32, p *Player) {
	entity, ok, err := t.Worlds.Entity(context.Background(), mapID, p.UID)
	if err != nil || !ok {
		return
	}
	p.SyncFromWorld(entity)
	if t.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	payload, err2 := protocol.CerealSerialize(protocol.SDHealth{
		UID: p.UID, HP: p.HP, MP: p.MP, MaxHP: p.MaxHP, MaxMP: p.MaxMP,
	})
	if err2 != nil {
		return
	}
	_ = t.Broadcast.SendTo(p.ChannelID, protocol.SM_HEALTH, payload, 0)
}

// parseUint64 解析十进制无符号整数（失败返回 0）。
func parseUint64(s string) uint64 {
	if s == "" {
		return 0
	}
	var v uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0
		}
		v = v*10 + uint64(c-'0')
	}
	return v
}

// mapUID 取地图的 UID（从任意在线玩家那里拿，同图一致）。
func (t *MapTicker) mapUID(mapID uint32) uint64 {
	for _, p := range t.Players.Snapshot() {
		if p.MapID == mapID {
			return p.MapUID
		}
	}
	return 0
}

// broadcastCorpseFade 广播尸体消失（SM_CODISAPPEAR）。
//
// 客户端收到后会把这只怪从场景里删掉。
//
// ★ 只发 SM_CODISAPPEAR，不发 SM_DEADFADEOUT：
//
//	SM_NOTIFYDEAD / SM_DEADFADEOUT 在**死亡那一刻**就该发（客户端靠它
//	播倒地动画并开始淡出）。1 秒后再发一次 DEADFADEOUT 会让客户端
//	重新播一遍淡出 —— 表现是尸体闪一下又淡一次。
//
//	所以职责分工是：
//	  SM_NOTIFYDEAD     死亡瞬间 → 播倒地
//	  SM_DEADFADEOUT    死亡瞬间 → 开始淡出
//	  SM_CODISAPPEAR    1 秒后   → 从场景移除（本函数）
func (t *MapTicker) broadcastCorpseFade(mapID uint32, ev world.TickEvent) {
	if t.Broadcast == nil || ev.UID == 0 {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMCODisappear{
		UID:    ev.UID,
		MapUID: t.mapUID(mapID),
	})
	if err != nil {
		return
	}
	t.broadcastMap(mapID, protocol.SM_CODISAPPEAR, payload)
}

// recalcStatsFor 重算某个实体的属性（buff 变化后调用）。
//
// 玩家要走 Player 那条路（世界实体的 BaseStats 由 RefreshStats 维护）；
// 怪物没有 Player 对象，直接让 world 层从 BaseStats 重算。
func (t *MapTicker) recalcStatsFor(mapID uint32, uid uint64) {
	if t.Worlds == nil || uid == 0 {
		return
	}
	if p, ok := t.Players.Get(uid); ok && t.WorldSvc != nil && t.WorldSvc.Items != nil {
		// 玩家：基础属性由职业/等级/装备决定，重新算一遍更准
		_ = t.Worlds.RefreshStats(context.Background(), mapID, uid,
			p.CombatStatsFor(t.WorldSvc.Items))
		return
	}
	_ = t.Worlds.RecalcStats(context.Background(), mapID, uid)
}

// syncAfterBuffTick 把 buff 效果改过的血量同步给客户端。
//
// 玩家：走 Player.SyncFromWorld + SM_HEALTH
// 怪物：只推 SM_STRIKE 的打击格（客户端据此播受击）
func (t *MapTicker) syncAfterBuffTick(mapID uint32, ev world.TickEvent) {
	entity, ok, err := t.Worlds.Entity(context.Background(), mapID, ev.UID)
	if err != nil || !ok {
		return
	}
	if p, exists := t.Players.Get(ev.UID); exists {
		p.SyncFromWorld(entity)
		t.syncPlayerHealth(mapID, p)
		return
	}
	// 怪物掉血：推打击格（回血不发，客户端只看血条）
	if ev.Amount < 0 && t.Broadcast != nil {
		payload, err := protocol.AppendWirePayload(nil, protocol.SMStrikeGrid{
			X: uint32(entity.X), Y: uint32(entity.Y),
		})
		if err == nil {
			t.broadcastMap(mapID, protocol.SM_STRIKEGRID, payload)
		}
	}
}
