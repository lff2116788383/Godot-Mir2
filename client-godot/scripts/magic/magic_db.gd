extends RefCounted
class_name MagicDB
## 魔法图形数据库
##
## 【逐行翻译】client/src/magicdb.hpp + magicdb.cpp（98 行）
##
## 文件名格式（magicdb.hpp:10-25，26 位十六进制）：
##
##   [0~1] [2~6] [7] [8~10] [11~13] [14~16] [17~19] [20~22] [23~25].PNG
##   file  image dir  dx      dy      head x  head y  tail x  tail y
##   1Byte 20bit 4b   ↑ 以下每个字段都是 12 位补码
##
##   key = seqID(gfxID, dir) = gfxID << 4 | dir，即文件名前 8 位
##   gfxID = file << 20 | image
##
## 用途：
##   dx/dy     —— 帧绘制偏移，所有帧共用同一个偏移
##   headX/Y   —— 魔法头部（命中判定点），相对锚点
##   tailX/Y   —— 魔法尾部，用于追踪型魔法的旋转对齐

const SysConstRef5 := preload("res://scripts/core/sys_const.gd")

## seqID（magicdb.hpp:44）
static func seq_id(gfx_id: int, dir: int) -> int:
	if gfx_id >= (1 << 28) or dir < 0 or dir >= 16:
		return -1
	return (gfx_id << 4) | dir


## to_2comp（hexstr.hpp:84）：十六进制补码转有符号
##   "FFF",3 -> -1    "800",3 -> -2048    "7FF",3 -> 2047
static func to_2comp(s: String, bytes: int) -> int:
	if s.length() < bytes or bytes <= 0 or bytes > 8:
		return 0
	var value := 0
	for i in range(bytes):
		var c := s[i]
		var d := 0
		if c >= "0" and c <= "9":
			d = c.unicode_at(0) - 48
		elif c >= "A" and c <= "F":
			d = c.unicode_at(0) - 65 + 10
		elif c >= "a" and c <= "f":
			d = c.unicode_at(0) - 97 + 10
		else:
			return 0
		value = (value << 4) | d
	var bits := bytes * 4
	if value & (1 << (bits - 1)):
		return value - (1 << bits)
	return value


## 解析的文件名信息（对应 _MagicStoreElement 的数值部分）
class ElementInfo:
	var dx: int = 0
	var dy: int = 0
	var head_x: int = 0
	var head_y: int = 0
	var tail_x: int = 0
	var tail_y: int = 0
	## 目录名（不含 .PNG）
	var base_name: String = ""


## 从文件名解析偏移（magicdb.cpp:70-78）
## 例："00000000F37F67000000000000"
static func parse_file_name(fname: String) -> ElementInfo:
	var info := ElementInfo.new()
	var base := fname
	if base.to_upper().ends_with(".PNG"):
		base = base.substr(0, base.length() - 4)
	info.base_name = base
	if base.length() < 26:
		return info
	info.dx = to_2comp(base.substr(8, 3), 3)
	info.dy = to_2comp(base.substr(11, 3), 3)
	info.head_x = to_2comp(base.substr(14, 3), 3)
	info.head_y = to_2comp(base.substr(17, 3), 3)
	info.tail_x = to_2comp(base.substr(20, 3), 3)
	info.tail_y = to_2comp(base.substr(23, 3), 3)
	return info


## 从 gfxID 反推文件名前缀
## gfxID = file << 20 | image，文件名前 8 位是 4 位 file + 4 位 image
static func gfx_file_name(gfx_id: int) -> String:
	var file_index := gfx_id >> 20
	var image_index := gfx_id & 0xFFFFF
	return "%08X" % ((file_index << 20) | image_index)


## 在素材目录中查找某个 seqID 对应的 PNG 文件
## 素材路径：texture/magic/
## 返回文件名或空串
static func find_file(assets_root: String, gfx_id: int, dir: int) -> String:
	var seq := seq_id(gfx_id, dir)
	if seq < 0:
		return ""
	var prefix := "%08X" % seq
	var dir_path := "%s/texture/magic" % assets_root
	var da := DirAccess.open(dir_path)
	if da == null:
		return ""
	da.list_dir_begin()
	var n := da.get_next()
	var found := ""
	while n != "":
		if n.to_upper().begins_with(prefix) and n.to_upper().ends_with(".PNG"):
			found = n
			break
		n = da.get_next()
	da.list_dir_end()
	return found


## frameCount（magicdb.cpp:19）
## 原版从 APNG 读帧数；这里因为素材是拆开的单帧 PNG，
## 需要统计同前缀文件数量（B9 补），当前返回 gfx_entry 声明的帧数
static func frame_count(gfx_entry: Dictionary, dir: int) -> int:
	var declared := int(gfx_entry.get("frameCount", 0))
	if int(gfx_entry.get("gfxID", SysConstRef5.SYS_U32NIL)) == SysConstRef5.SYS_U32NIL:
		return declared
	if declared > 0:
		return declared
	return 0


# ==================== 运行期查询 ====================
static var _records: Dictionary = {}
static var _loaded := false

static func load_data(path: String = "res://data/magicrecord.json") -> bool:
	if _loaded:
		return true
	if not FileAccess.file_exists(path):
		return false
	var f := FileAccess.open(path, FileAccess.READ)
	if f == null:
		return false
	var parsed = JSON.parse_string(f.get_as_text())
	f.close()
	if not parsed is Dictionary:
		return false
	_records = parsed.get("records", {})
	_loaded = true
	return true

static func record(magic_id: int) -> Dictionary:
	if not _loaded: load_data()
	return _records.get(str(magic_id), {})

static func record_count() -> int:
	if not _loaded: load_data()
	return _records.size()

static func name_of(magic_id: int) -> String:
	return str(record(magic_id).get("name", ""))

static func find_id_by_name(name_: String) -> int:
	if not _loaded: load_data()
	for k in _records.keys():
		if str(_records[k].get("name", "")) == name_:
			return int(k)
	return 0

static func gfx_entries(magic_id: int) -> Array:
	var gl = record(magic_id).get("gfxList", {})
	if gl is Dictionary:
		return gl.get("_array", [])
	return []

static func gfx_entry(magic_id: int, stage: String = "运行") -> Dictionary:
	var list := gfx_entries(magic_id)
	for e in list:
		if stage.is_empty() or str(e.get("stage", "")) == stage:
			return e
	return list[0] if not list.is_empty() else {}
