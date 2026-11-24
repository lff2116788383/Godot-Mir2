package transport

import (
	"context"
	"fmt"
	"sync"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// Dispatcher routes decoded client messages to registered handlers.
type Dispatcher struct {
	mu     sync.RWMutex
	routes map[protocol.CMType]HandlerFunc
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{routes: make(map[protocol.CMType]HandlerFunc)}
}

// Register installs or replaces one client message handler.
func (d *Dispatcher) Register(id protocol.CMType, handler HandlerFunc) error {
	if handler == nil {
		return fmt.Errorf("handler for message 0x%02x is nil", uint8(id))
	}
	d.mu.Lock()
	d.routes[id] = handler
	d.mu.Unlock()
	return nil
}

// HandlePacket dispatches one packet and ignores unregistered message IDs for now.
func (d *Dispatcher) HandlePacket(ctx context.Context, session *Session, packet protocol.Packet) error {
	d.mu.RLock()
	handler := d.routes[protocol.CMType(packet.Code)]
	d.mu.RUnlock()
	if handler == nil {
		return nil
	}
	return handler(ctx, session, packet)
}

// NewBootstrapDispatcher creates the B3 service-core message table.
func NewBootstrapDispatcher() *Dispatcher {
	dispatcher := NewDispatcher()
	_ = dispatcher.Register(protocol.CM_PING, PingHandler{}.HandlePacket)
	return dispatcher
}
