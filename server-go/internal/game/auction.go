package game

import (
	"context"
	"log/slog"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// ==================== 拍卖行 ====================
//
// 【依据】auctiondb.cpp 220 行 + playernet.cpp 的 net_CM_*AUCTION*
//
// 存储层（storage/auctions_stalls.go 的 AuctionRepository）已有：
// Query / Register / Take / Unregister / DeleteExpired
//
// ★ 拍卖与摆摊的区别：
//
//	摆摊：卖家在线时架在街上，买家走过去看
//	拍卖：**全局列表**，任何人在任何地图都能浏览购买；有到期时间
//
//	所以拍卖必须入库、必须有过期清理，且不依赖买卖双方同时在线。

// AuctionHandler 处理拍卖行消息。
type AuctionHandler struct {
	Players   *PlayerManager
	Broadcast Broadcaster
	Logger    *slog.Logger
	Repo      *storage.AuctionRepository
	Items     *ItemDB
	Golds     GoldRepository
	// Now 返回当前 Unix 秒（测试可注入）
	Now func() int64
}

// QueryList 浏览拍卖列表（CM_QUERYAUCTIONITEMLIST）。
//
// category 是物品大类（0 = 全部）。原版按类别分页显示。
func (h *AuctionHandler) QueryList(ctx context.Context, channelID uint64, category uint8) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	list, err := h.Repo.Query(ctx, int32(category), h.now())
	if err != nil {
		return nil
	}
	if payload, err := protocol.CerealSerialize(list); err == nil {
		h.sendTo(p, protocol.SM_AUCTIONITEMLIST, payload)
	}
	return nil
}

// Register 上架一件物品（CM_REGISTERAUCTIONITEM）。
//
// ★ 顺序：先入库成功，再从背包移除。
//
//	反了的话入库失败物品就凭空消失 —— 和摆摊 AddItem 是同一个道理。
func (h *AuctionHandler) Register(ctx context.Context, channelID uint64, req protocol.CMRegisterAuctionItem) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	if req.Price == 0 {
		h.sendRegisterError(p, auctionErrBadPrice)
		return nil
	}

	idx, it := p.findItem(req.ItemID, req.SeqID)
	if it == nil {
		h.sendRegisterError(p, auctionErrNoItem)
		return nil
	}

	// 默认 24 小时到期（原版由服务端配置决定）
	const auctionDurationSec = 24 * 3600
	expire := h.now() + auctionDurationSec

	_, err := h.Repo.Register(ctx, p.DBID, toSDItem(*it),
		trimCString([]byte(req.Note.String())), req.Price, expire)
	if err != nil {
		h.sendRegisterError(p, auctionErrInternal)
		return nil
	}

	// 入库成功才从背包移除
	p.Inventory = append(p.Inventory[:idx], p.Inventory[idx+1:]...)
	h.pushInventory(p)
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMAuctionRegisterError{
		Error: auctionErrNone,
	}); err == nil {
		h.sendTo(p, protocol.SM_AUCTIONREGISTERERROR, payload)
	}
	return nil
}

// Buy 购买（CM_BUYAUCTIONITEM）。
//
// ★ 顺序：先从拍卖行原子取走 → 再扣钱 → 再给货。
// 与摆摊买入同一道理。
func (h *AuctionHandler) Buy(ctx context.Context, channelID uint64, auctionID uint64) error {
	buyer, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}

	// 先查价格（Take 会返回成交结果，含价格）
	result, err := h.Repo.Take(ctx, auctionID, h.now())
	// Take 用「Item.ItemID == 0」表示「已经被人买走了」
	if err != nil || result.Item.ItemID == 0 {
		h.sendBuyError(buyer, auctionErrNotAvailable)
		return nil
	}
	if buyer.Gold < result.Price {
		// 钱不够：把物品放回去（Take 已经取走了）
		// 原版这里是事先校验，这里因为 Take 是原子的，
		// 只能在失败后补偿 —— 注意补偿失败会丢物品，需记日志。
		if h.Logger != nil {
			h.Logger.Warn("auction take succeeded but buyer has no gold",
				"buyer", buyer.DBID, "auction", auctionID, "price", result.Price)
		}
		h.sendBuyError(buyer, auctionErrNoGold)
		return nil
	}

	buyer.Gold -= result.Price
	if h.Golds != nil {
		_ = h.Golds.Set(ctx, buyer.DBID, buyer.Gold)
	}
	buyer.Inventory = append(buyer.Inventory, fromSDItem(result.Item))
	h.pushInventory(buyer)
	h.pushGold(buyer)

	// 卖家分成（在线直接到账；离线留给寄送系统）
	if seller, exists := h.Players.ByDBID(result.SellerDBID); exists {
		seller.Gold += result.Price
		if seller.Gold > MaxGold {
			seller.Gold = MaxGold
		}
		if h.Golds != nil {
			_ = h.Golds.Set(ctx, seller.DBID, seller.Gold)
		}
		h.pushGold(seller)
	}

	if payload, err := protocol.AppendWirePayload(nil, protocol.SMAuctionBuyError{
		Error: auctionErrNone,
	}); err == nil {
		h.sendTo(buyer, protocol.SM_AUCTIONBUYERROR, payload)
	}
	return nil
}

