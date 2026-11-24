package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

var (
	ErrServerClosed  = errors.New("transport server closed")
	ErrSessionClosed = errors.New("session closed")
	ErrSendQueueFull = errors.New("session send queue full")
)

// Handler processes one decoded client packet.
type Handler interface {
	HandlePacket(context.Context, *Session, protocol.Packet) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(context.Context, *Session, protocol.Packet) error

func (f HandlerFunc) HandlePacket(ctx context.Context, session *Session, packet protocol.Packet) error {
	return f(ctx, session, packet)
}

// SessionLifecycleHandler receives asynchronous open/close notifications.
type SessionLifecycleHandler interface {
	SessionOpened(context.Context, *Session)
	SessionClosed(context.Context, *Session)
}

// Options controls one legacy TCP server.
type Options struct {
	Address            string
	Handler            Handler
	Logger             *slog.Logger
	MaxSessions        int
	SendQueueSize      int
	MaxVariablePayload uint64
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
}

func normalizeOptions(options Options) Options {
	if options.Address == "" {
		options.Address = "127.0.0.1:7000"
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.MaxSessions <= 0 {
		options.MaxSessions = 1024
	}
	if options.SendQueueSize <= 0 {
		options.SendQueueSize = 256
	}
	if options.MaxVariablePayload == 0 {
		options.MaxVariablePayload = protocol.DefaultMaxVariablePayload
	}
	if options.ReadTimeout < 0 {
		options.ReadTimeout = 0
	}
	if options.WriteTimeout <= 0 {
		options.WriteTimeout = 10 * time.Second
	}
	return options
}

// Server owns the legacy client TCP listener and sessions.
type Server struct {
	options Options
	logger  *slog.Logger
	nextID  atomic.Uint64

	mu       sync.Mutex
	listener net.Listener
	sessions map[uint64]*Session
	wg       sync.WaitGroup
	closed   bool
}

func NewServer(options Options) *Server {
	options = normalizeOptions(options)
	return &Server{options: options, logger: options.Logger, sessions: make(map[uint64]*Session)}
}

// SetHandler 在服务创建之后回填 Handler。
//
// 为什么需要延迟设置：业务层要主动推消息（AOI 广播），需要 Server 句柄；
// 而 Server 又需要 Handler 才能启动 —— 用 SetHandler 打破这个循环。
// 必须在 ListenAndServe/Serve 之前调用。
func (s *Server) SetHandler(handler Handler) {
	s.mu.Lock()
	s.options.Handler = handler
	s.mu.Unlock()
}

// handler 返回当前 Handler（带锁读取，支持延迟设置）。
func (s *Server) handler() Handler {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.options.Handler
}

// ListenAndServe binds Address and blocks until ctx is canceled or the server fails.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.handler() == nil {
		return errors.New("transport handler is nil")
	}
	listener, err := net.Listen("tcp", s.options.Address)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.options.Address, err)
	}
	return s.Serve(ctx, listener)
}

// Serve accepts connections on listener until ctx is canceled.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if listener == nil {
		return errors.New("listener is nil")
	}
	if s.handler() == nil {
		return errors.New("transport handler is nil")
	}
	s.mu.Lock()
	if s.listener != nil || s.closed {
		s.mu.Unlock()
		_ = listener.Close()
		return ErrServerClosed
	}
	s.listener = listener
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
		case <-done:
		}
	}()
	defer close(done)

	var acceptDelay time.Duration
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				s.closeSessions()
				s.wg.Wait()
				return nil
			}
			if temporary, ok := err.(interface{ Temporary() bool }); ok && temporary.Temporary() {
				if acceptDelay == 0 {
					acceptDelay = 5 * time.Millisecond
				} else {
					acceptDelay *= 2
				}
				if acceptDelay > time.Second {
					acceptDelay = time.Second
				}
				select {
				case <-time.After(acceptDelay):
					continue
				case <-ctx.Done():
					s.closeSessions()
					s.wg.Wait()
					return nil
				}
			}
			return fmt.Errorf("accept: %w", err)
		}
		acceptDelay = 0
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetKeepAlive(true)
			_ = tcpConn.SetKeepAlivePeriod(30 * time.Second)
		}
		id := s.nextID.Add(1)
		session := newSession(ctx, id, conn, s.options)
		s.mu.Lock()
		if s.closed || len(s.sessions) >= s.options.MaxSessions {
			s.mu.Unlock()
			_ = conn.Close()
			continue
		}
		s.sessions[id] = session
		s.wg.Add(1)
		s.mu.Unlock()
		go s.runSession(ctx, session)
	}
}

