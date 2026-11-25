package game

import (
	"context"
	"log/slog"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 金币 / 经验 / 商店买卖 ====================
//
// 覆盖四条消息：
//
//	CM_REQUESTADDEXP     -> SM_EXP
//	CM_REQUESTADDHEALTH  -> SM_HEALTH
//	CM_QUERYSELLITEMLIST -> SM_SELLITEMLIST   （询问 NPC 某个物品的售价）
//	CM_BUY               -> SM_BUYSUCCEED / SM_BUYERROR
//
// ★ 原版的「买」不是「扣金币换物品」这么简单，而是 **以物易物**：
//
//	NPC 收到 AM_BUY 后回 AM_BUYCOST，里面是 SDBuyCost{item, costList}。
//	costList 里可以有金币，也可以是别的物品（某些 NPC 用矿石换装备）。
//	所以购买流程必须是：
//
//	  1. 问 NPC「这个要多少钱」-> 拿到 costList
//	  2. 逐个校验 costList 是否够（金币查 gold，物品查背包）
//	  3. 够了就逐个扣，再给物品
//
//	跳过第 1、2 步直接扣金币，会让「用矿石换」的 NPC 变成免费送，
//	而且金币扣错数量时客户端与服务器会永久不同步。
//
// 当前版本 NPC 还没有脚本运行时，价格用物品表 price 兜底；
// 但流程按上面三步走，接入真实 NPC 时只改 buildBuyCost 一个函数。

// 购买错误码（protocoldef.hpp:313-321）。
const (
	BuyErrNone         = 0
	BuyErrBadItem      = 1
	BuyErrLocked       = 2
	BuyErrSoldOut      = 3
	BuyErrInsufficient = 4
	BuyErrStallSelling = 5
	BuyErrBadCount     = 6
)

// MaxGold 是金币上限（SYS_MAXGOLD 的宽松替代）。
//
// 2^53 是 float64 能精确表示的最大整数：超过它，客户端把金币
// 转成浮点显示会变成 1.0000000000000002e+16 这种鬼东西。
const MaxGold uint64 = 1 << 53

// MaxBuyCount 是单次购买数量上限。
//
// 原版 parserf::valid_itemcount 只校验「能放进 uint16 的正数」；
// 这里再收一道到 999，避免客户端填个巨大值把背包刷爆。
const MaxBuyCount uint64 = 999

// ShopHandler 处理金币、经验与商店。
type ShopHandler struct {
	Players   *PlayerManager
	Worlds    *world.World
	Items     *ItemDB
	Broadcast Broadcaster
	Logger    *slog.Logger
	// Golds 金币落库（可为 nil）
	Golds GoldRepository
	// Chars 经验落库（可为 nil）
	Chars CharacterWriter
}

// GoldRepository 是金币持久化的最小接口。
type GoldRepository interface {
	Load(ctx context.Context, dbid uint32) (uint64, error)
	Set(ctx context.Context, dbid uint32, gold uint64) error
}

// ==================== 经验 ====================

// RequestAddExp 处理 CM_REQUESTADDEXP。
//
// ★ 原版这条消息是「客户端上报获得经验」，服务端直接 gainExp。
//
//	这在真实游戏里是可被伪造的（改包就能刷经验），原版也没校验 ——
//	因为正式服的经验来源全在服务端（打怪、任务），这条只用于
//	GM 补偿与离线收益。这里照抄，但记一条 warn，方便运营时发现异常。
func (h *ShopHandler) RequestAddExp(ctx context.Context, channelID uint64, req protocol.CMRequestAddExp) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	if req.AddExp == 0 {
		return nil
	}
	if h.Logger != nil {
		h.Logger.Warn("client-reported exp gain", "player", p.Name, "amount", req.AddExp)
	}
	h.GainExp(ctx, p, req.AddExp)
	return nil
}

// GainExp 加经验、按需升级、广播 SM_EXP。
//
// 顺序很重要：
//
//  1. 先加经验
//  2. 再算新等级（DisplayLevel 是经验的分段查表）
//  3. 升级了才重算 MaxHP/MaxMP 与战斗属性
//
// 如果先重算属性再算等级，玩家升级当次拿到的还是旧属性，
// 要多打一只怪才生效。
func (h *ShopHandler) GainExp(ctx context.Context, p *Player, exp uint64) {
	if exp == 0 {
		return
	}
	p.Exp += exp

	if newLevel := DisplayLevel(p.Exp); newLevel > p.Level {
		p.Level = newLevel
		// 升级时上限涨且当次回满（不是只加上限）
		p.MaxHP = 30 + int32(newLevel)*5
		p.MaxMP = 30 + int32(newLevel)*3
		p.HP = p.MaxHP
		p.MP = p.MaxMP

		if h.Worlds != nil {
			if err := h.Worlds.SetStats(ctx, p.MapID, p.UID, p.CombatStatsFor(h.Items)); err != nil && h.Logger != nil {
				h.Logger.Warn("apply level up stats failed", "player", p.Name, "error", err)
			}
		}
		h.SyncHealth(ctx, p)
		h.NotifyHealth(p)
	}

	h.sendWire(p.ChannelID, protocol.SM_EXP, protocol.SMExp{Exp: p.Exp})
	if h.Chars != nil {
		if err := h.Chars.UpdateExp(ctx, p.DBID, p.Exp); err != nil && h.Logger != nil {
			h.Logger.Warn("save exp failed", "player", p.Name, "error", err)
		}
	}
}

