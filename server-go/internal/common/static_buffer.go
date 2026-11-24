package common

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// StaticBuffer mirrors the semantic part of C++ StaticBuffer<Capacity>.
// The legacy wire representation is a uint16 length, Capacity+1 data bytes,
// and the natural two-byte alignment of the C++ struct. Capacity is stored at
// runtime because Go generic type parameters cannot carry integer constants.
type StaticBuffer struct {
	Limit int
	Data  []byte
}

// NewStaticBuffer creates a buffer with the requested legacy capacity.
func NewStaticBuffer(limit int) StaticBuffer {
	return StaticBuffer{Limit: limit}
}

// Capacity returns the configured compile-time capacity equivalent.
func (b StaticBuffer) Capacity() int {
	return b.Limit
}

// Len returns the current number of payload bytes.
func (b StaticBuffer) Len() int {
	return len(b.Data)
}

// Empty reports whether the buffer has no payload.
func (b StaticBuffer) Empty() bool {
	return len(b.Data) == 0
}

// Clear removes all payload bytes.
func (b *StaticBuffer) Clear() {
	b.Data = nil
}

// Assign replaces the payload after validating the static capacity.
func (b *StaticBuffer) Assign(data []byte) error {
	if b.Limit < 0 {
		return fmt.Errorf("invalid static buffer capacity %d", b.Limit)
	}
	if len(data) > b.Limit {
		return fmt.Errorf("static buffer length %d exceeds capacity %d", len(data), b.Limit)
	}
	b.Data = append(b.Data[:0], data...)
	return nil
}

// Bytes returns the payload.
func (b StaticBuffer) Bytes() []byte {
	return b.Data
}

// String returns the payload converted to a string.
func (b StaticBuffer) String() string {
	return string(b.Data)
}

// WireSize is sizeof(StaticBuffer<Capacity>) on the project's x64 MS ABI.
func (b StaticBuffer) WireSize() int {
	raw := 2 + (b.Limit + 1)
	if raw%2 != 0 {
		raw++
	}
	return raw
}

// AppendWire appends the packed legacy representation.
func (b StaticBuffer) AppendWire(dst []byte) ([]byte, error) {
	if b.Limit < 0 {
		return nil, fmt.Errorf("invalid static buffer capacity %d", b.Limit)
	}
	if len(b.Data) > b.Limit {
		return nil, fmt.Errorf("static buffer length %d exceeds capacity %d", len(b.Data), b.Limit)
	}
	start := len(dst)
	dst = append(dst, make([]byte, b.WireSize())...)
	binary.LittleEndian.PutUint16(dst[start:start+2], uint16(len(b.Data)))
	copy(dst[start+2:start+2+len(b.Data)], b.Data)
	return dst, nil
}

// ReadWire replaces b from a fixed-size legacy buffer.
func (b *StaticBuffer) ReadWire(src []byte) (int, error) {
	wireSize := b.WireSize()
	if len(src) < wireSize {
		return 0, fmt.Errorf("static buffer needs %d bytes, got %d", wireSize, len(src))
	}
	size := int(binary.LittleEndian.Uint16(src[:2]))
	if size > b.Limit {
		return 0, fmt.Errorf("static buffer length %d exceeds capacity %d", size, b.Limit)
	}
	b.Data = append(b.Data[:0], src[2:2+size]...)
	return wireSize, nil
}

var ErrStaticCollectionFull = errors.New("static collection is full")
