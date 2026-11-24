package world

import (
	"math"
	"math/rand"
	"testing"
)

func newRand() *rand.Rand { return rand.New(rand.NewSource(20261125)) }

// ---------- 伤害结算 ----------

func TestHitProbabilityZeroDeltaIsHalf(t *testing.T) {
	// 差值 0 时 sigmoid(0)=0.5，命中率 = 0.5/2 + 0.5 = 0.75
	// 【注意】不是 50%！因为公式是 sigmoid(x)/2 + 0.5，x=0 时是 0.75。
	// 这一点很容易想当然，必须用实测锁定。
	got := HitProbability(true, 10, 10)
	want := 0.75
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("命中率(差值0)=%v, want %v", got, want)
	}
}

func TestHitProbabilityMonotonic(t *testing.T) {
	// 命中越高，概率越大；闪避越高，概率越小
	low := HitProbability(true, 0, 10)
	mid := HitProbability(true, 10, 10)
	high := HitProbability(true, 20, 10)
	if !(low < mid && mid < high) {
		t.Fatalf("命中率应随命中单调递增: %v %v %v", low, mid, high)
	}
	// 关键：公式是 sigmoid(x)/2 + 0.5，所以取值区间是 (0.5, 1.0)
	// —— 下限永远是 0.5，不可能接近 0。这是传奇类游戏的「保底命中」设计。
	// 差值 +150 时 sigmoid≈1 -> 概率≈1.0
	if HitProbability(true, 60, 0) < 0.99 {
		t.Fatalf("极高命中应接近 1，实际 %v", HitProbability(true, 60, 0))
	}
	// 差值 -150 时 sigmoid≈0 -> 概率≈0.5（不是 0）
	if p := HitProbability(true, 0, 60); p < 0.49 || p > 0.51 {
		t.Fatalf("极低命中应趋近 0.5 下限，实际 %v", p)
	}
}

func TestResolveDamagePhysicalReduction(t *testing.T) {
	r := newRand()
	stats := CombatStats{AC: AbilityBounds{Min: 5, Max: 5}}
	node := PlainPhyDamage{Damage: 20, DCHit: 999}.ToNode()

	// 命中率几乎为 1（dcHit 远超 dodge），减伤固定 5
	var total int32
	const rounds = 200
	for i := 0; i < rounds; i++ {
		dmg, hit := ResolveDamage(node, stats, r)
		if !hit {
			continue
		}
		if dmg != 15 {
			t.Fatalf("物理伤害应为 20-5=15，实际 %d", dmg)
		}
		total += dmg
	}
	if total == 0 {
		t.Fatal("200 次全部未命中，命中率逻辑有误")
	}
}

func TestResolveDamageNeverNegative(t *testing.T) {
	r := newRand()
	// 减伤大于伤害 -> 必须钳到 0，不能出现负血
	stats := CombatStats{AC: AbilityBounds{Min: 100, Max: 100}}
	node := PlainPhyDamage{Damage: 3, DCHit: 999}.ToNode()
	for i := 0; i < 50; i++ {
		dmg, hit := ResolveDamage(node, stats, r)
		if hit && dmg != 0 {
			t.Fatalf("减伤超伤害时应为 0，实际 %d", dmg)
		}
	}
}

func TestResolveDamageInvalidNode(t *testing.T) {
	r := newRand()
	// magicID == 0 表示无效伤害，必须直接返回 false
	dmg, hit := ResolveDamage(DamageNode{}, CombatStats{}, r)
	if hit || dmg != 0 {
		t.Fatalf("无效伤害应返回 (0,false)，实际 (%d,%v)", dmg, hit)
	}
}

