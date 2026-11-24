package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

func TestAuctionLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, sellerDBID := openTestCharacter(t, OpenOptions{
		Path: filepath.Join(t.TempDir(), "auction.db3"),
		ItemCategoryMatch: func(itemID, category int64) bool {
			return itemID == 100 && category == AuctionCategoryAll
		},
	})
	accounts := NewAccountRepository(store)
	characters := NewCharacterRepository(store)
	if err := accounts.Create(ctx, "buyer", "pw"); err != nil {
		t.Fatal(err)
	}
	buyer, err := characters.Create(ctx, CreateCharacterParams{Account: "buyer", Name: "buyer", Gender: 0, Job: 2, Map: 1, MapX: 0, MapY: 0})
	if err != nil {
		t.Fatal(err)
	}
	items := NewItemRepository(store)
	auctions := NewAuctionRepository(store)
	item := protocol.SDItem{ItemID: 100, SeqID: 1, Count: 1, Duration: [2]uint64{10, 20}, ExtAttrList: map[int32]string{1: "x"}}
	if err := items.PutInventoryItem(ctx, sellerDBID, item); err != nil {
		t.Fatal(err)
	}
	auctionID, err := auctions.Register(ctx, sellerDBID, item, "note", 500, 0)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := items.LoadInventory(ctx, sellerDBID)
	if err != nil || len(inventory) != 0 {
		t.Fatalf("inventory after auction = %#v, %v", inventory, err)
	}
	list, err := auctions.Query(ctx, AuctionCategoryAll, time.Now().Unix())
	if err != nil || len(list.ItemList) != 1 {
		t.Fatalf("auction list = %#v, %v", list, err)
	}
	listing := list.ItemList[0]
	if listing.AuctionID != auctionID || listing.Seller.ID != sellerDBID || listing.Price != 500 || listing.Note != "note" || listing.Item.ItemID != 100 {
		t.Fatalf("listing = %+v", listing)
	}
	if listing.TimeLeft == 0 {
		t.Fatal("time left should be positive")
	}

	unregistered, ok, err := auctions.Unregister(ctx, sellerDBID, auctionID)
	if err != nil || !ok || unregistered.ItemID != 100 || unregistered.SeqID != 0 {
		t.Fatalf("unregister = %+v, %v, %v", unregistered, ok, err)
	}

	if err := items.PutInventoryItem(ctx, sellerDBID, item); err != nil {
		t.Fatal(err)
	}
	auctionID, err = auctions.Register(ctx, sellerDBID, item, "again", 700, 0)
	if err != nil {
		t.Fatal(err)
	}
	taken, err := auctions.Take(ctx, auctionID, time.Now().Unix())
	if err != nil || taken.SellerDBID != sellerDBID || taken.Price != 700 || taken.Item.ItemID != 100 {
		t.Fatalf("take = %+v, %v", taken, err)
	}
	if _, err := auctions.Take(ctx, auctionID, time.Now().Unix()); err == nil {
		t.Fatal("expected sold-out error")
	}
	_ = buyer
}

func TestStallLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, sellerDBID := openTestCharacter(t, OpenOptions{Path: filepath.Join(t.TempDir(), "stall.db3")})
	accounts := NewAccountRepository(store)
	characters := NewCharacterRepository(store)
	if err := accounts.Create(ctx, "buyer", "pw"); err != nil {
		t.Fatal(err)
	}
	buyer, err := characters.Create(ctx, CreateCharacterParams{Account: "buyer", Name: "buyer", Gender: 0, Job: 2, Map: 1, MapX: 0, MapY: 0})
	if err != nil {
		t.Fatal(err)
	}
	items := NewItemRepository(store)
	stalls := NewStallRepository(store)
	item := protocol.SDItem{ItemID: 200, SeqID: 1, Count: 2}
	if err := items.PutInventoryItem(ctx, sellerDBID, item); err != nil {
		t.Fatal(err)
	}
	stallItem, err := stalls.AddItem(ctx, sellerDBID, item, 300)
	if err != nil || stallItem.Item.SeqID == 0 || stallItem.Price != 300 {
		t.Fatalf("add stall item = %+v, %v", stallItem, err)
	}
	stall, ok, err := stalls.Load(ctx, sellerDBID)
	if err != nil || !ok || len(stall.ItemList) != 1 {
		t.Fatalf("load stall = %+v, %v, %v", stall, ok, err)
	}
	started, err := stalls.Start(ctx, sellerDBID, "shop", 1, 10, 20)
	if err != nil || !started {
		t.Fatalf("start stall = %v, %v", started, err)
	}
	active, err := stalls.ListSelling(ctx)
	if err != nil || len(active) != 1 {
		t.Fatalf("active stalls = %#v, %v", active, err)
	}
	sold, ok, err := stalls.TakeSoldItem(ctx, sellerDBID, buyer.DBID, 200, stallItem.Item.SeqID)
	if err != nil || !ok || sold.Item.ItemID != 200 || sold.Price != 300 {
		t.Fatalf("take sold item = %+v, %v, %v", sold, ok, err)
	}
	if stopped, err := stalls.Stop(ctx, sellerDBID); err != nil || !stopped {
		t.Fatalf("stop stall = %v, %v", stopped, err)
	}
}

func TestDeliveryListUnclaimed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, dbid := openTestCharacter(t, OpenOptions{Path: filepath.Join(t.TempDir(), "delivery.db3")})
	repo := NewDeliveryRepository(store)
	item := protocol.SDItem{ItemID: 100, Count: 1}
	if _, err := repo.Create(ctx, dbid, []protocol.SDItem{item}, nil); err != nil {
		t.Fatal(err)
	}
	unclaimed, err := repo.ListUnclaimed(ctx, dbid)
	if err != nil || len(unclaimed) != 1 {
		t.Fatalf("unclaimed = %#v, %v", unclaimed, err)
	}
}
