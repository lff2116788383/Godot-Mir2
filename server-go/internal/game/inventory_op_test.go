package game

import (
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// ==================== 背包/装备消息处理 ====================
//
// 这批测试的价值在「错误路径」：穿错槽、背包没有、等级不够、
// 腰带越界，都要回正确的错误码。错误码错了客户端会显示成别的提示，
// 而且服务端日志看不出问题。

// newInventoryEnv 建一个只有内存状态的处理环境（不建数据库）。
func newInventoryEnv(t *testing.T) (*InventoryHandler, *Player, *captureBroadcaster) {
	t.Helper()
	db := testItemDB(t)
	players := NewPlayerManager()
	cast := &captureBroadcaster{}

	p := &Player{
		ChannelID: 1, DBID: 1, UID: 0x2800000000000001,
		MapID: 1, MapUID: 0x1000000800000001,
		Name: "tester", Job: 1, Level: 30,
		HP: 50, MaxHP: 100, MP: 20, MaxMP: 50,
		Wear: make(map[int]*Item),
		Inventory: []Item{
			{ItemID: 6, SeqID: 1, Count: 5}, // 金创药（小）
		},
	}
	players.Add(p)

	h := &InventoryHandler{
		Players: players, Items: db, Broadcast: cast,
	}
	return h, p, cast
}

// findWearable 找一件指定类型的物品（测试辅助）。
func findWearable(t *testing.T, db *ItemDB, typ string) ItemRecord {
	t.Helper()
	for id := uint32(1); id < 3000; id++ {
		rec, ok := db.Get(id)
		if ok && rec.Type == typ {
			return rec
		}
	}
	t.Fatalf("no item of type %q found", typ)
	return ItemRecord{}
}

// findBordable 找一个可上腰带的物品。
func findBordable(t *testing.T, db *ItemDB) ItemRecord {
	t.Helper()
	for id := uint32(1); id < 3000; id++ {
		rec, ok := db.Get(id)
		if ok && rec.Beltable() {
			return rec
		}
	}
	t.Fatal("no beltable item found")
	return ItemRecord{}
}

// decodeEquipError 从捕获的消息里解出错误码。
func decodeEquipWearError(t *testing.T, cast *captureBroadcaster, channel uint64) (uint16, bool) {
	t.Helper()
	msgs := cast.of(channel, protocol.SM_EQUIPWEARERROR)
	if len(msgs) == 0 {
		return 0, false
	}
	var e protocol.SMEquipWearError
	if _, err := protocol.ReadWirePayload(&e, msgs[len(msgs)-1].Payload); err != nil {
		t.Fatalf("decode SMEquipWearError: %v", err)
	}
	return e.Error, true
}

// ---------- 穿装备 ----------

func TestEquipWearSuccess(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	weapon := findWearable(t, h.Items, TypeWeapon)

	// 背包里放一把武器
	p.AddItem(weapon, Item{ItemID: weapon.ItemID, SeqID: 1, Count: 1})

	err := h.RequestEquipWear(t.Context(), p.ChannelID, protocol.CMRequestEquipWear{
		ItemID: weapon.ItemID, SeqID: 1, WLType: uint16(WLGWeapon),
	})
	if err != nil {
		t.Fatalf("RequestEquipWear: %v", err)
	}

	// 武器槽应该有它
	got := p.wearItem(WLGWeapon)
	if got == nil || got.ItemID != weapon.ItemID {
		t.Fatalf("wear slot = %+v, want item %d", got, weapon.ItemID)
	}
	// 背包里那份应被移除
	if _, it := p.findItem(weapon.ItemID, 1); it != nil {
		t.Fatal("item should be removed from inventory")
	}
	// 应推 SM_EQUIPWEAR
	if len(cast.of(p.ChannelID, protocol.SM_EQUIPWEAR)) == 0 {
		t.Fatal("should send SM_EQUIPWEAR")
	}
}

func TestEquipWearSwapsOldItemToInventory(t *testing.T) {
	h, p, _ := newInventoryEnv(t)
	weapon := findWearable(t, h.Items, TypeWeapon)

	// 先穿一把
	p.setWearItem(WLGWeapon, &Item{ItemID: weapon.ItemID, SeqID: 9, Count: 1})
	// 背包再放一把（换装用）
	p.Inventory = nil
	p.AddItem(weapon, Item{ItemID: weapon.ItemID, SeqID: 1, Count: 1})

	if err := h.RequestEquipWear(t.Context(), p.ChannelID, protocol.CMRequestEquipWear{
		ItemID: weapon.ItemID, SeqID: 1, WLType: uint16(WLGWeapon),
	}); err != nil {
		t.Fatalf("RequestEquipWear: %v", err)
	}

	// 新装备在位
	got := p.wearItem(WLGWeapon)
	if got == nil || got.SeqID != 1 {
		t.Fatalf("wear slot should hold the new item (seqID 1), got %+v", got)
	}
	// 旧装备回背包（seqID 9）
	if _, it := p.findItem(weapon.ItemID, 9); it == nil {
		t.Fatal("old item should be put back into inventory")
	}
}

func TestEquipWearWrongSlotRejected(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	weapon := findWearable(t, h.Items, TypeWeapon)
	p.AddItem(weapon, Item{ItemID: weapon.ItemID, SeqID: 1, Count: 1})

	// 把武器往衣服槽穿：必须拒绝
	if err := h.RequestEquipWear(t.Context(), p.ChannelID, protocol.CMRequestEquipWear{
		ItemID: weapon.ItemID, SeqID: 1, WLType: uint16(WLGDress),
	}); err != nil {
		t.Fatalf("RequestEquipWear: %v", err)
	}

	code, ok := decodeEquipWearError(t, cast, p.ChannelID)
	if !ok {
		t.Fatal("should send SM_EQUIPWEARERROR")
	}
	if code != EQWErrBadWLType {
		t.Fatalf("error = %d, want BADWLTYPE(%d)", code, EQWErrBadWLType)
	}
	// 装备槽必须没变
	if p.wearItem(WLGDress) != nil {
		t.Fatal("dress slot should stay empty")
	}
}

func TestEquipWearInvalidSlotRejected(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	weapon := findWearable(t, h.Items, TypeWeapon)
	p.AddItem(weapon, Item{ItemID: weapon.ItemID, SeqID: 1, Count: 1})

	if err := h.RequestEquipWear(t.Context(), p.ChannelID, protocol.CMRequestEquipWear{
		ItemID: weapon.ItemID, SeqID: 1, WLType: 0, // WLG_NONE
	}); err != nil {
		t.Fatalf("RequestEquipWear: %v", err)
	}
	code, _ := decodeEquipWearError(t, cast, p.ChannelID)
	if code != EQWErrBadWLType {
		t.Fatalf("error = %d, want BADWLTYPE", code)
	}
}

func TestEquipWearNoItemRejected(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	weapon := findWearable(t, h.Items, TypeWeapon)

	// 背包里没有（只有一个药水）
	if err := h.RequestEquipWear(t.Context(), p.ChannelID, protocol.CMRequestEquipWear{
		ItemID: weapon.ItemID, SeqID: 1, WLType: uint16(WLGWeapon),
	}); err != nil {
		t.Fatalf("RequestEquipWear: %v", err)
	}
	code, _ := decodeEquipWearError(t, cast, p.ChannelID)
	if code != EQWErrNoItem {
		t.Fatalf("error = %d, want NOITEM(%d)", code, EQWErrNoItem)
	}
}

func TestEquipWearBadItemRejected(t *testing.T) {
	h, p, cast := newInventoryEnv(t)

	// 不存在的 itemID
	if err := h.RequestEquipWear(t.Context(), p.ChannelID, protocol.CMRequestEquipWear{
		ItemID: 999999, SeqID: 1, WLType: uint16(WLGWeapon),
	}); err != nil {
		t.Fatalf("RequestEquipWear: %v", err)
	}
	code, _ := decodeEquipWearError(t, cast, p.ChannelID)
	if code != EQWErrBadItem {
		t.Fatalf("error = %d, want BADITEM(%d)", code, EQWErrBadItem)
	}
}

// ---------- 脱装备 ----------

func TestGrabWearSuccess(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	weapon := findWearable(t, h.Items, TypeWeapon)
	p.Inventory = nil
	p.setWearItem(WLGWeapon, &Item{ItemID: weapon.ItemID, SeqID: 1, Count: 1})

	if err := h.RequestGrabWear(t.Context(), p.ChannelID, protocol.CMRequestGrabWear{
		WLType: uint16(WLGWeapon),
	}); err != nil {
		t.Fatalf("RequestGrabWear: %v", err)
	}

	if p.wearItem(WLGWeapon) != nil {
		t.Fatal("wear slot should be empty after grab")
	}
	if _, it := p.findItem(weapon.ItemID, 1); it == nil {
		t.Fatal("grabbed item should be in inventory")
	}
	if len(cast.of(p.ChannelID, protocol.SM_GRABWEAR)) == 0 {
		t.Fatal("should send SM_GRABWEAR")
	}
}

func TestGrabWearEmptySlotRejected(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	// 槽位本来就是空的
	if err := h.RequestGrabWear(t.Context(), p.ChannelID, protocol.CMRequestGrabWear{
		WLType: uint16(WLGDress),
	}); err != nil {
		t.Fatalf("RequestGrabWear: %v", err)
	}
	msgs := cast.of(p.ChannelID, protocol.SM_GRABWEARERROR)
	if len(msgs) == 0 {
		t.Fatal("should send SM_GRABWEARERROR")
	}
	var e protocol.SMGrabWearError
	_, _ = protocol.ReadWirePayload(&e, msgs[0].Payload)
	if e.Error != GWErrNoItem {
		t.Fatalf("error = %d, want NOITEM(%d)", e.Error, GWErrNoItem)
	}
}

// ---------- 腰带 ----------

func TestEquipBeltSuccess(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	potion := findBordable(t, h.Items)
	// 背包里原本有 item 6 的 seqID=1，这里另加一格用 seqID=2。
	// 药水可堆叠，AddItem 会合并进 seqID=1 —— 所以先清空再用独立 seq。
	p.Inventory = nil
	p.AddItem(potion, Item{ItemID: potion.ItemID, SeqID: 2, Count: 3})

	if err := h.RequestEquipBelt(t.Context(), p.ChannelID, protocol.CMRequestEquipBelt{
		ItemID: potion.ItemID, SeqID: 2, Slot: 0,
	}); err != nil {
		t.Fatalf("RequestEquipBelt: %v", err)
	}

	got := p.beltItem(0)
	if got == nil || got.ItemID != potion.ItemID {
		t.Fatalf("belt slot 0 = %+v", got)
	}
	if len(cast.of(p.ChannelID, protocol.SM_EQUIPBELT)) == 0 {
		t.Fatal("should send SM_EQUIPBELT")
	}
}

func TestEquipBeltNonBeltableRejected(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	weapon := findWearable(t, h.Items, TypeWeapon)
	p.AddItem(weapon, Item{ItemID: weapon.ItemID, SeqID: 1, Count: 1})

	if err := h.RequestEquipBelt(t.Context(), p.ChannelID, protocol.CMRequestEquipBelt{
		ItemID: weapon.ItemID, SeqID: 1, Slot: 0,
	}); err != nil {
		t.Fatalf("RequestEquipBelt: %v", err)
	}
	msgs := cast.of(p.ChannelID, protocol.SM_EQUIPBELTERROR)
	if len(msgs) == 0 {
		t.Fatal("should send SM_EQUIPBELTERROR")
	}
	var e protocol.SMEquipBeltError
	_, _ = protocol.ReadWirePayload(&e, msgs[0].Payload)
	if e.Error != EQBErrBadItemType {
		t.Fatalf("error = %d, want BADITEMTYPE(%d)", e.Error, EQBErrBadItemType)
	}
}

func TestEquipBeltBadSlotRejected(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	potion := findBordable(t, h.Items)
	p.AddItem(potion, Item{ItemID: potion.ItemID, SeqID: 2, Count: 1})

	// 腰带只有 6 格（0..5），6 越界
	if err := h.RequestEquipBelt(t.Context(), p.ChannelID, protocol.CMRequestEquipBelt{
		ItemID: potion.ItemID, SeqID: 2, Slot: 6,
	}); err != nil {
		t.Fatalf("RequestEquipBelt: %v", err)
	}
	msgs := cast.of(p.ChannelID, protocol.SM_EQUIPBELTERROR)
	if len(msgs) == 0 {
		t.Fatal("should send SM_EQUIPBELTERROR")
	}
	var e protocol.SMEquipBeltError
	_, _ = protocol.ReadWirePayload(&e, msgs[0].Payload)
	if e.Error != EQBErrBadSlot {
		t.Fatalf("error = %d, want BADSLOT(%d)", e.Error, EQBErrBadSlot)
	}
}

func TestGrabBeltSuccess(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	potion := findBordable(t, h.Items)
	p.Inventory = nil
	p.setBeltItem(1, &Item{ItemID: potion.ItemID, SeqID: 5, Count: 2})

	if err := h.RequestGrabBelt(t.Context(), p.ChannelID, protocol.CMRequestGrabBelt{
		Slot: 1,
	}); err != nil {
		t.Fatalf("RequestGrabBelt: %v", err)
	}

	if p.beltItem(1) != nil {
		t.Fatal("belt slot should be empty after grab")
	}
	if _, it := p.findItem(potion.ItemID, 5); it == nil {
		t.Fatal("grabbed item should be in inventory")
	}
	if len(cast.of(p.ChannelID, protocol.SM_GRABBELT)) == 0 {
		t.Fatal("should send SM_GRABBELT")
	}
}

// ---------- 使用物品 ----------

func TestConsumePotionViaMessage(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	p.HP = 20
	// 背包里已有 item 6（金创药小，hp=30）
	if err := h.ConsumeItem(t.Context(), p.ChannelID, protocol.CMConsumeItem{
		ItemID: 6, SeqID: 1, Count: 1,
	}); err != nil {
		t.Fatalf("ConsumeItem: %v", err)
	}

	if p.HP != 50 {
		t.Fatalf("HP = %d, want 50", p.HP)
	}
	// 数量应从 5 减到 4
	_, it := p.findItem(6, 1)
	if it == nil || it.Count != 4 {
		t.Fatalf("count = %+v, want 4", it)
	}
	// 应推血量
	if len(cast.of(p.ChannelID, protocol.SM_HEALTH)) == 0 {
		t.Fatal("should push SM_HEALTH after potion")
	}
}

func TestConsumeItemNotInInventory(t *testing.T) {
	h, p, cast := newInventoryEnv(t)
	cast.reset()
	if err := h.ConsumeItem(t.Context(), p.ChannelID, protocol.CMConsumeItem{
		ItemID: 6, SeqID: 99, Count: 1, // 错的 seqID
	}); err != nil {
		t.Fatalf("ConsumeItem: %v", err)
	}
	if p.HP != 50 {
		t.Fatalf("HP should not change, got %d", p.HP)
	}
	if len(cast.msgs) != 0 {
		t.Fatalf("should not send anything, got %d msgs", len(cast.msgs))
	}
}

// ---------- 丢弃 ----------

func TestDropItemRemovesFromInventory(t *testing.T) {
	h, p, _ := newInventoryEnv(t)
	// 没有 Worlds（测试内存路径），掉落会失败并**把物品还给玩家**。
	// 先确保背包里只有一格（newInventoryEnv 预置了 5 个药水）
	p.Inventory = []Item{{ItemID: 6, SeqID: 1, Count: 5}}
	before := len(p.Inventory)
	if err := h.DropItem(t.Context(), p.ChannelID, protocol.CMDropItem{
		ItemID: 6, SeqID: 1, Count: 2,
	}); err != nil {
		t.Fatalf("DropItem: %v", err)
	}

	// Worlds 为 nil -> 掉落「失败」-> 物品必须回背包（不能吞）
	if len(p.Inventory) != before {
		t.Fatalf("inventory entries = %d, want %d (item must be returned when drop fails)",
			len(p.Inventory), before)
	}
	_, it := p.findItem(6, 1)
	if it == nil || it.Count != 5 {
		t.Fatalf("item count = %+v, want 5 (unchanged)", it)
	}
}

func TestDropItemNotEnoughFails(t *testing.T) {
	h, p, _ := newInventoryEnv(t)
	// 只有 5 个，要丢 10 个
	if err := h.DropItem(t.Context(), p.ChannelID, protocol.CMDropItem{
		ItemID: 6, SeqID: 1, Count: 10,
	}); err != nil {
		t.Fatalf("DropItem: %v", err)
	}
	_, it := p.findItem(6, 1)
	if it == nil || it.Count != 5 {
		t.Fatalf("count = %+v, want 5 (untouched)", it)
	}
}

// ---------- 未登录 ----------

func TestInventoryHandlersIgnoreUnknownChannel(t *testing.T) {
	h, _, cast := newInventoryEnv(t)
	// 用一个不存在的 channel
	if err := h.RequestEquipWear(t.Context(), 999, protocol.CMRequestEquipWear{
		ItemID: 1, SeqID: 1, WLType: uint16(WLGWeapon),
	}); err != nil {
		t.Fatalf("should not error: %v", err)
	}
	if len(cast.msgs) != 0 {
		t.Fatalf("unknown channel should be ignored, got %d msgs", len(cast.msgs))
	}
}

// ---------- 腰带 payload ----------

func TestBuildBeltPayload(t *testing.T) {
	p := newInvPlayer()
	p.setBeltItem(0, &Item{ItemID: 6, SeqID: 1, Count: 2})
	p.setBeltItem(3, &Item{ItemID: 100, SeqID: 1, Count: 1})

	belt := BuildBeltPayload(p)
	if belt.List[0].ItemID != 6 || belt.List[0].Count != 2 {
		t.Fatalf("slot 0 = %+v", belt.List[0])
	}
	if belt.List[3].ItemID != 100 {
		t.Fatalf("slot 3 = %+v", belt.List[3])
	}
	// 空格是 itemID=0
	if belt.List[1].ItemID != 0 {
		t.Fatalf("slot 1 should be empty, got %+v", belt.List[1])
	}
}
