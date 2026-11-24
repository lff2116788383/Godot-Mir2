extends Node2D
## 主场景：加载地图 + 主角点击走路
## 阶段 2 验证入口：不依赖服务端

const GRID_X := 48
const GRID_Y := 32

@onready var map_view: MapView = $MapView
@onready var cam: Camera2D = $Camera2D
@onready var hud: Label = $UI/HUD

var _map: MapLoader.MapData = null
var _player: PlayerActor = null
var _human: HumanActor = null
var _spawn := Vector2i.ZERO
var _load_ms := 0
## 当前地图 UID（maprecord.inc: 比奇县_0 的 mapID = 1）
var map_uid: int = 0
## 场景实体管理器（联机时启用）
var co_manager: CoManager = null
## 网络客户端（联机时启用）
var net: MirClient = null
## 游戏界面（C1+C2 产物）
var ui: GameUi = null
## 战斗表现层（C4 产物）
var fx: FightEffects = null
## 背包数据（SDInventory）
var _inventory = null

const SdInventory := preload("res://scripts/item/sd_inventory.gd")


func _ready() -> void:
	map_view.set_camera(cam)

	# 按 maprecord.inc 第 1 条：比奇县_0 -> 0.bin
	var path := "%s/map/0.bin" % Game.assets_root
	var t0 := Time.get_ticks_msec()
	_map = MapLoader.load_file(path)
	_load_ms = Time.get_ticks_msec() - t0

	if _map == null:
		hud.text = "地图加载失败: " + path
		push_error(hud.text)
		return

	# 比奇县_0 的 mapID = 1（maprecord.inc 第 1 条），peerIndex = 1
	map_uid = Uid.build_map_uid(1, 1)
	_spawn = _find_spawn(Vector2i(_map.w / 2, _map.h / 2))
	map_view.load_map(_map)
	_create_player()

	print("[Main] 地图 %dx%d 格, 块=%d, 解析 %d ms" % [_map.w, _map.h, _map.tiles.size(), _load_ms])
	print("[Main] 出生点 = (%d,%d) 可走=%s" % [_spawn.x, _spawn.y, str(_is_walkable(_spawn))])

	var valid := 0
	for t in _map.tiles:
		if t >= 0:
			valid += 1
	print("[Main] 有效瓦片块=%d / %d (%.1f%%)" % [valid, _map.tiles.size(), 100.0 * valid / _map.tiles.size()])

	await get_tree().process_frame
	await get_tree().process_frame
	print("[Main] 纹理缓存: %s" % str(TexCache.stats()))

	_create_fx()
	_init_inventory()
	_create_ui()
	_update_hud()
	await _maybe_connect()
	_maybe_run_test()
	await _maybe_demo()
	_maybe_autoshot()


## 初始化背包（实际数据由服务端下发，这里先建空背包）
func _init_inventory() -> void:
	_inventory = SdInventory.new()
	print("[Main] 背包已初始化，容量 %d" % _inventory.capacity())


## 创建战斗表现层（对应 ProcessRun 的 m_ascendStrList）
func _create_fx() -> void:
	fx = FightEffects.new()
	fx.name = "FightEffects"
	add_child(fx)
	print("[Main] 战斗表现层已创建")


## 创建游戏界面（对应 GUIManager）
func _create_ui() -> void:
	ui = GameUi.new()
	ui.name = "GameUi"
	add_child(ui)
	# 注入数据引用
	ui.co_manager = co_manager
	ui.player_actor = _player
	ui.inventory_data = _inventory
	ItemGfx.setup(Game.assets_root)
	print("[Main] 界面已创建")


## 支持 --demo：演示 NPC 对话与战斗飘字（用于截图验证）
func _maybe_demo() -> void:
	var args := OS.get_cmdline_user_args()
	if args.find("--demo") < 0:
		return
	if ui == null:
		return
	await _wait_sec(0.3)
	# NPC 对话（实测格式）
	if ui.npc_chat != null:
		ui.npc_chat.show_layout(0x1234,
			"<layout><par>勇士，你终于来了。比奇县最近不太平，</par><par>南边的森林里出现了许多怪物。</par></layout>",
			"npc_hello")
	# 战斗飘字
	if fx != null:
		fx.on_damage(_player.grid.x + 2, _player.grid.y, 128)
		fx.on_magic_damage(_player.grid.x - 2, _player.grid.y, 256)
		fx.on_heal(_player.grid.x, _player.grid.y - 2, 30)
		fx.on_miss(_player.grid.x + 3, _player.grid.y + 1)
	# 打开技能面板（C5 产物，63 个魔法图标分 8 组）
	if ui.skill_board != null:
		ui.skill_board.show = true
		ui.skill_board.learn("火球术")
		ui.skill_board.learn("治愈术")
		ui.skill_board.learn("雷电术")
		ui.skill_board.learn("基本剑术")
	# 系统消息
	ui._push_sys_msg("欢迎来到比奇县")
	ui._push_sys_msg("按 F1 打开背包")


