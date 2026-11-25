package game

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
	"github.com/lff2116788383/godot-mir2/server-go/internal/transport"
)

// ==================== 登录链路的 transport 适配 ====================
//
// 这一层把 game.Handler 的方法包装成 transport.HandlerFunc，
// 并负责：
//   - 从 transport.Session 取 channelID（用 Session.ID()）
//   - 解码 wire payload
//   - 把回包写回同一个连接
//
// 为什么要单独一层：game 包不应 import transport（否则 game 里会充斥网络细节），
// 但 dispatcher 注册又需要 transport.HandlerFunc 的签名。放在 game 包的
// wire.go 里，依赖方向是 game -> transport，而不是反过来。

// RegisterActionRoutes 注册游戏内动作消息。
//
// B5 只接 CM_ACTION（移动/站立/攻击/拾取）。其余动作类消息
// （CM_REQUESTEQUIPWEAR、CM_CONSUMEITEM 等）在后续迭代追加。
func RegisterActionRoutes(dispatcher *transport.Dispatcher, handler *ActionHandler) error {
	if handler == nil {
		return nil
	}
	return dispatcher.Register(protocol.CM_ACTION, func(ctx context.Context, session *transport.Session, packet protocol.Packet) error {
		var req protocol.CMAction
		if err := decode(packet, &req); err != nil {
			return err
		}
		return handler.CMAction(ctx, session.ID(), req)
	})
}

// RegisterInventoryRoutes 注册背包/装备消息。
//
// 6 条：穿装备、脱装备、装腰带、取腰带、丢弃、使用。
func RegisterInventoryRoutes(dispatcher *transport.Dispatcher, handler *InventoryHandler) error {
	if handler == nil {
		return nil
	}
	routes := []struct {
		id      protocol.CMType
		handler transport.HandlerFunc
	}{
		{protocol.CM_REQUESTEQUIPWEAR, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRequestEquipWear
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.RequestEquipWear(ctx, s.ID(), req)
		}},
		{protocol.CM_REQUESTGRABWEAR, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRequestGrabWear
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.RequestGrabWear(ctx, s.ID(), req)
		}},
		{protocol.CM_REQUESTEQUIPBELT, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRequestEquipBelt
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.RequestEquipBelt(ctx, s.ID(), req)
		}},
		{protocol.CM_REQUESTGRABBELT, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRequestGrabBelt
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.RequestGrabBelt(ctx, s.ID(), req)
		}},
		{protocol.CM_DROPITEM, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMDropItem
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.DropItem(ctx, s.ID(), req)
		}},
		{protocol.CM_CONSUMEITEM, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMConsumeItem
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.ConsumeItem(ctx, s.ID(), req)
		}},
	}
	for _, route := range routes {
		if err := dispatcher.Register(route.id, route.handler); err != nil {
			return fmt.Errorf("register %v: %w", route.id, err)
		}
	}
	return nil
}

// RegisterLoginRoutes 把所有登录链路消息注册到 dispatcher。
//
// 未注册的消息会被 dispatcher 静默忽略（当前 B4 阶段的行为），
// 所以这里只注册已实现的 7 条。后续批次继续往这里加。
func RegisterLoginRoutes(dispatcher *transport.Dispatcher, handler *Handler) error {
	routes := []struct {
		id      protocol.CMType
		handler transport.HandlerFunc
	}{
		{protocol.CM_LOGIN, wrapLogin(handler)},
		{protocol.CM_QUERYCHAR, wrapQueryChar(handler)},
		{protocol.CM_ONLINE, wrapOnline(handler)},
		{protocol.CM_CREATEACCOUNT, wrapCreateAccount(handler)},
		{protocol.CM_CHANGEPASSWORD, wrapChangePassword(handler)},
		{protocol.CM_CREATECHAR, wrapCreateChar(handler)},
		{protocol.CM_DELETECHAR, wrapDeleteChar(handler)},
	}

	for _, route := range routes {
		if err := dispatcher.Register(route.id, route.handler); err != nil {
			return fmt.Errorf("register %v: %w", route.id, err)
		}
	}
	return nil
}

