package protocol

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type serdesFixtureFile struct {
	Fixtures map[string]struct {
		Size int    `json:"size"`
		Hex  string `json:"hex"`
	} `json:"fixtures"`
}

func loadSerdesFixtures(t *testing.T) serdesFixtureFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "serdes-golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures serdesFixtureFile
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures
}

func TestSerdesCppGolden(t *testing.T) {
	t.Parallel()

	fixtures := loadSerdesFixtures(t)
	groupVar := SDChatPeerGroupVar{
		Creator:    7,
		Createtime: 0x1122334455667788,
		MemberList: []SDChatGroupMember{{DBID: 1, Priority: 2}, {DBID: 5, Priority: 9}},
	}
	values := []struct {
		name  string
		value any
	}{
		{"SDSendPackage", SDSendPackage{Type: 0x12, Buf: []byte{0, 1, 2, 255}}},
		{"SDObserverNetPackage", SDObserverNetPackage{UID: 0x1122334455667788, X: -2, Y: 300, Package: SDSendPackage{Type: 0x12, Buf: []byte{0, 1, 2, 255}}}},
		{"SDInitQuest", SDInitQuest{QuestID: 100, FullScriptName: "quest/abc.lua"}},
		{"SDInitGuard", SDInitGuard{MonsterID: 0x11223344, MapUID: 0x5566778899aabbcc, X: -2, Y: 3, StrictLoc: true, Direction: 5}},
		{"SDInitPlayer", SDInitPlayer{
			DBID: 7, ChannID: 9, Name: "hero", MapUID: 0x1122334455667788,
			X: -10, Y: 20, StrictLoc: true, HP: 100, MP: 50, Exp: 123456789,
			Gold: 987654321, Gender: true, Job: 2, Hair: 3, HairColor: 4,
			PKPoint: 5, ViewRange: 15,
		}},
		{"SDInitNPChar", SDInitNPChar{LookID: 0x1234, NPCName: "merchant", FullScriptName: "npc/shop.lua", MapUID: 0x0102030405060708, X: 11, Y: 22, StrictLoc: false, GfxDir: 7}},
		{"SDInitMonster", SDInitMonster{MonsterID: 0xaabbccdd, MapUID: 0x0102030405060708, X: -3, Y: 4, StrictLoc: true, Direction: 6, MasterUID: 0x1122334455667788}},
		{"SDNPCXMLLayout", SDNPCXMLLayout{NPCUID: 42, EventPath: "npc/test", XMLLayout: "<xml/>"}},
		{"SDChatGroupMember", SDChatGroupMember{DBID: 1, Priority: 2}},
		{"SDChatPeerPlayerVar", SDChatPeerPlayerVar{Gender: true, Job: 2}},
		{"SDChatPeerGroupVar", groupVar},
		{"SDChatPeer", SDChatPeer{ID: 42, Name: "alice", AvatarPresent: true, Avatar: 123456789, DespKind: SDChatPeerDespGroup, GroupVar: groupVar}},
		{"SDChatPeerID", NewSDChatPeerID(CPR_GROUP, 42)},
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

func TestCerealfDtoRoundTrip(t *testing.T) {
	t.Parallel()

	value := SDInitPlayer{
		DBID: 7, ChannID: 9, Name: "hero", MapUID: 0x1122334455667788,
		X: -10, Y: 20, StrictLoc: true, HP: 100, MP: 50, Exp: 123456789,
		Gold: 987654321, Gender: true, Job: 2, Hair: 3, HairColor: 4,
		PKPoint: 5, ViewRange: 15,
	}
	archive, err := CerealSerialize(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded SDInitPlayer
	if err := CerealDeserialize(archive, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, value) {
		t.Fatalf("decoded = %#v", decoded)
	}
}
