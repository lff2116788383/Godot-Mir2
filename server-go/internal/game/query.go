package game

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// ==================== 查询类与杂项路由（补齐剩余 CM）====================
//
// 【依据】playernet.cpp 的 net_CM_QUERY* / net_CM_REQUEST*
//
// 这一批消息的共同点：**都是客户端主动问、服务端答**，没有状态机。
// 之前没接不是因为难，是因为「玩家不点就不会暴露」——
// 但客户端（main.gd）已经全部实现了解析，所以缺一条就是一处功能空白：
//
//	CM_QUERYPLAYERNAME    查目标玩家名（鼠标悬停显示）
//	CM_QUERYNPCNAME       查 NPC 名
//	CM_QUERYPLAYERWLDESP  查目标外观（看别人的装备）
//	CM_QUERYGOLD          查金币（交易/摆摊窗口要显示对方钱）
//	CM_QUERYUIDBUFF       查某人身上的 buff（看敌人有没有盾）
//	CM_QUERYBASEMAPUID    把地图名换成 mapUID（脚本/客户端传送用）
//	CM_QUERYRANKING       排行榜
//	CM_REQUESTLATESTCHATMESSAGE  拉最近聊天记录（登录时刷新聊天窗口）
//	CM_QUERYCHATMESSAGE   按 id 查单条消息
//	CM_QUERYCHATPEERLIST  搜聊天对象
//	CM_CREATECHATGROUP    建群
//	CM_SETFOLLOWTEAMLEADER  组队跟随开关
//	CM_REQUESTSPACEMOVE   主动传送（回城卷轴等）
//	CM_BLOCKPLAYER        拉黑
//	CM_REQUESTLOGOUT      主动登出
//	CM_REQUESTCONSUMEBELT 腰带吃药
//	CM_SETRUNTIMECONFIG   保存运行时配置
//	CM_MAKEITEM           GM 造物

// QueryHandler 处理各类查询与杂项消息。
type QueryHandler struct {
	Players   *PlayerManager
	Worlds    *WorldService
	Broadcast Broadcaster
	Logger    *slog.Logger
	Items     *ItemDB
	// Chars 排行榜与名字查询要走库
	Chars *storage.CharacterRepository
	Chat  *storage.ChatRepository
	// ItemRepo 外观查询要读穿戴
	ItemRepo *storage.ItemRepository
	// Teams 组队跟随开关
	Teams *TeamManager
	// Durability 腰带吃药要复用「消耗物品」逻辑
	Inventory *InventoryHandler
	// Sessions 登出要踢会话
	Sessions *SessionManager
	// Guilds 查公会名（名字颜色按公会区分）
	Guilds *storage.GuildRepository
}

// ==================== 名字与外观查询 ====================

// QueryPlayerName 查玩家名字（CM_QUERYPLAYERNAME，playernet.cpp:510）。
//
// 客户端鼠标悬停在别人身上时发这条，用来显示头顶名字。
//
// ★ 注意是「按 UID 查」而不是「按 dbid」：
//
//	客户端只知道它看到的 UID（含 peer/seq），不知道 dbid。
//	所以要先从 UID 解出 dbid，再查。
func (h *QueryHandler) QueryPlayerName(ctx context.Context, channelID uint64, uid uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	if !common.IsPlayerUID(uid) {
		return nil
	}
	dbid, err := common.GetPlayerDBID(uid)
	if err != nil {
		return nil
	}

	// 优先用内存里的（在线玩家已有名字，省一次查库）
	var name string
	var nameColor uint32
	if target, online := h.Players.ByDBID(dbid); online {
		name = target.Name
		nameColor = h.nameColor(ctx, target)
	} else if h.Chars != nil {
		ch, err := h.Chars.GetByDBID(ctx, dbid)
		if err != nil {
			return nil
		}
		name = ch.Name
		nameColor = h.nameColorOf(ctx, dbid, ch.PKPoint)
	}
	if name == "" {
		return nil
	}

	payload, err := protocol.CerealSerialize(protocol.SDPlayerName{
		UID:       uid,
		Name:      name,
		NameColor: nameColor,
	})
	if err != nil {
		return nil
	}
	h.send(p, protocol.SM_PLAYERNAME, payload)
	return nil
}

