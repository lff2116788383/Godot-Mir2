package game

import (
	"testing"
)

// ==================== 物品表 ====================

func testItemDB(t *testing.T) *ItemDB {
	t.Helper()
	db, err := LoadItemDB("../../content/items.json")
	if err != nil {
		t.Fatalf("LoadItemDB: %v", err)
	}
	return db
}

func TestItemDBLoads(t *testing.T) {
	db := testItemDB(t)
	if db.Count() < 1000 {
		t.Fatalf("item count = %d, want >= 1000", db.Count())
	}

	// 金创药（小）：itemID=6，type=恢复药水，hp=30
	rec, ok := db.Get(6)
	if !ok {
		t.Fatal("item 6 not found")
	}
	if rec.Name != "金创药（小）" {
		t.Fatalf("item 6 name = %q", rec.Name)
	}
	if rec.Type != TypePotion {
		t.Fatalf("item 6 type = %q, want 恢复药水", rec.Type)
	}
	if rec.Potion == nil || rec.Potion.HP != 30 {
		t.Fatalf("item 6 potion = %+v, want hp 30", rec.Potion)
	}
}

func TestItemDBNotFound(t *testing.T) {
	db := testItemDB(t)
	if _, ok := db.Get(0); ok {
		t.Fatal("itemID 0 should not exist (empty placeholder)")
	}
	if _, ok := db.Get(999999); ok {
		t.Fatal("missing itemID should not resolve")
	}
}

func TestItemDBFindByName(t *testing.T) {
	db := testItemDB(t)
	id, ok := db.FindByName("金创药（小）")
	if !ok {
		t.Fatal("FindByName failed")
	}
	if id != 6 {
		t.Fatalf("金创药（小） id = %d, want 6", id)
	}
}

// ==================== 穿戴判定 ====================

func TestWearableBySlot(t *testing.T) {
	db := testItemDB(t)

	// 找一件衣服、一件武器、一枚戒指
	var dress, weapon, ring, potion ItemRecord
	for _, probe := range []string{"衣服", "武器", "戒指", "恢复药水"} {
		for id := uint32(1); id < 2000; id++ {
			rec, ok := db.Get(id)
			if !ok || rec.Type != probe {
				continue
			}
			switch probe {
			case "衣服":
				if dress.ItemID == 0 {
					dress = rec
				}
			case "武器":
				if weapon.ItemID == 0 {
					weapon = rec
				}
			case "戒指":
				if ring.ItemID == 0 {
					ring = rec
				}
			case "恢复药水":
				if potion.ItemID == 0 {
					potion = rec
				}
			}
		}
	}

	if dress.ItemID == 0 || weapon.ItemID == 0 || ring.ItemID == 0 || potion.ItemID == 0 {
		t.Fatal("test data missing required item types")
	}

	// 衣服只能穿衣服槽
	if !dress.Wearable(WLGDress) {
		t.Fatal("dress should be wearable on DRESS slot")
	}
	if dress.Wearable(WLGWeapon) {
		t.Fatal("dress should NOT be wearable on WEAPON slot")
	}
	// 武器只能穿武器槽
	if !weapon.Wearable(WLGWeapon) {
		t.Fatal("weapon should be wearable on WEAPON slot")
	}
	if weapon.Wearable(WLGDress) {
		t.Fatal("weapon should NOT be wearable on DRESS slot")
	}
	// 戒指可穿两个戒指槽，不能穿手镯槽
	if !ring.Wearable(WLGRing0) || !ring.Wearable(WLGRing1) {
		t.Fatal("ring should fit both RING slots")
	}
	if ring.Wearable(WLGArmRing0) {
		t.Fatal("ring should NOT fit ARMRING slot")
	}
	// 药水不是装备
	if potion.Wearable(WLGDress) || potion.Beltable() == false {
		t.Fatal("potion: not wearable but beltable")
	}
}

func TestWLGValid(t *testing.T) {
	if WLGValid(WLGNone) {
		t.Fatal("WLGNone should be invalid")
	}
	if !WLGValid(WLGDress) || !WLGValid(WLGCharm) {
		t.Fatal("DRESS..CHARM should be valid")
	}
	if WLGValid(WLGEnd) {
		t.Fatal("WLGEnd should be invalid")
	}
	// 边界值来自 protocoldef.hpp，顺序不能凭记忆写
	if WLGDress != 1 || WLGHelmet != 2 || WLGWeapon != 3 {
		t.Fatalf("L段槽位值错: dress=%d helmet=%d weapon=%d", WLGDress, WLGHelmet, WLGWeapon)
	}
	if WLGShoes != 4 || WLGNecklace != 5 || WLGCharm != 11 {
		t.Fatalf("W段槽位值错: shoes=%d necklace=%d charm=%d", WLGShoes, WLGNecklace, WLGCharm)
	}
}

