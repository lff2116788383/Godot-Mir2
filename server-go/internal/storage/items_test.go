package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

func openTestCharacter(t *testing.T, options OpenOptions) (*Store, uint32) {
	t.Helper()
	ctx := context.Background()
	if options.Path == "" {
		options.Path = filepath.Join(t.TempDir(), "test.db3")
	}
	options.CreateIfMissing = true
	store, err := Open(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	accounts := NewAccountRepository(store)
	characters := NewCharacterRepository(store)
	if err := accounts.Create(ctx, "alice", "pw"); err != nil {
		t.Fatal(err)
	}
	character, err := characters.Create(ctx, CreateCharacterParams{
		Account: "alice", Name: "hero", Gender: 1, Job: 1, Map: 1, MapX: 10, MapY: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, character.DBID
}

func TestInventoryRoundTripAndReplace(t *testing.T) {
	t.Parallel()

	store, dbid := openTestCharacter(t, OpenOptions{})
	ctx := context.Background()
	repo := NewItemRepository(store)
	item := protocol.SDItem{
		ItemID: 100, SeqID: 1, Count: 3, Duration: [2]uint64{90, 100},
		ExtAttrList: map[int32]string{1: "abc", 2: "def"},
	}
	if err := repo.PutInventoryItem(ctx, dbid, item); err != nil {
		t.Fatal(err)
	}
	items, err := repo.LoadInventory(ctx, dbid)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !reflect.DeepEqual(items[0], item) {
		t.Fatalf("inventory = %#v", items)
	}

	item2 := protocol.SDItem{ItemID: 101, SeqID: 2, Count: 1}
	if err := repo.StoreInventory(ctx, dbid, []protocol.SDItem{item, item2}); err != nil {
		t.Fatal(err)
	}
	items, err = repo.LoadInventory(ctx, dbid)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || !reflect.DeepEqual(items[0], item) || !reflect.DeepEqual(items[1], item2) {
		t.Fatalf("inventory after replace = %#v", items)
	}

	removed, err := repo.RemoveInventoryItem(ctx, dbid, 100, 1)
	if err != nil || !removed {
		t.Fatalf("remove inventory item = %v, %v", removed, err)
	}
	items, err = repo.LoadInventory(ctx, dbid)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ItemID != 101 {
		t.Fatalf("inventory after remove = %#v", items)
	}

	duplicate := []protocol.SDItem{item, item}
	if err := repo.StoreInventory(ctx, dbid, duplicate); err == nil {
		t.Fatal("expected duplicate item error")
	}
}

func TestWearBeltAndSecuredItems(t *testing.T) {
	t.Parallel()

	store, dbid := openTestCharacter(t, OpenOptions{})
	ctx := context.Background()
	repo := NewItemRepository(store)
	wearItem := protocol.SDItem{ItemID: 200, Count: 1, Duration: [2]uint64{50, 60}, ExtAttrList: map[int32]string{3: "x"}}
	if err := repo.PutWearItem(ctx, dbid, 3, wearItem); err != nil {
		t.Fatal(err)
	}
	wear, err := repo.LoadWear(ctx, dbid)
	if err != nil {
		t.Fatal(err)
	}
	gotWear, ok := wear.GetWLItem(3)
	if !ok || !reflect.DeepEqual(gotWear, wearItem) {
		t.Fatalf("wear = %#v", wear.List)
	}

	beltItem := protocol.SDItem{ItemID: 300, Count: 10}
	if err := repo.PutBeltItem(ctx, dbid, 2, beltItem); err != nil {
		t.Fatal(err)
	}
	belt, err := repo.LoadBelt(ctx, dbid)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(belt.List[2], beltItem) {
		t.Fatalf("belt slot = %#v", belt.List[2])
	}

	securedItem := protocol.SDItem{
		ItemID: 400, SeqID: 5, Count: 2, Duration: [2]uint64{7, 8},
		ExtAttrList: map[int32]string{9: "nine"},
	}
	if err := repo.PutSecuredItem(ctx, dbid, securedItem); err != nil {
		t.Fatal(err)
	}
	secured, err := repo.LoadSecuredItems(ctx, dbid)
	if err != nil {
		t.Fatal(err)
	}
	if len(secured) != 1 || !reflect.DeepEqual(secured[0], securedItem) {
		t.Fatalf("secured = %#v", secured)
	}
	taken, ok, err := repo.TakeSecuredItem(ctx, dbid, 400, 5)
	if err != nil || !ok || !reflect.DeepEqual(taken, securedItem) {
		t.Fatalf("take secured = %#v, %v, %v", taken, ok, err)
	}
	secured, err = repo.LoadSecuredItems(ctx, dbid)
	if err != nil || len(secured) != 0 {
		t.Fatalf("secured after take = %#v, %v", secured, err)
	}
}

func TestMagicAndPlayerConfig(t *testing.T) {
	t.Parallel()

	store, dbid := openTestCharacter(t, OpenOptions{})
	ctx := context.Background()
	repo := NewItemRepository(store)
	if err := repo.LearnMagic(ctx, dbid, 10); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMagicExp(ctx, dbid, 10, 25); err != nil {
		t.Fatal(err)
	}
	magic, err := repo.LoadLearnedMagic(ctx, dbid)
	if err != nil {
		t.Fatal(err)
	}
	if len(magic) != 1 || magic[0].MagicID != 10 || magic[0].Exp != 25 {
		t.Fatalf("magic = %#v", magic)
	}

	keys := protocol.SDMagicKeyList{KeyList: map[uint32]uint8{10: 'a', 11: 'b'}}
	if err := repo.UpdateMagicKeyList(ctx, dbid, keys); err != nil {
		t.Fatal(err)
	}
	runtimeConfig := protocol.SDRuntimeConfig{Config: map[string]string{"volume": "0.5"}}
	if err := repo.UpdateRuntimeConfig(ctx, dbid, runtimeConfig); err != nil {
		t.Fatal(err)
	}
	config, err := repo.LoadPlayerConfig(ctx, dbid)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.MagicKeyList, keys) || !reflect.DeepEqual(config.RuntimeConfig, runtimeConfig) {
		t.Fatalf("player config = %#v", config)
	}
}

func TestGoldAndCharacterVars(t *testing.T) {
	t.Parallel()

	store, dbid := openTestCharacter(t, OpenOptions{})
	ctx := context.Background()
	gold := NewGoldRepository(store)
	if value, err := gold.Load(ctx, dbid); err != nil || value != 10000 {
		t.Fatalf("initial gold = %d, %v", value, err)
	}
	remaining, ok, err := gold.Remove(ctx, dbid, 3000)
	if err != nil || !ok || remaining != 7000 {
		t.Fatalf("remove gold = %d, %v, %v", remaining, ok, err)
	}
	if remaining, ok, err = gold.Remove(ctx, dbid, 999999); err != nil || ok {
		t.Fatalf("overdraw = %d, %v, %v", remaining, ok, err)
	}
	if total, err := gold.Add(ctx, dbid, 500); err != nil || total != 7500 {
		t.Fatalf("add gold = %d, %v", total, err)
	}
	if err := gold.Set(ctx, dbid, 123); err != nil {
		t.Fatal(err)
	}
	if value, err := gold.Load(ctx, dbid); err != nil || value != 123 {
		t.Fatalf("set gold = %d, %v", value, err)
	}

	vars := NewCharacterVarRepository(store)
	value := protocol.NewLuaTableVar(protocol.LuaTableEntry{
		Key:   protocol.WrapLuaVar(protocol.NewLuaStringVar("k")),
		Value: protocol.WrapLuaVar(protocol.NewLuaIntegerVar(7)),
	})
	if err := vars.Set(ctx, dbid, "quest", value); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := vars.Load(ctx, dbid, "quest")
	if err != nil || !ok || !reflect.DeepEqual(loaded, value) {
		t.Fatalf("character var = %#v, %v, %v", loaded, ok, err)
	}
	if err := vars.Set(ctx, dbid, "quest", protocol.NewLuaNilVar()); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := vars.Load(ctx, dbid, "quest"); err != nil || ok {
		t.Fatalf("character var after delete = %v, %v", ok, err)
	}
}

func TestQuestFields(t *testing.T) {
	t.Parallel()

	store, dbid := openTestCharacter(t, OpenOptions{})
	ctx := context.Background()
	repo := NewQuestRepository(store)
	if err := repo.CreateTable(ctx, "quest_test"); err != nil {
		t.Fatal(err)
	}
	value := protocol.NewLuaStringVar("state")
	if err := repo.UpdateFields(ctx, "quest_test", dbid, map[string]*protocol.LuaVar{"fld_states": &value}, false); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := repo.LoadField(ctx, "quest_test", dbid, "fld_states")
	if err != nil || !ok || !reflect.DeepEqual(loaded, value) {
		t.Fatalf("quest field = %#v, %v, %v", loaded, ok, err)
	}
	if err := repo.UpdateFields(ctx, "quest_test", dbid, map[string]*protocol.LuaVar{"fld_states": nil}, false); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repo.LoadField(ctx, "quest_test", dbid, "fld_states"); err != nil || ok {
		t.Fatalf("quest field after nil = %v, %v", ok, err)
	}
	if err := repo.Delete(ctx, "quest_test", dbid); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `select * from quest_test`); err != nil {
		t.Fatal(err)
	}
}