## 支持 --online：连 mock 服务端并显示场景实体
func _maybe_connect() -> void:
	var args := OS.get_cmdline_user_args()
	if args.find("--online") < 0:
		return

	co_manager = CoManager.new()
	co_manager.name = "CoManager"
	add_child(co_manager)
	co_manager.setup(Game.assets_root)
	co_manager.set_map_context(Callable(self, "can_move_to"), Callable(self, "make_path_finder"))
	co_manager.map_uid = map_uid
	co_manager.set_my_position_provider(func() -> Vector3i:
		return Vector3i(_player.grid.x, _player.grid.y, map_uid))
	co_manager.entity_query_requested.connect(func(uid: int):
		print("[Main] 请求查询实体 0x%X" % uid))

	net = MirClient.new()
	net.name = "Net"
	add_child(net)
	net.frame_received.connect(_on_net_frame)

	var ok = await net.connect_to("127.0.0.1", 7000)
	if not ok:
		print("[Main] 连接服务端失败（未启动 mock？）")
		return
	print("[Main] 已连接 mock 服务端")

	await net.login("test", "test")
	await _wait_sec(0.2)
	await net.query_char()
	await _wait_sec(0.2)
	await net.online()
	await _wait_sec(1.0)
	print("[Main] 场景实体数 = %d" % (co_manager.count() if co_manager != null else 0))


## 服务端消息分发
func _on_net_frame(code: int, body: PackedByteArray, _resp: int) -> void:
	if co_manager == null:
		return
	match code:
		MirClient.SM_CORECORD:
			co_manager.on_sm_corecord(body)
		MirClient.SM_ACTION:
			co_manager.on_sm_action(body)
		MirClient.SM_CODISAPPEAR:
			if body.size() >= 16:
				co_manager.remove_uid(body.decode_s64(0))
		MirClient.SM_DEADFADEOUT:
			# SMDeadFadeOut { u64 UID; u64 mapUID; u32 X; u32 Y; } = 24 字节
			if body.size() >= 16 and fx != null:
				var uid := body.decode_s64(0)
				var node = co_manager.find_uid(uid)
				if node != null:
					fx.start_fade_out(node)
		MirClient.SM_NOTIFYDEAD:
			# SMNotifyDead { u64 UID; } = 8 字节
			if body.size() >= 8 and fx != null:
				var uid2 := body.decode_s64(0)
				var node2 = co_manager.find_uid(uid2)
				if node2 != null and fx != null:
					fx.start_fade_out(node2)
		MirClient.SM_MISS:
			# SMMiss { u64 UID; } = 8 字节
			if body.size() >= 8 and fx != null:
				var uid3 := body.decode_s64(0)
				var node3 = co_manager.find_uid(uid3)
				if node3 != null:
					fx.on_miss(node3.grid.x, node3.grid.y)


func _wait_sec(sec: float) -> void:
	var t := 0.0
	while t < sec:
		await get_tree().process_frame
		t += get_process_delta_time()


## 支持 --run-test <名称>：在场景内跑自测（Autoload 可用）
func _maybe_run_test() -> void:
	var args := OS.get_cmdline_user_args()
	var idx := args.find("--run-test")
	if idx < 0 or idx + 1 >= args.size():
		return
	var name: String = args[idx + 1]
	if name == "dbg":
		var t0: Node = load("res://tools/_dbg3.gd").new()
		add_child(t0)
		await t0.run(self)
		return
	if name == "c5c6":
		var t5: Node = load("res://tools/c5_c6_scene.gd").new()
		add_child(t5)
		await t5.run(self)
		get_tree().quit(0)
		return
	if name == "c3c4":
		var tc: Node = load("res://tools/c3_c4_scene.gd").new()
		add_child(tc)
		await tc.run(self)
		get_tree().quit(0)
		return
	if name == "ui":
		var tu: Node = load("res://tools/ui_selftest.gd").new()
		add_child(tu)
		await tu.run(self)
		get_tree().quit(0)
		return
	if name == "sync":
		var ts: Node = load("res://tools/sync_selftest.gd").new()
		add_child(ts)
		await ts.run(self)
		get_tree().quit(0)
		return
	if name == "net":
		var tn: Node = load("res://tools/net_selftest.gd").new()
		add_child(tn)
		await tn.run(self)
		get_tree().quit(0)
		return
	if name == "anim":
		var ta: Node = load("res://tools/anim_selftest.gd").new()
		add_child(ta)
		await ta.run(self)
		get_tree().quit(0)
		return
	if name != "move":
		return
	var t: Node = load("res://tools/move_selftest.gd").new()
	add_child(t)
	await t.run(self)


