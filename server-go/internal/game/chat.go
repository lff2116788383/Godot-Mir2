package game

import (
	"context"
	"log/slog"
	"strings"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// ==================== 聊天（CM_CHATMESSAGE / CM_PLAYERSAY / CM_PLAYERBROADCAST）====================
//
// 【依据】playernet.cpp:577 net_CM_CHATMESSAGE / :650 net_CM_PLAYERSAY / :678 net_CM_PLAYERBROADCAST
//
// 三条消息的区别（很容易混）：
//
//	CM_PLAYERSAY        「原地说话」—— 半透明白字飘在头顶，视野内可见，不入库
//	CM_PLAYERBROADCAST  「喇叭」—— 全服可见，要花钱（原版走 serviceCore 扣金币）
//	CM_CHATMESSAGE      「私聊/群聊」—— 走会话面板，**入库**，有消息 ID
//
// 之前服务端三条全没注册，客户端 `say()` 发出去了但没有任何反应 ——
// 表现是「按回车打字，字打出去了，谁也看不到」。
// 这是 MMORPG 最基础的社交功能，缺了游戏就是单机。

// ChatHandler 处理所有聊天类消息。
type ChatHandler struct {
	Players   *PlayerManager
	Sessions  *SessionManager
	Broadcast Broadcaster
	Logger    *slog.Logger
	// Chat 聊天记录持久化。为 nil 时 CM_CHATMESSAGE 退化成「发出去但不入库」。
	Chat ChatRepository
	// Viewers 返回「能看到某格」的实体 UID 列表（原地说话用）。
	//
	// 抽成函数而不是直接依赖 world.World：
	// 这样 handler 不必知道 AOI 怎么算，测试也能给固定答案。
	Viewers func(ctx context.Context, mapID uint32, x, y int) []uint64
}

// ChatRepository 是聊天持久化的最小接口。
//
// 只声明 handler 真正用到的两个方法，不直接依赖 storage 包 ——
// 这样测试可以注入内存实现，也不必让 game 包知道 SQL 细节。
type ChatRepository interface {
	SaveMessage(ctx context.Context, from, to protocol.SDChatPeerID, message string, refer *uint64) (protocol.SDChatMessageDBSeq, error)
	LoadGroupMemberList(ctx context.Context, groupID uint32) ([]uint32, error)
}

// ChatPeer 类型（protocoldef.hpp:547-551）。
//
// 直接复用 protocol 包的 CPR_* 常量，不另建一套 ——
// 两套常量迟早对不上（新增一种 peer 类型时只改了一边）。
const (
	ChatPeerNone    = protocol.CPR_NONE
	ChatPeerSpecial = protocol.CPR_SPECIAL
	ChatPeerPlayer  = protocol.CPR_PLAYER
	ChatPeerGroup   = protocol.CPR_GROUP
)

// Say 处理 CM_PLAYERSAY（原地说话）。
//
// 【依据】playernet.cpp:650-676。原版逻辑：
//
//  1. 空内容直接返回（str_haschar 判「有没有可见字符」）
//  2. 先 postNetMessage(SM_PLAYERSAY) 发给自己
//  3. 再遍历 m_inViewCOList，只发给视野内的玩家
//
// ★ 为什么自己也要发一份：
//
//	客户端本地不显示自己说的话，全靠这条回包。
//	不回给自己，玩家会以为「字没打出去」而反复重打。
//
// ★ 只发视野内的玩家，不是全图：
//
//	原地说话是「喊一声」，隔了半张地图还听得见就不合理。
//	原版用 m_inViewCOList（视野内实体列表）。
func (h *ChatHandler) Say(ctx context.Context, channelID uint64, req protocol.CMPlayerSay) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	content := trimCString(req.Content[:])
	if !hasVisibleChar(content) {
		return nil
	}

	payload, err := protocol.AppendWirePayload(nil, protocol.SMPlayerSay{
		UID:     p.UID,
		Content: truncateCString(content, 128),
	})
	if err != nil {
		return nil
	}

	// 1. 发给自己
	if h.Broadcast != nil && p.ChannelID != 0 {
		_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_PLAYERSAY, payload, 0)
	}

	// 2. 发给视野内的其他玩家
	h.broadcastToViewers(ctx, p, protocol.SM_PLAYERSAY, payload)
	return nil
}

