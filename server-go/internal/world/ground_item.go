package world

// ==================== 地面物品（servermap.cpp 的 GridItemList）====================
//
// 原版行为：
//
//	addGridItemID  掉落物品进格，分配全局递增的 groundSeq
//	               同一批掉落共享一个 groundSeq（用于整批过期）
//	expireGridItemLater  120 秒后按 groundSeq 整批清除
//	removeGridItemID     被拾取时按 itemID 从尾部找第一个匹配删除
//	getGroundItemIDList  按圆收集「可走格」上的物品，下发给观察者中的玩家
//	postGridItemIDList   单格变化时只通知该格的玩家观察者
//
// ★ 三个容易搞错的点：
//  1. 过期时间固定 **120 秒**（不是 60、不是 30）
//  2. 同一批掉落共用一个 groundSeq，过期是**整批**一起清
//  3. 只有 **groundValid**（可走/可飞）的格才参与收集，墙上的格不广播

// GroundItemExpireMS 地面物品存活时长（servermap.cpp:1446 的 120*1000）。
const GroundItemExpireMS = 120 * 1000

// GroundItem 是地上一件物品。
type GroundItem struct {
	// ItemID 物品记录 ID（DBCOM_ITEMRECORD 的键）
	ItemID uint32
	// SeqID 实例序号。地面物品恒为 1（原版 addGridItemID 里写死 seqID=1）
	SeqID uint64
	// GroundSeq 掉落批次号。同批掉落共享，用于整批过期
	GroundSeq uint64
	// ExpireMS 过期时刻（世界时钟毫秒）。0 表示不过期
	ExpireMS int64
	// X, Y 所在格
	X, Y int
	// Owner 掉落归属（0 = 无归属，谁都能捡）
	Owner uint64
	// ProtectUntilMS 归属保护期到期时刻（世界时钟毫秒）
	//
	// ★ 保护期是「保护归属者的拾取权」，不是「禁止别人捡」：
	//   原版 realizeItem 在保护期内只有 owner 能捡。
	//   不做保护的话，多人打同一只怪时「谁手快谁得」，
	//   法师放完火墙去捡的路上会被路过的战士全捡走 —— 这在实战里非常气人。
	ProtectUntilMS int64
}

// GroundOwnerProtectMS 掉落归属保护期（原版 SYS_GROUNDITEMPROTECTTIME）。
//
// 取 30 秒：够玩家从 3~4 格开外跑过来，又不会让掉落长期无法被清理。
const GroundOwnerProtectMS = 30 * 1000

// GroundItemEvent 是地面物品变化事件，由上层转成 SM_GROUNDITEMIDLIST。
type GroundItemEvent struct {
	// Kind "appear" / "change" / "expire"
	Kind string
	X, Y int
}

// DropItems 把一批物品放到地上，共享同一个 groundSeq。
//
// 对照 servermap.cpp:addGridItemID → addGridItem：
//
//	groundSeq := m_groundSeq++
//	addGridItem(SDItem{itemID, seqID:1}, groundSeq, x, y, post)
//
// 返回可下发的事件（每个受影响的格一条）。
// 目标格不可走时整批丢弃 —— 原版 fflassert(groundValid(x,y)) 会直接断言失败，
// 服务端这里改为静默忽略，避免因异常坐标打挂地图 actor。
func (m *MapState) DropItems(x, y int, itemIDs []uint32) []GroundItemEvent {
	if len(itemIDs) == 0 {
		return nil
	}
	if !m.Data.InBounds(Point{X: x, Y: y}) || !m.Data.CanWalk(x, y) {
		return nil
	}
	cell, err := m.cell(x, y)
	if err != nil {
		return nil
	}

	m.groundSeq++
	seq := m.groundSeq
	expire := m.NowMS + GroundItemExpireMS

	for _, itemID := range itemIDs {
		cell.GroundItems = append(cell.GroundItems, uint64(itemID))
		m.groundItems = append(m.groundItems, GroundItem{
			ItemID:    itemID,
			SeqID:     1,
			GroundSeq: seq,
			ExpireMS:  expire,
			X:         x, Y: y,
		})
	}

	return []GroundItemEvent{{Kind: "appear", X: x, Y: y}}
}

// DropItemsOwned 放一批带归属的物品（掉落保护期）。
//
// 与 DropItems 的区别只有 Owner / ProtectUntilMS 两个字段，
// 但语义差很多：带归属的物品在保护期内只有归属者能捡。
func (m *MapState) DropItemsOwned(x, y int, itemIDs []uint32, owner uint64) []GroundItemEvent {
	events := m.DropItems(x, y, itemIDs)
	if len(events) == 0 || owner == 0 {
		return events
	}
	protected := m.NowMS + GroundOwnerProtectMS
	// 只给刚落地的那一批（尾部 len(itemIDs) 条）打标记
	start := len(m.groundItems) - len(itemIDs)
	if start < 0 {
		start = 0
	}
	for i := start; i < len(m.groundItems); i++ {
		m.groundItems[i].Owner = owner
		m.groundItems[i].ProtectUntilMS = protected
	}
	return events
}

