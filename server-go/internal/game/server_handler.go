package game

import (
	"context"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/transport"
)

// 连接生命周期与协议分发的包装。
//
// transport.Options.Handler 同时承担两件事：
//  1. HandlePacket   —— 处理每条消息
//  2. SessionClosed  —— 连接断开时清理登录态（通过类型断言调用）
//
// transport 用「类型断言 options.Handler.(SessionLifecycleHandler)」来发现
// 第二个能力，所以这里必须把两个方法放在**同一个对象**上，
// 不能拆成两个东西传进去。

// ServerHandler 把 dispatcher 与登录会话清理组合起来。
type ServerHandler struct {
	dispatcher   *transport.Dispatcher
	handler      *Handler
	onDisconnect func(ctx context.Context, channelID uint64)
}

// SetDisconnectHook 设置断线处理（用于把玩家从地图移除）。
func (s *ServerHandler) SetDisconnectHook(fn func(ctx context.Context, channelID uint64)) {
	s.onDisconnect = fn
}

// NewServerHandler 装配一个既能分发消息、又能清理会话的 Handler。
func NewServerHandler(dispatcher *transport.Dispatcher, handler *Handler) *ServerHandler {
	return &ServerHandler{dispatcher: dispatcher, handler: handler}
}

// HandlePacket 转发给 dispatcher。
func (s *ServerHandler) HandlePacket(ctx context.Context, session *transport.Session, packet protocol.Packet) error {
	return s.dispatcher.HandlePacket(ctx, session, packet)
}

// SessionOpened 目前不需要做什么（会话在首次 CM_LOGIN 时才建立登录态）。
func (s *ServerHandler) SessionOpened(context.Context, *transport.Session) {}

// SessionClosed 清理登录态。
//
// 不清理会导致「客户端断线重连后被判 MULTILOGIN」——
// 原版靠 channID 复用规避，Go 侧必须显式清理。
func (s *ServerHandler) SessionClosed(ctx context.Context, session *transport.Session) {
	// 先让玩家离开世界（移除地图实体 + 广播消失），再清会话
	if s.onDisconnect != nil {
		s.onDisconnect(ctx, session.ID())
	}
	s.handler.SessionClosed(session)
}

// RegisterBootstrapRoutes 注册当前批次已实现的全部消息。
func RegisterBootstrapRoutes(dispatcher *transport.Dispatcher, handler *Handler) error {
	// CM_PING 保留（客户端连上后先 ping 一次）
	if err := dispatcher.Register(protocol.CM_PING, transport.PingHandler{}.HandlePacket); err != nil {
		return err
	}
	return RegisterLoginRoutes(dispatcher, handler)
}

// RegisterAllRoutes 注册登录 + 动作 + 背包装备 + 技能/死亡/商店（B5 完整装配）。
//
// 后三个 handler 允许为 nil（早期测试只装了前两个），
// 传 nil 就跳过对应路由。
func RegisterAllRoutes(dispatcher *transport.Dispatcher, handler *Handler,
	actions *ActionHandler, inventory *InventoryHandler,
	spells *SpellHandler, deaths *DeathHandler, shops *ShopHandler) error {
	if err := RegisterBootstrapRoutes(dispatcher, handler); err != nil {
		return err
	}
	if err := RegisterActionRoutes(dispatcher, actions); err != nil {
		return err
	}
	if err := RegisterInventoryRoutes(dispatcher, inventory); err != nil {
		return err
	}
	if err := RegisterSpellRoutes(dispatcher, spells); err != nil {
		return err
	}
	if err := RegisterDeathRoutes(dispatcher, deaths); err != nil {
		return err
	}
	return RegisterShopRoutes(dispatcher, shops)
}