// BroadcastSay 处理 CM_PLAYERBROADCAST（喇叭/全服广播）。
//
// 【依据】playernet.cpp:678-694。原版是转给 serviceCore 统一处理
// （那里要扣金币、做频率限制、记日志）。
//
// ★ 当前版本直接全服广播，**没有扣金币也没有限流**：
//
//	原版的喇叭要花钱买，所以不怕刷屏；免费广播会被用来刷屏骚扰。
//	已在 REPLICA_AUDIT 记为「需要接经济系统」的待办。
//	真接之前建议限制频率（比如每人 10 秒一条）。
func (h *ChatHandler) BroadcastSay(ctx context.Context, channelID uint64, req protocol.CMPlayerBroadcast) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	content := trimCString(req.Content[:])
	if !hasVisibleChar(content) {
		return nil
	}

	payload, err := protocol.AppendWirePayload(nil, protocol.SMPlayerBroadcast{
		UID:     p.UID,
		Content: truncateCString(content, 128),
	})
	if err != nil {
		return nil
	}

	// 全服所有在线玩家
	if h.Broadcast == nil {
		return nil
	}
	for _, other := range h.Players.Snapshot() {
		if other.ChannelID == 0 {
			continue
		}
		_ = h.Broadcast.SendTo(other.ChannelID, protocol.SM_PLAYERBROADCAST, payload, 0)
	}
	return nil
}

// ChatMessage 处理 CM_CHATMESSAGE（私聊 / 群聊，入库）。
//
// 【依据】playernet.cpp:577-647。原版的完整流程：
//
//  1. 解析 CMChatMessageHeader（toCPID / hasRef / refID）+ 正文
//  2. dbSaveChatMessage 入库，拿到 (msgId, tstamp)
//  3. 按 toCPID.type 分流：
//     CPR_PLAYER  → 只发给对方（且不是自己）
//     CPR_GROUP   → 查群成员，发给除自己外的所有人
//     CPR_SPECIAL → 什么都不做（系统会话，客户端只读）
//  4. postNetMessage(SM_CHATMESSAGEOK, SDChatMessageDBSeq) 回给自己
//
// ★ 即使发给别人，也要给自己回 SM_CHATMESSAGEOK：
//
//	客户端靠这条消息把「发送中」的消息替换成「已送达」（带 msgId）。
//	不回的话，自己发出去的气泡会一直显示转圈。
func (h *ChatHandler) ChatMessage(ctx context.Context, channelID uint64, req protocol.CMChatMessageHeader, body string) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	body = strings.TrimSpace(body)
	if !hasVisibleChar(body) {
		return nil
	}

	to := protocol.SDChatPeerID{Data: req.ToCPID}
	from := protocol.NewSDChatPeerID(ChatPeerPlayer, p.DBID)

	// 入库。失败也要继续发 —— 聊天记录丢了不该阻塞即时通讯。
	seq := protocol.SDChatMessageDBSeq{}
	if h.Chat != nil {
		var refer *uint64
		if req.HasRef {
			r := req.RefID
			refer = &r
		}
		saved, err := h.Chat.SaveMessage(ctx, from, to, body, refer)
		if err != nil {
			h.log("save chat message failed", err)
		} else {
			seq = saved
		}
	}

	// 分发给接收方
	switch to.Type() {
	case ChatPeerPlayer:
		// 发给自己以外的那个玩家
		if to.ID() != p.DBID {
			h.deliverChat(ctx, to.ID(), seq, from, to, body, req, p)
		}
	case ChatPeerGroup:
		if h.Chat != nil {
			members, err := h.Chat.LoadGroupMemberList(ctx, to.ID())
			if err != nil {
				h.log("load group members failed", err)
			}
			for _, memberDBID := range members {
				if memberDBID == p.DBID {
					continue
				}
				h.deliverChat(ctx, memberDBID, seq, from, to, body, req, p)
			}
		}
	case ChatPeerSpecial:
		// 系统会话：客户端只读，服务端不需要分发
	default:
		h.log("unknown chat peer type", nil)
	}

	// 回执（客户端据此把消息标记成「已送达」）
	if h.Broadcast != nil && p.ChannelID != 0 {
		payload, err := protocol.CerealSerialize(seq)
		if err == nil {
			_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_CHATMESSAGEOK, payload, 0)
		}
	}
	return nil
}

