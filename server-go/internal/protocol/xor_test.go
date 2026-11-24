package protocol

import (
	"bytes"
	"testing"
)

func TestXORClassicKnownVector(t *testing.T) {
	t.Parallel()

	src := []byte{0, 1, 0, 2, 3, 0, 0, 4}
	encoded := xorEncode(src)
	want := []byte{0x9a, 1, 2, 3, 4}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("xorEncode = %x, want %x", encoded, want)
	}

	dst := make([]byte, len(src))
	n, err := xorDecodeClassic(dst, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 || !bytes.Equal(dst, src) {
		t.Fatalf("xorDecodeClassic = %x, count=%d", dst, n)
	}
}

func TestXORWideKnownVector(t *testing.T) {
	t.Parallel()

	src := make([]byte, 16)
	copy(src[8:], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	encoded := xorEncode64(src)
	want := []byte{0x02, 1, 2, 3, 4, 5, 6, 7, 8}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("xorEncode64 = %x, want %x", encoded, want)
	}

	dst := make([]byte, len(src))
	n, err := xorDecodeWide(dst, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if n != 8 || !bytes.Equal(dst, src) {
		t.Fatalf("xorDecodeWide = %x, count=%d", dst, n)
	}
}

func TestXORRoundTripAllSizes(t *testing.T) {
	t.Parallel()

	for size := 1; size <= 400; size++ {
		src := make([]byte, size)
		for i := range src {
			if (i+size)%7 == 0 {
				src[i] = byte(i + 1)
			}
		}

		encoded := xorEncode(src)
		dst := make([]byte, size)
		n, err := xorDecodeClassic(dst, encoded)
		if err != nil {
			t.Fatalf("size %d classic: %v", size, err)
		}
		if n != len(encoded)-(size+7)/8 {
			t.Fatalf("size %d classic decoded count %d", size, n)
		}
		if !bytes.Equal(dst, src) {
			t.Fatalf("size %d classic mismatch", size)
		}

		encoded64 := xorEncode64(src)
		dst64 := make([]byte, size)
		n64, err := xorDecodeWide(dst64, encoded64)
		if err != nil {
			t.Fatalf("size %d wide: %v", size, err)
		}
		if n64 != len(encoded64)-(size+63)/64 {
			t.Fatalf("size %d wide decoded count %d", size, n64)
		}
		if !bytes.Equal(dst64, src) {
			t.Fatalf("size %d wide mismatch", size)
		}
	}
}