func _create_player() -> void:
	_player = PlayerActor.new()
	add_child(_player)
	_player.setup(map_view, _check_path_grid)
	_player.set_grid(_spawn, PathFinder.DIR_DOWN)
	_player.grid_changed.connect(_on_player_grid_changed)
	cam.position = _player.position

	# 挂人形外观（B9）：裸身站立，验证贴图链路
	_player.hide_placeholder()      # 有真实贴图后不画红方块
	_human = HumanActor.new()
	_player.add_child(_human)
	_human.setup(Game.assets_root)
	# 贴图锚点在脚底，需上移使角色站在格中心
	_human.anchor_offset = Vector2(-10, -48)
	_human.set_appearance(0, 1, 0, 0, false)   # dress=0 裸身, hair=1, 无头盔武器, 女性

	# 绑定动画节点，切到站立动作
	_player.bind_animation(_human)
	_player.set_motion_type(MotionType.MOTION_STAND)
	print("[Main] 人形外观: dress=%d hair=%d 层可见: body0=%s body1=%s head=%s" % [
		_human.dress, _human.hair,
		str(_human.get_node("body0").visible),
		str(_human.get_node("body1").visible),
		str(_human.get_node("head").visible),
	])


## 格子通行判定，对应 ProcessRun::checkPathGrid
## 当前只有地形判定；占用/锁定要等 S7 其他角色接入后补
func _check_path_grid(x: int, y: int) -> int:
	if _map == null:
		return PathFinder.PF_NONE
	if x < 0 or y < 0 or x >= _map.w or y >= _map.h:
		return PathFinder.PF_NONE
	# LAND.canWalk / canFly 任一为真即可通行
	if MapLoader.is_walkable(_map, x, y):
		return PathFinder.PF_FREE
	return PathFinder.PF_OBSTACLE


func _is_walkable(g: Vector2i) -> bool:
	return _check_path_grid(g.x, g.y) == PathFinder.PF_FREE


## 供 CoManager 注入实体动作队列：能否站立于该格
func can_move_to(x: int, y: int) -> bool:
	return _check_path_grid(x, y) == PathFinder.PF_FREE


## 供 CoManager 注入实体动作队列：构造寻路器
func make_path_finder(max_step: int) -> PathFinder:
	return PathFinder.new(_check_path_grid, max_step, max_step)





func _on_player_grid_changed(g: Vector2i) -> void:
	_update_hud()


## 相机每帧跟随主角，移动过程中平滑推进
func _process(_dt: float) -> void:
	if _player != null and cam != null:
		cam.position = _player.position


func _update_hud() -> void:
	var st := TexCache.stats()
	var busy := "移动中" if _player != null and _player.is_moving() else "待命"
	hud.text = "地图 %dx%d | 格(%d,%d) 朝向%d %s | 加载 %dms | 纹理 %d 张 命中 %.0f%%" % [
		_map.w, _map.h, _player.grid.x, _player.grid.y, _player.facing, busy,
		_load_ms, st.cached, st.hit_rate * 100.0
	]


func _unhandled_input(event: InputEvent) -> void:
	if _map == null:
		return
	# UI 优先处理（对应原版 GUIManager::processEventDefault 先于地图）
	if ui != null and ui.handle_input(event):
		return
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		var world: Vector2 = get_global_mouse_position()
		_click_move(map_view.world_to_grid(world))


func _click_move(g: Vector2i) -> void:
	if _map == null:
		return
	if _player.is_moving():
		return
	if g.x < 0 or g.y < 0 or g.x >= _map.w or g.y >= _map.h:
		return
	if not _is_walkable(g):
		hud.text = "格(%d,%d) 不可行走" % [g.x, g.y]
		return
	if not _player.request_move(g):
		hud.text = "格(%d,%d) 无法到达" % [g.x, g.y]


func _find_spawn(center: Vector2i) -> Vector2i:
	for r in range(0, 60):
		for dy in range(-r, r + 1):
			for dx in range(-r, r + 1):
				if absi(dx) != r and absi(dy) != r:
					continue
				var g: Vector2i = center + Vector2i(dx, dy)
				if _is_walkable(g):
					return g
	return center


## 支持 --shot <路径> : 若干帧后截图并退出
func _maybe_autoshot() -> void:
	var args := OS.get_cmdline_user_args()
	var idx := args.find("--shot")
	if idx < 0 or idx + 1 >= args.size():
		return
	var out_path: String = args[idx + 1]
	for i in range(30):
		await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	var err := img.save_png(out_path)
	print("[shot] ", out_path, " err=", err, " ", img.get_width(), "x", img.get_height())
	get_tree().quit(0)
