package world

import (
	"testing"
	"time"
)

// TestAIWith1000MonstersPerf 测 1000 只怪 + 玩家的 AI 单 tick 耗时。
func TestAIWith1000MonstersPerf(t *testing.T) {
	const w, h = 200, 200
	data := &MapData{Width: w, Height: h}
	data.Cells = make([]Land, w*h)
	for i := range data.Cells {
		data.Cells[i] = Land{CanWalk: true}
	}
	// 撒一些障碍，逼寻路真的工作
	for i := 0; i < 2000; i++ {
		x := (i * 37) % w
		y := (i * 53) % h
		data.Cells[x+y*w] = Land{CanWalk: false}
	}

	st, _ := NewMapState(1, data)
	rec := MonsterRecord{
		Name: "鸡", HP: 50, View: 8, WalkWait: 100, AttackWait: 1000,
		BehaveMode: BehaveDefault, DC: AbilityBounds{Min: 1, Max: 2},
	}

	// 铺 1000 只怪（网格铺开，避开坐标碰撞）
	uid := uint64(100)
	for i := 0; i < 1000; i++ {
		x := 2 + (i%40)*4
		y := 2 + (i/40)*4
		if x >= w || y >= h || !st.Data.CanWalk(x, y) {
			continue
		}
		e := Entity{
			UID: uid, Type: EntityMonster, Name: "鸡", X: x, Y: y,
			HP: 50, MaxHP: 50, ViewRange: 8, Direction: DirDown,
			Stats: CombatStatsOf(rec),
		}
		if _, err := st.AddEntity(e); err == nil {
			uid++
			cur := uid - 1
			st.MutateEntity(cur, func(en *Entity) error {
				r := rec
				en.MonsterRec = &r
				return nil
			})
		}
	}
	// 几个玩家
	for i := 0; i < 5; i++ {
		st.AddEntity(Entity{
			UID: uint64(9000 + i), Type: EntityPlayer, Name: "p",
			X: 10 + i*5, Y: 10, HP: 1000, MaxHP: 1000,
			ViewRange: 10, Direction: DirDown,
		})
	}

	count := len(st.Entities)
	t.Logf("实体总数: %d", count)

	// 跑 20 个 AI tick（每个 1 秒）
	var total time.Duration
	maxTick := time.Duration(0)
	for i := 0; i < 20; i++ {
		ms := int64(1000 * (i + 1))
		start := time.Now()
		st.Advance(ms)
		d := time.Since(start)
		total += d
		if d > maxTick {
			maxTick = d
		}
	}

	avg := total / 20
	t.Logf("20 个 tick：总耗时 %v，平均 %v，最大 %v", total, avg, maxTick)

	// ★ 只断言平均值，不查峰值。
	//
	//	峰值受测试机负载影响极大：单跑这个测试是 55ms，
	//	但和其他包的测试并行跑时会跳到 170ms（CPU 争抢）。
	//	拿峰值当 CI 门禁会随机失败。
	//
	//	平均值 25ms 对应「1000 只怪、1 秒决策一次」的实际负载：
	//	100ms 的 tick 里花 25ms 跑 AI，还剩 75ms 给网络和其他逻辑。
	if avg > 25*time.Millisecond {
		t.Fatalf("AI 单 tick 平均 %v 超过 25ms", avg)
	}
}