func TestResolveDamageMagicElementRatio(t *testing.T) {
	// 元素抗性 +5 -> elemRatio = 1 + 0.1*5 = 1.5，减伤放大 1.5 倍
	r := newRand()
	stats := CombatStats{
		MAC:  AbilityBounds{Min: 10, Max: 10},
		Elem: ElementResist{Fire: 5},
	}
	node := MagicDamage{MagicID: 100, Damage: 100, MCHit: 999}.ToNode()
	// 注入元素解析：magicID 100 视为火
	SetMagicElementResolver(func(id int) Element {
		if id == 100 {
			return ElemFire
		}
		return ElemNone
	})
	defer SetMagicElementResolver(nil)

	for i := 0; i < 50; i++ {
		dmg, hit := ResolveDamage(node, stats, r)
		if !hit {
			continue
		}
		// 100 - round(10*1.5) = 85
		if dmg != 85 {
			t.Fatalf("火抗 5 时伤害应为 85，实际 %d", dmg)
		}
	}

	// 负抗性 -> elemRatio < 1，减伤变小，伤害变高
	stats.Elem = ElementResist{Fire: -5}
	for i := 0; i < 50; i++ {
		dmg, hit := ResolveDamage(node, stats, r)
		if !hit {
			continue
		}
		// 100 - round(10*0.5) = 95
		if dmg != 95 {
			t.Fatalf("火抗 -5 时伤害应为 95，实际 %d", dmg)
		}
	}
}

func TestRollAttackDamageRanges(t *testing.T) {
	r := newRand()
	dc := AbilityBounds{Min: 3, Max: 7}
	seen := map[int32]bool{}
	for i := 0; i < 500; i++ {
		node := RollAttackDamage(MagicIDPlainAttack, dc, AbilityBounds{}, 10, 0, 0, r)
		if !node.IsPhysical() {
			t.Fatal("物理攻击 magicID 应等于 MagicIDPlainAttack")
		}
		if node.Damage < 3 || node.Damage > 7 {
			t.Fatalf("伤害应在 [3,7]，实际 %d", node.Damage)
		}
		if node.DCHit != 10 {
			t.Fatalf("dcHit 应为 10")
		}
		seen[node.Damage] = true
	}
	if len(seen) < 3 {
		t.Fatalf("500 次应覆盖多个取值，实际只有 %d 种", len(seen))
	}
}

func TestRollAttackDamageDegenerate(t *testing.T) {
	r := newRand()
	// Min == Max 时必须稳定返回该值
	node := RollAttackDamage(MagicIDPlainAttack, AbilityBounds{Min: 5, Max: 5}, AbilityBounds{}, 0, 0, 0, r)
	if node.Damage != 5 {
		t.Fatalf("固定伤害应为 5，实际 %d", node.Damage)
	}
}

// ---------- Buff ----------

func TestBuffSeqLayout(t *testing.T) {
	b := &Buff{ID: 0x1234, SeqID: 0x5678}
	want := uint64(0x1234)<<32 | 0x5678
	if b.BuffSeq() != want {
		t.Fatalf("BuffSeq 拼接错误: %x want %x", b.BuffSeq(), want)
	}
	if b.FromBuff() != 0 {
		t.Fatal("FromBuffSeq 为 0 时 FromBuff 应为 0")
	}
	b.FromBuffSeq = uint64(0x99)<<32 | 1
	if b.FromBuff() != 0x99 {
		t.Fatalf("FromBuff 应取高 32 位，实际 %x", b.FromBuff())
	}
}

func TestBuffListRollSeqID(t *testing.T) {
	l := NewBuffList()
	// 第一次必须是 1，之后单调递增，且不同 ID 互不影响
	if got := l.RollSeqID(10); got != 1 {
		t.Fatalf("首次 seq 应为 1，实际 %d", got)
	}
	if got := l.RollSeqID(10); got != 2 {
		t.Fatalf("第二次 seq 应为 2，实际 %d", got)
	}
	if got := l.RollSeqID(20); got != 1 {
		t.Fatalf("另一个 buffID 应重新从 1 开始，实际 %d", got)
	}
}

