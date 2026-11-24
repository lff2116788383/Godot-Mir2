package protocol

import (
	"reflect"
	"testing"
)

func decodePortableTest(t *testing.T, value PortableMarshaler, target PortableUnmarshaler) {
	t.Helper()
	archive, err := MarshalPortable(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := UnmarshalPortable(archive, target); err != nil {
		t.Fatal(err)
	}
}

func TestSerdesOptionalFieldsAbsent(t *testing.T) {
	t.Parallel()

	peer := SDChatPeer{ID: 1, Name: "nobody", DespKind: SDChatPeerDespNone}
	peerTarget := SDChatPeer{
		AvatarPresent: true,
		Avatar:        99,
		DespKind:      SDChatPeerDespPlayer,
		PlayerVar:     SDChatPeerPlayerVar{Gender: true, Job: 7},
	}
	decodePortableTest(t, peer, &peerTarget)
	if !reflect.DeepEqual(peerTarget, peer) {
		t.Fatalf("peer = %+v", peerTarget)
	}

	message := SDChatMessage{From: NewSDChatPeerID(CPR_PLAYER, 1), To: NewSDChatPeerID(CPR_GROUP, 2), Message: "x"}
	messageTarget := SDChatMessage{HasSeq: true, Seq: SDChatMessageDBSeq{ID: 7}, HasRefer: true, Refer: 8}
	decodePortableTest(t, message, &messageTarget)
	if !reflect.DeepEqual(messageTarget, message) {
		t.Fatalf("message = %+v", messageTarget)
	}

	event := SDNPCEvent{MapUID: 3, Path: "p", Event: "e"}
	eventTarget := SDNPCEvent{HasValue: true, Value: "old"}
	decodePortableTest(t, event, &eventTarget)
	if !reflect.DeepEqual(eventTarget, event) {
		t.Fatalf("event = %+v", eventTarget)
	}

	quest := SDQuestDespUpdate{Name: "q", FSM: "f"}
	questTarget := SDQuestDespUpdate{HasDesp: true, Desp: "old"}
	decodePortableTest(t, quest, &questTarget)
	if !reflect.DeepEqual(questTarget, quest) {
		t.Fatalf("quest = %+v", questTarget)
	}
}

func TestSerdesMapRoundTrip(t *testing.T) {
	t.Parallel()

	keys := SDMagicKeyList{KeyList: map[uint32]uint8{1: 'a', 2: 'b', 9: '0'}}
	var keysTarget SDMagicKeyList
	decodePortableTest(t, keys, &keysTarget)
	if !reflect.DeepEqual(keysTarget, keys) {
		t.Fatalf("magic keys = %+v", keysTarget)
	}

	peers := SDSysSlavePeerList{List: map[uint64]SDSysSlavePeerAddress{1: {Host: "one", Port: 1}, 9: {Host: "nine", Port: 9}}}
	var peersTarget SDSysSlavePeerList
	decodePortableTest(t, peers, &peersTarget)
	if !reflect.DeepEqual(peersTarget, peers) {
		t.Fatalf("peer list = %+v", peersTarget)
	}
}

func TestSerdesEmptyCollections(t *testing.T) {
	t.Parallel()

	var zone SDSafeZone
	if !zone.Empty() {
		t.Fatal("zero safe zone should be empty")
	}

	var peers SDChatPeerList
	var decodedPeers SDChatPeerList
	decodePortableTest(t, peers, &decodedPeers)
	if len(decodedPeers) != 0 {
		t.Fatalf("peer count = %d", len(decodedPeers))
	}

	var uids SDUIDList
	if archive, err := MarshalPortable(uids); err != nil {
		t.Fatal(err)
	} else if len(archive) != 9 {
		t.Fatalf("empty uid list length = %d, want 9", len(archive))
	}
}

func TestSDTaggedValMapHelpers(t *testing.T) {
	t.Parallel()

	var values SDTaggedValMap
	if tag := values.Add(10); tag != 1 {
		t.Fatalf("first tag = %d", tag)
	}
	if tag := values.Add(20); tag != 2 {
		t.Fatalf("second tag = %d", tag)
	}
	if got := values.Sum(); got != 30 {
		t.Fatalf("sum = %d", got)
	}
}
