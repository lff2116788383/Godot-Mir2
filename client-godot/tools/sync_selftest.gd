extends Node
## 实体同步自检（场景内运行）
##
## 验证：SM_CORECORD 创建实体 / SM_ACTION 驱动动作 / 视野裁剪 / APNG 加载
## 需先启动 mock 服务端：tools/mock-server/mock-server.exe -port 7000

const MirClientScript := preload("res://scripts/net/mir_client.gd")
const CoManagerScript := preload("res://scripts/core/co_manager.gd")
const CoRecordScript := preload("res://scripts/core/co_record.gd")
const ApngLoaderScript := preload("res://scripts/data/apng_loader.gd")
const MonsterRecordScript := preload("res://scripts/item/monster_record.gd")
const ActionNodeScript := preload("res://scripts/action/action_node.gd")
const ActionTypeScript := preload("res://scripts/action/action_type.gd")
const UidScript := preload("res://scripts/core/uid.gd")

var _client = null
var _co = null
var _pass := 0
var _fail := 0
var _got := {}


func _ok(cond: bool, name: String, extra: String = "") -> void:
	if cond:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, ("  " + extra) if extra else "")


func run(main: Node) -> void:
	print("=== 实体同步自检 ===")

	# ---- 第 1 部分：纯解析（不需要服务端）----
	_test_corecord_parse()
	_test_apng()
	_test_monster_record()

	# ---- 第 2 部分：联网同步 ----
	await _test_net_sync(main)

	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])


# ==================== SMCORecord 解析 ====================

func _test_corecord_parse() -> void:
	print("\n[1] SMCORecord 解析")
	# 手工构造 49 字节：怪物
	var body := PackedByteArray()
	body.resize(49)

	var mon_uid: int = UidScript.build_monster_uid(1, 1)
	body.encode_s64(0, mon_uid)
	body.encode_s64(8, 0x1234)
	# ActionNode: ACTION_STAND=2, dir=5, x=395, y=393
	var act = ActionNodeScript.make_stand(5, 395, 393)
	var ab: PackedByteArray = act.to_bytes()
	for i in range(28):
		body[16 + i] = ab[i]
	body.encode_u32(44, 1)   # MonsterID = 1

	var r = CoRecordScript.parse(body)
	_ok(r != null, "解析成功")
	if r == null:
		return
	_ok(r.uid == mon_uid, "UID 一致", "0x%X" % r.uid)
	_ok(r.map_uid == 0x1234, "mapUID 一致")
	_ok(r.monster_id == 1, "MonsterID = 1")
	_ok(r.action.type == ActionTypeScript.ACTION_STAND, "动作 = STAND")
	_ok(r.action.x == 395 and r.action.y == 393, "坐标 (395,393)")

	# 玩家记录
	var ply_uid: int = UidScript.get_player_uid(1000)
	body.encode_s64(0, ply_uid)
	body.encode_s64(8, 0x1234)
	body.encode_u8(44, 1 | (2 << 1) | (1 << 4))   # gender=1 job=2 teamLeader=1
	body.encode_u32(45, 35)   # 实测 Level 偏移 45
	var r2 = CoRecordScript.parse(body)
	_ok(r2 != null and r2.gender == 1, "玩家 gender = 1")
	_ok(r2 != null and r2.job == 2, "玩家 job = 2")
	_ok(r2 != null and r2.team_leader, "玩家 teamLeader")
	_ok(r2 != null and r2.level == 35, "玩家 level = 35")


# ==================== APNG 拆帧 ====================

func _test_apng() -> void:
	print("\n[2] APNG 拆帧")
	# 先测文件名解析（12 位补码）
	var info := ApngLoaderScript.decode_file_name("00000000FF6FD9")
	_ok(int(info.get("key", -1)) == 0, "KEY = 0")
	_ok(int(info.get("dx", 0)) == -10, "DX = -10（12 位补码）", "实际 %d" % int(info.get("dx", 0)))
	_ok(int(info.get("dy", 0)) == -39, "DY = -39", "实际 %d" % int(info.get("dy", 0)))

	# 真实文件拆帧
	var p := "%s/texture/monster/00000000FF6FD9.PNG" % Game.assets_root
	if not FileAccess.file_exists(p):
		_ok(false, "怪物贴图存在", p)
		return
	var frames := ApngLoaderScript.load_frames(p)
	_ok(frames.size() == 4, "拆出 4 帧（acTL.num_frames=4）", "实际 %d" % frames.size())
	if frames.is_empty():
		return
	var tex: Texture2D = frames[0]
	_ok(tex != null and tex.get_width() == 66 and tex.get_height() == 80,
		"帧尺寸 66x80", str(tex.get_size()) if tex else "null")
	# 各帧内容应不同（否则拆帧可能重复解了同一帧）
	if frames.size() >= 2:
		var i0: Image = frames[0].get_image()
		var i1: Image = frames[1].get_image()
		_ok(i0.get_data() != i1.get_data(), "第0帧与第1帧内容不同")


