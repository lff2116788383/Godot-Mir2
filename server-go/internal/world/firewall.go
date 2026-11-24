package world

// ==================== 火墙（servermap.cpp:runFireWall / attackFireWall）====================
//
// 原版行为：
//
//	FireWallMagicNode {
//	    uid, groundSeq         创造者
//	    minDC, maxDC, mcHit    伤害区间与命中
//	    startTime, lastAttackTime
//	    duration               持续毫秒
//	    dps                    每秒攻击次数（不是每秒伤害！）
//	}
//
// 每 tick 逻辑（runFireWall 的协程体）：
//
//	1. 到期（currTime >= startTime+duration）-> 把火墙从该格移除并广播
//	2. dps > 0 时，attackGap = max(1, 1000/dps)
//	   若 currTime >= lastAttackTime + attackGap -> 攻击该格上所有人并更新 lastAttackTime
//	3. 等待 min(剩余时长, 下次攻击时间 - 当前时间)
//
// ★ 极易搞错的两点：
//  1. **dps 是「每秒攻击次数」**，attackGap 是间隔毫秒（1000/dps）。
//     把 dps 当伤害值会算成完全错误的量级。
//  2. 火墙**只攻击同一格上的玩家和怪物**（doUIDList(x,y)），不攻击创造者本人，
//     也不攻击 NPC。它不会跳过安全区保护 —— 但在 4 方向火焰蔓延时才走 map 伤害路径。

// 火墙 magic 名（DBCOM_MAGICID(u8"火墙")）。这里只用 ID 做区分，具体表在 magicrecord。
const FireWallMagicID = "火墙"

// FireWall 是一个铺在地格上的火墙。
type FireWall struct {
	// UID 创造者
	UID uint64
	// GroundSeq 批次号（同批火墙共享，用于整批清除）
	GroundSeq uint64
	// MinDC, MaxDC 每次攻击的伤害区间
	MinDC int32
	MaxDC int32
	// MCHit 魔法命中
	MCHit int
	// StartMS 起始时刻（世界时钟毫秒）
	StartMS int64
	// LastAttackMS 上次攻击时刻
	LastAttackMS int64
	// DurationMS 持续时长
	DurationMS int64
	// DPS 每秒攻击次数（原版字段名就叫 dps，但语义是频率）
	DPS int
	// X, Y 所在格
	X, Y int
}

// AttackGapMS 返回两次攻击的间隔毫秒。
//
// 对照 servermap.cpp:1634：
//
//	const uint64_t attackGap = std::max<uint64_t>(1, 1000 / p->dps);
//
// 至少 1 毫秒，避免 dps 很大时除出 0 造成每帧都打。
func (f FireWall) AttackGapMS() int64 {
	if f.DPS <= 0 {
		return 0 // 不攻击
	}
	gap := int64(1000 / f.DPS)
	if gap < 1 {
		gap = 1
	}
	return gap
}

// ExpireMS 返回到期时刻。
func (f FireWall) ExpireMS() int64 { return f.StartMS + f.DurationMS }

// FireWallHit 是一次火墙伤害命中的记录，由上层转成 AM_ATTACK / 伤害消息。
type FireWallHit struct {
	// TargetUID 被烧到的实体
	TargetUID uint64
	// OwnerUID 火墙创造者
	OwnerUID uint64
	// Damage 本次伤害
	Damage int32
	// MCHit 命中值（交给受击方结算）
	MCHit int
	X, Y  int
}

// AddFireWall 在某格铺一层火墙。
//
// 同一格可叠多层火墙（原版 fireWallList 是 vector），因此不做去重。
// 返回该格的变更事件，供上层广播 SM_GROUNDFIREWALLLIST。
func (m *MapState) AddFireWall(fw FireWall) []ObserverEvent {
	if !m.Data.InBounds(Point{X: fw.X, Y: fw.Y}) || !m.Data.CanWalk(fw.X, fw.Y) {
		return nil
	}
	cell, err := m.cell(fw.X, fw.Y)
	if err != nil {
		return nil
	}
	// 批次号：调用方未指定时自动分配（原版 m_groundSeq 同时给掉落和火墙用）
	if fw.GroundSeq == 0 {
		m.groundSeq++
		fw.GroundSeq = m.groundSeq
	}
	if fw.StartMS == 0 {
		fw.StartMS = m.NowMS
	}
	if fw.DurationMS <= 0 {
		fw.DurationMS = 0
	}
	cell.FireWalls = append(cell.FireWalls, fw)
	// 用 COAppear 之外的语义：这里复用 ObserverEvent 只为「该格观察者需刷新」，
	// 上层按 SM_GROUNDFIREWALLLIST 处理
	return []ObserverEvent{{Kind: COAppear, X: fw.X, Y: fw.Y}}
}

// FireWallsAt 返回某格所有火墙。
func (m *MapState) FireWallsAt(x, y int) []FireWall {
	cell, err := m.cell(x, y)
	if err != nil {
		return nil
	}
	return append([]FireWall(nil), cell.FireWalls...)
}

