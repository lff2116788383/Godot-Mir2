package world

import "testing"

func TestFireWallAttackGap(t *testing.T) {
	// ★ dps 是「每秒攻击次数」，不是伤害值
	cases := []struct {
		dps  int
		want int64
	}{
		{1, 1000}, // 每秒 1 次 -> 间隔 1000ms
		{2, 500},  // 每秒 2 次 -> 间隔 500ms
		{10, 100}, // 每秒 10 次 -> 100ms
		{0, 0},    // 0 表示不攻击
		{-5, 0},   // 负数当作不攻击
		{2000, 1}, // 极大值钳到 1ms，避免除出 0
		{5000, 1}, // 更极端，仍为 1ms
	}
	for _, c := range cases {
		fw := FireWall{DPS: c.dps}
		if got := fw.AttackGapMS(); got != c.want {
			t.Fatalf("dps=%d -> gap=%d, want %d", c.dps, got, c.want)
		}
	}
}

func TestFireWallExpireTime(t *testing.T) {
	fw := FireWall{StartMS: 1000, DurationMS: 5000}
	if got := fw.ExpireMS(); got != 6000 {
		t.Fatalf("expire = %d, want 6000", got)
	}
}

func TestAddFireWallAssignsSeqAndStart(t *testing.T) {
	st := newAOIMap(t)
	st.NowMS = 777

	events := st.AddFireWall(FireWall{X: 10, Y: 10, UID: 100, MinDC: 1, MaxDC: 3, DurationMS: 5000, DPS: 1})
	if len(events) != 1 || events[0].X != 10 || events[0].Y != 10 {
		t.Fatalf("events = %+v, want one event at (10,10)", events)
	}

	walls := st.FireWallsAt(10, 10)
	if len(walls) != 1 {
		t.Fatalf("fire walls = %d, want 1", len(walls))
	}
	if walls[0].GroundSeq == 0 {
		t.Fatal("groundSeq should be auto-assigned")
	}
	if walls[0].StartMS != 777 {
		t.Fatalf("start = %d, want world clock 777", walls[0].StartMS)
	}

	// 不可走格不铺火
	_ = st.Data.SetLand(30, 30, Land{CanWalk: false})
	if ev := st.AddFireWall(FireWall{X: 30, Y: 30, DurationMS: 1000}); len(ev) != 0 {
		t.Fatalf("fire wall on unwalkable grid should be rejected, got %+v", ev)
	}
}

func TestFireWallStackingAllowed(t *testing.T) {
	st := newAOIMap(t)
	// 同格叠两层（原版 fireWallList 是 vector，允许叠加）
	st.AddFireWall(FireWall{X: 10, Y: 10, UID: 1, MinDC: 1, MaxDC: 1, DurationMS: 5000, DPS: 1})
	st.AddFireWall(FireWall{X: 10, Y: 10, UID: 2, MinDC: 1, MaxDC: 1, DurationMS: 5000, DPS: 1})
	if got := len(st.FireWallsAt(10, 10)); got != 2 {
		t.Fatalf("stacked fire walls = %d, want 2", got)
	}
}

func TestTickFireWallsAttacksOccupants(t *testing.T) {
	st := newAOIMap(t)
	st.NowMS = 0

	// 火墙在 (10,10)，创造者 100 站在里面（不该被烧），另有玩家 200、怪物 300、NPC 400
	if _, err := st.AddEntity(Entity{UID: 100, Type: EntityPlayer, X: 10, Y: 10, HP: 100, MaxHP: 100}); err != nil {
		t.Fatalf("AddEntity owner: %v", err)
	}
	// 同格只能站一个实体，其余放在相邻格；
	// 火墙只烧**同格**，所以把 200 放同格测不了 —— 改为在另一格单独建火墙验证
	st.AddFireWall(FireWall{X: 10, Y: 10, UID: 100, MinDC: 5, MaxDC: 5, DurationMS: 5000, DPS: 10})

	st.NowMS = 1000
	hits, _ := st.TickFireWalls()
	for _, hit := range hits {
		if hit.TargetUID == 100 {
			t.Fatal("fire wall should not burn its own creator")
		}
	}

	// 换一格：创造者 100 在 (10,10)，玩家 200 在 (11,10) 并铺火墙
	st2 := newAOIMap(t)
	st2.NowMS = 0
	_, _ = st2.AddEntity(Entity{UID: 100, Type: EntityPlayer, X: 10, Y: 10, HP: 100, MaxHP: 100})
	_, _ = st2.AddEntity(Entity{UID: 200, Type: EntityPlayer, X: 11, Y: 10, HP: 100, MaxHP: 100})
	_, _ = st2.AddEntity(Entity{UID: 300, Type: EntityMonster, X: 12, Y: 10, HP: 100, MaxHP: 100})
	st2.AddFireWall(FireWall{X: 11, Y: 10, UID: 100, MinDC: 5, MaxDC: 5, DurationMS: 5000, DPS: 10})
	st2.AddFireWall(FireWall{X: 12, Y: 10, UID: 100, MinDC: 7, MaxDC: 7, DurationMS: 5000, DPS: 10})

	st2.NowMS = 1000 // 距 start 1000ms，dps=10 -> gap=100ms，应已攻击
	hits, _ = st2.TickFireWalls()

	var hit200, hit300 int32
	for _, hit := range hits {
		if hit.TargetUID == 200 {
			hit200 = hit.Damage
		}
		if hit.TargetUID == 300 {
			hit300 = hit.Damage
		}
	}
	if hit200 != 5 {
		t.Fatalf("player 200 should take 5 damage, got %d", hit200)
	}
	if hit300 != 7 {
		t.Fatalf("monster 300 should take 7 damage, got %d", hit300)
	}
}

