package game

import (
	"context"
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

	// 通知被邀请者（在线才有意义）
	//
	// ★ 没有专门的「邀请」消息类型，复用了 SM_GUILDINFO：
	//
	//	原版协议里 SM_GUILDINVITE 是独立消息，但 Go 侧协议表里
	//	只映射了 SM_PLAYERGUILD/SM_GUILDERROR/SM_GUILDINFO/SM_GUILDCHAT，
	//	没有 SM_GUILDINVITE。
	//
	//	复用的风险：客户端看到 SM_GUILDINFO 会以为「我已经在会里」
	//	而不是「有人邀请我」。要让邀请真正弹出确认框，
	//	需要在协议表里补 SM_GUILDINVITE 并让客户端处理。
	//	已在 REPLICA_AUDIT 记为待办。
	if target, exists := h.Players.ByDBID(targetDBID); exists {
		member, _, _ := h.Repo.LoadMembership(ctx, p.DBID)
		info, _ := h.Repo.LoadInfo(ctx, member.GuildID)
		if payload, err := protocol.CerealSerialize(info); err == nil {
			h.sendTo(target, protocol.SM_GUILDINFO, payload)
		}
	}
	return nil
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
