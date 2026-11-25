package game

import (
	"context"
	"math/rand"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 装备持久（磨损 / 修理）====================
//
// 【依据】player.cpp:2650 damageWearItem / :2990 repairInventoryItem
//
// 三件事必须一起做，缺一个经济系统就转不起来：
//
//	1. 攻击时武器磨损（1/200）
//	2. 被击时随机一件护甲磨损（1/300）
//	3. 持久耗尽 → 装备彻底销毁（不是「留在身上但不能用」）
//
// ★ 为什么这决定经济能不能转：
//
//	如果装备永不磨损，玩家打一次怪赚的金币就没有任何消耗出口。
//	金币只进不出 → 通货膨胀 → 商店系统失去意义 → 交易/拍卖都没有定价基准。
//	「修理费」是传奇里金币最主要的消耗口，和「药钱」并列。

const (
	// DurabilityLossRate 普通修理永久损失的最大持久比例（SYS_DURALOSSRATE）。
	//
	// 普通修理恢复当前持久，但**永久降低最大持久** (max-curr)/10。
	// 这就是「特殊修理」存在的理由 —— 它不损失上限但要花更多钱。
	DurabilityLossRate = 10

	// WeaponDurabilityOdds 每次攻击武器掉 1 点持久的概率倒数（SYS_WEAPONDURALOSSODDS）
	WeaponDurabilityOdds = 200
	// ArmorDurabilityOdds 每次被击护甲掉 1 点持久的概率倒数（SYS_ARMORDURALOSSODDS）
	ArmorDurabilityOdds = 300
)

// DamageWearItem 让某个部位的装备掉一点持久（player.cpp:2650）。
//
// 返回 true 表示**装备被销毁了**（持久耗尽）。
//
// 语义严格照原版：
//
//	1. 该部位没装备 / 装备没有持久概念 → false（药水、材料不磨损）
//	2. 掷骰没中                       → false
//	3. 当前持久 > 0                   → 减 1，推 SM_WEARITEMDURATION
//	4. 当前持久 == 0（这次要减到 -1）   → **装备永久销毁**
//
// ★ 第 4 条是「销毁」不是「留在身上」：
//
//	原版注释 "durability has been exhausted, this loss destroys the
//	item permanently / don't put it back into the inventory, it's gone"。
//	写成「留着但不能用」的话，玩家会以为还能修，背包里堆满尸体装备。
func (h *DurabilityHandler) DamageWearItem(ctx context.Context, p *Player, wlType int, odds int) bool {
	if p == nil || odds <= 0 {
		return false
	}
	item, ok := p.Wear[wlType]
	if !ok || item == nil {
		return false
	}
	if !item.HasDurability() {
		return false // 该物品没有持久概念
	}
	if h.randIntn(odds) != 0 {
		return false
	}

	// 还有持久：减 1 并通知客户端
	if item.Durability > 0 {
		item.Durability--
		h.notifyWearDuration(p, wlType, item)
		h.saveWear(p, wlType, item)
		return false
	}

	// 持久已耗尽：永久销毁
	rec, found := h.Items.Get(item.ItemID)
	name := "装备"
	if found {
		name = rec.Name
	}
	delete(p.Wear, wlType)
	h.notifyEquipWear(p, wlType)
	h.saveWearRemoval(p, wlType)
	h.sendText(p, name+"因持久耗尽而彻底损坏")

	// 装备没了要重算属性 —— 不然损坏后玩家还带着满属性
	if h.Worlds != nil {
		_ = h.Worlds.SetStats(ctx, p.MapID, p.UID, p.CombatStatsFor(h.Items))
	}
	return true
}

// DamageWeapon 是「攻击一次」的武器磨损。
func (h *DurabilityHandler) DamageWeapon(ctx context.Context, p *Player) bool {
	return h.DamageWearItem(ctx, p, WLGWeapon, WeaponDurabilityOdds)
}

// DamageRandomArmor 是「被击中一次」的护甲磨损。
//
// 【依据】player.cpp:2706 damageDefendWearItem：
//
//	先收集所有「有持久概念且不是武器」的装备位，
//	再随机挑一件去磨损。
//
// ★ 为什么随机挑而不是全磨：
//
//	全磨会让「被围殴」的玩家 5 分钟就一身全碎。
//	随机挑的期望是「每 N 次被击，随机一个部位掉一点」，
//	整体磨损速度与部位数量无关，符合手感。
func (h *DurabilityHandler) DamageRandomArmor(ctx context.Context, p *Player) bool {
	candidates := make([]int, 0, len(p.Wear))
	for wlType, item := range p.Wear {
		if wlType == WLGWeapon || item == nil {
			continue
		}
		if item.HasDurability() {
			candidates = append(candidates, wlType)
		}
	}
	if len(candidates) == 0 {
		return false
	}
	pick := candidates[h.randIntn(len(candidates))]
	return h.DamageWearItem(ctx, p, pick, ArmorDurabilityOdds)
}

// RepairItem 修理一件背包里的装备（player.cpp:2990）。
//
// special=true 是「特殊修理」：恢复满持久且**不损失上限**（要花更多钱）。
// special=false 是「普通修理」：恢复满持久，但永久降低上限。
//
// 返回 (恢复的持久点数, 是否成功)。
func (h *DurabilityHandler) RepairItem(p *Player, itemID uint32, seqID uint64, special bool) (int, bool) {
	idx := -1
	for i := range p.Inventory {
		if p.Inventory[i].ItemID == itemID && p.Inventory[i].SeqID == seqID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, false
	}
	item := &p.Inventory[idx]
	if !item.HasDurability() {
		return 0, false
	}
	if item.Durability >= item.MaxDurability {
		return 0, false // 满持久不用修
	}

	restored := int(item.MaxDurability - item.Durability)

	if !special {
		// 普通修理：永久损失 (max-curr)/10，余数按概率多损 1。
		//
		// 原版写法：lostMax = restored/10 + (rand(1..10) <= restored%10 ? 1 : 0)
		// 目的是让「只差 1 点的修理」也有 1/10 概率损上限 ——
		// 否则玩家可以频繁小修来回避上限损失。
		lostMax := restored / DurabilityLossRate
		if restored%DurabilityLossRate > 0 {
			if h.randIntn(DurabilityLossRate)+1 <= restored%DurabilityLossRate {
				lostMax++
			}
		}
		newMax := int(item.MaxDurability) - lostMax
		if newMax < 1 {
			newMax = 1 // 上限最低保留 1，不然后续永远无法再修
		}
		item.MaxDurability = uint32(newMax)
	}

	item.Durability = item.MaxDurability
	h.saveInventoryItem(p, *item)
	return restored, true
}

// ==================== 金币入账（拾取时）====================

// AddGold 给玩家加金币，返回实际加了多少（会按上限截断）。
//
// 【依据】inventorydb.cpp:116-125 dbGrantItemList 里的金币分支：
//
//	if(item.isGold()){
//	    if(item.count > SYS_MAXGOLD - gold) return nullopt;   // 溢出就整体拒绝
//	    gold += item.count;
//	}
//
// ★ 原版是「溢出则整批拒绝」，这里改为「按上限截断」：
//
//	拒绝会让「背包里 100 个金币 + 上限只剩 50 空间」时玩家一个都捡不到，
//	而截断至少能捡到 50。前者体验更差且更让人困惑。
func (h *DurabilityHandler) AddGold(ctx context.Context, p *Player, count uint64) uint64 {
	if p == nil || count == 0 {
		return 0
	}
	if p.Gold >= MaxGold {
		return 0
	}
	room := MaxGold - p.Gold
	add := count
	if add > room {
		add = room
	}
	p.Gold += add

	// 推给客户端（SM_GOLD 是定长消息）
	if h.Broadcast != nil && p.ChannelID != 0 {
		payload, err := protocol.AppendWirePayload(nil, protocol.SMGold{Gold: p.Gold})
		if err == nil {
			_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_GOLD, payload, 0)
		}
	}
	// 落库
	if h.Golds != nil {
		_ = h.Golds.Set(ctx, p.DBID, p.Gold)
	}
	return add
}