// RequestAddHealth 处理 CM_REQUESTADDHEALTH（GM/药水前的血量上报）。
func (h *ShopHandler) RequestAddHealth(ctx context.Context, channelID uint64, req protocol.CMRequestAddHealth) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	p.HP += req.AddHP
	if p.HP > p.MaxHP {
		p.HP = p.MaxHP
	}
	if p.HP < 0 {
		p.HP = 0
	}
	p.MP += req.AddMP
	if p.MP > p.MaxMP {
		p.MP = p.MaxMP
	}
	if p.MP < 0 {
		p.MP = 0
	}
	h.SyncHealth(ctx, p)
	h.NotifyHealth(p)
	return nil
}

// ==================== 金币 ====================

// AddGold 加金币并推 SM_GOLD，返回实际金币数。
func (h *ShopHandler) AddGold(ctx context.Context, p *Player, amount uint64) uint64 {
	if amount == 0 {
		return p.Gold
	}
	total := p.Gold + amount
	if total < p.Gold || total > MaxGold { // 溢出或超上限
		total = MaxGold
	}
	h.SetGold(ctx, p, total)
	return p.Gold
}

// SetGold 直接设置金币数。
func (h *ShopHandler) SetGold(ctx context.Context, p *Player, amount uint64) {
	if amount > MaxGold {
		amount = MaxGold
	}
	p.Gold = amount
	if h.Golds != nil {
		if err := h.Golds.Set(ctx, p.DBID, p.Gold); err != nil && h.Logger != nil {
			h.Logger.Warn("save gold failed", "player", p.Name, "gold", p.Gold, "error", err)
		}
	}
	h.sendWire(p.ChannelID, protocol.SM_GOLD, protocol.SMGold{Gold: p.Gold})
}

// ==================== 商店 ====================

// QuerySellItemList 处理 CM_QUERYSELLITEMLIST。
//
// 原版把请求转给 NPC actor（AM_QUERYSELLITEMLIST），由 NPC 脚本
// 决定能卖什么、多少钱。当前版本没有 NPC 脚本运行时，用物品表
// price 兜底：能查到 price>0 的物品就按原价卖。
//
// ★ 一次只查一个 itemID（协议如此），返回的 List 最多一条。
//
//	客户端打开商店时会对每个候选物品各发一次请求，不是一次全要。
func (h *ShopHandler) QuerySellItemList(ctx context.Context, channelID uint64, req protocol.CMQuerySellItemList) error {
	if _, ok := h.Players.ByChannel(channelID); !ok {
		return nil
	}

	list := protocol.SDSellItemList{NPCUID: req.NPCUID}
	if rec, exists := h.Items.Get(req.ItemID); exists {
		cost := h.priceOf(rec)
		if cost > 0 {
			list.List = append(list.List, protocol.SDSellItem{
				Item:     protocol.SDItem{ItemID: rec.ItemID, Count: 1},
				CostList: []protocol.SDCostItem{{ItemID: GoldItemID, Count: uint64(cost)}},
			})
		}
	}
	h.sendCereal(channelID, protocol.SM_SELLITEMLIST, list)
	return nil
}

// GoldItemID 是物品表里「金币」这一条的 itemID。
//
// ★ 不能硬编码成 1。虽然当前 itemrecord.inc 的第 1 条恰好是
// 「金币（小）」，但那是数组顺序，重新生成表就可能变。
// 所以在 LoadItemDB 之后按名字解析一次（见 ItemDB.GoldID）。
func GoldItemIDFor(items *ItemDB) uint32 {
	if items == nil {
		return 0
	}
	return items.GoldID()
}

// GoldItemID 是当前进程内解析出的金币 itemID（由 SetGoldItemID 注入）。
//
// 用包级变量而不是每处都传 ItemDB：购买路径里到处都要判
// 「这个 cost 是不是金币」，每处都传一遍会让函数签名很脏。
var GoldItemID uint32 = 1

