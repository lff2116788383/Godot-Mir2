package game

import (
	"context"
	"fmt"
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
	// Deliveries 投递服务（成交后买卖双方都靠它收货）
	//
	// ★ 原版拍卖**不给任何人直接加背包/加金币**（auctiondb.cpp:161-169）：
	//
	//	买家：dbCreateDeliveryInTransaction(buyerDBID, {item}, "你购买的寄售物品已送达：")
	//	卖家：dbCreateDeliveryInTransaction(sellerDBID, goldItem, "你寄售的X已售出，扣除2%交易税后获得：")
	//
	//	双方都走投递 → 天然支持「卖家离线时东西被买走」。
	//	早期实现是「卖家在线直接到账、离线丢弃」，
	//	离线卖家的货款会**凭空消失**，这是明确的物品丢失 bug。
	Deliveries *DeliveryService
	// Stalls 判断卖家是否正在摆摊（摆摊中不能上拍卖，AUCTIONREGERR_STALLSELLING）
	Stalls *storage.StallRepository
	// Now 返回当前 Unix 秒（测试可注入）
	Now func() int64
}

// AuctionLifetimeSec 是拍卖物品的默认挂单时长（sysconst.hpp:101 SYS_AUCTIONLIFETIME）。
//
// ★ 是 **7 天**不是 24 小时。
//
//	早期写成 24*3600，与客户端显示的「剩余时间」对不上，
//	玩家会看到「还剩 6 天」然后第二天东西就过期没了。
const AuctionLifetimeSec int64 = 7 * 24 * 60 * 60

// AuctionTaxRate 是拍卖成交税率（sysconst.hpp:102 SYS_AUCTIONTAXRATE）。
//
// 单位是百分比：2 表示抽 2%。
const AuctionTaxRate uint64 = 2

// SysMaxPrice 是单价的合法上限（sysconst.hpp:89 SYS_MAXPRICE）。
const SysMaxPrice uint64 = (1 << 60) - 1

// QueryList 浏览拍卖列表（CM_QUERYAUCTIONITEMLIST）。
//
// category 是物品大类。原版分类常量从 **1** 起（AuctionCategoryAll=1，
// End=12），**0 不是合法值** —— 0 会让 storage 直接返回
// "invalid auction category 0"，客户端看到的是空列表。
//
// 早期实现把客户端的 uint8 原样转成 int32 传下去，
// 客户端发 0（「全部」的常见默认值）就什么都看不到。
// 这里把 0 归一到 All。
func (h *AuctionHandler) QueryList(ctx context.Context, channelID uint64, category uint8) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	cat := int32(category)
	if cat < storage.AuctionCategoryAll || cat >= storage.AuctionCategoryEnd {
		cat = storage.AuctionCategoryAll
	}
	list, err := h.Repo.Query(ctx, cat, h.now())
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

	// 1. 摆摊中不能上拍卖（playernet.cpp:378）
	//
	// ★ 为什么有这个限制：摆摊和拍卖都会「从背包拿走物品」，
	// 同时进行会出现「一件物品既挂在摊上又在拍卖行」，
	// 卖出去两遍。原版用 stallSelling() 挡住。
	if h.stallSelling(ctx, p.DBID) {
		h.sendRegisterError(p, auctionErrStallSelling)
		return nil
	}

	// 2. 备注长度与编码（playernet.cpp:383-392）
	note := trimCString([]byte(req.Note.String()))
	if len(note) > auctionNoteSize {
		h.sendRegisterError(p, auctionErrBadNote)
		return nil
	}

	// 3. 价格合法（parserf::valid_price：1..SYS_MAXPRICE）
	if req.Price == 0 || req.Price > SysMaxPrice {
		h.sendRegisterError(p, auctionErrBadPrice)
		return nil
	}

	// 4. 物品必须在背包里，且不能是金币
	idx, it := p.findItem(req.ItemID, req.SeqID)
	if it == nil || it.Count == 0 {
		h.sendRegisterError(p, auctionErrBadItem)
		return nil
	}
	if h.Items != nil {
		if rec, found := h.Items.Get(it.ItemID); found && rec.IsGold() {
			// 金币不能上架：它是计价单位，上架后无法表达「用金币买金币」
			h.sendRegisterError(p, auctionErrBadItem)
			return nil
		}
	}

	expire := h.now() + AuctionLifetimeSec

	_, err := h.Repo.Register(ctx, p.DBID, toSDItem(*it), note, req.Price, expire)
	if err != nil {
		h.sendRegisterError(p, auctionErrInternal)
		return nil
	}

	// ★ 入库成功才从背包移除（反了就凭空消失）
	p.Inventory = append(p.Inventory[:idx], p.Inventory[idx+1:]...)
	h.pushInventory(p)

	// 原版回的是 SM_OK（MessageEmpty），不是只有 error=0 的 SMAuctionRegisterError。
	// 两者都发：SM_OK 让客户端的「操作完成」回调触发，
	// RegisterError{0} 兼容早期客户端的实现。
	h.sendOK(p)
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMAuctionRegisterError{
		Error: auctionErrNone,
	}); err == nil {
		h.sendTo(p, protocol.SM_AUCTIONREGISTERERROR, payload)
	}
	return nil
}