// ==================== 持久推送 ====================

func (h *DurabilityHandler) notifyWearDuration(p *Player, wlType int, item *Item) {
	if h.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMWearItemDuration{
		WLType:   uint32(wlType),
		Duration: [2]uint32{item.Durability, item.MaxDurability},
	})
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_WEARITEMDURATION, payload, 0)
}

// notifyEquipWear 通知客户端某部位装备已消失（SM_EQUIPWEAR）。
//
// SM_EQUIPWEAR 的 body 是 cerealf 序列化的 SDEquipWear{Item}，
// 装备为空时就是「这个部位没东西了」。
func (h *DurabilityHandler) notifyEquipWear(p *Player, wlType int) {
	if h.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	payload, err := protocol.CerealSerialize(protocol.SDEquipWear{
		WLType: int32(wlType),
		Item:   protocol.SDItem{},
	})
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_EQUIPWEAR, payload, 0)
}

func (h *DurabilityHandler) sendText(p *Player, text string) {
	if h.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	// SM_TEXT 是「裸字符串」消息（body 就是文本本身，没有结构体头）。
	// 原版 processrunnet.cpp:738 直接按 raw text 处理。
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_TEXT, []byte(text), 0)
}

// ==================== 落库 ====================

func (h *DurabilityHandler) saveWear(p *Player, wlType int, item *Item) {
	if h.ItemRepo == nil {
		return
	}
	_ = h.ItemRepo.PutWearItem(context.Background(), p.DBID, int32(wlType), protocol.SDItem{
		ItemID: item.ItemID, SeqID: uint32(item.SeqID), Count: uint64(item.Count),
		Duration: [2]uint64{uint64(item.Durability), uint64(item.MaxDurability)},
	})
}

