package game

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// discardLogger 返回一个不输出任何东西的 logger（测试用）。
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError + 100,
	}))
}

// dropItemAt 在指定格掉落物品（测试辅助）。
//
// 走 world.World 的公开 API —— mapCommand 是非导出接口，
// game 包无法自己构造实现，所以必须有这层公开方法。
func dropItemAt(ctx context.Context, w *world.World, mapID uint32, x, y int, itemIDs []uint32) error {
	_, err := w.DropItem(ctx, mapID, x, y, itemIDs)
	return err
}

// bumpLevel 提升玩家等级并刷新世界侧属性（测试辅助）。
//
// 玩家属性由 Level 派生，改了 Level 必须重算 Stats 并写回地图实体，
// 否则 Combat 用的还是旧属性。
func bumpLevel(t *testing.T, f *b5Fixture, uid uint64, level int) {
	t.Helper()
	p, ok := f.Players.Get(uid)
	if !ok {
		t.Fatalf("player %d not found", uid)
	}
	p.Level = level
	stats := PlayerCombatStats(p.Job, level)
	if err := f.Worlds.Worlds.SetStats(f.ctx, p.MapID, uid, stats); err != nil {
		t.Fatalf("SetStats: %v", err)
	}
}
