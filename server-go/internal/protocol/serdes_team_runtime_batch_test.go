package protocol

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
)

func TestSerdesTeamRuntimeCppGolden(t *testing.T) {
	t.Parallel()

	fixtures := loadSerdesFixtures(t)
	runtimeConfig := SDRuntimeConfig{Config: map[string]string{"volume": "0.5"}}
	jobReq := SDTeamJobReq{MinLevel: 10, MaxLevel: 30, MinCapacity: 5, MagicIDList: []uint32{2, 5}}
	teamReq := SDTeamReq{
		MaxMember: 5, AutoAccept: true,
		JobReqList: [3]OptionalTeamJobReq{{Present: true, Value: jobReq}, {}, {}},
	}
	teamPlayer := SDTeamPlayer{UID: 42, Level: 20, Job: 1, Name: "hero"}

	values := []struct {
		name  string
		value any
	}{
		{"SDRuntimeConfig", runtimeConfig},
		{"SDPlayerConfig", SDPlayerConfig{MagicKeyList: SDMagicKeyList{KeyList: map[uint32]uint8{1: 'a'}}, RuntimeConfig: runtimeConfig}},
		{"SDBuffedAbility", SDBuffedAbility{
			DC:    [2]SDTaggedValMap{{ValMap: map[int32]int32{1: 10}}, {}},
			MC:    [2]SDTaggedValMap{{ValMap: map[int32]int32{2: 20}}, {}},
			DCHit: SDTaggedValMap{ValMap: map[int32]int32{3: 30}},
			Speed: SDTaggedValMap{ValMap: map[int32]int32{1: 5}},
		}},
		{"SDHealth", SDHealth{
			UID: 42, HP: 100, MP: 50, MaxHP: 200, MaxMP: 100, HPRecover: 3, MPRecover: 4,
			BuffedMaxHP:     SDTaggedValMap{ValMap: map[int32]int32{1: 20}},
			BuffedMaxMP:     SDTaggedValMap{ValMap: map[int32]int32{1: 10}},
			BuffedHPRecover: SDTaggedValMap{ValMap: map[int32]int32{1: 2}},
			BuffedMPRecover: SDTaggedValMap{ValMap: map[int32]int32{1: 1}},
		}},
		{"SDBuffIDList", SDBuffIDList{UID: 42, IDList: []uint32{1, 2, 3}}},
		{"SDRegisterQuest", SDRegisterQuest{Name: "quest"}},
		{"SDQueryQuestUID", SDQueryQuestUID{Name: "quest"}},
		{"SDTeamJobReq", jobReq},
		{"SDTeamReq", teamReq},
		{"SDTeamPlayer", teamPlayer},
		{"SDTeamCandidate", SDTeamCandidate{Player: teamPlayer}},
		{"SDRequestJoinTeam", SDRequestJoinTeam{Player: teamPlayer, Job: 1, Stat: 10, MagicIDList: []uint32{2, 5}}},
		{"SDTeamMemberList", SDTeamMemberList{
			TeamLeader: 42,
			MemberList: []SDTeamPlayer{teamPlayer, {UID: 43, Level: 21, Job: 2, Name: "bob"}},
			Req:        teamReq,
		}},
		{"SDTransferTeamLeader", SDTransferTeamLeader{MemberList: []uint64{42, 43}, Req: teamReq}},
		{"SDTeamInfo", SDTeamInfo{Leader: 42, Name: "party", Job: 1, Level: 20, MemberCount: 2, Req: teamReq}},
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

func TestSDRuntimeConfigHelpers(t *testing.T) {
	t.Parallel()

	var config SDRuntimeConfig
	if !config.SetConfig("a", "1") {
		t.Fatal("first set should change")
	}
	if config.SetConfig("a", "1") {
		t.Fatal("same value should not change")
	}
	if value, ok := config.GetConfig("a"); !ok || value != "1" {
		t.Fatalf("get = %q, %v", value, ok)
	}
	if !config.SetConfig("a", "") {
		t.Fatal("reset should change")
	}
	if _, ok := config.GetConfig("a"); ok {
		t.Fatal("reset should remove value")
	}
	if config.SetConfig("missing", "") {
		t.Fatal("resetting missing key should not change")
	}
}

func TestSDHealthHelpers(t *testing.T) {
	t.Parallel()

	health := SDHealth{
		HP: 150, MP: 80, MaxHP: 200, MaxMP: 100,
		BuffedMaxHP: SDTaggedValMap{ValMap: map[int32]int32{1: 50}},
	}
	if got := health.GetMaxHP(); got != 250 {
		t.Fatalf("max hp = %d", got)
	}
	if health.UpdateHealth(200, 100, 0, 0) != true {
		t.Fatal("health should change")
	}
	if health.HP != 250 || health.MP != 100 {
		t.Fatalf("health = %d/%d", health.HP, health.MP)
	}
	if health.UpdateHealth(0, 0, 0, 0) {
		t.Fatal("unchanged health should not report a change")
	}
	if health.Dead() {
		t.Fatal("health should be alive")
	}
}

func TestSDTeamHelpers(t *testing.T) {
	t.Parallel()

	req := SDTeamJobReq{MinLevel: 10, MaxLevel: 30, MinCapacity: 5, MagicIDList: []uint32{2, 5}}
	if req.Empty() || !req.Valid() {
		t.Fatalf("job req state: empty=%v valid=%v", req.Empty(), req.Valid())
	}
	if !(SDTeamJobReq{MinLevel: -1, MaxLevel: -1}).Empty() {
		t.Fatal("zero job req should be empty")
	}

	members := SDTeamMemberList{
		TeamLeader: 42,
		MemberList: []SDTeamPlayer{{UID: 42}, {UID: 43}},
	}
	if !members.HasMember(43) || members.HasMember(99) {
		t.Fatal("team member lookup mismatch")
	}
	if got := members.GetUIDList(); !reflect.DeepEqual(got, []uint64{42, 43}) {
		t.Fatalf("uids = %v", got)
	}
	members.Clear()
	if members.TeamLeader != 0 || len(members.MemberList) != 0 {
		t.Fatalf("cleared team = %+v", members)
	}
}
