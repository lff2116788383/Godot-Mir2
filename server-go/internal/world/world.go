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
