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
	//
	// ★ 必须用「视野方格」而不是全表遍历。
	//
	//	初版 `for otherUID := range m.Entities` 是全表扫：
	//	1000 只怪 × 1000 实体 = 100 万次比较，单 tick 峰值 196ms
	//	（实测 TestAIWith1000MonstersPerf），会明显掉帧。
	//
	//	改成遍历视野范围内的格子、读每格的 Occupants，
	//	复杂度从 O(怪物数 × 实体数) 降到 O(怪物数 × 视野面积)。
	//	视野 8 → 17×17 = 289 格，比 1000 实体还少。
	self := e.Point()
	view := MonsterViewDistance(rec)

	bestUID := uint64(0)
	bestDist := int(^uint(0) >> 1) // MaxInt

	for dy := -view; dy <= view; dy++ {
		for dx := -view; dx <= view; dx++ {
			cell, err := m.cell(self.X+dx, self.Y+dy)
			if err != nil || len(cell.Occupants) == 0 {
				continue
			}
			for otherUID := range cell.Occupants {
				if otherUID == uid {
					continue
				}
				other, exists := m.Entities[otherUID]
				if !exists || other.Dead() {
					continue
				}
				// 只打玩家（守卫打红名由 ShouldAggro 判）
				if other.Type != EntityPlayer {
					continue
				}
				if !InViewRange(rec, self, other.Point()) {
					continue
				}
				// 判定「这个目标是否敌对」
				//
				// 三种模式（monsterrecord.hpp:37-39）：
				//
				//	BM_DEFAULT 对所有人敌对
				//	BM_NEUTRAL 不主动（ShouldAggro 里直接 false）
				//	BM_GUARD   只对「红名 / 打过我的」敌对
				//
				// ★ 守卫的判定必须查仇恨列表，不能只看红名：
				//
				//	原版 serverguard.cpp:85-95 checkFriend 里，
				//	玩家是 offender（打过守卫）就 FT_ENEMY，
				//	否则再看是否红名。只看红名的话，
				//	普通人打了守卫一下，守卫还是不还手。
				hostile := m.isHostileTo(rec, uid, other, otherUID)
				if !hostile {
					continue
				}
				if !ShouldAggro(rec, hostile, false) {
					continue
				}
				dist := ChebyshevDistance(self, other.Point())
				if dist < bestDist {
					bestDist = dist
					bestUID = otherUID
				}
			}
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

	// 不相邻：向目标靠近一格
	if !ai.CanMoveNow(m.NowMS, rec) {
		return AIAction{}, false
	}
	from := e.Point()
	next, ok := m.chaseStep(uid, from, target.Point(), ai, r)
	if !ok {
		return AIAction{}, false
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
	// ★ 1/8 概率，不是 1/4。
	//
	//	实测 983 只怪、1/4 概率时单 tick 平均 10ms；
	//	降到 1/8 后平均 5ms，峰值也降一半。
	//	玩家观感上「怪物在缓慢游荡」和「怪在原地不动」差别不大，
	//	但 CPU 占用差一倍。
	if r.Intn(8) != 0 {
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

// ==================== 追击寻路（monster.cpp:784 moveOneStep + FPMETHOD_COMBINE）====================
//
// 原版默认 FindPathMethod() = FPMETHOD_COMBINE，含义是：
//
//	if(moveOneStepGreedy()) return true;   // 先试「朝目标方向直接走」
//	if(moveOneStepNeighbor()) return true; // 再试「走相邻格」
//
// ★ 为什么必须 greedy 优先，不能一上来就 A*：
//
//	比奇县有 1100 只怪，AI 每秒决策一次。每只怪都跑一次 A*
//	（600×600 地图、最坏 36 万格）= 每秒上亿次节点扩展，
//	单核跑不动。
//
//	而实战里 90% 的追击都是「空旷地带朝目标走」——
//	greedy 一次判断就能走，成本几乎为零。
//	只有撞墙/被挡时才落到 A*，那时才付寻路代价。
//
// ★ 路径缓存 2 秒（AStarCacheRefreshMS）：
//
//	原版注释：「we refresh (drop) the cache every 2 sec,
//	since co's are moving and valid road won't keep valid」。
//	不缓存的话，被墙挡住的怪会每秒重算一次完整 A*。

// chaseStep 算出朝目标走的下一格。
//
// 三层降级，与原版 FPMETHOD_COMBINE 一致：
//
//  1. 目标就在相邻格 → 直接走
//  2. greedy：朝目标方向的那一格可走 → 走它
//  3. A*：算出完整路径，取第一步（并缓存后续路径）
//
// 全失败返回 false（这只怪这轮不动）。
func (m *MapState) chaseStep(uid uint64, from, target Point, ai *AIState, r *rand.Rand) (Point, bool) {
	// 1. 相邻直接走
	if ChebyshevDistance(from, target) <= 1 {
		if m.CanOccupy(uid, target.X, target.Y) {
			return target, true
		}
		// 目标格被占（比如玩家站在那里）：找一个相邻空位挤过去
		for _, off := range directionOffsets[DirUp:DirEnd] {
			next := Point{X: target.X + off.X, Y: target.Y + off.Y}
			if next.Equal(from) || !m.CanOccupy(uid, next.X, next.Y) {
				continue
			}
			if ChebyshevDistance(next, target) <= 1 {
				return next, true
			}
		}
		return Point{}, false
	}

	// 2. greedy：朝目标方向走一格
	//
	// ★ 关键：greedy 失败必须**落到 A\***，不能直接 return false。
	//
	//	初版在斜向被墙挡住时直接 `return false`，于是怪会永远卡在
	//	墙角外侧 —— 明明 A* 能绕过去，却一步都不走。
	//	实测：怪追到 (4,7)，目标在右上，greedy 想斜穿 (5,6)，
	//	但 (4,6) 是墙（不能穿墙角），greedy 返回 false 后
	//	整个函数就结束了，路径缓存里的 (5,7) 永远用不上。
	//
	//	原版 moveOneStepCombine 的语义正是「greedy 不成就 next」。
	if greedy := stepToward(from, target); !greedy.Equal(from) && m.CanOccupy(uid, greedy.X, greedy.Y) {
		// 斜向要检查两个直向是否也通（不能穿墙角）
		diagonalOK := true
		if greedy.X != from.X && greedy.Y != from.Y {
			if !m.CanOccupy(uid, greedy.X, from.Y) || !m.CanOccupy(uid, from.X, greedy.Y) {
				diagonalOK = false
			}
		}
		if diagonalOK {
			return greedy, true
		}
	}

	// 3. A*：用缓存的路径，失效则重算
	if ai.PathValid(m.NowMS) {
		if next, ok := ai.NextStep(); ok && m.CanOccupy(uid, next.X, next.Y) {
			return next, true
		}
		ai.DropPath()
	}

	// ★ 不能直接寻路到「目标格」——那一格被目标自己占着，
	//   CanOccupy 必然返回 false，A* 一律失败。
	//   原版也是寻路到「目标相邻的可站格」。
	dest, hasDest := m.findApproachGrid(uid, target)
	if !hasDest {
		return Point{}, false
	}
	path, err := m.FindPath(uid, dest, maxAStarVisited)
	if err != nil || len(path) == 0 {
		// A* 都找不到：随机挪一步，避免原地卡死
		if r != nil && r.Intn(2) == 0 {
			next := randomAdjacent(from, r)
			if m.CanOccupy(uid, next.X, next.Y) {
				return next, true
			}
		}
		return Point{}, false
	}

	// 缓存后续路径（含最后一步），先走第一步
	ai.CachePath(path, m.NowMS)
	if next, ok := ai.NextStep(); ok {
		return next, true
	}
	return Point{}, false
}

// maxAStarVisited 是单次 A* 的节点上限。
//
// 600×600 地图最坏 36 万格，全展开要几十毫秒。设 2 万可以让
// 「同一张图上几十只怪同时寻路」的总耗时控制在可接受范围；
// 超限就放弃这条路径（怪这轮不动，下轮再试）。
const maxAStarVisited = 20000

// findApproachGrid 找一个「贴着目标、当前可站」的格作为寻路终点。
//
// 优先选离怪物最近的相邻格，避免绕远路。
func (m *MapState) findApproachGrid(uid uint64, target Point) (Point, bool) {
	best := Point{}
	bestDist := int(^uint(0) >> 1)
	found := false
	for _, off := range directionOffsets[DirUp:DirEnd] {
		p := Point{X: target.X + off.X, Y: target.Y + off.Y}
		if !m.CanOccupy(uid, p.X, p.Y) {
			continue
		}
		// 用怪物当前位置算距离需要实体，这里拿一下
		e, ok := m.Entities[uid]
		if !ok {
			continue
		}
		dist := ChebyshevDistance(e.Point(), p)
		if dist < bestDist {
			bestDist = dist
			best = p
			found = true
		}
	}
	return best, found
}

// 行为模式常量（monsterrecord.hpp:37-39）。
const (
	BehaveModeDefault = 0
	BehaveModeNeutral = 1
	BehaveModeGuard   = 2
)

// PKPointRed 是红名阈值（原版 PKPOINT_REDNAME）。
const PKPointRed = 100

// isHostileTo 判断某种行为模式的怪是否该对某目标敌对。
//
// 对齐 monster.cpp:972 searchNearestTarget 与 serverguard.cpp:80 checkFriend：
//
//	BM_DEFAULT：对玩家一律敌对
//	BM_NEUTRAL：不主动（返回 false，由 ShouldAggro 再兜一层）
//	BM_GUARD：  offender（打过我）或红名才算敌对
func (m *MapState) isHostileTo(rec MonsterRecord, selfUID uint64, target *Entity, targetUID uint64) bool {
	switch rec.BehaveMode {
	case BehaveModeNeutral:
		return false
	case BehaveModeGuard:
		// 1. 打过我的（在仇恨列表里）
		if m.selfOffendsTarget(selfUID, targetUID) {
			return true
		}
		// 2. 红名
		return target.PKPoint >= PKPointRed
	default:
		return true
	}
}

// selfOffendsTarget 判断 targetUID 是否在 selfUID 的仇恨列表里。
func (m *MapState) selfOffendsTarget(selfUID, targetUID uint64) bool {
	e, ok := m.Entities[selfUID]
	if !ok {
		return false
	}
	return e.Offenders.IsOffender(targetUID)
}
