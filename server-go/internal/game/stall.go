package game

import (
	"context"
	"log/slog"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// ==================== 摆摊 ====================
//
// 【依据】stalldb.cpp 295 行 + playernet.cpp 的 net_CM_*STALL*
//
// 摆摊是「离线也能卖」的机制：卖家摆上东西后可以走开（甚至下线），
// 买家直接从摊位上买。所以**摊位数据必须入库**，不能只在内存。
//
// 存储层（storage/auctions_stalls.go 的 StallRepository）已有：
// Load / AddItem / RemoveItem / Start / Stop / TakeSoldItem / ListSelling
//
// ★ 摆摊与交易的关键区别：
//
//	交易：双方必须同时在线、面对面
//	摆摊：卖家可以离线，买家自助 —— 所以是「先扣物品进摊位」而不是「交换」

// 摆摊错误码（protocoldef.hpp:243-256 STALLERR_*）。
//
// ★ 定义在 game 包而不是 storage：
//
//	错误码是**协议层**概念（客户端据此弹提示），
//	storage 层只关心「操作成功与否」。
//	放进 storage 会让存储层承担不该它管的协议知识。
const (
	stallErrNone       uint8 = 0
	stallErrNotAllowed uint8 = 1
	stallErrBusy       uint8 = 2
	stallErrDead       uint8 = 3
	stallErrSelling    uint8 = 4
	stallErrNotSelling uint8 = 5
	stallErrFull       uint8 = 6
	stallErrEmpty      uint8 = 7
	stallErrBadItem    uint8 = 8
	stallErrBadPrice   uint8 = 9
	stallErrBadName    uint8 = 10
	stallErrBadCount   uint8 = 11
	stallErrNoGold     uint8 = 12
	stallErrSelfBuy    uint8 = 13
)

// StallHandler 处理摊位相关消息。
type StallHandler struct {
	Players   *PlayerManager
	Broadcast Broadcaster
	Logger    *slog.Logger
	Repo      *storage.StallRepository
	Items     *ItemDB
	Golds     GoldRepository
	Worlds    WorldLookup
}

// AddItem 往摊位加一件物品（CM_ADDSTALLITEM）。
//
// 语义是「从背包拿出这件东西放到摊位上」——
// 不是「标记这件东西可卖」。所以必须真的从背包移除，
// 否则玩家可以同时「摆着卖」和「拿去交易/寄存」。
func (h *StallHandler) AddItem(ctx context.Context, channelID uint64, req protocol.CMAddStallItem) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	if req.Price == 0 {
		h.sendError(p, uint8(stallErrBadPrice))
		return nil
	}

	idx, it := p.findItem(req.ItemID, req.SeqID)
	if it == nil {
		h.sendError(p, uint8(stallErrBadItem))
		return nil
	}

	sdItem := toSDItem(*it)
	added, err := h.Repo.AddItem(ctx, p.DBID, sdItem, req.Price)
	if err != nil {
		h.sendError(p, uint8(stallErrBusy))
		return nil
	}

	// 摊位入库成功才从背包移除（顺序反了会「物品消失但没上摊」）
	p.Inventory = append(p.Inventory[:idx], p.Inventory[idx+1:]...)
	h.pushInventory(p)

	// 推最新摊位给自己（客户端刷新面板）
	h.pushStall(ctx, p)
	_ = added
	return nil
}

// RemoveItem 从摊位取回物品（CM_REMOVESTALLITEM）。
func (h *StallHandler) RemoveItem(ctx context.Context, channelID uint64, req protocol.CMRemoveStallItem) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	item, found, err := h.Repo.RemoveItem(ctx, p.DBID, req.ItemID, req.SeqID)
	if err != nil || !found {
		h.sendError(p, uint8(stallErrBadItem))
		return nil
	}
	// 回到背包（带持久值）
	p.Inventory = append(p.Inventory, fromSDItem(item))
	h.pushInventory(p)
	h.pushStall(ctx, p)
	return nil
}

// Start 开摊（CM_STARTSTALL）。
func (h *StallHandler) Start(ctx context.Context, channelID uint64, req protocol.CMStartStall) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	// StaticBuffer 有 String() 方法，直接转
	name := trimCString([]byte(req.Name.String()))
	if name == "" {
		name = p.Name + "的摊位"
	}
	ok, err := h.Repo.Start(ctx, p.DBID, name, p.MapID, int32(p.X), int32(p.Y))
	if err != nil || !ok {
		h.sendError(p, uint8(stallErrBadItem))
		return nil
	}
	h.pushStall(ctx, p)
	return nil
}

// Close 收摊（CM_CLOSESTALL）。
func (h *StallHandler) Close(ctx context.Context, channelID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	if _, err := h.Repo.Stop(ctx, p.DBID); err != nil {
		return nil
	}
	h.pushStall(ctx, p)
	return nil
}

