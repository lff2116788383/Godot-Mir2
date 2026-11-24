package protocol

import (
	"errors"
	"fmt"

	"github.com/klauspost/compress/zstd"
)

const (
	// CerealCompressionNone stores the PortableBinary archive unchanged.
	CerealCompressionNone byte = 0

	// CerealCompressionZstd stores a Zstandard frame.
	CerealCompressionZstd byte = 1

	// CerealCompressionXOR stores a sparse XOR mask and non-zero bytes.
	CerealCompressionXOR byte = 2
)

// CerealSerialize serializes one value using PortableBinary and the same
// automatic compression policy as cerealf::serialize.
func CerealSerialize(value PortableMarshaler) ([]byte, error) {
	raw, err := MarshalPortable(value)
	if err != nil {
		return nil, err
	}
	return compressCerealArchive(raw)
}

// CerealSerializeUncompressed serializes one value and appends CF_NONE.
func CerealSerializeUncompressed(value PortableMarshaler) ([]byte, error) {
	raw, err := MarshalPortable(value)
	if err != nil {
		return nil, err
	}
	return append(raw, CerealCompressionNone), nil
}

// CerealDeserialize decodes one cerealf archive into value.
func CerealDeserialize(data []byte, value PortableUnmarshaler) error {
	if len(data) == 0 {
		return errors.New("cerealf archive is empty")
	}
	flag := data[len(data)-1]
	body := data[:len(data)-1]

	var raw []byte
	switch flag {
	case CerealCompressionNone:
		raw = body

	case CerealCompressionXOR:
		if len(body) < 1 {
			return errors.New("cerealf XOR archive is empty")
		}
		rawSize := int(body[len(body)-1])
		encoded := body[:len(body)-1]
		maskSize := (rawSize + 7) / 8
		if len(encoded) < maskSize {
			return fmt.Errorf("cerealf XOR body %d is shorter than mask %d", len(encoded), maskSize)
		}
		raw = make([]byte, rawSize)
		if _, err := xorDecodeClassic(raw, encoded); err != nil {
			return fmt.Errorf("cerealf XOR decode: %w", err)
		}

	case CerealCompressionZstd:
		decoder, err := zstd.NewReader(nil)
		if err != nil {
			return fmt.Errorf("create zstd decoder: %w", err)
		}
		defer decoder.Close()
		raw, err = decoder.DecodeAll(body, nil)
		if err != nil {
			return fmt.Errorf("cerealf zstd decode: %w", err)
		}

	default:
		return fmt.Errorf("unknown cerealf compression flag %d", flag)
	}

	if err := UnmarshalPortable(raw, value); err != nil {
		return fmt.Errorf("cerealf PortableBinary decode: %w", err)
	}
	return nil
}

// CerealIsCompressed reports whether the archive uses XOR or Zstandard.
func CerealIsCompressed(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return data[len(data)-1]&(CerealCompressionZstd|CerealCompressionXOR) != 0
}

func compressCerealArchive(raw []byte) ([]byte, error) {
	if len(raw) < 8 {
		return append(raw, CerealCompressionNone), nil
	}

	if len(raw) < 64 {
		encoded := xorEncode(raw)
		maskSize := (len(raw) + 7) / 8
		totalSize := maskSize + (len(encoded) - maskSize)
		if totalSize+2 < len(raw) {
			// raw is shorter than 64, therefore the length fits in one byte.
			result := append(make([]byte, 0, totalSize+2), encoded...)
			result = append(result, byte(len(raw)), CerealCompressionXOR)
			return result, nil
		}
	}

	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, fmt.Errorf("create zstd encoder: %w", err)
	}
	compressed := encoder.EncodeAll(raw, nil)
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close zstd encoder: %w", err)
	}
	if len(compressed)+1 < len(raw) {
		return append(compressed, CerealCompressionZstd), nil
	}

	return append(raw, CerealCompressionNone), nil
}
