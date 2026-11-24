package protocol

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestPortableBinaryCppGoldenBasic(t *testing.T) {
	t.Parallel()

	writer := NewPortableWriter()
	writer.Uint8(0x12)
	writer.Uint16(0x3456)
	writer.Uint32(0x789abcde)
	writer.Uint64(0x1122334455667788)
	writer.Bool(true)
	writer.Int32(-2)
	writer.String("abc")
	writer.Bytes([]byte{0, 1, 2, 255})
	writer.SizeTag(2)
	writer.Uint32(0x11223344)
	writer.Uint32(0x55667788)
	writer.Uint16(0x0102)
	writer.Uint16(0x0304)
	got, err := writer.BytesCopy()
	if err != nil {
		t.Fatal(err)
	}
	want, err := hex.DecodeString("01125634debc9a78887766554433221101feffffff03000000000000006162630400000000000000000102ff0200000000000000443322118877665502010403")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("portable archive\n got %x\nwant %x", got, want)
	}

	reader, err := NewPortableReader(want)
	if err != nil {
		t.Fatal(err)
	}
	if reader.Uint8() != 0x12 || reader.Uint16() != 0x3456 || reader.Uint32() != 0x789abcde || reader.Uint64() != 0x1122334455667788 {
		t.Fatal("integer round-trip mismatch")
	}
	if !reader.Bool() || reader.Int32() != -2 || reader.String() != "abc" {
		t.Fatal("bool/signed/string round-trip mismatch")
	}
	if !bytes.Equal(reader.Bytes(), []byte{0, 1, 2, 255}) {
		t.Fatal("byte vector mismatch")
	}
	if size := reader.SizeTag(); size != 2 {
		t.Fatalf("vector size = %d", size)
	}
	if reader.Uint32() != 0x11223344 || reader.Uint32() != 0x55667788 {
		t.Fatal("uint32 vector mismatch")
	}
	if reader.Uint16() != 0x0102 || reader.Uint16() != 0x0304 {
		t.Fatal("array mismatch")
	}
	if reader.Err() != nil || reader.Remaining() != 0 {
		t.Fatalf("reader err=%v remaining=%d", reader.Err(), reader.Remaining())
	}
}

func TestPortableBinaryCppGoldenOptionalVariant(t *testing.T) {
	t.Parallel()

	writer := NewPortableWriter()
	writer.Bool(true) // std::optional<uint64_t> == nullopt
	writer.Bool(false)
	writer.Uint64(5)
	writer.Int32(0) // variant<monostate, uint32_t, string>
	writer.Int32(1)
	writer.Uint32(0x11223344)
	writer.Int32(2)
	writer.String("xy")
	got, err := writer.BytesCopy()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString("01010005000000000000000000000001000000443322110200000002000000000000007879")
	if !bytes.Equal(got, want) {
		t.Fatalf("portable archive\n got %x\nwant %x", got, want)
	}
}

func TestPortableBinaryBounds(t *testing.T) {
	t.Parallel()

	reader, err := NewPortableReader([]byte{1, 4, 0, 0, 0, 0, 0, 0, 0, 'a'})
	if err != nil {
		t.Fatal(err)
	}
	reader.SetMaxBytes(3)
	if data := reader.Bytes(); data != nil || reader.Err() == nil {
		t.Fatalf("expected max-bytes error, data=%x err=%v", data, reader.Err())
	}

	if _, err := NewPortableReader(nil); err == nil {
		t.Fatal("expected empty archive error")
	}
	if _, err := NewPortableReader([]byte{2}); err == nil {
		t.Fatal("expected endian flag error")
	}
}

func TestPortableBinaryBigEndianRead(t *testing.T) {
	t.Parallel()

	reader, err := NewPortableReader([]byte{0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88})
	if err != nil {
		t.Fatal(err)
	}
	if got := reader.Uint16(); got != 0x1234 {
		t.Fatalf("uint16 = %04x", got)
	}
	if got := reader.Uint32(); got != 0x56789abc {
		t.Fatalf("uint32 = %08x", got)
	}
	if got := reader.Uint64(); got != 0xdef0112233445566 {
		t.Fatalf("uint64 = %016x", got)
	}
}
