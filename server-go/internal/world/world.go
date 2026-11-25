package world

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/actor"
)

var ErrMapNotFound = errors.New("map not found")

type mapCommand interface {
	apply(*MapState) (any, error)
}

type mapReply struct {
	value any
	err   error
}

type mapRequest struct {
	command mapCommand
	reply   chan mapReply
}

type mapActor struct{ state *MapState }

func (a *mapActor) Receive(_ *actor.Context, message actor.Message) error {
	request, ok := message.(mapRequest)
	if !ok {
		return fmt.Errorf("unexpected map actor message %T", message)
	}
	value, err := request.command.apply(a.state)
	request.reply <- mapReply{value: value, err: err}
	return nil
}
func (*mapActor) Stop(*actor.Context) {}

type commandFunc func(*MapState) (any, error)

func (f commandFunc) apply(state *MapState) (any, error) { return f(state) }

// World owns one actor-executed MapState per map ID.
type World struct {
	system  *actor.System
	timeout time.Duration
	mu      sync.RWMutex
	maps    map[uint32]actor.ID
}

func NewWorld(system *actor.System) *World {
	if system == nil {
		panic("nil actor system")
	}
	return &World{system: system, timeout: 5 * time.Second, maps: make(map[uint32]actor.ID)}
}

func (w *World) CreateMap(id uint32, data *MapData) error {
	if data == nil {
		return errors.New("map data is nil")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.maps[id]; exists {
		return fmt.Errorf("map %d already exists", id)
	}
	state, err := NewMapState(id, data)
	if err != nil {
		return err
	}
	actorID, err := w.system.Spawn(&mapActor{state: state})
	if err != nil {
		return err
	}
	w.maps[id] = actorID
	return nil
}

func (w *World) RemoveMap(ctx context.Context, id uint32) error {
	w.mu.Lock()
	actorID, ok := w.maps[id]
	if ok {
		delete(w.maps, id)
	}
	w.mu.Unlock()
	if !ok {
		return ErrMapNotFound
	}
	return w.system.Stop(actorID)
}

func (w *World) Execute(ctx context.Context, mapID uint32, command mapCommand) (any, error) {
	w.mu.RLock()
	actorID, ok := w.maps[mapID]
	w.mu.RUnlock()
	if !ok {
		return nil, ErrMapNotFound
	}
	reply := make(chan mapReply, 1)
	if err := w.system.Send(ctx, actorID, mapRequest{command: command, reply: reply}); err != nil {
		return nil, err
	}
	timeout := w.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-reply:
		return result.value, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errors.New("map command timed out")
	}
}

// AddEntity 把实体加入地图，返回 AOI 事件（供上层转成 AM_OBSERVE）。
func (w *World) AddEntity(ctx context.Context, mapID uint32, entity Entity) ([]ObserverEvent, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.AddEntity(entity)
	}))
	if err != nil {
		return nil, err
	}
	events, _ := value.([]ObserverEvent)
	return events, nil
}

func (w *World) RemoveEntity(ctx context.Context, mapID uint32, uid uint64) (Entity, bool, []ObserverEvent, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		entity, ok, events := state.RemoveEntity(uid)
		if !ok {
			return removeEntityResult{}, nil
		}
		return removeEntityResult{Entity: entity, Events: events}, nil
	}))
	if err != nil {
		return Entity{}, false, nil, err
	}
	result, ok := value.(removeEntityResult)
	if !ok {
		return Entity{}, false, nil, nil
	}
	return result.Entity, true, result.Events, nil
}

// removeEntityResult 让「实体不存在」也能正常穿过命令队列。
type removeEntityResult struct {
	Entity Entity
	Events []ObserverEvent
}

func (w *World) MoveEntity(ctx context.Context, mapID uint32, uid uint64, x, y, maxStep int) (Point, []ObserverEvent, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		from, moveErr, events := state.MoveEntity(uid, x, y, maxStep)
		return moveResult{From: from, Events: events}, moveErr
	}))
	if err != nil {
		return Point{}, nil, err
	}
	result, _ := value.(moveResult)
	return result.From, result.Events, nil
}

