package game

import (
	"errors"
)

// ==================== 背包与装备操作（serdesmsg.hpp SDInventory / SDBelt）====================
//
// 原版 SDInventory 是 std::vector<SDItem>，**没有容量上限**，
// 靠 (itemID, seqID) 保证唯一。这里照同样的语义实现。
//
// ★ 三个必须照抄的细节：
//  1. **seqID 在同一 itemID 内唯一**，不同 itemID 可以复用同一个 seqID
//  2. 穿戴时**背包里那件被移除，旧装备回背包**（不是简单交换）
//  3. 脱装备**永远进背包**（原版注释：server doesn't track if item is grabbed
//     or in inventory, when disarms the wear item, server always put it into
//     the inventory）

var (
	ErrItemNotFound  = errors.New("item not found in inventory")
	ErrItemNotEnough = errors.New("item count not enough")
	ErrInventoryFull = errors.New("inventory full")
)

// findItem 在背包里找指定 (itemID, seqID)，返回索引与指针；找不到返回 -1。
func (p *Player) findItem(itemID, seqID uint32) (int, *Item) {
	for i := range p.Inventory {
		if p.Inventory[i].ItemID == itemID && uint32(p.Inventory[i].SeqID) == seqID {
			return i, &p.Inventory[i]
		}
	}
	return -1, nil
}

// hasCount 判断背包里某 (itemID, seqID) 的数量是否够。
func (p *Player) hasCount(itemID, seqID uint32, count uint32) bool {
	_, it := p.findItem(itemID, seqID)
	if it == nil {
		return false
	}
	return it.Count >= count
}

// InventoryMaxHold 是单格最大堆叠数（SYS_INVGRIDMAXHOLD）。
const InventoryMaxHold = 99

// addItem 往背包加物品（serdesmsg.cpp:129 SDInventory::add）。
//
// ★ 原版行为，容易写错的两点：
//  1. **只有 packable 的物品才合并**（药水/卷轴/护身符/药粉）。
//     武器、衣服、戒指等不可堆叠，同 itemID 也必须各占一格。
//     初版对所有物品都合并，导致「两把同名武器变成一把 count=2」。
//  2. 合并不超过 **99**（SYS_INVGRIDMAXHOLD）。超了就新占一格。
//
// 参数 rec 用来判断 packable；不能传零值（会全当不可堆叠）。
func (p *Player) AddItem(rec ItemRecord, item Item) {
	if item.Count == 0 {
		item.Count = 1
	}

	// 只有可堆叠的物品才尝试合并
	if rec.Packable() {
		for i := range p.Inventory {
			if p.Inventory[i].ItemID != item.ItemID {
				continue
			}
			// 原版注释：只支持改一件，不做 (55+56)->(99,12) 的自动拆分
			if p.Inventory[i].Count+item.Count <= InventoryMaxHold {
				p.Inventory[i].Count += item.Count
				return
			}
		}
	}

	// 新占一格，分配唯一 seqID
	if item.SeqID == 0 {
		item.SeqID = p.nextSeqID(item.ItemID)
	}
	p.Inventory = append(p.Inventory, item)
}

// addItemNoRec 是不知道物品记录时的退化版本：一律新占一格。
//
// 用于「旧装备回背包」这类场景 —— 那里只知道 Item，
// 但为安全起见宁可多占一格，也不要错误合并。
func (p *Player) addItemNoRec(item Item) {
	if item.Count == 0 {
		item.Count = 1
	}
	if item.SeqID == 0 {
		item.SeqID = p.nextSeqID(item.ItemID)
	}
	p.Inventory = append(p.Inventory, item)
}

// removeItem 从背包移除指定数量，返回是否移除成功。
//
// 数量扣完时把该格删掉（原版 remove 返回 nullptr 表示整件移除）。
func (p *Player) removeItem(itemID, seqID uint32, count uint32) bool {
	idx, it := p.findItem(itemID, seqID)
	if it == nil || it.Count < count {
		return false
	}
	it.Count -= count
	if it.Count == 0 {
		p.Inventory = append(p.Inventory[:idx], p.Inventory[idx+1:]...)
	}
	return true
}

