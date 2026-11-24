package protocol

import (
	"errors"
	"fmt"
	"io"
)

var (
	errVarintOverflow = errors.New("varint overflows uint64")
	errVarintTooLong  = errors.New("varint is too long")
)

// appendUvarint mirrors msgf::encodeLength: little-endian base-128 groups.
func appendUvarint(dst []byte, value uint64) []byte {
	for value >= 0x80 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

// readUvarint reads one base-128 integer from r without buffering ahead.
func readUvarint(r io.Reader) (uint64, error) {
	var value uint64
	var one [1]byte

	for i := 0; i < 10; i++ {
		if _, err := io.ReadFull(r, one[:]); err != nil {
			return 0, err
		}
		b := one[0]
		if i == 9 && b > 1 {
			return 0, errVarintOverflow
		}
		value |= uint64(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			return value, nil
		}
	}
	return 0, errVarintTooLong
}

func readBoundedUvarint(r io.Reader, max uint64, what string) (uint64, error) {
	value, err := readUvarint(r)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", what, err)
	}
	if value > max {
		return 0, fmt.Errorf("%s %d exceeds limit %d", what, value, max)
	}
	return value, nil
}