// CanPickup 判断某玩家能否拾取这一格的某件物品。
//
// 判定顺序：
//
//  1. 物品不存在 → false
//  2. 无归属 / 保护期已过 → true
//  3. 归属者本人 → true
//  4. 其他人在保护期内 → false
//
// ★ 保护期用世界时钟而不是系统时间：
//
//	地图 actor 串行执行，同一 tick 内 NowMS 一致。
//	用 time.Now() 会让「同一 tick 内的两次拾取」行为不确定。
func (m *MapState) CanPickup(x, y int, itemID uint32, pickerUID uint64) bool {
	for _, item := range m.groundItems {
		if item.X != x || item.Y != y || item.ItemID != itemID {
			continue
		}
		if item.Owner == 0 || item.ProtectUntilMS <= m.NowMS {
			return true
		}
		return item.Owner == pickerUID
	}
	return false
}

// PickupItem 从 (x,y) 拾取一件指定 itemID 的物品。
//
// 对照 servermap.cpp:removeGridItemID —— 从**尾部**往前找第一个匹配。
// 从尾部删是正确的：后掉的物品先被捡，与客户端显示顺序一致。
//
// 返回 (是否拾到, 事件)。
func (m *MapState) PickupItem(x, y int, itemID uint32) (bool, []GroundItemEvent) {
	cell, err := m.cell(x, y)
	if err != nil {
		return false, nil
	}

	// 归属保护期在调用方（World.PickupItem）校验，
	// 这里只做「按 itemID 删除」的纯操作。
	// 先删 CellState 里的 id（尾部第一个匹配）
	found := false
	for i := len(cell.GroundItems) - 1; i >= 0; i-- {
		if uint32(cell.GroundItems[i]) == itemID {
			cell.GroundItems = append(cell.GroundItems[:i], cell.GroundItems[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}

	// 再删明细（同样从尾部找，保证与上面删的是同一件）
	for i := len(m.groundItems) - 1; i >= 0; i-- {
		item := m.groundItems[i]
		if item.X == x && item.Y == y && item.ItemID == itemID {
			m.groundItems = append(m.groundItems[:i], m.groundItems[i+1:]...)
			break
		}
	}

	return true, []GroundItemEvent{{Kind: "change", X: x, Y: y}}
}

// CountGroundItem 返回某格某 itemID 的数量（servermap.cpp:getGridItemIDCount）。
func (m *MapState) CountGroundItem(x, y int, itemID uint32) int {
	cell, err := m.cell(x, y)
	if err != nil {
		return 0
	}
	count := 0
	for _, id := range cell.GroundItems {
		if uint32(id) == itemID {
			count++
		}
	}
	return count
}

// GroundItemIDsAt 返回某格的物品 ID 列表。
func (m *MapState) GroundItemIDsAt(x, y int) []uint32 {
	cell, err := m.cell(x, y)
	if err != nil {
		return nil
	}
	out := make([]uint32, 0, len(cell.GroundItems))
	for _, id := range cell.GroundItems {
		out = append(out, uint32(id))
	}
	return out
}

// ExpireGroundItems 清除所有已到期的地面物品，返回受影响的格。
//
// 对照 servermap.cpp:expireGridItemLater 的延时回调：
// 到达期限后**按 groundSeq 整批**清除，而不是逐件。
// 这里改用统一的到期扫描，效果等价且不依赖延时器。
func (m *MapState) ExpireGroundItems() []GroundItemEvent {
	if len(m.groundItems) == 0 {
		return nil
	}
	// 收集到期批次
	expiredSeq := make(map[uint64]struct{})
	for _, item := range m.groundItems {
		if item.ExpireMS > 0 && m.NowMS >= item.ExpireMS {
			expiredSeq[item.GroundSeq] = struct{}{}
		}
	}
	if len(expiredSeq) == 0 {
		return nil
	}

	// 按 groundSeq 整批清除明细
	kept := m.groundItems[:0]
	affected := make(map[Point]struct{})
	for _, item := range m.groundItems {
		if _, drop := expiredSeq[item.GroundSeq]; drop {
			affected[Point{X: item.X, Y: item.Y}] = struct{}{}
			continue
		}
		kept = append(kept, item)
	}
	m.groundItems = kept

	// 同步各格的 id 列表：重算受影响的格
	var events []GroundItemEvent
	for p := range affected {
		cell, err := m.cell(p.X, p.Y)
		if err != nil {
			continue
		}
		cell.GroundItems = cell.GroundItems[:0]
		for _, item := range m.groundItems {
			if item.X == p.X && item.Y == p.Y {
				cell.GroundItems = append(cell.GroundItems, uint64(item.ItemID))
			}
		}
		events = append(events, GroundItemEvent{Kind: "expire", X: p.X, Y: p.Y})
	}
	return events
}

// GroundItemSnapshot 是一条下发给客户端的格子物品列表。
type GroundItemSnapshot struct {
	X       int
	Y       int
	ItemIDs []uint32
}

// CollectGroundItems 按圆收集地面物品，用于下发 SM_GROUNDITEMIDLIST。
//
// 对照 servermap.cpp:getGroundItemIDList —— 只收集 **groundValid** 的格，
// 且即使中心格非法也要能覆盖到合法格（注释明确写了）。
func (m *MapState) CollectGroundItems(cx, cy, r int) []GroundItemSnapshot {
	if r <= 0 {
		return nil
	}
	var out []GroundItemSnapshot
	DoCircle(m.Data, cx, cy, r, func(x, y int) bool {
		if !m.Data.CanWalk(x, y) {
			return false // 原版：groundValid = validC && land.canThrough()
		}
		ids := m.GroundItemIDsAt(x, y)
		if len(ids) == 0 {
			return false
		}
		out = append(out, GroundItemSnapshot{X: x, Y: y, ItemIDs: ids})
		return false
	})
	return out
}
