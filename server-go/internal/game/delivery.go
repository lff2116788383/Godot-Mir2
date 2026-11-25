package game

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// ==================== 投递（寄送）系统 ====================
//
// 【依据】server/src/deliverydb.cpp 108 行 + player.cpp:3331 sendDelivery
//
// ★★ 最重要的一件事：原版**没有「邮箱列表」协议**。★★
//
// 很多人（包括本仓库早期的审计）会以为缺一个 `SM_DELIVERYLIST`，
// 于是去补协议表。实际上原版的做法是：
//
//	dbCreateDeliveryInTransaction() 在同一个事务里做两件事：
//	  1. insert into tbl_delivery(...)               ← 物品存这儿
//	  2. dbSaveChatMessage(SYS_CHATDBID_SYSTEM, 玩家) ← 聊天消息存这儿
//	消息正文是一段 XML，里面带
//	  <par><event id="SYS_DELIVERY" record="...">收取</event></par>
//
// 于是「邮箱」就是**系统助手的聊天记录**，玩家在聊天窗口里
// 看到「你收到了一份系统投递：/ 金币：10000 / [收取]」，
// 点「收取」时客户端把 `record` 原样塞进 CM_CLAIMDELIVERY。
//
// 这条设计有一个很重要的副产品：**离线可收**。
// 投递写的是 DB，玩家下次登录时聊天记录会随
// SM_CHATMESSAGELIST 一起推下来（见 chat.go 的登录补推）。
// 如果做成「在线才推」，离线玩家拍卖卖出去的东西就永远收不到了。

// 投递相关的系统事件名（sysconst.hpp:208-209）。
const (
	// SysEventGuildInvite 是公会邀请的 event id（SYS_GIRESP）。
	SysEventGuildInvite = "_RSVD_NAME_GIRESP_2961748350172"
	// SysEventDelivery 是投递「收取」按钮的 event id（SYS_DELIVERY）。
	SysEventDelivery = "_RSVD_NAME_DELIVERY_5639021748316"
)

// SysChatDBID 是「系统助手」的 peer id（sysconst.hpp 的 SYS_CHATDBID_SYSTEM）。
const SysChatDBID uint32 = 0xFFFFFF01

// DeliveryService 是投递的唯一创建入口。
//
// ★ 为什么要有这一层而不是各处直接调 Repo.Create：
//
//	原版的 dbCreateDelivery 是「建投递 + 写聊天消息」的**原子操作**。
//	如果分开写，任何一处忘了写聊天消息，玩家就永远看不到这个投递
//	（tbl_delivery 里有记录，但没有任何 UI 入口能拿到 record）。
//	把两件事锁在一个函数里，调用方就不可能漏。
type DeliveryService struct {
	Repo  *storage.DeliveryRepository
	Chat  ChatRepository
	Items *ItemDB
	// Push 把聊天消息即时推给在线的收件人（离线玩家下次登录自然能看到）
	Players   *PlayerManager
	Broadcast Broadcaster
	Logger    *slog.Logger
}

// NewDeliveryService 构造投递服务。
func NewDeliveryService(repo *storage.DeliveryRepository, chat ChatRepository,
	items *ItemDB, players *PlayerManager, broadcast Broadcaster, logger *slog.Logger) *DeliveryService {
	return &DeliveryService{
		Repo: repo, Chat: chat, Items: items,
		Players: players, Broadcast: broadcast, Logger: logger,
	}
}

