package world

import (
	"container/heap"
	"errors"
	"math/rand"
)

var (
	ErrEntityNotFound    = errors.New("entity not found")
	ErrEntityExists      = errors.New("entity already exists")
	ErrInvalidCoordinate = errors.New("invalid map coordinate")
	ErrGridBlocked       = errors.New("grid blocked")
	ErrGridOccupied      = errors.New("grid occupied")
	ErrInvalidMove       = errors.New("invalid move")
	ErrEntityDead        = errors.New("entity dead")
	ErrAttackOutOfRange  = errors.New("attack out of range")
	ErrNoPath            = errors.New("no path")

	// buff 相关（buff.go）
	ErrNilBuff    = errors.New("nil buff")
	ErrBuffExists = errors.New("buff already exists")
)

type EntityType uint8

const (
	EntityPlayer EntityType = iota + 1
	EntityMonster
	EntityNPC
)

type Entity struct {
	UID       uint64
	Type      EntityType
	Name      string
	X         int
	Y         int
	Direction Direction
	HP        int32
	MaxHP     int32
	MP        int32
	MaxMP     int32
	Speed     int

	// ---- 战斗运行时（B4）----
	// Buffs     挂在实体上的 buff 列表（buff.go）
	// Offenders  仇恨列表（combat_state.go）
	// AI         怪物 AI 状态（monster.go）
	// SafeZone   当前地图安全区的副本（对应 BattleObject::m_safeZone）
	// DropSettled 掉落是否已结算（避免死亡事件重复触发掉落）
	Buffs     *BuffList
	Offenders OffenderList
	AI        AIState
	SafeZone  SafeZone

	// DropSettled 掉落是否已结算（防止死亡事件重复触发掉落）
	DropSettled bool

	// 受击/攻击用的属性（由怪物/玩家记录填充）
	Stats CombatStats
}

func (e Entity) Point() Point { return Point{X: e.X, Y: e.Y} }
func (e Entity) Dead() bool   { return e.HP <= 0 }

// Occupant is one entity currently owned by a map grid.
type Occupant uint64

// CellState is runtime state that does not belong to static MapData.
type CellState struct {
	Locked      bool
	Occupants   map[uint64]struct{}
	GroundItems []uint64
}

// MapState is mutable world state and is only executed by one map actor.
type MapState struct {
	ID       uint32
	Data     *MapData
	Cells    []CellState
	Entities map[uint64]*Entity

	// NowMS 世界时钟（毫秒）。所有冷却、buff 过期、刷新都以它为准，
	// 而不是各实体自己读系统时间 —— 这样地图 actor 串行执行时
	// 同一 tick 内所有实体看到一致的时间（对应原版 ServerMap 的时钟）。
	NowMS int64

	// Spawns 怪物刷新管理器（monster.go）
	Spawns *SpawnManager

	// Zone 本图安全区（SDSafeZone 合并结果）
	Zone SafeZone
}

func NewMapState(id uint32, data *MapData) (*MapState, error) {
	if data == nil {
		return nil, errors.New("map data is nil")
	}
	return &MapState{
		ID: id, Data: data.Clone(), Cells: make([]CellState, data.Width*data.Height),
		Entities: make(map[uint64]*Entity),
	}, nil
}

func (m *MapState) cell(x, y int) (*CellState, error) {
	p := Point{X: x, Y: y}
	if !m.Data.InBounds(p) {
		return nil, ErrInvalidCoordinate
	}
	return &m.Cells[x+y*m.Data.Width], nil
}

// Entity 返回实体快照（值拷贝）。
//
// ★ 注意：拿到的是副本，改它不会写回地图状态。
// 需要修改实体时请用 MutateEntity，否则会出现「改了但没生效」的隐蔽 bug。
func (m *MapState) Entity(uid uint64) (Entity, bool) {
	entity, ok := m.Entities[uid]
	if !ok {
		return Entity{}, false
	}
	return *entity, true
}