// ==================== 背包容器 ====================

func newInvPlayer() *Player {
	return &Player{
		UID: 1, DBID: 1, Level: 30, Job: 1,
		HP: 100, MaxHP: 100, MP: 50, MaxMP: 50,
		Wear: make(map[int]*Item),
	}
}

func TestInventoryAddRemove(t *testing.T) {
	p := newInvPlayer()

	// ★ 不可堆叠的物品（这里用「道具」类）同 itemID 也必须各占一格
	//   （serdesmsg.cpp:143 只有 packable 才合并）
	plain := ItemRecord{Type: "道具"}
	p.AddItem(plain, Item{ItemID: 100, SeqID: 1, Count: 1})
	if len(p.Inventory) != 1 {
		t.Fatalf("inventory = %d", len(p.Inventory))
	}
	p.AddItem(plain, Item{ItemID: 100, SeqID: 2, Count: 1})
	if len(p.Inventory) != 2 {
		t.Fatalf("non-packable same itemID should take separate entries, got %d", len(p.Inventory))
	}

	// ★ 可堆叠的物品（药水）才合并
	potion := ItemRecord{Type: TypePotion}
	p2 := newInvPlayer()
	p2.AddItem(potion, Item{ItemID: 6, SeqID: 1, Count: 2})
	p2.AddItem(potion, Item{ItemID: 6, Count: 3})
	if len(p2.Inventory) != 1 {
		t.Fatalf("packable should merge, got %d entries", len(p2.Inventory))
	}
	if p2.Inventory[0].Count != 5 {
		t.Fatalf("merged count = %d, want 5", p2.Inventory[0].Count)
	}

	// 堆叠上限 99：超过就新占一格
	p3 := newInvPlayer()
	p3.AddItem(potion, Item{ItemID: 6, SeqID: 1, Count: 95})
	p3.AddItem(potion, Item{ItemID: 6, Count: 10}) // 95+10 > 99
	if len(p3.Inventory) != 2 {
		t.Fatalf("overflow should take a new entry, got %d", len(p3.Inventory))
	}
}

func TestInventoryRemovePartial(t *testing.T) {
	p := newInvPlayer()
	p.addItemNoRec(Item{ItemID: 100, SeqID: 1, Count: 5})

	if !p.removeItem(100, 1, 2) {
		t.Fatal("partial remove should succeed")
	}
	if p.Inventory[0].Count != 3 {
		t.Fatalf("count = %d, want 3", p.Inventory[0].Count)
	}

	// 移除剩余全部：整格删掉
	if !p.removeItem(100, 1, 3) {
		t.Fatal("remove rest should succeed")
	}
	if len(p.Inventory) != 0 {
		t.Fatalf("inventory should be empty, got %d", len(p.Inventory))
	}

	// 再移除失败
	if p.removeItem(100, 1, 1) {
		t.Fatal("remove from empty should fail")
	}
}

func TestInventoryRemoveTooManyFails(t *testing.T) {
	p := newInvPlayer()
	p.addItemNoRec(Item{ItemID: 100, SeqID: 1, Count: 2})
	// 要移除 3 件但只有 2 件：必须失败且**不改动**数量
	if p.removeItem(100, 1, 3) {
		t.Fatal("removing more than available should fail")
	}
	if p.Inventory[0].Count != 2 {
		t.Fatalf("count must be untouched on failure, got %d", p.Inventory[0].Count)
	}
}

func TestInventorySeqIDUniquePerItem(t *testing.T) {
	p := newInvPlayer()
	p.addItemNoRec(Item{ItemID: 100, SeqID: 3, Count: 1})
	if got := p.nextSeqID(100); got != 4 {
		t.Fatalf("nextSeqID(100) = %d, want 4", got)
	}
	// 不同 itemID 各自从 1 开始
	if got := p.nextSeqID(200); got != 1 {
		t.Fatalf("nextSeqID(200) = %d, want 1", got)
	}
}

func TestWearSlot(t *testing.T) {
	p := newInvPlayer()
	if p.wearItem(WLGWeapon) != nil {
		t.Fatal("weapon slot should start empty")
	}
	p.setWearItem(WLGWeapon, &Item{ItemID: 100, SeqID: 1, Count: 1})
	if got := p.wearItem(WLGWeapon); got == nil || got.ItemID != 100 {
		t.Fatalf("wear item = %+v", got)
	}
	// 清空
	p.setWearItem(WLGWeapon, nil)
	if p.wearItem(WLGWeapon) != nil {
		t.Fatal("slot should be empty after clear")
	}
}

