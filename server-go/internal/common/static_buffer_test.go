package common

import (
	"bytes"
	"testing"
)

func TestStaticBufferWireLayout(t *testing.T) {
	t.Parallel()

	var buffer StaticBuffer[Cap64]
	if err := buffer.Assign([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if buffer.WireSize() != 68 {
		t.Fatalf("wire size = %d", buffer.WireSize())
	}

	wire, err := buffer.AppendWire(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != buffer.WireSize() {
		t.Fatalf("encoded length = %d", len(wire))
	}
	if wire[0] != 3 || wire[1] != 0 || !bytes.Equal(wire[2:5], []byte("abc")) {
		t.Fatalf("wire prefix = %x", wire[:8])
	}
	if !bytes.Equal(wire[5:], make([]byte, len(wire)-5)) {
		t.Fatal("tail must be zero padded")
	}

	var decoded StaticBuffer[Cap64]
	n, err := decoded.ReadWire(wire)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(wire) || decoded.String() != "abc" {
		t.Fatalf("decoded=%q consumed=%d", decoded.String(), n)
	}
}

func TestStaticVectorCapacity(t *testing.T) {
	t.Parallel()

	var vector StaticVector[uint32, testCap2]
	if err := vector.Push(1); err != nil {
		t.Fatal(err)
	}
	if err := vector.Push(2); err != nil {
		t.Fatal(err)
	}
	if err := vector.Push(3); err != ErrStaticCollectionFull {
		t.Fatalf("push past capacity = %v", err)
	}
}

type testCap2 struct{}

func (testCap2) WireCapacity() int { return 2 }