// MutateEntity 在持有地图所有权的上下文里原地修改实体。
//
// 这是 Entity 的写版本：fn 收到真实指针，修改会写回。
// 地图 actor 内部使用；跨 actor 一律走 World 的命令队列。
func (m *MapState) MutateEntity(uid uint64, fn func(*Entity) error) error {
	entity, ok := m.Entities[uid]
	if !ok {
		return ErrEntityNotFound
	}
	return fn(entity)
}

func (m *MapState) AddEntity(entity Entity) error {
	if _, exists := m.Entities[entity.UID]; exists {
		return ErrEntityExists
	}
	if !m.Data.InBounds(entity.Point()) {
		return ErrInvalidCoordinate
	}
	cell, err := m.cell(entity.X, entity.Y)
	if err != nil {
		return err
	}
	if cell.Locked || len(cell.Occupants) != 0 {
		return ErrGridOccupied
	}
	entity.Direction = DirectionFromDelta(0, 0)
	if !entity.Direction.Valid() {
		entity.Direction = DirDown
	}
	entityCopy := entity
	// 每个实体都持有自己的 buff 列表与安全区副本
	if entityCopy.Buffs == nil {
		entityCopy.Buffs = NewBuffList()
	}
	entityCopy.SafeZone = m.Zone.Clone()
	m.Entities[entity.UID] = &entityCopy
	if cell.Occupants == nil {
		cell.Occupants = make(map[uint64]struct{})
	}
	cell.Occupants[entity.UID] = struct{}{}
	return nil
}

func (m *MapState) RemoveEntity(uid uint64) (Entity, bool) {
	entity, ok := m.Entities[uid]
	if !ok {
		return Entity{}, false
	}
	if cell, err := m.cell(entity.X, entity.Y); err == nil {
		delete(cell.Occupants, uid)
	}
	delete(m.Entities, uid)
	return *entity, true
}

func (m *MapState) CanOccupy(ignoreUID uint64, x, y int) bool {
	cell, err := m.cell(x, y)
	if err != nil || cell.Locked {
		return false
	}
	if !m.Data.CanWalk(x, y) {
		return false
	}
	for uid := range cell.Occupants {
		if uid != ignoreUID {
			return false
		}
	}
	return true
}

func (m *MapState) MoveEntity(uid uint64, x, y, maxStep int) (Point, error) {
	entity, ok := m.Entities[uid]
	if !ok {
		return Point{}, ErrEntityNotFound
	}
	from := entity.Point()
	to := Point{X: x, Y: y}
	if from.Equal(to) {
		return from, nil
	}
	if !m.Data.InBounds(to) {
		return from, ErrInvalidCoordinate
	}
	if !HopValid(maxStep, from, to) || !lineClear(m, from, to, uid) {
		return from, ErrInvalidMove
	}
	if !m.CanOccupy(uid, x, y) {
		return from, ErrGridOccupied
	}
	oldCell, err := m.cell(from.X, from.Y)
	if err != nil {
		return from, err
	}
	newCell, err := m.cell(to.X, to.Y)
	if err != nil {
		return from, err
	}
	delete(oldCell.Occupants, uid)
	if newCell.Occupants == nil {
		newCell.Occupants = make(map[uint64]struct{})
	}
	newCell.Occupants[uid] = struct{}{}
	entity.Direction = DirectionBetween(from, to)
	entity.X, entity.Y = x, y
	return from, nil
}

func lineClear(m *MapState, from, to Point, uid uint64) bool {
	steps := ChebyshevDistance(from, to)
	for i := 1; i <= steps; i++ {
		p := Point{
			X: from.X + (to.X-from.X)*i/steps,
			Y: from.Y + (to.Y-from.Y)*i/steps,
		}
		if p == to {
			continue
		}
		if !m.CanOccupy(uid, p.X, p.Y) {
			return false
		}
	}
	return true
}

type DamageEvent struct {
	AttackerUID uint64
	TargetUID   uint64
	Damage      int32
	TargetHP    int32
	Dead        bool
}

