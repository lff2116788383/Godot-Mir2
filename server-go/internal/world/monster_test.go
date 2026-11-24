package world

import (
	"math/rand"
	"testing"
)

// ---------- 掉落 ----------

func TestRollDropCertain(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	rec := MonsterRecord{
		DropItems: [][]DropItem{
			{{Item: "金创药", ProbRecip: 1, Repeat: 1, Count: 2}},
		},
	}
	out := RollDrop(rec, r)
	if len(out) != 1 || out[0].Item != "金创药" || out[0].Count != 2 {
		t.Fatalf("probRecip=1 必掉，实际 %v", out)
	}
}

func TestRollDropExclusiveChain(t *testing.T) {
	// 内层互斥链：三项任意命中一个就退出本组，绝不能掉两个
	r := rand.New(rand.NewSource(11))
	rec := MonsterRecord{
		DropItems: [][]DropItem{
			{
				{Item: "A", ProbRecip: 1, Repeat: 1, Count: 1},
				{Item: "B", ProbRecip: 1, Repeat: 1, Count: 1},
				{Item: "C", ProbRecip: 1, Repeat: 1, Count: 1},
			},
		},
	}
	for i := 0; i < 100; i++ {
		out := RollDrop(rec, r)
		if len(out) != 1 {
			t.Fatalf("互斥链只应命中 1 项，实际 %d 项: %v", len(out), out)
		}
		// 因为 A 的 probRecip=1 必中，所以永远是 A
		if out[0].Item != "A" {
			t.Fatalf("应按顺序先判 A，实际 %s", out[0].Item)
		}
	}
}

func TestRollDropMultipleGroups(t *testing.T) {
	// 外层各组独立：一组掉药，一组掉装备，互不影响
	r := rand.New(rand.NewSource(13))
	rec := MonsterRecord{
		DropItems: [][]DropItem{
			{{Item: "药", ProbRecip: 1, Count: 1}},
			{{Item: "剑", ProbRecip: 1, Count: 1}},
		},
	}
	out := RollDrop(rec, r)
	if len(out) != 2 {
		t.Fatalf("两组独立判定应掉 2 件，实际 %d: %v", len(out), out)
	}
}

func TestRollDropProbability(t *testing.T) {
	// probRecip=2 应约 50% 命中
	r := rand.New(rand.NewSource(17))
	rec := MonsterRecord{
		DropItems: [][]DropItem{
			{{Item: "稀有", ProbRecip: 2, Repeat: 1, Count: 1}},
		},
	}
	hits := 0
	const rounds = 4000
	for i := 0; i < rounds; i++ {
		if len(RollDrop(rec, r)) > 0 {
			hits++
		}
	}
	ratio := float64(hits) / float64(rounds)
	// 1/2 概率，4k 次应落在 0.45~0.55
	if ratio < 0.45 || ratio > 0.55 {
		t.Fatalf("probRecip=2 命中率应约 0.5，实际 %.3f", ratio)
	}
}

func TestRollDropNoDrop(t *testing.T) {
	r := rand.New(rand.NewSource(19))
	rec := MonsterRecord{DropItems: nil}
	if out := RollDrop(rec, r); len(out) != 0 {
		t.Fatalf("无掉落表不应掉落，实际 %v", out)
	}
}

// ---------- AI 冷却 ----------

func TestAICooldown(t *testing.T) {
	rec := MonsterRecord{WalkWait: 1000, AttackWait: 1500}
	var s AIState
	// 初始 lastMoveMS=0，nowMS=0 -> 已过 0 < 1000，不该能动
	if s.CanMoveNow(0, rec) {
		t.Fatal("初始应处于冷却")
	}
	s.MarkMove(1000)
	if s.CanMoveNow(1500, rec) {
		t.Fatal("刚移动 500ms，未到 1000ms 冷却")
	}
	if !s.CanMoveNow(2000, rec) {
		t.Fatal("已过 1000ms，应可移动")
	}

	s.MarkAttack(3000)
	if s.CanAttackNow(3500, rec) {
		t.Fatal("刚攻击 500ms，未到 1500ms 冷却")
	}
	if !s.CanAttackNow(4500, rec) {
		t.Fatal("已过 1500ms，应可攻击")
	}
}

