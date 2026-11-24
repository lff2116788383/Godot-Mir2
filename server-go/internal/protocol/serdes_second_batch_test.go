package protocol

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
)

func TestSerdesSecondBatchCppGolden(t *testing.T) {
	t.Parallel()

	fixtures := loadSerdesFixtures(t)
	groupVar := SDChatPeerGroupVar{
		Creator:    7,
		Createtime: 0x1122334455667788,
		MemberList: []SDChatGroupMember{{DBID: 1, Priority: 2}, {DBID: 5, Priority: 9}},
	}
	peer := SDChatPeer{
		ID: 42, Name: "alice", AvatarPresent: true, Avatar: 123456789,
		DespKind: SDChatPeerDespGroup, GroupVar: groupVar,
	}
	war1 := SDGuildWar{GuildID: 9, GuildName: "enemy", EndTime: 123456789}
	member1 := SDGuildMember{DBID: 1, Name: "alice", Rank: 1, Title: "leader", Online: true}
	wall := SDGridFireWall{X: 1, Y: 2, Count: 3}
	zone := SDSafeZone{
		RectList:   [][4]int32{{1, 2, 3, 4}, {5, 6, 7, 8}},
		CircleList: [][3]int32{{9, 10, 11}},
	}
	magic := SDLearnedMagic{MagicID: 1, Exp: 100}
	dbSeq := SDChatMessageDBSeq{ID: 0x1122334455667788, Timestamp: 123456789}
	rankingEntry := SDRankingEntry{DBID: 1, Level: 50, Gold: 123456789, Name: "hero"}

	values := []struct {
		name  string
		value any
	}{
		{"SDStartInvOp", SDStartInvOp{InvOp: 5, UID: 0x1122334455667788, QueryTag: "q", CommitTag: "c", TypeList: []string{"item", "gold"}}},
		{"SDStartInput", SDStartInput{UID: 42, Title: "rename", CommitTag: "ok", Security: true}},
		{"SDNPCSell", SDNPCSell{NPCUID: 0x1122334455667788, ItemList: []uint32{1, 2, 0xffffffff}}},
		{"SDCostItem", SDCostItem{ItemID: 55, Count: 3}},
		{"SDPlayerName", SDPlayerName{UID: 42, Name: "hero", NameColor: 0x11223344}},
		{"SDNPCName", SDNPCName{UID: 43, Name: "merchant"}},
		{"SDPlayerGuild", SDPlayerGuild{DBID: 7, GuildID: 9, GuildName: "guild", Title: "leader"}},
		{"SDGuildWar", war1},
		{"SDGuildWarList", SDGuildWarList{GuildID: 7, WarList: []SDGuildWar{war1, {GuildID: 10, GuildName: "other", EndTime: 987654321}}}},
		{"SDGuildMember", member1},
		{"SDGuildInfo", SDGuildInfo{GuildID: 7, Name: "guild", Notice: "hello", MemberList: []SDGuildMember{member1, {DBID: 2, Name: "bob", Rank: 2, Title: "member", Online: false}}}},
		{"SDGroundItemIDList", SDGroundItemIDList{MapUID: 0x1122334455667788, GridItemIDList: []SDGridItemIDList{{X: 1, Y: 2, ItemIDList: []uint32{10, 20}}, {X: -3, Y: 4, ItemIDList: []uint32{}}}}},
		{"SDGridFireWall", wall},
		{"SDGroundFireWallList", SDGroundFireWallList{MapUID: 0x1122334455667788, FireWallList: []SDGridFireWall{wall, {X: 4, Y: 5, Count: 6}}}},
		{"SDSafeZone", zone},
		{"SDSafeZoneList", SDSafeZoneList{MapUID: 0x1122334455667788, Zone: zone}},
		{"SDLearnedMagic", magic},
		{"SDLearnedMagicList", SDLearnedMagicList{MagicList: []SDLearnedMagic{magic, {MagicID: 2, Exp: 200}}}},
		{"SDMagicKeyList", SDMagicKeyList{KeyList: map[uint32]uint8{1: 'a'}}},
		{"SDAddFriendNotif", SDAddFriendNotif{Notif: 3}},
		{"SDBlockPlayerNotif", SDBlockPlayerNotif{Notif: 4}},
		{"SDChatMessageDBSeq", dbSeq},
		{"SDChatMessage", SDChatMessage{
			HasSeq: true, Seq: dbSeq, HasRefer: true, Refer: 0x0102030405060708,
			From: NewSDChatPeerID(CPR_PLAYER, 42), To: NewSDChatPeerID(CPR_GROUP, 43), Message: "hi",
		}},
		{"SDNPCEvent", SDNPCEvent{X: 1, Y: 2, MapUID: 0x1122334455667788, Path: "npc/p", Event: "ev", HasValue: true, Value: "val"}},
		{"SDTaggedValMap", SDTaggedValMap{ValMap: map[int32]int32{1: 10, 2: 20}}},
		{"SDPeerConfig", SDPeerConfig{DropRate: 1.5, GoldRate: 0.75}},
		{"SDSysSlavePeerPort", SDSysSlavePeerPort{Port: 7000}},
		{"SDSysSlavePeerList", SDSysSlavePeerList{List: map[uint64]SDSysSlavePeerAddress{1: {Host: "host1", Port: 7001}, 2: {Host: "host2", Port: 7002}}}},
		{"SDSysPeerIndex", SDSysPeerIndex{Index: 3, MasterConfig: "cfg"}},
		{"SDQuestDespUpdate", SDQuestDespUpdate{Name: "quest", FSM: "main", HasDesp: true, Desp: "desp"}},
		{"SDRankingEntry", rankingEntry},
		{"SDRankingList", SDRankingList{Type: 1, Entries: []SDRankingEntry{rankingEntry, {DBID: 2, Level: 40, Gold: 987654321, Name: "bob"}}}},
		{"SDChatPeerList", SDChatPeerList{peer, {ID: 43, Name: "bob", DespKind: SDChatPeerDespPlayer, PlayerVar: SDChatPeerPlayerVar{Gender: false, Job: 1}}}},
		{"SDUIDList", SDUIDList{1, 0x1122334455667788}},
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