func (h *DurabilityHandler) saveWearRemoval(p *Player, wlType int) {
	if h.ItemRepo == nil {
		return
	}
	_, _ = h.ItemRepo.RemoveWearItem(context.Background(), p.DBID, int32(wlType))
}

func (h *DurabilityHandler) saveInventoryItem(p *Player, item Item) {
	if h.ItemRepo == nil {
		return
	}
	_ = h.ItemRepo.PutInventoryItem(context.Background(), p.DBID, protocol.SDItem{
		ItemID: item.ItemID, SeqID: uint32(item.SeqID), Count: uint64(item.Count),
		Duration: [2]uint64{uint64(item.Durability), uint64(item.MaxDurability)},
	})
}

// RandSource 是随机源的最小接口。
type RandSource interface {
	Intn(n int) int
}

// randIntn 是「可注入的随机源」，测试用固定源保证可复现。
func (h *DurabilityHandler) randIntn(n int) int {
	if n <= 0 {
		return 0
	}
	if h.Rand != nil {
		return h.Rand.Intn(n)
	}
	return rand.Intn(n)
}

// DurabilityHandler 负责装备磨损、修理与金币入账。
//
// 这些操作都需要「写库 + 推包 + 可能改属性」，放在一起
// 是因为它们在真实流程里总是同时发生：
//
//	武器碎了 → 删装备 + 推 EQUIPWEAR + 重算属性 + 发提示文字
type DurabilityHandler struct {
	Items     *ItemDB
	Worlds    *world.World
	Broadcast Broadcaster
	ItemRepo  ItemRepository
	Golds     GoldRepository
	// Rand 可注入的随机源（测试用「恒返回 0」保证概率必中）。
	//
	// 用接口而不是 *rand.Rand：测试需要「任何 n 都返回 0」，
	// 而 math/rand 即使种子固定也不保证第一次返回 0 ——
	// 用 *rand.Rand 会让概率性断言随机失败。
	Rand RandSource
}
