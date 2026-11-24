extends SceneTree
## B9 自检：人形角色贴图定位
## 用法：godot --headless --path client-godot --script tools/test_human.gd

const HumanGfxScript := preload("res://scripts/actor/human_gfx.gd")
const MotionTypeScript := preload("res://scripts/action/motion_type.gd")
const ProtocolDefScript := preload("res://scripts/core/protocol_def.gd")

const ASSETS := "E:/Desktop/game/mir2x_assets-main"

var _pass := 0
var _fail := 0


func _init() -> void:
	print("=== 人形角色贴图自检 ===")
	_test_human_key()
	_test_gfx_ids()
	_test_decode()
	_test_find_texture()
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0 if _fail == 0 else 1)


func _ok(c: bool, name: String, detail: String = "") -> void:
	if c:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, "  ", detail)


func _test_human_key() -> void:
	print("[1] humanKey 位编码")
	# gfxID = id<<9 | motion<<3 | dir
	var gfx_id: int = (5 << 9) | (3 << 3) | 2
	var k: int = HumanGfxScript.human_key(gfx_id, true, false, 0)
	_ok((k & 0x7) == 2, "dir 在 bit0-2 = 2", "实际 %d" % (k & 0x7))
	_ok(((k >> 3) & 0x3F) == ((gfx_id >> 3) & 0x3F), "motion 在 bit3-8")
	_ok(((k >> 9) & 0x1) == 0, "layer 在 bit9 = 0")
	_ok(((k >> 10) & 0x1) == 0, "shadow 在 bit10 = 0")
	_ok(((k >> 11) & 0x1) == 1, "gender 在 bit11 = 1")
	_ok(((k >> 12) & 0xFF) == 5, "id 在 bit12-19 = 5", "实际 %d" % ((k >> 12) & 0xFF))

	var ks: int = HumanGfxScript.human_key(gfx_id, false, true, 1)
	_ok(((ks >> 9) & 0x1) == 1, "layer=1 置位")
	_ok(((ks >> 10) & 0x1) == 1, "shadow=true 置位")


func _test_gfx_ids() -> void:
	print("[2] 各层 gfxID 计算")
	# dress: (dress - 0) << 9 | motion << 3 | (dir - 1)
	var d := HumanGfxScript.gfx_dress_id(3, MotionTypeScript.MOTION_STAND, 2)
	var gm: int = MotionTypeScript.MOTION_STAND - MotionTypeScript.MOTION_BEGIN
	_ok(d == ((3 << 9) | (gm << 3) | 1), "dress id 公式", "实际 %d" % d)

	# weapon: (weapon - 1) << 9 | ...
	var w := HumanGfxScript.gfx_weapon_id(2, MotionTypeScript.MOTION_STAND, 2)
	_ok(w == ((1 << 9) | (gm << 3) | 1), "weapon id 公式（-WEAPON_BEGIN）", "实际 %d" % w)

	# 非法输入返回 -1
	_ok(HumanGfxScript.gfx_dress_id(-1, MotionTypeScript.MOTION_STAND, 2) == -1, "dress=-1 返回 -1")
	_ok(HumanGfxScript.gfx_weapon_id(0, MotionTypeScript.MOTION_STAND, 2) == -1, "weapon=0 返回 -1")
	_ok(HumanGfxScript.gfx_helmet_id(1, MotionTypeScript.MOTION_MON_STAND, 2) == -1, "怪物动作返回 -1")


func _test_decode() -> void:
	print("[3] 文件名解码")
	var i := HumanGfxScript.decode_file_name("00000002012FD3.PNG")
	_ok(i.encode == 2, "encode = 2")
	_ok(i.frame == 2, "frame = 2", "实际 %d" % i.frame)
	_ok(i.dress == 0 and i.motion == 0 and i.direction == 0, "dress/motion/dir 全 0")
	_ok(i.layer == 0 and i.shadow == 0 and i.gender == 0, "layer/shadow/gender 全 0")
	_ok(i.dy == 0xD3, "dy = 0xD3 = 211", "实际 %d" % i.dy)

	# 构造一个多层编码验证
	# layer=1 shadow=1 gender=1 dress=5 motion=3 dir=2 frame=7
	var enc: int = (1 << 24) | (1 << 23) | (1 << 22) | (5 << 14) | (3 << 8) | (2 << 5) | 7
	var name := "%08X" % enc
	var i2 := HumanGfxScript.decode_file_name(name + "001122.PNG")
	_ok(i2.layer == 1, "layer=1")
	_ok(i2.shadow == 1, "shadow=1")
	_ok(i2.gender == 1, "gender=1")
	_ok(i2.dress == 5, "dress=5")
	_ok(i2.motion == 3, "motion=3")
	_ok(i2.direction == 2, "dir=2")
	_ok(i2.frame == 7, "frame=7")


func _test_find_texture() -> void:
	print("[4] 实际素材查找")
	var assets := OS.get_environment("MIR2X_ASSETS")
	if assets.is_empty():
		assets = ASSETS
	if not DirAccess.dir_exists_absolute(assets + "/texture/hero"):
		print("  [skip] 素材不存在")
		return

	# 裸身（dress=0）站立朝向 2，帧 0
	# 注意：素材里裸身层是 gender=0（女），性别位必须与素材一致才能查到
	var gid: int = HumanGfxScript.gfx_dress_id(0, MotionTypeScript.MOTION_STAND, 2)
	var r := HumanGfxScript.find_texture(assets, "hero", gid, false, false, 0, 0)
	_ok(not r.is_empty(), "找到裸身站立贴图（gender=0）", str(r))
	if not r.is_empty():
		_ok(FileAccess.file_exists(str(r["path"])), "文件真实存在")
		_ok(int(r["dx"]) <= 0 and int(r["dy"]) <= 0, "偏移为负（向左上）", "dx=%d dy=%d" % [int(r["dx"]), int(r["dy"])])

	# 男性层（gender=1）
	var rm := HumanGfxScript.find_texture(assets, "hero", gid, true, false, 0, 0)
	print("    [info] gender=1 查找结果: ", "命中" if not rm.is_empty() else "未命中（该组合可能不存在）")

	# 影子层：实测 texture/hero 里 shadow=1 的文件数为 0
	# 即本素材包未导出影子贴图，角色渲染时影子层会被跳过
	var r2 := HumanGfxScript.find_texture(assets, "hero", gid, false, true, 0, 0)
	_ok(r2.is_empty(), "影子层素材不存在（确认实测结论）")

	# 统计全目录 shadow 文件数，固定这个事实
	var cnt := 0
	var da := DirAccess.open(assets + "/texture/hero")
	if da != null:
		da.list_dir_begin()
		var fn := da.get_next()
		while fn != "":
			if fn.to_upper().ends_with(".PNG"):
				var e := fn.substr(0, 8).hex_to_int()
				if (e >> 23) & 1 == 1:
					cnt += 1
			fn = da.get_next()
		da.list_dir_end()
	_ok(cnt == 0, "全目录 shadow=1 文件数为 0", "实际 %d" % cnt)

	# 武器层（weapon=1 是最小合法值）
	var wid: int = HumanGfxScript.gfx_weapon_id(1, MotionTypeScript.MOTION_STAND, 2)
	var r3 := HumanGfxScript.find_texture(assets, "weapon", wid, false, false, 0, 0)
	_ok(not r3.is_empty(), "找到武器贴图（gender=0）", str(r3))
