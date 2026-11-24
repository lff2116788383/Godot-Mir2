package world

import (
	"sort"
	"testing"
)

// ---------- 圆形遍历语义 ----------

func TestLDistance2IsEuclideanSquared(t *testing.T) {
	// 3-4-5 直角三角形：欧氏距离平方是 25，切比雪夫是 4
	if got := LDistance2(0, 0, 3, 4); got != 25 {
		t.Fatalf("LDistance2(0,0,3,4) = %d, want 25", got)
	}
	if got := LDistance2(0, 0, 0, 0); got != 0 {
		t.Fatalf("LDistance2 same point = %d, want 0", got)
	}
	// 对称性
	if LDistance2(2, 7, 5, 3) != LDistance2(5, 3, 2, 7) {
		t.Fatal("LDistance2 should be symmetric")
	}
}

func TestInCircleUsesRadiusMinusOne(t *testing.T) {
	// ★ 关键语义：半径 r 的实际覆盖是 (r-1)²
	// r=1 -> 只覆盖中心格（(1-1)²=0）
	if !InCircle(5, 5, 5, 5, 1) {
		t.Fatal("r=1 should cover the center grid")
	}
	if InCircle(6, 5, 5, 5, 1) {
		t.Fatal("r=1 should NOT cover adjacent grid")
	}
	if InCircle(5, 5, 5, 5, 0) {
		t.Fatal("r=0 should cover nothing")
	}

	// r=2 -> (2-1)²=1，覆盖上下左右（距离平方 1），不覆盖对角（距离平方 2）
	if !InCircle(6, 5, 5, 5, 2) {
		t.Fatal("r=2 should cover orthogonal neighbour")
	}
	if InCircle(6, 6, 5, 5, 2) {
		t.Fatal("r=2 should NOT cover diagonal (LDistance2=2 > 1)")
	}

	// r=3 -> (3-1)²=4，覆盖对角（距离平方 2）和距离 2 的正交（4）
	if !InCircle(6, 6, 5, 5, 3) {
		t.Fatalf("r=3 should cover diagonal, LDistance2=2 <= 4")
	}
	if !InCircle(7, 5, 5, 5, 3) {
		t.Fatalf("r=3 should cover distance-2 orthogonal, LDistance2=4 <= 4")
	}
	if InCircle(7, 6, 5, 5, 3) {
		t.Fatalf("r=3 should NOT cover (7,6), LDistance2=5 > 4")
	}
}

func TestDoCircleCountMatchesTheory(t *testing.T) {
	md, err := NewMapData(64, 64)
	if err != nil {
		t.Fatalf("NewMapData: %v", err)
	}
	// r=10 时理论格数 = (r-1)² 圆内的整数点数
	// 用独立计算校验，而不是直接写死一个数
	want := 0
	for dx := -9; dx <= 9; dx++ {
		for dy := -9; dy <= 9; dy++ {
			if dx*dx+dy*dy <= 81 {
				want++
			}
		}
	}
	got := 0
	DoCircle(md, 32, 32, 10, func(x, y int) bool {
		got++
		return false
	})
	if got != want {
		t.Fatalf("DoCircle r=10 count = %d, want %d", got, want)
	}
}

func TestDoCircleClipsToMapBounds(t *testing.T) {
	md, err := NewMapData(16, 16)
	if err != nil {
		t.Fatalf("NewMapData: %v", err)
	}
	// 圆心贴左上角，必须裁掉越界格，且不 panic
	count := 0
	DoCircle(md, 0, 0, 6, func(x, y int) bool {
		if x < 0 || y < 0 || x >= 16 || y >= 16 {
			t.Fatalf("DoCircle yielded out-of-bounds grid (%d,%d)", x, y)
		}
		count++
		return false
	})
	if count == 0 {
		t.Fatal("DoCircle at corner should still yield the corner grid")
	}
	if count > 36 {
		t.Fatalf("corner circle count = %d, should be clipped well below full circle", count)
	}
}

func TestDoCircleEarlyExit(t *testing.T) {
	md, err := NewMapData(64, 64)
	if err != nil {
		t.Fatalf("NewMapData: %v", err)
	}
	count := 0
	stopped := DoCircle(md, 32, 32, 10, func(x, y int) bool {
		count++
		return true // 第一次就要求停止
	})
	if !stopped {
		t.Fatal("DoCircle should report early stop")
	}
	if count != 1 {
		t.Fatalf("early exit should visit exactly 1 grid, got %d", count)
	}
}

