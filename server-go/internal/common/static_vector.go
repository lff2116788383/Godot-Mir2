package common

import (
	"encoding/binary"
	"fmt"
)

// StaticVector mirrors C++ StaticVector<T, Capacity>.
// It implements the packed wire representation for the primitive element
// types used by the legacy protocol.
type StaticVector[T any, C Capacity] struct {
	Data []T
}

// Capacity returns the compile-time capacity.
func (StaticVector[T, C]) Capacity() int {
	var capacity C
	return capacity.WireCapacity()
}

// Len returns the current number of elements.
func (v StaticVector[T, C]) Len() int {
	return len(v.Data)
}

// Empty reports whether the vector has no elements.
func (v StaticVector[T, C]) Empty() bool {
	return len(v.Data) == 0
}

// Clear removes all elements.
func (v *StaticVector[T, C]) Clear() {
	v.Data = nil
}

// Push appends one element after checking the static capacity.
func (v *StaticVector[T, C]) Push(value T) error {
	if len(v.Data) >= v.Capacity() {
		return ErrStaticCollectionFull
	}
	v.Data = append(v.Data, value)
	return nil
}

// Assign replaces all elements after checking the static capacity.
func (v *StaticVector[T, C]) Assign(values []T) error {
	if len(values) > v.Capacity() {
		return ErrStaticCollectionFull
	}
	v.Data = append(v.Data[:0], values...)
	return nil
}

// Slice returns the current elements.
func (v StaticVector[T, C]) Slice() []T {
	return v.Data
}

func staticVectorElementSize[T any]() (size, alignment int, err error) {
	var zero T
	switch any(zero).(type) {
	case uint32:
		return 4, 4, nil
	case uint64:
		return 8, 8, nil
	default:
		return 0, 0, fmt.Errorf("unsupported StaticVector element type %T", zero)
	}
}

func alignUp(value, alignment int) int {
	if alignment <= 1 {
		return value
	}
	return (value + alignment - 1) / alignment * alignment
}

func (v StaticVector[T, C]) WireSize() (int, error) {
	size, alignment, err := staticVectorElementSize[T]()
	if err != nil {
		return 0, err
	}
	dataOffset := alignUp(2, alignment)
	return dataOffset + v.Capacity()*size, nil
}

// AppendWire appends the packed vector, including all unused capacity slots.
func (v StaticVector[T, C]) AppendWire(dst []byte) ([]byte, error) {
	elementSize, alignment, err := staticVectorElementSize[T]()
	if err != nil {
		return nil, err
	}
	if len(v.Data) > v.Capacity() {
		return nil, fmt.Errorf("static vector length %d exceeds capacity %d", len(v.Data), v.Capacity())
	}
	wireSize, err := v.WireSize()
	if err != nil {
		return nil, err
	}
	start := len(dst)
	dst = append(dst, make([]byte, wireSize)...)
	binary.LittleEndian.PutUint16(dst[start:start+2], uint16(len(v.Data)))
	dataOffset := alignUp(2, alignment)
	_ = elementSize

	for i := 0; i < v.Capacity(); i++ {
		offset := start + dataOffset + i*elementSize
		if i >= len(v.Data) {
			continue
		}
		switch value := any(v.Data[i]).(type) {
		case uint32:
			binary.LittleEndian.PutUint32(dst[offset:offset+4], value)
		case uint64:
			binary.LittleEndian.PutUint64(dst[offset:offset+8], value)
		default:
			return nil, fmt.Errorf("unsupported StaticVector element type %T", value)
		}
	}
	return dst, nil
}

// ReadWire parses the packed vector representation.
func (v *StaticVector[T, C]) ReadWire(src []byte) (int, error) {
	elementSize, alignment, err := staticVectorElementSize[T]()
	if err != nil {
		return 0, err
	}
	wireSize, err := v.WireSize()
	if err != nil {
		return 0, err
	}
	if len(src) < wireSize {
		return 0, fmt.Errorf("static vector needs %d bytes, got %d", wireSize, len(src))
	}
	size := int(binary.LittleEndian.Uint16(src[:2]))
	if size > v.Capacity() {
		return 0, fmt.Errorf("static vector length %d exceeds capacity %d", size, v.Capacity())
	}
	dataOffset := alignUp(2, alignment)
	v.Data = make([]T, size)
	for i := 0; i < size; i++ {
		offset := dataOffset + i*elementSize
		switch any(new(T)).(type) {
		case *uint32:
			v.Data[i] = any(binary.LittleEndian.Uint32(src[offset : offset+4])).(T)
		case *uint64:
			v.Data[i] = any(binary.LittleEndian.Uint64(src[offset : offset+8])).(T)
		default:
			return 0, fmt.Errorf("unsupported StaticVector element type %T", new(T))
		}
	}
	return wireSize, nil
}
