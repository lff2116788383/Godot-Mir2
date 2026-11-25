package world

import "testing"

// mkGuardMap 造一张 30x30 全可走地图。
func mkGuardMap(t *testing.T) *MapState {
	t.Helper()
	const w, h = 30, 30
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	st, err := NewMapState(1, data)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// addGuardMonster 放一个守卫（behaveMode=BM_GUARD）。
func addGuardMonster(t *testing.T, st *MapState, uid uint64, x, y int) {
	t.Helper()
	rec := MonsterRecord{
		Name: "大刀卫士", HP: 9999, View: 10, WalkWait: 1, AttackWait: 1,
		BehaveMode: BehaveModeGuard,
		DC:         AbilityBounds{Min: 10, Max: 15},
		AC:         AbilityBounds{Min: 110, Max: 110},
	}
	st.AddEntity(Entity{
		UID: uid, Type: EntityMonster, Name: "大刀卫士", X: x, Y: y,
		HP: 9999, MaxHP: 9999, ViewRange: 10, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	})
	st.MutateEntity(uid, func(e *Entity) error { e.MonsterRec = &rec; return nil })
}

// TestGuardIgnoresWhiteNamePlayer 守卫不该打白名玩家。
func TestGuardIgnoresWhiteNamePlayer(t *testing.T) {
	st := mkGuardMap(t)
	addGuardMonster(t, st, 10, 10, 10)

	// 白名玩家（PKPoint=0）就在守卫旁边
	st.AddEntity(Entity{
		UID: 20, Type: EntityPlayer, Name: "alice", X: 11, Y: 10,
		HP: 100, MaxHP: 100, ViewRange: 10, Direction: DirDown,
		PKPoint: 0,
	})

	attacks := 0
	for ms := int64(1000); ms <= 20000; ms += 100 {
		for _, ev := range st.Advance(ms) {
			if ev.Kind == "ai_attack" {
				attacks++
			}
		}
	}

	p, _ := st.Entity(20)
	t.Logf("20 秒：守卫攻击 %d 次，白名玩家 HP %d/100", attacks, p.HP)

	if attacks != 0 {
		t.Fatalf("守卫不该攻击白名玩家，实际攻击 %d 次", attacks)
	}
	if p.HP != 100 {
		t.Fatalf("白名玩家不该掉血，实际 HP %d", p.HP)
	}
}

// TestGuardAttacksRedNamePlayer 守卫该打红名玩家。
func TestGuardAttacksRedNamePlayer(t *testing.T) {
	st := mkGuardMap(t)
	addGuardMonster(t, st, 10, 10, 10)

	// 红名玩家（PKPoint=200）
	st.AddEntity(Entity{
		UID: 20, Type: EntityPlayer, Name: "pk", X: 11, Y: 10,
		HP: 500, MaxHP: 500, ViewRange: 10, Direction: DirDown,
		PKPoint: 200,
	})

	attacks := 0
	for ms := int64(1000); ms <= 20000; ms += 100 {
		for _, ev := range st.Advance(ms) {
			if ev.Kind == "ai_attack" {
				attacks++
			}
		}
	}

	p, _ := st.Entity(20)
	t.Logf("20 秒：守卫攻击 %d 次，红名玩家 HP %d/500", attacks, p.HP)

	if attacks == 0 {
		t.Fatal("守卫应该攻击红名玩家")
	}
	if p.HP >= 500 {
		t.Fatal("红名玩家应该掉血")
	}
}

// TestGuardRetaliatesAfterBeingHit 白名玩家打了守卫，守卫要还手。
//
// 原版 serverguard.cpp:95：玩家在 offender 列表里就 FT_ENEMY，
// 与是否红名无关。
func TestGuardRetaliatesAfterBeingHit(t *testing.T) {
	st := mkGuardMap(t)
	addGuardMonster(t, st, 10, 10, 10)

	st.AddEntity(Entity{
		UID: 20, Type: EntityPlayer, Name: "alice", X: 11, Y: 10,
		HP: 500, MaxHP: 500, ViewRange: 10, Direction: DirDown,
		PKPoint: 0,
	})

	// 玩家打守卫一下（记进守卫的仇恨列表）
	st.MutateEntity(10, func(e *Entity) error {
		e.Offenders.AddDamage(20, 5, 1)
		return nil
	})

	attacks := 0
	for ms := int64(1000); ms <= 20000; ms += 100 {
		for _, ev := range st.Advance(ms) {
			if ev.Kind == "ai_attack" {
				attacks++
			}
		}
	}

	p, _ := st.Entity(20)
	t.Logf("20 秒：守卫还手 %d 次，玩家 HP %d/500", attacks, p.HP)

	if attacks == 0 {
		t.Fatal("被打过的守卫应该还手")
	}
}

// TestNeutralMonsterDoesNotAttack 中立怪不主动攻击。
func TestNeutralMonsterDoesNotAttack(t *testing.T) {
	st := mkGuardMap(t)

	rec := MonsterRecord{
		Name: "鹿", HP: 50, View: 10, WalkWait: 1, AttackWait: 1,
		BehaveMode: BehaveModeNeutral,
		DC:         AbilityBounds{Min: 1, Max: 2},
	}
	st.AddEntity(Entity{
		UID: 10, Type: EntityMonster, Name: "鹿", X: 10, Y: 10,
		HP: 50, MaxHP: 50, ViewRange: 10, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	})
	st.MutateEntity(10, func(e *Entity) error { e.MonsterRec = &rec; return nil })

	st.AddEntity(Entity{
		UID: 20, Type: EntityPlayer, Name: "alice", X: 11, Y: 10,
		HP: 100, MaxHP: 100, ViewRange: 10, Direction: DirDown,
	})

	attacks := 0
	for ms := int64(1000); ms <= 20000; ms += 100 {
		for _, ev := range st.Advance(ms) {
			if ev.Kind == "ai_attack" {
				attacks++
			}
		}
	}

	p, _ := st.Entity(20)
	t.Logf("20 秒：中立怪攻击 %d 次，玩家 HP %d/100", attacks, p.HP)
	if attacks != 0 {
		t.Fatalf("中立怪不该主动攻击，实际 %d 次", attacks)
	}
	if p.HP != 100 {
		t.Fatalf("玩家不该掉血，实际 %d", p.HP)
	}
}
