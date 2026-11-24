package world

import (
	"math"
	"math/rand"
)

// MagicIDPlainAttack mirrors DBCOM_MAGICID(u8"物理攻击")，即魔法表第 1 条。
//
// 【依据】common/src/magicrecord.inc 第 1 条是 u8"物理攻击"，
// 之前客户端复刻时也确认过 magicrecord.json 的 records[0] 就是它。
const MagicIDPlainAttack = 1

// Element 对应 magicrecord 的 elem 枚举（magicElemID）。
type Element uint8

const (
	ElemNone Element = iota
	ElemFire
	ElemIce
	ElemLight
	ElemWind
	ElemHoly
	ElemDark
	ElemPhantom
)

// DamageNode 对应 server/src/damagenode.hpp。
//
// C++ 原文：
//
//	struct DamageNode { int magicID; int damage; int dcHit; int mcHit; int modifierID; }
//	operator bool() { return magicID != 0; }
//
// magicID == 0 表示「无效伤害」，受击方必须直接返回（struckDamage 开头就判 !node）。
type DamageNode struct {
	MagicID    int
	Damage     int32
	DCHit      int
	MCHit      int
	ModifierID int
}

// Valid 对应 C++ 的 operator bool()。
func (n DamageNode) Valid() bool { return n.MagicID != 0 }

// IsPhysical 对应 struckDamage 里的 phyDC 判断。
func (n DamageNode) IsPhysical() bool { return n.MagicID == MagicIDPlainAttack }

// PlainPhyDamage 对应 damagenode.hpp 同名结构，构造普通物理攻击伤害。
type PlainPhyDamage struct {
	Damage     int32
	DCHit      int
	ModifierID int
}

// ToNode 对应 PlainPhyDamage::operator DamageNode()。
// 注意 C++ 用 memset 清零，所以 MCHit 必须是 0。
func (d PlainPhyDamage) ToNode() DamageNode {
	return DamageNode{
		MagicID:    MagicIDPlainAttack,
		Damage:     d.Damage,
		DCHit:      d.DCHit,
		ModifierID: d.ModifierID,
	}
}

// MagicDamage 对应 damagenode.hpp 同名结构。
type MagicDamage struct {
	MagicID    int
	Damage     int32
	MCHit      int
	ModifierID int
}

// ToNode 对应 MagicDamage::operator DamageNode()。
func (d MagicDamage) ToNode() DamageNode {
	return DamageNode{
		MagicID:    d.MagicID,
		Damage:     d.Damage,
		MCHit:      d.MCHit,
		ModifierID: d.ModifierID,
	}
}

// AttackModifierID 对应 magicrecord/attackmodifier 里的 穿刺/吸血 等。
// 目前只用到吸血，其它按 0 处理（不修改）。
const (
	AttackModifierNone = 0
	AttackModifierVamp = 1 // 吸血
)

// AbilityBounds 是一组 [min,max] 区间，对应 C++ 里的 int[2]。
type AbilityBounds struct {
	Min int
	Max int
}

// RandInclusive 对应 mathf::rand<int>(min, max + 1)，即闭区间 [Min,Max]。
func (b AbilityBounds) RandInclusive(r *rand.Rand) int {
	if b.Max <= b.Min {
		return b.Min
	}
	return b.Min + r.Intn(b.Max-b.Min+1)
}

// ElementResist 是怪物的元素抗性表，对应 MonsterRecord::acElem。
type ElementResist struct {
	Fire    int
	Ice     int
	Light   int
	Wind    int
	Holy    int
	Dark    int
	Phantom int
}

// ResistFor 对应 struckDamage 里 switch(magicElemID(mr.elem)) 的分支。
func (e ElementResist) ResistFor(elem Element) int {
	switch elem {
	case ElemFire:
		return e.Fire
	case ElemIce:
		return e.Ice
	case ElemLight:
		return e.Light
	case ElemWind:
		return e.Wind
	case ElemHoly:
		return e.Holy
	case ElemDark:
		return e.Dark
	case ElemPhantom:
		return e.Phantom
	}
	return 0
}

// CombatStats 是受击方用于结算的属性集合。
//
// 【依据】monster.cpp:715-790 struckDamage：
//   - 命中率：sigmoid((dcHit - dcDodge)/2.5)/2 + 0.5
//   - 物理减伤：damage - rand(ac[0], ac[1]+1)，下限 0
//   - 魔法减伤：rand(mac[0],mac[1]+1) * elemRatio，elemRatio = max(0, 1 + 0.1*抗性)
type CombatStats struct {
	DC      AbilityBounds // 攻击方：物理伤害区间
	MC      AbilityBounds // 攻击方：魔法伤害区间
	DCHit   int           // 攻击方：命中
	DCDodge int           // 受击方：闪避
	MCHit   int           // 攻击方：魔法命中
	MCDodge int           // 受击方：魔法闪避
	AC      AbilityBounds // 受击方：物防区间
	MAC     AbilityBounds // 受击方：魔防区间
	Elem    ElementResist // 受击方：元素抗性
}

