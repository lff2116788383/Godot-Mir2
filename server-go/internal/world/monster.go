package world

import (
	"math/rand"
)

// MonsterRecord 是怪物静态配置里 AI 与掉落需要的部分。
//
// 【依据】common/src/monsterrecord.hpp:60-118：
//
//	hp/mp/exp          基础数值
//	dc[2]/mc[2]        伤害区间
//	ac[2]/mac[2]       防御区间
//	dcHit/mcHit        命中
//	dcDodge/mcDodge    闪避
//	view               视野半径（0 = 真瞎，只能靠接触发现）
//	ai                 0..100，越大越「聪明」（决定追击策略）
//	behaveMode         0=默认(敌对玩家) 1=中立 2=守卫
//	walkWait/walkStep  走路间隔 ms / 每步格数
//	attackWait         攻击间隔 ms
//	dropItemList       二维掉落表：外层每个分组内按顺序判定，命中即停
//
// 掉落表的结构很关键（monsterrecord.hpp:110-116）：
//
//	dropItemList 是 initializer_list<initializer_list<DropItem>>
//	内层每一组是一条「互斥链」：按顺序逐个 roll，命中一个就退出该组
//	外层各组独立判定，所以一只怪可以掉多组东西
type MonsterRecord struct {
	Name       string
	LookID     int
	HP         int32
	MP         int32
	Exp        uint64
	DC         AbilityBounds
	MC         AbilityBounds
	AC         AbilityBounds
	MAC        AbilityBounds
	DCHit      int
	MCHit      int
	DCDodge    int
	MCDodge    int
	View       int
	AI         int
	BehaveMode int
	WalkWait   int
	WalkStep   int
	AttackWait int
	DCDodgeSet bool
	DropItems  [][]DropItem
	Elem       ElementResist
}

// DropItem 是一条掉落配置。
//
// 【依据】monsterrecord.hpp:104-109：
//
//	item      物品名
//	probRecip 概率倒数，1 表示必掉，N 表示 1/N 概率
//	repeat    最多重复几次（每次独立 roll）
//	count     每次掉几个
type DropItem struct {
	Item      string
	ProbRecip int
	Repeat    int
	Count     int
}

// DropResult 是一次掉落结算的产物。
type DropResult struct {
	Item  string
	Count int
}

// BehaveMode 取值（monsterrecord.hpp:32-35）。
const (
	BehaveDefault = 0 // 敌对玩家
	BehaveNeutral = 1 // 中立，被打才还手
	BehaveGuard   = 2 // 守卫，只打红名
)

// RollDrop 结算一只怪的掉落。
//
// 【依据】monsterrecord.hpp 的 dropItemList 语义 + 传奇通用规则：
//   - 外层每个分组都独立判定（所以能掉多样东西）
//   - 内层是互斥链：按顺序 roll，命中一个就退出该组
//   - probRecip=1 必掉；probRecip=N 表示 1/N 概率
//   - repeat 是「最多几次」，每次独立 roll
//
// 注意：原版具体实现要看 monster.cpp 的 goDie，但掉落表本身的语义
// 从结构定义就能确定。这里按结构语义实现，行为差异在 B8 差分时对齐。
func RollDrop(rec MonsterRecord, r *rand.Rand) []DropResult {
	var out []DropResult
	for _, group := range rec.DropItems {
		// hit 标记本组是否已命中，命中后整组退出（互斥链）
		hit := false
		for _, d := range group {
			if hit {
				break
			}
			repeat := d.Repeat
			if repeat <= 0 {
				repeat = 1
			}
			count := d.Count
			if count <= 0 {
				count = 1
			}
			for i := 0; i < repeat; i++ {
				if !rollProb(d.ProbRecip, r) {
					continue
				}
				out = append(out, DropResult{Item: d.Item, Count: count})
				hit = true
				break
			}
		}
	}
	return out
}

// rollProb 判定 probRecip 概率：1 必中，N 表示 1/N。
func rollProb(probRecip int, r *rand.Rand) bool {
	if probRecip <= 1 {
		return true
	}
	return r.Intn(probRecip) == 0
}

