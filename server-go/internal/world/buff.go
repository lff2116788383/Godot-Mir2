package world

import (
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
}

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
