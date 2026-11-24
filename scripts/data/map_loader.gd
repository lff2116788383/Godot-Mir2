extends RefCounted
class_name MapLoader
## mir2x 地图 .bin 解析器
##
## 格式（已对 8 张地图字节级验证，见 docs/map-format.md）：
##   文件 = [zstd 帧][末字节 flag]  flag=1 表示 CF_ZSTD
##   解压后 = [endian:1B][w:u64][h:u64][count:u64][count × BLOCK]
##   BLOCK = TILE(8B) + CELL[4](58B)
##
## 两个必须记住的坑：
## 1. 压缩标记在**文件末尾**（cerealf::serialize 追加），不在开头。
## 2. 解压后第 0 字节是 cereal PortableBinary 的**端序标记**（1=小端），
##    不是数据。所有 cerealf 消息都有这 1 字节。
##
## 关键：源码头文件里 TILE/CELL/OBJECT 是位域，但 serialize() 把
## 每个位域拆成独立 u32 落盘，所以 TILE=2×u32，OBJECT=7×u32。

const BLOCK_SIZE := 260
const TILE_SIZE := 8
## CELL = LAND(3×u8) + LIGHT(4×u8) + 2 × OBJECT(7×u32) = 3 + 4 + 56 = 63
const CELL_SIZE := 63
const OBJ_SIZE := 28

## 地图瓦片像素尺寸（sysconst.hpp:55-56）
const GRID_X := 48
const GRID_Y := 32


class MapData:
	var w: int = 0
	var h: int = 0
	## 平铺的 texID 数组，-1 表示无瓦片
	var tiles: PackedInt32Array
	## 每个格的可行走标记，1 字节/格
	var walkable: PackedByteArray

	func block_index(x: int, y: int) -> int:
		return (x / 2) + (y / 2) * (w / 2)

	func tile_tex_id(x: int, y: int) -> int:
		if x < 0 or y < 0 or x >= w or y >= h:
			return -1
		return tiles[block_index(x, y)]


static func load_file(path: String) -> MapData:
	if not FileAccess.file_exists(path):
		push_error("地图文件不存在: " + path)
		return null
	var raw := FileAccess.get_file_as_bytes(path)
	if raw.is_empty():
		push_error("地图文件为空: " + path)
		return null
	return load_bytes(raw)


static func load_bytes(raw: PackedByteArray) -> MapData:
	if raw.size() < 2:
		push_error("地图数据过短")
		return null

	# 压缩标记在文件末尾，由 cerealf::serialize 追加
	var flag := raw[raw.size() - 1]
	var body := raw.slice(0, raw.size() - 1)
	var payload: PackedByteArray

	match flag:
		0:
			payload = body
		1:
			# zstd；解压缓冲区要给足，800x800 地图解压后 41.6 MB
			payload = body.decompress(raw.size() * 256 + 67108864, FileAccess.COMPRESSION_ZSTD)
			if payload.is_empty():
				push_error("zstd 解压失败")
				return null
		_:
			push_error("暂不支持的压缩标记: %d" % flag)
			return null

	# 第 0 字节是 cereal PortableBinary 端序标记（1 = 小端），跳过
	if payload.size() < 25:
		push_error("地图头不完整")
		return null
	var endian := payload[0]
	if endian != 1:
		push_error("非小端序列化数据，暂不支持: %d" % endian)
		return null

	var md := MapData.new()
	md.w = payload.decode_u64(1)
	md.h = payload.decode_u64(9)
	var count := payload.decode_u64(17)

	if md.w <= 0 or md.h <= 0 or md.w % 2 != 0 or md.h % 2 != 0:
		push_error("地图尺寸非法: %dx%d" % [md.w, md.h])
		return null

	var need := 25 + count * BLOCK_SIZE
	if payload.size() < need:
		push_error("地图数据不完整: need=%d actual=%d" % [need, payload.size()])
		return null

	var nblocks := md.w * md.h / 4
	if count != nblocks:
		push_error("块数量与尺寸不匹配: count=%d expect=%d" % [count, nblocks])
		return null

	md.tiles.resize(nblocks)
	md.tiles.fill(-1)
	md.walkable.resize(md.w * md.h)
	md.walkable.fill(0)

	var off := 25
	for bi in range(nblocks):
		# TILE
		var tile_valid := payload.decode_u32(off)
		var tile_tex := payload.decode_u32(off + 4) & 0x00FFFFFF
		if tile_valid != 0:
			md.tiles[bi] = tile_tex
		off += TILE_SIZE

		# 4 个 CELL：索引 = (y%2)*2 + (x%2)
		var bx := (bi % (md.w / 2)) * 2
		var by := (bi / (md.w / 2)) * 2
		for ci in range(4):
			var cx := bx + (ci % 2)
			var cy := by + (ci / 2)
			var can_walk := payload[off + 2]
			md.walkable[cy * md.w + cx] = can_walk
			off += CELL_SIZE
	return md


static func is_walkable(md: MapData, x: int, y: int) -> bool:
	if x < 0 or y < 0 or x >= md.w or y >= md.h:
		return false
	return md.walkable[y * md.w + x] != 0