func TestAICooldownZeroMeansImmediate(t *testing.T) {
	// walkWait=0 表示无冷却，随时可动
	rec := MonsterRecord{WalkWait: 0, AttackWait: 0}
	var s AIState
	if !s.CanMoveNow(0, rec) {
		t.Fatal("walkWait=0 应无冷却")
	}
	if !s.CanAttackNow(0, rec) {
		t.Fatal("attackWait=0 应无冷却")
	}
}

func TestAIPathCacheExpiry(t *testing.T) {
	var s AIState
	if s.PathValid(0) {
		t.Fatal("空路径无效")
	}
	s.CachePath([]Point{{1, 1}, {2, 2}}, 1000)
	if !s.PathValid(1000) {
		t.Fatal("刚缓存的路径应有效")
	}
	// AStarCacheRefreshMS = 2000，所以 3000 就是边界
	if !s.PathValid(2999) {
		t.Fatal("未到 2000ms 应仍有效")
	}
	if s.PathValid(3000) {
		t.Fatal("到达刷新时刻应失效（源码注释：每 2 秒丢弃）")
	}
}

func TestAINextStepPops(t *testing.T) {
	var s AIState
	s.CachePath([]Point{{1, 1}, {2, 2}}, 0)
	p1, ok := s.NextStep()
	if !ok || p1.X != 1 || p1.Y != 1 {
		t.Fatalf("第一步应是 (1,1)，实际 %v", p1)
	}
	p2, ok := s.NextStep()
	if !ok || p2.X != 2 {
		t.Fatalf("第二步应是 (2,2)，实际 %v", p2)
	}
	if _, ok := s.NextStep(); ok {
		t.Fatal("路径已空，不应再返回")
	}
}

// ---------- 视野与仇恨 ----------

func TestMonsterViewDistance(t *testing.T) {
	// view=0 是真瞎，但必须钳到 1，否则永远不主动攻击
	if got := MonsterViewDistance(MonsterRecord{View: 0}); got != 1 {
		t.Fatalf("view=0 应钳到 1，实际 %d", got)
	}
	if got := MonsterViewDistance(MonsterRecord{View: 7}); got != 7 {
		t.Fatalf("view=7 应保持，实际 %d", got)
	}
}

func TestInViewRange(t *testing.T) {
	rec := MonsterRecord{View: 5}
	self := Point{10, 10}
	// 切比雪夫距离：对角 (15,15) 距离 5，算在视野内
	if !InViewRange(rec, self, Point{15, 15}) {
		t.Fatal("对角距离 5 应在视野内")
	}
	if InViewRange(rec, self, Point{16, 10}) {
		t.Fatal("距离 6 超出视野")
	}
}

func TestShouldAggro(t *testing.T) {
	// 默认模式：敌对就主动
	if !ShouldAggro(MonsterRecord{BehaveMode: BehaveDefault}, true, false) {
		t.Fatal("默认模式遇敌对目标应主动")
	}
	if ShouldAggro(MonsterRecord{BehaveMode: BehaveDefault}, false, false) {
		t.Fatal("默认模式遇友方不应主动")
	}
	// 中立：不主动，但被打就还手
	if ShouldAggro(MonsterRecord{BehaveMode: BehaveNeutral}, true, false) {
		t.Fatal("中立模式不应主动")
	}
	if !ShouldAggro(MonsterRecord{BehaveMode: BehaveNeutral}, true, true) {
		t.Fatal("中立模式被打后应还手")
	}
	// 守卫：只打红名
	if !ShouldAggro(MonsterRecord{BehaveMode: BehaveGuard}, true, false) {
		t.Fatal("守卫遇红名应攻击")
	}
	if ShouldAggro(MonsterRecord{BehaveMode: BehaveGuard}, false, false) {
		t.Fatal("守卫遇白名不应攻击")
	}
}

