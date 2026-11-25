package game

import (
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// TestPhysicalKillSettlesLoot 是「物理攻击打死怪要掉落」的回归测试。
//
// 背景：结算逻辑最初只写在 SpellHandler.settleKill 里，
// 而玩家用普通攻击打死怪走的是 ActionHandler.handleAttack，
// 那条路径只广播了 SM_NOTIFYDEAD，**没有结算掉落和经验**。
//
// 后果：战士（近战职业）打怪永远不掉东西、不得经验，
// 只有法师用火球才有奖励。客户端完全不报错，只是「打怪没意义」。
//
// 这条测试直接调 handleAttack 的公开入口，断言：
//  1. 经验有涨
//  2. 地上出现了掉落物
//  3. 掉落有归属保护（别人短时间内捡不走）
func TestPhysicalKillSettlesLoot(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "warrior", 100, 100)

	// 用「必掉一件」的合成怪，让断言稳定。
	//
	// 黑野猪虽然配了掉落表，但全是概率（金币 1/4、药 1/10），
	// 打死一只可能什么都不掉 —— 测「有没有结算」不该受概率干扰。
	uid, mx, my := f.spawnGuaranteedDropMonster(t, 101, 100)

	expBefore := p.Exp
	f.Cast.reset()

	killed := false
	for i := 0; i < 80; i++ {
		action := common.ActionNode{
			Type:      common.ActionAttack,
			Direction: common.DirectionRight,
			X:         int16(p.X), Y: int16(p.Y),
			AimX: int16(mx), AimY: int16(my),
			AimUID: uid,
		}
		if err := f.Actions.handleAttack(f.ctx, p, action); err != nil {
			t.Fatalf("attack #%d: %v", i, err)
		}
		if e, ok, _ := f.Worlds.Worlds.Entity(f.ctx, 1, uid); ok && e.Dead() {
			killed = true
			break
		}
	}
	if !killed {
		t.Fatal("怪没被打死")
	}

	// 1. 经验
	if p.Exp <= expBefore {
		t.Fatalf("物理击杀应该给经验：%d -> %d", expBefore, p.Exp)
	}
	t.Logf("经验 %d -> %d", expBefore, p.Exp)

	// 2. 地上有掉落
	snaps := f.Worlds.Worlds.CollectGroundItems(f.ctx, 1, mx, my, 2)
	if len(snaps) == 0 || len(snaps[0].ItemIDs) == 0 {
		t.Fatalf("物理击杀应该掉落物品，实际 (%d,%d) 周围一件都没有", mx, my)
	}
	itemID := snaps[0].ItemIDs[0]
	t.Logf("掉落物 itemID=%d 在 (%d,%d)", itemID, snaps[0].X, snaps[0].Y)

	// 3. 掉落有归属保护
	if f.Worlds.Worlds.CanPickup(f.ctx, 1, snaps[0].X, snaps[0].Y, itemID, 99999) {
		t.Fatal("保护期内陌生人应该捡不走")
	}
	if !f.Worlds.Worlds.CanPickup(f.ctx, 1, snaps[0].X, snaps[0].Y, itemID, p.UID) {
		t.Fatal("归属者本人应该能捡")
	}
	t.Log("归属保护生效")

	// 4. 死亡淡出包发出去了
	if n := len(f.Cast.of(p.ChannelID, protocol.SM_DEADFADEOUT)); n == 0 {
		t.Fatal("应该广播 SM_DEADFADEOUT")
	}
}

// spawnGuaranteedDropMonster 刷一只「必掉一件金创药（小）」的怪。
//
// 用意：测的是「物理击杀有没有走结算」，不是「掉落概率对不对」。
// 用必掉配置才能让断言稳定 —— 概率性掉落会让这个测试间歇性失败。
func (f *b5spellFixture) spawnGuaranteedDropMonster(t *testing.T, x, y int) (uint64, int, int) {
	t.Helper()

	// 找一个空格（玩家/NPC 可能占了目标格）
	for try := 0; try < 20; try++ {
		if f.Worlds.Worlds.CanOccupy(f.ctx, 1, 0, x+try, y) {
			x += try
			break
		}
	}

	uid, err := common.BuildMonsterUID(2001, 0, 7001)
	if err != nil {
		t.Fatalf("build monster uid: %v", err)
	}

	rec := world.MonsterRecord{
		Name: "测试怪", HP: 10, Exp: 100,
		View: 5, WalkWait: 1000, AttackWait: 1000,
		BehaveMode: world.BehaveModeNeutral, DeadFadeOut: 1,
		DC: world.AbilityBounds{Min: 0, Max: 1},
		// probRecip=1 表示必掉
		DropItems: [][]world.DropItem{{
			{Item: "金创药（小）", ProbRecip: 1, Repeat: 1, Count: 1},
		}},
	}
	entity := world.Entity{
		UID: uid, Type: world.EntityMonster, Name: rec.Name,
		X: x, Y: y, HP: 10, MaxHP: 10, ViewRange: 5,
		Direction: world.DirLeft, Stats: world.CombatStatsOf(rec),
	}
	if _, err := f.Worlds.Worlds.SpawnMonster(f.ctx, 1, entity, rec); err != nil {
		t.Fatalf("spawn monster: %v", err)
	}
	return uid, x, y
}
