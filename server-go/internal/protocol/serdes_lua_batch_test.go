package protocol

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
)

func TestSerdesLuaCppGolden(t *testing.T) {
	t.Parallel()

	fixtures := loadSerdesFixtures(t)
	table := NewLuaTableVar(LuaTableEntry{
		Key:   WrapLuaVar(NewLuaStringVar("k")),
		Value: WrapLuaVar(NewLuaIntegerVar(7)),
	})
	nested := NewLuaArrayVar(
		WrapLuaVar(NewLuaIntegerVar(42)),
		WrapLuaVar(NewLuaStringVar("x")),
		WrapLuaVar(NewLuaBoolVar(true)),
		LuaVarWrapper{},
		WrapLuaVar(table),
	)

	values := []struct {
		name  string
		value any
	}{
		{"LuaVarNested", nested},
		{"SDRemoteCall", SDRemoteCall{Code: "return 1", Args: nested}},
		{"SDRemoteCallResult", SDRemoteCallResult{
			Error:   []string{"e1", "e2"},
			VarList: []LuaVar{NewLuaIntegerVar(5), NewLuaStringVar("ok")},
		}},
		{"SDSendNotify", SDSendNotify{
			Key: 0x1122334455667788, SeqID: 0x0102030405060708,
			VarList: nested, WaitConsume: true,
		}},
	}

	for _, testCase := range values {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture, ok := fixtures.Fixtures[testCase.name]
			if !ok {
				t.Fatalf("missing C++ fixture %s", testCase.name)
			}
			want, err := hex.DecodeString(fixture.Hex)
			if err != nil {
				t.Fatal(err)
			}
			if len(want) != fixture.Size {
				t.Fatalf("fixture size mismatch: %d vs %d", len(want), fixture.Size)
			}

			marshaler, ok := testCase.value.(PortableMarshaler)
			if !ok {
				t.Fatalf("%T is not a PortableMarshaler", testCase.value)
			}
			got, err := MarshalPortable(marshaler)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("archive mismatch\n got %x\nwant %x", got, want)
			}

			target := reflect.New(reflect.TypeOf(testCase.value)).Interface().(PortableUnmarshaler)
			if err := UnmarshalPortable(want, target); err != nil {
				t.Fatal(err)
			}
			decoded := reflect.ValueOf(target).Elem().Interface()
			if !reflect.DeepEqual(decoded, testCase.value) {
				t.Fatalf("decoded = %#v\nwant %#v", decoded, testCase.value)
			}
		})
	}
}

func TestLuaVarAllKindsRoundTrip(t *testing.T) {
	t.Parallel()

	values := []LuaVar{
		NewLuaNilVar(),
		NewLuaIntegerVar(-42),
		NewLuaNumberVar(1.25),
		NewLuaBoolVar(true),
		NewLuaStringVar("hello"),
		NewLuaArrayVar(WrapLuaVar(NewLuaIntegerVar(1)), LuaVarWrapper{}),
		NewLuaTableVar(
			LuaTableEntry{Key: WrapLuaVar(NewLuaStringVar("a")), Value: WrapLuaVar(NewLuaIntegerVar(1))},
			LuaTableEntry{Key: WrapLuaVar(NewLuaStringVar("b")), Value: WrapLuaVar(NewLuaBoolVar(false))},
		),
	}
	for _, value := range values {
		archive, err := MarshalPortable(value)
		if err != nil {
			t.Fatalf("kind %d marshal: %v", value.Kind, err)
		}
		var decoded LuaVar
		if err := UnmarshalPortable(archive, &decoded); err != nil {
			t.Fatalf("kind %d unmarshal: %v", value.Kind, err)
		}
		if !reflect.DeepEqual(decoded, value) {
			t.Fatalf("kind %d decoded = %#v", value.Kind, decoded)
		}
	}
}

func TestLuaVarRejectsInvalidKind(t *testing.T) {
	t.Parallel()

	value := LuaVar{Kind: LuaVarKind(99)}
	if _, err := MarshalPortable(value); err == nil {
		t.Fatal("expected invalid kind marshal error")
	}
}
