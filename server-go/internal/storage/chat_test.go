package storage

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

func openCharacters(t *testing.T, names ...string) (*Store, []uint32) {
	t.Helper()
	ctx := context.Background()
	store, err := Open(ctx, OpenOptions{Path: filepath.Join(t.TempDir(), "chat.db3"), CreateIfMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	accounts := NewAccountRepository(store)
	characters := NewCharacterRepository(store)
	result := make([]uint32, 0, len(names))
	for i, name := range names {
		account := name + "_account"
		if err := accounts.Create(ctx, account, "pw"); err != nil {
			t.Fatal(err)
		}
		character, err := characters.Create(ctx, CreateCharacterParams{
			Account: account, Name: name, Gender: uint8(i % 2), Job: uint8(1 << (i % 3)), Map: 1, MapX: int32(i), MapY: int32(i + 1),
		})
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, character.DBID)
	}
	return store, result
}

func TestChatMessagesGroupsFriendsAndBlocks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, dbids := openCharacters(t, "hero", "bob")
	repo := NewChatRepository(store)
	heroCPID := protocol.NewSDChatPeerID(protocol.CPR_PLAYER, dbids[0])
	bobCPID := protocol.NewSDChatPeerID(protocol.CPR_PLAYER, dbids[1])

	seq, err := repo.SaveMessage(ctx, heroCPID, bobCPID, "hello", nil)
	if err != nil || seq.ID == 0 {
		t.Fatalf("save message = %+v, %v", seq, err)
	}
	message, ok, err := repo.QueryMessage(ctx, seq.ID)
	if err != nil || !ok {
		t.Fatalf("query message = %v, %v", ok, err)
	}
	if message.From != heroCPID || message.To != bobCPID || message.Message != "hello" || !message.HasSeq {
		t.Fatalf("message = %+v", message)
	}

	peer, ok, err := repo.LoadPeer(ctx, bobCPID)
	if err != nil || !ok || peer.Name != "bob" || peer.DespKind != protocol.SDChatPeerDespPlayer {
		t.Fatalf("player peer = %+v, %v, %v", peer, ok, err)
	}
	system, ok, err := repo.LoadPeer(ctx, protocol.NewSDChatPeerID(protocol.CPR_SPECIAL, chatDBIDSystem))
	if err != nil || !ok || system.Name != chatNameSystem {
		t.Fatalf("system peer = %+v, %v, %v", system, ok, err)
	}

	group, err := repo.CreateGroup(ctx, dbids[0], "party", []uint32{dbids[1]})
	if err != nil {
		t.Fatal(err)
	}
	members, err := repo.LoadGroupMemberList(ctx, group.ID)
	if err != nil || !reflect.DeepEqual(members, []uint32{dbids[0], dbids[1]}) {
		t.Fatalf("group members = %v, %v", members, err)
	}
	groupCPID := protocol.NewSDChatPeerID(protocol.CPR_GROUP, group.ID)
	if _, err := repo.SaveMessage(ctx, heroCPID, groupCPID, "group hi", nil); err != nil {
		t.Fatal(err)
	}

	sent, err := repo.RetrieveLatest(ctx, dbids[0], []uint64{bobCPID.Data}, 10, true, true)
	if err != nil || len(sent) != 1 || sent[0].Message != "hello" {
		t.Fatalf("latest direct = %#v, %v", sent, err)
	}
	groupMessages, err := repo.RetrieveLatest(ctx, dbids[0], []uint64{groupCPID.Data}, 10, false, true)
	if err != nil || len(groupMessages) != 1 || groupMessages[0].Message != "group hi" {
		t.Fatalf("latest group = %#v, %v", groupMessages, err)
	}

	peers, err := repo.QueryPeerList(ctx, "hero", true, false)
	if err != nil || len(peers) != 1 || peers[0].Name != "hero" {
		t.Fatalf("player search = %#v, %v", peers, err)
	}
	groups, err := repo.QueryPeerList(ctx, "party", false, true)
	if err != nil || len(groups) != 1 || groups[0].ID != group.ID {
		t.Fatalf("group search = %#v, %v", groups, err)
	}

	if err := repo.AddFriend(ctx, dbids[0], dbids[1]); err != nil {
		t.Fatal(err)
	}
	friends, err := repo.ListFriends(ctx, dbids[0])
	if err != nil || len(friends) != 2 {
		t.Fatalf("friends = %#v, %v", friends, err)
	}
	if removed, err := repo.RemoveFriend(ctx, dbids[0], dbids[1]); err != nil || !removed {
		t.Fatalf("remove friend = %v, %v", removed, err)
	}

	if err := repo.BlockPlayer(ctx, dbids[0], dbids[1]); err != nil {
		t.Fatal(err)
	}
	blocked, err := repo.ListBlocked(ctx, dbids[0])
	if err != nil || !reflect.DeepEqual(blocked, []uint32{dbids[1]}) {
		t.Fatalf("blocked = %v, %v", blocked, err)
	}
	if unblocked, err := repo.UnblockPlayer(ctx, dbids[0], dbids[1]); err != nil || !unblocked {
		t.Fatalf("unblock = %v, %v", unblocked, err)
	}
}

func TestDeliveryClaim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, dbids := openCharacters(t, "hero")
	repo := NewDeliveryRepository(store)
	item := protocol.SDItem{ItemID: 100, Count: 5, ExtAttrList: map[int32]string{1: "abc"}}
	delivery, err := repo.Create(ctx, dbids[0], []protocol.SDItem{item}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(delivery.Record) != 32 {
		t.Fatalf("record length = %d", len(delivery.Record))
	}
	items, err := repo.Claim(ctx, dbids[0], delivery.Record)
	if err != nil || len(items) != 1 || !reflect.DeepEqual(items[0], item) {
		t.Fatalf("claim = %#v, %v", items, err)
	}
	if _, err := repo.Claim(ctx, dbids[0], delivery.Record); !errors.Is(err, ErrDeliveryClaimed) {
		t.Fatalf("second claim error = %v", err)
	}
}