func TestTickFireWallsSkipsNPC(t *testing.T) {
	st := newAOIMap(t)
	st.NowMS = 0
	if _, err := st.AddEntity(Entity{UID: 500, Type: EntityNPC, X: 10, Y: 10, HP: 100, MaxHP: 100}); err != nil {
		t.Fatalf("AddEntity npc: %v", err)
	}
	st.AddFireWall(FireWall{X: 10, Y: 10, UID: 999, MinDC: 50, MaxDC: 50, DurationMS: 5000, DPS: 10})

	st.NowMS = 1000
	hits, _ := st.TickFireWalls()
	if len(hits) != 0 {
		t.Fatalf("NPC should not be burned, got %+v", hits)
	}
}

func TestTickFireWallsRespectsAttackGap(t *testing.T) {
	st := newAOIMap(t)
	st.NowMS = 0
	_, _ = st.AddEntity(Entity{UID: 100, Type: EntityPlayer, X: 10, Y: 10, HP: 1000, MaxHP: 1000})
	st.AddFireWall(FireWall{X: 10, Y: 10, UID: 100, MinDC: 1, MaxDC: 1, DurationMS: 10000, DPS: 1})

	// gap = 1000ms。NowMS=500 时不该打
	st.NowMS = 500
	if hits, _ := st.TickFireWalls(); len(hits) != 0 {
		t.Fatalf("before gap, no hits expected, got %+v", hits)
	}
	// NowMS=1000 应该打
	st.NowMS = 1000
	hits, _ := st.TickFireWalls()
	if len(hits) != 0 {
		t.Fatalf("owner in same grid should be excluded; hits = %+v", hits)
	}

	// 放个非创造者进来再测
	st2 := newAOIMap(t)
	st2.NowMS = 0
	_, _ = st2.AddEntity(Entity{UID: 1, Type: EntityPlayer, X: 10, Y: 10, HP: 1000, MaxHP: 1000})
	st2.AddFireWall(FireWall{X: 10, Y: 10, UID: 999, MinDC: 1, MaxDC: 1, DurationMS: 10000, DPS: 1})

	st2.NowMS = 999
	if hits, _ := st2.TickFireWalls(); len(hits) != 0 {
		t.Fatalf("at 999ms (< gap) no hit expected, got %+v", hits)
	}
	st2.NowMS = 1000
	if hits, _ := st2.TickFireWalls(); len(hits) != 1 {
		t.Fatalf("at 1000ms one hit expected, got %+v", hits)
	}
	// 再推进 500ms 不该打（上次是 1000，下次 2000）
	st2.NowMS = 1500
	if hits, _ := st2.TickFireWalls(); len(hits) != 0 {
		t.Fatalf("1500ms should be within gap, got %+v", hits)
	}
	st2.NowMS = 2000
	if hits, _ := st2.TickFireWalls(); len(hits) != 1 {
		t.Fatalf("2000ms should hit again, got %+v", hits)
	}
}

func TestTickFireWallsExpires(t *testing.T) {
	st := newAOIMap(t)
	st.NowMS = 0
	st.AddFireWall(FireWall{X: 10, Y: 10, UID: 1, MinDC: 1, MaxDC: 1, DurationMS: 3000, DPS: 0})

	st.NowMS = 2999
	_, expired := st.TickFireWalls()
	if len(expired) != 0 {
		t.Fatalf("should not expire at 2999, got %+v", expired)
	}
	if got := len(st.FireWallsAt(10, 10)); got != 1 {
		t.Fatalf("fire wall should still exist, %d", got)
	}

	st.NowMS = 3000
	_, expired = st.TickFireWalls()
	if len(expired) != 1 || expired[0].X != 10 || expired[0].Y != 10 {
		t.Fatalf("should expire at 3000, got %+v", expired)
	}
	if got := len(st.FireWallsAt(10, 10)); got != 0 {
		t.Fatalf("fire wall should be removed, %d left", got)
	}
}

