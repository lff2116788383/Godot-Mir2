package world

import (
	"math"
	"math/rand"
	"sort"
)

// BuffRecord 是随 buff ID 变化的静态配置。
//
// 【依据】common/src/buffrecord.hpp:198-240：
//
//	name          buff 名（用于 aura 互查）
//	favor         -1 = debuff, 0 = 中性, 1 = buff（头顶图标边框色）
//	dispellable   能否被驱散
//	stackCount    0 = 同名 buff 不能叠加（最多 1 个）
//	               N = 最多 N 个同名 buff 共存
//	stackReplace  堆叠满时的替换策略
//	iconShow/iconGfxID  头顶图标
//
// 本工程先用一份最小可运行配置，B5 接入 buffrecord.inc 后替换为查表。
type BuffRecord struct {
	Name         string
	Favor        int
	Dispellable  bool
	StackCount   int
	StackReplace int
	IconShow     bool
	IconGfxID    uint32
	// DurationMS 是 buff 的基础持续时间，BADUR_UNLIMITED 表示永久
	DurationMS int64

	// Acts 属性修正列表（buffrecord.hpp 的 actList 里 attributeModifier 那部分）
	//
	// 【依据】buffrecord.hpp:248-256 BuffActAttributeModifierParam
	//
	//	buff 的实际效果不是「加一个数字」而是「按属性名 + 百分比修正」。
	//	同一个 buff 可能同时改 HP/AC/MAC 好几项（见 buffrecord.inc 的「HP」act）。
	Acts []BuffAttributeModifier

	// Triggers 定时触发列表（buffrecord.hpp:237-245 的 trigger）
	//
	// 用途：治愈术的「HP持续」每 500ms 回一次血、施毒术每 500ms 掉一次血。
	// 没有这一项，治愈术就只是「上限涨 20% 但一点血都不回」。
	Triggers []BuffTrigger

	// AttackMods 攻击时概率触发的修正（buffrecord.hpp:246-250）
	AttackMods []AttackModifier

	// Auras 光环列表（buffrecord.hpp:220-231 的 aura）
	//
	// 用途：龙纹圣光/恢复光环 —— 半径内所有友方获得 buff。
	// 没有这一项，「光环」就只是个图标，范围内的人毫无感觉。
	Auras []BuffAura
}

// BuffAura 是一条光环（buffrecord.hpp:220-231）。
//
// 【依据】
//
//	.aura { .radius = 10, .linger { .in = 500, .out = 500 } }
//
//	radius     生效半径（格）
//	lingerIn   进入光环后延迟多久生效（毫秒）—— 防止「一闪而过也吃到 buff」
//	lingerOut  离开光环后延迟多久失效（毫秒）—— 防止「走一步就掉 buff」
type BuffAura struct {
	Name      string
	Radius    int
	LingerIn  int64
	LingerOut int64
}

// BuffTrigger 是一条定时触发（buffrecord.hpp:237-245）。
//
// 【依据】buffacttrigger.cpp:9-43：
//
//	on = BATGR_TIME 时按 tps 频率触发，共 duration*tps/1000 次
//	每次触发执行 arg（BuffValuePercentage）
type BuffTrigger struct {
	// Name act 名（"HP持续" / "MP持续" / "HP恢复" ...）
	Name string
	// Duration 持续时长（毫秒）。BADUR_UNLIMITED 表示无限制
	Duration int64
	// On 触发时机（BATGR_TIME / BATGR_ATTACK / BATGR_HITTED / BATGR_MOVE）
	On string
	// TPS 每秒触发次数（tps=2 表示每 500ms 一次）
	TPS int
	// Value / Percentage 每次触发的量（BuffValuePercentage 的两段）
	Value      int
	Percentage int
}

// TriggerIntervalMS 返回两次触发之间的间隔（毫秒）。
//
// tps<=0 时返回 0 表示「不按时间触发」——
// 不能让调用方拿到 0 去做除法。
func (t BuffTrigger) TriggerIntervalMS() int64 {
	if t.TPS <= 0 {
		return 0
	}
	return 1000 / int64(t.TPS)
}