// removeWholeItem 整件移除（丢弃/穿戴时用）。
func (p *Player) removeWholeItem(itemID, seqID uint32) (Item, bool) {
	idx, it := p.findItem(itemID, seqID)
	if it == nil {
		return Item{}, false
	}
	item := *it
	p.Inventory = append(p.Inventory[:idx], p.Inventory[idx+1:]...)
	return item, true
}

// nextSeqID 为指定 itemID 分配一个新的 seqID（同 itemID 内唯一）。
//
// 原版 SDItem::buildItemList 会给新物品分配 seqID；
// 地面掉落/商店购买/怪物掉落都要各自唯一。
func (p *Player) nextSeqID(itemID uint32) uint64 {
	var maxSeq uint64
	for i := range p.Inventory {
		if p.Inventory[i].ItemID != itemID {
			continue
		}
		if p.Inventory[i].SeqID > maxSeq {
			maxSeq = p.Inventory[i].SeqID
		}
	}
	return maxSeq + 1
}

// ==================== 装备槽 ====================

// wearItem 取某槽位当前装备。
func (p *Player) wearItem(wltype int) *Item {
	if p.Wear == nil {
		return nil
	}
	return p.Wear[wltype]
}

// setWearItem 设置某槽位装备（传 nil 表示清空）。
func (p *Player) setWearItem(wltype int, item *Item) {
	if p.Wear == nil {
		p.Wear = make(map[int]*Item)
	}
	if item == nil {
		delete(p.Wear, wltype)
		return
	}
	// 存副本，避免外部修改背包里的对象
	cp := *item
	p.Wear[wltype] = &cp
}

// ==================== 腰带 ====================

// beltSlotCount 是腰带格数（原版固定 6 格）。
const beltSlotCount = 6

// beltItem 取腰带某格。
func (p *Player) beltItem(slot int) *Item {
	if slot < 0 || slot >= beltSlotCount {
		return nil
	}
	return p.Belt[slot]
}

// setBeltItem 设置腰带某格。
func (p *Player) setBeltItem(slot int, item *Item) {
	if slot < 0 || slot >= beltSlotCount {
		return
	}
	if item == nil {
		p.Belt[slot] = nil
		return
	}
	cp := *item
	p.Belt[slot] = &cp
}

// ==================== 药剂效果 ====================

// consumePotion 喝一瓶药，返回是否成功。
//
// 原版 hero.cpp 的 consumePotion 会按 potion.hp/mp 加血加蓝，
// 并受 SYS_POTIONRECOVER 节流（time 字段控制恢复速度）。
// MVP 阶段先直接加血蓝，上限钳到 MaxHP/MaxMP。
func (p *Player) consumePotion(rec ItemRecord) bool {
	if rec.Potion == nil {
		return false
	}
	changed := false
	if rec.Potion.HP > 0 {
		p.HP += int32(rec.Potion.HP)
		if p.MaxHP > 0 && p.HP > p.MaxHP {
			p.HP = p.MaxHP
		}
		changed = true
	}
	if rec.Potion.MP > 0 {
		p.MP += int32(rec.Potion.MP)
		if p.MaxMP > 0 && p.MP > p.MaxMP {
			p.MP = p.MaxMP
		}
		changed = true
	}
	return changed
}

// canWear 检查等级/职业是否满足穿戴要求。
//
// 原版 canWear 校验 equip.req.dc/mc/sc/level/job。
// 当前物品表只提取了 name/type/weight/price，没提 req 字段，
// 所以这里只做「等级」的粗略校验（如果后续提取 req 再补全）。
// 不做校验的话，1 级号能穿顶级装备 —— 那是明显的逻辑漏洞。
func canWear(rec ItemRecord, level int, job uint8) bool {
	// 物品表暂无 req 字段：按价格做粗略等级门槛（高价装备要求高等级）
	// 这是临时策略，提取 req 后替换。
	if rec.Price <= 0 {
		return true
	}
	requiredLevel := rec.Price / 1000
	if requiredLevel < 1 {
		requiredLevel = 1
	}
	if requiredLevel > 50 {
		requiredLevel = 50
	}
	return level >= requiredLevel
}