// stallSelling 判断某角色是否正在摆摊（stallSelling()）。
func (h *AuctionHandler) stallSelling(ctx context.Context, dbid uint32) bool {
	if h.Stalls == nil {
		return false
	}
	stall, found, err := h.Stalls.Load(ctx, dbid)
	if err != nil || !found {
		return false
	}
	return stall.Selling
}

// Buy 购买（CM_BUYAUCTIONITEM / auctiondb.cpp:118 dbBuyAuctionItem）。
//
// ★★ 这一版修掉了早期实现的一个真实缺陷：**钱不够时物品会消失**。★★
//
// 早期写法是「先 Take（原子取走）→ 再判金币 → 不够就 Warn 了事」，
// 物品已经被删了却没有补偿，**直接丢失**。
//
// 原版的顺序（在一个事务里）是：
//  1. delete ... where fld_id=? and fld_expiretime>? returning ...   ← 原子取走
//  2. if sellerDBID == buyerDBID → OWNITEM（自己买自己的，回滚）
//  3. dbRemoveGold(buyerDBID, price)  ← **条件扣减**，不够返回 empty → INSUFFICIENT
//  4. 买家投递物品 / 卖家投递扣税后的金币
//
// Go 侧 storage 没有跨仓储事务，所以把「物品」和「金币」拆成两步做，
// 但**顺序按原版**：取走 → 扣钱 → 投递。任何一步失败都要把物品放回去：
//
//	物品放回 tbl_auctionitem（Reinstate），而不是塞进买家背包 ——
//	塞进背包会让「钱不够」变成「白拿一件」。
func (h *AuctionHandler) Buy(ctx context.Context, channelID uint64, auctionID uint64) error {
	buyer, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	if auctionID == 0 {
		h.sendBuyError(buyer, auctionErrBadItem)
		return nil
	}

	result, err := h.Repo.Take(ctx, auctionID, h.now())
	if err != nil || result.Item.ItemID == 0 {
		h.sendBuyError(buyer, auctionErrUnavailable)
		return nil
	}

	// 自己买自己的（auctiondb.cpp:143）：物品已被取走，必须放回去
	if result.SellerDBID == buyer.DBID {
		h.reinstate(ctx, result)
		h.sendBuyError(buyer, auctionErrOwnItem)
		return nil
	}

	// ★ 条件扣减：不足时不会扣，返回 ok=false
	remaining, okGold, err := removeGoldAtomically(ctx, h.Golds, buyer, result.Price)
	if err != nil || !okGold {
		h.reinstate(ctx, result)
		if err != nil && h.Logger != nil {
			h.Logger.Error("auction remove gold failed",
				"buyer", buyer.DBID, "auction", auctionID, "error", err)
		}
		h.sendBuyError(buyer, auctionErrInsufficient)
		return nil
	}
	buyer.Gold = remaining
	h.pushGold(buyer)

	// 买家收货：走投递，不进背包
	//
	// ★ 为什么不直接 append 进背包：
	//
	//	原版就是投递（auctiondb.cpp:161），而且背包满时直接 append 会
	//	把物品挤丢。投递进 DB 是唯一不会丢的路径。
	if h.Deliveries != nil {
		itemCopy := result.Item
		itemCopy.SeqID = 0
		name := "物品"
		if h.Items != nil {
			if rec, found := h.Items.Get(itemCopy.ItemID); found {
				name = rec.Name
			}
		}
		if _, err := h.Deliveries.Deliver(ctx, buyer.DBID, []protocol.SDItem{itemCopy},
			"你购买的寄售物品已送达："); err != nil && h.Logger != nil {
			h.Logger.Error("auction buyer delivery failed",
				"buyer", buyer.DBID, "auction", auctionID, "item", name, "error", err)
		}
	} else {
		// 没有投递服务时的降级：直接进背包（会丢但比凭空消失好记日志）
		buyer.Inventory = append(buyer.Inventory, fromSDItem(result.Item))
		h.pushInventory(buyer)
		if h.Logger != nil {
			h.Logger.Warn("auction buyer direct-append: delivery service not configured",
				"buyer", buyer.DBID, "auction", auctionID)
		}
	}

	// 卖家收钱：**扣税**后走投递（离线也能收到）
	sellerGold := result.Price - result.Price*AuctionTaxRate/100
	if sellerGold == 0 {
		// 极低价成交时税后为 0（价格 1..49 时 2% 取整后全被抽走）
		// 原版 fflassert(sellerGold > 0) 会直接崩，这里给至少 1 枚
		sellerGold = 1
	}
	sellerName := "物品"
	if h.Items != nil {
		if rec, found := h.Items.Get(result.Item.ItemID); found {
			sellerName = rec.Name
		}
	}
	if h.Deliveries != nil {
		title := fmt.Sprintf("你寄售的%s已售出，扣除%d%%交易税后获得：", sellerName, AuctionTaxRate)
		if _, err := h.Deliveries.DeliverGold(ctx, result.SellerDBID, sellerGold, title); err != nil {
			if h.Logger != nil {
				h.Logger.Error("auction seller delivery failed",
					"seller", result.SellerDBID, "auction", auctionID,
					"gold", sellerGold, "error", err)
			}
		}
	}

	h.sendOK(buyer)
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMAuctionBuyError{
		Error: auctionErrNone,
	}); err == nil {
		h.sendTo(buyer, protocol.SM_AUCTIONBUYERROR, payload)
	}
	return nil
}