// decode 把 wire payload 解成具体结构，并校验没有多余字节。
func decode[T any](packet protocol.Packet, value *T) error {
	n, err := protocol.ReadWirePayload(value, packet.Payload)
	if err != nil {
		return fmt.Errorf("decode %v: %w", packet.Code, err)
	}
	if n != len(packet.Payload) {
		return fmt.Errorf("decode %v: consumed %d of %d bytes", packet.Code, n, len(packet.Payload))
	}
	return nil
}

func wrapLogin(h *Handler) transport.HandlerFunc {
	return func(ctx context.Context, session *transport.Session, packet protocol.Packet) error {
		var req protocol.CMLogin
		if err := decode(packet, &req); err != nil {
			return err
		}
		return h.CMLogin(ctx, session.ID(), packet.ResponseID, req, session)
	}
}

func wrapQueryChar(h *Handler) transport.HandlerFunc {
	return func(ctx context.Context, session *transport.Session, packet protocol.Packet) error {
		return h.CMQueryChar(ctx, session.ID(), packet.ResponseID, session)
	}
}

func wrapOnline(h *Handler) transport.HandlerFunc {
	return func(ctx context.Context, session *transport.Session, packet protocol.Packet) error {
		_, err := h.CMOnline(ctx, session.ID(), packet.ResponseID, session)
		return err
	}
}

func wrapCreateAccount(h *Handler) transport.HandlerFunc {
	return func(ctx context.Context, session *transport.Session, packet protocol.Packet) error {
		var req protocol.CMCreateAccount
		if err := decode(packet, &req); err != nil {
			return err
		}
		return h.CMCreateAccount(ctx, packet.ResponseID, req, session)
	}
}

func wrapChangePassword(h *Handler) transport.HandlerFunc {
	return func(ctx context.Context, session *transport.Session, packet protocol.Packet) error {
		var req protocol.CMChangePassword
		if err := decode(packet, &req); err != nil {
			return err
		}
		return h.CMChangePassword(ctx, packet.ResponseID, req, session)
	}
}

func wrapCreateChar(h *Handler) transport.HandlerFunc {
	return func(ctx context.Context, session *transport.Session, packet protocol.Packet) error {
		var req protocol.CMCreateChar
		if err := decode(packet, &req); err != nil {
			return err
		}
		return h.CMCreateChar(ctx, session.ID(), packet.ResponseID, req, session)
	}
}

func wrapDeleteChar(h *Handler) transport.HandlerFunc {
	return func(ctx context.Context, session *transport.Session, packet protocol.Packet) error {
		var req protocol.CMDeleteChar
		if err := decode(packet, &req); err != nil {
			return err
		}
		return h.CMDeleteChar(ctx, session.ID(), packet.ResponseID, req, session)
	}
}

// SessionCleaner 在连接断开时清理登录态。
//
// transport.Server 的 SessionLifecycleHandler 会在会话关闭时回调，
// 不清理会让「同账号重连」被判成 MULTILOGIN（原版靠 channID 复用解决）。
func (h *Handler) SessionClosed(session *transport.Session) {
	if session == nil {
		return
	}
	h.Sessions.Drop(session.ID())
}

// BuildHandler 是给 cmd 用的便捷装配函数。
func BuildHandler(store *storage.Store) *Handler {
	return NewHandler(
		NewSessionManager(),
		storage.NewAccountRepository(store),
		storage.NewCharacterRepository(store),
	)
}

// RegisterSpellRoutes 注册施法相关消息。
//
// 只有一条 CM_SETMAGICKEY 走独立消息；真正的施法动作（ACTION_SPELL）
// 是 CM_ACTION 的一个 type，由 RegisterActionRoutes 的 CMAction
// 分发到 SpellHandler（见 ActionHandler.CMAction）。
func RegisterSpellRoutes(dispatcher *transport.Dispatcher, handler *SpellHandler) error {
	if handler == nil {
		return nil
	}
	// CM_SETMAGICKEY：设置技能快捷键（magicID -> 按键）
	return dispatcher.Register(protocol.CM_SETMAGICKEY, func(ctx context.Context, session *transport.Session, packet protocol.Packet) error {
		var req protocol.CMSetMagicKey
		if err := decode(packet, &req); err != nil {
			return err
		}
		return handler.SetMagicKey(ctx, session.ID(), req)
	})
}

