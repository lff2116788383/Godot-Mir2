package game

import (
	"context"
	"log/slog"
	"sync"
	"time"

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

	// interval 推进间隔（测试可改小/改大）
	interval time.Duration
	// now 时钟源（测试可注入，避免依赖真实时间）
	now func() int64

	mu      sync.Mutex
	mapIDs  []uint32
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
		case "buff_expired":
			t.broadcastBuffOff(mapID, ev)
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