// ---------- 观察者列表维护 ----------

func newAOIMap(t *testing.T) *MapState {
	t.Helper()
	md, err := NewMapData(64, 64)
	if err != nil {
		t.Fatalf("NewMapData: %v", err)
	}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if err := md.SetLand(x, y, Land{CanWalk: true}); err != nil {
				t.Fatalf("SetLand: %v", err)
			}
		}
	}
	st, err := NewMapState(1, md)
	if err != nil {
		t.Fatalf("NewMapState: %v", err)
	}
	return st
}

func observerUIDs(m *MapState, x, y int) []uint64 {
	return append([]uint64(nil), m.VisibleObservers(x, y)...)
}

func TestUpdateGridObserverAddRemove(t *testing.T) {
	st := newAOIMap(t)

	// 空白格加入观察者：无实体可通知，事件为空
	events := st.UpdateGridObserver(42, 10, 10, true)
	if len(events) != 0 {
		t.Fatalf("empty grid should produce no appear events, got %d", len(events))
	}
	if got := observerUIDs(st, 10, 10); len(got) != 1 || got[0] != 42 {
		t.Fatalf("observers = %v, want [42]", got)
	}

	// 重复加入不应重复通知
	if events := st.UpdateGridObserver(42, 10, 10, true); len(events) != 0 {
		t.Fatalf("duplicate add should produce no events, got %d", len(events))
	}
	if got := observerUIDs(st, 10, 10); len(got) != 1 {
		t.Fatalf("observers after duplicate add = %v, want 1 entry", got)
	}

	// 移除
	events = st.UpdateGridObserver(42, 10, 10, false)
	if len(events) != 1 || events[0].Kind != ObserverRemove || events[0].Observer != 42 {
		t.Fatalf("remove events = %+v, want one ObserverRemove", events)
	}
	if got := observerUIDs(st, 10, 10); len(got) != 0 {
		t.Fatalf("observers after remove = %v, want empty", got)
	}

	// 移除不存在的观察者：无事件
	if events := st.UpdateGridObserver(42, 10, 10, false); len(events) != 0 {
		t.Fatalf("remove non-observer should produce no events, got %d", len(events))
	}
}

func TestUpdateGridObserverNotifiesOccupants(t *testing.T) {
	st := newAOIMap(t)

	// 先放一个实体在 (10,10)
	if _, err := st.AddEntity(Entity{UID: 7, Type: EntityMonster, X: 10, Y: 10, HP: 10, MaxHP: 10}); err != nil {
		t.Fatalf("AddEntity: %v", err)
	}

	// 观察者 42 开始观察 (10,10)：应看到实体 7
	events := st.UpdateGridObserver(42, 10, 10, true)
	if len(events) != 1 {
		t.Fatalf("events = %+v, want 1 COAppear", events)
	}
	ev := events[0]
	if ev.Kind != COAppear || ev.Observer != 42 || ev.Target != 7 || ev.X != 10 || ev.Y != 10 {
		t.Fatalf("event = %+v, want COAppear observer=42 target=7", ev)
	}

	// 实体自己成为自己的观察者时不该通知自己
	if events := st.UpdateGridObserver(7, 10, 10, true); len(events) != 0 {
		t.Fatalf("self-observe should not notify self, got %+v", events)
	}
}

func TestUpdateGridObserverCircleCount(t *testing.T) {
	st := newAOIMap(t)
	st.UpdateGridObserverCircle(1, 8, 32, 32, true)

	// 统计被 1 观察的格数，应等于圆内格数
	count := 0
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			for _, uid := range st.VisibleObservers(x, y) {
				if uid == 1 {
					count++
				}
			}
		}
	}
	// r=8 -> (8-1)²=49；统计圆内格数
	want := 0
	for dx := -7; dx <= 7; dx++ {
		for dy := -7; dy <= 7; dy++ {
			if dx*dx+dy*dy <= 49 {
				want++
			}
		}
	}
	if count != want {
		t.Fatalf("observed grids = %d, want %d", count, want)
	}

	// 0 视野不观察任何格
	before := count
	st.UpdateGridObserverCircle(2, 0, 32, 32, true)
	after := 0
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			for _, uid := range st.VisibleObservers(x, y) {
				if uid == 2 {
					after++
				}
			}
		}
	}
	if after != 0 {
		t.Fatalf("range 0 should observe nothing, got %d", after)
	}
	_ = before
}

