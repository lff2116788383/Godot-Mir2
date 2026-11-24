package common

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestActionNodeWireLayout(t *testing.T) {
	t.Parallel()

	node := ActionNode{
		Type:      ActionType(7),
		Speed:     100,
		Direction: 3,
		X:         -2,
		Y:         3,
		AimX:      -4,
		AimY:      5,
		AimUID:    0x1122334455667788,
		ExtParam:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	got := node.AppendWire(nil)
	want, err := hex.DecodeString("870c0300feff0300fcff050088776655443322110102030405060708")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ActionNode wire = %x, want %x", got, want)
	}

	decoded, err := ParseActionNode(got)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != node {
		t.Fatalf("decoded = %+v, want %+v", decoded, node)
	}
}

func TestActionNodeCppBitfieldVectors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		node ActionNode
		want string
	}{
		{"type", ActionNode{Type: 31}, "1f000000"},
		{"speed", ActionNode{Speed: 511}, "e03f0000"},
		{"direction", ActionNode{Direction: 31}, "00001f00"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want, err := hex.DecodeString(tc.want)
			if err != nil {
				t.Fatal(err)
			}
			if got := tc.node.AppendWire(nil)[:4]; !bytes.Equal(got, want) {
				t.Fatalf("flags = %x, want %x", got, want)
			}
		})
	}
}

func TestActionNodeExtensions(t *testing.T) {
	t.Parallel()

	var node ActionNode
	node.SetSpellExt(0x11223344, 0x55667788)
	magicID, modifierID := node.SpellExt()
	if magicID != 0x11223344 || modifierID != 0x55667788 {
		t.Fatalf("spell ext = %08x/%08x", magicID, modifierID)
	}

	node.SetMoveExt(true)
	if !node.MoveExt() {
		t.Fatal("move ext should be true")
	}
	node.SetMoveExt(false)
	if node.MoveExt() {
		t.Fatal("move ext should be false")
	}
}