// TriggerCount 返回这条 trigger 总共触发几次（-1 表示无限制）。
func (t BuffTrigger) TriggerCount() int {
	if t.Duration == BadurUnlimited {
		return -1
	}
	if t.Duration <= 0 || t.TPS <= 0 {
		return 0
	}
	return int(t.Duration * int64(t.TPS) / 1000)
}

// BATGR_* 触发时机（buffrecord.hpp:62-71）。
const (
	BATGRTime   = "BATGR_TIME"
	BATGRMove   = "BATGR_MOVE"
	BATGRAttack = "BATGR_ATTACK"
	BATGRHitted = "BATGR_HITTED"
)

// BuffAttributeModifier 是一条属性修正（对应 buffrecord.hpp:255 attributeModifier）。
//
// 【依据】buffrecord.hpp:34-43 BuffValuePercentage：
//
//	int gain(int base) const {
//	    return value + lround(base * percentage / 100.0);
//	}
//
// 也就是「固定值 + 百分比」两段叠加。
type BuffAttributeModifier struct {
	// Attr 属性名（"HP" / "MP" / "AC下限" / "AC上限" / "MAC下限" / "MAC上限" ...）
	Attr string
	// Value 固定加成
	Value int
	// Percentage 百分比加成（20 表示 +20%）
	Percentage int
}

// Gain 是按 base 算出的加成量（buffrecord.hpp:39-42 的 gain）。
//
// 用四舍五入而不是截断：截断会让「+15% 的 33 点」变成 +4，
// 而正确值是 +5。百分比 buff 在低属性时误差尤其明显。
func (m BuffAttributeModifier) Gain(base int) int {
	return m.Value + int(math.Round(float64(base)*float64(m.Percentage)/100.0))
}

// AttrHP / AttrMP 等是属性名常量。
//
// 直接用中文字符串而不用枚举：buffrecord.inc 里写的就是中文，
// 换成枚举要维护一张双向映射表，改一个名字要动两处。
const (
	AttrHP     = "HP"
	AttrMP     = "MP"
	AttrACMin  = "AC下限"
	AttrACMax  = "AC上限"
	AttrMACMin = "MAC下限"
	AttrMACMax = "MAC上限"
	AttrHPCure = "HP恢复"
	AttrMPCure = "MP恢复"
)

// BadurUnlimited 对应 buffrecord.hpp 的 BADUR_UNLIMITED。
const BadurUnlimited int64 = -1

// Buff 是一个挂在实体上的 buff 实例。
//
// 【依据】server/src/buff.hpp:19-40：
//
//	base 类持有 bo / fromUID / fromBuffSeq / id / seqID
//	buffSeq() = (id << 32) | seqID —— 这个拼法是 BuffList 的排序键
//	done() 判断 activeActList 是否为空
type Buff struct {
	// 来源
	FromUID     uint64
	FromBuffSeq uint64

	// 身份：ID + SeqID 唯一确定一个 buff 实例
	ID    uint32
	SeqID uint32

	// 剩余时间（毫秒）；BadurUnlimited 表示永久
	RemainMS int64

	// 由哪个 act 触发（对应 activeActList 的简化表达）
	ActiveActs int

	// TriggerRemainMS 距离下一次 trigger 触发的剩余毫秒
	//
	// 单独一个字段而不是复用 RemainMS：RemainMS 管的是「buff 整体还剩多久」，
	// TriggerRemainMS 管「下次回血还有多久」。治愈术的 trigger 是
	// 500ms 一次、buff 本身 6000ms，两者不同频。
	TriggerRemainMS int64
	// TriggeredCount 已触发次数（用于 TriggerCount 上限判定）
	TriggeredCount int

	// 记录（若上层注入了查表函数则从中取）
	Record BuffRecord
}

// BuffSeq 对应 buff.hpp 的 buffSeq()。
// 注意是 uint64：高 32 位 id、低 32 位 seqID。
func (b *Buff) BuffSeq() uint64 {
	return uint64(b.ID)<<32 | uint64(b.SeqID)
}

// FromBuff 对应 buff.hpp 的 fromBuff()，取来源 buff 的 ID。
func (b *Buff) FromBuff() uint32 { return uint32(b.FromBuffSeq >> 32) }

