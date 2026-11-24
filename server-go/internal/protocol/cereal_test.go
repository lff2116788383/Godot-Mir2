package protocol

import (
	"bytes"
	"fmt"
	"testing"
)

type portableTestUint32 struct {
	Value uint32
}

func (v portableTestUint32) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.Value)
	return nil
}

func (v *portableTestUint32) UnmarshalPortable(reader *PortableReader) error {
	v.Value = reader.Uint32()
	return reader.Err()
}

type portableTestBytes struct {
	Data []byte
}

func (v portableTestBytes) MarshalPortable(writer *PortableWriter) error {
	writer.Bytes(v.Data)
	return nil
}

func (v *portableTestBytes) UnmarshalPortable(reader *PortableReader) error {
	v.Data = append([]byte(nil), reader.Bytes()...)
	return reader.Err()
}

func TestCerealSmallArchiveUsesNoneFlag(t *testing.T) {
	t.Parallel()

	encoded, err := CerealSerialize(portableTestUint32{Value: 0x11223344})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 6 {
		t.Fatalf("length = %d, want 6", len(encoded))
	}
	if encoded[len(encoded)-1] != CerealCompressionNone {
		t.Fatalf("flag = %d, want CF_NONE", encoded[len(encoded)-1])
	}
	if CerealIsCompressed(encoded) {
		t.Fatal("small archive should not be compressed")
	}

	var decoded portableTestUint32
	if err := CerealDeserialize(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Value != 0x11223344 {
		t.Fatalf("decoded value = %08x", decoded.Value)
	}
}

func TestCerealXORCompression(t *testing.T) {
	t.Parallel()

	value := portableTestBytes{Data: make([]byte, 10)}
	encoded, err := CerealSerialize(value)
	if err != nil {
		t.Fatal(err)
	}
	if encoded[len(encoded)-1] != CerealCompressionXOR {
		t.Fatalf("flag = %d, want CF_XOR; bytes=%x", encoded[len(encoded)-1], encoded)
	}

	var decoded portableTestBytes
	if err := CerealDeserialize(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Data, value.Data) {
		t.Fatal("XOR round-trip mismatch")
	}
}

func TestCerealZstdCompression(t *testing.T) {
	t.Parallel()

	value := portableTestBytes{Data: make([]byte, 4096)}
	encoded, err := CerealSerialize(value)
	if err != nil {
		t.Fatal(err)
	}
	if encoded[len(encoded)-1] != CerealCompressionZstd {
		t.Fatalf("flag = %d, want CF_ZSTD; length=%d", encoded[len(encoded)-1], len(encoded))
	}
	if !CerealIsCompressed(encoded) {
		t.Fatal("zstd archive should be compressed")
	}

	var decoded portableTestBytes
	if err := CerealDeserialize(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Data, value.Data) {
		t.Fatal("zstd round-trip mismatch")
	}
}

func TestCerealUncompressedExplicit(t *testing.T) {
	t.Parallel()

	value := portableTestBytes{Data: make([]byte, 4096)}
	encoded, err := CerealSerializeUncompressed(value)
	if err != nil {
		t.Fatal(err)
	}
	if encoded[len(encoded)-1] != CerealCompressionNone {
		t.Fatalf("flag = %d", encoded[len(encoded)-1])
	}
	var decoded portableTestBytes
	if err := CerealDeserialize(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Data, value.Data) {
		t.Fatal("uncompressed round-trip mismatch")
	}
}

func TestCerealRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	var decoded portableTestUint32
	if err := CerealDeserialize([]byte{1, 0, 0, 0, 0, 99}, &decoded); err == nil {
		t.Fatal("expected unknown flag error")
	} else if got := fmt.Sprint(err); got == "" {
		t.Fatal("error should not be empty")
	}
}