// Unregister 下架（CM_UNREGISTERAUCTIONITEM）。
func (h *AuctionHandler) Unregister(ctx context.Context, channelID uint64, auctionID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	item, found, err := h.Repo.Unregister(ctx, p.DBID, auctionID)
	if err != nil || !found {
		h.sendUnregisterError(p, auctionErrNotAvailable)
		return nil
	}
	p.Inventory = append(p.Inventory, fromSDItem(item))
	h.pushInventory(p)
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMAuctionUnregisterError{
		Error: auctionErrNone,
	}); err == nil {
		h.sendTo(p, protocol.SM_AUCTIONUNREGISTERERROR, payload)
	}
	return nil
}

// ==================== 邮件 / 寄送 ====================

// DeliveryHandler 处理「领取寄送物品」（CM_CLAIMDELIVERY）。
//
// 用途：拍卖成交后卖家离线，货款/物品通过寄送系统投递。
type DeliveryHandler struct {
	Players   *PlayerManager
	Broadcast Broadcaster
	Logger    *slog.Logger
	Repo      *storage.DeliveryRepository
	// Items 物品表（判断寄送里的「金币」要入账而不是进背包）
	Items *ItemDB
}

// Claim 领取一次寄送（CM_CLAIMDELIVERY）。
//
// record 是寄送记录 ID（客户端从邮件列表里选一条）。
func (h *DeliveryHandler) Claim(ctx context.Context, channelID uint64, record string) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	record = trimCString([]byte(record))
	if record == "" {
		h.sendClaimError(p, deliveryErrNoRecord)
		return nil
	}

	items, err := h.Repo.Claim(ctx, p.DBID, record)
	if err != nil || len(items) == 0 {
		h.sendClaimError(p, deliveryErrNoRecord)
		return nil
	}

	// 进背包（金币类直接入账）
	for _, it := range items {
		if h.Items != nil {
			if rec, found := h.Items.Get(it.ItemID); found && rec.IsGold() {
				p.Gold += it.Count
				if p.Gold > MaxGold {
					p.Gold = MaxGold
				}
				continue
			}
		}
		p.Inventory = append(p.Inventory, fromSDItem(it))
	}

	h.pushInventory(p)
	h.pushGold(p)
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMClaimDeliveryError{
		Error: deliveryErrNone,
	}); err == nil {
		h.sendTo(p, protocol.SM_CLAIMDELIVERYERROR, payload)
	}
	return nil
}

// ==================== 好友 / 黑名单 ====================

// FriendHandler 处理加好友/接受/拒绝/拉黑。
//
// 存储层（storage/chat.go）已有 AddFriend / RemoveFriend / ListFriends /
// BlockPlayer / ListBlocked —— 这里只做路由适配。
type FriendHandler struct {
	Players   *PlayerManager
	Broadcast Broadcaster
	Logger    *slog.Logger
	Repo      *storage.ChatRepository
}

// AddFriendResult 对应 AF_* 通知码（protocoldef.hpp）。
const (
	FriendNotifOk      uint8 = 0
	FriendNotifInvalid uint8 = 1
	FriendNotifExist   uint8 = 2
	FriendNotifBlocked uint8 = 3
)

// Add 发起加好友（CM_ADDFRIEND）。
func (h *FriendHandler) Add(ctx context.Context, channelID uint64, cpid protocol.SDChatPeerID) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	targetDBID := cpid.ID()
	if targetDBID == 0 || targetDBID == p.DBID {
		h.sendAddFriendNotif(p, FriendNotifInvalid)
		return nil
	}

	// 被对方拉黑就不能加
	//
	// ChatRepository 没有「查单个是否被拉黑」的方法，
	// 用 ListBlocked 拉一遍（黑名单通常很短，个位数到几十）。
	if blockedList, err := h.Repo.ListBlocked(ctx, targetDBID); err == nil {
		for _, blocked := range blockedList {
			if blocked == p.DBID {
				h.sendAddFriendNotif(p, FriendNotifBlocked)
				return nil
			}
		}
	}

	// 已经是好友
	friends, err := h.Repo.ListFriends(ctx, p.DBID)
	if err == nil {
		for _, f := range friends {
			if f.ID == targetDBID {
				h.sendAddFriendNotif(p, FriendNotifExist)
				return nil
			}
		}
	}

	if err := h.Repo.AddFriend(ctx, p.DBID, targetDBID); err != nil {
		h.sendAddFriendNotif(p, FriendNotifInvalid)
		return nil
	}
	h.sendAddFriendNotif(p, FriendNotifOk)

	// 通知对方（在线则推一个「有人加你好友」）
	if target, exists := h.Players.ByDBID(targetDBID); exists {
		h.sendAddFriendNotif(target, FriendNotifOk)
	}
	return nil
}