// ---------- 刷新 ----------

func TestSpawnManagerBasic(t *testing.T) {
	m := NewSpawnManager(rand.New(rand.NewSource(23)))
	m.AddPoint(SpawnPoint{MonsterName: "鸡", X: 10, Y: 10, Count: 2, IntervalMS: 1000})

	// 首次 tick 刷 1 只（每 tick 最多补 1 只，避免瞬满）
	out := m.Update(0)
	if len(out) != 1 || out[0].X != 10 || out[0].Y != 10 {
		t.Fatalf("首次应刷 1 只定点怪，实际 %v", out)
	}
	if m.AliveCount("鸡") != 1 {
		t.Fatal("存活数应为 1")
	}

	// 立刻再 tick 应该被间隔挡住
	if out := m.Update(0); len(out) != 0 {
		t.Fatal("间隔未到不应再刷")
	}

	// 到 1000ms 刷第 2 只
	out = m.Update(1000)
	if len(out) != 1 || m.AliveCount("鸡") != 2 {
		t.Fatalf("应刷到 2 只，实际 %d", m.AliveCount("鸡"))
	}

	// 已达上限，再 tick 也不刷
	if out := m.Update(9999); len(out) != 0 {
		t.Fatal("已达 Count 上限不应再刷")
	}
}

func TestSpawnManagerDeathFreesSlot(t *testing.T) {
	m := NewSpawnManager(rand.New(rand.NewSource(29)))
	m.AddPoint(SpawnPoint{MonsterName: "鹿", X: 1, Y: 1, Count: 1, IntervalMS: 100})
	m.Update(0)
	if m.TotalAlive() != 1 {
		t.Fatal("应刷出 1 只")
	}
	m.OnDeath("鹿")
	if m.AliveCount("鹿") != 0 {
		t.Fatal("死亡后应释放名额")
	}
	// 下一 tick 可再刷
	if out := m.Update(100); len(out) != 1 {
		t.Fatal("名额释放后应能再刷")
	}
}

func TestSpawnManagerArea(t *testing.T) {
	m := NewSpawnManager(rand.New(rand.NewSource(31)))
	m.AddPoint(SpawnPoint{
		MonsterName: "虫", Count: 20, IntervalMS: 1,
		Area: [4]int{100, 200, 10, 10},
	})
	// 刷 20 只，全部应落在区域 [100,110) x [200,210)
	for i := 0; i < 20; i++ {
		out := m.Update(int64(i * 10))
		for _, req := range out {
			if req.X < 100 || req.X >= 110 || req.Y < 200 || req.Y >= 210 {
				t.Fatalf("刷怪点应在区域内，实际 (%d,%d)", req.X, req.Y)
			}
		}
	}
}

func TestSpawnManagerDeathUnknownMonster(t *testing.T) {
	m := NewSpawnManager(nil)
	m.AddPoint(SpawnPoint{MonsterName: "狼", Count: 1, IntervalMS: 100})
	m.Update(0)
	// 不存在的怪名不应崩溃，也不应影响计数
	m.OnDeath("不存在的怪")
	if m.AliveCount("狼") != 1 {
		t.Fatal("无关怪名不应影响计数")
	}
}

// ---------- CombatStatsOf ----------

func TestCombatStatsOf(t *testing.T) {
	rec := MonsterRecord{
		DC:    AbilityBounds{Min: 2, Max: 6},
		AC:    AbilityBounds{Min: 1, Max: 3},
		DCHit: 12, MCDodge: 8,
		Elem: ElementResist{Ice: -2},
	}
	cs := CombatStatsOf(rec)
	if cs.DC.Min != 2 || cs.DC.Max != 6 {
		t.Fatal("DC 应透传")
	}
	if cs.AC.Min != 1 || cs.AC.Max != 3 {
		t.Fatal("AC 应透传")
	}
	if cs.DCHit != 12 || cs.MCDodge != 8 {
		t.Fatal("命中/闪避应透传")
	}
	if cs.Elem.Ice != -2 {
		t.Fatal("元素抗性应透传")
	}
}