func TestBuffListAddDuplicate(t *testing.T) {
	l := NewBuffList()
	b := &Buff{ID: 1, SeqID: 1, ActiveActs: 1}
	if err := l.Add(b); err != nil {
		t.Fatalf("首次添加不应报错: %v", err)
	}
	dup := &Buff{ID: 1, SeqID: 1, ActiveActs: 1}
	if err := l.Add(dup); err == nil {
		t.Fatal("重复 buffSeq 应报错")
	}
}

func TestBuffListSortedBySeq(t *testing.T) {
	l := NewBuffList()
	// 乱序插入，遍历必须按 buffSeq 升序
	l.Add(&Buff{ID: 5, SeqID: 1, ActiveActs: 1})
	l.Add(&Buff{ID: 2, SeqID: 1, ActiveActs: 1})
	l.Add(&Buff{ID: 9, SeqID: 1, ActiveActs: 1})
	act := l.Active()
	for i := 1; i < len(act); i++ {
		if act[i-1].BuffSeq() > act[i].BuffSeq() {
			t.Fatal("遍历顺序必须按 buffSeq 升序（C++ 是 std::map）")
		}
	}
}

func TestBuffTickExpiry(t *testing.T) {
	l := NewBuffList()
	b := &Buff{ID: 1, SeqID: 1, RemainMS: 1000, ActiveActs: 1}
	l.Add(b)
	if exp := l.Tick(500); len(exp) != 0 {
		t.Fatal("未到期不应过期")
	}
	if b.RemainMS != 500 {
		t.Fatalf("剩余时间应递减到 500，实际 %d", b.RemainMS)
	}
	exp := l.Tick(600)
	if len(exp) != 1 || exp[0] != b.BuffSeq() {
		t.Fatalf("到期应返回 buffSeq，实际 %v", exp)
	}
	if l.Len() != 0 {
		t.Fatal("到期后应从列表移除")
	}
}

func TestBuffTickUnlimited(t *testing.T) {
	l := NewBuffList()
	b := &Buff{ID: 1, SeqID: 1, RemainMS: BadurUnlimited, ActiveActs: 1}
	l.Add(b)
	// 永久 buff 无论过多久都不该过期
	if exp := l.Tick(1 << 40); len(exp) != 0 {
		t.Fatal("永久 buff 不应过期")
	}
	if l.Len() != 1 {
		t.Fatal("永久 buff 应仍在列表")
	}
}

func TestBuffStackRule(t *testing.T) {
	// stackCount = 0 表示同名 buff 最多 1 个
	if CanStack(BuffRecord{StackCount: 0}, 0) != true {
		t.Fatal("空列表时应可添加")
	}
	if CanStack(BuffRecord{StackCount: 0}, 1) != false {
		t.Fatal("stackCount=0 时已有 1 个就不能再加")
	}
	// stackCount = 3 表示最多 3 个
	if CanStack(BuffRecord{StackCount: 3}, 2) != true {
		t.Fatal("stackCount=3 时第 3 个可加")
	}
	if CanStack(BuffRecord{StackCount: 3}, 3) != false {
		t.Fatal("stackCount=3 时第 4 个不可加")
	}
}

func TestBuffListRemoveBySource(t *testing.T) {
	l := NewBuffList()
	l.Add(&Buff{ID: 1, SeqID: 1, FromUID: 100, ActiveActs: 1})
	l.Add(&Buff{ID: 2, SeqID: 1, FromUID: 100, ActiveActs: 1})
	l.Add(&Buff{ID: 3, SeqID: 1, FromUID: 200, ActiveActs: 1})
	if n := l.RemoveBySource(100); n != 2 {
		t.Fatalf("应移除 2 个，实际 %d", n)
	}
	if l.Len() != 1 {
		t.Fatalf("应剩 1 个，实际 %d", l.Len())
	}
}

// ---------- 安全区 ----------