type moveResult struct {
	From   Point
	Events []ObserverEvent
}

// SetViewRange 调整实体视野，返回观察圈变化事件。
func (w *World) SetViewRange(ctx context.Context, mapID uint32, uid uint64, viewRange int) ([]ObserverEvent, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.SetViewRange(uid, viewRange), nil
	}))
	if err != nil {
		return nil, err
	}
	events, _ := value.([]ObserverEvent)
	return events, nil
}

// ForwardAction 返回应该收到某个实体动作的观察者 uid 列表。
func (w *World) ForwardAction(ctx context.Context, mapID uint32, selfUID uint64, srcX, srcY, dstX, dstY int) ([]uint64, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.ForwardAction(selfUID, srcX, srcY, dstX, dstY), nil
	}))
	if err != nil {
		return nil, err
	}
	observers, _ := value.([]uint64)
	return observers, nil
}

// Advance 推进地图时钟，返回本 tick 的世界事件。
func (w *World) Advance(ctx context.Context, mapID uint32, nowMS int64) ([]TickEvent, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.Advance(nowMS), nil
	}))
	if err != nil {
		return nil, err
	}
	events, _ := value.([]TickEvent)
	return events, nil
}

// Combat 执行一次完整伤害结算（roll → 命中 → 减伤 → 扣血 → 仇恨）。
func (w *World) Combat(ctx context.Context, mapID uint32, attackerUID, targetUID uint64, magicID int) (AttackResult, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.Combat(attackerUID, targetUID, magicID, nil)
	}))
	if err != nil {
		return AttackResult{}, err
	}
	result, _ := value.(AttackResult)
	return result, nil
}

func (w *World) FindPath(ctx context.Context, mapID uint32, uid uint64, target Point) ([]Point, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.FindPath(uid, target, 0)
	}))
	if err != nil {
		return nil, err
	}
	return value.([]Point), nil
}

func (w *World) Attack(ctx context.Context, mapID uint32, attackerUID, targetUID uint64, damage int32) (DamageEvent, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.Attack(attackerUID, targetUID, damage)
	}))
	if err != nil {
		return DamageEvent{}, err
	}
	return value.(DamageEvent), nil
}

func (w *World) Entity(ctx context.Context, mapID uint32, uid uint64) (Entity, bool, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		entity, ok := state.Entity(uid)
		if !ok {
			return entity, ErrEntityNotFound
		}
		return entity, nil
	}))
	if errors.Is(err, ErrEntityNotFound) {
		return Entity{}, false, nil
	}
	if err != nil {
		return Entity{}, false, err
	}
	return value.(Entity), true, nil
}

func (w *World) MapCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.maps)
}

// CanOccupy 判断某格能否被 uid 占据（网格查询，不修改状态）。
func (w *World) CanOccupy(ctx context.Context, mapID uint32, uid uint64, x, y int) bool {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.CanOccupy(uid, x, y), nil
	}))
	if err != nil {
		return false
	}
	ok, _ := value.(bool)
	return ok
}

// MoveEntityFull 移动实体并返回移动后的完整实体快照。
//
// 与 MoveEntity 的区别：这里返回 Entity 而不是起点，调用方需要
// 同步方向/坐标/血量时用这个。AOI 事件仍然返回，供上层广播。
func (w *World) MoveEntityFull(ctx context.Context, mapID uint32, uid uint64, x, y, maxStep int) (Entity, bool, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		_, moveErr, events := state.MoveEntity(uid, x, y, maxStep)
		if moveErr != nil {
			return moveEntityResult{OK: false, Events: events}, moveErr
		}
		entity, ok := state.Entity(uid)
		return moveEntityResult{OK: ok, Entity: entity, Events: events}, nil
	}))
	if err != nil {
		return Entity{}, false, err
	}
	result, _ := value.(moveEntityResult)
	return result.Entity, result.OK, nil
}

type moveEntityResult struct {
	OK     bool
	Entity Entity
	Events []ObserverEvent
}