# ==================== 怪物记录 ====================

func _test_monster_record() -> void:
	print("\n[3] 怪物记录")
	var n := MonsterRecordScript.count()
	_ok(n >= 300, "怪物表已加载", "%d 条" % n)

	var r := MonsterRecordScript.get_record(1)
	_ok(not r.is_empty(), "怪物 1 存在")
	_ok(MonsterRecordScript.name_of(1) == "沙漠蜥蜴", "怪物 1 名称", MonsterRecordScript.name_of(1))

	# 帧表类映射（TaoDog 有自定义帧表）
	var cls := MonsterRecordScript.gfx_class_of(1, r)
	print("    [info] 怪物1 自定义帧表类 = '%s'" % cls)


# ==================== 联网同步 ====================

func _test_net_sync(main: Node) -> void:
	print("\n[4] 联网实体同步")

	_client = MirClientScript.new()
	add_child(_client)
	_client.frame_received.connect(_on_frame)

	var ok = await _client.connect_to("127.0.0.1", 7000)
	_ok(ok, "连接 mock 服务端")
	if not ok:
		print("    [提示] 请先启动 tools/mock-server/mock-server.exe -port 7000")
		return

	# 登录（mock 接受任意账号）
	var login_ok: bool = await _client.login("test", "test")
	_ok(login_ok, "登录成功")
	await _wait(0.3)

	# 查人
	await _client.query_char()
	await _wait(0.3)

	# 进图
	await _client.online()
	await _wait(0.8)

	# 此时应收到 SM_CORECORD × 3
	_ok(_got.has(MirClientScript.SM_CORECORD), "收到 SM_CORECORD")

	# 用 CoManager 处理收到的消息
	_co = CoManagerScript.new()
	_co.name = "CoManagerTest"
	add_child(_co)
	_co.setup(Game.assets_root)
	_co.set_map_context(Callable(main, "can_move_to"), Callable(main, "make_path_finder"))
	_co.map_uid = int(main.get("map_uid"))
	_co.set_my_position_provider(func() -> Vector3i:
		return Vector3i(395, 395, _co.map_uid))
	_co.set_my_uid(0)

	var records: Array = _got_arrays.get(MirClientScript.SM_CORECORD, [])
	_ok(records.size() >= 3, "收到 3 条以上 CORECORD", "实际 %d" % records.size())

	# 诊断：比对 map_uid
	var first = CoRecordScript.parse(records[0])
	print("    [info] 记录 map_uid=0x%X  本地 map_uid=0x%X  一致=%s" % [
		first.map_uid if first else -1, _co.map_uid,
		str(first != null and first.map_uid == _co.map_uid)])
	if first != null:
		print("    [info] 记录位置 (%d,%d) 视野内=%s" % [
			first.action.x, first.action.y,
			str(_co.in_my_hero_view(first.action.x, first.action.y))])
		print("    [info] uid=0x%X 类型=%d (MON=4)" % [first.uid, UidScript.get_uid_type(first.uid)])

	for b in records:
		_co.on_sm_corecord(b)
	_ok(_co.count() >= 3, "创建了 3 个实体", "实际 %d" % _co.count())

	# 验证实体有贴图
	var uids: Array = _co.all_uids()
	if not uids.is_empty():
		var node = _co.find_uid(uids[0])
		_ok(node != null, "能查到实体节点")
		if node != null:
			_ok(node.get_child_count() >= 1, "实体有子节点（Sprite）")
			await _wait(0.2)
			# 取第一个 Sprite2D 子节点检查贴图
			var spr: Sprite2D = null
			for c in node.get_children():
				if c is Sprite2D:
					spr = c
					break
			if spr != null:
				_ok(spr.visible, "怪物 Sprite 可见")
				_ok(spr.texture != null, "怪物贴图已加载")

	# 验证视野裁剪：远处实体不应创建
	var far_body := PackedByteArray()
	far_body.resize(49)
	var far_uid: int = UidScript.build_monster_uid(1, 1, 999)
	far_body.encode_s64(0, far_uid)
	far_body.encode_s64(8, _co.map_uid)
	var far_act = ActionNodeScript.make_stand(5, 100, 100)
	var fab: PackedByteArray = far_act.to_bytes()
	for i in range(28):
		far_body[16 + i] = fab[i]
	far_body.encode_u32(44, 1)
	var before: int = _co.count()
	_co.on_sm_corecord(far_body)
	_ok(_co.count() == before, "视野外实体被拒绝创建", "before=%d after=%d" % [before, _co.count()])

	_client.close_connection()
	await _wait(0.2)


var _got_arrays: Dictionary = {}


func _on_frame(code: int, body: PackedByteArray, _resp: int) -> void:
	_got[code] = body
	if code == MirClientScript.SM_CORECORD:
		if not _got_arrays.has(code):
			_got_arrays[code] = []
		_got_arrays[code].append(body)


func _wait(sec: float) -> void:
	var t := 0.0
	while t < sec:
		await get_tree().process_frame
		t += get_process_delta_time()
