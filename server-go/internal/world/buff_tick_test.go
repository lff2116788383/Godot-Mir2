package world

import "testing"

// TestBuffExpiryRestoresStats 是「buff 到期属性回落」的回归测试。
//
// 背景：初版 buff 到期只推 SM_BUFF off，**属性不会回落** ——
// 玩家上过一次幽灵盾，之后永久带着 +MAC。
// 这个 bug 比「buff 没效果」更严重：数值只涨不落。
func TestBuffExpiryRestoresStats(t *testing.T) {
	const w, h = 20, 20
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	st, _ := NewMapState(1, data)

	base := CombatStats{MAC: AbilityBounds{Min: 40, Max: 60}}
	st.AddEntity(Entity{
		UID: 10, Type: EntityPlayer, Name: "alice", X: 5, Y: 5,
		HP: 100, MaxHP: 100, ViewRange: 5, Direction: DirDown,
		Stats: base, BaseStats: base,
	})

	// 挂幽灵盾（+2 +5%，持续 2 秒）
	st.MutateEntity(10, func(e *Entity) error {
		e.Buffs = NewBuffList()
		return e.Buffs.Add(&Buff{
			ID: 9, SeqID: 1, ActiveActs: 1, RemainMS: 2000,
			Record: BuffRecord{
				Name: "幽灵盾", DurationMS: 2000,
				Acts: []BuffAttributeModifier{
					{Attr: AttrMACMin, Value: 2, Percentage: 5},
					{Attr: AttrMACMax, Value: 2, Percentage: 5},
				},
			},
		})
	})
	// 手动叠一次（模拟 RefreshStats）
	st.MutateEntity(10, func(e *Entity) error {
		e.Stats = e.BaseStats
		ApplyAttributeModifiers(&e.Stats, e.Buffs)
		return nil
	})

	e, _ := st.Entity(10)
	if e.Stats.MAC.Min != 44 {
		t.Fatalf("上 buff 后 MAC.Min = %d, want 44", e.Stats.MAC.Min)
	}

	// 跑 2.5 秒让 buff 到期
	expired := false
	for ms := int64(100); ms <= 2500; ms += 100 {
		for _, ev := range st.Advance(ms) {
			if ev.Kind == "buff_expired" && ev.UID == 10 {
				expired = true
			}
		}
	}
	if !expired {
		t.Fatal("2.5 秒后 buff 应该到期")
	}

	// ★ 这里断言的是「caller 应当重算」：
	// world 层只负责发事件，重算是 game 层的事。
	// 但我们可以直接验证「从 BaseStats 重算后回到原值」。
	st.MutateEntity(10, func(e *Entity) error {
		e.Stats = e.BaseStats
		ApplyAttributeModifiers(&e.Stats, e.Buffs)
		return nil
	})
	e, _ = st.Entity(10)
	if e.Stats.MAC.Min != 40 {
		t.Fatalf("buff 到期后 MAC.Min 应回到 40，实际 %d（属性没回落！）", e.Stats.MAC.Min)
	}
	t.Logf("幽灵盾到期：MAC 44 → %d（已回落）", e.Stats.MAC.Min)
}

