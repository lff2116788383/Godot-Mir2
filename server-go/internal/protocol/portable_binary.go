package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const (
	portableLittleEndianFlag byte = 1
	portableBigEndianFlag    byte = 0
	portableDefaultMaxBytes       = 64 << 20
)

// PortableWriter implements the data layout of cereal::PortableBinaryOutputArchive.
// The first byte records little-endian output, matching the existing x64 build.
type PortableWriter struct {
	buf []byte
	err error
}

// NewPortableWriter creates a little-endian portable binary writer.
func NewPortableWriter() *PortableWriter {
	return &PortableWriter{buf: []byte{portableLittleEndianFlag}}
}

// Uint8 appends one byte.
func (w *PortableWriter) Uint8(value uint8) {
	if w.err != nil {
		return
	}
	w.buf = append(w.buf, value)
}

// Uint16 appends one little-endian uint16.
func (w *PortableWriter) Uint16(value uint16) {
	var raw [2]byte
	binary.LittleEndian.PutUint16(raw[:], value)
	w.buf = append(w.buf, raw[:]...)
}

// Uint32 appends one little-endian uint32.
func (w *PortableWriter) Uint32(value uint32) {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], value)
	w.buf = append(w.buf, raw[:]...)
}

// Uint64 appends one little-endian uint64.
func (w *PortableWriter) Uint64(value uint64) {
	var raw [8]byte
	binary.LittleEndian.PutUint64(raw[:], value)
	w.buf = append(w.buf, raw[:]...)
}

// Int8 appends one signed byte.
func (w *PortableWriter) Int8(value int8) { w.Uint8(uint8(value)) }

// Int16 appends one little-endian signed 16-bit value.
func (w *PortableWriter) Int16(value int16) { w.Uint16(uint16(value)) }

// Int32 appends one little-endian signed 32-bit value.
func (w *PortableWriter) Int32(value int32) { w.Uint32(uint32(value)) }

// Int64 appends one little-endian signed 64-bit value.
func (w *PortableWriter) Int64(value int64) { w.Uint64(uint64(value)) }

// Bool appends cereal's one-byte bool representation.
func (w *PortableWriter) Bool(value bool) {
	if value {
		w.Uint8(1)
	} else {
		w.Uint8(0)
	}
}

// Float32 appends an IEEE-754 float32.
func (w *PortableWriter) Float32(value float32) { w.Uint32(math.Float32bits(value)) }

// Float64 appends an IEEE-754 float64.
func (w *PortableWriter) Float64(value float64) { w.Uint64(math.Float64bits(value)) }

// SizeTag appends cereal::size_type, which is uint64_t in this build.
func (w *PortableWriter) SizeTag(size uint64) { w.Uint64(size) }

// Raw appends bytes without a length prefix, as binary_data does.
func (w *PortableWriter) Raw(data []byte) {
	if w.err != nil {
		return
	}
	w.buf = append(w.buf, data...)
}

// Bytes appends the std::vector<uint8_t> representation: size followed by raw bytes.
func (w *PortableWriter) Bytes(data []byte) {
	w.SizeTag(uint64(len(data)))
	w.Raw(data)
}

// String appends the std::string representation: byte length followed by UTF-8 bytes.
func (w *PortableWriter) String(value string) { w.Bytes([]byte(value)) }

// Err returns the first writer error.
func (w *PortableWriter) Err() error { return w.err }

// BytesCopy returns a copy of the encoded data.
func (w *PortableWriter) BytesCopy() ([]byte, error) {
	if w.err != nil {
		return nil, w.err
	}
	return append([]byte(nil), w.buf...), nil
}

// PortableReader reads the data layout of cereal::PortableBinaryInputArchive.
type PortableReader struct {
	data     []byte
	offset   int
	little   bool
	err      error
	maxBytes uint64
}

// NewPortableReader validates the archive endianness prefix.
func NewPortableReader(data []byte) (*PortableReader, error) {
	if len(data) == 0 {
		return nil, errors.New("portable binary archive is empty")
	}
	var little bool
	switch data[0] {
	case portableLittleEndianFlag:
		little = true
	case portableBigEndianFlag:
		little = false
	default:
		return nil, fmt.Errorf("invalid portable binary endian flag %d", data[0])
	}
	return &PortableReader{
		data:     data,
		offset:   1,
		little:   little,
		maxBytes: portableDefaultMaxBytes,
	}, nil
}

// SetMaxBytes limits length-prefixed allocations.
func (r *PortableReader) SetMaxBytes(max uint64) { r.maxBytes = max }

