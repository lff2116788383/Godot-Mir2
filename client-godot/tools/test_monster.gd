extends SceneTree
## B6 怪物帧表自检
## 用法：godot --headless --path client-godot --script tools/test_monster.gd

const MF := preload("res://scripts/actor/monster_frames.gd")
const CM := preload("res://scripts/actor/client_monster.gd")
const MT := preload("res://scripts/action/motion_type.gd")

var _pass := 0
var _fail := 0


func _init() -> void:
	print("=== 怪物帧表自检 ===")
	_test_load()
	_test_default()
	_test_taodog()
	_test_zuma()
	_test_gfx_key()
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0 if _fail == 0 else 1)


func _ok(c: bool, name: String, detail: String = "") -> void:
	if c:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, "  ", detail)


func _test_load() -> void:
	print("[1] 数据加载")
	_ok(MF.load_data(), "加载 monster_frames.json")
	_ok(MF.class_count() == 12, "12 个怪物类", "实际 %d" % MF.class_count())


func _test_default() -> void:
	print("[2] 默认帧表（clientmonster.cpp:618）")
	# 未覆盖的怪物走默认表
	_ok(MF.frame_count("", MT.MOTION_MON_STAND) == 4, "STAND=4")
	_ok(MF.frame_count("", MT.MOTION_MON_WALK) == 6, "WALK=6")
	_ok(MF.frame_count("", MT.MOTION_MON_ATTACK0) == 6, "ATTACK0=6")
	_ok(MF.frame_count("", MT.MOTION_MON_HITTED) == 2, "HITTED=2")
	_ok(MF.frame_count("", MT.MOTION_MON_DIE) == 10, "DIE=10")
	_ok(MF.frame_count("", MT.MOTION_MON_SPELL0) == 10, "SPELL0=10")
	_ok(MF.frame_count("", MT.MOTION_MON_SPAWN) == 10, "SPAWN=10")
	_ok(MF.frame_count("", MT.MOTION_MON_SPECIAL) == 6, "SPECIAL=6")


func _test_taodog() -> void:
	print("[3] TaoDog 双状态帧表")
	# standMode 时：STAND=4 无 gfxLookID
	var a: Dictionary = MF.frame_info("ClientTaoDog", MT.MOTION_MON_STAND, true)
	_ok(int(a.get("count", 0)) == 4, "standMode STAND=4")
	_ok(not a.has("gfxLookID"), "standMode 无 gfxLookID 覆盖")

	# 非 standMode 时：STAND=4 且 gfxLookID=0x59=89
	var b: Dictionary = MF.frame_info("ClientTaoDog", MT.MOTION_MON_STAND, false)
	_ok(int(b.get("count", 0)) == 4, "非standMode STAND=4")
	_ok(int(b.get("gfxLookID", 0)) == 89, "非standMode gfxLookID=0x59=89", "实际 %d" % int(b.get("gfxLookID", 0)))

	# SPAWN=10（从趴到站）
	var c: Dictionary = MF.frame_info("ClientTaoDog", MT.MOTION_MON_SPAWN, true)
	_ok(int(c.get("count", 0)) == 10, "standMode SPAWN=10")


func _test_zuma() -> void:
	print("[4] ZumaMonster 帧重定向")
	var a: Dictionary = MF.frame_info("ClientZumaMonster", MT.MOTION_MON_SPAWN, true)
	_ok(int(a.get("count", 0)) == 6, "standMode SPAWN=6")
	_ok(int(a.get("gfxMotionID", 0)) == MT.MOTION_MON_SPAWN, "gfxMotionID 指向 SPAWN")

	var b: Dictionary = MF.frame_info("ClientZumaMonster", MT.MOTION_MON_SPAWN, false)
	_ok(int(b.get("count", 0)) == 6, "非standMode SPAWN=6")
	_ok(int(b.get("begin", 0)) == 5, "begin=5", "实际 %d" % int(b.get("begin", 0)))
	_ok(bool(b.get("reverse", false)), "reverse=true")

	var c: Dictionary = MF.frame_info("ClientZumaMonster", MT.MOTION_MON_STAND, false)
	_ok(int(c.get("count", 0)) == 1, "非standMode STAND 复用 SPAWN=1 帧")


func _test_gfx_key() -> void:
	print("[5] 贴图键合成")
	# creatureKey 位编码：dir 0-2, motion 3-6, shadow 8, look 9-19
	var k := CM.creature_key(5, 3, 2, false)
	_ok((k & 0x7) == 2, "direction 在 bit0-2")
	_ok(((k >> 3) & 0xF) == 3, "motion 在 bit3-6")
	_ok(((k >> 8) & 0x1) == 0, "shadow 在 bit8")
	_ok(((k >> 9) & 0x7FF) == 5, "look 在 bit9-19")

	var ks := CM.creature_key(5, 3, 2, true)
	_ok(((ks >> 8) & 0x1) == 1, "shadow=true 置位")

	# gfx_id 组合
	var mon = CM.new()
	mon.setup(1, {"name": "沙漠蜥蜴", "lookID": 0}, "")
	var g: Dictionary = mon.gfx_id(MT.MOTION_MON_STAND, 3, 2, false)
	_ok(not g.is_empty(), "gfx_id 返回非空")
	_ok(int(g.get("frame", -1)) == 2, "frame = begin(0) + 2", "实际 %d" % int(g.get("frame", -1)))
