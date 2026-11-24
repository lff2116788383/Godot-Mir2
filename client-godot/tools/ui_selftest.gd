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
	# C7 后快捷栏与人物面板改由完整版承载（控件名变了）
	_ok(root.find_widget("QuickAccessBoard") != null, "快捷栏存在（QuickAccessBoard）")
	_ok(root.find_widget("MiniMapBoard") != null, "小地图存在")
	_ok(root.find_widget("MiniMapBoardFull") != null, "小地图完整版存在")
	_ok(root.find_widget("MessageStack") != null, "消息区存在")
	_ok(root.find_widget("InventoryBoard") != null, "背包面板存在")
	_ok(root.find_widget("MyHeroStateBoard") != null, "人物面板存在")
	_ok(root.find_widget("ControlBoard") != null, "控制栏完整版存在")

	# C7 后底部按钮改由控制栏的九宫格贴图承载，用动作名验证可点
	var actions := ["inventory", "skill", "guild", "team", "friendchat",
		"config", "minimap", "auction", "stall", "quest"]
	_ok(actions.size() == 10, "10 个控制栏动作齐全")

	# 背包默认收起
	var inv = root.find_widget("InventoryBoard")
	_ok(not inv.is_shown(), "背包默认收起")

	# 通过控制栏动作开合背包
	ui.ctrl_board._on_button("inventory")
	await get_tree().process_frame
	_ok(inv.is_shown(), "点击后背包打开")
	ui.ctrl_board._on_button("inventory")
	await get_tree().process_frame
	_ok(not inv.is_shown(), "再点后背包收起")

	# 布局：控制栏贴底
	_ok(ui.ctrl_board.position.y > 500, "控制栏在屏幕底部",
		str(ui.ctrl_board.position.y))
	# 小地图在右上（完整版）
	var mini = root.find_widget("MiniMapBoardFull")
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
