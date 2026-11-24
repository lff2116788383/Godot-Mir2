package protocol

import (
	"errors"
	"fmt"
	"io"
)

const (
	// DefaultMaxVariablePayload bounds type-3 payloads before allocation.
	DefaultMaxVariablePayload uint64 = 16 << 20
)

// Packet is one decoded legacy client/server message frame.
type Packet struct {
	Code        uint8
	HasResponse bool
	ResponseID  uint64
	Payload     []byte
}

// PacketDecoder reads legacy message frames from a stream.
type PacketDecoder struct {
	Reader             io.Reader
	MaxVariablePayload uint64
}

// NewPacketDecoder creates a decoder with the default payload limit.
func NewPacketDecoder(r io.Reader) *PacketDecoder {
	return &PacketDecoder{
		Reader:             r,
		MaxVariablePayload: DefaultMaxVariablePayload,
	}
}

// ReadClientPacket reads one client-to-server packet.
func (d *PacketDecoder) ReadClientPacket() (Packet, error) {
	return d.readPacket(clientMessageAttributeForHead)
}

// ReadServerPacket reads one server-to-client packet.
func (d *PacketDecoder) ReadServerPacket() (Packet, error) {
	return d.readPacket(serverMessageAttributeForHead)
}

func (d *PacketDecoder) readPacket(lookup func(uint8) (MessageAttribute, bool)) (Packet, error) {
	if d.Reader == nil {
		return Packet{}, errors.New("nil packet reader")
	}

	var headBuf [1]byte
	if _, err := io.ReadFull(d.Reader, headBuf[:]); err != nil {
		return Packet{}, fmt.Errorf("read message head: %w", err)
	}
	rawHead := headBuf[0]
	hasResponse := rawHead&0x80 != 0
	code := rawHead & 0x7f

	attr, ok := lookup(code)
	if !ok {
		return Packet{}, fmt.Errorf("unknown message head code 0x%02x", code)
	}

	packet := Packet{
		Code:        code,
		HasResponse: hasResponse,
	}
	if hasResponse {
		responseID, err := readUvarint(d.Reader)
		if err != nil {
			return Packet{}, fmt.Errorf("%s: read response id: %w", attr.Name, err)
		}
		packet.ResponseID = responseID
	}

	var err error
	switch attr.Kind {
	case MessageEmpty:
		return packet, nil
	case MessageFixedRaw:
		packet.Payload, err = readFixedPayload(d.Reader, attr.DataLen)
	case MessageVariable:
		packet.Payload, err = d.readVariablePayload(attr.Name)
	case MessageFixedCompressed:
		packet.Payload, err = d.readCompressedPayload(attr)
	default:
		err = fmt.Errorf("%s has invalid message kind %d", attr.Name, attr.Kind)
	}
	if err != nil {
		return Packet{}, fmt.Errorf("%s: %w", attr.Name, err)
	}
	return packet, nil
}

func readFixedPayload(r io.Reader, size int) ([]byte, error) {
	if size < 0 {
		return nil, fmt.Errorf("negative fixed payload size %d", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("read fixed payload: %w", err)
	}
	return payload, nil
}

func (d *PacketDecoder) readVariablePayload(name string) ([]byte, error) {
	limit := d.MaxVariablePayload
	if limit == 0 {
		limit = DefaultMaxVariablePayload
	}
	size, err := readBoundedUvarint(d.Reader, limit, name+" payload length")
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	if size > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("payload length %d overflows int", size)
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(d.Reader, payload); err != nil {
		return nil, fmt.Errorf("read variable payload: %w", err)
	}
	return payload, nil
}

func (d *PacketDecoder) readCompressedPayload(attr MessageAttribute) ([]byte, error) {
	compressedSize, err := readBoundedUvarint(d.Reader, uint64(attr.DataLen), attr.Name+" compressed payload length")
	if err != nil {
		return nil, err
	}

	wide := attr.DataLen >= 256
	maskLen := (attr.DataLen + 7) / 8
	if wide {
		maskLen = (attr.DataLen + 63) / 64
	}
	if compressedSize > uint64(attr.DataLen) {
		return nil, fmt.Errorf("compressed size %d exceeds payload size %d", compressedSize, attr.DataLen)
	}
	total := uint64(maskLen) + compressedSize
	if total > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("compressed frame length %d overflows int", total)
	}
	encoded := make([]byte, int(total))
	if _, err := io.ReadFull(d.Reader, encoded); err != nil {
		return nil, fmt.Errorf("read compressed payload: %w", err)
	}

	payload := make([]byte, attr.DataLen)
	decodedBytes, err := xorDecode(payload, encoded, wide)
	if err != nil {
		return nil, err
	}
	if decodedBytes != int(compressedSize) {
		return nil, fmt.Errorf("decoded %d bytes, frame declared %d", decodedBytes, compressedSize)
	}
	return payload, nil
}

// AppendClientPacket appends one client-to-server packet to dst.
func AppendClientPacket(dst []byte, id CMType, payload []byte, responseID uint64) ([]byte, error) {
	attr, ok := ClientMessageAttribute(id)
	if !ok {
		return nil, fmt.Errorf("unknown client message 0x%02x", uint8(id))
	}
	return appendPacket(dst, uint8(id), attr, payload, responseID)
}

// AppendServerPacket appends one server-to-client packet to dst.
func AppendServerPacket(dst []byte, id SMType, payload []byte, responseID uint64) ([]byte, error) {
	attr, ok := ServerMessageAttribute(id)
	if !ok {
		return nil, fmt.Errorf("unknown server message 0x%02x", uint8(id))
	}
	return appendPacket(dst, uint8(id), attr, payload, responseID)
}

func appendPacket(dst []byte, code uint8, attr MessageAttribute, payload []byte, responseID uint64) ([]byte, error) {
	if err := attr.ValidatePayload(payload); err != nil {
		return nil, err
	}

	head := code & 0x7f
	if responseID != 0 {
		head |= 0x80
	}
	dst = append(dst, head)
	if responseID != 0 {
		dst = appendUvarint(dst, responseID)
	}

	switch attr.Kind {
	case MessageEmpty:
		return dst, nil
	case MessageFixedRaw:
		return append(dst, payload...), nil
	case MessageVariable:
		dst = appendUvarint(dst, uint64(len(payload)))
		return append(dst, payload...), nil
	case MessageFixedCompressed:
		var encoded []byte
		if attr.DataLen >= 256 {
			encoded = xorEncode64(payload)
		} else {
			encoded = xorEncode(payload)
		}
		maskLen := (attr.DataLen + 7) / 8
		if attr.DataLen >= 256 {
			maskLen = (attr.DataLen + 63) / 64
		}
		compressedSize := len(encoded) - maskLen
		dst = appendUvarint(dst, uint64(compressedSize))
		return append(dst, encoded...), nil
	default:
		return nil, fmt.Errorf("%s has invalid message kind %d", attr.Name, attr.Kind)
	}
}