func TestWearSlotStoresCopy(t *testing.T) {
	// setWearItem 必须存副本：否则外部改背包里的对象会同时改掉装备
	p := newInvPlayer()
	src := Item{ItemID: 100, SeqID: 1, Count: 1}
	p.setWearItem(WLGWeapon, &src)
	src.Count = 99 // 改源对象
	if got := p.wearItem(WLGWeapon); got.Count != 1 {
		t.Fatalf("wear slot should hold a copy, got count %d", got.Count)
	}
}

func TestBeltSlot(t *testing.T) {
	p := newInvPlayer()
	if p.beltItem(0) != nil {
		t.Fatal("belt should start empty")
	}
	p.setBeltItem(0, &Item{ItemID: 6, SeqID: 1, Count: 1})
	if got := p.beltItem(0); got == nil || got.ItemID != 6 {
		t.Fatalf("belt item = %+v", got)
	}
	// 越界槽位静默忽略
	p.setBeltItem(-1, &Item{ItemID: 9})
	p.setBeltItem(6, &Item{ItemID: 9})
	if p.beltItem(-1) != nil || p.beltItem(6) != nil {
		t.Fatal("out-of-range belt slot should be ignored")
	}
}

// ==================== 药剂效果 ====================

func TestConsumePotionHeals(t *testing.T) {
	p := newInvPlayer()
	p.HP = 20
	p.MP = 10
	rec := ItemRecord{Type: TypePotion, Potion: &ItemPotion{HP: 30, MP: 15}}
	if !p.consumePotion(rec) {
		t.Fatal("consumePotion should succeed")
	}
	if p.HP != 50 {
		t.Fatalf("HP = %d, want 50", p.HP)
	}
	if p.MP != 25 {
		t.Fatalf("MP = %d, want 25", p.MP)
	}
}

func TestConsumePotionCapsAtMax(t *testing.T) {
	p := newInvPlayer()
	p.HP = 95
	p.MaxHP = 100
	rec := ItemRecord{Type: TypePotion, Potion: &ItemPotion{HP: 50}}
	p.consumePotion(rec)
	if p.HP != 100 {
		t.Fatalf("HP should cap at MaxHP 100, got %d", p.HP)
	}
}

func TestConsumePotionNoEffect(t *testing.T) {
	p := newInvPlayer()
	// 没 potion 字段
	if p.consumePotion(ItemRecord{Type: TypePotion}) {
		t.Fatal("potion without attribute should not change anything")
	}
	// hp/mp 都是 0
	if p.consumePotion(ItemRecord{Type: TypePotion, Potion: &ItemPotion{}}) {
		t.Fatal("zero-effect potion should report failure")
	}
}

// ==================== 腰带判定 ====================

func TestBeltableTypes(t *testing.T) {
	// 只有恢复药水与传送卷轴可上腰带（itemrecord.hpp:161-167）
	for _, typ := range []string{TypePotion, TypeScroll} {
		if !(ItemRecord{Type: typ}).Beltable() {
			t.Fatalf("%s should be beltable", typ)
		}
	}
	// 强效药水、药粉、护身符都不能
	for _, typ := range []string{TypeDope, TypePowder, TypeCharm, TypeGold, TypeWeapon} {
		if (ItemRecord{Type: typ}).Beltable() {
			t.Fatalf("%s should NOT be beltable", typ)
		}
	}
}

// ==================== 技能书 ====================

func TestMagicBookDetection(t *testing.T) {
	// 普通技能书：type 是技能书但没有 magic，不能学
	plain := ItemRecord{Type: TypeBook, Book: &ItemBook{Job: "战士", Level: 7}}
	if !plain.IsBook() {
		t.Fatal("should be recognized as book")
	}
	if plain.IsMagicBook() {
		t.Fatal("plain skill book must NOT be a magic book")
	}

	// 秘籍：有 magic 才能学
	secret := ItemRecord{Type: TypeBook, Book: &ItemBook{Job: "战士", Level: 7, Magic: "攻杀剑术"}}
	if !secret.IsMagicBook() {
		t.Fatal("book with magic should be a magic book")
	}

	// 非技能书
	if (ItemRecord{Type: TypeWeapon}).IsBook() {
		t.Fatal("weapon is not a book")
	}
}

// ==================== 等级门槛 ====================

func TestCanWearLevelGate(t *testing.T) {
	// 便宜装备低等级也能穿
	cheap := ItemRecord{Price: 100}
	if !canWear(cheap, 1, 1) {
		t.Fatal("cheap item should be wearable at level 1")
	}
	// 极贵装备需要高等级（当前按 price/1000 估算，最多 50）
	expensive := ItemRecord{Price: 100000}
	if canWear(expensive, 1, 1) {
		t.Fatal("expensive item should require higher level")
	}
	if !canWear(expensive, 50, 1) {
		t.Fatal("level 50 should satisfy the highest gate")
	}
}
