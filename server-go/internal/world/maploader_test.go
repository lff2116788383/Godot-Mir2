package world

import (
	"os"
	"path/filepath"
	"testing"
)

// mapAssetsRoot 返回地图素材目录，可由环境变量覆盖。
func mapAssetsRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("MIR2X_ASSETS"); root != "" {
		return root
	}
	return filepath.Join("..", "..", "..", "mir2x_assets-main")
}

func TestLoadMapIfAvailable(t *testing.T) {
	root := mapAssetsRoot(t)
	path := filepath.Join(root, "map", "0.bin")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("map asset not present: %v", err)
	}

	md, err := LoadMapFile(path)
	if err != nil {
		t.Fatalf("LoadMapFile: %v", err)
	}

	// 客户端已字节级验证过的基准值（docs/map-format.md）
	if md.Width != 800 || md.Height != 800 {
		t.Fatalf("size = %dx%d, want 800x800", md.Width, md.Height)
	}
	blocks := (md.Width / 2) * (md.Height / 2)
	if len(md.Tiles) != blocks {
		t.Fatalf("tiles = %d, want %d", len(md.Tiles), blocks)
	}
	if len(md.Cells) != md.Width*md.Height {
		t.Fatalf("cells = %d, want %d", len(md.Cells), md.Width*md.Height)
	}

	validTiles := 0
	for _, tile := range md.Tiles {
		if tile.Valid {
			validTiles++
		}
	}
	// 客户端实测 159,997（800x800/4 = 160,000 块里有 3 块无瓦片，
	// 这是地图本身的内容，不是解析错误）
	if validTiles != 159997 {
		t.Fatalf("valid tiles = %d, want 159997", validTiles)
	}

	// 可走格约占一半（客户端实测 54.8%）
	walkable := md.WalkableCount()
	total := md.Width * md.Height
	ratio := float64(walkable) / float64(total)
	if ratio < 0.40 || ratio > 0.70 {
		t.Fatalf("walkable ratio = %.3f, want 0.40..0.70", ratio)
	}

	// 物件总量与深度分布（客户端实测 157,264 个）
	ground := md.ObjectCount(ObjGround)
	over0 := md.ObjectCount(ObjOverGround0)
	over1 := md.ObjectCount(ObjOverGround1)
	t.Logf("map 0: %dx%d walkable=%d (%.1f%%) objects ground=%d over0=%d over1=%d lights=%d",
		md.Width, md.Height, walkable, ratio*100, ground, over0, over1, md.LightCount())

	if ground < 100000 {
		t.Fatalf("ground objects = %d, want > 100000", ground)
	}
	if over0 < 50000 {
		t.Fatalf("overground0 objects = %d, want > 50000", over0)
	}
	if md.LightCount() < 100 {
		t.Fatalf("light cells = %d, want > 100", md.LightCount())
	}

	// 物件贴图 id 应为有效 24 位且高 8 位为 0
	for i, obj := range md.Objs {
		if obj.Valid && obj.TexID == 0 {
			t.Fatalf("object %d valid but texID == 0", i)
		}
		if obj.TexID > 0x00FFFFFF {
			t.Fatalf("object %d texID = %#x exceeds 24 bits", i, obj.TexID)
		}
		break
	}
}

func TestLoadMapRejectsBadInput(t *testing.T) {
	if _, err := LoadMapBytes(nil); err == nil {
		t.Fatal("nil input should fail")
	}
	if _, err := LoadMapBytes([]byte{0x01}); err == nil {
		t.Fatal("single byte should fail")
	}
	// 未知压缩标记
	if _, err := LoadMapBytes([]byte{0x00, 0x00, 0x99}); err == nil {
		t.Fatal("unknown compression flag should fail")
	}
}

func TestLoadMapEndianFlag(t *testing.T) {
	// 构造一个最小的合法负载：w=2 h=2 count=1，但端序标记为 0（非法）
	payload := make([]byte, 1+8*3+mapBlockSize)
	payload[0] = 0 // 端序标记非法
	// w=2, h=2, count=1 小端
	payload[1] = 2
	payload[9] = 2
	payload[17] = 1
	raw := append(payload, 0) // flag=0 不压缩
	if _, err := LoadMapBytes(raw); err == nil {
		t.Fatal("endian flag 0 should be rejected")
	}
}

func TestLoadMapMinimal(t *testing.T) {
	// 构造一个 2x2 的地图，唯一一个 block，cell(0,0) 可走，其余不可走
	payload := make([]byte, 1+8*3+mapBlockSize)
	payload[0] = 1
	// w=2 h=2 count=1
	payload[1] = 2
	payload[9] = 2
	payload[17] = 1
	// TILE 有效，texID = 0x1234
	payload[25] = 1    // tile.valid = 1
	payload[29] = 0x34 // tile.texID low
	payload[30] = 0x12 // tile.texID high
	// cell 0 的 LAND：canWalk = 1（第 3 个字节）
	cellBase := 25 + mapTileSize
	payload[cellBase+2] = 1 // canWalk
	// cell 0 的第 0 个 OBJECT 有效
	objBase := cellBase + 7
	payload[objBase] = 1 // obj.valid
	payload[objBase+4] = 0x78
	payload[objBase+5] = 0x56
	payload[objBase+8] = 1 // depth = 1

	raw := append(payload, 0)
	md, err := LoadMapBytes(raw)
	if err != nil {
		t.Fatalf("LoadMapBytes: %v", err)
	}
	if md.Width != 2 || md.Height != 2 {
		t.Fatalf("size = %dx%d, want 2x2", md.Width, md.Height)
	}
	if !md.CanWalk(0, 0) {
		t.Fatal("cell(0,0) should be walkable")
	}
	if md.CanWalk(1, 0) {
		t.Fatal("cell(1,0) should not be walkable")
	}
	tile, ok := md.TileAt(0, 0)
	if !ok || tile.TexID != 0x1234 {
		t.Fatalf("tile = %+v, want texID 0x1234", tile)
	}
	obj, ok := md.ObjectAt(0, 0, 0)
	if !ok || !obj.Valid || obj.TexID != 0x5678 || obj.Depth != 1 {
		t.Fatalf("object = %+v, want texID 0x5678 depth 1", obj)
	}
	// 同块的其他 3 格不该有物件
	if o, _ := md.ObjectAt(1, 0, 0); o.Valid {
		t.Fatal("cell(1,0) should have no valid object")
	}
}