// Deliver 创建一份投递并发聊天消息（对应 dbCreateDelivery）。
//
// title 是投递说明，例如「你购买的寄售物品已送达：」。
// 返回投递记录号（客户端点「收取」时要带回来）。
func (s *DeliveryService) Deliver(ctx context.Context, recipientDBID uint32,
	items []protocol.SDItem, title string) (string, error) {

	if s.Repo == nil {
		return "", fmt.Errorf("delivery repository not configured")
	}
	if len(items) == 0 {
		return "", storage.ErrDeliveryEmpty
	}
	if title == "" {
		title = "你收到了一份系统投递："
	}

	// ★ 顺序：先建投递拿到 record，再写聊天消息并把 messageID 回填。
	//
	//	原版是一个事务（dbCreateDeliveryInTransaction + 回填 fld_messageid）。
	//	这里没有跨仓储事务，所以拆成「先建、后写消息、最后 update」：
	//	最坏情况是投递存在但没聊天消息 —— 玩家看不到，但物品没丢
	//	（可以用 ListUnclaimed 补推）。反过来「消息在、投递不在」
	//	会让玩家点「收取」报错，更糟。
	delivery, err := s.Repo.Create(ctx, recipientDBID, items, nil)
	if err != nil {
		return "", fmt.Errorf("create delivery: %w", err)
	}

	body := buildDeliveryXML(title, s.Items, items, delivery.Record)

	if s.Chat != nil {
		from := protocol.NewSDChatPeerID(protocol.CPR_SPECIAL, SysChatDBID)
		to := protocol.NewSDChatPeerID(protocol.CPR_PLAYER, recipientDBID)
		seq, err := s.Chat.SaveMessage(ctx, from, to, body, nil)
		if err == nil {
			// 回填 fld_messageid（对应原版的 update ... returning）
			if bindErr := s.Repo.BindMessage(ctx, delivery.Record, seq.ID); bindErr != nil && s.Logger != nil {
				s.Logger.Warn("bind delivery message failed",
					"record", delivery.Record, "error", bindErr)
			}
			s.pushToOnline(recipientDBID, seq, from, to, body)
		} else if s.Logger != nil {
			// 投递已经建好，只是没聊天消息 —— 不返回错误，
			// 因为物品已经安全落地，玩家可以靠 ListUnclaimed 补推拿到
			s.Logger.Warn("save delivery chat message failed",
				"record", delivery.Record, "error", err)
		}
	}

	return delivery.Record, nil
}

// DeliverGold 投递金币（走「金币（超）」等面额拆分）。
//
// ★ 不能直接把金币塞成背包物品：
//
//	原版寄送里的金币是 itemID = 金币 的 SDItem，Claim 时判断
//	isGold() 直接加到 Gold 而不进背包。否则「1 万金币」会变成
//	占 6 个背包格的物品（buildGoldItem 会把 10000 拆成 5000+5000）。
func (s *DeliveryService) DeliverGold(ctx context.Context, recipientDBID uint32,
	count uint64, title string) (string, error) {
	items := BuildGoldItems(s.Items, count)
	if len(items) == 0 {
		return "", fmt.Errorf("gold count must be positive")
	}
	return s.Deliver(ctx, recipientDBID, items, title)
}

// pushToOnline 把聊天消息即时推给在线玩家。
//
// 离线玩家不需要处理：消息已经入库，下次登录会随
// RetrieveLatest 一起拉下来（那就是原版的「邮箱」）。
func (s *DeliveryService) pushToOnline(dbid uint32, seq protocol.SDChatMessageDBSeq,
	from, to protocol.SDChatPeerID, body string) {
	if s.Players == nil || s.Broadcast == nil {
		return
	}
	target, ok := s.Players.ByDBID(dbid)
	if !ok || target.ChannelID == 0 {
		return
	}
	list := protocol.SDChatMessageList{{
		HasSeq:  true,
		Seq:     seq,
		From:    from,
		To:      to,
		Message: body,
	}}
	payload, err := protocol.CerealSerialize(list)
	if err != nil {
		return
	}
	_ = s.Broadcast.SendTo(target.ChannelID, protocol.SM_CHATMESSAGELIST, payload, 0)
}