// nameColor 按玩家状态算名字颜色（player.cpp:2984 nameColor）。
//
// 原版规则（protocoldef.hpp 的 NC_*）：
//
//	红名（PK 点 >= 200）→ 红
//	黄名（有 PK 点）    → 黄
//	普通                → 白
func (h *QueryHandler) nameColor(ctx context.Context, p *Player) uint32 {
	return h.nameColorOf(ctx, p.DBID, p.PKPoint)
}

func (h *QueryHandler) nameColorOf(ctx context.Context, dbid uint32, pkPoint int32) uint32 {
	// 颜色值是客户端约定的 RGB（protocoldef.hpp NameColor）
	switch {
	case pkPoint >= SysPKPointRedName:
		return NameColorRed
	case pkPoint > 0:
		return NameColorYellow
	default:
		return NameColorWhite
	}
}

// 名字颜色常量（protocoldef.hpp 的 NameColor 枚举值）。
const (
	NameColorWhite  uint32 = 0xFFFFFFFF // 普通
	NameColorYellow uint32 = 0xFFFFFF00 // 有 PK 点
	NameColorRed    uint32 = 0xFFFF0000 // 红名
)

// QueryNPCName 查 NPC 名字（CM_QUERYNPCNAME，playernet.cpp:501）。
func (h *QueryHandler) QueryNPCName(ctx context.Context, channelID uint64, uid uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Worlds == nil || h.Worlds.NPCs == nil {
		return nil
	}
	npc, found := h.Worlds.NPCs.Get(uid)
	if !found {
		return nil
	}
	payload, err := protocol.CerealSerialize(protocol.SDNPCName{
		UID:  uid,
		Name: npc.Def.Name,
	})
	if err != nil {
		return nil
	}
	h.send(p, protocol.SM_NPCNAME, payload)
	return nil
}

// QueryPlayerWLDesp 查目标外观（CM_QUERYPLAYERWLDESP，playernet.cpp:527）。
//
// 客户端用它画「别人身上的装备」——不查这个，看别人永远是裸体。
func (h *QueryHandler) QueryPlayerWLDesp(ctx context.Context, channelID uint64, uid uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || !common.IsPlayerUID(uid) {
		return nil
	}
	dbid, err := common.GetPlayerDBID(uid)
	if err != nil {
		return nil
	}

	desp := protocol.SDWLDesp{Wear: protocol.SDWear{}}
	if target, online := h.Players.ByDBID(dbid); online {
		// 在线：直接用内存里的穿戴
		for slot, it := range target.Wear {
			if it == nil || slot < 0 || slot >= len(desp.Wear.List) {
				continue
			}
			desp.Wear.List[int32(slot)] = toSDItem(*it)
		}
		desp.Hair = uint32(target.Hair)
		desp.HairColor = uint32(target.HairColor)
	} else if h.ItemRepo != nil {
		// 离线：读库（看离线玩家的角色面板时用）
		if wear, err := h.ItemRepo.LoadWear(ctx, dbid); err == nil {
			desp.Wear = wear
		}
	}

	payload, err := protocol.CerealSerialize(protocol.SDUIDWLDesp{
		UID:  uid,
		Desp: desp,
	})
	if err != nil {
		return nil
	}
	h.send(p, protocol.SM_PLAYERWLDESP, payload)
	return nil
}

// QueryGold 查金币（CM_QUERYGOLD，playernet.cpp:315）。
//
// ★ 只能查**自己**的金币。
//
//	原版 cmQG.uid 必须等于自己（否则返回 0），
//	因为「查别人有多少钱」在交易/摆摊场景里是信息泄露。
func (h *QueryHandler) QueryGold(ctx context.Context, channelID uint64, uid uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	// 别人一律回 0（与原版一致，不报错）
	gold := uint64(0)
	if uid == p.UID {
		gold = p.Gold
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMGold{Gold: gold})
	if err != nil {
		return nil
	}
	h.send(p, protocol.SM_GOLD, payload)
	return nil
}

