package world

import (
	"math/rand"
)

// ==================== 怪物 AI 驱动（server/src/monster.cpp:404 runAICoro）====================
//
// 原版是一个协程主循环：
//
//	while(!dead()){
//	    if(targetUID && 超过 SYS_TARGETSEC 没再攻击) targetUID = 0;
//	    if(targetUID && !validTarget(targetUID))    targetUID = 0;
//	    if(!targetUID) targetUID = pickTarget();
//	    if(targetUID)  trackAttackUID(targetUID);   // 追+打
//	    else if(master) followMaster();
//	    else            randomMove();               // 空闲游荡
//	    idleWait(1000);                             // 每秒迭代一次
//	}
//
// ★ Go 侧为什么不用协程：
//
//	地图 actor 本来就是串行执行的，每个 tick 遍历所有怪物跑一次
//	「决策 + 动作」即可，不需要为每只怪开协程。
//	原版用协程是为了能 yield 等网络/寻路，Go 侧这些是同步调用。
//
//	代价：单次 tick 的时间 = 怪物数 × 单只决策耗时。
//	比奇县有 1107 只怪，实测单 tick 在 3ms 以内（见 REPLICA_AUDIT）。
//
// ★ 每秒迭代一次而不是每 tick：
//
//	原版 asyncIdleWait(1000)，即 AI 决策周期是 1 秒。
//	每 100ms 的 tick 都跑一遍会让 CPU 白烧 10 倍，
//	而且怪物会「抖」（走一格就重新决策）。

// AITickIntervalMS 是 AI 决策周期（原版 asyncIdleWait(1000)）。
const AITickIntervalMS int64 = 1000

// TargetIdleMS 是目标闲置多久后放弃（sysconst.hpp SYS_TARGETSEC = 10 秒）。
const TargetIdleMS int64 = 10 * 1000

// AIAction 是一次 AI 决策产生的动作（供上层转成网络包）。
type AIAction struct {
	// Kind "move" / "attack" / "none"
	Kind string
	// UID 动作发出者
	UID uint64
	// Target 攻击目标（Kind=attack 时有值）
	Target uint64
	// From / To 移动起止（Kind=move 时有值）
	From, To Point
	// Damage / Hit / Dead 攻击结果（Kind=attack 时有值）
	Damage int32
	Hit    bool
	Dead   bool
}

// RunAI 推进地图上所有怪物的 AI。
//
// 返回本 tick 产生的动作列表，由上层（game 层）转成 SM_ACTION / SM_STRIKE。
func (m *MapState) RunAI(r *rand.Rand) []AIAction {
	// 只有到决策周期才跑
	if m.NowMS-m.lastAITickMS < AITickIntervalMS {
		return nil
	}
	m.lastAITickMS = m.NowMS

	if r == nil {
		r = m.aiRand
	}

	var actions []AIAction
	// 遍历时不能改 map，先快照 UID
	uids := make([]uint64, 0, len(m.Entities))
	for uid, e := range m.Entities {
		if e.Type != EntityMonster || e.Dead() {
			continue
		}
		uids = append(uids, uid)
	}

	for _, uid := range uids {
		e, ok := m.Entities[uid]
		if !ok || e.MonsterRec == nil {
			continue
		}
		if act, ok := m.runOneAI(uid, e, r); ok {
			actions = append(actions, act)
		}
	}
	return actions
}

// runOneAI 跑一只怪的决策。
func (m *MapState) runOneAI(uid uint64, e *Entity, r *rand.Rand) (AIAction, bool) {
	rec := *e.MonsterRec
	ai := &e.AI

	// 1. 目标失效检查
	if ai.TargetUID != 0 {
		if m.NowMS-ai.LastAttackMS > TargetIdleMS {
			ai.TargetUID = 0
		} else if target, ok := m.Entities[ai.TargetUID]; !ok || target.Dead() {
			ai.TargetUID = 0
		}
	}

	// 2. 没目标就找
	if ai.TargetUID == 0 {
		ai.TargetUID = m.pickTarget(uid, e, rec)
	}

	// 3. 有目标：追 + 打
	if ai.TargetUID != 0 {
		if act, ok := m.trackAndAttack(uid, e, rec, ai, r); ok {
			return act, true
		}
		return AIAction{}, false
	}

	// 4. 没目标：随机游荡
	if act, ok := m.randomMove(uid, e, rec, ai, r); ok {
		return act, true
	}
	return AIAction{}, false
}

