package transport

import (
	"context"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

func TestDispatcherUnknownMessageIsIgnored(t *testing.T) {
	t.Parallel()

	dispatcher := NewDispatcher()
	if err := dispatcher.HandlePacket(context.Background(), nil, protocol.Packet{Code: uint8(protocol.CM_PING)}); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Register(protocol.CM_PING, func(context.Context, *Session, protocol.Packet) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.HandlePacket(context.Background(), nil, protocol.Packet{Code: uint8(protocol.CM_PING)}); err != nil {
		t.Fatal(err)
	}
}
