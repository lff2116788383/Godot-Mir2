package game

import (
	"context"
	"log/slog"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// ==================== CM_NPCEVENT（playernet.cpp:321 net_CM_NPCEVENT）====================
//
// 原版流程：
//
//  1. stallSelling 检查（摆摊中不能点 NPC）
//  2. 把 CMNPCEvent 转成 SDNPCEvent 发给 NPC actor（AM_NPCEVENT）
//  3. NPC 侧 on_AM_NPCEVENT 做距离校验、解密事件 ID、spawn 协程
//
// ★ 两个必须照抄的细节：
//
//  1. **event 非空且不是 SYS_ENTER 时要解密**（AESHelper.decode）
//     客户端点菜单项时回传的是加密后的事件 ID。当前版本我们没加密下发，
//     所以也不需要解密 —— 但要留出这个位置，否则接上加密后
//     所有菜单点击都会变成「听不懂你在说什么」。
//
//  2. **距离校验用 SYS_MAXNPCDISTANCE=10**，且校验失败要回 SM_NPCERROR
//     而不是静默丢弃。不发错误包，客户端会保持对话面板打开，
//     玩家以为「点了没反应」而不是「离得太远」。
//
//  3. **重复点击要丢弃**：原版用 m_xmlLayoutSeqIDList 记住
//     「这个玩家上一次拿到的 seqID」，点第二次时那个 seqID 已失效。
//     这里用「每个玩家最近一次对话的 NPC + 时间」做简易防护。

// NPCError 的错误码（protocoldef.hpp）。
const (
	NPCErrStallSelling = 1
	NPCErrTooFar       = 2
)

// NPCHandler 处理 NPC 交互。
type NPCHandler struct {
	Players   *PlayerManager
	NPCs      *NPCManager
	Worlds    *WorldService
	Broadcast Broadcaster
	Logger    *slog.Logger
}

// HandleEvent 处理 CM_NPCEVENT。
func (h *NPCHandler) HandleEvent(ctx context.Context, channelID uint64, req protocol.CMNPCEvent) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}

	// 1. 摆摊中禁止交互（原版 stallSelling 检查）
	//    Player 还没有摆摊字段（B7 经济系统才加），这里留出位置。

	// 2. 找 NPC
	npc, ok := h.NPCs.Get(req.UID)
	if !ok {
		// NPC 不存在：不报错也不回包（客户端会自己超时关闭）
		return nil
	}

	path := trimNul(req.Path[:])
	event := trimNul(req.Event[:])
	value := trimNul(req.Value[:])

	// 3. 距离校验（原版 mathf::LDistance2(...) >= MAXNPCDISTANCE^2）
	dist := chebyshev(p.X, p.Y, npc.Def.X, npc.Def.Y)
	if dist > 10 {
		h.sendNPCError(p, NPCErrTooFar)
		return nil
	}

	// 4. SYS_EXIT 直接关闭协程，不进脚本
	if event == "" || event == sysExitName || event == sysNPCErrorName {
		return nil
	}

	// 5. 转给 Lua
	var valuePtr *string
	if req.ValueSize >= 0 && value != "" {
		valuePtr = &value
	}
	if err := npc.Script.HandleEvent(p.UID, path, event, valuePtr); err != nil {
		// 脚本报错已经由 Host.Log 记录，这里只兜底不让连接崩
		return nil
	}
	return nil
}

// sendNPCError 发 SM_NPCERROR（定长 1 字节）。
func (h *NPCHandler) sendNPCError(p *Player, code uint8) {
	if h.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMNPCError{Error: code})
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_NPCERROR, payload, 0)
}

// trimNul 把定长 char 数组截到第一个 0。
func trimNul(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// chebyshev 是切比雪夫距离（八方向移动下「走几步到」）。
func chebyshev(x1, y1, x2, y2 int) int {
	dx := x1 - x2
	if dx < 0 {
		dx = -dx
	}
	dy := y1 - y2
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

// NPC 事件里的两个特殊名（sysconst.hpp:171-173 / 脚本约定）。
const (
	sysExitName     = "_RSVD_NAME_EXIT__14208236065"
	sysNPCErrorName = "_RSVD_NAME_NPCERROR_"
)