// Done 对应 buff.hpp 的 done()：没有活跃 act 即视为完成。
func (b *Buff) Done() bool { return b.ActiveActs <= 0 }

// BuffResolver 由上层注入，按 buffID 取静态配置。
var buffResolver func(buffID uint32) (BuffRecord, bool)

// SetBuffResolver 注入 buff 配置查表（测试或 B5 用）。
func SetBuffResolver(fn func(uint32) (BuffRecord, bool)) {
	buffResolver = fn
}

// BuffList 管理一个实体上的所有 buff。
//
// 【依据】server/src/bufflist.hpp:13-60：
//
//	m_buffSeqBase[buffID] 自增计数器，rollSeqID 保证同 ID 的 seq 单调递增且从 1 开始
//	m_activeBuffList 按 buffSeq 排序（C++ 用 std::map<uint64_t,...>）
//	addBuff 时断言 buffSeq 不重复
//
// 这里用切片 + 排序保持与 C++ std::map 一致的遍历顺序（按 buffSeq 升序），
// 避免依赖 Go map 的随机遍历顺序 —— 那是计划里明令禁止的。
type BuffList struct {
	seqBase map[uint32]uint32
	active  []*Buff
}

// NewBuffList 创建空 buff 列表。
func NewBuffList() *BuffList {
	return &BuffList{seqBase: make(map[uint32]uint32)}
}

// RollSeqID 对应 bufflist.hpp:33-36。
//
//	m_buffSeqBase[id] = max(1, m_buffSeqBase[id] + 1)
//
// 即第一次分配得到 1，之后每次 +1，永不回绕到 0。
func (l *BuffList) RollSeqID(buffID uint32) uint32 {
	next := l.seqBase[buffID] + 1
	if next < 1 {
		next = 1
	}
	l.seqBase[buffID] = next
	return next
}

// Add 对应 bufflist.hpp:38-47 addBuff。
// 返回错误当 buffSeq 已存在（C++ 是 fflassert，这里改成 error 以符合 Go 风格）。
func (l *BuffList) Add(buff *Buff) error {
	if buff == nil {
		return ErrNilBuff
	}
	seq := buff.BuffSeq()
	for _, existing := range l.active {
		if existing.BuffSeq() == seq {
			return ErrBuffExists
		}
	}
	if buff.Record.Name == "" && buffResolver != nil {
		if rec, ok := buffResolver(buff.ID); ok {
			buff.Record = rec
		}
	}
	if buff.RemainMS == 0 {
		buff.RemainMS = buff.Record.DurationMS
	}
	if buff.ActiveActs == 0 {
		buff.ActiveActs = 1
	}
	l.active = append(l.active, buff)
	sort.SliceStable(l.active, func(i, j int) bool {
		return l.active[i].BuffSeq() < l.active[j].BuffSeq()
	})
	return nil
}

// Has 按 buffSeq 查找。
func (l *BuffList) Has(buffSeq uint64) (*Buff, bool) {
	for _, b := range l.active {
		if b.BuffSeq() == buffSeq {
			return b, true
		}
	}
	return nil, false
}

// HasID 判断是否已挂着某 ID 的 buff。
func (l *BuffList) HasID(buffID uint32) (*Buff, bool) {
	for _, b := range l.active {
		if b.ID == buffID {
			return b, true
		}
	}
	return nil, false
}

// CountID 统计同 ID buff 数量（用于 stackCount 判定）。
func (l *BuffList) CountID(buffID uint32) int {
	n := 0
	for _, b := range l.active {
		if b.ID == buffID {
			n++
		}
	}
	return n
}

// Remove 移除指定 buffSeq，返回是否命中。
func (l *BuffList) Remove(buffSeq uint64) bool {
	for i, b := range l.active {
		if b.BuffSeq() == buffSeq {
			l.active = append(l.active[:i], l.active[i+1:]...)
			return true
		}
	}
	return false
}

// RemoveID 移除所有同 ID 的 buff，返回移除数量。
func (l *BuffList) RemoveID(buffID uint32) int {
	n := 0
	kept := l.active[:0]
	for _, b := range l.active {
		if b.ID == buffID {
			n++
			continue
		}
		kept = append(kept, b)
	}
	l.active = kept
	return n
}