// pickTarget 选目标（monster.cpp:972 searchNearestTarget）。
//
// 优先级：
//
//  1. 仇恨列表里最后一个还活着的（最近打我的人）
//  2. 中立怪直接返回（不主动）
//  3. 视野 0 直接返回
//  4. 视野内最近的敌对目标
func (m *MapState) pickTarget(uid uint64, e *Entity, rec MonsterRecord) uint64 {
	// 1. 仇恨优先：被打过就一定还手
	if offender, ok := e.Offenders.TopOffender(); ok {
		if target, exists := m.Entities[offender.UID]; exists && !target.Dead() {
			return offender.UID
		}
	}

	// 2. 中立怪不主动攻击
	if rec.BehaveMode == BehaveNeutral {
		return 0
	}

	// 3. 视野 0 的怪看不见东西（MonsterViewDistance 已钳到至少 1）
	// 4. 找视野内最近的目标
	bestUID := uint64(0)
	bestDist := int(^uint(0) >> 1) // MaxInt
	for otherUID, other := range m.Entities {
		if otherUID == uid || other.Dead() {
			continue
		}
		// 只打玩家（守卫打红名由 ShouldAggro 判）
		if other.Type != EntityPlayer {
			continue
		}
		if !InViewRange(rec, e.Point(), other.Point()) {
			continue
		}
		// 守卫只打红名 —— 这里用 PKPoint 粗判
		hostile := other.PKPoint >= 100 || rec.BehaveMode == BehaveDefault
		if !ShouldAggro(rec, hostile, false) {
			continue
		}
		dist := ChebyshevDistance(e.Point(), other.Point())
		if dist < bestDist {
			bestDist = dist
			bestUID = otherUID
		}
	}
	return bestUID
}

// trackAndAttack 追击并攻击目标（monster.cpp:385 trackAttackUID）。
//
// 相邻 → 攻击；不相邻 → 朝目标走一步。
func (m *MapState) trackAndAttack(uid uint64, e *Entity, rec MonsterRecord,
	ai *AIState, r *rand.Rand) (AIAction, bool) {
	target, ok := m.Entities[ai.TargetUID]
	if !ok || target.Dead() {
		ai.TargetUID = 0
		return AIAction{}, false
	}

	dist := ChebyshevDistance(e.Point(), target.Point())

	// 相邻：攻击
	if dist <= 1 {
		if !ai.CanAttackNow(m.NowMS, rec) {
			return AIAction{}, false
		}
		result, err := m.Combat(uid, ai.TargetUID, MagicIDPlainAttack, r)
		if err != nil {
			return AIAction{}, false
		}
		ai.MarkAttack(m.NowMS)
		return AIAction{
			Kind:   "attack",
			UID:    uid,
			Target: ai.TargetUID,
			Damage: result.Damage,
			Hit:    result.Hit,
			Dead:   result.Dead,
		}, true
	}

	// 不相邻：朝目标走一步
	if !ai.CanMoveNow(m.NowMS, rec) {
		return AIAction{}, false
	}
	from := e.Point()
	next := stepToward(from, target.Point())
	if !m.CanOccupy(uid, next.X, next.Y) {
		// 挡路：随机挪一步（避免卡死在墙角）
		if r == nil || r.Intn(2) == 0 {
			return AIAction{}, false
		}
		next = randomAdjacent(from, r)
		if !m.CanOccupy(uid, next.X, next.Y) {
			return AIAction{}, false
		}
	}

	_, err, _ := m.MoveEntity(uid, next.X, next.Y, 1)
	if err != nil {
		return AIAction{}, false
	}
	ai.MarkMove(m.NowMS)
	return AIAction{Kind: "move", UID: uid, From: from, To: next, Target: ai.TargetUID}, true
}

// randomMove 空闲游荡（monster.cpp 的 randomMove）。
//
// ★ 不是每只怪每秒都动：
//
//	原版 randomMove 里还有一层随机，大部分时候是「站着不动」。
//	这里用 1/4 概率触发，否则 1107 只怪每秒集体移动，
//	网络包会把客户端淹掉。
func (m *MapState) randomMove(uid uint64, e *Entity, rec MonsterRecord,
	ai *AIState, r *rand.Rand) (AIAction, bool) {
	if r == nil {
		return AIAction{}, false
	}
	if r.Intn(4) != 0 {
		return AIAction{}, false
	}
	if !ai.CanMoveNow(m.NowMS, rec) {
		return AIAction{}, false
	}

	from := e.Point()
	next := randomAdjacent(from, r)
	if !m.CanOccupy(uid, next.X, next.Y) {
		return AIAction{}, false
	}
	_, err, _ := m.MoveEntity(uid, next.X, next.Y, 1)
	if err != nil {
		return AIAction{}, false
	}
	ai.MarkMove(m.NowMS)
	return AIAction{Kind: "move", UID: uid, From: from, To: next}, true
}

// stepToward 朝目标走一步（八方向，取距离缩短最多的方向）。
func stepToward(from, to Point) Point {
	dx, dy := 0, 0
	if to.X > from.X {
		dx = 1
	} else if to.X < from.X {
		dx = -1
	}
	if to.Y > from.Y {
		dy = 1
	} else if to.Y < from.Y {
		dy = -1
	}
	return Point{X: from.X + dx, Y: from.Y + dy}
}

// randomAdjacent 随机选一个相邻格（八方向）。
func randomAdjacent(from Point, r *rand.Rand) Point {
	if r == nil {
		return from
	}
	dir := Direction(r.Intn(8) + 1)
	return dir.Front(from, 1)
}

// SetAISeed 设置 AI 随机源（测试可注入固定种子）。
func (m *MapState) SetAISeed(seed int64) {
	m.aiRand = rand.New(rand.NewSource(seed))
}
