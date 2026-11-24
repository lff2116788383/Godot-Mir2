package protocol

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestServerFixedRawPacket(t *testing.T) {
	t.Parallel()

	payload := []byte{0x78, 0x56, 0x34, 0x12}
	encoded, err := AppendServerPacket(nil, SM_PING, payload, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{byte(SM_PING), 0x78, 0x56, 0x34, 0x12}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("encoded = %x, want %x", encoded, want)
	}

	packet, err := NewPacketDecoder(bytes.NewReader(encoded)).ReadServerPacket()
	if err != nil {
		t.Fatal(err)
	}
	if packet.Code != uint8(SM_PING) || packet.HasResponse || !bytes.Equal(packet.Payload, payload) {
		t.Fatalf("decoded packet = %+v", packet)
	}
}

func TestClientResponsePacket(t *testing.T) {
	t.Parallel()

	payload := []byte{0x78, 0x56, 0x34, 0x12}
	encoded, err := AppendClientPacket(nil, CM_PING, payload, 300)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x81, 0xac, 0x02, 0x78, 0x56, 0x34, 0x12}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("encoded = %x, want %x", encoded, want)
	}

	packet, err := NewPacketDecoder(bytes.NewReader(encoded)).ReadClientPacket()
	if err != nil {
		t.Fatal(err)
	}
	if packet.Code != uint8(CM_PING) || !packet.HasResponse || packet.ResponseID != 300 {
		t.Fatalf("decoded packet = %+v", packet)
	}
}

func TestEmptyPacket(t *testing.T) {
	t.Parallel()

	encoded, err := AppendServerPacket(nil, SM_ERROR, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, []byte{byte(SM_ERROR)}) {
		t.Fatalf("encoded = %x", encoded)
	}

	packet, err := NewPacketDecoder(bytes.NewReader(encoded)).ReadServerPacket()
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Payload) != 0 {
		t.Fatalf("payload = %x", packet.Payload)
	}
}

func TestFixedCompressedPacket(t *testing.T) {
	t.Parallel()

	encoded, err := AppendServerPacket(nil, SM_LOGINERROR, []byte{1}, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{byte(SM_LOGINERROR), 1, 1, 1}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("encoded = %x, want %x", encoded, want)
	}

	packet, err := NewPacketDecoder(bytes.NewReader(encoded)).ReadServerPacket()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(packet.Payload, []byte{1}) {
		t.Fatalf("payload = %x", packet.Payload)
	}
}

func TestWideFixedCompressedPacket(t *testing.T) {
	t.Parallel()

	attr, ok := ClientMessageAttribute(CM_CREATECHATGROUP)
	if !ok {
		t.Fatal("missing CM_CREATECHATGROUP")
	}
	payload := make([]byte, attr.DataLen)
	encoded, err := AppendClientPacket(nil, CM_CREATECHATGROUP, payload, 0)
	if err != nil {
		t.Fatal(err)
	}

	packet, err := NewPacketDecoder(bytes.NewReader(encoded)).ReadClientPacket()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(packet.Payload, payload) {
		t.Fatal("wide packet payload mismatch")
	}
}

func TestVariablePacket(t *testing.T) {
	t.Parallel()

	payload := []byte("hello chat")
	encoded, err := AppendClientPacket(nil, CM_CHATMESSAGE, payload, 0)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := NewPacketDecoder(bytes.NewReader(encoded)).ReadClientPacket()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(packet.Payload, payload) {
		t.Fatalf("payload = %x", packet.Payload)
	}
}

func TestVariablePayloadLimit(t *testing.T) {
	t.Parallel()

	encoded, err := AppendClientPacket(nil, CM_CHATMESSAGE, []byte("12345"), 0)
	if err != nil {
		t.Fatal(err)
	}
	decoder := NewPacketDecoder(bytes.NewReader(encoded))
	decoder.MaxVariablePayload = 4
	if _, err := decoder.ReadClientPacket(); err == nil {
		t.Fatal("expected payload limit error")
	}
}

func TestPayloadValidation(t *testing.T) {
	t.Parallel()

	if _, err := AppendClientPacket(nil, CM_LOGIN, []byte{1}, 0); err == nil {
		t.Fatal("expected fixed payload validation error")
	}
	if _, err := AppendServerPacket(nil, SM_ERROR, []byte{1}, 0); err == nil {
		t.Fatal("expected empty payload validation error")
	}
}

func TestUnknownAndTruncatedPackets(t *testing.T) {
	t.Parallel()

	if _, err := NewPacketDecoder(bytes.NewReader([]byte{0x7f})).ReadClientPacket(); err == nil {
		t.Fatal("expected unknown message error")
	}

	if _, err := NewPacketDecoder(bytes.NewReader([]byte{byte(SM_PING), 1, 2})).ReadServerPacket(); err == nil {
		t.Fatal("expected truncated payload error")
	} else if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected error type: %v", err)
	}
}