// RemoveBySource 移除由某来源实体施加的所有 buff（对应 removeFromBuff）。
func (l *BuffList) RemoveBySource(fromUID uint64) int {
	n := 0
	kept := l.active[:0]
	for _, b := range l.active {
		if b.FromUID == fromUID {
			n++
			continue
		}
		kept = append(kept, b)
	}
	l.active = kept
	return n
}

// Tick 推进所有 buff 的剩余时间，返回本次到期的 buffSeq 列表。
//
// 对应 BaseBuff 的 act 到期逻辑：duration 归零即视为该 act 完成，
// 所有 act 都完成时整个 buff 被移除（buff.hpp 的 done()）。
// 这里把「每层 act」简化成 ActiveActs 计数：每次到期减 1，减到 0 就移除。
func (l *BuffList) Tick(deltaMS int64) []uint64 {
	var expired []uint64
	kept := l.active[:0]
	for _, b := range l.active {
		if b.RemainMS == BadurUnlimited {
			kept = append(kept, b)
			continue
		}
		b.RemainMS -= deltaMS
		if b.RemainMS > 0 {
			kept = append(kept, b)
			continue
		}
		b.ActiveActs--
		if b.Done() {
			expired = append(expired, b.BuffSeq())
			continue // 不进 kept，即移除
		}
		b.RemainMS = b.Record.DurationMS // 进入下一层 act
		kept = append(kept, b)
	}
	l.active = kept
	return expired
}

// Active 返回按 buffSeq 升序的快照（遍历安全，调用方可随意增删）。
// 对应 bufflist.hpp:55-64 的 buffSeqList()。
func (l *BuffList) Active() []*Buff {
	out := make([]*Buff, len(l.active))
	copy(out, l.active)
	return out
}

// ActiveIDs 返回当前所有 buff 的 ID 列表（用于 SM_BUFFIDLIST 下发）。
func (l *BuffList) ActiveIDs() []uint32 {
	out := make([]uint32, 0, len(l.active))
	for _, b := range l.active {
		out = append(out, b.ID)
	}
	return out
}

// Len 返回活跃 buff 数。
func (l *BuffList) Len() int { return len(l.active) }

// CanStack 判断某 ID 的 buff 还能否再叠加一个。
//
// 【依据】buffrecord.hpp:205 stackCount：
//
//	0 表示同名 buff 不能叠加（最多 1 个）
//	N 表示最多 N 个
func CanStack(rec BuffRecord, current int) bool {
	limit := rec.StackCount
	if limit <= 0 {
		limit = 1
	}
	return current < limit
}

// ApplyAttributeModifiers 把实体身上所有 buff 的属性修正叠加到 stats 上。
//
// 【依据】server/src/buffactattributemodifier.cpp 的 onActivate：
// 在原属性上按 gain(base) 叠加，多個 buff 是**累加**而不是覆盖。
//
// ★ 百分比按「叠加前的原始值」算还是「逐层放大」？
//
//	原版是**逐层作用在已经改过的值上**（buff 按 buffSeq 顺序依次 apply）。
//	这里保持一致 —— 否则「+20% 和 +20%」会算成 +40%，
//	而原版实际是 ×1.44（1.2×1.2）。两层 buff 差别不大，
//	但五六层叠加时差异会非常明显，且与原版不一致。
//
// ★ HP/MP 的修正只改**上限**，不改当前值：
//
//	改当前值会让「上 buff 瞬间回满血」，这是 exploit（反复上/下 buff 回血）。
//	当前值超出新上限时由调用方钳制。
func ApplyAttributeModifiers(stats *CombatStats, buffs *BuffList) {
	if stats == nil || buffs == nil {
		return
	}
	for _, buff := range buffs.Active() {
		for _, act := range buff.Record.Acts {
			applyOne(stats, act)
		}
	}
}

