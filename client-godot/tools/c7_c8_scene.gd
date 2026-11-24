extends Node
## C7+C8 场景集成自检（Autoload 可用）
##
## 验证：
##   1. 全部 C7 面板确实装配进控件树
##   2. 控制栏按视口正确贴底、宽度自适应
##   3. 面板开合不互相打架
##   4. 音频 Autoload 可用、能取到真实音效流
##   5. UI 图库纹理能真正加载（不是空指针）

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
	print("=== C7+C8 场景集成自检 ===")
	var ui = main.get("ui")
	_ok(ui != null, "界面存在")
	if ui == null:
		_report()
		return

	_test_assembly(ui)
	_test_layout(ui)
	_test_audio()
	_test_tex()
	_report()


func _report() -> void:
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])


# ==================== 面板装配 ====================

func _test_assembly(ui) -> void:
	print("\n[C7-1] 面板装配")
	var root = ui.control_board
	_ok(root != null, "根控件存在")

	var names := [
		"ControlBoard", "MyHeroStateBoard", "HeroStateBoard",
		"QuickAccessBoard", "PurchaseBoard", "DirectTradeBoard",
		"StallBoardFull", "AuctionBoardFull", "FriendChatBoard",
		"QuestStateBoard", "HorseBoard", "MiniMapBoardFull",
		"InputStringBoard",
	]
	var found := 0
	for n in names:
		if root.find_widget(n) != null:
			found += 1
		else:
			print("    [miss] %s" % n)
	_ok(found == names.size(), "%d 个 C7 面板全部装配" % names.size(),
		"%d/%d" % [found, names.size()])

	# 关键属性直读（不经控件树）
	_ok(ui.ctrl_board != null, "ctrl_board 引用可用")
	_ok(ui.my_state_board != null, "my_state_board 引用可用")
	_ok(ui.quick_access != null, "quick_access 引用可用")
	_ok(ui.mini_map2 != null, "mini_map2 引用可用")
	_ok(ui.input_board != null, "input_board 引用可用")
	_ok(ui.menu != null, "menu 已创建")

	# 默认收起的面板
	var closed := ["MyHeroStateBoard", "HeroStateBoard", "QuickAccessBoard",
		"PurchaseBoard", "DirectTradeBoard", "StallBoardFull",
		"AuctionBoardFull", "FriendChatBoard", "QuestStateBoard",
		"HorseBoard", "InputStringBoard"]
	var all_closed := true
	for n in closed:
		var w = root.find_widget(n)
		if w != null and w.show:
			all_closed = false
			print("    [open] %s" % n)
	_ok(all_closed, "面板默认收起")


# ==================== 布局 ====================

func _test_layout(ui) -> void:
	print("\n[C7-1] 控制栏布局")
	var cb = ui.ctrl_board
	var vw: int = int(ui.vw)
	var vh: int = int(ui.vh)
	_ok(cb.vw == vw, "控制栏视口宽同步", "%d vs %d" % [cb.vw, vw])
	_ok(cb.w == vw, "控制栏宽 = 视口宽", "%d vs %d" % [cb.w, vw])

	var bh: int = cb.bar_height()
	_ok(bh == 152, "普通模式高 152", str(bh))
	_ok(int(cb.position.y) == vh - bh, "贴底 y = vh - 152",
		"%d vs %d" % [int(cb.position.y), vh - bh])
	_ok(int(cb.position.x) == 0, "左对齐 x=0")

	# 中段宽度
	var mw: int = cb.middle_width()
	_ok(mw == vw - 178 - 166, "中段宽 = vw-344", "%d vs %d" % [mw, vw - 178 - 166])

	# 快捷栏在控制栏上方 48
	var qa = ui.quick_access
	_ok(int(qa.position.y) == vh - bh - 48, "快捷栏 y = vh - 152 - 48",
		str(int(qa.position.y)))

	# 小地图贴右上
	var mm = ui.mini_map2
	_ok(int(mm.position.x) == vw - 1 - 200, "小地图 x = vw-201",
		str(int(mm.position.x)))
	_ok(int(mm.position.y) == 0, "小地图 y = 0")

	# 自身状态居中
	var ms = ui.my_state_board
	_ok(int(ms.position.x) == vw / 2 - 164, "自身状态 x = vw/2-164",
		str(int(ms.position.x)))
	_ok(int(ms.position.y) == vh / 2 - 233, "自身状态 y = vh/2-233",
		str(int(ms.position.y)))

	# 切最小化后高度变化
	cb.mode = 2   # MODE_MINIMIZE
	_ok(cb.bar_height() == 24, "最小化后高 24", str(cb.bar_height()))
	cb.mode = 0