// CombatStatsOf 从怪物记录构造受击结算用的属性集合。
func CombatStatsOf(rec MonsterRecord) CombatStats {
	return CombatStats{
		DC:      rec.DC,
		MC:      rec.MC,
		DCHit:   rec.DCHit,
		DCDodge: rec.DCDodge,
		MCHit:   rec.MCHit,
		MCDodge: rec.MCDodge,
		AC:      rec.AC,
		MAC:     rec.MAC,
		Elem:    rec.Elem,
	}
}

// AIState 是怪物运行时 AI 状态。
//
// 【依据】monster.cpp 的 m_astarCache / m_lastActionTime / m_idleWaitToken：
//
//	lastMoveMS   上次移动时刻，用于 walkWait 冷却
//	lastAttackMS 上次攻击时刻，用于 attackWait 冷却
//	targetUID    当前仇恨目标
//	path         缓存的 A* 路径（原版每 2 秒丢弃一次）
//	pathExpireMS 路径失效时刻
type AIState struct {
	LastMoveMS   int64
	LastAttackMS int64
	TargetUID    uint64
	Path         []Point
	PathExpireMS int64
	Idle         bool
}

// AStarCacheRefreshMS 对应 monster.hpp:33 的 m_refresh = 2000。
//
// 源码注释解释得很清楚：
//
//	we refresh (drop) the cache every 2 sec
//	since co's are moving and valid road won't keep valid
//	2 sec because many monsters have WalkWait = 1sec
const AStarCacheRefreshMS int64 = 2000

// CanMoveNow 判断 walkWait 冷却是否已过。
func (s *AIState) CanMoveNow(nowMS int64, rec MonsterRecord) bool {
	if rec.WalkWait <= 0 {
		return true
	}
	return nowMS-s.LastMoveMS >= int64(rec.WalkWait)
}

// CanAttackNow 判断 attackWait 冷却是否已过。
func (s *AIState) CanAttackNow(nowMS int64, rec MonsterRecord) bool {
	if rec.AttackWait <= 0 {
		return true
	}
	return nowMS-s.LastAttackMS >= int64(rec.AttackWait)
}

// MarkMove 记录一次移动。
func (s *AIState) MarkMove(nowMS int64) { s.LastMoveMS = nowMS }

// MarkAttack 记录一次攻击。
func (s *AIState) MarkAttack(nowMS int64) { s.LastAttackMS = nowMS }

// PathValid 判断缓存路径是否还能用（对应 AStarCache::retrieve 的时间校验）。
func (s *AIState) PathValid(nowMS int64) bool {
	if len(s.Path) == 0 {
		return false
	}
	return nowMS < s.PathExpireMS
}

// CachePath 缓存一条路径，2 秒后失效。
func (s *AIState) CachePath(path []Point, nowMS int64) {
	s.Path = path
	s.PathExpireMS = nowMS + AStarCacheRefreshMS
}

// DropPath 主动丢弃缓存（对应 AStarCache 的 drop）。
func (s *AIState) DropPath() {
	s.Path = nil
	s.PathExpireMS = 0
}

// NextStep 取路径的下一步；同时把已走过的点从缓存里删掉。
func (s *AIState) NextStep() (Point, bool) {
	if len(s.Path) == 0 {
		return Point{}, false
	}
	next := s.Path[0]
	s.Path = s.Path[1:]
	return next, true
}

// MonsterViewDistance 返回实际使用的视野半径。
//
// 【依据】monster.cpp:988-993 注释：
//
//	"for monster like ServerCannibalPlant with view distance 1"
//
// 视野 0 的怪（真瞎）只能靠接触发现目标，这里统一钳到至少 1，
// 否则这类怪永远不会主动攻击任何东西。
func MonsterViewDistance(rec MonsterRecord) int {
	if rec.View < 1 {
		return 1
	}
	return rec.View
}

// InViewRange 判断目标是否在视野内（切比雪夫距离）。
func InViewRange(rec MonsterRecord, self, target Point) bool {
	return ChebyshevDistance(self, target) <= MonsterViewDistance(rec)
}