// applyOne 应用一条属性修正。
func applyOne(stats *CombatStats, act BuffAttributeModifier) {
	switch act.Attr {
	case AttrACMin:
		stats.AC.Min += act.Gain(stats.AC.Min)
	case AttrACMax:
		stats.AC.Max += act.Gain(stats.AC.Max)
	case AttrMACMin:
		stats.MAC.Min += act.Gain(stats.MAC.Min)
	case AttrMACMax:
		stats.MAC.Max += act.Gain(stats.MAC.Max)
	case "DC下限":
		stats.DC.Min += act.Gain(stats.DC.Min)
	case "DC上限":
		stats.DC.Max += act.Gain(stats.DC.Max)
	case "MC下限":
		stats.MC.Min += act.Gain(stats.MC.Min)
	case "MC上限":
		stats.MC.Max += act.Gain(stats.MC.Max)
	case "SC下限":
		stats.SC.Min += act.Gain(stats.SC.Min)
	case "SC上限":
		stats.SC.Max += act.Gain(stats.SC.Max)
	}
}

// MaxHPDelta 返回 buff 对 HP 上限的总修正量（供 Entity 血量钳制用）。
func MaxHPDelta(base int32, buffs *BuffList) int32 {
	if buffs == nil {
		return 0
	}
	cur := int(base)
	for _, buff := range buffs.Active() {
		for _, act := range buff.Record.Acts {
			if act.Attr == AttrHP {
				cur += act.Gain(cur)
			}
		}
	}
	return int32(cur - int(base))
}

// MaxMPDelta 返回 buff 对 MP 上限的总修正量。
func MaxMPDelta(base int32, buffs *BuffList) int32 {
	if buffs == nil {
		return 0
	}
	cur := int(base)
	for _, buff := range buffs.Active() {
		for _, act := range buff.Record.Acts {
			if act.Attr == AttrMP {
				cur += act.Gain(cur)
			}
		}
	}
	return int32(cur - int(base))
}

// TickTriggers 推进所有 buff 的定时触发，返回本 tick 要执行的效果。
//
// 【依据】buffacttrigger.cpp:9-43 的 BATGR_TIME 分支。
//
// 返回值是「这条 trigger 应该作用于谁、生效什么」的列表，
// 由上层转成扣血/回血/回蓝，world 层不直接改 HP（HP 属于 BattleObject）。
//
// ★ 为什么不在 Tick 里直接改 HP：
//
//	BuffList 挂在 Entity 上，但 Entity 的 HP 变更要经过
//	「死亡判定 + 广播 + 仇恨处理」一整条链路。
//	让 buff 直接改 HP 会绕过这些，出现「毒死了但不掉经验」。
type BuffTriggerEffect struct {
	// OwnerUID 受影响实体
	OwnerUID uint64
	// BuffID / BuffSeq 来源 buff
	BuffID  uint32
	BuffSeq uint64
	// Name act 名（"HP持续" / "MP持续"）
	Name string
	// Amount 变动量（正数回、负数掉）
	Amount int32
	// IsMP 作用于 MP（否则作用于 HP）
	IsMP bool
}

// TickTriggers 推进 trigger 计时并返回本 tick 的效果列表。
//
// 按 deltaMS 累进，而不是「每 tick 触发一次」：
// tick 是 100ms、trigger 是 500ms，必须攒够 500ms 才触发一次，
// 否则治愈术会变成「每 100ms 回一次」——10 倍速度。
func (l *BuffList) TickTriggers(ownerUID uint64, deltaMS int64) []BuffTriggerEffect {
	if l == nil || deltaMS <= 0 {
		return nil
	}
	var out []BuffTriggerEffect
	for _, b := range l.active {
		for i := range b.Record.Triggers {
			trig := &b.Record.Triggers[i]
			if trig.On != BATGRTime {
				continue // 只处理按时间触发的
			}
			interval := trig.TriggerIntervalMS()
			if interval <= 0 {
				continue
			}
			total := trig.TriggerCount()
			if total == 0 {
				continue
			}

			// ★ 首次触发要等满一个间隔，不能「一挂上就触发」。
			//
			//	TriggerRemainMS 初值 0 会让第一个 tick 就命中，
			//	于是「duration 6000 / tps 2」实际触发 13 次而不是 12 次，
			//	而且治愈术挂上瞬间立刻回一次血 —— 玩家会觉得回血快了一倍。
			if b.TriggerRemainMS <= 0 && b.TriggeredCount == 0 {
				b.TriggerRemainMS = interval
			}
			b.TriggerRemainMS -= deltaMS
			// 可能一个 tick 跨过多个触发点（tick 200ms、interval 100ms）
			for b.TriggerRemainMS <= 0 {
				if total > 0 && b.TriggeredCount >= total {
					break
				}
				b.TriggeredCount++
				b.TriggerRemainMS += interval

				isMP := trig.Name == "MP持续" || trig.Name == "MP恢复"
				// 每次的变动量按「当前上限」算百分比部分；
				// 这里拿不到上限，交给上层按实际 MaxHP/MaxMP 结算。
				out = append(out, BuffTriggerEffect{
					OwnerUID: ownerUID,
					BuffID:   b.ID,
					BuffSeq:  b.BuffSeq(),
					Name:     trig.Name,
					// Amount 先存「固定值」，百分比由上层补
					Amount: int32(trig.Value),
					IsMP:   isMP,
				})
			}
		}
	}
	return out
}

