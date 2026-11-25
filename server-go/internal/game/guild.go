package game

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// ==================== 公会 ====================
//
// 【依据】guilddb.cpp 520 行 + playernet.cpp 的 net_CM_*GUILD*
//
// 存储层（storage/guilds.go）早就写好了 20+ 个方法：
// LoadMembership / LoadInfo / Create / Invite / AcceptInvite /
// RejectInvite / Leave / Kick / TransferLeader / SetNotice /
// SetMemberTitle / Disband / DeclareWar / AtWar / LoadWarList …
//
// 这里只做「路由 → 玩家 → 调存储 → 推包」这一层。
//
// ★ 与组队/交易的关键区别：**公会要入库**。
//
//	组队是临时协作，掉线就没了；公会是长期组织，必须持久化。
//	所以公会的权威数据在 DB，内存里不缓存 ——
//	每次查询都读库，避免「两个进程/两次重连看到的成员列表不一样」。

// GuildHandler 处理公会相关消息。
type GuildHandler struct {
	Players   *PlayerManager
	Broadcast Broadcaster
	Logger    *slog.Logger
	Repo      *storage.GuildRepository
	// Chat 系统消息落库（邀请通知靠它，见 Invite）
	//
	// 可以留空：留空时邀请仍然成功，只是被邀请者收不到通知
	// （也不会入库，离线再看就没有了）。
	Chat ChatRepository
}

// QueryPlayerGuild 查某角色所属公会（CM_QUERYPLAYERGUILD）。
//
// 客户端用它画「目标玩家」的头顶公会名。
func (h *GuildHandler) QueryPlayerGuild(ctx context.Context, channelID uint64, dbid uint32) error {
	_, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}

	member, found, err := h.Repo.LoadMembership(ctx, dbid)
	if err != nil || !found {
		return nil
	}
	info, err := h.Repo.LoadInfo(ctx, member.GuildID)
	if err != nil {
		return nil
	}

	payload, err := protocol.CerealSerialize(protocol.SDGuildInfo{
		GuildID: info.GuildID,
		Name:    info.Name,
		Notice:  info.Notice,
	})
	if err == nil {
		h.sendToChannel(channelID, protocol.SM_PLAYERGUILD, payload)
	}
	return nil
}

// QueryGuildInfo 查公会详情（CM_QUERYGUILDINFO）。
func (h *GuildHandler) QueryGuildInfo(ctx context.Context, channelID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	member, found, err := h.Repo.LoadMembership(ctx, p.DBID)
	if err != nil || !found {
		h.sendError(p, uint8(storage.GuildErrNoGuild))
		return nil
	}
	info, err := h.Repo.LoadInfo(ctx, member.GuildID)
	if err != nil {
		return nil
	}
	if payload, err := protocol.CerealSerialize(info); err == nil {
		h.sendTo(p, protocol.SM_GUILDINFO, payload)
	}
	return nil
}

// Invite 邀请某人入会（CM_INVITEGUILDMEMBER）。
//
// 原版按名字邀请（客户端只知道自己看到的角色名）。
//
// ★★ 这里曾经写错过，值得留档。★★
//
// 早期实现认为「协议表缺 SM_GUILDINVITE，所以只能复用 SM_GUILDINFO」，
// 并在审计里记成「需要补协议表」。**这个判断是错的。**
//
// 翻原版 playernet.cpp:2248 net_CM_INVITEGUILDMEMBER，邀请通知走的是：
//
//	postSystemMessage(memberDBID, xml)     // 存成系统助手的聊天消息
//
// 那段 xml 长这样（每行一个 <par>）：
//
//	你被<t color="red">邀请者</t>邀请加入<t color="red">会名</t>行会，你可以选择
//	<event id="SYS_GIRESP" accept="" guild="7">同意</event>
//	<event id="SYS_GIRESP" reject="" guild="7">拒绝</event>
//
// 客户端点「同意」时把 `accept=""/reject=""` 与 `guild` 原样塞进
// CM_RESPONDGUILDINVITE（字段就是 guildID + accept，见 clientmsg.hpp:491）。
//
// 也就是说：**不需要任何新协议**。协议表本来就只有
// SM_GUILDWARLIST / SM_PLAYERGUILD / SM_GUILDERROR / SM_GUILDINFO / SM_GUILDCHAT。
// 邀请和投递一样，复用「系统聊天气泡 + <event> 回调」这套通用机制。
//
// ★ 走系统消息还有一个副作用是**离线可收**：
// 邀请存进 tbl_chatmessage，被邀请者下次登录会看到这条消息，
// 但那时 tbl_guildinvite 里那条邀请可能已经过期或被撤，
// 点「同意」会拿到 GUILDERR_NOINVITE —— 与原版行为一致。
func (h *GuildHandler) Invite(ctx context.Context, channelID uint64, name string) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	name = trimCString([]byte(name))
	if name == "" {
		return nil
	}

	targetDBID, err := h.Repo.FindCharacterDBID(ctx, name)
	if err != nil {
		h.sendError(p, uint8(storage.GuildErrNoPlayer))
		return nil
	}
	if targetDBID == p.DBID {
		h.sendError(p, uint8(storage.GuildErrBadTarget))
		return nil
	}

	code, err := h.Repo.Invite(ctx, p.DBID, targetDBID)
	if err != nil || code != storage.GuildErrNone {
		h.sendError(p, uint8(code))
		return nil
	}

	// 取公会名用于消息正文（邀请成功后一定能查到）
	member, found, _ := h.Repo.LoadMembership(ctx, p.DBID)
	if !found {
		return nil
	}
	info, err := h.Repo.LoadInfo(ctx, member.GuildID)
	if err != nil {
		return nil
	}

	h.postSystemMessage(ctx, targetDBID, buildGuildInviteXML(p.Name, info.Name, info.GuildID))
	return nil
}