// ShouldAggro 判断怪物是否该主动攻击某目标。
//
// 【依据】monsterrecord.hpp:32-35 behaveMode：
//
//	BM_DEFAULT 0 敌对玩家（默认主动）
//	BM_NEUTRAL 1 中立（被打才还手）
//	BM_GUARD   2 守卫（只打红名）
//
// 这里把「是否敌对」作为入参，由上层按阵营/红名判定后传入，
// 本函数只负责把行为模式与敌对状态组合成「是否主动攻击」。
func ShouldAggro(rec MonsterRecord, hostile bool, alreadyOffended bool) bool {
	if alreadyOffended {
		return true // 已经被打过，任何模式都会还手
	}
	switch rec.BehaveMode {
	case BehaveNeutral:
		return false
	case BehaveGuard:
		return hostile // 守卫只打红名
	default:
		return hostile
	}
}

// SpawnPoint 是一个刷新点配置。
type SpawnPoint struct {
	MonsterName string
	X           int
	Y           int
	Count       int    // 同时存在数量
	IntervalMS  int64  // 刷新间隔
	Area        [4]int // 刷新区域 x,y,w,h（可选，全 0 表示定点）
}

// SpawnState 是一个刷新点的运行时状态。
type SpawnState struct {
	Config      SpawnPoint
	Alive       int
	NextSpawnMS int64
}

// SpawnManager 管理地图上的怪物刷新。
//
// 【依据】servermap.cpp 的怪物刷新逻辑（m_spawnPointList / 定时刷新）。
// 原版用 map 自己的定时器驱动；这里做成纯函数式状态机，
// 由地图 actor 每次 tick 调用 Update，便于单测。
type SpawnManager struct {
	points []*SpawnState
	r      *rand.Rand
}

// NewSpawnManager 创建刷新管理器。
func NewSpawnManager(r *rand.Rand) *SpawnManager {
	if r == nil {
		r = rand.New(rand.NewSource(1))
	}
	return &SpawnManager{r: r}
}

// AddPoint 添加刷新点。
func (m *SpawnManager) AddPoint(p SpawnPoint) {
	if p.Count <= 0 {
		p.Count = 1
	}
	if p.IntervalMS <= 0 {
		p.IntervalMS = 10000
	}
	m.points = append(m.points, &SpawnState{Config: p})
}

// SpawnRequest 是「该刷一只怪」的请求。
type SpawnRequest struct {
	MonsterName string
	X           int
	Y           int
}

// Update 推进刷新逻辑，返回本 tick 需要刷出的怪物。
//
// 规则：
//   - 存活数 < Count 且已到刷新时刻 -> 请求刷怪
//   - 有刷新区域时在区域内随机取点，否则用定点
//   - 每次 tick 每个刷新点最多补一只，避免瞬间刷满
func (m *SpawnManager) Update(nowMS int64) []SpawnRequest {
	var out []SpawnRequest
	for _, st := range m.points {
		if st.Alive >= st.Config.Count {
			continue
		}
		if nowMS < st.NextSpawnMS {
			continue
		}
		x, y := st.Config.X, st.Config.Y
		area := st.Config.Area
		if area[2] > 0 && area[3] > 0 {
			x = area[0] + m.r.Intn(area[2])
			y = area[1] + m.r.Intn(area[3])
		}
		out = append(out, SpawnRequest{MonsterName: st.Config.MonsterName, X: x, Y: y})
		st.Alive++
		st.NextSpawnMS = nowMS + st.Config.IntervalMS
	}
	return out
}

// OnDeath 通知某刷新点有怪死亡，释放名额。
func (m *SpawnManager) OnDeath(monsterName string) {
	for _, st := range m.points {
		if st.Config.MonsterName == monsterName && st.Alive > 0 {
			st.Alive--
			return
		}
	}
}

// AliveCount 返回某怪物的当前存活数。
func (m *SpawnManager) AliveCount(monsterName string) int {
	n := 0
	for _, st := range m.points {
		if st.Config.MonsterName == monsterName {
			n += st.Alive
		}
	}
	return n
}

// TotalAlive 返回所有刷新点的存活总数。
func (m *SpawnManager) TotalAlive() int {
	n := 0
	for _, st := range m.points {
		n += st.Alive
	}
	return n
}
