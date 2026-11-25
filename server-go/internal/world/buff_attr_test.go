package world

import "testing"

// TestAttributeModifierPercentage 百分比修正按 gain(base) 公式算。
//
// 【依据】buffrecord.hpp:34-43：
//
//	gain(base) = value + lround(base * percentage / 100)
func TestAttributeModifierPercentage(t *testing.T) {
	cases := []struct {
		mod  BuffAttributeModifier
		base int
		want int
	}{
		{BuffAttributeModifier{Attr: AttrHP, Percentage: 20}, 100, 20},
		{BuffAttributeModifier{Attr: AttrHP, Percentage: -20}, 100, -20},
		{BuffAttributeModifier{Value: 2, Percentage: 5}, 40, 4}, // 2 + 2 = 4
		{BuffAttributeModifier{Percentage: 15}, 33, 5},          // 4.95 → 5（四舍五入）
		{BuffAttributeModifier{Percentage: 5}, 10, 1},           // 0.5 → 1
	}
	for _, c := range cases {
		if got := c.mod.Gain(c.base); got != c.want {
			t.Errorf("%+v on base %d = %d, want %d", c.mod, c.base, got, c.want)
		}
	}
}

// TestApplyAttributeModifiers 幽灵盾的 MAC +2 +5% 真的加到属性上。
func TestApplyAttributeModifiers(t *testing.T) {
	buffs := NewBuffList()
	// 幽灵盾（buffrecord.inc:9）：MAC下限 +2 +5%，MAC上限 +2 +5%
	rec := BuffRecord{
		Name: "幽灵盾", Favor: 1,
		Acts: []BuffAttributeModifier{
			{Attr: AttrMACMin, Value: 2, Percentage: 5},
			{Attr: AttrMACMax, Value: 2, Percentage: 5},
		},
	}
	if err := buffs.Add(&Buff{ID: 9, SeqID: 1, ActiveActs: 2, Record: rec}); err != nil {
		t.Fatalf("add buff: %v", err)
	}

	stats := CombatStats{MAC: AbilityBounds{Min: 40, Max: 60}}
	ApplyAttributeModifiers(&stats, buffs)

	// 40 → 40 + 2 + round(40*5/100)=2 → 44
	// 60 → 60 + 2 + round(60*5/100)=3 → 65
	if stats.MAC.Min != 44 {
		t.Fatalf("MAC.Min = %d, want 44", stats.MAC.Min)
	}
	if stats.MAC.Max != 65 {
		t.Fatalf("MAC.Max = %d, want 65", stats.MAC.Max)
	}
	t.Logf("幽灵盾：MAC 40/60 → %d/%d", stats.MAC.Min, stats.MAC.Max)
}

// TestMultipleBuffsStack 多个 buff 是累乘而不是简单相加。
//
// 两个 +20% 应该是 ×1.44，不是 +40%。
func TestMultipleBuffsStack(t *testing.T) {
	buffs := NewBuffList()
	rec := BuffRecord{
		Name: "治愈术",
		Acts: []BuffAttributeModifier{{Attr: AttrACMax, Percentage: 20}},
	}
	buffs.Add(&Buff{ID: 1, SeqID: 1, ActiveActs: 1, Record: rec})
	buffs.Add(&Buff{ID: 1, SeqID: 2, ActiveActs: 1, Record: rec})

	stats := CombatStats{AC: AbilityBounds{Min: 0, Max: 100}}
	ApplyAttributeModifiers(&stats, buffs)

	// 100 → 120 → 144
	if stats.AC.Max != 144 {
		t.Fatalf("AC.Max = %d, want 144（两个 +20%% 应累乘到 ×1.44）", stats.AC.Max)
	}
	t.Logf("两层 +20%%：AC.Max 100 → %d", stats.AC.Max)
}

// TestDebuffLowersStats 施毒术（HP -20%）是负修正。
func TestDebuffLowersStats(t *testing.T) {
	delta := MaxHPDelta(100, func() *BuffList {
		b := NewBuffList()
		b.Add(&Buff{ID: 2, SeqID: 1, ActiveActs: 1, Record: BuffRecord{
			Name: "施毒术", Favor: -1,
			Acts: []BuffAttributeModifier{{Attr: AttrHP, Percentage: -20}},
		}})
		return b
	}())
	if delta != -20 {
		t.Fatalf("HP 上限修正 = %d, want -20", delta)
	}
}

// TestNoBuffsNoChange 没有 buff 时属性不变。
func TestNoBuffsNoChange(t *testing.T) {
	stats := CombatStats{AC: AbilityBounds{Min: 10, Max: 20}}
	base := stats
	ApplyAttributeModifiers(&stats, nil)
	if stats != base {
		t.Fatalf("无 buff 不该改变属性：%+v vs %+v", stats, base)
	}
}