// Query 查某人的摊位（CM_QUERYSTALL）。
func (h *StallHandler) Query(ctx context.Context, channelID uint64, sellerDBID uint32) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	stall, found, err := h.Repo.Load(ctx, sellerDBID)
	if err != nil || !found {
		h.sendError(p, uint8(stallErrBadItem))
		return nil
	}
	if payload, err := protocol.CerealSerialize(stall); err == nil {
		h.sendTo(p, protocol.SM_QUERYSTALLOK, payload)
	}
	return nil
}

// Buy 买摊位上的东西（CM_BUYSTALLITEM）。
//
// ★ 顺序：先从摊位原子取走（含扣库存），再扣钱、再给货。
//
//	反过来「先扣钱再取货」的话，取货失败（被人抢先买了）
//	玩家就白花了钱且拿不到东西。
func (h *StallHandler) Buy(ctx context.Context, channelID uint64, req protocol.CMBuyStallItem) error {
	buyer, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	if req.SellerDBID == buyer.DBID {
		h.sendError(buyer, uint8(stallErrSelfBuy))
		return nil
	}

	// 1. 校验价格（防止客户端伪造低价）
	stall, found, err := h.Repo.Load(ctx, req.SellerDBID)
	if err != nil || !found {
		h.sendError(buyer, uint8(stallErrBadItem))
		return nil
	}
	var listed *protocol.SDStallItem
	for i := range stall.ItemList {
		if stall.ItemList[i].Item.ItemID == req.ItemID &&
			stall.ItemList[i].Item.SeqID == req.SeqID {
			listed = &stall.ItemList[i]
			break
		}
	}
	if listed == nil {
		h.sendError(buyer, uint8(stallErrBadItem))
		return nil
	}
	// ★ 价格以服务端摊位为准，客户端传的 req.Price 只用于校验一致性
	if listed.Price != req.Price {
		h.sendError(buyer, uint8(stallErrBadPrice))
		return nil
	}
	totalPrice := listed.Price * req.Count
	if buyer.Gold < totalPrice {
		h.sendError(buyer, uint8(stallErrNoGold))
		return nil
	}

	// 2. 从摊位取走（原子：库存够才扣）
	sold, taken, err := h.Repo.TakeSoldItem(ctx, req.SellerDBID, buyer.DBID, req.ItemID, req.SeqID)
	if err != nil || !taken {
		h.sendError(buyer, uint8(stallErrBadItem))
		return nil
	}

	// 3. 扣钱 + 给货
	buyer.Gold -= totalPrice
	if h.Golds != nil {
		_ = h.Golds.Set(ctx, buyer.DBID, buyer.Gold)
	}
	buyer.Inventory = append(buyer.Inventory, fromSDItem(sold.Item))
	h.pushInventory(buyer)
	h.pushGold(buyer)

	// 4. 卖家在线则把货款给他（离线则留在摊位上，下次登录取）
	if seller, exists := h.Players.ByDBID(req.SellerDBID); exists {
		seller.Gold += totalPrice
		if seller.Gold > MaxGold {
			seller.Gold = MaxGold
		}
		if h.Golds != nil {
			_ = h.Golds.Set(ctx, seller.DBID, seller.Gold)
		}
		h.pushGold(seller)
	}

	// 双方刷新摊位面板
	h.pushStall(ctx, buyer)
	if seller, exists := h.Players.ByDBID(req.SellerDBID); exists {
		h.pushStall(ctx, seller)
	}
	return nil
}

// ==================== 内部 ====================

func (h *StallHandler) pushStall(ctx context.Context, p *Player) {
	if p == nil || h.Repo == nil {
		return
	}
	stall, found, err := h.Repo.Load(ctx, p.DBID)
	if err != nil {
		return
	}
	if !found {
		// 没有摊位：推空列表让客户端清空面板
		stall = protocol.SDStall{}
	}
	if payload, err := protocol.CerealSerialize(stall); err == nil {
		h.sendTo(p, protocol.SM_STALL, payload)
	}
}

func (h *StallHandler) pushInventory(p *Player) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	list := make([]protocol.SDItem, 0, len(p.Inventory))
	for _, it := range p.Inventory {
		list = append(list, toSDItem(it))
	}
	if payload, err := protocol.CerealSerialize(protocol.SDInventory{ItemList: list}); err == nil {
		_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_INVENTORY, payload, 0)
	}
}

func (h *StallHandler) pushGold(p *Player) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMGold{Gold: p.Gold}); err == nil {
		_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_GOLD, payload, 0)
	}
}

func (h *StallHandler) sendError(p *Player, code uint8) {
	if code == uint8(stallErrNone) || h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMStallError{Error: code}); err == nil {
		_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_STALLERROR, payload, 0)
	}
}

func (h *StallHandler) sendTo(p *Player, id protocol.SMType, payload []byte) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, id, payload, 0)
}