// Remaining returns the number of unread bytes.
func (r *PortableReader) Remaining() int {
	if r.offset >= len(r.data) {
		return 0
	}
	return len(r.data) - r.offset
}

// Err returns the first reader error.
func (r *PortableReader) Err() error { return r.err }

func (r *PortableReader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

func (r *PortableReader) ReadRaw(size int) []byte {
	if r.err != nil {
		return nil
	}
	if size < 0 || size > r.Remaining() {
		r.fail(fmt.Errorf("portable binary needs %d bytes, remaining %d", size, r.Remaining()))
		return nil
	}
	value := r.data[r.offset : r.offset+size]
	r.offset += size
	return value
}

// Uint8 reads one byte.
func (r *PortableReader) Uint8() uint8 {
	raw := r.ReadRaw(1)
	if raw == nil {
		return 0
	}
	return raw[0]
}

// Uint16 reads one endian-aware uint16.
func (r *PortableReader) Uint16() uint16 {
	raw := r.ReadRaw(2)
	if raw == nil {
		return 0
	}
	if r.little {
		return binary.LittleEndian.Uint16(raw)
	}
	return binary.BigEndian.Uint16(raw)
}

// Uint32 reads one endian-aware uint32.
func (r *PortableReader) Uint32() uint32 {
	raw := r.ReadRaw(4)
	if raw == nil {
		return 0
	}
	if r.little {
		return binary.LittleEndian.Uint32(raw)
	}
	return binary.BigEndian.Uint32(raw)
}

// Uint64 reads one endian-aware uint64.
func (r *PortableReader) Uint64() uint64 {
	raw := r.ReadRaw(8)
	if raw == nil {
		return 0
	}
	if r.little {
		return binary.LittleEndian.Uint64(raw)
	}
	return binary.BigEndian.Uint64(raw)
}

func (r *PortableReader) Int8() int8   { return int8(r.Uint8()) }
func (r *PortableReader) Int16() int16 { return int16(r.Uint16()) }
func (r *PortableReader) Int32() int32 { return int32(r.Uint32()) }
func (r *PortableReader) Int64() int64 { return int64(r.Uint64()) }

// Bool reads cereal's one-byte bool representation.
func (r *PortableReader) Bool() bool { return r.Uint8() != 0 }

// Float32 reads an IEEE-754 float32.
func (r *PortableReader) Float32() float32 { return math.Float32frombits(r.Uint32()) }

// Float64 reads an IEEE-754 float64.
func (r *PortableReader) Float64() float64 { return math.Float64frombits(r.Uint64()) }

// SizeTag reads cereal::size_type.
func (r *PortableReader) SizeTag() uint64 { return r.Uint64() }

// Bytes reads a length-prefixed byte vector.
func (r *PortableReader) Bytes() []byte {
	size := r.SizeTag()
	if r.err != nil {
		return nil
	}
	if r.maxBytes != 0 && size > r.maxBytes {
		r.fail(fmt.Errorf("portable binary length %d exceeds limit %d", size, r.maxBytes))
		return nil
	}
	if size > uint64(r.Remaining()) {
		r.fail(fmt.Errorf("portable binary length %d exceeds remaining bytes %d", size, r.Remaining()))
		return nil
	}
	return r.ReadRaw(int(size))
}

// String reads a std::string.
func (r *PortableReader) String() string { return string(r.Bytes()) }

// PortableMarshaler writes the same fields in the same order as a C++ serialize() function.
type PortableMarshaler interface {
	MarshalPortable(*PortableWriter) error
}

// PortableUnmarshaler reads the same fields in the same order as a C++ serialize() function.
type PortableUnmarshaler interface {
	UnmarshalPortable(*PortableReader) error
}

// MarshalPortable writes one cereal PortableBinary archive.
func MarshalPortable(value PortableMarshaler) ([]byte, error) {
	writer := NewPortableWriter()
	if err := value.MarshalPortable(writer); err != nil {
		return nil, err
	}
	return writer.BytesCopy()
}

// UnmarshalPortable reads one complete cereal PortableBinary archive.
func UnmarshalPortable(data []byte, value PortableUnmarshaler) error {
	reader, err := NewPortableReader(data)
	if err != nil {
		return err
	}
	if err := value.UnmarshalPortable(reader); err != nil {
		return err
	}
	if err := reader.Err(); err != nil {
		return err
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("portable binary archive has %d trailing bytes", reader.Remaining())
	}
	return nil
}