// RegisterDeathRoutes 注册死亡与复活。
func RegisterDeathRoutes(dispatcher *transport.Dispatcher, handler *DeathHandler) error {
	if handler == nil {
		return nil
	}
	routes := []struct {
		id      protocol.CMType
		handler transport.HandlerFunc
	}{
		{protocol.CM_REQUESTDIE, func(ctx context.Context, s *transport.Session, _ protocol.Packet) error {
			return handler.RequestDie(ctx, s.ID())
		}},
		{protocol.CM_REQUESTREVIVE, func(ctx context.Context, s *transport.Session, _ protocol.Packet) error {
			return handler.RequestRevive(ctx, s.ID())
		}},
	}
	for _, route := range routes {
		if err := dispatcher.Register(route.id, route.handler); err != nil {
			return fmt.Errorf("register %v: %w", route.id, err)
		}
	}
	return nil
}

// RegisterShopRoutes 注册金币、经验与商店买卖。
func RegisterShopRoutes(dispatcher *transport.Dispatcher, handler *ShopHandler) error {
	if handler == nil {
		return nil
	}
	routes := []struct {
		id      protocol.CMType
		handler transport.HandlerFunc
	}{
		{protocol.CM_REQUESTADDEXP, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRequestAddExp
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.RequestAddExp(ctx, s.ID(), req)
		}},
		{protocol.CM_REQUESTADDHEALTH, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRequestAddHealth
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.RequestAddHealth(ctx, s.ID(), req)
		}},
		{protocol.CM_QUERYSELLITEMLIST, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMQuerySellItemList
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.QuerySellItemList(ctx, s.ID(), req)
		}},
		{protocol.CM_BUY, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMBuy
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.Buy(ctx, s.ID(), req)
		}},
	}
	for _, route := range routes {
		if err := dispatcher.Register(route.id, route.handler); err != nil {
			return fmt.Errorf("register %v: %w", route.id, err)
		}
	}
	return nil
}

// RegisterNPCRoutes 注册 NPC 对话消息。
//
// CM_NPCEVENT 是定长 410 字节的压缩消息，用 decode（wire 解码）处理。
func RegisterNPCRoutes(dispatcher *transport.Dispatcher, handler *NPCHandler) error {
	if handler == nil {
		return nil
	}
	return dispatcher.Register(protocol.CM_NPCEVENT,
		func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMNPCEvent
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.HandleEvent(ctx, s.ID(), req)
		})
}

// RegisterChatRoutes 注册聊天类消息。
//
// 三条消息的编码方式不同，容易踩坑：
//
//	CM_PLAYERSAY / CM_PLAYERBROADCAST  定长 128 字节（FixedCompressed）
//	CM_CHATMESSAGE                     变长：16 字节头 + 原始正文
//
// 前两条用 decode（wire 解码会做 XOR/压缩还原），
// 后者不能整体 wire 解码 —— 因为正文是裸字符串没有长度前缀，
// 必须手工切前 16 字节当头部，其余当正文。
func RegisterChatRoutes(dispatcher *transport.Dispatcher, handler *ChatHandler) error {
	if handler == nil {
		return nil
	}
	routes := []struct {
		id      protocol.CMType
		handler transport.HandlerFunc
	}{
		{protocol.CM_PLAYERSAY, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMPlayerSay
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.Say(ctx, s.ID(), req)
		}},
		{protocol.CM_PLAYERBROADCAST, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMPlayerBroadcast
			if err := decode(p, &req); err != nil {
				return err
			}
			return handler.BroadcastSay(ctx, s.ID(), req)
		}},
		{protocol.CM_CHATMESSAGE, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			header, body, err := splitChatMessage(p.Payload)
			if err != nil {
				return err
			}
			return handler.ChatMessage(ctx, s.ID(), header, body)
		}},
	}
	for _, route := range routes {
		if err := dispatcher.Register(route.id, route.handler); err != nil {
			return fmt.Errorf("register %v: %w", route.id, err)
		}
	}
	return nil
}