func TestSafeZoneRect(t *testing.T) {
	z := SafeZone{Rects: [][4]int{{10, 20, 5, 4}}} // x=10..14, y=20..23
	// 左闭右开：右下角 (14,23) 在范围内，(15,23) 不在
	if !z.Contains(10, 20) {
		t.Fatal("左上角应在范围内")
	}
	if !z.Contains(14, 23) {
		t.Fatal("右下内角应在范围内")
	}
	if z.Contains(15, 23) {
		t.Fatal("x 超出右边界")
	}
	if z.Contains(10, 24) {
		t.Fatal("y 超出下边界")
	}
	if z.Contains(9, 20) {
		t.Fatal("x 小于左边界")
	}
}

func TestSafeZoneCircle(t *testing.T) {
	z := SafeZone{Circles: [][3]int{{0, 0, 5}}}
	if !z.Contains(0, 0) {
		t.Fatal("圆心在范围内")
	}
	if !z.Contains(3, 4) {
		t.Fatal("3-4-5 正好在圆上，应算包含（<=）")
	}
	if z.Contains(4, 4) {
		t.Fatal("4-4 距离约 5.66，超出半径")
	}
}

func TestSafeZoneEmptyAndMerge(t *testing.T) {
	var z SafeZone
	if !z.Empty() {
		t.Fatal("零值应为空")
	}
	other := SafeZone{Rects: [][4]int{{0, 0, 2, 2}}}
	z.Merge(other)
	if z.Empty() {
		t.Fatal("合并后不应为空")
	}
	if !z.Contains(1, 1) {
		t.Fatal("合并区域应可命中")
	}
}

// ---------- 仇恨列表 ----------

func TestOffenderAddAndAccumulate(t *testing.T) {
	var o OffenderList
	o.AddDamage(100, 10, 1000)
	o.AddDamage(100, 15, 2000)
	if o.DamageOf(100) != 25 {
		t.Fatalf("同来源应累加伤害，实际 %d", o.DamageOf(100))
	}
	// activeTime 必须刷新成最新
	snap := o.Snapshot()
	if len(snap) != 1 || snap[0].ActiveTime != 2000 {
		t.Fatalf("activeTime 应被刷新为 2000，实际 %v", snap)
	}
}

func TestOffenderZeroUIDIgnored(t *testing.T) {
	var o OffenderList
	o.AddDamage(0, 100, 1)
	if o.Len() != 0 {
		t.Fatal("uid=0 不应入表（原版 fflassert(nUID)）")
	}
}

func TestOffenderTopAndPrune(t *testing.T) {
	var o OffenderList
	o.AddDamage(1, 50, 100)
	o.AddDamage(2, 200, 100)
	o.AddDamage(3, 30, 100)
	top, ok := o.TopOffender()
	if !ok || top.UID != 2 {
		t.Fatalf("仇恨最高应是 uid=2，实际 %v", top)
	}
	// 时间推进到 200，ttl=50 -> 记录 (activeTime=100) 已过期
	if n := o.Prune(200, 50); n != 3 {
		t.Fatalf("3 条都应过期，实际 %d", n)
	}
	if o.Len() != 0 {
		t.Fatal("清理后应为空")
	}
}

func TestOffenderIsAndRemove(t *testing.T) {
	var o OffenderList
	o.AddDamage(7, 1, 10)
	if !o.IsOffender(7) {
		t.Fatal("应识别为仇恨来源")
	}
	if !o.RemoveOffender(7) {
		t.Fatal("移除应成功")
	}
	if o.IsOffender(7) {
		t.Fatal("移除后不应再是仇恨来源")
	}
	if o.RemoveOffender(7) {
		t.Fatal("重复移除应返回 false")
	}
}

func TestOffenderSnapshotSorted(t *testing.T) {
	var o OffenderList
	o.AddDamage(1, 10, 1)
	o.AddDamage(2, 90, 1)
	o.AddDamage(3, 40, 1)
	snap := o.Snapshot()
	if snap[0].UID != 2 || snap[1].UID != 3 || snap[2].UID != 1 {
		t.Fatalf("快照应按伤害降序，实际 %v", snap)
	}
}
