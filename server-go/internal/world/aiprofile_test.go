package world

import "testing"

// mkFlatMap 造一张全可走的 w×h 地图。
func mkFlatMap(t *testing.T, w, h int) *MapState {
	t.Helper()
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	st, err := NewMapState(1, data)
	if err != nil {
		t.Fatalf("new map: %v", err)
	}
	return st
}

// TestBossUsesRangedSpell 祖玛教主会远程打人，不需要贴身。
func TestBossUsesRangedSpell(t *testing.T) {
	st := mkFlatMap(t, 40, 40)

	name := "祖玛教主"
	rec := MonsterRecord{
		Name: name, HP: 5000, View: 10, WalkWait: 1000, AttackWait: 1000,
		BehaveMode: BehaveDefault,
		DC:         AbilityBounds{Min: 20, Max: 30},
		MC:         AbilityBounds{Min: 40, Max: 60},
	}
	ApplyProfile(&rec, name)
	if rec.Profile == nil || len(rec.Profile.Specials) == 0 {
		t.Fatal("祖玛教主应该有特殊技能")
	}

	monsterUID := uint64(10)
	st.AddEntity(Entity{
		UID: monsterUID, Type: EntityMonster, Name: name, X: 10, Y: 10,
		HP: 5000, MaxHP: 5000, ViewRange: 10, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	})
	st.MutateEntity(monsterUID, func(e *Entity) error { e.MonsterRec = &rec; return nil })

	// 玩家在 5 格外（远于近身攻击距离 1）
	st.AddEntity(Entity{
		UID: 20, Type: EntityPlayer, Name: "alice", X: 15, Y: 10,
		HP: 500, MaxHP: 500, ViewRange: 10, Direction: DirLeft, PKPoint: 0,
	})

	spellHits := 0
	moves := 0
	for ms := int64(1000); ms <= 15000; ms += 100 {
		for _, ev := range st.Advance(ms) {
			switch ev.Kind {
			case "ai_attack":
				spellHits++
			case "ai_move":
				moves++
			}
		}
	}

	p, _ := st.Entity(20)
	t.Logf("15 秒：攻击 %d 次，移动 %d 次，玩家 HP %d/500", spellHits, moves, p.HP)

	if spellHits == 0 {
		t.Fatal("Boss 应该远程攻击到玩家")
	}
	if p.HP >= 500 {
		t.Fatal("玩家应该掉血")
	}
	// ★ 关键断言：远程攻击让它不需要走到贴身才打
	// （移动次数应该明显少于攻击次数，否则说明它一直在追）
	if moves > spellHits {
		t.Fatalf("Boss 不该一直追着走：移动 %d 次 > 攻击 %d 次", moves, spellHits)
	}
}

// TestCannibalPlantCannotMove 食人花不能移动。
func TestCannibalPlantCannotMove(t *testing.T) {
	st := mkFlatMap(t, 40, 40)

	name := "食人花"
	rec := MonsterRecord{
		Name: name, HP: 500, View: 10, WalkWait: 1, AttackWait: 1000,
		BehaveMode: BehaveDefault, DC: AbilityBounds{Min: 5, Max: 10},
	}
	ApplyProfile(&rec, name)
	if rec.WalkWait < 100000 {
		t.Fatalf("食人花的 WalkWait 应该被改成极大值，实际 %d", rec.WalkWait)
	}

	st.AddEntity(Entity{
		UID: 10, Type: EntityMonster, Name: name, X: 10, Y: 10,
		HP: 500, MaxHP: 500, ViewRange: 10, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	})
	st.MutateEntity(10, func(e *Entity) error { e.MonsterRec = &rec; return nil })

	// 玩家在 5 格外：它的技能射程是 2，够不到
	st.AddEntity(Entity{
		UID: 20, Type: EntityPlayer, Name: "alice", X: 15, Y: 10,
		HP: 500, MaxHP: 500, ViewRange: 10, Direction: DirLeft,
	})

	moves := 0
	for ms := int64(1000); ms <= 20000; ms += 100 {
		for _, ev := range st.Advance(ms) {
			if ev.Kind == "ai_move" && ev.UID == 10 {
				moves++
			}
		}
	}

	if moves != 0 {
		t.Fatalf("食人花不该移动，实际移动 %d 次", moves)
	}
	e, _ := st.Entity(10)
	if e.X != 10 || e.Y != 10 {
		t.Fatalf("食人花位置变了：(%d,%d)", e.X, e.Y)
	}
	t.Log("食人花 20 秒内保持原地不动")
}