// QueryUIDBuff 查某人身上的 buff（CM_QUERYUIDBUFF，playernet.cpp:474）。
func (h *QueryHandler) QueryUIDBuff(ctx context.Context, channelID uint64, uid uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	// 只支持查玩家（怪物/NPC 没有 buff 表）
	list := []uint32(nil)
	if common.IsPlayerUID(uid) {
		if dbid, err := common.GetPlayerDBID(uid); err == nil {
			if target, online := h.Players.ByDBID(dbid); online {
				list = target.Buffs
			}
		}
	}
	payload, err := protocol.CerealSerialize(protocol.SDBuffIDList{
		UID:    uid,
		IDList: list,
	})
	if err != nil {
		return nil
	}
	h.send(p, protocol.SM_BUFFIDLIST, payload)
	return nil
}

// QueryBaseMapUID 把地图 ID 换成 mapUID（CM_QUERYBASEMAPUID，playernet.cpp:299）。
//
// 客户端脚本与传送逻辑需要「地图名 → mapUID」。
func (h *QueryHandler) QueryBaseMapUID(ctx context.Context, channelID uint64, mapID uint32) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	mapUID, err := common.GetBaseMapUID(mapID, 0)
	if err != nil {
		return nil
	}
	// ★ 原版回的是通用的 SM_UID（playernet.cpp:308），不是专门的 mapUID 消息。
	//   协议表里没有 SM_MAPUID 这条 —— 客户端按 SM_UID 收。
	payload, err := protocol.AppendWirePayload(nil, protocol.SMUID{UID: mapUID})
	if err != nil {
		return nil
	}
	h.send(p, protocol.SM_UID, payload)
	return nil
}

// ==================== 排行榜 ====================

// QueryRanking 排行榜（CM_QUERYRANKING，playernet.cpp:1614）。
//
// 原版就是一条 SQL：按 fld_gold 或 fld_exp 倒序取前 100。
//
// ★ 两个榜的排序字段不同，不能统一按 exp：
//
//	RANKING_GOLD → order by fld_gold
//	其他         → order by fld_exp
//
//	按 exp 排金币榜会让「等级高但没钱」的人排前面。
func (h *QueryHandler) QueryRanking(ctx context.Context, channelID uint64, rankingType uint8) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Chars == nil {
		return nil
	}

	const rankingGold uint8 = 1 // RANKING_GOLD
	orderBy := "fld_exp"
	if rankingType == rankingGold {
		orderBy = "fld_gold"
	}

	entries, err := h.Chars.Ranking(ctx, orderBy, 100)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Warn("query ranking failed", "type", rankingType, "error", err)
		}
		return nil
	}

	// 转成协议结构（Level 由 exp 反算，原版 SYS_LEVEL 是纯函数）
	list := protocol.SDRankingList{
		Type:    rankingType,
		Entries: make([]protocol.SDRankingEntry, 0, len(entries)),
	}
	for _, e := range entries {
		list.Entries = append(list.Entries, protocol.SDRankingEntry{
			DBID:  e.DBID,
			Level: uint32(DisplayLevel(e.Exp)),
			Gold:  e.Gold,
			Name:  e.Name,
		})
	}

	payload, err := protocol.CerealSerialize(list)
	if err != nil {
		return nil
	}
	h.send(p, protocol.SM_RANKINGLIST, payload)
	return nil
}

// ==================== 聊天记录 ====================

