package world

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
)

// ==================== 地图 .bin 加载（mir2xmapdata.hpp）====================
//
// 格式（与客户端 scripts/data/map_loader.gd 使用同一套验证结论）：
//
//	文件 = [zstd 帧][末尾 1 字节 flag]   flag=1 表示 CF_ZSTD
//	解压后 = [endian:u8][w:u64][h:u64][count:u64][count × BLOCK]
//	BLOCK  = TILE(2×u32) + CELL[4](LAND 3×u8 + LIGHT 4×u8 + OBJECT[2] 各 7×u32)
//
// 三个必须记住的点：
//  1. 压缩标记在**文件末尾**（cerealf::serialize 追加），不在开头。
//  2. 解压后第 0 字节是 cereal PortableBinary 的**端序标记**（1=小端），不是数据。
//  3. 头文件里 TILE/OBJECT/LAND 是位域，但 serialize() 把**每个位域拆成独立字段**
//     落盘，所以 TILE=2×u32、OBJECT=7×u32（不是 1 个打包 u32）。
//     按位域紧凑解读会全错（客户端那边曾把 CELL 算成 58，实际 63）。
const (
	mapBlockSize = 260 // TILE(8) + CELL[4](63×4)
	mapTileSize  = 8
	mapCellSize  = 63
	mapObjSize   = 28

	// 地图压缩标记（cerealf CF_ZSTD）
	cfZstd = 1
)

// 物件深度（mir2xmapdata.hpp ObjDepthType）
const (
	ObjGround = iota
	ObjOverGround0
	ObjOverGround1
	ObjSky
)

// MapTile 是小地图块贴图（BLOCK 级别，覆盖 2×2 格）。
type MapTile struct {
	Valid bool
	TexID uint32
}

// MapObject 是一个地格上的静态物件。
type MapObject struct {
	Valid      bool
	TexID      uint32
	Depth      uint8
	Animated   bool
	Alpha      uint8
	TickType   uint8
	FrameCount uint8
}

// MapLight 是地格光照位域。
type MapLight struct {
	Valid  bool
	Radius uint8
	Alpha  uint8
	Color  uint8
}

// LoadMapFile 读取并解析一张地图 .bin。
func LoadMapFile(path string) (*MapData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read map %s: %w", filepath.Base(path), err)
	}
	return LoadMapBytes(raw)
}

// LoadMapBytes 解析地图二进制内容。
func LoadMapBytes(raw []byte) (*MapData, error) {
	if len(raw) < 2 {
		return nil, errors.New("map data too short")
	}

	flag := raw[len(raw)-1]
	body := raw[:len(raw)-1]

	var payload []byte
	switch flag {
	case 0:
		payload = body
	case cfZstd:
		dec, err := zstd.NewReader(nil)
		if err != nil {
			return nil, fmt.Errorf("create zstd decoder: %w", err)
		}
		defer dec.Close()
		payload, err = dec.DecodeAll(body, nil)
		if err != nil {
			return nil, fmt.Errorf("zstd decode map: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown map compression flag %d", flag)
	}

	return parseMapPayload(payload)
}

// parseMapPayload 解析解压后的主体。
func parseMapPayload(p []byte) (*MapData, error) {
	// 第 0 字节是端序标记
	if len(p) < 1+8*3 {
		return nil, errors.New("map payload too short")
	}
	if p[0] != 1 {
		return nil, fmt.Errorf("unsupported map endian flag %d (only little-endian)", p[0])
	}
	off := 1
	w := int(uint64(binary.LittleEndian.Uint64(p[off:])))
	off += 8
	h := int(uint64(binary.LittleEndian.Uint64(p[off:])))
	off += 8
	count := int(uint64(binary.LittleEndian.Uint64(p[off:])))
	off += 8

	if w <= 0 || h <= 0 || w%2 != 0 || h%2 != 0 {
		return nil, fmt.Errorf("invalid map size %dx%d (must be even)", w, h)
	}
	expectBlocks := w * h / 4
	if count != expectBlocks {
		return nil, fmt.Errorf("map block count mismatch: header=%d expected=%d", count, expectBlocks)
	}
	if len(p)-off < count*mapBlockSize {
		return nil, fmt.Errorf("map payload truncated: need %d have %d", count*mapBlockSize, len(p)-off)
	}

	md := &MapData{
		Width:  w,
		Height: h,
		Cells:  make([]Land, w*h),
		Lights: make([]MapLight, w*h),
		Objs:   make([]MapObject, w*h*2),
		Tiles:  make([]MapTile, count),
	}

	for bi := 0; bi < count; bi++ {
		base := off + bi*mapBlockSize

		// TILE: valid(1bit) 落盘为 u32，texID(24bit) 落盘为 u32
		tileValid := binary.LittleEndian.Uint32(p[base:]) != 0
		tileTex := binary.LittleEndian.Uint32(p[base+4:]) & 0x00FFFFFF
		md.Tiles[bi] = MapTile{Valid: tileValid, TexID: tileTex}

		bx := (bi % (w / 2)) * 2
		by := (bi / (w / 2)) * 2

		for ci := 0; ci < 4; ci++ {
			cb := base + mapTileSize + ci*mapCellSize
			cx := bx + ci%2
			cy := by + ci/2
			cellIndex := cx + cy*w

			// LAND: type(6) canFly(1) canWalk(1)，各落盘为 1 字节
			landType := p[cb] & 0x3F
			canFly := p[cb+1]&0x01 != 0
			canWalk := p[cb+2]&0x01 != 0
			md.Cells[cellIndex] = Land{Type: landType, CanFly: canFly, CanWalk: canWalk}

			// LIGHT: valid(1) radius(2) alpha(2) color(3)，各落盘为 1 字节
			md.Lights[cellIndex] = MapLight{
				Valid:  p[cb+3]&0x01 != 0,
				Radius: p[cb+4] & 0x03,
				Alpha:  p[cb+5] & 0x03,
				Color:  p[cb+6] & 0x07,
			}

			// OBJECT[2]: 各 7 个 u32
			for oi := 0; oi < 2; oi++ {
				ob := cb + 7 + oi*mapObjSize
				obj := MapObject{
					Valid:      binary.LittleEndian.Uint32(p[ob:]) != 0,
					TexID:      binary.LittleEndian.Uint32(p[ob+4:]) & 0x00FFFFFF,
					Depth:      uint8(binary.LittleEndian.Uint32(p[ob+8:]) & 0x03),
					Animated:   binary.LittleEndian.Uint32(p[ob+12:]) != 0,
					Alpha:      uint8(binary.LittleEndian.Uint32(p[ob+16:]) & 0x01),
					TickType:   uint8(binary.LittleEndian.Uint32(p[ob+20:]) & 0x07),
					FrameCount: uint8(binary.LittleEndian.Uint32(p[ob+24:]) & 0x0F),
				}
				// 没有贴图的物件视为无效（与客户端一致）
				if obj.TexID == 0 {
					obj.Valid = false
				}
				md.Objs[cellIndex*2+oi] = obj
			}
		}
	}

	return md, nil
}
