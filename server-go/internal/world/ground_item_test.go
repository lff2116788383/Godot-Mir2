package world

import "testing"

func TestDropItemsSharesGroundSeq(t *testing.T) {
	st := newAOIMap(t)

	// 一批 3 件物品，应共享同一个 groundSeq
	events := st.DropItems(10, 10, []uint32{100, 101, 102})
	if len(events) != 1 || events[0].Kind != "appear" {
		t.Fatalf("events = %+v, want one appear", events)
	}

	if got := st.CountGroundItem(10, 10, 100); got != 1 {
		t.Fatalf("count of item 100 = %d, want 1", got)
	}
	ids := st.GroundItemIDsAt(10, 10)
	if len(ids) != 3 {
		t.Fatalf("ground ids = %v, want 3 items", ids)
	}

	// 同批共享 groundSeq
	var seqs []uint64
	for _, item := range st.groundItems {
		seqs = append(seqs, item.GroundSeq)
	}
	if len(seqs) != 3 {
		t.Fatalf("groundItems = %d, want 3", len(seqs))
	}
	if !(seqs[0] == seqs[1] && seqs[1] == seqs[2]) {
		t.Fatalf("same batch should share groundSeq, got %v", seqs)
	}

	// 第二批判新，seq 必须递增
	st.DropItems(11, 10, []uint32{200})
	var lastSeq uint64
	for _, item := range st.groundItems {
		if item.X == 11 && item.ItemID == 200 {
			lastSeq = item.GroundSeq
		}
	}
	if lastSeq <= seqs[0] {
		t.Fatalf("new batch seq %d should exceed first batch %d", lastSeq, seqs[0])
	}

	// 不可走格掉落被忽略（原版 fflassert(groundValid)）
	md := st.Data
	_ = md.SetLand(50, 50, Land{CanWalk: false})
	if ev := st.DropItems(50, 50, []uint32{300}); len(ev) != 0 {
		t.Fatalf("drop on unwalkable grid should be ignored, got %+v", ev)
	}
	// 越界同样忽略
	if ev := st.DropItems(-1, 999, []uint32{300}); len(ev) != 0 {
		t.Fatalf("drop out of bounds should be ignored, got %+v", ev)
	}
}

func TestDropItemsEmpty(t *testing.T) {
	st := newAOIMap(t)
	if ev := st.DropItems(10, 10, nil); len(ev) != 0 {
		t.Fatalf("empty drop should produce nothing, got %+v", ev)
	}
	if len(st.groundItems) != 0 {
		t.Fatal("empty drop should not allocate ground items")
	}
}

func TestPickupRemovesFromTail(t *testing.T) {
	st := newAOIMap(t)

	// 同一格掉两件相同 itemID
	st.DropItems(10, 10, []uint32{100})
	st.DropItems(10, 10, []uint32{100})
	if got := st.CountGroundItem(10, 10, 100); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}

	// 捡一件
	ok, ev := st.PickupItem(10, 10, 100)
	if !ok {
		t.Fatal("pickup should succeed")
	}
	if len(ev) != 1 || ev[0].Kind != "change" {
		t.Fatalf("events = %+v, want one change", ev)
	}
	if got := st.CountGroundItem(10, 10, 100); got != 1 {
		t.Fatalf("count after pickup = %d, want 1", got)
	}

	// 再捡一件
	if ok, _ := st.PickupItem(10, 10, 100); !ok {
		t.Fatal("second pickup should succeed")
	}
	if got := st.CountGroundItem(10, 10, 100); got != 0 {
		t.Fatalf("count after second pickup = %d, want 0", got)
	}

	// 空格捡东西
	if ok, _ := st.PickupItem(10, 10, 100); ok {
		t.Fatal("pickup from empty grid should fail")
	}
}

func TestPickupKeepsOtherItems(t *testing.T) {
	st := newAOIMap(t)
	st.DropItems(10, 10, []uint32{100, 200, 300})

	// 捡中间的 200
	if ok, _ := st.PickupItem(10, 10, 200); !ok {
		t.Fatal("pickup 200 should succeed")
	}
	ids := st.GroundItemIDsAt(10, 10)
	if len(ids) != 2 {
		t.Fatalf("remaining = %v, want 2 items", ids)
	}
	for _, id := range ids {
		if id == 200 {
			t.Fatal("picked item 200 should be gone")
		}
	}
	// 明细也必须同步
	details := 0
	for _, item := range st.groundItems {
		if item.X == 10 && item.Y == 10 {
			details++
			if item.ItemID == 200 {
				t.Fatal("item 200 should be gone from details too")
			}
		}
	}
	if details != 2 {
		t.Fatalf("groundItems for (10,10) = %d, want 2", details)
	}
}