// RequestLatestChatMessage 拉最近聊天（CM_REQUESTLATESTCHATMESSAGE，playernet.cpp:1604）。
//
// ★ 这条是「聊天窗口刷新的唯一来源」。
//
//	之前 storage 有 RetrieveLatest 实现，但**没有任何路由调用它**，
//	也没在登录时推 —— 结果是：玩家重登后聊天窗口完全空白，
//	之前收到的寄送通知、公会邀请、私聊全部看不到。
//
//	（前面补的「投递补推」只是把未领取的重新生成一遍，
//	 历史对话仍然要这条消息才能看到。）
func (h *QueryHandler) RequestLatestChatMessage(ctx context.Context, channelID uint64, req protocol.CMRequestLatestChatMessage) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Chat == nil {
		return nil
	}

	cpids := make([]uint64, 0, req.CPIDList.Len()+2)
	for _, cpid := range req.CPIDList.Data {
		cpids = append(cpids, cpid)
	}
	// 原版总会把「系统助手」和「自己」塞进去（player.cpp:2432）
	cpids = append(cpids,
		protocol.NewSDChatPeerID(protocol.CPR_SPECIAL, SysChatDBID).Data,
		protocol.NewSDChatPeerID(protocol.CPR_PLAYER, p.DBID).Data,
	)

	limit := int(req.LimitCount)
	if limit <= 0 {
		limit = 1
	}
	if limit > 100 {
		limit = 100 // 防客户端传大数把库拉爆
	}

	list, err := h.Chat.RetrieveLatest(ctx, p.DBID, cpids, limit, req.IncludeSend, req.IncludeRecv)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Warn("retrieve latest chat failed", "dbid", p.DBID, "error", err)
		}
		return nil
	}
	if payload, err := protocol.CerealSerialize(list); err == nil {
		h.send(p, protocol.SM_CHATMESSAGELIST, payload)
	}
	return nil
}

// QueryChatMessage 按 id 查单条消息（CM_QUERYCHATMESSAGE，playernet.cpp:565）。
func (h *QueryHandler) QueryChatMessage(ctx context.Context, channelID uint64, msgID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Chat == nil {
		return nil
	}
	msg, found, err := h.Chat.QueryMessage(ctx, msgID)
	if err != nil || !found {
		return nil
	}
	payload, err := protocol.CerealSerialize(msg)
	if err != nil {
		return nil
	}
	h.send(p, protocol.SM_QUERYCHATMESSAGEOK, payload)
	return nil
}

// QueryChatPeerList 搜聊天对象（CM_QUERYCHATPEERLIST，playernet.cpp:551）。
func (h *QueryHandler) QueryChatPeerList(ctx context.Context, channelID uint64, input string) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Chat == nil {
		return nil
	}
	// 玩家与群都搜（原版 includePlayer/includeGroup 默认都开）
	list, err := h.Chat.QueryPeerList(ctx, input, true, true)
	if err != nil {
		return nil
	}
	payload, err := protocol.CerealSerialize(list)
	if err != nil {
		return nil
	}
	h.send(p, protocol.SM_QUERYCHATPEERLISTOK, payload)
	return nil
}

// CreateChatGroup 建群（CM_CREATECHATGROUP，playernet.cpp:1665）。
func (h *QueryHandler) CreateChatGroup(ctx context.Context, channelID uint64, req protocol.CMCreateChatGroup) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Chat == nil {
		return nil
	}
	name := trimCString([]byte(req.Name.String()))
	if name == "" {
		return nil
	}
	memberIDs := make([]uint32, 0, req.List.Len())
	for _, id := range req.List.Data {
		if id != 0 {
			memberIDs = append(memberIDs, id)
		}
	}
	peer, err := h.Chat.CreateGroup(ctx, p.DBID, name, memberIDs)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Warn("create chat group failed", "name", name, "error", err)
		}
		return nil
	}
	if payload, err := protocol.CerealSerialize(peer); err == nil {
		h.send(p, protocol.SM_CREATECHATGROUPOK, payload)
	}
	return nil
}

// BlockPlayer 拉黑（CM_BLOCKPLAYER，playernet.cpp:858）。
func (h *QueryHandler) BlockPlayer(ctx context.Context, channelID uint64, cpid uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Chat == nil {
		return nil
	}
	target := protocol.SDChatPeerID{Data: cpid}
	if target.Type() != protocol.CPR_PLAYER {
		return nil
	}
	blockedDBID := target.ID()
	if blockedDBID == 0 || blockedDBID == p.DBID {
		return nil
	}
	if err := h.Chat.BlockPlayer(ctx, p.DBID, blockedDBID); err != nil {
		if h.Logger != nil {
			h.Logger.Warn("block player failed", "target", blockedDBID, "error", err)
		}
	}
	return nil
}

