package world

import "testing"

// TestMonsterPathfindsAroundWall 验证怪能绕过「凹形墙」追到玩家。
//
// 布局（W=墙, .=空地, M=怪, P=玩家）：
//
//	列: 0 1 2 3 4 5 6 7 8
//	 0  . . . . . . . . .
//	 1  . W W W W W W W .
//	 2  . W . . . . . W .
//	 3  . W . M . . . W .
//	 4  . W . . . . . W .
//	 5  . W W W W W P W .    <- 玩家在墙围内
//	 6  . . . . . . . . .
//
// 玩家在封闭区域里（只在 5,7 处留一个缺口），怪必须绕到缺口才能进去。
func TestMonsterPathfindsAroundWall(t *testing.T) {
	const w, h = 10, 8
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	// 砌墙：把怪围在一个「只在上方留一个缺口」的盒子里
	//
	//	列: 0 1 2 3 4 5 6 7 8
	//	 1  . W W W W W W W .
	//	 2  . W . . . . . W .
	//	 3  . W . M . . . W .
	//	 4  . W . . . . . W .
	//	 5  . . . . . . . . .    <- 盒子底边不封，从下面出去
	//	 6  . . . . . . . P .
	//
	// 怪要走到玩家 (8,6) 必须先往下出盒、再绕过去。
	block := func(x, y int) { data.Cells[x+y*w] = Land{CanWalk: false} }
	for x := 1; x <= 7; x++ {
		block(x, 1) // 顶边
	}
	for y := 2; y <= 4; y++ {
		block(1, y) // 左边
		block(7, y) // 右边
	}

	st, err := NewMapState(1, data)
	if err != nil {
		t.Fatal(err)
	}

	rec := MonsterRecord{
		Name: "鸡", HP: 50, View: 10, WalkWait: 1, AttackWait: 1,
		BehaveMode: BehaveDefault, DC: AbilityBounds{Min: 1, Max: 2},
	}
	// 怪在 (3,3)：被墙围住，玩家在 (7,5)：墙外
	st.AddEntity(Entity{
		UID: 10, Type: EntityMonster, Name: "鸡", X: 3, Y: 3,
		HP: 50, MaxHP: 50, ViewRange: 10, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	})
	st.MutateEntity(10, func(e *Entity) error { e.MonsterRec = &rec; return nil })

	st.AddEntity(Entity{
		UID: 20, Type: EntityPlayer, Name: "alice", X: 8, Y: 6,
		HP: 500, MaxHP: 500, ViewRange: 10, Direction: DirDown,
	})

	moves, attacks := 0, 0
	var lastPos Point
	for ms := int64(1000); ms <= 60000; ms += 100 {
		for _, ev := range st.Advance(ms) {
			switch ev.Kind {
			case "ai_move":
				moves++
			case "ai_attack":
				attacks++
			}
		}
		if e, ok := st.Entity(10); ok {
			lastPos = e.Point()
		}
	}

	p, _ := st.Entity(20)
	t.Logf("60 秒：移动 %d 次，攻击 %d 次", moves, attacks)
	t.Logf("怪物终点 (%d,%d)，玩家 (%d,%d)，玩家 HP %d/500",
		lastPos.X, lastPos.Y, p.X, p.Y, p.HP)

	if moves == 0 {
		t.Fatal("怪物应该移动")
	}
	if attacks == 0 {
		t.Fatal("绕出墙后应该攻击到玩家（说明寻路成功）")
	}
}

// TestMonsterUsesAStarWhenGreedyFails 验证「greedy 被挡死时降级到 A*」。
//
// 布局：怪与玩家之间隔一道长竖墙，只有很远的端点能绕过去。
// greedy（朝目标走）会一直撞墙，必须靠 A* 找到绕行路线。
//
//	列: 0 1 2 3 4 5 6 7 8 9
//	 0  . . . . W . . . . .
//	 1  . . . . W . . . . .
//	 2  . . M . W . . P . .     <- 怪 (2,2)，玩家 (7,2)，中间 x=4 全程是墙
//	 3  . . . . W . . . . .
//	 4  . . . . W . . . . .
//	 5  . . . . W . . . . .
//	 6  . . . . W . . . . .
//	 7  . . . . . . . . . .     <- 墙只到 y=6，从这里绕
func TestMonsterUsesAStarWhenGreedyFails(t *testing.T) {
	const w, h = 10, 10
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	// x=4 的竖墙，y=0..6（下方留口）
	for y := 0; y <= 6; y++ {
		data.Cells[4+y*w] = Land{CanWalk: false}
	}

	st, _ := NewMapState(1, data)
	rec := MonsterRecord{
		Name: "鸡", HP: 50, View: 10, WalkWait: 1, AttackWait: 1,
		BehaveMode: BehaveDefault, DC: AbilityBounds{Min: 1, Max: 2},
	}
	st.AddEntity(Entity{
		UID: 10, Type: EntityMonster, Name: "鸡", X: 2, Y: 2,
		HP: 50, MaxHP: 50, ViewRange: 10, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	})
	st.MutateEntity(10, func(e *Entity) error { e.MonsterRec = &rec; return nil })
	st.AddEntity(Entity{
		UID: 20, Type: EntityPlayer, Name: "alice", X: 7, Y: 2,
		HP: 500, MaxHP: 500, ViewRange: 10, Direction: DirDown,
	})

	moves, attacks := 0, 0
	var lastPos Point
	for ms := int64(1000); ms <= 60000; ms += 100 {
		for _, ev := range st.Advance(ms) {
			switch ev.Kind {
			case "ai_move":
				moves++
			case "ai_attack":
				attacks++
			}
		}
		if e, ok := st.Entity(10); ok {
			lastPos = e.Point()
		}
	}

	p, _ := st.Entity(20)
	t.Logf("60 秒：移动 %d 次，攻击 %d 次", moves, attacks)
	t.Logf("怪物终点 (%d,%d)，玩家 (%d,%d)，玩家 HP %d/500",
		lastPos.X, lastPos.Y, p.X, p.Y, p.HP)

	if attacks == 0 {
		t.Fatal("应该绕过竖墙打到玩家")
	}
	// 绕行的路至少有 10 格（下到 y>6 再上来），移动次数应该明显多于直线距离
	if moves < 8 {
		t.Fatalf("绕行需要多步，实际只移动 %d 次", moves)
	}
}
