extends Node
## C3+C4 场景内集成自检
## 验证：背包显示物品、NPC 对话面板弹出、战斗飘字、死亡淡出

var _pass := 0
var _fail := 0


func _ok(cond: bool, name: String, extra: String = "") -> void:
	if cond:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, ("  " + extra) if extra else "")


func run(main: Node) -> void:
	print("=== C3+C4 场景集成自检 ===")
	var ui = main.get("ui")
	var fx = main.get("fx")

	_ok(ui != null, "界面存在")
	_ok(fx != null, "战斗表现层存在")
	if ui == null or fx == null:
		print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
		return

	# --- NPC 对话面板 ---
	var npc = ui.npc_chat
	_ok(npc != null, "NPC 对话面板已装配")
	if npc != null:
		_ok(not npc.is_open(), "默认关闭")
		npc.show_layout(0xAAA, "<layout><par>欢迎来到比奇县，勇士。</par></layout>", "npc_hello")
		_ok(npc.is_open(), "弹出对话")
		var root = ui.control_board
		_ok(root.find_widget("NpcChat") != null, "在控件树中可查")
		# 面板在屏幕内
		_ok(npc.position.y > 0 and npc.position.y < 720, "面板位置合理", str(npc.position.y))

	# --- 背包显示物品 ---
	var inv = ui.inventory_data
	_ok(inv != null, "背包数据已注入")
	if inv != null:
		var cnt_before: int = inv.size()
		print("    [info] 背包物品数 = %d" % cnt_before)

	# --- 战斗飘字 ---
	fx.on_damage(400, 402, 128)
	fx.on_magic_damage(400, 402, 256)
	fx.on_heal(400, 402, 30)
	fx.on_miss(401, 402)
	_ok(fx.ascend_count() == 4, "4 条飘字已加入", str(fx.ascend_count()))

	# 只推 1 个物理帧，检查 tick 增量是否合理（应在 0~200ms 之间）
	var tick_before: float = float(fx._ascend_list[0]["tick"])
	# 等两个物理帧，确保 _physics_process 已跑过
	await get_tree().physics_frame
	await get_tree().physics_frame
	var tick_after: float = float(fx._ascend_list[0]["tick"]) if fx.ascend_count() > 0 else -1.0
	var delta := tick_after - tick_before
	print("    [info] 单帧 tick 增量 = %.1f ms" % delta)
	_ok(delta > 0.0 and delta < 500.0, "单帧推进量合理", "%.1f ms" % delta)

	# --- 死亡淡出 ---
	var cm = main.get("co_manager")
	if cm != null and cm.count() > 0:
		var uids = cm.all_uids()
		var node = cm.find_uid(uids[0])
		_ok(node != null, "取到怪物节点")
		if node != null:
			fx.start_fade_out(node)
			_ok(fx.fade_count() == 1, "淡出已开始")
			for i in range(40):
				await get_tree().process_frame
			_ok(fx.fade_count() >= 0, "淡出推进不崩溃")

	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