// ==================== 组队跟随 / 传送 / 登出 ====================

// SetFollowTeamLeader 组队跟随开关（CM_SETFOLLOWTEAMLEADER，playernet.cpp:1447）。
//
// ★ 这个开关只影响**客户端自动跑位**，服务端不需要做任何逻辑。
//
//	原版也是只把标志位存到玩家身上，由客户端读它决定
//	「队长走了我要不要自动跟」。所以这里只需要记住值。
func (h *QueryHandler) SetFollowTeamLeader(ctx context.Context, channelID uint64, follow uint8) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	p.FollowTeamLeader = follow != 0
	return nil
}

// RequestSpaceMove 主动传送（CM_REQUESTSPACEMOVE，playernet.cpp:91）。
//
// 用途：回城卷轴、NPC 传送、任务传送。
//
// ★ 必须校验目标格可走，否则会把玩家送进墙里。
//
//	原版是让地图 actor 找最近可走格（requestSpaceMove）。
//	这里复用 DeathHandler.findFreeGrid 的同类逻辑。
func (h *QueryHandler) RequestSpaceMove(ctx context.Context, channelID uint64, req protocol.CMRequestSpaceMove) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Worlds == nil {
		return nil
	}
	// 只允许在自己当前地图内传送（跨图要走 NPC 脚本/回城卷轴）
	if req.MapUID != p.MapUID {
		return nil
	}
	// h.Worlds 是 *WorldService，真正的传送在 world 包
	if h.Worlds.Worlds == nil {
		return nil
	}
	entity, okMove, err := h.Worlds.Worlds.Teleport(ctx, p.MapID, p.UID, int(req.X), int(req.Y))
	if err != nil || !okMove {
		return nil
	}
	p.SyncFromWorld(entity)
	return nil
}

// RequestLogout 主动登出（CM_REQUESTLOGOUT，playernet.cpp:184）。
//
// ★ 顺序：先回 SM_LOGOUTOK 再清会话。
//
//	反了的话客户端永远收不到回执，界面卡在「正在登出」。
func (h *QueryHandler) RequestLogout(ctx context.Context, channelID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	// 先确认收到（MessageEmpty）
	if h.Broadcast != nil && p.ChannelID != 0 {
		_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_LOGOUTOK, nil, 0)
	}
	if h.Worlds != nil {
		h.Worlds.Leave(ctx, channelID)
	}
	if h.Sessions != nil {
		h.Sessions.Drop(channelID)
	}
	return nil
}

// ==================== 腰带 / 配置 / GM ====================

// RequestConsumeBelt 腰带吃药（CM_REQUESTCONSUMEBELT，playernet.cpp:1234）。
//
// 复用 InventoryHandler 的「消耗物品」逻辑 —— 它已经处理了
// 药水效果、数量扣减、广播。
func (h *QueryHandler) RequestConsumeBelt(ctx context.Context, channelID uint64, slot uint16) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Inventory == nil {
		return nil
	}
	if int(slot) >= len(p.Belt) {
		return nil
	}
	it := p.Belt[slot]
	if it == nil {
		return nil
	}
	// 复用 CM_CONSUMEITEM 的完整链路（药水效果/数量/广播都在那边）
	return h.Inventory.ConsumeItem(ctx, channelID, protocol.CMConsumeItem{
		ItemID: it.ItemID,
		SeqID:  uint32(it.SeqID),
		Count:  1,
	})
}

