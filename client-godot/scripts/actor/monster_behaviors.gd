extends RefCounted
class_name MonsterBehaviors
## 怪物行为差异表
##
## 数据来自 data/monster_behaviors.json，
## 由 tools/extract_monster_behaviors.py 从 mir2x 的 32 个 Client* 类自动提取。
##
## ── 提取到的真实分布（32 个类）──
##   有 standMode 趴站切换 : 7 个（TaoDog / ZumaTaurus / CannibalPlant 等）
##   有自定义帧表          : 13 个
##   用 MOTION_MON_ATTACK1 : 2 个（牛魔守卫 / 沙漠仙人掌）
##   纯默认行为            : 19 个
##
## ── 为什么可以数据化 ──
## 原版 32 个类看着像 32 份独立实现，实际差异只有 4 个维度：
##   1. 帧表（已在 monster_frames.json）
##   2. spawn/stand/transf 三段式（= 有没有趴站切换）
##   3. 攻击动作类型（ATTACK0 还是 ATTACK1）
##   4. 特殊动作（SPECIAL / SPELL0 / SPELL1）
## 死亡特效是基类 clientmonster.cpp:420 统一处理的，子类不覆盖。
##
## 所以不需要 32 个 GDScript 文件，一张表 + 一个解释器就够。

static var _classes: Dictionary = {}
## 怪物名 -> 类名（由提取器从 clientmonster.cpp 的 create() 得到）
static var _name_index: Dictionary = {}
static var _loaded := false


static func load_data(path: String = "res://data/monster_behaviors.json") -> bool:
	if _loaded:
		return true
	if not FileAccess.file_exists(path):
		push_warning("怪物行为表不存在: " + path)
		return false
	var f := FileAccess.open(path, FileAccess.READ)
	if f == null:
		return false
	var parsed = JSON.parse_string(f.get_as_text())
	f.close()
	if parsed == null or not parsed.has("classes"):
		return false
	_classes = parsed["classes"]
	# 建「怪物名 -> 类名」索引，支持按怪物名查
	_name_index.clear()
	for cls_name in _classes.keys():
		var nm := str(_classes[cls_name].get("monster_name", ""))
		if not nm.is_empty():
			_name_index[nm] = cls_name
	_loaded = true
	return true


## 按怪物名查行为（clientmonster.cpp 的 create 映射）
## 返回 {} 表示该怪物没有自定义行为（走默认）
static func of_monster_name(monster_name: String) -> Dictionary:
	if not _loaded:
		load_data()
	if _name_index.has(monster_name):
		return of(_name_index[monster_name])
	return {}


## 按怪物名查类名
static func class_of_monster(monster_name: String) -> String:
	if not _loaded:
		load_data()
	return str(_name_index.get(monster_name, ""))


## 取某类的行为描述；不存在则返回默认行为
static func of(class_name_: String) -> Dictionary:
	if not _loaded:
		load_data()
	if not class_name_.is_empty() and _classes.has(class_name_):
		return _classes[class_name_]
	# 默认行为（对应原版 ClientMonster 基类）
	return {
		"class_name": class_name_,
		"attack_motion": "MOTION_MON_ATTACK0",
		"has_stand_mode": false,
		"override_spawn": false,
		"override_stand": false,
		"override_transf": false,
		"custom_frame_table": false,
		"special_motions": [],
		"attack_segments": 1,
		"is_default": true,
	}


## 攻击动作用 ATTACK0 还是 ATTACK1
## 返回 MotionType 常量值
static func attack_motion_of(class_name_: String) -> int:
	var b := of(class_name_)
	var mt := preload("res://scripts/action/motion_type.gd")
	if str(b.get("attack_motion", "")) == "MOTION_MON_ATTACK1":
		return mt.MOTION_MON_ATTACK1
	return mt.MOTION_MON_ATTACK0


## 是否有趴站切换（决定 SPAWN/STAND/TRANSF 的处理方式）
static func has_stand_mode(class_name_: String) -> bool:
	return bool(of(class_name_).get("has_stand_mode", false))


## 特殊动作列表
static func special_motions_of(class_name_: String) -> Array:
	return of(class_name_).get("special_motions", [])


## 攻击段数
static func attack_segments_of(class_name_: String) -> int:
	return int(of(class_name_).get("attack_segments", 1))


static func count() -> int:
	if not _loaded:
		load_data()
	return _classes.size()


## 统计（用于自检与文档）
static func stats() -> Dictionary:
	if not _loaded:
		load_data()
	var n_stand := 0
	var n_frames := 0
	var n_atk1 := 0
	var n_default := 0
	for v in _classes.values():
		if bool(v.get("has_stand_mode", false)):
			n_stand += 1
		if bool(v.get("custom_frame_table", false)):
			n_frames += 1
		if str(v.get("attack_motion", "")) == "MOTION_MON_ATTACK1":
			n_atk1 += 1
		if bool(v.get("is_default", false)):
			n_default += 1
	return {
		"total": _classes.size(),
		"stand_mode": n_stand,
		"custom_frames": n_frames,
		"attack1": n_atk1,
		"default": n_default,
	}


## 全部类名
static func all_class_names() -> Array:
	if not _loaded:
		load_data()
	return _classes.keys()