// buildDeliveryXML 拼投递的聊天正文（deliverydb.cpp:47-60）。
//
// 输出形如：
//
//	<layout>
//	    <par>你购买的寄售物品已送达：</par>
//	    <par>屠龙：1</par>
//	    <par>金币：10000</par>
//	    <par><event id="SYS_DELIVERY" record="abc...">收取</event></par>
//	</layout>
//
// ★ 物品按 itemID 汇总后再输出（原版用 std::map 累加）：
//
//	同一件物品分两格寄（比如两把不同持久的屠龙）时，
//	原版显示的是「屠龙：2」而不是两行「屠龙：1」。
//	这里用有序切片模拟 map 的稳定输出顺序。
func buildDeliveryXML(title string, items *ItemDB, itemList []protocol.SDItem, record string) string {
	var b strings.Builder
	b.WriteString("<layout>")
	b.WriteString(xmlPar(title))

	// 按 itemID 汇总，保持首次出现顺序（原版 std::map 按 key 排序，
	// 但 itemID 顺序对玩家无意义，稳定的首次出现顺序更好读）
	type summary struct {
		itemID uint32
		count  uint64
	}
	var order []summary
	index := make(map[uint32]int)
	for _, it := range itemList {
		if at, ok := index[it.ItemID]; ok {
			order[at].count += it.Count
			continue
		}
		index[it.ItemID] = len(order)
		order = append(order, summary{itemID: it.ItemID, count: it.Count})
	}
	for _, s := range order {
		name := fmt.Sprintf("物品%d", s.itemID)
		isGold := false
		if items != nil {
			if rec, found := items.Get(s.itemID); found {
				if rec.IsGold() {
					isGold = true
					name = "金币"
				} else {
					name = rec.Name
				}
			}
		}
		_ = isGold
		b.WriteString(xmlPar(fmt.Sprintf("%s：%d", name, s.count)))
	}

	b.WriteString(fmt.Sprintf(`<par><event id="%s" record="%s">收取</event></par>`,
		SysEventDelivery, escapeXMLAttr(record)))
	b.WriteString("</layout>")
	return b.String()
}

// xmlPar 生成一个 <par> 段落。
func xmlPar(text string) string {
	return "<par>" + escapeXMLText(text) + "</par>"
}

// escapeXMLText 转义 XML 文本节点里的特殊字符。
//
// ★ 物品名里可能有 & 或 <（虽然当前表里没有，但脚本能塞任意名字），
// 不转义会让整段 XML 解析失败 —— 客户端会直接不显示这条消息。
func escapeXMLText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// escapeXMLAttr 转义 XML 属性值（record 是 hex，实际不会触发，防御性处理）。
func escapeXMLAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// decodeDeliveryPayload 解码投递里的物品列表。
//
// 投递的 payload 是 storage 层用 SDShowSecuredItemList 包的一层
// Cerealf blob（deliveries.go encodeDeliveryItems）。这里要用同样的容器解。
func decodeDeliveryPayload(blob []byte) ([]protocol.SDItem, error) {
	var value protocol.SDShowSecuredItemList
	if err := protocol.CerealDeserialize(blob, &value); err != nil {
		return nil, err
	}
	return value.ItemList, nil
}

// BuildGoldItems 把金币数额拆成物品表里的金币面额（sditem.cpp:199 buildGoldItem）。
//
// ★ 面额阈值来自原版，注意是 **< 5000 用「超」**：
//
//	<10 小 / <100 中 / <500 大 / <2000 特 / <5000 超 / >=5000 拆成多个 5000 超
//
//	所以 5000 会变成一个「金币（超）」count=5000，
//	10000 会变成两个「金币（超）」count=5000 —— 这也是为什么
//	投递金币不能直接进背包（会占多格）。
func BuildGoldItems(items *ItemDB, count uint64) []protocol.SDItem {
	if count == 0 || items == nil {
		return nil
	}
	pick := func(name string) (uint32, bool) {
		id := items.IDOfName(name)
		return id, id != 0
	}
	var result []protocol.SDItem
	for count > 0 {
		var name string
		var chunk uint64
		switch {
		case count < 10:
			name, chunk = "金币（小）", count
		case count < 100:
			name, chunk = "金币（中）", count
		case count < 500:
			name, chunk = "金币（大）", count
		case count < 2000:
			name, chunk = "金币（特）", count
		case count < 5000:
			name, chunk = "金币（超）", count
		default:
			name, chunk = "金币（超）", 5000
		}
		id, ok := pick(name)
		if !ok {
			// 面额名不在表里时退化成最小面额，至少能投递出去
			id = GoldItemID
		}
		result = append(result, protocol.SDItem{ItemID: id, Count: chunk})
		count -= chunk
	}
	return result
}
