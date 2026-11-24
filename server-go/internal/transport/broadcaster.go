package transport

import (
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// ==================== 业务侧的主动推送通道 ====================
//
// game 包需要「按 channelID 主动给某个玩家发消息」（AOI 广播、伤害通知、
// 血量变更），但它不该直接持有 *Server。所以这里提供一个薄适配：
//
//	broadcaster := transport.NewBroadcaster(server)
//	actionHandler.Broadcast = broadcaster
//
// 语义：连接已断时 SendTo 静默返回 nil（不报错），
// 因为「对方掉线了」对广播方来说是正常情况，不该中断其他收件人的投递。

// Broadcaster 把消息发给指定连接。
type Broadcaster struct {
	server *Server
}

// NewBroadcaster 基于 TCP 服务创建一个推送器。
func NewBroadcaster(server *Server) *Broadcaster {
	return &Broadcaster{server: server}
}

// SendTo 发给单个连接。
//
// 连接不存在（已断开）时返回 nil —— 广播场景下这是正常情况，
// 返回错误会让调用方在循环里反复处理同一个失效连接。
func (b *Broadcaster) SendTo(channelID uint64, id protocol.SMType, payload []byte, responseID uint64) error {
	if b == nil || b.server == nil || channelID == 0 {
		return nil
	}
	session := b.server.SessionByID(channelID)
	if session == nil {
		return nil
	}
	return session.Send(id, payload, responseID)
}

// SessionCount 透传当前连接数（监控用）。
func (b *Broadcaster) SessionCount() int {
	if b == nil || b.server == nil {
		return 0
	}
	return b.server.SessionCount()
}