// PickupOne 从 (x,y) 拾取一件物品（不指定 itemID 时取最后一件）。
//
// 返回拾到的 itemID 列表（当前一次只拾一件）。
func (w *World) PickupOne(ctx context.Context, mapID uint32, x, y int, uid uint64) ([]uint32, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		ids := state.GroundItemIDsAt(x, y)
		if len(ids) == 0 {
			return []uint32(nil), nil
		}
		// 从尾部取（与 PickupItem 的尾部语义一致）
		itemID := ids[len(ids)-1]
		ok, _ := state.PickupItem(x, y, itemID)
		if !ok {
			return []uint32(nil), nil
		}
		return []uint32{itemID}, nil
	}))
	if err != nil {
		return nil, err
	}
	got, _ := value.([]uint32)
	return got, nil
}

// VisibleEntities 返回某个实体视野内可见的其他实体 UID。
//
// 对应原版「AM_QUERYOBSERVED 的反向」：把观察者圈里所有格上的实体收集起来。
// 进场时用它一次性把周围实体推给客户端（避免逐个 COAppear）。
func (w *World) VisibleEntities(ctx context.Context, mapID uint32, uid uint64) ([]uint64, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.VisibleEntities(uid), nil
	}))
	if err != nil {
		return nil, err
	}
	out, _ := value.([]uint64)
	return out, nil
}

// DropItem 在指定格掉落物品（供战斗掉落、GM 命令、测试使用）。
func (w *World) DropItem(ctx context.Context, mapID uint32, x, y int, itemIDs []uint32) ([]GroundItemEvent, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.DropItems(x, y, itemIDs), nil
	}))
	if err != nil {
		return nil, err
	}
	events, _ := value.([]GroundItemEvent)
	return events, nil
}

// SetStats 更新实体的战斗属性（升级/换装后调用）。
func (w *World) SetStats(ctx context.Context, mapID uint32, uid uint64, stats CombatStats) error {
	_, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return nil, state.MutateEntity(uid, func(e *Entity) error {
			e.Stats = stats
			return nil
		})
	}))
	return err
}

// CombatWithBounds 用显式伤害区间结算一次攻击（技能伤害用）。
func (w *World) CombatWithBounds(ctx context.Context, mapID uint32, attackerUID, targetUID uint64,
	magicID uint32, bounds AbilityBounds, hit int) (AttackResult, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.CombatWithBounds(attackerUID, targetUID, int(magicID), bounds, hit, nil)
	}))
	if err != nil {
		return AttackResult{}, err
	}
	result, _ := value.(AttackResult)
	return result, nil
}

// CommitDeath 结算死亡（掉落 + 经验），每只怪只结算一次。
func (w *World) CommitDeath(ctx context.Context, mapID uint32, uid uint64) (DeathResult, bool, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		result, ok := state.CommitDeath(uid, nil)
		return deathSettleResult{Result: result, OK: ok}, nil
	}))
	if err != nil {
		return DeathResult{}, false, err
	}
	settled, _ := value.(deathSettleResult)
	return settled.Result, settled.OK, nil
}

type deathSettleResult struct {
	Result DeathResult
	OK     bool
}

// MutateEntity 在 actor 上下文里修改实体。
//
// 这是 Entity 的写版本：fn 拿到真实指针，改动会写回地图状态。
//
// ★ 为什么必须要这个：
//
//	Entity() 返回的是值拷贝，`e := World.Entity(...); e.HP = 10` 不生效。
//	改血量、改属性、挂 buff 都必须走这里，否则会出现
//	「代码跑了但状态没变」这种最难查的一类 bug。
func (w *World) MutateEntity(ctx context.Context, mapID uint32, uid uint64, fn func(*Entity) error) error {
	_, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return nil, state.MutateEntity(uid, fn)
	}))
	return err
}

// NowMS 返回地图当前世界时钟（毫秒）。
//
// 冷却、buff 过期都用它，不读系统时间 —— 地图 actor 串行执行，
// 同一 tick 内所有实体看到一致的时间，行为才可复现。
func (w *World) NowMS(ctx context.Context, mapID uint32) int64 {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.NowMS, nil
	}))
	if err != nil {
		return 0
	}
	now, _ := value.(int64)
	return now
}

