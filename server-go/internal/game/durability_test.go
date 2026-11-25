package game

import (
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// zeroRand 是「任何 n 都返回 0」的随机源。
//
// ★ 不能用 rand.New(rand.NewSource(0))：
//
//	Go 的 math/rand 即使种子固定，Intn(n) 也不保证第一次返回 0。
//	要用「必中概率」做断言就得真的恒返回 0，否则测试会随机失败。
type zeroRand struct{}

func (zeroRand) Intn(int) int { return 0 }

// mkDurHandler 造一个「必掉持久」的 handler。
func mkDurHandler(f *b5spellFixture) *DurabilityHandler {
	return &DurabilityHandler{
		Items:     f.Items,
		Worlds:    f.Worlds.Worlds,
		Broadcast: f.Cast,
		// 不落库：ItemRepo 留 nil，避免测试依赖 DB
		Golds: nil,
		Rand:  zeroRand{},
	}
}

// TestWeaponLosesDurability 武器攻击时掉持久。
func TestWeaponLosesDurability(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)

	weaponID, ok := f.Items.FindByName("铁剑")
	if !ok {
		t.Skip("物品表里没有铁剑")
	}
	rec, _ := f.Items.Get(weaponID)
	if rec.Equip == nil || rec.Equip.Duration <= 0 {
		t.Skip("铁剑没有持久配置")
	}

	p.Wear[WLGWeapon] = &Item{
		ItemID: weaponID, SeqID: 1, Count: 1,
		Durability: uint32(rec.Equip.Duration), MaxDurability: uint32(rec.Equip.Duration),
	}

	h := mkDurHandler(f)
	f.Cast.reset()
	if destroyed := h.DamageWeapon(f.ctx, p); destroyed {
		t.Fatal("第一点持久不该直接销毁")
	}

	got := p.Wear[WLGWeapon]
	want := uint32(rec.Equip.Duration) - 1
	if got.Durability != want {
		t.Fatalf("持久 = %d, want %d", got.Durability, want)
	}
	if n := len(f.Cast.of(p.ChannelID, protocol.SM_WEARITEMDURATION)); n != 1 {
		t.Fatalf("应该推 SM_WEARITEMDURATION，实际 %d 条", n)
	}
	t.Logf("铁剑持久 %d → %d", rec.Equip.Duration, got.Durability)
}

// TestItemDestroyedWhenDurabilityExhausted 持久耗尽的装备被永久销毁。
func TestItemDestroyedWhenDurabilityExhausted(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)

	weaponID, _ := f.Items.FindByName("铁剑")
	// 当前持久 0，最大持久 10 → 已损坏，再触发一次就销毁
	p.Wear[WLGWeapon] = &Item{
		ItemID: weaponID, SeqID: 1, Count: 1,
		Durability: 0, MaxDurability: 10,
	}

	h := mkDurHandler(f)
	if destroyed := h.DamageWeapon(f.ctx, p); !destroyed {
		t.Fatal("持久耗尽时应该销毁装备")
	}
	if _, exists := p.Wear[WLGWeapon]; exists {
		t.Fatal("销毁后武器位应该空了")
	}
	if n := len(f.Cast.of(p.ChannelID, protocol.SM_EQUIPWEAR)); n != 1 {
		t.Fatalf("应该推 SM_EQUIPWEAR，实际 %d 条", n)
	}
}

// TestBrokenItemGivesNoStats 损坏的装备不提供属性。
func TestBrokenItemGivesNoStats(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)

	weaponID, _ := f.Items.FindByName("铁剑")
	rec, _ := f.Items.Get(weaponID)

	// 完好
	p.Wear[WLGWeapon] = &Item{ItemID: weaponID, SeqID: 1, Count: 1,
		Durability: 10, MaxDurability: 10}
	good := p.CombatStatsFor(f.Items)

	// 损坏
	p.Wear[WLGWeapon] = &Item{ItemID: weaponID, SeqID: 1, Count: 1,
		Durability: 0, MaxDurability: 10}
	broken := p.CombatStatsFor(f.Items)

	if broken.DC.Max >= good.DC.Max {
		t.Fatalf("损坏的武器不该提供 DC：完好 %d vs 损坏 %d", good.DC.Max, broken.DC.Max)
	}
	t.Logf("DC.Max 完好 %d → 损坏 %d（武器 %v）", good.DC.Max, broken.DC.Max, rec.Equip.DC)
}