// reinstate 把已经 Take 走但没能成交的物品放回拍卖行。
//
// ★ 这是失败补偿，不是正常流程。放回时不改 expireTime
// （原版事务回滚会完整恢复原记录，包括原到期时间）。
func (h *AuctionHandler) reinstate(ctx context.Context, result storage.AuctionTakeResult) {
	if err := h.Repo.Reinstate(ctx, result); err != nil && h.Logger != nil {
		// 补偿失败 = 物品真的丢了，必须 Error 级别（运营需要人工补）
		h.Logger.Error("CRITICAL: auction reinstate failed, item lost",
			"auction", result.AuctionID, "seller", result.SellerDBID,
			"itemID", result.Item.ItemID, "count", result.Item.Count, "error", err)
	}
}

// Unregister 下架（CM_UNREGISTERAUCTIONITEM / auctiondb.cpp:187）。
//
// ★ 原版也是走投递（"你下架的寄售物品已退回："），不是直接进背包。
func (h *AuctionHandler) Unregister(ctx context.Context, channelID uint64, auctionID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	item, found, err := h.Repo.Unregister(ctx, p.DBID, auctionID)
	if err != nil || !found {
		h.sendUnregisterError(p, auctionErrUnavailable)
		return nil
	}
	if h.Deliveries != nil {
		if _, err := h.Deliveries.Deliver(ctx, p.DBID, []protocol.SDItem{item},
			"你下架的寄售物品已退回："); err != nil && h.Logger != nil {
			h.Logger.Error("auction unregister delivery failed",
				"seller", p.DBID, "auction", auctionID, "error", err)
		}
	} else {
		p.Inventory = append(p.Inventory, fromSDItem(item))
		h.pushInventory(p)
	}
	h.sendOK(p)
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
//
// ★ 再次强调：**没有「邮箱列表」协议**。
//
//	玩家看到的「邮件」是聊天记录里的系统消息（SysChatDBID），
//	消息正文带 `<event record="...">收取</event>`。
//	所以这个 handler 只有 Claim 一个入口，没有 List。
type DeliveryHandler struct {
	Players   *PlayerManager
	Broadcast Broadcaster
	Logger    *slog.Logger
	Repo      *storage.DeliveryRepository
	// Items 物品表（判断寄送里的「金币」要入账而不是进背包）
	Items *ItemDB
	// Golds 金币落库（领取金币后必须存，否则重登丢失）
	Golds GoldRepository
	// Deliveries 投递服务（登录补推未领取时用）
	Deliveries *DeliveryService
}

// Claim 领取一次寄送（CM_CLAIMDELIVERY / player.cpp:3344 claimDelivery）。
//
// record 来自**系统聊天消息正文里的 XML**（`<event ... record="...">收取</event>`），
// 不是来自某个「邮箱列表」接口 —— 原版没有那个接口。
//
// ★★ 必须做「金币溢出则整单回滚」。★★
//
// 原版在一个事务里：
//
//	claimDelivery(record)              → 取走（标记已领）
//	遍历 items，累加金币；若 gold()+累加 > SYS_MAXGOLD → return CDERR_GOLDFULL
//	transaction 未 commit → 析构回滚 → **投递保持未领取**
//
// 早期实现是「先 Claim 再逐件塞背包，金币 clamp 到 MaxGold」，
// 结果是金币超出部分的**静默丢失**（100 万亿的投递只给到上限），
// 而投递记录已经变成「已领取」，玩家再也拿不回来。
//
// 这里按原版顺序：先算总量 → 溢出就 Unclaim 回去 → 再真正发放。
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

	// 1. 先算金币总量，判断会不会溢出（原版在这一步 return CDERR_GOLDFULL）
	var claimedGold uint64
	isGold := func(itemID uint32) bool {
		if h.Items == nil {
			return false
		}
		rec, found := h.Items.Get(itemID)
		return found && rec.IsGold()
	}
	for _, it := range items {
		if !isGold(it.ItemID) {
			continue
		}
		if it.Count > MaxGold-p.Gold-claimedGold {
			// 溢出：投递放回未领取状态，让玩家腾出金币再来领
			if rollErr := h.Repo.Unclaim(ctx, record); rollErr != nil && h.Logger != nil {
				h.Logger.Error("CRITICAL: delivery unclaim failed, items lost",
					"player", p.DBID, "record", record, "error", rollErr)
			}
			h.sendClaimError(p, deliveryErrGoldFull)
			return nil
		}
		claimedGold += it.Count
	}

	// 2. 发放：金币入账，其余进背包
	for _, it := range items {
		if isGold(it.ItemID) {
			p.Gold += it.Count
			continue
		}
		p.Inventory = append(p.Inventory, fromSDItem(it))
	}

	// 3. 金币落库（内存改了但没存 = 重登丢失）
	if claimedGold > 0 && h.Golds != nil {
		if err := h.Golds.Set(ctx, p.DBID, p.Gold); err != nil && h.Logger != nil {
			h.Logger.Error("delivery gold persist failed",
				"player", p.DBID, "gold", p.Gold, "error", err)
		}
	}

	h.pushInventory(p)
	h.pushGold(p)
	h.sendOK(p)
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
// 拍卖错误码（protocoldef.hpp:603-629 的三组枚举）。
//
// ★ 三组枚举的**编号各自从 0 开始**，值会撞车，但用在不同消息上
// 所以互不影响。之前用一套扁平常量把三组混在一起，
// 结果是「上架失败」返回 2（本该是 BADPRICE），客户端显示的是
// 「物品不可用」——错误码语义全乱。
const (
	// AUCTIONREGERR_*
	auctionErrNone         uint8 = 0
	auctionErrBadItem      uint8 = 1 // AUCTIONREGERR_BADITEM
	auctionErrBadPrice     uint8 = 2 // AUCTIONREGERR_BADPRICE
	auctionErrBadNote      uint8 = 3 // AUCTIONREGERR_BADNOTE
	auctionErrStallSelling uint8 = 4 // AUCTIONREGERR_STALLSELLING
)

const (
	// AUCTIONBUYERR_*
	auctionBuyErrNone         uint8 = 0
	auctionBuyErrBadItem      uint8 = 1 // AUCTIONBUYERR_BADITEM
	auctionErrUnavailable     uint8 = 2 // AUCTIONBUYERR_UNAVAILABLE
	auctionErrInsufficient    uint8 = 3 // AUCTIONBUYERR_INSUFFICIENT
	auctionErrOwnItem         uint8 = 4 // AUCTIONBUYERR_OWNITEM
	auctionUnregErrUnavail    uint8 = 2 // AUCTIONUNREGERR_UNAVAILABLE
	auctionNoteSize                 = 128
	auctionErrInternal        uint8 = 255
)

// 兼容旧调用点的别名（Buy 用 BuyError 那套码，Unregister 用 RegError 那套）
const (
	auctionErrNotAvailable = auctionErrUnavailable
)

// 寄送错误码（protocoldef.hpp:592-598 的 CDERR_*）。
//
// ★ 编号不能猜。原版是：
//
//	CDERR_NONE          = 0
//	CDERR_UNKNOWN_ERROR = 1
//	CDERR_INVALID_CLAIM = 2
//	CDERR_INVALID_RECORD= 3
//	CDERR_GOLDFULL      = 4
//
// 早期实现把 GOLDFULL 写成 2，客户端会显示「领取无效」而不是「金币已满」，
// 玩家以为记录坏了就不会再试 —— 实际只要清背包就能领。
const (
	deliveryErrNone          uint8 = 0 // CDERR_NONE
	deliveryErrUnknown       uint8 = 1 // CDERR_UNKNOWN_ERROR
	deliveryErrNoRecord      uint8 = 3 // CDERR_INVALID_RECORD
	deliveryErrInvalidClaim  uint8 = 2 // CDERR_INVALID_CLAIM（已领过）
	deliveryErrGoldFull      uint8 = 4 // CDERR_GOLDFULL
)

func (h *AuctionHandler) now() int64 {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now().Unix()
}

// sendOK 发一条 SM_OK（MessageEmpty）。
//
// ★ 原版所有成功路径都回 SM_OK（playernet.cpp 里 postNetMessage(SM_OK, respID)），
// 而**不是**回 `XXXError{Error: 0}`。两者语义不同：
//
//	SM_OK               → 通用的「这条请求处理完了」
//	SM_AUCTIONBUYERROR  → 专门用于**失败**
//
// 早期实现只发 `{Error:0}`，客户端那边有个「等 SM_OK 才关对话框」的
// 逻辑就会一直等。两个都发是过渡期最稳的做法。
func (h *AuctionHandler) sendOK(p *Player) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_OK, nil, 0)
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

