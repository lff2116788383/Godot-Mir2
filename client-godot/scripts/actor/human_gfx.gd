extends RefCounted
class_name HumanGfx
## 人形角色贴图定位（主角、其他玩家）
##
## 【逐行翻译】client/src/pngtexoffdb.hpp:50-63 + hero.cpp:1411-1460
##
## 文件名格式（tools/herowil2png/src/main.cpp:73-88）：
##   [24-21] [20-17] [16] [15] [14-11] [10-8] [7-5] [4-1]
##   layer   shadow  gender dress  motion  dir    frame  帧
##   ↑ 每组 4 位十六进制
##
##   nEncode = layer<<24 | shadow<<23 | gender<<22 | dress<<14 | motion<<8 | dir<<5 | frame
##   实测文件名尾部固定 6 位：dxSign(1) + dySign(1) + abs(dx):%02X + abs(dy):%02X
##
## humanKey（pngtexoffdb.hpp:55）：
##   gfxID = id << 9 | motion << 3 | direction
##   key   = dir 0-2 | motion 3-8 | layer 9 | shadow 10 | gender 11 | id 12-19
##
## 各层 gfxID 计算（hero.cpp:1411-1460）：
##   weapon: ((weapon - WEAPON_BEGIN) << 9) | (gfxMotion << 3) | (dir - DIR_BEGIN)
##   dress : ((dress  - DRESS_BEGIN ) << 9) | ...
##   hair  : ((hair   - HAIR_BEGIN  ) << 9) | ...
##   helmet: ((helmet - HELMET_BEGIN) << 9) | ...

const SysConstRef11 := preload("res://scripts/core/sys_const.gd")
const MotionTypeRef2 := preload("res://scripts/action/motion_type.gd")
const ProtocolDefRef2 := preload("res://scripts/core/protocol_def.gd")

## 层级（dress 有 layer 0/1，weapon 也有 glow layer 1）
const LAYER_BODY := 0
const LAYER_OVERLAY := 1


## gfxMotionID（hero.hpp:148）
## 只对玩家动作有效，返回 motion - MOTION_BEGIN
static func gfx_motion_id(motion: int) -> int:
	if motion >= MotionTypeRef2.MOTION_BEGIN and motion < MotionTypeRef2.MOTION_END:
		return motion - MotionTypeRef2.MOTION_BEGIN
	return -1


## humanKey（pngtexoffdb.hpp:55）
## 返回纹理查找键
static func human_key(gfx_id: int, gender: bool, shadow: bool = false, layer: int = 0) -> int:
	return ((gfx_id & 0x1FF) << 0) \
		| ((layer & 0x1) << 9) \
		| ((1 if shadow else 0) << 10) \
		| ((1 if gender else 0) << 11) \
		| (((gfx_id >> 9) & 0xFF) << 12)


## gfxWeaponID（hero.cpp:1411）
static func gfx_weapon_id(weapon: int, motion: int, direction: int) -> int:
	if weapon < ProtocolDefRef2.WEAPON_BEGIN or weapon >= ProtocolDefRef2.WEAPON_END:
		return -1
	if direction < ProtocolDefRef2.DIR_BEGIN or direction >= ProtocolDefRef2.DIR_END:
		return -1
	var gm := gfx_motion_id(motion)
	if gm < 0:
		return -1
	return ((weapon - ProtocolDefRef2.WEAPON_BEGIN) << 9) + (gm << 3) + (direction - ProtocolDefRef2.DIR_BEGIN)


## gfxDressID（hero.cpp:1429）
static func gfx_dress_id(dress: int, motion: int, direction: int) -> int:
	if dress < ProtocolDefRef2.DRESS_BEGIN or dress >= ProtocolDefRef2.DRESS_END:
		return -1
	if direction < ProtocolDefRef2.DIR_BEGIN or direction >= ProtocolDefRef2.DIR_END:
		return -1
	var gm := gfx_motion_id(motion)
	if gm < 0:
		return -1
	return ((dress - ProtocolDefRef2.DRESS_BEGIN) << 9) + (gm << 3) + (direction - ProtocolDefRef2.DIR_BEGIN)


## gfxHairID（hero.cpp:1420）
## 注意：hair gfx id 从 0 开始（注释：hair gfx id start from 0）
static func gfx_hair_id(hair: int, motion: int, direction: int) -> int:
	if hair < ProtocolDefRef2.HAIR_BEGIN or hair >= ProtocolDefRef2.HAIR_END:
		return -1
	if direction < ProtocolDefRef2.DIR_BEGIN or direction >= ProtocolDefRef2.DIR_END:
		return -1
	var gm := gfx_motion_id(motion)
	if gm < 0:
		return -1
	return ((hair - ProtocolDefRef2.HAIR_BEGIN) << 9) + (gm << 3) + (direction - ProtocolDefRef2.DIR_BEGIN)