func (s *Server) runSession(ctx context.Context, session *Session) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.sessions, session.id)
		s.mu.Unlock()
		_ = session.Close()
		if lifecycle, ok := s.handler().(SessionLifecycleHandler); ok {
			lifecycle.SessionClosed(ctx, session)
		}
	}()
	if lifecycle, ok := s.handler().(SessionLifecycleHandler); ok {
		lifecycle.SessionOpened(ctx, session)
	}
	if err := session.run(ctx, s.handler()); err != nil && !errors.Is(err, ErrSessionClosed) && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) && ctx.Err() == nil {
		s.logger.Info("session closed with error", "session", session.id, "remote", session.remoteAddr, "error", err)
	}
}

func (s *Server) closeSessions() {
	s.mu.Lock()
	s.closed = true
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()
	for _, session := range sessions {
		_ = session.Close()
	}
}

// Close stops the listener and all active sessions.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	listener := s.listener
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	for _, session := range sessions {
		_ = session.Close()
	}
	return nil
}

// Addr returns the active listener address.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *Server) SessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// SessionByID 按连接 ID 取会话。
//
// 业务层需要「按 channelID 主动推消息」（AOI 广播、伤害通知），
// 而 Session.Send 挂在会话上，所以要能按 ID 反查。
// 返回 nil 表示连接已断，调用方应静默丢弃消息。
func (s *Server) SessionByID(id uint64) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

// Session is one accepted client connection.
type Session struct {
	id         uint64
	conn       net.Conn
	remoteAddr string
	connected  time.Time
	options    Options
	sendQueue  chan []byte
	done       chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
	closeOnce  sync.Once
}

func newSession(parent context.Context, id uint64, conn net.Conn, options Options) *Session {
	ctx, cancel := context.WithCancel(parent)
	return &Session{
		id: id, conn: conn, remoteAddr: conn.RemoteAddr().String(), connected: time.Now(),
		options: options, sendQueue: make(chan []byte, options.SendQueueSize), done: make(chan struct{}),
		ctx: ctx, cancel: cancel,
	}
}

func (s *Session) ID() uint64               { return s.id }
func (s *Session) RemoteAddr() string       { return s.remoteAddr }
func (s *Session) ConnectedAt() time.Time   { return s.connected }
func (s *Session) Context() context.Context { return s.ctx }

func (s *Session) run(ctx context.Context, handler Handler) error {
	writerErr := make(chan error, 1)
	go func() { writerErr <- s.writeLoop(s.ctx) }()

	decoder := &protocol.PacketDecoder{Reader: s.conn, MaxVariablePayload: s.options.MaxVariablePayload}
	var readErr error
	for {
		if s.options.ReadTimeout > 0 {
			_ = s.conn.SetReadDeadline(time.Now().Add(s.options.ReadTimeout))
		}
		packet, err := decoder.ReadClientPacket()
		if err != nil {
			readErr = err
			break
		}
		if err := handler.HandlePacket(s.ctx, s, packet); err != nil {
			readErr = fmt.Errorf("handle packet 0x%02x: %w", packet.Code, err)
			break
		}
	}
	_ = s.Close()
	writeErr := <-writerErr
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, net.ErrClosed) {
		return readErr
	}
	return writeErr
}

func (s *Session) writeLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ErrSessionClosed
		case <-s.done:
			return ErrSessionClosed
		case frame := <-s.sendQueue:
			if s.options.WriteTimeout > 0 {
				_ = s.conn.SetWriteDeadline(time.Now().Add(s.options.WriteTimeout))
			}
			if _, err := s.conn.Write(frame); err != nil {
				return err
			}
		}
	}
}

// Send encodes and queues one server packet.
func (s *Session) Send(id protocol.SMType, payload []byte, responseID uint64) error {
	frame, err := protocol.AppendServerPacket(nil, id, payload, responseID)
	if err != nil {
		return err
	}
	select {
	case <-s.done:
		return ErrSessionClosed
	case s.sendQueue <- frame:
		return nil
	default:
		return ErrSendQueueFull
	}
}

// Close closes the connection once.
func (s *Session) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		s.cancel()
		err = s.conn.Close()
	})
	return err
}
