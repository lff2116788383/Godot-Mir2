package world

import "testing"

// TestMonsterAttacksPlayer 验证「怪主动找到并攻击玩家」的完整链路。
func TestMonsterAttacksPlayer(t *testing.T) {
	data := &MapData{Width: 30, Height: 30}
	data.Cells = make([]Land, 30*30)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	st, _ := NewMapState(1, data)

	rec := MonsterRecord{
		Name: "鸡", HP: 50, View: 5, WalkWait: 1, AttackWait: 1,
		BehaveMode: BehaveDefault,
		DC:         AbilityBounds{Min: 3, Max: 5},
	}
	monster := Entity{
		UID: 10, Type: EntityMonster, Name: "鸡", X: 10, Y: 10,
		HP: 50, MaxHP: 50, ViewRange: 5, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	}
	st.AddEntity(monster)
	st.MutateEntity(10, func(e *Entity) error { e.MonsterRec = &rec; return nil })

	// 玩家在 3 格外
	player := Entity{
		UID: 20, Type: EntityPlayer, Name: "alice", X: 13, Y: 10,
		HP: 100, MaxHP: 100, ViewRange: 10, Direction: DirDown,
	}
	if _, err := st.AddEntity(player); err != nil {
		t.Fatal(err)
	}

	moves, attacks := 0, 0
	for ms := int64(1000); ms <= 30000; ms += 100 {
		// Advance 内部已经驱动了 AI，返回的事件里带 ai_move / ai_attack
		for _, ev := range st.Advance(ms) {
			switch ev.Kind {
			case "ai_move":
				moves++
			case "ai_attack":
				attacks++
			}
		}
	}

	t.Logf("30 秒：移动 %d 次，攻击 %d 次", moves, attacks)

	p, _ := st.Entity(20)
	t.Logf("玩家 HP: %d/100", p.HP)

	if moves == 0 {
		t.Fatal("怪物应该朝玩家移动")
	}
	if attacks == 0 {
		t.Fatal("追上后应该攻击")
	}
	if p.HP >= 100 {
		t.Fatal("玩家应该掉血")
	}
}