// chatMessageHeaderSize 是 CMChatMessageHeader 的固定长度。
//
// 【依据】common/src/clientmsg.hpp:219-224：
//
//	uint64_t toCPID;
//	uint64_t hasRef : 1;   // 与 refID 共用同一个 uint64
//	uint64_t refID  : 63;
//
// 所以是 8 + 8 = 16 字节。
const chatMessageHeaderSize = 16

// splitChatMessage 把 CM_CHATMESSAGE 的 body 切成「头部 + 正文」。
//
// 【依据】common/src/clientmsg.hpp:219-224：
//
//	uint64_t toCPID;
//	uint64_t hasRef :  1;
//	uint64_t refID  : 63;
//
// 后两个字段共用一个 uint64（GCC x86 的位域从最低位开始排）：
//
//	bit 0     → hasRef
//	bit 1..63 → refID
//
// ★ 为什么不用 wire 解码：
//
//	CM_CHATMESSAGE 是 MessageVariable，body = 16 字节头 + 裸字符串正文。
//	正文没有长度前缀（靠整包长度算），wire 解码器按结构体字段读会读到越界
//	或者把正文前几个字节当成别的字段。
func splitChatMessage(payload []byte) (protocol.CMChatMessageHeader, string, error) {
	if len(payload) < chatMessageHeaderSize {
		return protocol.CMChatMessageHeader{}, "", fmt.Errorf(
			"chat message too short: %d < %d", len(payload), chatMessageHeaderSize)
	}
	toCPID := binary.LittleEndian.Uint64(payload[0:8])
	flags := binary.LittleEndian.Uint64(payload[8:16])

	header := protocol.CMChatMessageHeader{
		ToCPID: toCPID,
		HasRef: flags&1 != 0,
		RefID:  flags >> 1,
	}
	body := string(payload[chatMessageHeaderSize:])
	return header, body, nil
}

// RegisterTeamRoutes 注册组队消息。
func RegisterTeamRoutes(dispatcher *transport.Dispatcher, teams *TeamManager) error {
	if teams == nil {
		return nil
	}

	fixed := []struct {
		id protocol.CMType
		fn func(ctx context.Context, s *transport.Session, p protocol.Packet) error
	}{
		{protocol.CM_REQUESTJOINTEAM, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRequestJoinTeam
			if err := decode(p, &req); err != nil {
				return err
			}
			return teams.handleJoin(ctx, s.ID(), req.UID)
		}},
		{protocol.CM_REQUESTLEAVETEAM, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRequestLeaveTeam
			if err := decode(p, &req); err != nil {
				return err
			}
			return teams.handleLeave(ctx, s.ID(), req.UID)
		}},
		{protocol.CM_QUERYTEAMINFO, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMQueryTeamInfo
			if err := decode(p, &req); err != nil {
				return err
			}
			return teams.handleQueryInfo(ctx, s.ID(), req.UID)
		}},
		{protocol.CM_TRANSFERTEAMLEADER, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMTransferTeamLeader
			if err := decode(p, &req); err != nil {
				return err
			}
			return teams.handleTransfer(ctx, s.ID(), req.UID)
		}},
	}
	for _, r := range fixed {
		if err := dispatcher.Register(r.id, r.fn); err != nil {
			return fmt.Errorf("register %v: %w", r.id, err)
		}
	}

	// 变长消息（cerealf 序列化）
	variable := []struct {
		id protocol.CMType
		fn func(ctx context.Context, s *transport.Session, p protocol.Packet) error
	}{
		{protocol.CM_CREATETEAM, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.SDTeamReq
			if err := protocol.CerealDeserialize(p.Payload, &req); err != nil {
				return nil // 内容非法就静默丢弃，不回错误
			}
			return teams.handleCreate(ctx, s.ID(), req)
		}},
		{protocol.CM_SETTEAMREQUIREMENT, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.SDTeamReq
			if err := protocol.CerealDeserialize(p.Payload, &req); err != nil {
				return nil
			}
			return teams.handleSetRequirement(ctx, s.ID(), req)
		}},
	}
	for _, r := range variable {
		if err := dispatcher.Register(r.id, r.fn); err != nil {
			return fmt.Errorf("register %v: %w", r.id, err)
		}
	}
	return nil
}

