extends RefCounted
class_name MonsterFrames
## 怪物帧数表查询
##
## 【数据来源】tools/extract_monster_frames.py 从 40 个 Client* 怪物类提取
## 结果在 data/monster_frames.json，覆盖 12 个有自定义帧表的类。
##
## 原版结构（MonsterFrameGfxSeq）：
##   gfxLookID   —— 换用另一套外观（如跎犬趴下时用 0x59）
##   gfxMotionID —— 换用另一个动作的帧（如祖玛站立复用 SPAWN 帧）
##   begin       —— 起始帧偏移
##   count       —— 帧数
##   reverse     —— 是否倒放
##
## 未覆盖的 28 个怪物走 ClientMonster::getFrameGfxSeq 的默认表。

const SysConstRef7 := preload("res://scripts/core/sys_const.gd")

static var _data: Dictionary = {}
static var _loaded := false


static func load_data(path: String = "res://data/monster_frames.json") -> bool:
	if _loaded:
		return true
	if not FileAccess.file_exists(path):
		push_error("怪物帧表不存在: " + path)
		return false
	var f := FileAccess.open(path, FileAccess.READ)
	if f == null:
		return false
	var txt := f.get_as_text()
	f.close()
	var parsed = JSON.parse_string(txt)
	if parsed == null or not parsed is Dictionary:
		return false
	_data = parsed
	_loaded = true
	return true


static func class_count() -> int:
	return _data.size()


## 怪物默认帧表（ClientMonster::getFrameGfxSeq）
## 对应 clientmonster.cpp 的默认实现
const DEFAULT_FRAMES := {
	35: 4,   ## MON_STAND
	36: 6,   ## MON_WALK
	37: 6,   ## MON_ATTACK0
	38: 2,   ## MON_HITTED
	39: 10,  ## MON_DIE
	40: 6,   ## MON_ATTACK1
	41: 10,  ## MON_SPELL0
	42: 10,  ## MON_SPELL1
	43: 10,  ## MON_SPAWN
	44: 6,   ## MON_SPECIAL
}


## 查询某怪物某动作的帧信息
##   class_name_v : 如 "ClientTaoDog"（空串则用默认表）
##   motion       : MOTION_MON_*
##   stand_mode   : 是否有 standMode 分支时传入
## 返回 {count, gfxLookID, gfxMotionID, begin, reverse}，count=0 表示无此动作
static func frame_info(class_name_v: String, motion: int, stand_mode: bool = false) -> Dictionary:
	if not _loaded:
		load_data()

	var entry: Dictionary = _data.get(class_name_v, {})
	var frames: Dictionary = entry.get("frames", {})
	if not frames.is_empty():
		var modes: Array = frames.get("modes", [])
		if not modes.is_empty():
			# 按 standMode 选分支
			var want := "standMode" if stand_mode else "!standMode"
			for m in modes:
				if str(m.get("cond", "")) == want:
					var t: Dictionary = m.get("table", {})
					if t.has(str(motion)):
						return _normalize(t[str(motion)])
					# 该分支明确返回 {} 表示无此动作
					if not t.is_empty():
						return {}
		var dflt: Dictionary = frames.get("default", {})
		if dflt.has(str(motion)):
			return _normalize(dflt[str(motion)])

	# 回落到默认表
	var c := int(DEFAULT_FRAMES.get(motion, 0))
	if c <= 0:
		return {}
	return {"count": c}


static func _normalize(d: Dictionary) -> Dictionary:
	var r := {
		"count": int(d.get("count", 0)),
	}
	if d.has("gfxLookID"):
		r["gfxLookID"] = int(d["gfxLookID"])
	if d.has("gfxMotionID"):
		r["gfxMotionID"] = int(d["gfxMotionID"])
	if d.has("begin"):
		r["begin"] = int(d["begin"])
	if d.has("reverse"):
		r["reverse"] = bool(d["reverse"])
	return r


## 帧数便捷查询
static func frame_count(class_name_v: String, motion: int, stand_mode: bool = false) -> int:
	return int(frame_info(class_name_v, motion, stand_mode).get("count", 0))


## 按怪物中文名反查类名
static func class_by_monster_name(monster_name: String) -> String:
	if not _loaded:
		load_data()
	for k in _data.keys():
		if str(_data[k].get("monster_name", "")) == monster_name:
			return str(k)
	return ""