// Sigmoid 对应 mathf::sigmoid。
//
// 客户端复刻时已核对过 mathf.hpp 的实现是标准 1/(1+e^-x)。
func Sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// HitProbability 对应 struckDamage 里的 hitProb 计算。
//
//	物理：sigmoid((dcHit - dcDodge) / 2.5) / 2.0 + 0.5
//	魔法：sigmoid((mcHit - mcDodge) / 2.5) / 2.0 + 0.5
//
// 注意是除以 2.5 而不是 1.0 —— 这个系数让命中率曲线更平滑。
//
// ★ 取值范围是 (0.5, 1.0)，不是 (0,1)：
//
//	因为外面有个 "+ 0.5"，所以差值 0 时命中率是 0.75 而不是 0.5，
//	差值极负时也只会趋近 0.5（保底命中），永远不可能低到 0。
//	这是传奇类游戏「避免完全打不中」的设计，写测试时极易想当然写错。
func HitProbability(physical bool, attackerHit, targetDodge int) float64 {
	delta := float64(attackerHit - targetDodge)
	return Sigmoid(delta/2.5)/2.0 + 0.5
}

// ResolveDamage 结算一次伤害，返回最终扣血量与是否命中。
//
// 逐行对应 monster.cpp:715-786：
//
//  1. node 无效（magicID==0）直接返回 false,0
//  2. 掷命中：rand(0,1) > hitProb -> 未命中（原版返回 true 表示"处理了"，这里用 hit=false 表达）
//  3. 物理：max(0, damage - rand(ac))
//     魔法：max(0, damage - lround(rand(mac) * elemRatio))
//     elemRatio = max(0, 1 + 0.1 * 抗性)
//
// 返回值：(实际伤害, 是否命中)
func ResolveDamage(node DamageNode, stats CombatStats, r *rand.Rand) (int32, bool) {
	if !node.Valid() {
		return 0, false
	}
	physical := node.IsPhysical()

	var hitProb float64
	if physical {
		hitProb = HitProbability(true, node.DCHit, stats.DCDodge)
	} else {
		hitProb = HitProbability(false, node.MCHit, stats.MCDodge)
	}
	if r.Float64() > hitProb {
		return 0, false
	}

	var finalDamage int32
	if physical {
		reduce := int32(stats.AC.RandInclusive(r))
		finalDamage = node.Damage - reduce
	} else {
		elemRatio := 1.0 + 0.1*float64(stats.Elem.ResistFor(elemOfMagic(node.MagicID)))
		if elemRatio < 0 {
			elemRatio = 0
		}
		reduce := int32(math.Round(float64(stats.MAC.RandInclusive(r)) * elemRatio))
		finalDamage = node.Damage - reduce
	}
	if finalDamage < 0 {
		finalDamage = 0
	}
	return finalDamage, true
}

// elemOfMagic 查魔法元素的钩子。
// 真实实现需要 MagicRecord 表（B5 迁移），先返回 None。
// 【待办】B5 接入 magicrecord 后替换为查表。
var elemOfMagic = func(magicID int) Element {
	return ElemNone
}

// SetMagicElementResolver 允许上层注入魔法元素查询（测试或 B5 用）。
func SetMagicElementResolver(fn func(magicID int) Element) {
	if fn != nil {
		elemOfMagic = fn
	}
}

// RollAttackDamage 对应 monster.cpp:584-607 getAttackDamage。
//
// 物理：rand(min(dc), max(dc)+1) 的随机值 + dcHit
// 魔法：rand(min(mc), max(mc)+1) 的随机值 + mcHit
func RollAttackDamage(magicID int, dc, mc AbilityBounds, dcHit, mcHit, modifierID int, r *rand.Rand) DamageNode {
	if magicID == MagicIDPlainAttack {
		return PlainPhyDamage{
			Damage:     int32(dc.RandInclusive(r)),
			DCHit:      dcHit,
			ModifierID: modifierID,
		}.ToNode()
	}
	return MagicDamage{
		MagicID:    magicID,
		Damage:     int32(mc.RandInclusive(r)),
		MCHit:      mcHit,
		ModifierID: modifierID,
	}.ToNode()
}