// SetRuntimeConfig 保存运行时配置（CM_SETRUNTIMECONFIG，playernet.cpp:1590）。
//
// 客户端在那里存「自动喝药阈值 / 自动修理 / 拾取过滤」这类偏好。
//
// ★ 必须落库：不存的话重登全丢，玩家每次都要重设 —— 这是最招骂的体验。
func (h *QueryHandler) SetRuntimeConfig(ctx context.Context, channelID uint64, req protocol.CMSetRuntimeConfig) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.ItemRepo == nil {
		return nil
	}
	key := trimCString([]byte(req.KeyBuf.String()))
	value := trimCString([]byte(req.ValueBuf.String()))

	cfg, err := h.ItemRepo.LoadPlayerConfig(ctx, p.DBID)
	if err != nil {
		return nil
	}
	if cfg.RuntimeConfig.Config == nil {
		cfg.RuntimeConfig.Config = make(map[string]string)
	}
	cfg.RuntimeConfig.Config[key] = value

	if err := h.ItemRepo.UpdateRuntimeConfig(ctx, p.DBID, cfg.RuntimeConfig); err != nil {
		if h.Logger != nil {
			h.Logger.Warn("save runtime config failed", "key", key, "error", err)
		}
	}
	return nil
}

// MakeItem GM 造物（CM_MAKEITEM，playernet.cpp:1548）。
//
// ★ 这是**调试命令**，正式服必须挡住。
//
//	原版不检查 GM 身份（`isGM` 在 Lua 绑定里，C++ 这条没判），
//	意味着任何客户端都能发这条刷物品 —— 原版能容忍是因为
//	它的客户端不提供入口、且是自建服。
//
// Go 侧加了一道：只有 Name 以 GM 白名单命中才允许，否则静默忽略。
// 白名单从环境变量读（MIR2X_GM_NAMES，逗号分隔），没配就全部拒绝。
func (h *QueryHandler) MakeItem(ctx context.Context, channelID uint64, itemID uint32, count uint16) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Items == nil || count == 0 {
		return nil
	}
	if !h.isGM(p.Name) {
		if h.Logger != nil {
			h.Logger.Warn("CM_MAKEITEM rejected: not a GM",
				"player", p.Name, "itemID", itemID, "count", count)
		}
		return nil
	}

	rec, found := h.Items.Get(itemID)
	if !found {
		return nil
	}

	// 可堆叠的按 99 上限拆包，不可堆叠的一个一格（原版同样的拆法）
	remaining := uint32(count)
	for remaining > 0 {
		chunk := uint32(1)
		if rec.Packable() {
			chunk = remaining
			if chunk > InventoryMaxHold {
				chunk = InventoryMaxHold
			}
		}
		p.AddItem(rec, Item{ItemID: itemID, Count: chunk})
		remaining -= chunk
	}
	if h.Inventory != nil {
		// 复用现有的背包推送（SM_INVENTORY 全量）
		h.Inventory.pushInventory(p.ChannelID, p)
	}
	return nil
}

// isGM 判断是否 GM（从环境变量读白名单）。
//
// ★ 不做「客户端自报 GM」的判断 —— 那个字段客户端能随便改。
func (h *QueryHandler) isGM(name string) bool {
	raw := os.Getenv("MIR2X_GM_NAMES")
	if raw == "" || name == "" {
		return false
	}
	for _, candidate := range strings.Split(raw, ",") {
		if strings.TrimSpace(candidate) == name {
			return true
		}
	}
	return false
}

// RequestKillPets 杀掉自己的宠物（CM_REQUESTKILLPETS，playernet.cpp:209）。
//
// 原版 `requestKillPets()` 会把玩家名下所有「跟随型」召唤物清掉
// （神兽/骷髅）。
//
// ★ 为什么需要这条：宠物卡在地图角落时，玩家没法自己解散 ——
//	只能自杀或等宠物自己死。原版就是给玩家一个「一键清宠」的出口。
func (h *QueryHandler) RequestKillPets(ctx context.Context, channelID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Worlds == nil || h.Worlds.Worlds == nil {
		return nil
	}
	// 找本图上主人是 p.UID 的召唤物，逐个置死
	uids, err := h.Worlds.Worlds.EntitiesByMaster(ctx, p.MapID, p.UID)
	if err != nil {
		return nil
	}
	for _, uid := range uids {
		// ★ 用 CommitDeath 而不是 Death.KillByDamage：
		//
		//	KillByDamage 是「玩家死亡」的入口（要推 SM_NOTIFYDEAD、
		//	掉落、经验结算）。宠物是怪物，走 world 的通用死亡提交。
		if _, _, err := h.Worlds.Worlds.CommitDeath(ctx, p.MapID, uid); err != nil {
			continue
		}
	}
	if h.Logger != nil && len(uids) > 0 {
		h.Logger.Info("killed pets on request", "player", p.Name, "count", len(uids))
	}
	return nil
}

