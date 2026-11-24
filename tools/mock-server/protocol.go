package main

// 协议帧编解码，严格对齐 mir2x 的 wire format
//
// 帧结构（docs/protocol.md）：
//   [headCode(1B)] [respID:LEB128 ?] [bodySize:LEB128 ?] [mask ?] [body ?]
//
// headCode 最高位 0x80 = hasResp
// 消息体形态由消息表决定（type 0/1/2/3），不来自线上数据

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)


// Frame 解出的一帧
type Frame struct {
	HeadCode uint8
	RespID   uint64
	Body     []byte
}

// 读 LEB128
func readVInt(r io.ByteReader) (uint64, error) {
	var val uint64
	var shift uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		val |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return val, nil
		}
		shift += 7
		if shift > 63 {
			return 0, errors.New("vint overflow")
		}
	}
}

// 写 LEB128
func writeVInt(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v > 0 {
			out = append(out, b|0x80)
		} else {
			out = append(out, b)
			return out
		}
	}
}

// ReadFrame 从连接读一帧（客户端 -&gt; 服务端）
// 必须传入 *bufio.Reader，因为需要 io.ByteReader 逐字节读 LEB128
func ReadFrame(br *bufio.Reader) (*Frame, error) {
	r := io.Reader(br)

	head, err := br.ReadByte()
	if err != nil {
		return nil, err
	}

	hasResp := head&0x80 != 0
	code := head & 0x7f

	spec, ok := cmTable[code]
	if !ok {
		return nil, fmt.Errorf("未注册的客户端消息码 %d", code)
	}
	mtype, dataLen := spec[0], spec[1]

	f := &Frame{HeadCode: code}

	if hasResp {
		rid, err := readVInt(br)
		if err != nil {
			return nil, err
		}
		f.RespID = rid
	}

	switch mtype {
	case 0:
		// 空体
	case 2:
		buf := make([]byte, dataLen)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		f.Body = buf
	case 3:
		n, err := readVInt(br)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		f.Body = buf
	case 1:
		// 定长 + XOR：dataCount + mask + body
		dataCount, err := readVInt(br)
		if err != nil {
			return nil, err
		}
		maskLen := (dataLen + 7) / 8
		if dataLen >= 256 {
			maskLen = (dataLen + 63) / 64
		}
		mask := make([]byte, maskLen)
		if _, err := io.ReadFull(r, mask); err != nil {
			return nil, err
		}
		comp := make([]byte, dataCount)
		if _, err := io.ReadFull(r, comp); err != nil {
			return nil, err
		}
		body, err := xorDecode(mask, comp, dataLen)
		if err != nil {
			return nil, err
		}
		f.Body = body
	}
	return f, nil
}

// WriteFrame 写一帧（服务端 -&gt; 客户端）
func WriteFrame(w io.Writer, code uint8, respID uint64, body []byte) error {
	var out []byte

	if respID != 0 {
		out = append(out, code|0x80)
		out = append(out, writeVInt(respID)...)
	} else {
		out = append(out, code)
	}

	spec, ok := smTable[code]
	if !ok {
		return fmt.Errorf("未注册的服务端消息码 %d", code)
	}
	mtype, dataLen := spec[0], spec[1]

	switch mtype {
	case 0:
		// 空体
	case 2:
		out = append(out, body...)
	case 3:
		out = append(out, writeVInt(uint64(len(body)))...)
		out = append(out, body...)
	case 1:
		if len(body) != dataLen {
			return fmt.Errorf("SM %d 需要 %d 字节，实际 %d", code, dataLen, len(body))
		}
		maskLen := (dataLen + 7) / 8
		if dataLen >= 256 {
			maskLen = (dataLen + 63) / 64
		}
		mask := make([]byte, maskLen)
		var comp []byte
		for i := 0; i < dataLen; i++ {
			if body[i] != 0 {
				mask[i/8] |= 1 << (i % 8)
				comp = append(comp, body[i])
			}
		}
		out = append(out, writeVInt(uint64(len(comp)))...)
		out = append(out, mask...)
		out = append(out, comp...)
	}

	_, err := w.Write(out)
	return err
}

// xorDecode 还原定长数据
func xorDecode(mask, comp []byte, dataLen int) ([]byte, error) {
	out := make([]byte, dataLen)
	ci := 0
	for i := 0; i < dataLen; i++ {
		if mask[i/8]&(1<<(i%8)) != 0 {
			if ci >= len(comp) {
				return nil, errors.New("xor 数据不足")
			}
			out[i] = comp[ci]
			ci++
		}
	}
	return out, nil
}

// 小端写入辅助
func putU16(b []byte, off int, v uint16) { binary.LittleEndian.PutUint16(b[off:], v) }
func putU32(b []byte, off int, v uint32) { binary.LittleEndian.PutUint32(b[off:], v) }
func putU64(b []byte, off int, v uint64) { binary.LittleEndian.PutUint64(b[off:], v) }
func getU16(b []byte, off int) uint16    { return binary.LittleEndian.Uint16(b[off:]) }
func getU32(b []byte, off int) uint32    { return binary.LittleEndian.Uint32(b[off:]) }
func getU64(b []byte, off int) uint64    { return binary.LittleEndian.Uint64(b[off:]) }