func (m *MapState) Attack(attackerUID, targetUID uint64, damage int32) (DamageEvent, error) {
	attacker, ok := m.Entities[attackerUID]
	if !ok {
		return DamageEvent{}, ErrEntityNotFound
	}
	target, ok := m.Entities[targetUID]
	if !ok {
		return DamageEvent{}, ErrEntityNotFound
	}
	if attacker.Dead() || target.Dead() {
		return DamageEvent{}, ErrEntityDead
	}
	if ChebyshevDistance(attacker.Point(), target.Point()) > 1 {
		return DamageEvent{}, ErrAttackOutOfRange
	}
	if damage < 0 {
		damage = 0
	}
	target.HP -= damage
	if target.HP < 0 {
		target.HP = 0
	}
	return DamageEvent{
		AttackerUID: attackerUID, TargetUID: targetUID, Damage: damage,
		TargetHP: target.HP, Dead: target.HP == 0,
	}, nil
}

type pathNode struct {
	point Point
	cost  float64
	index int
}

type pathPriorityQueue []*pathNode

func (q pathPriorityQueue) Len() int           { return len(q) }
func (q pathPriorityQueue) Less(i, j int) bool { return q[i].cost < q[j].cost }
func (q pathPriorityQueue) Swap(i, j int)      { q[i], q[j] = q[j], q[i]; q[i].index = i; q[j].index = j }
func (q *pathPriorityQueue) Push(x any) {
	node := x.(*pathNode)
	node.index = len(*q)
	*q = append(*q, node)
}
func (q *pathPriorityQueue) Pop() any {
	old := *q
	n := len(old)
	node := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return node
}

// FindPath returns an 8-direction path excluding the source grid.
func (m *MapState) FindPath(uid uint64, target Point, maxVisited int) ([]Point, error) {
	entity, ok := m.Entities[uid]
	if !ok {
		return nil, ErrEntityNotFound
	}
	if !m.Data.InBounds(target) || !m.Data.CanWalk(target.X, target.Y) {
		return nil, ErrNoPath
	}
	if maxVisited <= 0 {
		maxVisited = m.Data.Width * m.Data.Height
	}
	start := entity.Point()
	open := &pathPriorityQueue{}
	heap.Init(open)
	heap.Push(open, &pathNode{point: start, cost: 0})
	gScore := map[Point]float64{start: 0}
	parent := make(map[Point]Point)
	closed := make(map[Point]bool)
	visited := 0
	for open.Len() > 0 {
		node := heap.Pop(open).(*pathNode)
		if closed[node.point] {
			continue
		}
		closed[node.point] = true
		visited++
		if visited > maxVisited {
			return nil, ErrNoPath
		}
		if node.point.Equal(target) {
			return reconstructPath(parent, start, target), nil
		}
		for _, offset := range directionOffsets[DirUp:DirEnd] {
			next := Point{X: node.point.X + offset.X, Y: node.point.Y + offset.Y}
			if !m.CanOccupy(uid, next.X, next.Y) {
				continue
			}
			if offset.X != 0 && offset.Y != 0 {
				if !m.CanOccupy(uid, node.point.X+offset.X, node.point.Y) || !m.CanOccupy(uid, node.point.X, node.point.Y+offset.Y) {
					continue
				}
			}
			stepCost := 1.0
			if offset.X != 0 && offset.Y != 0 {
				stepCost = 1.41421356237
			}
			tentative := gScore[node.point] + stepCost
			if previous, seen := gScore[next]; !seen || tentative < previous {
				gScore[next] = tentative
				parent[next] = node.point
				heuristic := float64(ChebyshevDistance(next, target))
				heap.Push(open, &pathNode{point: next, cost: tentative + heuristic})
			}
		}
	}
	return nil, ErrNoPath
}