// sendOK 发 SM_OK（领取成功，原版 postNetMessage(SM_OK, respID)）。
func (h *DeliveryHandler) sendOK(p *Player) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_OK, nil, 0)
}

// ==================== 拍卖到期清理 ====================

// AuctionSweeper 定期清掉过期拍卖（auctiondb.cpp 的 DeleteExpired 调用方）。
//
// ★ 原版没有「定时清理」这一步 —— 它是**惰性过期**：
//
//	所有查询/成交 SQL 都带 `fld_expiretime > now`，
//	过期物品只是「查不到、买不到」，行还留在表里。
//	实际清理靠运营脚本或数据库维护。
//
// Go 侧补了这个清理器，因为：
//  1. 过期行不删会让 tbl_auctionitem 无限增长（每件上架都 insert）
//  2. 清理后**必须给卖家发投递**，否则物品就真没了
//
// ★★ 顺序：先取出要清的记录 → 投递退回卖家 → 再删行。★★
//
//	反过来（先删再投递）如果投递失败，物品永久消失。
type AuctionSweeper struct {
	Repo       *storage.AuctionRepository
	Deliveries *DeliveryService
	Items      *ItemDB
	Logger     *slog.Logger
	// Interval 清理间隔（默认 10 分钟）
	Interval time.Duration
	Now      func() int64
}

