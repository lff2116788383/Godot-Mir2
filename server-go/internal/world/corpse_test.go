package world

import "testing"

// TestCorpseFadesOut 验证尸体在 1 秒后从地图移除。
func TestCorpseFadesOut(t *testing.T) {
	const w, h = 20, 20
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	st, _ := NewMapState(1, data)

	rec := MonsterRecord{
		Name: "鸡", HP: 10, View: 5, WalkWait: 1000, AttackWait: 1000,
		BehaveMode: BehaveModeDefault, DeadFadeOut: 1,
	}
	st.AddEntity(Entity{
		UID: 10, Type: EntityMonster, Name: "鸡", X: 5, Y: 5,
		HP: 10, MaxHP: 10, ViewRange: 5, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	})
	st.MutateEntity(10, func(e *Entity) error { e.MonsterRec = &rec; return nil })
	st.AddEntity(Entity{
		UID: 20, Type: EntityPlayer, Name: "alice", X: 6, Y: 5,
		HP: 100, MaxHP: 100, ViewRange: 10, Direction: DirLeft,
	})

	// 直接打死（Combat 要命中+破防，测试里属性为 0 打不动）
	st.Advance(1000)
	if _, err := st.Attack(20, 10, 999); err != nil {
		t.Fatalf("attack: %v", err)
	}
	e, ok := st.Entity(10)
	if !ok || !e.Dead() {
		t.Fatalf("怪应该死了: ok=%v hp=%d", ok, e.HP)
	}
	dieAt := e.DieAtMS
	t.Logf("死亡时刻 %d", dieAt)

	// 500ms 后还在
	fades := 0
	for _, ev := range st.Advance(dieAt + 500) {
		if ev.Kind == "corpse_fade" {
			fades++
		}
	}
	if _, still := st.Entity(10); !still {
		t.Fatal("500ms 时尸体不该消失")
	}

	// 再过 600ms 应该消失
	for _, ev := range st.Advance(dieAt + 1100) {
		if ev.Kind == "corpse_fade" {
			fades++
		}
	}
	if _, still := st.Entity(10); still {
		t.Fatal("1.1 秒后尸体应该消失")
	}
	if fades == 0 {
		t.Fatal("应该产生 corpse_fade 事件")
	}
	t.Logf("corpse_fade 事件 %d 次", fades)
}

// TestNoFadeOutKeepsCorpse 验证 deadFadeOut=0 的怪尸体被立刻移除（不保留）。
func TestNoFadeOutKeepsCorpse(t *testing.T) {
	const w, h = 20, 20
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	st, _ := NewMapState(1, data)

	rec := MonsterRecord{
		Name: "Boss", HP: 10, View: 5, WalkWait: 1000, AttackWait: 1000,
		BehaveMode: BehaveModeDefault, DeadFadeOut: 0,
	}
	st.AddEntity(Entity{
		UID: 10, Type: EntityMonster, Name: "Boss", X: 5, Y: 5,
		HP: 10, MaxHP: 10, ViewRange: 5, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	})
	st.MutateEntity(10, func(e *Entity) error { e.MonsterRec = &rec; return nil })
	st.AddEntity(Entity{
		UID: 20, Type: EntityPlayer, Name: "alice", X: 6, Y: 5,
		HP: 100, MaxHP: 100, ViewRange: 10, Direction: DirLeft,
	})

	st.Advance(1000)
	if _, err := st.Attack(20, 10, 999); err != nil {
		t.Fatalf("attack: %v", err)
	}

	// 下一次 tick 就该消失
	st.Advance(st.NowMS + 100)
	if _, still := st.Entity(10); still {
		t.Fatal("deadFadeOut=0 的怪死后不该保留尸体")
	}
}
