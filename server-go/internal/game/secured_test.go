package game

import (
	"context"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// memSecured 是内存版仓库，用于测试不依赖 DB。
type memSecured struct {
	items map[uint64]protocol.SDItem // itemID<<32|seqID -> item
}

func newMemSecured() *memSecured {
	return &memSecured{items: make(map[uint64]protocol.SDItem)}
}

func secKey(itemID, seqID uint32) uint64 { return uint64(itemID)<<32 | uint64(seqID) }

func (m *memSecured) PutSecuredItem(_ context.Context, _ uint32, item protocol.SDItem) error {
	m.items[secKey(item.ItemID, item.SeqID)] = item
	return nil
}

func (m *memSecured) TakeSecuredItem(_ context.Context, _ uint32, itemID, seqID uint32) (protocol.SDItem, bool, error) {
	k := secKey(itemID, seqID)
	it, ok := m.items[k]
	if !ok {
		return protocol.SDItem{}, false, nil
	}
	delete(m.items, k)
	return it, true, nil
}

func (m *memSecured) LoadSecuredItems(_ context.Context, _ uint32) ([]protocol.SDItem, error) {
	out := make([]protocol.SDItem, 0, len(m.items))
	for _, it := range m.items {
		out = append(out, it)
	}
	return out, nil
}

func (m *memSecured) RemoveInventoryItem(_ context.Context, _ uint32, _, _ uint32) (bool, error) {
	return true, nil
}

// TestSecureAndRetrieve 寄存后再取回，物品与持久都保住。
func TestSecureAndRetrieve(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)

	repo := newMemSecured()
	h := &SecuredHandler{
		Players: f.Players, Broadcast: f.Cast, Logger: discardLogger(),
		Items: f.Items, Repo: repo,
	}

	weaponID, _ := f.Items.FindByName("铁剑")
	// 背包里放一把「持久不满」的武器
	p.Inventory = []Item{{
		ItemID: weaponID, SeqID: 42, Count: 1,
		Durability: 7, MaxDurability: 10,
	}}

	if !h.Secure(f.ctx, p, weaponID, 42) {
		t.Fatal("寄存应该成功")
	}
	if len(p.Inventory) != 0 {
		t.Fatal("寄存后背包应该空了")
	}

	f.Cast.reset()
	if err := h.Retrieve(f.ctx, p.ChannelID, weaponID, 42); err != nil {
		t.Fatalf("取回: %v", err)
	}
	if len(p.Inventory) != 1 {
		t.Fatalf("取回后背包应有 1 件，实际 %d", len(p.Inventory))
	}
	got := p.Inventory[0]
	if got.Durability != 7 || got.MaxDurability != 10 {
		t.Fatalf("持久丢失！%d/%d, want 7/10", got.Durability, got.MaxDurability)
	}
	if n := len(f.Cast.of(p.ChannelID, protocol.SM_REMOVESECUREDITEM)); n != 1 {
		t.Fatalf("应该推 SM_REMOVESECUREDITEM，实际 %d 条", n)
	}
	t.Logf("寄存→取回：持久 %d/%d 保住", got.Durability, got.MaxDurability)
}

// TestRetrieveNotInStorage 取回不存在的物品不报错也不凭空造物。
func TestRetrieveNotInStorage(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)
	h := &SecuredHandler{
		Players: f.Players, Broadcast: f.Cast, Logger: discardLogger(),
		Items: f.Items, Repo: newMemSecured(),
	}

	p.Inventory = nil
	if err := h.Retrieve(f.ctx, p.ChannelID, 999, 1); err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if len(p.Inventory) != 0 {
		t.Fatalf("不该凭空多出物品，实际 %d 件", len(p.Inventory))
	}
}

// TestRepairViaNpcCostsGold 走 NPC 修理：扣钱 + 恢复持久。
func TestRepairViaNpcCostsGold(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)
	p.Gold = 1000

	weaponID, _ := f.Items.FindByName("铁剑")
	p.Inventory = []Item{{
		ItemID: weaponID, SeqID: 5, Count: 1,
		Durability: 20, MaxDurability: 100,
	}}

	// 脚本算价：(100-20)*30 = 2400，但玩家只有 1000 → 应该失败
	h := &npcHostAdapter{
		Players: f.Players, Durability: mkDurHandler(f),
	}
	if h.RepairItem(p.UID, weaponID, 5, false, 2400) {
		t.Fatal("钱不够不该修理成功")
	}
	if p.Gold != 1000 {
		t.Fatalf("失败时不该扣钱，Gold=%d", p.Gold)
	}

	// 便宜一点就能修
	if !h.RepairItem(p.UID, weaponID, 5, false, 300) {
		t.Fatal("钱够应该修理成功")
	}
	if p.Gold != 700 {
		t.Fatalf("应该扣 300，实际 Gold=%d", p.Gold)
	}
	if p.Inventory[0].Durability != p.Inventory[0].MaxDurability {
		t.Fatalf("修好后持久应满：%d/%d",
			p.Inventory[0].Durability, p.Inventory[0].MaxDurability)
	}
	t.Logf("修理：Gold 1000→%d，持久 20/100→%d/%d",
		p.Gold, p.Inventory[0].Durability, p.Inventory[0].MaxDurability)
}