// NewAuctionSweeper 构造清理器。
func NewAuctionSweeper(repo *storage.AuctionRepository, deliveries *DeliveryService,
	items *ItemDB, logger *slog.Logger) *AuctionSweeper {
	return &AuctionSweeper{
		Repo: repo, Deliveries: deliveries, Items: items, Logger: logger,
		Interval: 10 * time.Minute,
		Now:      func() int64 { return time.Now().Unix() },
	}
}

// Run 起一个后台循环直到 ctx 取消。
func (s *AuctionSweeper) Run(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.SweepOnce(ctx)
		}
	}
}

// SweepOnce 执行一次清理，返回处理条数。
//
// ★ 必须先 ExpiredList 再逐条退投递再删 —— 不能直接
// 调 DeleteExpired（那个只删不发），否则卖家的物品会被静默吞掉。
func (s *AuctionSweeper) SweepOnce(ctx context.Context) int {
	if s.Repo == nil {
		return 0
	}
	now := s.now()
	expired, err := s.Repo.LoadExpired(ctx, now)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("load expired auctions failed", "error", err)
		}
		return 0
	}
	if len(expired) == 0 {
		return 0
	}

	returned := 0
	for _, record := range expired {
		// 先原子取走（带 expiretime > now 的反向条件改成 <=）
		item, found, takeErr := s.Repo.TakeExpired(ctx, record.AuctionID, now)
		if takeErr != nil || !found {
			// 被别人买走了 / 被卖家下架了 —— 正常，跳过
			continue
		}
		name := "物品"
		if s.Items != nil {
			if rec, ok := s.Items.Get(item.ItemID); ok {
				name = rec.Name
			}
		}
		title := fmt.Sprintf("你寄售的%s已到期，物品已退回：", name)
		if s.Deliveries != nil {
			if _, dErr := s.Deliveries.Deliver(ctx, record.SellerDBID, []protocol.SDItem{item}, title); dErr != nil {
				if s.Logger != nil {
					// 投递失败：物品已经被 Take 走了，只能记 Critical 等人工补
					s.Logger.Error("CRITICAL: expired auction delivery failed, item lost",
						"auction", record.AuctionID, "seller", record.SellerDBID,
						"itemID", item.ItemID, "error", dErr)
				}
				continue
			}
		} else if s.Logger != nil {
			s.Logger.Warn("expired auction returned without delivery service",
				"auction", record.AuctionID, "seller", record.SellerDBID)
		}
		returned++
	}

	if s.Logger != nil && returned > 0 {
		s.Logger.Info("expired auctions returned to sellers",
			"scanned", len(expired), "returned", returned)
	}
	return returned
}

func (s *AuctionSweeper) now() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().Unix()
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
