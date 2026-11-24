extends Node
## C5+C6 场景集成自检

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
	print("=== C5+C6 场景集成自检 ===")
	var ui = main.get("ui")
	_ok(ui != null, "界面存在")
	if ui == null:
		print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
		return

	var root = ui.control_board

	# C5 面板全部装配
	var names := ["SkillBoard", "TeamStateBoard", "GuildBoard", "RuntimeConfigBoard",
		"PurchaseBoard", "AuctionBoard", "StallBoard", "SecuredItemListBoard"]
	var found := 0
	for n in names:
		if root.find_widget(n) != null:
			found += 1
		else:
			print("    [miss] %s" % n)
	_ok(found == 8, "8 个 C5 面板全部装配", "%d/8" % found)

	# 默认都收起
	var all_closed := true
	for n in names:
		var w = root.find_widget(n)
		if w != null and w.show:
			all_closed = false
	_ok(all_closed, "所有面板默认收起")

	# 点技能按钮打开技能面板（C7 后按钮改由控制栏的九宫格贴图承载，
	# 不再有 Btn_技能 控件，直接走控制栏动作分发）
	var ui2 = main.get("ui")
	_ok(ui2.ctrl_board != null, "控制栏存在")
	ui2.ctrl_board._on_button("skill")
	await get_tree().process_frame
	var sk = root.find_widget("SkillBoard")
	_ok(sk.show, "技能面板已打开")
	_ok(sk.position.x >= 0 and sk.position.y >= 0, "面板位置在屏幕内", str(sk.position))
	ui2.ctrl_board._on_button("skill")
	await get_tree().process_frame
	_ok(not sk.show, "再点收起")

	# 点行会按钮
	ui2.ctrl_board._on_button("guild")
	await get_tree().process_frame
	var gb = root.find_widget("GuildBoard")
	_ok(gb.show, "行会面板已打开")

	# 点设置
	ui2.ctrl_board._on_button("config")
	await get_tree().process_frame
	var cb = root.find_widget("RuntimeConfigBoard")
	_ok(cb.show, "设置面板已打开")

	# C6 行为表在场景中可用
	var cm = main.get("co_manager")
	if cm != null and cm.count() > 0:
		var uids = cm.all_uids()
		var node = cm.find_uid(uids[0])
		if node != null:
			_ok(not node.behavior.is_empty(), "怪物已加载行为描述")
			print("    [info] 怪物 %s 行为: atk=%s standMode=%s" % [
				str(node.behavior.get("monster_name", "?")),
				str(node.behavior.get("attack_motion", "?")).replace("MOTION_MON_", ""),
				str(node.behavior.get("has_stand_mode", false))])

	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
