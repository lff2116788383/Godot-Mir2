package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

func TestDirectTradeCommit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, firstDBID := openTestCharacter(t, OpenOptions{Path: filepath.Join(t.TempDir(), "trade.db3")})
	accounts := NewAccountRepository(store)
	characters := NewCharacterRepository(store)
	if err := accounts.Create(ctx, "second", "pw"); err != nil {
		t.Fatal(err)
	}
	second, err := characters.Create(ctx, CreateCharacterParams{Account: "second", Name: "second", Gender: 0, Job: 2, Map: 1})
	if err != nil {
		t.Fatal(err)
	}
	inventory := NewItemRepository(store)
	firstItem := protocol.SDItem{ItemID: 100, SeqID: 1, Count: 1}
	secondItem := protocol.SDItem{ItemID: 200, SeqID: 1, Count: 2, ExtAttrList: map[int32]string{1: "x"}}
	if err := inventory.PutInventoryItem(ctx, firstDBID, firstItem); err != nil {
		t.Fatal(err)
	}
	if err := inventory.PutInventoryItem(ctx, second.DBID, secondItem); err != nil {
		t.Fatal(err)
	}
	trades := NewTradeRepository(store)
	offer0 := protocol.SDDirectTradeOffer{UID: 1, Gold: 100, Locked: true, Confirmed: true, ItemList: []protocol.SDItem{firstItem}}
	offer1 := protocol.SDDirectTradeOffer{UID: 2, Gold: 200, Locked: true, Confirmed: true, ItemList: []protocol.SDItem{secondItem}}
	result, err := trades.Commit(ctx, firstDBID, second.DBID, offer0, offer1)
	if err != nil {
		t.Fatal(err)
	}
	if result[0].Gold != 10100 || result[1].Gold != 9900 {
		t.Fatalf("gold = %d/%d", result[0].Gold, result[1].Gold)
	}
	if len(result[0].Inventory.ItemList) != 1 || result[0].Inventory.ItemList[0].ItemID != 200 {
		t.Fatalf("first inventory = %+v", result[0].Inventory)
	}
	if len(result[1].Inventory.ItemList) != 1 || result[1].Inventory.ItemList[0].ItemID != 100 {
		t.Fatalf("second inventory = %+v", result[1].Inventory)
	}

	badOffer := offer0
	badOffer.Confirmed = false
	if _, err := trades.Commit(ctx, firstDBID, second.DBID, badOffer, offer1); err == nil {
		t.Fatal("expected invalid trade error")
	}
}