# ==================== 音频（Autoload）====================

func _test_audio() -> void:
	print("\n[C8] 音频 Autoload")
	var audio = get_node_or_null("/root/Audio")
	_ok(audio != null, "Audio Autoload 已注册")
	if audio == null:
		return

	_ok(not audio.assets_root.is_empty(), "音频素材根可达", audio.assets_root)

	# 真实加载一个脚步音效
	var step = audio.get_seff(0x01000001)
	_ok(step != null, "脚步音效 01000001 可加载")
	if step is AudioStreamWAV:
		_ok((step as AudioStreamWAV).get_length() > 0.01, "音效有实际时长",
			"%.3fs" % (step as AudioStreamWAV).get_length())

	# 各段的代表性音效
	var probes := [
		[0x01010000 + 52, "挥剑"],
		[0x01020000 + 103, "按钮点击"],
		[0x01030000 + 139, "男声受击"],
		[0x02000001, "怪物音"],
		[0x04000000 + 13 * 16, "魔法音"],
	]
	var okn := 0
	for p in probes:
		var st: AudioStream = audio.get_seff(int(p[0]))
		if st != null:
			okn += 1
		else:
			print("    [miss] %s %s" % [p[1], str(p[0])])
	_ok(okn == probes.size(), "5 段代表性音效全部可加载", "%d/5" % okn)

	# BGM
	var bgm = audio.get_bgm(0x00040007)
	_ok(bgm != null, "登录 BGM 可加载")

	# 播放（headless 无音频设备时 enabled=false，播放会拒绝，这是预期行为）
	var is_headless := DisplayServer.get_name() == "headless"
	var played: bool = audio.play_seff(0x01010000 + 52, 10, 90)
	if is_headless:
		_ok(not played, "headless 下拒绝播放（预期）")
	else:
		_ok(played, "带定位播放返回成功")
	var sp: Dictionary = audio.spatial_param(10, 0)
	_ok(int(sp["distance"]) == 10, "播放距离 10")
	audio.stop_all_seff()

	# 统计
	var st2: Dictionary = audio.stats()
	print("    [info] 音频统计: %s" % str(st2))
	if is_headless:
		_ok(int(st2["played"]) == 0, "headless 下播放计数为 0（预期）", str(st2["played"]))
	else:
		_ok(int(st2["played"]) >= 1, "播放计数 > 0", str(st2["played"]))
	_ok(int(st2["seff_loaded"]) >= 5, "至少加载 5 个音效流",
		str(st2["seff_loaded"]))


# ==================== UI 贴图实载 ====================

func _test_tex() -> void:
	print("\n[C7-0] UI 贴图实载")
	var ProgUse = load("res://scripts/data/proguse_tex.gd")
	_ok(ProgUse != null, "ProgUseTex 可用")
	# 控制栏背景必须能真的拿到
	var bg = ProgUse.retrieve(0x00000012)
	_ok(bg != null, "控制栏背景 0x12 可加载")
	if bg != null:
		_ok(bg.get_width() == 800 and bg.get_height() == 133,
			"背景 800x133", "%dx%d" % [bg.get_width(), bg.get_height()])

	# 状态框
	var frame = ProgUse.retrieve(0x06000000)
	_ok(frame != null, "自身状态框 0x6000000 可加载")
	if frame != null:
		_ok(frame.get_width() == 328 and frame.get_height() == 466,
			"状态框 328x466", "%dx%d" % [frame.get_width(), frame.get_height()])

	# 按钮
	var btn = ProgUse.retrieve(0x0000001C)
	_ok(btn != null, "关闭按钮 0x1C 可加载")

	# 血条
	var hp = ProgUse.retrieve(0x00000018)
	_ok(hp != null, "HP 条 0x18 可加载")
	if hp != null:
		_ok(hp.get_width() == 36 and hp.get_height() == 88, "HP 条 36x88",
			"%dx%d" % [hp.get_width(), hp.get_height()])
