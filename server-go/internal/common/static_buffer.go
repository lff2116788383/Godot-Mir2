package common

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// StaticBuffer mirrors C++ StaticBuffer<Capacity>.
// The legacy wire representation is a uint16 length, Capacity+1 data bytes,
// and the natural two-byte alignment of the C++ struct.
type StaticBuffer[C Capacity] struct {
	Data []byte
}

// Capacity returns the compile-time capacity.
func (StaticBuffer[C]) Capacity() int {
	var capacity C
	return capacity.WireCapacity()
}

// Len returns the current number of payload bytes.
func (b StaticBuffer[C]) Len() int {
	return len(b.Data)
}

// Empty reports whether the buffer has no payload.
func (b StaticBuffer[C]) Empty() bool {
	return len(b.Data) == 0
}

// Clear removes all payload bytes.
func (b *StaticBuffer[C]) Clear() {
	b.Data = nil
}

// Assign replaces the payload after validating the static capacity.
func (b *StaticBuffer[C]) Assign(data []byte) error {
	if len(data) > b.Capacity() {
		return fmt.Errorf("static buffer length %d exceeds capacity %d", len(data), b.Capacity())
	}
	b.Data = append(b.Data[:0], data...)
	return nil
}

// Bytes returns the payload.
func (b StaticBuffer[C]) Bytes() []byte {
	return b.Data
}

// String returns the payload converted to a string.
func (b StaticBuffer[C]) String() string {
	return string(b.Data)
}

// WireSize is sizeof(StaticBuffer<Capacity>) on the project's x64 MS ABI.
func (b StaticBuffer[C]) WireSize() int {
	raw := 2 + (b.Capacity() + 1)
	if raw%2 != 0 {
		raw++
	}
	return raw
}

// AppendWire appends the packed legacy representation.
func (b StaticBuffer[C]) AppendWire(dst []byte) ([]byte, error) {
	if len(b.Data) > b.Capacity() {
		return nil, fmt.Errorf("static buffer length %d exceeds capacity %d", len(b.Data), b.Capacity())
	}
	start := len(dst)
	dst = append(dst, make([]byte, b.WireSize())...)
	binary.LittleEndian.PutUint16(dst[start:start+2], uint16(len(b.Data)))
	copy(dst[start+2:start+2+len(b.Data)], b.Data)
	return dst, nil
}

// ReadWire replaces b from a fixed-size legacy buffer.
func (b *StaticBuffer[C]) ReadWire(src []byte) (int, error) {
	wireSize := b.WireSize()
	if len(src) < wireSize {
		return 0, fmt.Errorf("static buffer needs %d bytes, got %d", wireSize, len(src))
	}
	size := int(binary.LittleEndian.Uint16(src[:2]))
	if size > b.Capacity() {
		return 0, fmt.Errorf("static buffer length %d exceeds capacity %d", size, b.Capacity())
	}
	b.Data = append(b.Data[:0], src[2:2+size]...)
	return wireSize, nil
}

var ErrStaticCollectionFull = errors.New("static collection is full")