func TestExpireGroundItemsWholeBatch(t *testing.T) {
	st := newAOIMap(t)
	st.NowMS = 1000

	// 第一批 2 件
	st.DropItems(10, 10, []uint32{100, 101})
	// 时间过半，第二批 1 件（过期时刻更晚）
	st.NowMS = 1000 + GroundItemExpireMS/2
	st.DropItems(11, 10, []uint32{200})

	// 推进到第一批刚好过期
	st.NowMS = 1000 + GroundItemExpireMS
	events := st.ExpireGroundItems()
	if len(events) != 1 || events[0].X != 10 {
		t.Fatalf("events = %+v, want expire at (10,10)", events)
	}
	if got := st.GroundItemIDsAt(10, 10); len(got) != 0 {
		t.Fatalf("first batch should be cleared, got %v", got)
	}
	// 第二批还没到期
	if got := st.GroundItemIDsAt(11, 10); len(got) != 1 {
		t.Fatalf("second batch should survive, got %v", got)
	}

	// 再推进到第二批过期
	st.NowMS = 1000 + GroundItemExpireMS/2 + GroundItemExpireMS
	events = st.ExpireGroundItems()
	if len(events) != 1 || events[0].X != 11 {
		t.Fatalf("events = %+v, want expire at (11,10)", events)
	}
	if len(st.groundItems) != 0 {
		t.Fatalf("all items should be gone, %d left", len(st.groundItems))
	}
}

func TestExpireIsBatchWide(t *testing.T) {
	// 同一批掉在两格（原版一次掉落可能产生多格），必须同时过期
	st := newAOIMap(t)
	st.NowMS = 0

	// 手工构造同批两格，模拟原版 addGridItemID 被同批多次调用
	st.DropItems(10, 10, []uint32{100})
	batch := st.groundItems[0].GroundSeq
	st.DropItems(20, 20, []uint32{200})
	// 把第二件强制改成同批
	st.groundItems[len(st.groundItems)-1].GroundSeq = batch
	st.groundItems[len(st.groundItems)-1].ExpireMS = st.groundItems[0].ExpireMS

	st.NowMS = GroundItemExpireMS
	events := st.ExpireGroundItems()
	if len(events) != 2 {
		t.Fatalf("same batch across 2 grids should produce 2 events, got %+v", events)
	}
	if len(st.groundItems) != 0 {
		t.Fatal("whole batch should be cleared")
	}
}

func TestExpireNoItemsNoEvents(t *testing.T) {
	st := newAOIMap(t)
	if ev := st.ExpireGroundItems(); len(ev) != 0 {
		t.Fatalf("empty map should produce no events, got %+v", ev)
	}
}

func TestCollectGroundItemsSkipsUnwalkable(t *testing.T) {
	st := newAOIMap(t)
	st.DropItems(10, 10, []uint32{100})
	// 在不可走格上手工塞物品（模拟墙上的残留）
	_ = st.Data.SetLand(12, 10, Land{CanWalk: false})
	cell, err := st.cell(12, 10)
	if err != nil {
		t.Fatalf("cell: %v", err)
	}
	cell.GroundItems = []uint64{200}
	st.groundItems = append(st.groundItems, GroundItem{
		ItemID: 200, SeqID: 1, GroundSeq: 99, ExpireMS: st.NowMS + GroundItemExpireMS, X: 12, Y: 10,
	})

	snap := st.CollectGroundItems(10, 10, 6)
	if len(snap) != 1 {
		t.Fatalf("snapshot = %+v, want only the walkable grid", snap)
	}
	if snap[0].X != 10 || len(snap[0].ItemIDs) != 1 || snap[0].ItemIDs[0] != 100 {
		t.Fatalf("snapshot[0] = %+v, want (10,10) with item 100", snap[0])
	}

	// r=0 不收集
	if got := st.CollectGroundItems(10, 10, 0); len(got) != 0 {
		t.Fatalf("range 0 should collect nothing, got %+v", got)
	}
}

func TestCollectGroundItemsRespectsRadius(t *testing.T) {
	st := newAOIMap(t)
	st.DropItems(10, 10, []uint32{100})
	st.DropItems(20, 10, []uint32{200}) // 距离 10，超出 r=6 的 (6-1)²=25

	snap := st.CollectGroundItems(10, 10, 6)
	if len(snap) != 1 {
		t.Fatalf("snapshot = %+v, want only the near grid", snap)
	}

	// 半径够大就能收两格
	snap = st.CollectGroundItems(10, 10, 12)
	if len(snap) != 2 {
		t.Fatalf("snapshot with r=12 = %+v, want both grids", snap)
	}
}

func TestAdvanceEmitsGroundItemExpire(t *testing.T) {
	st := newAOIMap(t)
	st.NowMS = 0
	st.DropItems(10, 10, []uint32{100})

	events := st.Advance(GroundItemExpireMS)
	var gotExpire bool
	for _, ev := range events {
		if ev.Kind == "ground_item_expire" && ev.X == 10 && ev.Y == 10 {
			gotExpire = true
		}
	}
	if !gotExpire {
		t.Fatalf("Advance should emit ground_item_expire, got %+v", events)
	}
	if len(st.GroundItemIDsAt(10, 10)) != 0 {
		t.Fatal("items should be expired after Advance")
	}
}
