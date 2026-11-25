package game

import (
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// mkTrades 造一个接在 fixture 上的交易管理器。
func mkTrades(f *b5spellFixture) *DirectTradeManager {
	return NewDirectTradeManager(f.Players, f.Cast, discardLogger())
}

// tradeTwo 建立一次「甲请求 → 乙接受」的交易，返回管理器。
func tradeTwo(t *testing.T, f *b5spellFixture) (*DirectTradeManager, *Player, *Player) {
	t.Helper()
	a := f.mkPlayer(t, 1, "alice", 100, 100)
	b := f.mkPlayer(t, 2, "bob", 101, 100)
	tm := mkTrades(f)

	if code := tm.Request(f.ctx, a, b.UID); code != DirectTradeErrNone {
		t.Fatalf("请求交易失败 code=%d", code)
	}
	if code := tm.Respond(f.ctx, b, a.UID, true); code != DirectTradeErrNone {
		t.Fatalf("接受交易失败 code=%d", code)
	}
	f.Cast.reset()
	return tm, a, b
}

// TestTradeSwapsItems 交易完成后物品互换。
func TestTradeSwapsItems(t *testing.T) {
	f := newB5SpellFixture(t)
	tm, a, b := tradeTwo(t, f)

	swordID, _ := f.Items.FindByName("铁剑")
	potID, _ := f.Items.FindByName("金创药（小）")

	// a 给剑，b 给药
	a.Inventory = []Item{{ItemID: swordID, SeqID: 11, Count: 1, Durability: 8, MaxDurability: 10}}
	b.Inventory = []Item{{ItemID: potID, SeqID: 21, Count: 5}}

	// a 摆并锁定
	if code := tm.Update(f.ctx, a, protocol.SDDirectTradeOfferRequest{
		UID: b.UID, ItemList: []protocol.SDDirectTradeItem{{ItemID: swordID, SeqID: 11, Count: 1}}, Locked: true,
	}); code != DirectTradeErrNone {
		t.Fatalf("a 更新失败 code=%d", code)
	}
	// b 摆并锁定
	if code := tm.Update(f.ctx, b, protocol.SDDirectTradeOfferRequest{
		UID: a.UID, ItemList: []protocol.SDDirectTradeItem{{ItemID: potID, SeqID: 21, Count: 5}}, Locked: true,
	}); code != DirectTradeErrNone {
		t.Fatalf("b 更新失败 code=%d", code)
	}

	// 双方确认
	if code := tm.Commit(f.ctx, a, b.UID); code != DirectTradeErrNone {
		t.Fatalf("a 确认失败 code=%d", code)
	}
	if code := tm.Commit(f.ctx, b, a.UID); code != DirectTradeErrNone {
		t.Fatalf("b 确认失败 code=%d", code)
	}

	// 交换结果
	if findInv(a, swordID) != nil {
		t.Fatal("a 不该还有剑")
	}
	if findInv(a, potID) == nil {
		t.Fatal("a 应该拿到药")
	}
	if findInv(b, swordID) == nil {
		t.Fatal("b 应该拿到剑")
	}
	if findInv(b, potID) != nil {
		t.Fatal("b 不该还有药")
	}
	// 持久值要跟过去
	got := findInv(b, swordID)
	if got.Durability != 8 || got.MaxDurability != 10 {
		t.Fatalf("持久丢失！%d/%d want 8/10", got.Durability, got.MaxDurability)
	}
	t.Logf("交易成功：剑(持久 %d/%d) ↔ 药×5", got.Durability, got.MaxDurability)
}

// TestTradeSwapsGold 金币也能交换。
func TestTradeSwapsGold(t *testing.T) {
	f := newB5SpellFixture(t)
	tm, a, b := tradeTwo(t, f)
	a.Gold, b.Gold = 1000, 500

	// a 给 300 金币，b 什么都不给
	tm.Update(f.ctx, a, protocol.SDDirectTradeOfferRequest{UID: b.UID, Gold: 300, Locked: true})
	tm.Update(f.ctx, b, protocol.SDDirectTradeOfferRequest{UID: a.UID, Locked: true})

	tm.Commit(f.ctx, a, b.UID)
	tm.Commit(f.ctx, b, a.UID)

	if a.Gold != 700 {
		t.Fatalf("a 金币 = %d, want 700", a.Gold)
	}
	if b.Gold != 800 {
		t.Fatalf("b 金币 = %d, want 800", b.Gold)
	}
}

// TestTradeLockedCannotChange 锁定后不能改内容。
func TestTradeLockedCannotChange(t *testing.T) {
	f := newB5SpellFixture(t)
	tm, a, b := tradeTwo(t, f)

	swordID, _ := f.Items.FindByName("铁剑")
	a.Inventory = []Item{{ItemID: swordID, SeqID: 11, Count: 1}}

	tm.Update(f.ctx, a, protocol.SDDirectTradeOfferRequest{
		UID: b.UID, ItemList: []protocol.SDDirectTradeItem{{ItemID: swordID, SeqID: 11, Count: 1}}, Locked: true,
	})

	// 改成空清单 + 保持锁定 → 应该被拒
	f.Cast.reset()
	code := tm.Update(f.ctx, a, protocol.SDDirectTradeOfferRequest{
		UID: b.UID, ItemList: nil, Locked: true,
	})
	if code != DirectTradeErrNone {
		t.Fatalf("期望静默拒绝，实际 code=%d", code)
	}
	// 应该收到 LOCKED 错误
	if n := len(f.Cast.of(a.ChannelID, protocol.SM_DIRECTTRADEERROR)); n == 0 {
		t.Fatal("锁定后改动应该收到 LOCKED 错误")
	}
}

// TestTradeBothMustLock 双方都锁定才能确认。
func TestTradeBothMustLock(t *testing.T) {
	f := newB5SpellFixture(t)
	tm, a, b := tradeTwo(t, f)

	// 只有 a 锁定
	tm.Update(f.ctx, a, protocol.SDDirectTradeOfferRequest{UID: b.UID, Locked: true})
	// b 不锁定

	f.Cast.reset()
	tm.Commit(f.ctx, a, b.UID)
	if n := len(f.Cast.of(a.ChannelID, protocol.SM_DIRECTTRADEERROR)); n == 0 {
		t.Fatal("未双方锁定就确认应该报 NOTREADY")
	}
	// 交易不该完成
	if n := len(f.Cast.of(a.ChannelID, protocol.SM_COMPLETEDIRECTTRADE)); n != 0 {
		t.Fatal("未锁定不该成交")
	}
}

// TestTradeTooFar 距离太远不能发起。
func TestTradeTooFar(t *testing.T) {
	f := newB5SpellFixture(t)
	a := f.mkPlayer(t, 1, "alice", 100, 100)
	b := f.mkPlayer(t, 2, "bob", 150, 150) // 50 格远
	tm := mkTrades(f)

	if code := tm.Request(f.ctx, a, b.UID); code != DirectTradeErrTooFar {
		t.Fatalf("远距离应该 TOOFAR，实际 %d", code)
	}
}

// TestTradeReject 拒绝交易后双方都释放。
func TestTradeReject(t *testing.T) {
	f := newB5SpellFixture(t)
	a := f.mkPlayer(t, 1, "alice", 100, 100)
	b := f.mkPlayer(t, 2, "bob", 101, 100)
	tm := mkTrades(f)

	tm.Request(f.ctx, a, b.UID)
	tm.Respond(f.ctx, b, a.UID, false)

	// 双方都应能重新发起
	if code := tm.Request(f.ctx, a, b.UID); code != DirectTradeErrNone {
		t.Fatalf("拒绝后应该能重新发起，实际 code=%d", code)
	}
}

// TestTradeMissingItemFails 物品中途被拿走时整体失败，不会只成交一半。
func TestTradeMissingItemFails(t *testing.T) {
	f := newB5SpellFixture(t)
	tm, a, b := tradeTwo(t, f)

	swordID, _ := f.Items.FindByName("铁剑")
	a.Inventory = []Item{{ItemID: swordID, SeqID: 11, Count: 1}}
	tm.Update(f.ctx, a, protocol.SDDirectTradeOfferRequest{
		UID: b.UID, ItemList: []protocol.SDDirectTradeItem{{ItemID: swordID, SeqID: 11, Count: 1}}, Locked: true,
	})
	tm.Update(f.ctx, b, protocol.SDDirectTradeOfferRequest{UID: a.UID, Locked: true})

	// 锁定后 a 的剑被别处拿走（比如寄存了）
	a.Inventory = nil

	f.Cast.reset()
	tm.Commit(f.ctx, a, b.UID)
	tm.Commit(f.ctx, b, a.UID)

	// 应该报 COMMITFAILED，且 b 拿不到剑
	if n := len(f.Cast.of(a.ChannelID, protocol.SM_DIRECTTRADEERROR)); n == 0 {
		t.Fatal("物品不足应该报错")
	}
	if findInv(b, swordID) != nil {
		t.Fatal("物品不足时不该凭空给 b 一把剑")
	}
}

// findInv 在背包里找某 itemID（找不到返回 nil）。
func findInv(p *Player, itemID uint32) *Item {
	for i := range p.Inventory {
		if p.Inventory[i].ItemID == itemID {
			return &p.Inventory[i]
		}
	}
	return nil
}