// TestHPTriggerHeals 治愈术的「HP持续」按 500ms 回血。
//
// 【依据】buffrecord.inc 治愈术的 HP持续：
//
//	.duration = 6000, .on = BATGR_TIME, .tps = 2,
//	.arg = buffArgWrapper<BuffValuePercentage>(20, 5)
//
// 即每 500ms 回「20 + 5% 上限」点血。
func TestHPTriggerHeals(t *testing.T) {
	const w, h = 20, 20
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	st, _ := NewMapState(1, data)

	st.AddEntity(Entity{
		UID: 10, Type: EntityPlayer, Name: "alice", X: 5, Y: 5,
		HP: 10, MaxHP: 100, ViewRange: 5, Direction: DirDown,
	})

	st.MutateEntity(10, func(e *Entity) error {
		e.Buffs = NewBuffList()
		return e.Buffs.Add(&Buff{
			ID: 1, SeqID: 1, ActiveActs: 2, RemainMS: 6000,
			Record: BuffRecord{
				Name: "治愈术", DurationMS: 6000,
				Triggers: []BuffTrigger{{
					Name: "HP持续", Duration: 6000, On: BATGRTime, TPS: 2,
					Value: 20, Percentage: 5,
				}},
			},
		})
	})

	// 跑 1 秒 → 应该触发 2 次（500ms 一次）
	triggers := 0
	for ms := int64(100); ms <= 1000; ms += 100 {
		for _, ev := range st.Advance(ms) {
			if ev.Kind == "buff_trigger" && ev.UID == 10 {
				triggers++
			}
		}
	}
	if triggers != 2 {
		t.Fatalf("1 秒内应触发 2 次（tps=2），实际 %d 次", triggers)
	}

	e, _ := st.Entity(10)
	// 每次回 20 + round(100*5/100)=5 → 25 点，两次 50 点，10 → 60
	if e.HP != 60 {
		t.Fatalf("HP = %d, want 60（10 + 25×2）", e.HP)
	}
	t.Logf("治愈术 1 秒：HP 10 → %d", e.HP)
}

// TestPoisonTriggerDamages 施毒术的「HP持续」掉血。
func TestPoisonTriggerDamages(t *testing.T) {
	const w, h = 20, 20
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	st, _ := NewMapState(1, data)

	st.AddEntity(Entity{
		UID: 10, Type: EntityPlayer, Name: "victim", X: 5, Y: 5,
		HP: 100, MaxHP: 100, ViewRange: 5, Direction: DirDown,
	})

	st.MutateEntity(10, func(e *Entity) error {
		e.Buffs = NewBuffList()
		return e.Buffs.Add(&Buff{
			ID: 2, SeqID: 1, ActiveActs: 2, RemainMS: 6000,
			Record: BuffRecord{
				Name: "施毒术", DurationMS: 6000,
				Triggers: []BuffTrigger{{
					Name: "HP持续", Duration: 6000, On: BATGRTime, TPS: 2,
					Value: 0, Percentage: -5,
				}},
			},
		})
	})

	for ms := int64(100); ms <= 1000; ms += 100 {
		st.Advance(ms)
	}
	e, _ := st.Entity(10)
	// 每次 -5 点（100*5%），两次 -10 → 90
	if e.HP != 90 {
		t.Fatalf("HP = %d, want 90（100 - 5×2）", e.HP)
	}
	t.Logf("施毒术 1 秒：HP 100 → %d", e.HP)
}

// TestTriggerCountLimited trigger 次数受 duration×tps 限制。
//
// 治愈术 duration=6000、tps=2 → 共 12 次。跑 10 秒不该超过 12 次。
func TestTriggerCountLimited(t *testing.T) {
	const w, h = 20, 20
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	st, _ := NewMapState(1, data)

	st.AddEntity(Entity{
		UID: 10, Type: EntityPlayer, Name: "alice", X: 5, Y: 5,
		HP: 1, MaxHP: 10000, ViewRange: 5, Direction: DirDown,
	})

	st.MutateEntity(10, func(e *Entity) error {
		e.Buffs = NewBuffList()
		return e.Buffs.Add(&Buff{
			ID: 1, SeqID: 1, ActiveActs: 2, RemainMS: 60000, // buff 本身很长
			Record: BuffRecord{
				Name: "治愈术", DurationMS: 60000,
				Triggers: []BuffTrigger{{
					Name: "HP持续", Duration: 6000, On: BATGRTime, TPS: 2,
					Value: 1, Percentage: 0,
				}},
			},
		})
	})

	triggers := 0
	for ms := int64(100); ms <= 10000; ms += 100 {
		for _, ev := range st.Advance(ms) {
			if ev.Kind == "buff_trigger" {
				triggers++
			}
		}
	}
	// duration=6000、tps=2 → 12 次
	if triggers > 12 {
		t.Fatalf("触发 %d 次，不该超过 12 次（6000ms × 2tps / 1000）", triggers)
	}
	t.Logf("10 秒内触发 %d 次（上限 12）", triggers)
}