// buildGuildInviteXML 拼公会邀请的系统消息正文（playernet.cpp:2248）。
//
// ★ 三个 <par> 之间**不能用 <br> 或 \n**：
//
//	客户端的 XML 布局引擎只认 <layout>/<par> 结构，
//	裸换行会被当成空白丢掉，「同意」「拒绝」会挤在同一行。
func buildGuildInviteXML(inviter, guildName string, guildID uint32) string {
	return string("<layout>") +
		"<par>你被<t color=\"red\">" + escapeXMLText(inviter) + "</t>邀请加入<t color=\"red\">" +
		escapeXMLText(guildName) + "</t>行会，你可以选择</par>" +
		fmt.Sprintf("<par><event id=\"%s\" accept=\"\" guild=\"%d\">同意</event></par>", SysEventGuildInvite, guildID) +
		fmt.Sprintf("<par><event id=\"%s\" reject=\"\" guild=\"%d\">拒绝</event></par>", SysEventGuildInvite, guildID) +
		"</layout>"
}

// postSystemMessage 以「系统助手」身份给某角色发一条聊天消息
// （player.cpp:3508 postSystemMessage）。
//
// 在线就直接推下去；离线只入库，下次登录随聊天记录拉下来。
func (h *GuildHandler) postSystemMessage(ctx context.Context, toDBID uint32, body string) {
	from := protocol.NewSDChatPeerID(protocol.CPR_SPECIAL, SysChatDBID)
	to := protocol.NewSDChatPeerID(protocol.CPR_PLAYER, toDBID)

	var seq protocol.SDChatMessageDBSeq
	if h.Chat != nil {
		saved, err := h.Chat.SaveMessage(ctx, from, to, body, nil)
		if err != nil {
			if h.Logger != nil {
				h.Logger.Warn("save guild invite message failed", "to", toDBID, "error", err)
			}
		} else {
			seq = saved
		}
	}

	target, online := h.Players.ByDBID(toDBID)
	if !online || target.ChannelID == 0 || h.Broadcast == nil {
		return
	}
	list := protocol.SDChatMessageList{{
		HasSeq:  h.Chat != nil,
		Seq:     seq,
		From:    from,
		To:      to,
		Message: body,
	}}
	payload, err := protocol.CerealSerialize(list)
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(target.ChannelID, protocol.SM_CHATMESSAGELIST, payload, 0)
}

// RespondInvite 回应邀请（CM_RESPONDGUILDINVITE）。
func (h *GuildHandler) RespondInvite(ctx context.Context, channelID uint64, guildID uint32, accept bool) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	if !accept {
		if _, _, err := h.Repo.RejectInvite(ctx, p.DBID, guildID); err != nil {
			return nil
		}
		return nil
	}
	if code, err := h.Repo.AcceptInvite(ctx, p.DBID, guildID); err != nil || code != storage.GuildErrNone {
		h.sendError(p, uint8(code))
		return nil
	}
	// 入会成功：把最新公会信息推给自己 + 通知全会在线的成员
	h.pushGuildToMembers(ctx, guildID)
	return nil
}

// Leave 退出公会（CM_LEAVEGUILD）。
func (h *GuildHandler) Leave(ctx context.Context, channelID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	// 先记下 guildID：退出后就查不到了
	member, found, _ := h.Repo.LoadMembership(ctx, p.DBID)
	if !found {
		return nil
	}
	if code, err := h.Repo.Leave(ctx, p.DBID); err != nil || code != storage.GuildErrNone {
		h.sendError(p, uint8(code))
		return nil
	}
	h.pushGuildToMembers(ctx, member.GuildID)
	return nil
}

// Kick 踢人（CM_KICKGUILDMEMBER）。
func (h *GuildHandler) Kick(ctx context.Context, channelID uint64, targetDBID uint32) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	member, found, _ := h.Repo.LoadMembership(ctx, p.DBID)
	if !found {
		h.sendError(p, uint8(storage.GuildErrNoGuild))
		return nil
	}
	if code, err := h.Repo.Kick(ctx, p.DBID, targetDBID); err != nil || code != storage.GuildErrNone {
		h.sendError(p, uint8(code))
		return nil
	}
	// 被踢的人也要收到「公会列表清空」
	if target, exists := h.Players.ByDBID(targetDBID); exists {
		if payload, err := protocol.CerealSerialize(protocol.SDGuildInfo{}); err == nil {
			h.sendTo(target, protocol.SM_GUILDINFO, payload)
		}
	}
	h.pushGuildToMembers(ctx, member.GuildID)
	return nil
}

