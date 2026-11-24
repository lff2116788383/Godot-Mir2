package world

import "sort"

// SafeZone 是一张地图上所有安全区的合并结果。
//
// 【依据】common/src/serdesmsg.hpp:1043-1072 SDSafeZone：
//
//	rectList   矩形区 [x, y, w, h]
//	circleList 圆形区 [x, y, r]
//	contains   二者取「或」—— 落在任一区域即算安全
//
// 原版战斗对象持有它的一份副本（BattleObject::m_safeZone），
// 因为安全区是地图属性，但判定发生在实体侧。
type SafeZone struct {
	Rects   [][4]int // x, y, w, h
	Circles [][3]int // x, y, r
}

// Contains 对应 SDSafeZone::contains。
//
//	矩形：mathf::pointInRectangle(x, y, rx, ry, rw, rh)
//	圆形：mathf::pointInCircle(x, y, cx, cy, r) —— 内部用 int64 平方防溢出
func (z SafeZone) Contains(x, y int) bool {
	for _, r := range z.Rects {
		// pointInRectangle 是左闭右开还是闭区间？
		// 【依据】mathf.hpp pointInRectangle(x,y,rx,ry,rw,rh):
		//   x >= rx && x < rx + rw && y >= ry && y < ry + rh
		if x >= r[0] && x < r[0]+r[2] && y >= r[1] && y < r[1]+r[3] {
			return true
		}
	}
	for _, c := range z.Circles {
		dx := int64(x) - int64(c[0])
		dy := int64(y) - int64(c[1])
		if dx*dx+dy*dy <= int64(c[2])*int64(c[2]) {
			return true
		}
	}
	return false
}

// Empty 对应 SDSafeZone::empty。
func (z SafeZone) Empty() bool { return len(z.Rects) == 0 && len(z.Circles) == 0 }

// Clone 复制一份（战斗对象各自持有副本）。
func (z SafeZone) Clone() SafeZone {
	out := SafeZone{}
	if len(z.Rects) > 0 {
		out.Rects = append(out.Rects, z.Rects...)
	}
	if len(z.Circles) > 0 {
		out.Circles = append(out.Circles, z.Circles...)
	}
	return out
}

// Merge 合并另一个安全区（对应 SDSafeZoneList 的 "all zones merged"）。
func (z *SafeZone) Merge(other SafeZone) {
	z.Rects = append(z.Rects, other.Rects...)
	z.Circles = append(z.Circles, other.Circles...)
}

// Offender 是一条仇恨记录。
//
// 【依据】server/src/battleobject.hpp:66-71：
//
//	struct Offender { uint64_t uid; uint64_t damage; uint64_t activeTime; }
//
// activeTime 是最后一次造成伤害的时刻（秒），用于仇恨衰减。
type Offender struct {
	UID        uint64
	Damage     uint64
	ActiveTime int64
}

// OffenderList 是仇恨列表。
//
// 【依据】battleobject.cpp:940-978：
//
//	isOffender     线性查找
//	removeOffender 线性删除首个匹配
//	addOffenderDamage 命中则累加 damage 并刷新 activeTime，否则追加
//
// C++ 用 std::vector 线性扫描，这里保持一致（列表通常很短，几十个玩家以内），
// 不做 map 优化以免改变遍历顺序语义。
type OffenderList struct {
	list []Offender
}

// IsOffender 对应 battleobject.cpp:940。
func (o *OffenderList) IsOffender(uid uint64) bool {
	for _, off := range o.list {
		if off.UID == uid {
			return true
		}
	}
	return false
}

// DamageOf 返回某来源累计造成的伤害。
func (o *OffenderList) DamageOf(uid uint64) uint64 {
	for _, off := range o.list {
		if off.UID == uid {
			return off.Damage
		}
	}
	return 0
}

// RemoveOffender 对应 battleobject.cpp:950。返回是否删除成功。
func (o *OffenderList) RemoveOffender(uid uint64) bool {
	for i, off := range o.list {
		if off.UID == uid {
			o.list = append(o.list[:i], o.list[i+1:]...)
			return true
		}
	}
	return false

}

// AddDamage 对应 battleobject.cpp:960-978。
//
//	已存在 -> damage += n, activeTime = now
//	不存在 -> 追加 {uid, n, now}
//
// 与原版一样，n 为负数时不由本方法处理（原版是 fflassert(nDamage >= 0)）。
func (o *OffenderList) AddDamage(uid uint64, damage uint64, nowSec int64) {
	if uid == 0 {
		return // 原版 fflassert(nUID)
	}
	for i := range o.list {
		if o.list[i].UID == uid {
			o.list[i].Damage += damage
			o.list[i].ActiveTime = nowSec
			return
		}
	}
	o.list = append(o.list, Offender{UID: uid, Damage: damage, ActiveTime: nowSec})
}

// TopOffender 取仇恨最高的来源（用于怪物选目标）。
//
// 原版没有这个函数，怪物选目标走 getValidChaseGrid/getChaseGrid 那套；
// 但「按累计伤害挑主要目标」是所有 MMO 掉落的通用规则（掉落归属），
// 原版在 goDie 里用 m_offenderList 算掉落归属者，这里提供等价查询。
func (o *OffenderList) TopOffender() (Offender, bool) {
	if len(o.list) == 0 {
		return Offender{}, false
	}
	best := o.list[0]
	for _, off := range o.list[1:] {
		if off.Damage > best.Damage {
			best = off
		}
	}
	return best, true
}

// Prune 清理超时的仇恨记录，返回被清理的数量。
//
// 原版没有显式清理函数，但 Offender.activeTime 的存在说明仇恨会随时间失效
// （否则这个字段没有意义）。这里给出一个显式入口，参数为「多久算过期」。
func (o *OffenderList) Prune(nowSec int64, ttlSec int64) int {
	if ttlSec <= 0 {
		return 0
	}
	n := 0
	kept := o.list[:0]
	for _, off := range o.list {
		if nowSec-off.ActiveTime > ttlSec {
			n++
			continue
		}
		kept = append(kept, off)
	}
	o.list = kept
	return n
}

// Snapshot 返回按伤害降序的副本（用于结算掉落与调试）。
func (o *OffenderList) Snapshot() []Offender {
	out := make([]Offender, len(o.list))
	copy(out, o.list)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Damage > out[j].Damage
	})
	return out
}

// Len 返回记录数。
func (o *OffenderList) Len() int { return len(o.list) }

// Clear 清空仇恨。
func (o *OffenderList) Clear() { o.list = nil }
