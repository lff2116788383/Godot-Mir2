extends Node
## 界面自检（场景内运行）
## 验证：控件树构建、面板开关、按钮点击、事件分发

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
	print("=== 界面自检 ===")
	var ui = main.get("ui")
	_ok(ui != null, "界面已创建")
	if ui == null:
		print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
		return

	var root = ui.control_board
	_ok(root != null, "根控件存在")
	_ok(root.find_widget("ControlBar") != null, "底部按钮栏存在")
	_ok(root.find_widget("QuickAccess") != null, "快捷栏存在")
	_ok(root.find_widget("MiniMapBoard") != null, "小地图存在")
	_ok(root.find_widget("MessageStack") != null, "消息区存在")
	_ok(root.find_widget("InventoryBoard") != null, "背包面板存在")
	_ok(root.find_widget("MyHeroStateBoard") != null, "人物面板存在")

	# 10 个功能按钮
	var btn_names := ["背包", "技能", "行会", "队伍", "好友", "设置", "地图", "拍卖", "摊位", "帮助"]
	var found := 0
	for n in btn_names:
		if root.find_widget("Btn_" + n) != null:
			found += 1
	_ok(found == 10, "10 个功能按钮全部存在", "%d/10" % found)

	# 背包默认收起
	var inv = root.find_widget("InventoryBoard")
	_ok(not inv.is_shown(), "背包默认收起")

	# 点击背包按钮 -> 面板打开
	var btn_inv = root.find_widget("Btn_背包")
	_ok(btn_inv != null, "背包按钮可查")
	if btn_inv != null:
		btn_inv.on_click.call(btn_inv)
		await get_tree().process_frame
		_ok(inv.is_shown(), "点击后背包打开")
		btn_inv.on_click.call(btn_inv)
		await get_tree().process_frame
		_ok(not inv.is_shown(), "再点后背包收起")

	# 布局：按钮栏在底部
	_ok(ui.control_bar.position.y > 600, "按钮栏在屏幕底部", str(ui.control_bar.position.y))
	# 小地图在右上
	var mini = root.find_widget("MiniMapBoard")
	_ok(mini.position.x > 1000, "小地图在右侧", str(mini.position.x))
	_ok(mini.position.y < 50, "小地图在顶部", str(mini.position.y))

	# 控件树规模
	print("    [info] 控件总数 = %d" % _count_all(root))
	_ok(_count_all(root) >= 17, "控件树规模合理（17 个以上）")

	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])


func _count_all(w) -> int:
	var n := 1
	for c in w.get_children_widgets():
		n += _count_all(c)
	return n