// ---------- 移动时的增量更新 ----------

func TestMoveGridObserverCircleOnlyChangesDifference(t *testing.T) {
	st := newAOIMap(t)
	const r = 6
	st.UpdateGridObserverCircle(1, r, 20, 20, true)

	// 向右移动 5 格
	events := st.MoveGridObserverCircle(1, r, 20, 20, 25, 20)
	if len(events) == 0 {
		t.Fatal("moving should produce observer add/remove events")
	}

	// 验证结果等价于「从零在建新圆」：两边的观察集合必须一致
	st2 := newAOIMap(t)
	st2.UpdateGridObserverCircle(1, r, 25, 20, true)

	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			got := containsUID(st.VisibleObservers(x, y), 1)
			want := containsUID(st2.VisibleObservers(x, y), 1)
			if got != want {
				t.Fatalf("observers mismatch at (%d,%d): incremental=%v fresh=%v", x, y, got, want)
			}
		}
	}
}

func TestMoveGridObserverCircleNoOpOnSameGrid(t *testing.T) {
	st := newAOIMap(t)
	st.UpdateGridObserverCircle(1, 5, 10, 10, true)
	if events := st.MoveGridObserverCircle(1, 5, 10, 10, 10, 10); len(events) != 0 {
		t.Fatalf("same grid move should produce no events, got %d", len(events))
	}
}

func TestResizeGridObserverCircleGrowAndShrink(t *testing.T) {
	st := newAOIMap(t)
	const x, y = 30, 30

	// 从 4 扩到 7
	st.ResizeGridObserverCircle(1, x, y, 0, 4)
	st.ResizeGridObserverCircle(1, x, y, 4, 7)

	countObserved := func(uid uint64) int {
		n := 0
		for yy := 0; yy < 64; yy++ {
			for xx := 0; xx < 64; xx++ {
				if containsUID(st.VisibleObservers(xx, yy), uid) {
					n++
				}
			}
		}
		return n
	}

	wantR7 := 0
	for dx := -6; dx <= 6; dx++ {
		for dy := -6; dy <= 6; dy++ {
			if dx*dx+dy*dy <= 36 {
				wantR7++
			}
		}
	}
	if got := countObserved(1); got != wantR7 {
		t.Fatalf("after grow to 7: observed %d grids, want %d", got, wantR7)
	}

	// 缩到 3
	st.ResizeGridObserverCircle(1, x, y, 7, 3)
	wantR3 := 0
	for dx := -2; dx <= 2; dx++ {
		for dy := -2; dy <= 2; dy++ {
			if dx*dx+dy*dy <= 4 {
				wantR3++
			}
		}
	}
	if got := countObserved(1); got != wantR3 {
		t.Fatalf("after shrink to 3: observed %d grids, want %d", got, wantR3)
	}

	// 缩到 0：全部取消
	st.ResizeGridObserverCircle(1, x, y, 3, 0)
	if got := countObserved(1); got != 0 {
		t.Fatalf("after shrink to 0: observed %d grids, want 0", got)
	}
}

func TestSetViewRangeClamps(t *testing.T) {
	st := newAOIMap(t)
	if _, err := st.AddEntity(Entity{UID: 5, Type: EntityPlayer, X: 20, Y: 20, HP: 10, MaxHP: 10}); err != nil {
		t.Fatalf("AddEntity: %v", err)
	}

	st.SetViewRange(5, 999)
	e, _ := st.Entity(5)
	if e.ViewRange != MaxViewRange {
		t.Fatalf("view range should clamp to %d, got %d", MaxViewRange, e.ViewRange)
	}

	st.SetViewRange(5, -10)
	e, _ = st.Entity(5)
	if e.ViewRange != 0 {
		t.Fatalf("negative view range should clamp to 0, got %d", e.ViewRange)
	}
}

// ---------- 实体增删的 AOI 联动 ----------

