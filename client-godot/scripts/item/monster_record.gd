extends RefCounted
class_name MonsterRecord
## 怪物记录查询
##
## 【逐行翻译】common/src/monsterrecord.hpp
## 数据来自 data/monsterrecord.json（364 条，由 tools/inc2json.py 从 monsterrecord.inc 生成）
##
## 说明：原版是编译期常量表，这里改运行时查 JSON；字段名与 .inc 保持一致。

static var _records: Dictionary = {}
static var _loaded := false

## 有自定义帧表的怪物类（对应 client/src/client*.hpp）
## 依据：40 个 Client* 类里，12 个重写了 getFrameGfxSeq
## 详见 data/monster_frames.json 与 tools/extract_monster_frames.py
static var _gfx_class_map: Dictionary = {}


static func load_data(path: String = "res://data/monsterrecord.json") -> bool:
	if _loaded:
		return true
	if not FileAccess.file_exists(path):
		push_error("怪物表不存在: " + path)
		return false
	var f := FileAccess.open(path, FileAccess.READ)
	if f == null:
		return false
	var parsed = JSON.parse_string(f.get_as_text())
	f.close()
	if parsed == null or not parsed.has("records"):
		return false
	_records = parsed["records"]

	# 载入帧表类映射
	var fp := "res://data/monster_frames.json"
	if FileAccess.file_exists(fp):
		var ff := FileAccess.open(fp, FileAccess.READ)
		if ff != null:
			var fj = JSON.parse_string(ff.get_as_text())
			ff.close()
			if fj is Dictionary:
				_gfx_class_map = fj
	_loaded = true
	return true


## 取怪物记录（返回带 _id 的 Dictionary）
static func get_record(monster_id: int) -> Dictionary:
	if not _loaded:
		load_data()
	var key := str(monster_id)
	if not _records.has(key):
		return {}
	var r: Dictionary = _records[key]
	# 补上 _id，供 ClientMonster.monster_id() 使用
	if not r.has("_id"):
		r = r.duplicate()
		r["_id"] = monster_id
	return r


## 怪物名（monsterrecord.hpp name 字段）
static func name_of(monster_id: int) -> String:
	return str(get_record(monster_id).get("name", ""))


## 查该怪物是否有自定义帧表类
## 返回值用作 MonsterFrames.frame_info 的 class 参数
static func gfx_class_of(monster_id: int, mr: Dictionary = {}) -> String:
	if mr.is_empty():
		mr = get_record(monster_id)
	var nm := str(mr.get("name", ""))
	# 帧表 JSON 的键是 C++ 类名，如 ClientTaoDog
	# 这里按怪物名反查：monster_frames.json 里每个类都记了 monster_name
	for cls_name in _gfx_class_map.keys():
		var entry = _gfx_class_map[cls_name]
		if entry is Dictionary and str(entry.get("monster_name", "")) == nm:
			return cls_name
	return ""


## 怪物视野（monsterrecord.hpp view）
static func view_of(monster_id: int) -> int:
	return int(get_record(monster_id).get("view", 0))


## 怪物血量
static func hp_of(monster_id: int) -> int:
	return int(get_record(monster_id).get("hp", 0))


static func count() -> int:
	if not _loaded:
		load_data()
	return _records.size()