// Buy 处理 CM_BUY。
//
// 流程（对照 playernet.cpp:878 net_CM_BUY）：
//
//  1. 数量合法性（1..MaxBuyCount）
//  2. 构造 costList
//  3. 逐个校验够不够 -> 不够报 BUYERR_INSUFFCIENT
//  4. 逐个扣（金币走 SetGold，物品走 removeItem）
//  5. 给物品（packable 的按堆叠规则合并）
//  6. 回 SM_BUYSUCCEED + 更新背包
func (h *ShopHandler) Buy(ctx context.Context, channelID uint64, req protocol.CMBuy) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}

	fail := func(code uint16) error {
		h.sendWire(channelID, protocol.SM_BUYERROR, protocol.SMBuyError{
			NPCUID: req.NPCUID, ItemID: req.ItemID, SeqID: req.SeqID, Error: code,
		})
		return nil
	}

	rec, exists := h.Items.Get(req.ItemID)
	if !exists {
		return fail(BuyErrBadItem)
	}
	if req.Count == 0 || req.Count > MaxBuyCount {
		return fail(BuyErrBadCount)
	}

	costList, ok := h.buildBuyCost(rec, req.Count)
	if !ok {
		return fail(BuyErrSoldOut)
	}

	// 校验：金币与物品分开查
	var goldCost uint64
	for _, cost := range costList {
		if cost.ItemID == GoldItemID {
			goldCost += cost.Count
			continue
		}
		if !p.HasItemCount(cost.ItemID, uint32(cost.Count)) {
			return fail(BuyErrInsufficient)
		}
	}
	if goldCost > p.Gold {
		return fail(BuyErrInsufficient)
	}

	// 扣费
	if goldCost > 0 {
		h.SetGold(ctx, p, p.Gold-goldCost)
	}
	for _, cost := range costList {
		if cost.ItemID == GoldItemID {
			continue
		}
		p.RemoveItemByID(cost.ItemID, uint32(cost.Count))
	}

	// 给物品
	count := uint32(req.Count)
	p.AddItem(rec, Item{ItemID: rec.ItemID, Count: count})

	// 回包
	h.sendWire(channelID, protocol.SM_BUYSUCCEED, protocol.SMBuySucceed{
		NPCUID: req.NPCUID, ItemID: rec.ItemID, SeqID: req.SeqID,
	})
	h.notifyInventory(p)
	return nil
}

// buildBuyCost 构造购买某物品 count 个的花费列表。
//
// 当前模型：cost = price * count，只收金币。
// 接入 npcrecord 后这里改成查 NPC 的售卖表（可能含以物易物）。
func (h *ShopHandler) buildBuyCost(rec ItemRecord, count uint64) ([]protocol.SDCostItem, bool) {
	price := h.priceOf(rec)
	if price <= 0 {
		return nil, false
	}
	return []protocol.SDCostItem{{
		ItemID: GoldItemID,
		Count:  uint64(price) * count,
	}}, true
}

// priceOf 返回物品的售价。
//
// 物品表的 price 是「卖给 NPC 的价格」，买价通常更高。
// 原版由 NPC 脚本决定，这里用 1 倍（不赚差价），等 npcrecord 接入再调。
func (h *ShopHandler) priceOf(rec ItemRecord) int {
	if rec.Price <= 0 {
		return 0
	}
	return rec.Price
}

// Sell 处理卖出（当前协议里没有独立的 CM_SELL，由 NPC 输入框流程承载）。
//
// 保留这个方法是为了让「玩家把物品卖给 NPC」在后端有落点：
// 后续接 npcrecord 时，NPC 脚本会调它。
func (h *ShopHandler) Sell(ctx context.Context, p *Player, itemID, seqID uint32) bool {
	rec, ok := h.Items.Get(itemID)
	if !ok {
		return false
	}
	if _, it := p.findItem(itemID, seqID); it == nil {
		return false
	}
	p.removeWholeItem(itemID, seqID)
	price := h.priceOf(rec)
	if price > 0 {
		h.AddGold(ctx, p, uint64(price))
	}
	h.notifyInventory(p)
	return true
}

// ==================== 辅助 ====================

func (h *ShopHandler) sendWire(channelID uint64, id protocol.SMType, value any) {
	if h.Broadcast == nil || channelID == 0 {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, value)
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(channelID, id, payload, 0)
}

func (h *ShopHandler) sendCereal(channelID uint64, id protocol.SMType, value protocol.PortableMarshaler) {
	if h.Broadcast == nil || channelID == 0 {
		return
	}
	payload, err := protocol.CerealSerialize(value)
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(channelID, id, payload, 0)
}

// NotifyHealth 把血量推给玩家自己（SM_HEALTH）。
func (h *ShopHandler) NotifyHealth(p *Player) {
	if p.ChannelID == 0 || h.Broadcast == nil {
		return
	}
	payload, err := protocol.CerealSerialize(protocol.SDHealth{
		UID: p.UID, HP: p.HP, MP: p.MP, MaxHP: p.MaxHP, MaxMP: p.MaxMP,
	})
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_HEALTH, payload, 0)
}

// SyncHealth 把 Player 的血量写回世界实体。
func (h *ShopHandler) SyncHealth(ctx context.Context, p *Player) {
	if h.Worlds == nil {
		return
	}
	_ = h.Worlds.MutateEntity(ctx, p.MapID, p.UID, func(e *world.Entity) error {
		e.HP = p.HP
		e.MP = p.MP
		e.MaxHP = p.MaxHP
		e.MaxMP = p.MaxMP
		return nil
	})
}

func (h *ShopHandler) notifyInventory(p *Player) {
	payload, err := BuildInventoryPayload(p)
	if err != nil || h.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_INVENTORY, payload, 0)
}