// TestRepairRestoresAndLowersMax 普通修理恢复持久但永久降低上限。
func TestRepairRestoresAndLowersMax(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)

	weaponID, _ := f.Items.FindByName("铁剑")
	// 满持久 100，当前 20（缺 80）
	p.Inventory = append(p.Inventory, Item{
		ItemID: weaponID, SeqID: 7, Count: 1,
		Durability: 20, MaxDurability: 100,
	})

	h := mkDurHandler(f)
	restored, ok := h.RepairItem(p, weaponID, 7, false)
	if !ok {
		t.Fatal("修理应该成功")
	}
	if restored != 80 {
		t.Fatalf("恢复 %d 点，want 80", restored)
	}

	item := p.Inventory[0]
	if item.Durability != item.MaxDurability {
		t.Fatalf("修好后当前持久应等于最大：%d vs %d", item.Durability, item.MaxDurability)
	}
	if item.MaxDurability >= 100 {
		t.Fatalf("普通修理应该降低最大持久，实际还是 %d", item.MaxDurability)
	}
	t.Logf("普通修理：持久 20/100 → %d/%d", item.Durability, item.MaxDurability)
}

// TestSpecialRepairKeepsMax 特殊修理不损失上限。
func TestSpecialRepairKeepsMax(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)

	weaponID, _ := f.Items.FindByName("铁剑")
	p.Inventory = append(p.Inventory, Item{
		ItemID: weaponID, SeqID: 8, Count: 1,
		Durability: 20, MaxDurability: 100,
	})

	h := mkDurHandler(f)
	if _, ok := h.RepairItem(p, weaponID, 8, true); !ok {
		t.Fatal("特殊修理应该成功")
	}
	item := p.Inventory[0]
	if item.MaxDurability != 100 {
		t.Fatalf("特殊修理不该损失上限，实际 %d", item.MaxDurability)
	}
	if item.Durability != 100 {
		t.Fatalf("特殊修理应该满持久，实际 %d", item.Durability)
	}
}

// TestGoldGoesToWalletNotInventory 拾到金币加到 Gold，不占背包格。
func TestGoldGoesToWalletNotInventory(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)
	p.Gold = 0
	p.Inventory = nil

	h := mkDurHandler(f)
	goldID := f.Items.GoldID()
	if goldID == 0 {
		t.Fatal("物品表里找不到金币")
	}
	rec, ok := f.Items.Get(goldID)
	if !ok || !rec.IsGold() {
		t.Fatalf("itemID %d 不是金币", goldID)
	}

	added := h.AddGold(f.ctx, p, 500)
	if added != 500 || p.Gold != 500 {
		t.Fatalf("金币 = %d（加 %d），want 500", p.Gold, added)
	}
	if len(p.Inventory) != 0 {
		t.Fatalf("金币不该占背包，实际 %d 格", len(p.Inventory))
	}
	if n := len(f.Cast.of(p.ChannelID, protocol.SM_GOLD)); n != 1 {
		t.Fatalf("应该推 SM_GOLD，实际 %d 条", n)
	}
}

// TestGoldCappedAtMax 金币到达上限后不再增加。
func TestGoldCappedAtMax(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)
	p.Gold = MaxGold - 10

	h := mkDurHandler(f)
	added := h.AddGold(f.ctx, p, 100)
	if added != 10 {
		t.Fatalf("只能加 10 枚，实际 %d", added)
	}
	if p.Gold != MaxGold {
		t.Fatalf("金币 = %d, want MaxGold", p.Gold)
	}
}