func TestAddEntityCreatesObserverCircleAndNotifies(t *testing.T) {
	st := newAOIMap(t)

	// 先放一个已经有视野的玩家 A
	A := Entity{UID: 100, Type: EntityPlayer, X: 20, Y: 20, HP: 10, MaxHP: 10, ViewRange: 6}
	if _, err := st.AddEntity(A); err != nil {
		t.Fatalf("AddEntity A: %v", err)
	}
	if got := len(st.VisibleObservers(20, 20)); got != 1 {
		t.Fatalf("A should observe its own grid, observers = %d", got)
	}

	// B 出现在 A 的视野内
	B := Entity{UID: 200, Type: EntityPlayer, X: 23, Y: 20, HP: 10, MaxHP: 10, ViewRange: 6}
	events, err := st.AddEntity(B)
	if err != nil {
		t.Fatalf("AddEntity B: %v", err)
	}

	// B 应该观察到 A（在 B 的圆内），A 应该收到 B 出现的通知
	var bSawA, aSawB bool
	for _, ev := range events {
		if ev.Kind == COAppear && ev.Observer == 200 && ev.Target == 100 {
			bSawA = true
		}
		if ev.Kind == COAppear && ev.Observer == 100 && ev.Target == 200 {
			aSawB = true
		}
	}
	if !bSawA {
		t.Fatalf("B should be told about A; events = %+v", events)
	}
	if !aSawB {
		t.Fatalf("A should be told about B; events = %+v", events)
	}
}

func TestRemoveEntityTearsDownObserverCircle(t *testing.T) {
	st := newAOIMap(t)

	A := Entity{UID: 100, Type: EntityPlayer, X: 20, Y: 20, HP: 10, MaxHP: 10, ViewRange: 5}
	if _, err := st.AddEntity(A); err != nil {
		t.Fatalf("AddEntity A: %v", err)
	}

	_, ok, events := st.RemoveEntity(100)
	if !ok {
		t.Fatal("RemoveEntity should report found")
	}
	if len(events) == 0 {
		t.Fatal("removing an observing entity should emit observer-remove events")
	}

	// 观察圈必须完全拆除
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if containsUID(st.VisibleObservers(x, y), 100) {
				t.Fatalf("entity 100 still observes (%d,%d) after removal", x, y)
			}
		}
	}
	if _, ok := st.Entity(100); ok {
		t.Fatal("entity should be gone")
	}

	// 移除不存在的实体
	if _, ok, _ := st.RemoveEntity(999); ok {
		t.Fatal("removing missing entity should report not found")
	}
}

func TestMoveEntityUpdatesObservers(t *testing.T) {
	st := newAOIMap(t)

	// 观察者 100 在 (10,10)，视野 5
	if _, err := st.AddEntity(Entity{UID: 100, Type: EntityPlayer, X: 10, Y: 10, HP: 10, MaxHP: 10, ViewRange: 5}); err != nil {
		t.Fatalf("AddEntity observer: %v", err)
	}
	// 目标 200 在 (12,10)
	if _, err := st.AddEntity(Entity{UID: 200, Type: EntityMonster, X: 12, Y: 10, HP: 10, MaxHP: 10}); err != nil {
		t.Fatalf("AddEntity target: %v", err)
	}
	if !containsUID(st.VisibleObservers(12, 10), 100) {
		t.Fatal("observer 100 should see (12,10) before move")
	}

	// 目标走远到 (18,10)，超出视野。距离 6 必须分段走
	// （HopValid 只允许 |delta| = maxStep 或 maxStep 的对角，maxStep=3 时一次最多 3 格）
	var events []ObserverEvent
	var from Point
	var moveErr error
	for _, step := range []Point{{15, 10}, {18, 10}} {
		var stepEvents []ObserverEvent
		from, moveErr, stepEvents = st.MoveEntity(200, step.X, step.Y, 3)
		if moveErr != nil {
			t.Fatalf("MoveEntity to %+v: %v", step, moveErr)
		}
		// 累积：CODisappear 是在「离开可见格」的那一步产生的，
		// 只保留最后一次会丢掉它
		events = append(events, stepEvents...)
	}
	if from != (Point{X: 15, Y: 10}) {
		t.Fatalf("from = %+v, want (15,10)", from)
	}
	if containsUID(st.VisibleObservers(18, 10), 100) {
		t.Fatal("observer 100 (range 5, radius 4) should NOT observe (18,10), distance is 8")
	}

	// 关键：观察者 100 要收到 200 在旧格消失的通知
	var gotDisappear bool
	for _, ev := range events {
		if ev.Kind == CODisappear && ev.Observer == 100 && ev.Target == 200 {
			gotDisappear = true
		}
	}
	if !gotDisappear {
		t.Fatalf("observer should get CODisappear for the moved entity; events = %+v", events)
	}
}