func reconstructPath(parent map[Point]Point, start, target Point) []Point {
	if start.Equal(target) {
		return nil
	}
	reversed := make([]Point, 0)
	for current := target; !current.Equal(start); {
		reversed = append(reversed, current)
		previous, ok := parent[current]
		if !ok {
			return nil
		}
		current = previous
	}
	path := make([]Point, len(reversed))
	for i := range reversed {
		path[len(reversed)-1-i] = reversed[i]
	}
	return path
}

// ==================== 世界时钟与 tick（B4）====================

// TickEvent 是 tick 产生的一条世界事件，交给上层转成网络消息。
type TickEvent struct {
	Kind    string // "buff_expired" / "spawn" / "death" / "heal"
	UID     uint64
	BuffSeq uint64
	Monster string
	X       int
	Y       int
	Amount  int32
}

// Advance 推进世界时钟到 nowMS，并处理本 tick 的到期事件。
//
// 【依据】原版 ServerMap 的定时驱动：
//   - buff 按 duration 递减到期（buffrecord 的 duration）
//   - 怪物刷新按 IntervalMS 补位（servermap.cpp 的刷新逻辑）
//   - 各实体冷却以地图时钟为准
//
// 返回本 tick 产生的事件，由上层（地图 actor 的持有者）转成网络消息。
// 这里不直接发消息，保持 MapState 不依赖 transport/protocol。
func (m *MapState) Advance(nowMS int64) []TickEvent {
	if nowMS < m.NowMS {
		return nil // 时钟不能倒退
	}
	deltaMS := nowMS - m.NowMS
	m.NowMS = nowMS

	var events []TickEvent

	// 1. buff 过期
	if deltaMS > 0 {
		for uid, e := range m.Entities {
			if e.Buffs == nil || e.Buffs.Len() == 0 {
				continue
			}
			for _, seq := range e.Buffs.Tick(deltaMS) {
				events = append(events, TickEvent{Kind: "buff_expired", UID: uid, BuffSeq: seq})
			}
		}
	}

	// 2. 怪物刷新
	if m.Spawns != nil {
		for _, req := range m.Spawns.Update(nowMS) {
			events = append(events, TickEvent{Kind: "spawn", Monster: req.MonsterName, X: req.X, Y: req.Y})
		}
	}

	return events
}

// AttackResult 是一次攻击的完整结果。
type AttackResult struct {
	Damage   int32
	Hit      bool
	Dead     bool
	Killer   uint64
	HealGain int32 // 吸血回复量（modifierID 为吸血的场合）
}

// Combat 执行一次带属性结算的攻击。
//
// 【依据】battleobject.cpp:690 getAttackDamage + monster.cpp:715 struckDamage：
//
//  1. 用攻击方属性 roll 出 DamageNode
//  2. 用受击方属性结算（命中判定 + 减伤）
//  3. 命中则扣血、记仇恨、处理吸血
//  4. 血量归零则标记死亡
//
// 与原来的 Attack（直接传 damage）相比，这才是原版真实的链路。
// Attack 保留给测试与简化调用。
func (m *MapState) Combat(attackerUID, targetUID uint64, magicID int, r *rand.Rand) (AttackResult, error) {
	attacker, ok := m.Entities[attackerUID]
	if !ok {
		return AttackResult{}, ErrEntityNotFound
	}
	target, ok := m.Entities[targetUID]
	if !ok {
		return AttackResult{}, ErrEntityNotFound
	}
	if attacker.Dead() || target.Dead() {
		return AttackResult{}, ErrEntityDead
	}
	if ChebyshevDistance(attacker.Point(), target.Point()) > 1 {
		return AttackResult{}, ErrAttackOutOfRange
	}

	// 1. roll 伤害
	node := RollAttackDamage(magicID, attacker.Stats.DC, attacker.Stats.MC,
		attacker.Stats.DCHit, attacker.Stats.MCHit, AttackModifierNone, r)

	// 2. 结算
	damage, hit := ResolveDamage(node, target.Stats, r)
	res := AttackResult{Damage: damage, Hit: hit}
	if !hit {
		return res, nil
	}

	// 3. 扣血 + 仇恨
	if damage > 0 {
		target.HP -= damage
		if target.HP < 0 {
			target.HP = 0
		}
		target.Offenders.AddDamage(attackerUID, uint64(damage), m.NowMS/1000)

		// 吸血：回复量取 min(伤害, 20)（monster.cpp:756）
		if node.ModifierID == AttackModifierVamp {
			heal := damage
			if heal > 20 {
				heal = 20
			}
			attacker.HP += heal
			if attacker.MaxHP > 0 && attacker.HP > attacker.MaxHP {
				attacker.HP = attacker.MaxHP
			}
			res.HealGain = heal
		}
	}

	// 4. 死亡判定
	if target.HP == 0 {
		res.Dead = true
		res.Killer = attackerUID
	}
	return res, nil
}

