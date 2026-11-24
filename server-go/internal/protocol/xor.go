package protocol

import "fmt"

// xorEncode implements zcompf::xorEncode. The returned buffer is mask || data.
func xorEncode(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	maskLen := (len(src) + 7) / 8
	out := make([]byte, maskLen, maskLen+len(src))
	for i, value := range src {
		if value != 0 {
			out[i/8] |= 1 << (i % 8)
			out = append(out, value)
		}
	}
	return out
}

// xorEncode64 implements zcompf::xorEncode64. The returned buffer is mask || data.
func xorEncode64(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	maskLen := (len(src) + 63) / 64
	out := make([]byte, maskLen, maskLen+len(src))
	for offset := 0; offset < len(src); offset += 8 {
		end := min(offset+8, len(src))
		block := src[offset:end]
		nonZero := false
		for _, value := range block {
			if value != 0 {
				nonZero = true
				break
			}
		}
		if nonZero {
			out[offset/64] |= 1 << ((offset % 64) / 8)
			out = append(out, block...)
		}
	}
	return out
}

func xorDecodeClassic(dst, encoded []byte) (int, error) {
	maskLen := (len(dst) + 7) / 8
	if len(encoded) < maskLen {
		return 0, fmt.Errorf("XOR payload %d bytes is shorter than mask %d", len(encoded), maskLen)
	}
	mask := encoded[:maskLen]
	compressed := encoded[maskLen:]
	read := 0
	for i := range dst {
		if mask[i/8]&(1<<(i%8)) != 0 {
			if read >= len(compressed) {
				return 0, fmt.Errorf("XOR payload ended at byte %d", i)
			}
			dst[i] = compressed[read]
			read++
		} else {
			dst[i] = 0
		}
	}
	if read != len(compressed) {
		return 0, fmt.Errorf("XOR payload has %d trailing bytes", len(compressed)-read)
	}
	return read, nil
}

func xorDecodeWide(dst, encoded []byte) (int, error) {
	maskLen := (len(dst) + 63) / 64
	if len(encoded) < maskLen {
		return 0, fmt.Errorf("XOR64 payload %d bytes is shorter than mask %d", len(encoded), maskLen)
	}
	mask := encoded[:maskLen]
	compressed := encoded[maskLen:]
	read := 0
	for offset := 0; offset < len(dst); offset += 8 {
		end := min(offset+8, len(dst))
		blockLen := end - offset
		if mask[offset/64]&(1<<((offset%64)/8)) != 0 {
			if read+blockLen > len(compressed) {
				return 0, fmt.Errorf("XOR64 payload ended at byte %d", offset)
			}
			copy(dst[offset:end], compressed[read:read+blockLen])
			read += blockLen
		} else {
			clear(dst[offset:end])
		}
	}
	if read != len(compressed) {
		return 0, fmt.Errorf("XOR64 payload has %d trailing bytes", len(compressed)-read)
	}
	return read, nil
}

func xorDecode(dst, encoded []byte, wide bool) (int, error) {
	if wide {
		return xorDecodeWide(dst, encoded)
	}
	return xorDecodeClassic(dst, encoded)
}