// deliverChat 把一条聊天消息推给某个角色。
func (h *ChatHandler) deliverChat(ctx context.Context, toDBID uint32, seq protocol.SDChatMessageDBSeq,
	from, to protocol.SDChatPeerID, body string, req protocol.CMChatMessageHeader, sender *Player) {
	target, ok := h.Players.ByDBID(toDBID)
	if !ok || target.ChannelID == 0 {
		return
	}
	list := protocol.SDChatMessageList{{
		HasSeq: true, Seq: seq,
		HasRefer: req.HasRef, Refer: req.RefID,
		From: from, To: to,
		Message: body,
	}}
	payload, err := protocol.CerealSerialize(list)
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(target.ChannelID, protocol.SM_CHATMESSAGELIST, payload, 0)
}

// broadcastToViewers 把一条消息发给「能看到发言者」的玩家（不含发言者自己）。
//
// 用 world 层的观察圈而不是「同图所有人」：
// 原地说话只该传几十格，不该传到地图另一端。
func (h *ChatHandler) broadcastToViewers(ctx context.Context, speaker *Player, id protocol.SMType, payload []byte) {
	if h.Broadcast == nil || h.Viewers == nil {
		return
	}
	uids := h.Viewers(ctx, speaker.MapID, speaker.X, speaker.Y)
	for _, uid := range uids {
		if uid == speaker.UID {
			continue
		}
		target, ok := h.Players.Get(uid)
		if !ok || target.ChannelID == 0 {
			continue
		}
		_ = h.Broadcast.SendTo(target.ChannelID, id, payload, 0)
	}
}

func (h *ChatHandler) log(msg string, err error) {
	if h.Logger == nil {
		return
	}
	if err != nil {
		h.Logger.Warn(msg, "error", err)
		return
	}
	h.Logger.Debug(msg)
}

// ==================== 辅助 ====================

// trimCString 把定长 char 数组转成去掉尾部 0 的字符串。
//
// 客户端的 content[128] 是定长数组，没用满的部分是 0。
// 不裁掉的话，日志和广播里会带一串不可见字符。
func trimCString(buf []byte) string {
	end := len(buf)
	for end > 0 && buf[end-1] == 0 {
		end--
	}
	return string(buf[:end])
}

// truncateCString 把字符串塞进定长 char 数组，超长截断并补 0。
//
// ★ 必须按**字符边界**截断，不能直接切字节：
//
//	中文一个字 3 字节，切在中间会产生非法 UTF-8，
//	客户端解析时整条消息会变成乱码或直接丢弃。
func truncateCString(s string, size int) [128]byte {
	var out [128]byte
	if size > len(out) {
		size = len(out)
	}
	// 留一个字节给结尾 0
	limit := size - 1
	if len(s) <= limit {
		copy(out[:], s)
		return out
	}
	// 往前退到 UTF-8 字符边界
	cut := limit
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	copy(out[:], s[:cut])
	return out
}

// hasVisibleChar 对应原版 str_haschar：判断有没有可显示字符。
//
// 全空白（空格/换行/制表）视为空 —— 否则玩家可以刷一屏空白气泡。
func hasVisibleChar(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}