// CommitDeath 结算死亡，返回掉落（每只怪只结算一次）。
//
// 【依据】battleobject.cpp 的 goDie + monster.cpp 的掉落逻辑：
//   - 死亡时按 dropItemList 掉落
//   - 掉落归属给仇恨最高的来源（m_offenderList）
//
// 返回 (是否首次结算, 掉落列表)
func (m *MapState) CommitDeath(uid uint64, rec MonsterRecord, r *rand.Rand) (bool, []DropResult) {
	e, ok := m.Entities[uid]
	if !ok || !e.Dead() {
		return false, nil
	}
	if e.DropSettled {
		return false, nil // 已结算过，防重复掉落
	}
	e.DropSettled = true
	drops := RollDrop(rec, r)
	return true, drops
}

// DropOwner 返回掉落归属（仇恨最高的来源）。
func (m *MapState) DropOwner(uid uint64) (uint64, bool) {
	e, ok := m.Entities[uid]
	if !ok {
		return 0, false
	}
	top, ok := e.Offenders.TopOffender()
	if !ok {
		return 0, false
	}
	return top.UID, true
}

// AddBuff 给实体挂一个 buff。
func (m *MapState) AddBuff(uid uint64, buff *Buff) error {
	e, ok := m.Entities[uid]
	if !ok {
		return ErrEntityNotFound
	}
	if e.Buffs == nil {
		e.Buffs = NewBuffList()
	}
	// 叠加规则检查
	if current, exists := e.Buffs.HasID(buff.ID); exists {
		if !CanStack(current.Record, e.Buffs.CountID(buff.ID)) {
			return ErrBuffExists
		}
	}
	selseq := e.Buffs.RollSeqID(buff.ID)
	buff.SeqID = selseq
	return e.Buffs.Add(buff)
}

// SetSafeZone 设置地图安全区，并同步到所有已存在的实体。
func (m *MapState) SetSafeZone(z SafeZone) {
	m.Zone = z
	for _, e := range m.Entities {
		e.SafeZone = z.Clone()
	}
}

// InSafeZone 判断某实体是否站在安全区内。
//
// 【依据】battleobject.hpp 的 inSafeZone()：
//
//	return isSafeGrid(X(), Y())
func (m *MapState) InSafeZone(uid uint64) bool {
	e, ok := m.Entities[uid]
	if !ok {
		return false
	}
	return e.SafeZone.Contains(e.X, e.Y)
}

// SafeZoneBlocksAttack 判断安全区是否阻挡本次攻击。
//
// 【依据】battleobject.hpp:222-230 safeZoneBlocksAttack 的注释：
//
//	"receiver side of Player::canDamageTarget()'s safe zone rule"
//	"fire wall, fixed location strikes and pets deliver damage without going through it"
//
// 即：安全区保护站在区内的受击者，但火墙/固定打击/宠物的伤害不走这条规则。
// 这里把「是否地图伤害」作为参数由调用方决定。
func (m *MapState) SafeZoneBlocksAttack(targetUID uint64, isMapDamage bool) bool {
	if isMapDamage {
		return false // 地图伤害绕过安全区
	}
	return m.InSafeZone(targetUID)
}