func TestMoveEntityRejectsInvalid(t *testing.T) {
	st := newAOIMap(t)
	if _, err := st.AddEntity(Entity{UID: 1, Type: EntityPlayer, X: 10, Y: 10, HP: 10, MaxHP: 10}); err != nil {
		t.Fatalf("AddEntity: %v", err)
	}
	// 越界
	if _, err, _ := st.MoveEntity(1, 999, 999, 3); err != ErrInvalidCoordinate {
		t.Fatalf("out-of-bounds move err = %v, want ErrInvalidCoordinate", err)
	}
	// 跳跃过大
	if _, err, _ := st.MoveEntity(1, 20, 10, 1); err != ErrInvalidMove {
		t.Fatalf("long hop err = %v, want ErrInvalidMove", err)
	}
	// 目标被占
	if _, err := st.AddEntity(Entity{UID: 2, Type: EntityPlayer, X: 11, Y: 10, HP: 10, MaxHP: 10}); err != nil {
		t.Fatalf("AddEntity 2: %v", err)
	}
	if _, err, _ := st.MoveEntity(1, 11, 10, 1); err != ErrGridOccupied {
		t.Fatalf("occupied move err = %v, want ErrGridOccupied", err)
	}
}

// ---------- 动作广播 ----------

func TestForwardActionDedupesAndSkipsSelf(t *testing.T) {
	st := newAOIMap(t)

	// 三个观察者分散在 (10,10) 的相邻格
	// （同一格不能放多个实体：AddEntity 会返回 ErrGridOccupied）
	positions := []Point{{9, 10}, {10, 9}, {10, 11}}
	for i, uid := range []uint64{100, 101, 102} {
		p := positions[i]
		if _, err := st.AddEntity(Entity{UID: uid, Type: EntityPlayer, X: p.X, Y: p.Y, HP: 10, MaxHP: 10, ViewRange: 6}); err != nil {
			t.Fatalf("AddEntity %d: %v", uid, err)
		}
	}

	// 发起者 200 从 (9,10) 走到 (11,10)，源/目标格的观察者并集都含这三人
	got := st.ForwardAction(200, 9, 10, 11, 10)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if len(got) != 3 {
		t.Fatalf("forward targets = %v, want 3 distinct observers", got)
	}
	for _, uid := range got {
		if uid == 200 {
			t.Fatal("forward should not include the actor itself")
		}
	}
}

func TestQueryObservedOnlyCountsPlayers(t *testing.T) {
	st := newAOIMap(t)

	// 只有怪物在观察时不算「有人在看」
	if _, err := st.AddEntity(Entity{UID: 300, Type: EntityMonster, X: 15, Y: 15, HP: 10, MaxHP: 10, ViewRange: 5}); err != nil {
		t.Fatalf("AddEntity monster: %v", err)
	}
	if st.QueryObserved(15, 15) {
		t.Fatal("monster observer should not count as observed-by-player")
	}

	// 加入玩家后应算
	if _, err := st.AddEntity(Entity{UID: 301, Type: EntityPlayer, X: 15, Y: 15, HP: 10, MaxHP: 10, ViewRange: 5}); err == nil {
		t.Fatal("same grid should reject second occupant")
	}
	if _, err := st.AddEntity(Entity{UID: 301, Type: EntityPlayer, X: 16, Y: 15, HP: 10, MaxHP: 10, ViewRange: 5}); err != nil {
		t.Fatalf("AddEntity player: %v", err)
	}
	if !st.QueryObserved(15, 15) {
		t.Fatal("player observer should make the grid observed")
	}
}

func containsUID(list []uint64, uid uint64) bool {
	for _, v := range list {
		if v == uid {
			return true
		}
	}
	return false
}