// TickFireWalls 推进火墙状态，返回本 tick 的命中与到期格。
//
// 完全照抄 runFireWall 的循环体，只是改由世界时钟驱动而非协程：
//
//  1. 到期则移除
//  2. dps>0 且达到间隔则攻击该格上所有玩家/怪物（排除创造者与 NPC）
func (m *MapState) TickFireWalls() ([]FireWallHit, []Point) {
	var hits []FireWallHit
	var expired []Point

	for y := 0; y < m.Data.Height; y++ {
		for x := 0; x < m.Data.Width; x++ {
			cell, err := m.cell(x, y)
			if err != nil || len(cell.FireWalls) == 0 {
				continue
			}

			kept := cell.FireWalls[:0]
			gridChanged := false
			for _, fw := range cell.FireWalls {
				// 1. 到期
				if m.NowMS >= fw.ExpireMS() {
					gridChanged = true
					continue
				}

				// 2. 攻击
				if gap := fw.AttackGapMS(); gap > 0 {
					if m.NowMS >= fw.LastAttackMS+gap {
						hits = append(hits, m.fireWallAttack(fw)...)
						fw.LastAttackMS = m.NowMS
					}
				}
				kept = append(kept, fw)
			}

			// 写回（注意 kept 复用了底层数组）
			cell.FireWalls = kept
			if gridChanged {
				expired = append(expired, Point{X: x, Y: y})
			}
		}
	}

	return hits, expired
}

// fireWallAttack 对某格上的玩家与怪物造成火墙伤害。
//
// 对照 servermap.cpp:attackFireWall 的 doUIDList 回调：
//   - 跳过创造者自己
//   - 只打 UID_PLY / UID_MON（NPC 不打）
//   - 伤害 = rand(minDC, maxDC+1)，即 [minDC, maxDC] 闭区间
func (m *MapState) fireWallAttack(fw FireWall) []FireWallHit {
	cell, err := m.cell(fw.X, fw.Y)
	if err != nil {
		return nil
	}

	damage := fw.MinDC
	if fw.MaxDC > fw.MinDC {
		// 用世界时钟做种子，保证同一 tick 内可复现
		span := uint32(fw.MaxDC - fw.MinDC + 1)
		damage = fw.MinDC + int32(randForTick(m.NowMS, fw.GroundSeq, fw.X, fw.Y)%span)
	}

	var hits []FireWallHit
	for uid := range cell.Occupants {
		if uid == fw.UID {
			continue
		}
		entity, ok := m.Entities[uid]
		if !ok {
			continue
		}
		if entity.Type != EntityPlayer && entity.Type != EntityMonster {
			continue // NPC 不吃火墙
		}
		hits = append(hits, FireWallHit{
			TargetUID: uid, OwnerUID: fw.UID, Damage: damage, MCHit: fw.MCHit,
			X: fw.X, Y: fw.Y,
		})
	}
	return hits
}

// randForTick 是一个确定性的伪随机数，用于火墙伤害。
//
// 为什么不用 math/rand 全局源：火墙伤害在同一 tick 内应当可复现
// （回放/测试需要）。这里用 (nowMS, groundSeq, x, y) 混合成一个值，
// 保证「同一 tick、同一火墙」每次都算出同样的伤害。
func randForTick(nowMS int64, groundSeq uint64, x, y int) uint32 {
	h := uint64(nowMS)
	h ^= groundSeq * 0x9E3779B97F4A7C15
	h ^= uint64(x) * 0xBF58476D1CE4E5B9
	h ^= uint64(y) * 0x94D049BB133111EB
	h ^= h >> 30
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 27
	h *= 0x94D049BB133111EB
	h ^= h >> 31
	return uint32(h)
}

// RemoveFireWall 按 groundSeq 移除某格的火墙（对应原版按 seq 查找后 erase）。
func (m *MapState) RemoveFireWall(x, y int, groundSeq uint64) bool {
	cell, err := m.cell(x, y)
	if err != nil {
		return false
	}
	for i, fw := range cell.FireWalls {
		if fw.GroundSeq == groundSeq {
			cell.FireWalls = append(cell.FireWalls[:i], cell.FireWalls[i+1:]...)
			return true
		}
	}
	return false
}

// FireWallSnapshot 是一条下发给客户端的地格火墙列表。
type FireWallSnapshot struct {
	X int
	Y int
	// OwnerUIDs 各层火墙的创造者（客户端只需知道有火，用于绘制）
	OwnerUIDs []uint64
}

// CollectFireWalls 按圆收集火墙，用于下发 SM_GROUNDFIREWALLLIST。
//
// 与地面物品一致：只收集 groundValid 的格。
func (m *MapState) CollectFireWalls(cx, cy, r int) []FireWallSnapshot {
	if r <= 0 {
		return nil
	}
	var out []FireWallSnapshot
	DoCircle(m.Data, cx, cy, r, func(x, y int) bool {
		if !m.Data.CanWalk(x, y) {
			return false
		}
		walls := m.FireWallsAt(x, y)
		if len(walls) == 0 {
			return false
		}
		owners := make([]uint64, 0, len(walls))
		for _, fw := range walls {
			owners = append(owners, fw.UID)
		}
		out = append(out, FireWallSnapshot{X: x, Y: y, OwnerUIDs: owners})
		return false
	})
	return out
}