// RegisterSecuredRoutes 注册仓库消息。
//
// ★ 只有「取回」需要路由：
//
//	寄存（uidSecureItem）是 NPC 脚本调的服务端函数，不经过客户端消息。
//	只有「从仓库取回」是玩家点客户端按钮 → CM_REQUESTRETRIEVESECUREDITEM。
func RegisterSecuredRoutes(dispatcher *transport.Dispatcher, secured *SecuredHandler) error {
	if secured == nil {
		return nil
	}
	return dispatcher.Register(protocol.CM_REQUESTRETRIEVESECUREDITEM,
		func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRequestRetrieveSecuredItem
			if err := decode(p, &req); err != nil {
				return err
			}
			return secured.Retrieve(ctx, s.ID(), req.ItemID, req.SeqID)
		})
}

// RegisterDirectTradeRoutes 注册面对面交易消息。
func RegisterDirectTradeRoutes(dispatcher *transport.Dispatcher, trades *DirectTradeManager) error {
	if trades == nil {
		return nil
	}

	fixed := []struct {
		id protocol.CMType
		fn func(ctx context.Context, s *transport.Session, p protocol.Packet) error
	}{
		{protocol.CM_REQUESTDIRECTTRADE, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRequestDirectTrade
			if err := decode(p, &req); err != nil {
				return err
			}
			return trades.handleRequest(ctx, s.ID(), req.UID)
		}},
		{protocol.CM_RESPONDDIRECTTRADE, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRespondDirectTrade
			if err := decode(p, &req); err != nil {
				return err
			}
			return trades.handleRespond(ctx, s.ID(), req.UID, req.Accept != 0)
		}},
		{protocol.CM_COMMITDIRECTTRADE, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMCommitDirectTrade
			if err := decode(p, &req); err != nil {
				return err
			}
			return trades.handleCommit(ctx, s.ID(), req.UID)
		}},
		{protocol.CM_CANCELDIRECTTRADE, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMCancelDirectTrade
			if err := decode(p, &req); err != nil {
				return err
			}
			return trades.handleCancel(ctx, s.ID(), req.UID)
		}},
	}
	for _, r := range fixed {
		if err := dispatcher.Register(r.id, r.fn); err != nil {
			return fmt.Errorf("register %v: %w", r.id, err)
		}
	}

	// 变长消息：更新报价
	return dispatcher.Register(protocol.CM_UPDATEDIRECTTRADE,
		func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.SDDirectTradeOfferRequest
			if err := protocol.CerealDeserialize(p.Payload, &req); err != nil {
				return nil
			}
			return trades.handleUpdate(ctx, s.ID(), req)
		})
}

