package protocol

import (
	"bytes"
	"math"
	"testing"
)

func TestUvarintRoundTrip(t *testing.T) {
	t.Parallel()

	values := []uint64{0, 1, 127, 128, 255, 300, 16384, math.MaxUint64}
	for _, value := range values {
		encoded := appendUvarint(nil, value)
		decoded, err := readUvarint(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("readUvarint(%d): %v", value, err)
		}
		if decoded != value {
			t.Fatalf("readUvarint(%d) = %d", value, decoded)
		}
	}
}

func TestUvarintKnownEncoding(t *testing.T) {
	t.Parallel()

	if got := appendUvarint(nil, 300); !bytes.Equal(got, []byte{0xac, 0x02}) {
		t.Fatalf("encode 300 = %x", got)
	}
}