// TestPetFollowsMaster 神兽无目标时跟随主人。
func TestPetFollowsMaster(t *testing.T) {
	st := mkFlatMap(t, 60, 60)

	name := "神兽"
	rec := MonsterRecord{
		Name: name, HP: 500, View: 10, WalkWait: 100, AttackWait: 1000,
		BehaveMode: BehaveNeutral, // 不主动攻击，避免干扰跟随测试
		DC:         AbilityBounds{Min: 5, Max: 10},
	}
	ApplyProfile(&rec, name)

	// 主人在远处 (40,40)
	st.AddEntity(Entity{
		UID: 30, Type: EntityPlayer, Name: "master", X: 40, Y: 40,
		HP: 500, MaxHP: 500, ViewRange: 10, Direction: DirDown,
	})
	// 神兽在 (10,10)，离主人 30 格
	st.AddEntity(Entity{
		UID: 10, Type: EntityMonster, Name: name, X: 10, Y: 10,
		HP: 500, MaxHP: 500, ViewRange: 10, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	})
	st.MutateEntity(10, func(e *Entity) error {
		e.MonsterRec = &rec
		e.AI.MasterUID = 30
		return nil
	})

	for ms := int64(1000); ms <= 30000; ms += 100 {
		st.Advance(ms)
	}

	e, _ := st.Entity(10)
	dist := ChebyshevDistance(e.Point(), Point{X: 40, Y: 40})
	t.Logf("神兽从 (10,10) 跟到 (%d,%d)，距主人 %d 格", e.X, e.Y, dist)

	if dist > 5 {
		t.Fatalf("神兽应该跟到主人附近，实际还差 %d 格", dist)
	}
}

// TestPetDiesWithMaster 主人死了神兽也死。
func TestPetDiesWithMaster(t *testing.T) {
	st := mkFlatMap(t, 40, 40)

	name := "神兽"
	rec := MonsterRecord{
		Name: name, HP: 500, View: 10, WalkWait: 100, AttackWait: 1000,
		BehaveMode: BehaveNeutral, DC: AbilityBounds{Min: 1, Max: 2},
	}
	ApplyProfile(&rec, name)

	st.AddEntity(Entity{
		UID: 30, Type: EntityPlayer, Name: "master", X: 11, Y: 10,
		HP: 100, MaxHP: 100, ViewRange: 10, Direction: DirDown,
	})
	st.AddEntity(Entity{
		UID: 10, Type: EntityMonster, Name: name, X: 10, Y: 10,
		HP: 500, MaxHP: 500, ViewRange: 10, Direction: DirDown,
		Stats: CombatStatsOf(rec),
	})
	st.MutateEntity(10, func(e *Entity) error {
		e.MonsterRec = &rec
		e.AI.MasterUID = 30
		return nil
	})

	// 主人死掉
	st.MutateEntity(30, func(e *Entity) error {
		e.HP = 0
		e.DieAtMS = st.NowMS
		return nil
	})

	for ms := int64(1000); ms <= 5000; ms += 100 {
		st.Advance(ms)
	}

	e, ok := st.Entity(10)
	if ok && !e.Dead() {
		t.Fatalf("主人死后神兽应该也死，实际 HP=%d", e.HP)
	}
	t.Log("主人死亡 → 神兽跟随死亡")
}

// TestStandardMonsterHasNoProfile 普通怪不该有 profile。
func TestStandardMonsterHasNoProfile(t *testing.T) {
	rec := MonsterRecord{Name: "鸡"}
	ApplyProfile(&rec, "鸡")
	if rec.Profile != nil {
		t.Fatal("普通怪不该有 AI profile")
	}
}