func TestFireWallDamageIsDeterministicPerTick(t *testing.T) {
	// 同一 tick、同一火墙必须算出同样的伤害（回放可复现）
	st := newAOIMap(t)
	st.NowMS = 0
	_, _ = st.AddEntity(Entity{UID: 1, Type: EntityPlayer, X: 10, Y: 10, HP: 1000, MaxHP: 1000})
	st.AddFireWall(FireWall{X: 10, Y: 10, UID: 999, MinDC: 1, MaxDC: 100, DurationMS: 100000, DPS: 1000})

	st.NowMS = 1000
	first, _ := st.TickFireWalls()
	if len(first) != 1 {
		t.Fatalf("want 1 hit, got %+v", first)
	}
	st.NowMS = 1000 // 同一时刻再算
	second, _ := st.TickFireWalls()
	// 第二次因为 lastAttackMS 已更新可能不再打，这里直接校验 seed 函数本身
	if randForTick(1000, 1, 10, 10) != randForTick(1000, 1, 10, 10) {
		t.Fatal("randForTick should be deterministic")
	}
	if randForTick(1000, 1, 10, 10) == randForTick(2000, 1, 10, 10) {
		t.Fatal("randForTick should differ across ticks")
	}
	_ = second
}

func TestRemoveFireWallBySeq(t *testing.T) {
	st := newAOIMap(t)
	st.AddFireWall(FireWall{X: 10, Y: 10, UID: 1, DurationMS: 5000})
	st.AddFireWall(FireWall{X: 10, Y: 10, UID: 2, DurationMS: 5000})

	walls := st.FireWallsAt(10, 10)
	if len(walls) != 2 {
		t.Fatalf("walls = %d, want 2", len(walls))
	}
	seq := walls[0].GroundSeq
	if !st.RemoveFireWall(10, 10, seq) {
		t.Fatal("RemoveFireWall should succeed")
	}
	left := st.FireWallsAt(10, 10)
	if len(left) != 1 || left[0].GroundSeq == seq {
		t.Fatalf("wrong wall removed: %+v", left)
	}
	// 重复移除同一个 seq
	if st.RemoveFireWall(10, 10, seq) {
		t.Fatal("removing already-removed seq should fail")
	}
}

func TestCollectFireWalls(t *testing.T) {
	st := newAOIMap(t)
	st.AddFireWall(FireWall{X: 20, Y: 20, UID: 1, DurationMS: 5000})
	st.AddFireWall(FireWall{X: 20, Y: 20, UID: 2, DurationMS: 5000})
	st.AddFireWall(FireWall{X: 60, Y: 60, UID: 3, DurationMS: 5000})

	// r=6 只能收到近的那格（圆形半径 = 5）
	snap := st.CollectFireWalls(20, 20, 6)
	if len(snap) != 1 {
		t.Fatalf("snapshot = %+v, want only (20,20)", snap)
	}
	if len(snap[0].OwnerUIDs) != 2 {
		t.Fatalf("owners = %v, want 2", snap[0].OwnerUIDs)
	}

	// r=0 不收
	if got := st.CollectFireWalls(20, 20, 0); len(got) != 0 {
		t.Fatalf("range 0 should collect nothing, got %+v", got)
	}
}

func TestAdvanceEmitsFireWallEvents(t *testing.T) {
	st := newAOIMap(t)
	st.NowMS = 0
	_, _ = st.AddEntity(Entity{UID: 1, Type: EntityPlayer, X: 10, Y: 10, HP: 100, MaxHP: 100})
	// 时长必须 > gap(1000ms)，否则「到期判断先于攻击」会吃掉这一刀
	// （原版 runFireWall 就是先判到期再攻击，不是 bug）
	st.AddFireWall(FireWall{X: 10, Y: 10, UID: 999, MinDC: 3, MaxDC: 3, DurationMS: 2000, DPS: 1})

	events := st.Advance(1000)
	var gotHit bool
	for _, ev := range events {
		if ev.Kind == "firewall_hit" && ev.UID == 1 && ev.Amount == 3 {
			gotHit = true
		}
		if ev.Kind == "firewall_expire" {
			t.Fatalf("should not expire at 1000ms with duration 2000, got %+v", ev)
		}
	}
	if !gotHit {
		t.Fatalf("Advance should emit firewall_hit, got %+v", events)
	}

	// 再推进到 2000ms：到期
	events = st.Advance(2000)
	var gotExpire bool
	for _, ev := range events {
		if ev.Kind == "firewall_expire" {
			gotExpire = true
		}
	}
	if !gotExpire {
		t.Fatalf("Advance should emit firewall_expire at 2000ms, got %+v", events)
	}
	if got := len(st.FireWallsAt(10, 10)); got != 0 {
		t.Fatalf("fire wall should be removed, %d left", got)
	}
}