// SpawnMonster 在地图上刷一只怪物，并把怪物记录挂到实体上。
//
// 记录挂在实体上而不是靠死亡时反查表：掉落与经验在刷怪那一刻就固定了，
// 反查会受配置热更新影响（详见 Entity.MonsterRec 的注释）。
func (w *World) SpawnMonster(ctx context.Context, mapID uint32, entity Entity, rec MonsterRecord) ([]ObserverEvent, error) {
	entity.MonsterRec = &rec
	entity.Type = EntityMonster
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return state.AddEntity(entity)
	}))
	if err != nil {
		return nil, err
	}
	events, _ := value.([]ObserverEvent)
	return events, nil
}

// Teleport 把实体瞬移到指定坐标，返回瞬移后的实体快照。
//
// 与 MoveEntityFull 的区别：不校验步长（复活/传送卷轴/GM 拉人都用它）。
// 仍然产生完整 AOI 事件。
func (w *World) Teleport(ctx context.Context, mapID uint32, uid uint64, x, y int) (Entity, bool, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		_, moveErr, events := state.Teleport(uid, x, y)
		if moveErr != nil {
			return moveEntityResult{OK: false, Events: events}, moveErr
		}
		entity, ok := state.Entity(uid)
		return moveEntityResult{OK: ok, Entity: entity, Events: events}, nil
	}))
	if err != nil {
		return Entity{}, false, err
	}
	result, _ := value.(moveEntityResult)
	return result.Entity, result.OK, nil
}

// MapSize 返回地图尺寸（w, h）。
func (w *World) MapSize(ctx context.Context, mapID uint32) (int, int, error) {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		return [2]int{state.Data.Width, state.Data.Height}, nil
	}))
	if err != nil {
		return 0, 0, err
	}
	size, _ := value.([2]int)
	return size[0], size[1], nil
}

// CanWalk 判断某格地形是否可走。
//
// ★ 只看地形，不看实体占用：
//
//	刷怪/随机取点用的是这个语义（原版 groundValid）。
//	如果要判「这格现在能不能站人」用 CanOccupy。
func (w *World) CanWalk(ctx context.Context, mapID uint32, x, y int) bool {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		if !state.Data.InBounds(Point{X: x, Y: y}) {
			return false, nil
		}
		return state.Data.CanWalk(x, y), nil
	}))
	if err != nil {
		return false
	}
	ok, _ := value.(bool)
	return ok
}

// AddSafeZoneCircle 追加一个圆形安全区。
//
// 地图脚本会多次调用（一张图可能有多个安全区），所以是「追加」不是「覆盖」。
func (w *World) AddSafeZoneCircle(ctx context.Context, mapID uint32, x, y, radius int) error {
	_, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		zone := state.Zone.Clone()
		zone.Circles = append(zone.Circles, [3]int{x, y, radius})
		state.SetSafeZone(zone)
		return nil, nil
	}))
	return err
}

// CountWalkable 统计矩形区域内可走格数。
//
// 一次 actor 调用算完整块。
//
// ★ 为什么不暴露「逐格 CanWalk + Lua 循环」：
//
//	道馆_1 的刷怪配置要扫 41 万格。每次跨语言回调约 200μs
//	（gopher-lua 是 AST 解释器），累计 30 秒以上 —— 加载一张图卡半分钟。
//	批量算完再返回，整个区域只花一次往返。
func (w *World) CountWalkable(ctx context.Context, mapID uint32, x, y, width, height int) int {
	value, err := w.Execute(ctx, mapID, commandFunc(func(state *MapState) (any, error) {
		count := 0
		for yy := y; yy < y+height; yy++ {
			if yy < 0 || yy >= state.Data.Height {
				continue
			}
			for xx := x; xx < x+width; xx++ {
				if xx < 0 || xx >= state.Data.Width {
					continue
				}
				if state.Data.CanWalk(xx, yy) {
					count++
				}
			}
		}
		return count, nil
	}))
	if err != nil {
		return 0
	}
	n, _ := value.(int)
	return n
}