// PendingTriggerPercentage 返回某条目 trigger 的百分比部分。
//
// 上层拿到 BuffTriggerEffect 后，按实体的 MaxHP/MaxMP 算百分比量，
// 与 Amount（固定值）相加得到最终变动量。
func PendingTriggerPercentage(buffs *BuffList, buffSeq uint64, name string) int {
	if buffs == nil {
		return 0
	}
	for _, b := range buffs.active {
		if b.BuffSeq() != buffSeq {
			continue
		}
		for _, t := range b.Record.Triggers {
			if t.Name == name {
				return t.Percentage
			}
		}
	}
	return 0
}

// AuraCandidate 是光环范围内的一个候选目标。
type AuraCandidate struct {
	UID uint64
	X   int
	Y   int
}

// AuraTargets 返回某条光环覆盖到的目标。
//
// 【依据】buffactaura.cpp 的 onActivate：按 radius 扫描周围格，
// 对每个范围内的友方挂上「使能」类 buff。
//
// ★ linger（进出延迟）由上层实现：
//
//	world 层只负责「这一帧谁在范围内」，延迟逻辑需要跨 tick 记状态，
//	放在地图 actor 的 AuraState 里更合适（这里是纯函数，好测）。
func AuraTargets(cx, cy int, aura BuffAura, candidates []AuraCandidate) []uint64 {
	if aura.Radius <= 0 || len(candidates) == 0 {
		return nil
	}
	var out []uint64
	for _, c := range candidates {
		dx := c.X - cx
		if dx < 0 {
			dx = -dx
		}
		dy := c.Y - cy
		if dy < 0 {
			dy = -dy
		}
		d := dx
		if dy > d {
			d = dy
		}
		if d <= aura.Radius {
			out = append(out, c.UID)
		}
	}
	return out
}

// HasAura 判断 buff 是否带某条光环。
func (r BuffRecord) HasAura(name string) bool {
	for _, a := range r.Auras {
		if a.Name == name {
			return true
		}
	}
	return false
}

// ==================== attackModifier（攻击概率修正）====================

// AttackModifier 是一条攻击时概率触发的效果（buffrecord.hpp:246-250）。
//
// 【依据】
//
//	.attackModifier { .prob = 100 }
//
// prob 是「万分比」概率（100 = 1%）。原版用它实现「死亡威慑」这类
// 攻击时附带额外效果（恐惧/吸血）的 buff。
type AttackModifier struct {
	Name string
	Prob int
}

// RollAttackModifiers 在攻击时掷骰，返回本次触发的 modifier 名列表。
//
// ★ prob 是万分比而不是百分比：
//
//	buffrecord.inc 里「战栗使能」写的是 prob=100，
//	如果当成 100% 那就是每次都触发，实际上它只是 1% 概率。
func RollAttackModifiers(rec BuffRecord, r *rand.Rand) []string {
	var out []string
	if r == nil || len(rec.AttackMods) == 0 {
		return nil
	}
	for _, m := range rec.AttackMods {
		if m.Prob <= 0 {
			continue
		}
		if m.Prob >= 10000 || r.Intn(10000) < m.Prob {
			out = append(out, m.Name)
		}
	}
	return out
}
