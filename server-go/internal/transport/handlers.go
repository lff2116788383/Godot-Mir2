package transport

import (
	"context"
	"fmt"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// PingHandler implements the legacy CM_PING/SM_PING exchange.
type PingHandler struct{}

func (PingHandler) HandlePacket(_ context.Context, session *Session, packet protocol.Packet) error {
	if packet.Code != uint8(protocol.CM_PING) {
		return nil
	}
	var request protocol.CMPing
	consumed, err := protocol.ReadWirePayload(&request, packet.Payload)
	if err != nil {
		return fmt.Errorf("decode CMPing: %w", err)
	}
	if consumed != len(packet.Payload) {
		return fmt.Errorf("CMPing consumed %d of %d bytes", consumed, len(packet.Payload))
	}
	payload, err := protocol.AppendWirePayload(nil, request)
	if err != nil {
		return fmt.Errorf("encode SMPing: %w", err)
	}
	return session.Send(protocol.SM_PING, payload, packet.ResponseID)
}