// RequestHostOffline 让角色从服务器下线（CM_REQUESTHOSTOFFLINE，playernet.cpp:190）。
//
// 与 CM_REQUESTLOGOUT 的区别：
//
//	logout    → 玩家主动登出（回登录界面）
//	hostOffline → 断线（网络异常/踢下线），客户端不回登录界面而是提示重连
//
// 两条都要清理会话，只是回执不同。
func (h *QueryHandler) RequestHostOffline(ctx context.Context, channelID uint64) error {
	if h.Worlds != nil {
		h.Worlds.Leave(ctx, channelID)
	}
	if h.Sessions != nil {
		h.Sessions.Drop(channelID)
	}
	return nil
}

// ==================== 辅助 ====================

// QueryCORecord 客户端主动要某个实体的记录（CM_QUERYCORECORD，playernet.cpp:57）。
//
// ★ 用途：客户端「丢失了某个实体」（AOI 重算、重连后残留半透明单位）
// 时会主动补查一次。不回的话客户端永远画不出这个人。
//
// ★ 原版是「发给目标 actor、目标自己回报」，Go 侧直接从 world 读：
//
//	原版要求目标 actor 存在（`post` 失败就 reportDeadUID），
//	Go 侧没有独立的 actor 消息，直接 Entity() 查一次等价。
func (h *QueryHandler) QueryCORecord(ctx context.Context, channelID uint64, aimUID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Worlds == nil || aimUID == 0 || aimUID == p.UID {
		return nil
	}
	record, err := h.Worlds.BuildCORecord(ctx, p.MapID, aimUID)
	if err != nil {
		return nil
	}
	record.MapUID = p.MapUID
	payload, err := protocol.AppendWirePayload(nil, record)
	if err != nil {
		return nil
	}
	h.send(p, protocol.SM_CORECORD, payload)
	return nil
}

// RequestMagicDamage 客户端上报「我对某目标造成了法术伤害」
// （CM_REQUESTMAGICDAMAGE，playernet.cpp:110）。
//
// ★ 这是一条**客户端主导的伤害上报**，看起来像作弊入口，但原版就是这样：
//
//	原版 `dispatchAttackDamage(aimUID, magicID, 0)` —— damage 传 0，
//	真正的伤害数值由被攻击方自己按 magicID 算。
//	所以客户端只能「声称我放了某个技能」，不能「声称我打了多少」。
//
// Go 侧当前的法术伤害走 SpellHandler.Cast（服务端自己算），
// 这条只用于「客户端本地预演后补报」的场景 —— 直接忽略即可，不报错。
//
// 这里保留成 no-op 并记日志，而不是不注册：
// 不注册的话客户端会收到「未知消息」并可能断线重连。
func (h *QueryHandler) RequestMagicDamage(ctx context.Context, channelID uint64, magicID uint32, aimUID uint64) error {
	if h.Logger != nil {
		h.Logger.Debug("client-reported magic damage ignored (server-authoritative)",
			"channel", channelID, "magic", magicID, "aim", aimUID)
	}
	return nil
}

func (h *QueryHandler) send(p *Player, id protocol.SMType, payload []byte) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, id, payload, 0)
}

// sortRankingEntries 按金币或经验降序排（DB 已排序，这里是兜底）。
//
// 留着这个函数是为了「内存榜」场景（例如以后做实时活动榜），
// 当前走 SQL 排序，不调用它。
func sortRankingEntries(entries []protocol.SDRankingEntry, byGold bool) {
	sort.SliceStable(entries, func(i, j int) bool {
		if byGold {
			return entries[i].Gold > entries[j].Gold
		}
		return entries[i].Level > entries[j].Level
	})
}

// 确保 time 被引用（部分分支用 time.Now 做超时）
var _ = time.Now
var _ = fmt.Sprintf
var _ = storage.ErrCharacterNotFound
