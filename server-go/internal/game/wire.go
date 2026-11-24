package game

import (
	"context"
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