// RegisterGuildRoutes 注册公会消息。
func RegisterGuildRoutes(dispatcher *transport.Dispatcher, guilds *GuildHandler) error {
	if guilds == nil {
		return nil
	}
	routes := []struct {
		id protocol.CMType
		fn transport.HandlerFunc
	}{
		// 定长消息
		{protocol.CM_QUERYPLAYERGUILD, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMQueryPlayerGuild
			if err := decode(p, &req); err != nil {
				return err
			}
			return guilds.QueryPlayerGuild(ctx, s.ID(), req.DBID)
		}},
		{protocol.CM_INVITEGUILDMEMBER, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMInviteGuildMember
			if err := decode(p, &req); err != nil {
				return err
			}
			return guilds.Invite(ctx, s.ID(), trimCString([]byte(req.Name.String())))
		}},
		{protocol.CM_RESPONDGUILDINVITE, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRespondGuildInvite
			if err := decode(p, &req); err != nil {
				return err
			}
			return guilds.RespondInvite(ctx, s.ID(), req.GuildID, req.Accept != 0)
		}},
		{protocol.CM_KICKGUILDMEMBER, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMKickGuildMember
			if err := decode(p, &req); err != nil {
				return err
			}
			return guilds.Kick(ctx, s.ID(), req.DBID)
		}},
		{protocol.CM_TRANSFERGUILDLEADER, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMTransferGuildLeader
			if err := decode(p, &req); err != nil {
				return err
			}
			return guilds.TransferLeader(ctx, s.ID(), req.DBID)
		}},
		{protocol.CM_SETGUILDNOTICE, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMSetGuildNotice
			if err := decode(p, &req); err != nil {
				return err
			}
			return guilds.SetNotice(ctx, s.ID(), trimCString([]byte(req.Notice.String())))
		}},
		{protocol.CM_SETGUILDMEMBERTITLE, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMSetGuildMemberTitle
			if err := decode(p, &req); err != nil {
				return err
			}
			return guilds.SetMemberTitle(ctx, s.ID(), req.DBID, trimCString([]byte(req.Title.String())))
		}},
		{protocol.CM_GUILDCHAT, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMGuildChat
			if err := decode(p, &req); err != nil {
				return err
			}
			return guilds.GuildChat(ctx, s.ID(), req)
		}},
	}
	for _, r := range routes {
		if err := dispatcher.Register(r.id, r.fn); err != nil {
			return fmt.Errorf("register %v: %w", r.id, err)
		}
	}

	// 无参数消息
	empty := []struct {
		id protocol.CMType
		fn transport.HandlerFunc
	}{
		{protocol.CM_QUERYGUILDINFO, func(ctx context.Context, s *transport.Session, _ protocol.Packet) error {
			return guilds.QueryGuildInfo(ctx, s.ID())
		}},
		{protocol.CM_LEAVEGUILD, func(ctx context.Context, s *transport.Session, _ protocol.Packet) error {
			return guilds.Leave(ctx, s.ID())
		}},
		{protocol.CM_DISBANDGUILD, func(ctx context.Context, s *transport.Session, _ protocol.Packet) error {
			return guilds.Disband(ctx, s.ID())
		}},
	}
	for _, r := range empty {
		if err := dispatcher.Register(r.id, r.fn); err != nil {
			return fmt.Errorf("register %v: %w", r.id, err)
		}
	}
	return nil
}

// RegisterStallRoutes 注册摆摊消息。
func RegisterStallRoutes(dispatcher *transport.Dispatcher, stalls *StallHandler) error {
	if stalls == nil {
		return nil
	}
	routes := []struct {
		id protocol.CMType
		fn transport.HandlerFunc
	}{
		{protocol.CM_ADDSTALLITEM, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMAddStallItem
			if err := decode(p, &req); err != nil {
				return err
			}
			return stalls.AddItem(ctx, s.ID(), req)
		}},
		{protocol.CM_REMOVESTALLITEM, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMRemoveStallItem
			if err := decode(p, &req); err != nil {
				return err
			}
			return stalls.RemoveItem(ctx, s.ID(), req)
		}},
		{protocol.CM_STARTSTALL, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMStartStall
			if err := decode(p, &req); err != nil {
				return err
			}
			return stalls.Start(ctx, s.ID(), req)
		}},
		{protocol.CM_QUERYSTALL, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMQueryStall
			if err := decode(p, &req); err != nil {
				return err
			}
			return stalls.Query(ctx, s.ID(), req.DBID)
		}},
		{protocol.CM_BUYSTALLITEM, func(ctx context.Context, s *transport.Session, p protocol.Packet) error {
			var req protocol.CMBuyStallItem
			if err := decode(p, &req); err != nil {
				return err
			}
			return stalls.Buy(ctx, s.ID(), req)
		}},
		{protocol.CM_CLOSESTALL, func(ctx context.Context, s *transport.Session, _ protocol.Packet) error {
			return stalls.Close(ctx, s.ID())
		}},
	}
	for _, r := range routes {
		if err := dispatcher.Register(r.id, r.fn); err != nil {
			return fmt.Errorf("register %v: %w", r.id, err)
		}
	}
	return nil
}