// TransferLeader 移交会长（CM_TRANSFERGUILDLEADER）。
func (h *GuildHandler) TransferLeader(ctx context.Context, channelID uint64, targetDBID uint32) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	member, found, _ := h.Repo.LoadMembership(ctx, p.DBID)
	if !found {
		return nil
	}
	if code, err := h.Repo.TransferLeader(ctx, p.DBID, targetDBID); err != nil || code != storage.GuildErrNone {
		h.sendError(p, uint8(code))
		return nil
	}
	h.pushGuildToMembers(ctx, member.GuildID)
	return nil
}

// SetNotice 改公告（CM_SETGUILDNOTICE）。
func (h *GuildHandler) SetNotice(ctx context.Context, channelID uint64, notice string) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	member, found, _ := h.Repo.LoadMembership(ctx, p.DBID)
	if !found {
		return nil
	}
	if code, err := h.Repo.SetNotice(ctx, p.DBID, notice); err != nil || code != storage.GuildErrNone {
		h.sendError(p, uint8(code))
		return nil
	}
	h.pushGuildToMembers(ctx, member.GuildID)
	return nil
}

// SetMemberTitle 改成员称号（CM_SETGUILDMEMBERTITLE）。
func (h *GuildHandler) SetMemberTitle(ctx context.Context, channelID uint64, targetDBID uint32, title string) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	member, found, _ := h.Repo.LoadMembership(ctx, p.DBID)
	if !found {
		return nil
	}
	if code, err := h.Repo.SetMemberTitle(ctx, p.DBID, targetDBID, title); err != nil || code != storage.GuildErrNone {
		h.sendError(p, uint8(code))
		return nil
	}
	h.pushGuildToMembers(ctx, member.GuildID)
	return nil
}

// Disband 解散公会（CM_DISBANDGUILD）。
func (h *GuildHandler) Disband(ctx context.Context, channelID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	memberDBIDs, code, err := h.Repo.Disband(ctx, p.DBID)
	if err != nil || code != storage.GuildErrNone {
		h.sendError(p, uint8(code))
		return nil
	}
	// 通知所有原成员：公会没了
	for _, dbid := range memberDBIDs {
		if m, exists := h.Players.ByDBID(dbid); exists {
			if payload, err := protocol.CerealSerialize(protocol.SDGuildInfo{}); err == nil {
				h.sendTo(m, protocol.SM_GUILDINFO, payload)
			}
		}
	}
	return nil
}

// GuildChat 公会频道说话（CM_GUILDCHAT）。
func (h *GuildHandler) GuildChat(ctx context.Context, channelID uint64, req protocol.CMGuildChat) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok || h.Repo == nil {
		return nil
	}
	content := trimCString(req.Content[:])
	if !hasVisibleChar(content) {
		return nil
	}
	member, found, _ := h.Repo.LoadMembership(ctx, p.DBID)
	if !found {
		h.sendError(p, uint8(storage.GuildErrNoGuild))
		return nil
	}

	members, err := h.Repo.LoadMemberList(ctx, member.GuildID)
	if err != nil {
		return nil
	}

	var nameBuf common.StaticBuffer[common.Cap64]
	_ = nameBuf.Assign([]byte(p.Name))
	payload, err := protocol.AppendWirePayload(nil, protocol.SMGuildChat{
		UID: p.UID, Name: nameBuf, Content: truncateCString(content, 128),
	})
	if err != nil {
		return nil
	}
	// 发给全会在线的成员（含自己）
	for _, dbid := range members {
		if m, exists := h.Players.ByDBID(dbid); exists && m.ChannelID != 0 {
			_ = h.Broadcast.SendTo(m.ChannelID, protocol.SM_GUILDCHAT, payload, 0)
		}
	}
	return nil
}

// ==================== 内部 ====================

// pushGuildToMembers 把最新公会信息推给所有在线成员。
func (h *GuildHandler) pushGuildToMembers(ctx context.Context, guildID uint32) {
	info, err := h.Repo.LoadInfo(ctx, guildID)
	if err != nil {
		return
	}
	payload, err := protocol.CerealSerialize(info)
	if err != nil {
		return
	}
	for _, m := range info.MemberList {
		if online, exists := h.Players.ByDBID(m.DBID); exists {
			h.sendTo(online, protocol.SM_GUILDINFO, payload)
		}
	}
}

func (h *GuildHandler) sendError(p *Player, code uint8) {
	if code == uint8(storage.GuildErrNone) {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMGuildError{Error: code})
	if err == nil {
		h.sendTo(p, protocol.SM_GUILDERROR, payload)
	}
}

func (h *GuildHandler) sendTo(p *Player, id protocol.SMType, payload []byte) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, id, payload, 0)
}

func (h *GuildHandler) sendToChannel(channelID uint64, id protocol.SMType, payload []byte) {
	if h.Broadcast == nil || channelID == 0 {
		return
	}
	_ = h.Broadcast.SendTo(channelID, id, payload, 0)
}