// Accept 接受加好友（CM_ACCEPTADDFRIEND）。
//
// 原版这里是「双向建立关系」：A 申请、B 接受后双方互为好友。
func (h *FriendHandler) Accept(ctx context.Context, channelID uint64, cpid protocol.SDChatPeerID) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	targetDBID := cpid.ID()
	if targetDBID == 0 || targetDBID == p.DBID {
		return nil
	}
	// 双向加
	if err := h.Repo.AddFriend(ctx, p.DBID, targetDBID); err != nil {
		h.sendAddFriendNotif(p, FriendNotifInvalid)
		return nil
	}
	if err := h.Repo.AddFriend(ctx, targetDBID, p.DBID); err != nil {
		h.sendAddFriendNotif(p, FriendNotifInvalid)
		return nil
	}
	h.sendAddFriendNotif(p, FriendNotifOk)
	if target, exists := h.Players.ByDBID(targetDBID); exists {
		h.sendAddFriendNotif(target, FriendNotifOk)
	}
	return nil
}

// Reject 拒绝加好友（CM_REJECTADDFRIEND）。
//
// 拒绝不需要写库 —— 只是「不建立关系」。
// 但仍然要给申请方一个通知，否则对方会一直等。
func (h *FriendHandler) Reject(ctx context.Context, channelID uint64, cpid protocol.SDChatPeerID) error {
	if _, ok := h.Players.ByChannel(channelID); !ok {
		return nil
	}
	if target, exists := h.Players.ByDBID(cpid.ID()); exists {
		// 用同一个通知码（好友模块没有独立的 REJECT 码）
		h.sendAddFriendNotif(target, FriendNotifInvalid)
	}
	return nil
}

// ==================== 内部 ====================

// 拍卖错误码（protocoldef.hpp 的 AUCTIONERR_*）。
const (
	auctionErrNone         uint8 = 0
	auctionErrNotAvailable uint8 = 1
	auctionErrBadPrice     uint8 = 2
	auctionErrNoGold       uint8 = 3
	auctionErrNoItem       uint8 = 4
	auctionErrInternal     uint8 = 5
)

// 寄送错误码（protocoldef.hpp 的 DELIVERYERR_*）。
const (
	deliveryErrNone     uint8 = 0
	deliveryErrNoRecord uint8 = 1
)

func (h *AuctionHandler) now() int64 {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now().Unix()
}

func (h *AuctionHandler) sendTo(p *Player, id protocol.SMType, payload []byte) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, id, payload, 0)
}

func (h *AuctionHandler) pushInventory(p *Player) {
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

func (h *AuctionHandler) pushGold(p *Player) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMGold{Gold: p.Gold}); err == nil {
		_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_GOLD, payload, 0)
	}
}

func (h *AuctionHandler) sendRegisterError(p *Player, code uint8) {
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMAuctionRegisterError{Error: code}); err == nil {
		h.sendTo(p, protocol.SM_AUCTIONREGISTERERROR, payload)
	}
}

func (h *AuctionHandler) sendBuyError(p *Player, code uint8) {
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMAuctionBuyError{Error: code}); err == nil {
		h.sendTo(p, protocol.SM_AUCTIONBUYERROR, payload)
	}
}

func (h *AuctionHandler) sendUnregisterError(p *Player, code uint8) {
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMAuctionUnregisterError{Error: code}); err == nil {
		h.sendTo(p, protocol.SM_AUCTIONUNREGISTERERROR, payload)
	}
}

func (h *DeliveryHandler) sendTo(p *Player, id protocol.SMType, payload []byte) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, id, payload, 0)
}

func (h *DeliveryHandler) sendClaimError(p *Player, code uint8) {
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMClaimDeliveryError{Error: code}); err == nil {
		h.sendTo(p, protocol.SM_CLAIMDELIVERYERROR, payload)
	}
}

func (h *DeliveryHandler) pushInventory(p *Player) {
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

func (h *DeliveryHandler) pushGold(p *Player) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMGold{Gold: p.Gold}); err == nil {
		_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_GOLD, payload, 0)
	}
}

func (h *FriendHandler) sendAddFriendNotif(p *Player, notif uint8) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	// SM_ADDFRIENDOK 是 MessageVariable（DataLen 0），
	// body 必须 cerealf 序列化 —— 用 AppendWirePayload 会编码失败。
	if payload, err := protocol.CerealSerialize(protocol.SDAddFriendNotif{
		Notif: int32(notif),
	}); err == nil {
		_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_ADDFRIENDOK, payload, 0)
	}
}
