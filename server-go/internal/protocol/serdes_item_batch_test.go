package protocol

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
)

func TestSerdesItemBatchCppGolden(t *testing.T) {
	t.Parallel()

	fixtures := loadSerdesFixtures(t)
	emptyItem := SDItem{Count: 1}
	item := SDItem{
		ItemID: 100, SeqID: 7, Count: 3, Duration: [2]uint64{90, 100},
		ExtAttrList: map[int32]string{1: "abc"},
	}
	item2 := SDItem{ItemID: 101, SeqID: 8, Count: 1}
	cost := SDCostItem{ItemID: 55, Count: 3}
	groupVar := SDChatPeerGroupVar{
		Creator: 7, Createtime: 0x1122334455667788,
		MemberList: []SDChatGroupMember{{DBID: 1, Priority: 2}, {DBID: 5, Priority: 9}},
	}
	peer := SDChatPeer{
		ID: 42, Name: "alice", AvatarPresent: true, Avatar: 123456789,
		DespKind: SDChatPeerDespGroup, GroupVar: groupVar,
	}
	wear := SDWear{List: map[int32]SDItem{1: item}}
	belt := SDBelt{List: [6]SDItem{item, emptyItem, emptyItem, emptyItem, emptyItem, emptyItem}}
	inventory := SDInventory{ItemList: []SDItem{item, item2}}
	dropOdds := SDDropItemOdds{Item: item, Odds: 2}
	sellItem := SDSellItem{Item: item, CostList: []SDCostItem{cost}}
	auctionItem := SDAuctionItem{
		AuctionID: 0x0102030405060708, Seller: peer, Note: "note",
		TimeLeft: 3600, Price: 500, Item: item,
	}
	directItem := SDDirectTradeItem{ItemID: 100, SeqID: 7, Count: 2}
	stallItem := SDStallItem{Item: item, Price: 500}
	wlDesp := SDWLDesp{Wear: wear, Hair: 3, HairColor: 4}
	storage := SDItemStorage{Gold: 1000, Wear: wear, Belt: belt, Inventory: inventory}

	values := []struct {
		name  string
		value any
	}{
		{"SDItem", item},
		{"SDItemEmptyExtAttrs", item2},
		{"SDDropItem", SDDropItem{X: 1, Y: 2, Item: item}},
		{"SDDropItemList", SDDropItemList{X: 3, Y: 4, ItemList: []SDItem{item, item2}}},
		{"SDDropItemOdds", dropOdds},
		{"SDDropOnDie", SDDropOnDie{PlayerUID: 0x1122334455667788, AllowDefaultDrop: true, ItemList: []SDDropItemOdds{dropOdds}}},
		{"SDDropOnHit", SDDropOnHit{ItemList: []SDDropItemOdds{dropOdds}}},
		{"SDBelt", belt},
		{"SDWear", wear},
		{"SDShowSecuredItemList", SDShowSecuredItemList{ItemList: []SDItem{item, item2}}},
		{"SDSellItem", sellItem},
		{"SDSellItemList", SDSellItemList{NPCUID: 0x1122334455667788, List: []SDSellItem{sellItem}}},
		{"SDAuctionItem", auctionItem},
		{"SDAuctionItemList", SDAuctionItemList{Category: 2, ItemList: []SDAuctionItem{auctionItem}}},
		{"SDUpdateItem", SDUpdateItem{Item: item}},
		{"SDInventory", inventory},
		{"SDGrantItemList", SDGrantItemList{PlayerUID: 0x1122334455667788, ItemList: []SDItem{item, item2}}},
		{"SDItemGrant", SDItemGrant{Gold: 1000, Inventory: inventory}},
		{"SDDirectTradeItem", directItem},
		{"SDDirectTradeOfferRequest", SDDirectTradeOfferRequest{UID: 0x1122334455667788, Gold: 100, Locked: true, ItemList: []SDDirectTradeItem{directItem}}},
		{"SDDirectTradeOffer", SDDirectTradeOffer{UID: 0x1122334455667788, Gold: 200, Locked: true, Confirmed: true, ItemList: []SDItem{item}}},
		{"SDDirectTradeResult", SDDirectTradeResult{UID: 0x1122334455667788, Gold: 300, Inventory: inventory}},
		{"SDEquipWear", SDEquipWear{UID: 42, WLType: 3, Item: item}},
		{"SDGrabWear", SDGrabWear{WLType: 3, Item: item}},
		{"SDEquipBelt", SDEquipBelt{Slot: 2, Item: item}},
		{"SDGrabBelt", SDGrabBelt{Slot: 2, Item: item}},
		{"SDBuyCost", SDBuyCost{Item: item, CostList: []SDCostItem{cost}}},
		{"SDPickUpItemList", SDPickUpItemList{FailedItemID: 99, ItemList: []SDItem{item, item2}}},
		{"SDStallItem", stallItem},
		{"SDStall", SDStall{DBID: 7, Selling: true, Name: "shop", ItemList: []SDStallItem{stallItem}}},
		{"SDItemStorage", storage},
		{"SDWLDesp", wlDesp},
		{"SDUIDWLDesp", SDUIDWLDesp{UID: 42, Desp: wlDesp}},
		{"SDStartGameScene", SDStartGameScene{
			UID: 42, MapUID: 0x1122334455667788, X: -10, Y: 20, Direction: 3,
			Desp: wlDesp, Name: "hero", NameColor: 0x11223344, ViewRange: 15,
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

func TestSDItemAndInventoryHelpers(t *testing.T) {
	t.Parallel()

	item := SDItem{ItemID: 100, SeqID: 7, Count: 1}
	if got := item.ItemIDSeq(); got != 100<<32|7 {
		t.Fatalf("item id seq = %x", got)
	}

	inventory := SDInventory{ItemList: []SDItem{item}}
	if found, ok := inventory.Find(100, 7); !ok || !reflect.DeepEqual(found, item) {
		t.Fatalf("find = %+v, %v", found, ok)
	}
	if _, ok := inventory.Find(100, 8); ok {
		t.Fatal("unexpected inventory match")
	}

	var wear SDWear
	wear.SetWLItem(3, item)
	if found, ok := wear.GetWLItem(3); !ok || !reflect.DeepEqual(found, item) {
		t.Fatalf("wear item = %+v, %v", found, ok)
	}
	wear.Clear()
	if len(wear.List) != 0 {
		t.Fatalf("wear list length = %d", len(wear.List))
	}
}