## gfxHelmetID（hero.cpp:1447）
static func gfx_helmet_id(helmet: int, motion: int, direction: int) -> int:
	if helmet < ProtocolDefRef2.HELMET_BEGIN or helmet >= ProtocolDefRef2.HELMET_END:
		return -1
	if direction < ProtocolDefRef2.DIR_BEGIN or direction >= ProtocolDefRef2.DIR_END:
		return -1
	var gm := gfx_motion_id(motion)
	if gm < 0:
		return -1
	return ((helmet - ProtocolDefRef2.HELMET_BEGIN) << 9) + (gm << 3) + (direction - ProtocolDefRef2.DIR_BEGIN)


# ==================== 文件名解码 ====================

## 一个解码后的贴图信息
class TileInfo:
	var encode: int = 0      ## 24 位编码
	var layer: int = 0       ## bit 24
	var shadow: int = 0      ## bit 23
	var gender: int = 0      ## bit 22
	var dress: int = 0       ## bit 14-21
	var motion: int = 0      ## bit 8-13
	var direction: int = 0   ## bit 5-7
	var frame: int = 0       ## bit 0-4
	## 偏移（文件名尾部 8 位）
	var dx: int = 0
	var dy: int = 0
	var dx_positive: bool = true
	var dy_positive: bool = true

	func to_str() -> String:
		return "Tile{layer=%d shadow=%d gender=%d dress=%d motion=%d dir=%d frame=%d dx=%d dy=%d}" % [
			layer, shadow, gender, dress, motion, direction, frame, dx, dy
		]


## 解码 hero/weapon/hair/helmet 的文件名
## 例：0000000000DFD3
##   前 8 位 = encode，后 6 位 = sign(dx) sign(dy) abs(dx):%04X abs(dy):%04X
static func decode_file_name(fname: String) -> TileInfo:
	var info := TileInfo.new()
	var base := fname
	if base.to_upper().ends_with(".PNG"):
		base = base.substr(0, base.length() - 4)
	if base.length() < 14:
		return info

	info.encode = base.substr(0, 8).hex_to_int()
	info.layer = (info.encode >> 24) & 0x1
	info.shadow = (info.encode >> 23) & 0x1
	info.gender = (info.encode >> 22) & 0x1
	info.dress = (info.encode >> 14) & 0xFF
	info.motion = (info.encode >> 8) & 0x3F
	info.direction = (info.encode >> 5) & 0x7
	info.frame = info.encode & 0x1F

	# 尾部固定 6 位：[dxSign:1][dySign:1][dx:2][dy:2]
	# 实测 texture/hero 全部 10816 个文件尾部均为 6 位，dx/dy 各 1 字节（0-255）
	var tail := base.substr(8)
	if tail.length() >= 6:
		info.dx_positive = tail[0] == "1"
		info.dy_positive = tail[1] == "1"
		info.dx = tail.substr(2, 2).hex_to_int()
		info.dy = tail.substr(4, 2).hex_to_int()
	return info


## 在素材目录里查某层的贴图文件
## layer_dir: "hero" / "weapon" / "hair" / "helmet"
## 返回 {path, dx, dy} 或 null
static func find_texture(assets_root: String, layer_dir: String, gfx_id: int,
		gender: bool, shadow: bool, layer: int, frame: int) -> Dictionary:
	if gfx_id < 0:
		return {}

	var dir := "%s/texture/%s" % [assets_root, layer_dir]
	if not DirAccess.dir_exists_absolute(dir):
		return {}

	# gfxID 的位布局：id<<9 | motion<<3 | dir(已减 DIR_BEGIN)
	# 文件名 encode 的位布局：dress<<14 | motion<<8 | dir<<5 | frame
	#   - 名字里的 dress 是「层 ID」（衣服编号 / 武器编号-1）
	#   - motion 也用减过 MOTION_BEGIN 的值
	#   - dir 同样用减过 DIR_BEGIN 的值
	var want_dress := (gfx_id >> 9) & 0xFF
	var want_motion := (gfx_id >> 0) & 0x7   # 先占位，下面用 gfx_motion 覆盖
	want_motion = (gfx_id >> 3) & 0x3F
	var want_dir := gfx_id & 0x7

	var da := DirAccess.open(dir)
	if da == null:
		return {}
	da.list_dir_begin()
	var fn := da.get_next()
	var found := {}
	while fn != "":
		if fn.to_upper().ends_with(".PNG"):
			var info := decode_file_name(fn)
			if info.dress == want_dress \
					and info.motion == want_motion \
					and info.direction == want_dir \
					and info.frame == frame \
					and info.gender == (1 if gender else 0) \
					and info.shadow == (1 if shadow else 0) \
					and info.layer == layer:
				found = {
					"path": "%s/%s" % [dir, fn],
					"dx": info.dx if info.dx_positive else -info.dx,
					"dy": info.dy if info.dy_positive else -info.dy,
				}
				break
		fn = da.get_next()
	da.list_dir_end()
	return found
