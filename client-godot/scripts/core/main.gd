extends Node2D
## 主场景：加载地图 + 主角点击走路
## 阶段 2 验证入口：不依赖服务端

const GRID_X := 48
const GRID_Y := 32

@onready var map_view: MapView = $MapView
@onready var cam: Camera2D = $Camera2D
@onready var hud: Label = $UI/HUD

var _map: MapLoader.MapData = null
var _map_cache: Dictionary = {}
var _preload_queue: Array[int] = []
var _preload_thread: Thread = null
var _preload_map_id: int = 0
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
var debug_overlay: GfxDebugOverlay = null
## 背包数据（SDInventory）
var _inventory = null

const SdInventory := preload("res://scripts/item/sd_inventory.gd")
const LoginFlowRef := preload("res://scripts/ui/login/login_flow.gd")
const SdMsgParserRef := preload("res://scripts/net/sd_msg_parser.gd")
const WorldSessionRef := preload("res://scripts/core/world_session.gd")
const MapRecordRef := preload("res://scripts/item/map_record.gd")
const MagicDBRef := preload("res://scripts/magic/magic_db.gd")
const GfxDebugOverlayRef := preload("res://scripts/core/gfx_debug_overlay.gd")
const SdMsgParser = SdMsgParserRef

## 登录链路（--login 模式启用）
var login_flow: LoginFlowRef = null
var world: WorldSession = null
var _capture_world_frames := false
var _pending_world_frames: Array = []
## 地图物件层（地面物件/上层物件/地面物品/光照）
var obj_layer: MapObjectLayer = null
## 角色运行时状态（由 SM_HEALTH / SM_GOLD / SM_EXP 等驱动）
var hero_state: Dictionary = {
	"hp": 100, "mp": 50, "max_hp": 100, "max_mp": 50,
	"hp_recover": 0, "mp_recover": 0,
	"gold": 0, "exp": 0,
	"belt": [null, null, null, null, null, null],
	"wear": {},              # wltype -> SDItem
	"magics": [],            # 已学魔法 id
	"buffs": [],             # buff id
	"view_range": 10,
}


func _ready() -> void:
	# --login：从 Logo → 登录 → 选人 → 建号 走完整链路（C9）
	if OS.get_cmdline_user_args().find("--login") >= 0:
		await _start_login_flow()
		_maybe_run_test()
		return

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
	_create_obj_layer()
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
	debug_overlay = GfxDebugOverlayRef.new()
	debug_overlay.name = "GfxDebugOverlay"
	add_child(debug_overlay)
	debug_overlay.setup(_player, co_manager, obj_layer)
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


func _ensure_co_manager() -> void:
	if co_manager != null:
		return
	co_manager = CoManager.new()
	co_manager.name = "CoManager"
	add_child(co_manager)
	co_manager.setup(Game.assets_root)
	co_manager.map_uid = map_uid
	co_manager.set_my_position_provider(func() -> Vector3i:
		return Vector3i(_player.grid.x, _player.grid.y, map_uid) if _player != null else Vector3i.ZERO)
	co_manager.entity_query_requested.connect(func(uid: int):
		if net != null:
			net.query_corecord(uid))
	if _player != null:
		co_manager.set_map_context(Callable(self, "can_move_to"), Callable(self, "make_path_finder"))


func _setup_world_session() -> void:
	if ui == null:
		return
	if world == null:
		world = WorldSessionRef.new()
		world.name = "WorldSession"
		add_child(world)
		world.setup({
			"main": self, "net": net, "ui": ui, "co_manager": co_manager,
			"player": _player, "obj_layer": obj_layer, "fx": fx,
			"inventory": _inventory, "map_uid": map_uid,
		})
		if _player != null and not _player.move_finished.is_connected(world.on_player_move_finished):
			_player.move_finished.connect(world.on_player_move_finished)
	else:
		world.net = net
		world.ui = ui
		world.co_manager = co_manager
		world.player = _player
		world.obj_layer = obj_layer
		world.fx = fx
		world.inventory = _inventory
		world.map_uid = map_uid
		world._bind_ui()
	ui.world = world
	if co_manager != null and not co_manager.map_change_requested.is_connected(world._on_map_change_requested):
		co_manager.map_change_requested.connect(world._on_map_change_requested)
	if ui.ctrl_board != null:
		ui.ctrl_board.player = _player
		ui.ctrl_board.map_id = Uid.get_map_id(map_uid) if map_uid != 0 else 1


func apply_server_scene(d: Dictionary) -> void:
	var target_map_uid := int(d.get("map_uid", map_uid))
	var target_map_id := Uid.get_map_id(target_map_uid)
	if target_map_id <= 0:
		target_map_id = Uid.get_map_id(map_uid)
	if target_map_id <= 0:
		target_map_id = 1
	var need_reload := (_map == null or _player == null or map_uid != target_map_uid)
	map_uid = target_map_uid
	if need_reload:
		var path := MapRecordRef.path_for(target_map_id)
		if not FileAccess.file_exists(path):
			path = "%s/map/%d.bin" % [Game.assets_root, target_map_id - 1]
		var md = _map_cache.get(target_map_id)
		if md == null:
			md = MapLoader.load_file(path)
			if md != null:
				_map_cache[target_map_id] = md
		if md == null:
			push_error("地图加载失败: " + path)
			return
		_map = md
		map_view.set_camera(cam)
		map_view.load_map(_map)
		if obj_layer != null:
			obj_layer.queue_free()
			obj_layer = null
		_create_obj_layer()
		if _player != null:
			_player.queue_free()
			_player = null
			_human = null
		_spawn = _find_spawn(Vector2i(_map.w / 2, _map.h / 2))
		_create_player()
	if _inventory == null:
		_init_inventory()
	if fx == null:
		_create_fx()
	_ensure_co_manager()
	if ui == null:
		_create_ui()
	else:
		ui.player_actor = _player
		ui.co_manager = co_manager
		ui.inventory_data = _inventory
	_setup_world_session()
	if debug_overlay != null:
		debug_overlay.setup(_player, co_manager, obj_layer)
	if world != null:
		world.net = net
		world.player = _player
		world.co_manager = co_manager
		world.obj_layer = obj_layer
		world.map_uid = map_uid
		if _player != null and not _player.move_finished.is_connected(world.on_player_move_finished):
			_player.move_finished.connect(world.on_player_move_finished)
	if co_manager != null:
		co_manager.map_uid = map_uid
		co_manager.set_my_uid(Game.self_uid)
		co_manager.set_map_context(Callable(self, "can_move_to"), Callable(self, "make_path_finder"))
	if ui != null and ui.ctrl_board != null:
		ui.ctrl_board.player = _player
		ui.ctrl_board.map_id = target_map_id
	if map_view != null:
		map_view.visible = true
	if hud != null:
		hud.visible = true
	_update_hud()
	_queue_neighbor_preloads(target_map_id)


func _queue_neighbor_preloads(current_map_id: int) -> void:
	_preload_queue.clear()
	for sw in MapRecordRef.switches_of(current_map_id):
		var nid := MapRecordRef.find_id_by_name(str(sw.get("endName", "")))
		if nid > 0 and nid != current_map_id and not _map_cache.has(nid) and not _preload_queue.has(nid):
			_preload_queue.append(nid)


func _process_preload() -> void:
	if _preload_thread != null and not _preload_thread.is_alive():
		_preload_thread.wait_to_finish()
		_preload_thread = null
	if _preload_thread == null and not _preload_queue.is_empty():
		var nid: int = _preload_queue.pop_front()
		if _map_cache.has(nid):
			return
		_preload_map_id = nid
		var path := MapRecordRef.path_for(nid)
		_preload_thread = Thread.new()
		_preload_thread.start(func():
			var md = MapLoader.load_file(path)
			call_deferred("_finish_preload", nid, md))


func _finish_preload(mid: int, md) -> void:
	if md != null:
		_map_cache[mid] = md


func _capture_world_frame(code: int, body: PackedByteArray, resp: int) -> void:
	if world != null:
		return
	_pending_world_frames.append([code, body, resp])


func _enter_world_from_online(online_body: PackedByteArray) -> void:
	var od := SdMsgParser.parse_online_ok(online_body)
	if od.is_empty():
		od = {"uid": 0, "name": "玩家", "gender": 0, "job": 1,
			"map_uid": Uid.build_map_uid(1, 1), "action": null}
	if int(od.get("uid", 0)) != 0:
		Game.set_self(int(od["uid"]), str(od.get("name", "")))
	map_uid = int(od.get("map_uid", Uid.build_map_uid(1, 1)))
	apply_server_scene(od)
	if world != null:
		world.bind_net(net)
		world.apply_start_scene(od)
	if net != null:
		if net.frame_received.is_connected(_capture_world_frame):
			net.frame_received.disconnect(_capture_world_frame)
		_capture_world_frames = false
		if not net.frame_received.is_connected(_on_net_frame):
			net.frame_received.connect(_on_net_frame)
		var pending := _pending_world_frames.duplicate()
		_pending_world_frames.clear()
		for frame in pending:
			_on_net_frame(int(frame[0]), frame[1], int(frame[2]))


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

	_ensure_co_manager()

	net = MirClient.new()
	net.name = "Net"
	add_child(net)
	net.frame_received.connect(_on_net_frame)
	_setup_world_session()
	if world != null:
		world.bind_net(net)

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
	# 先处理「角色自身状态」一类，它们不依赖场景实体管理器
	if _on_hero_state_frame(code, body):
		return
	if world != null and world.handle_frame(code, body):
		return
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
	if name == "c9c8":
		var t9b: Node = load("res://tools/c9net_scene.gd").new()
		add_child(t9b)
		await t9b.run(self)
		return
	if name == "c7c8":
		var t7: Node = load("res://tools/c7_c8_scene.gd").new()
		add_child(t7)
		await t7.run(self)
		get_tree().quit(0)
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


## 创建地图物件层（对应 processrun.cpp draw 的物件分批绘制）
func _create_obj_layer() -> void:
	obj_layer = MapObjectLayer.new()
	obj_layer.name = "ObjectLayer"
	add_child(obj_layer)
	obj_layer.setup(_map, cam)
	# 物件贴图与瓦片同源：texture/map/{texID:08X}.PNG
	obj_layer.obj_tex_fn = func(tid: int) -> Texture2D:
		return TexCache.get_texture("%s/texture/map/%08X.PNG" % [Game.assets_root, tid])
	obj_layer.item_icon_fn = func(iid: int) -> Texture2D:
		return ItemGfx.icon_of_item(iid)
	print("[Main] 物件层已创建，g_ground=%d g_over0=%d g_over1=%d 光照格=%d" % [
		_map.obj_count_for_depth(MapLoader.OBJD_GROUND),
		_map.obj_count_for_depth(MapLoader.OBJD_OVERGROUND0),
		_map.obj_count_for_depth(MapLoader.OBJD_OVERGROUND1),
		_map.light_count()])


func _create_player() -> void:
	_player = PlayerActor.new()
	# 玩家与 co_manager 里的实体分属不同父节点，各自 y_sort；
	# 两者同处 main 下，main 也启用 y_sort 让它们互相正确遮挡
	y_sort_enabled = true
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
	_process_preload()
	if login_flow != null and login_flow.in_login_flow():
		return
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
	if login_flow != null and login_flow.in_login_flow():
		login_flow.handle_input(event)
		return
	if _map == null:
		return
	if ui != null and ui.handle_input(event):
		return
	if event is InputEventMouseMotion:
		var mg := map_view.world_to_grid(get_global_mouse_position())
		if debug_overlay != null and world != null:
			var info: Dictionary = world.describe_cell(mg)
			debug_overlay.set_hover(mg, info.get("hit"), info.get("items", []))
		return
	if event is InputEventMouseButton and event.pressed:
		var gp: Vector2 = get_global_mouse_position()
		var g := map_view.world_to_grid(gp)
		if event.button_index == MOUSE_BUTTON_RIGHT:
			_click_move(g)
		elif event.button_index == MOUSE_BUTTON_LEFT:
			if world != null and world.handle_world_click(g, event.shift_pressed):
				return
			_click_move(g)
		return
	if event is InputEventKey and event.pressed and not event.echo:
		if event.keycode >= KEY_1 and event.keycode <= KEY_6:
			if world != null:
				world.on_quick_slot(int(event.keycode - KEY_1))
			return
		if event.keycode == KEY_TAB:
			if world != null:
				world.request_nearest_pickup()
			return
		if event.keycode == KEY_F3:
			if debug_overlay != null:
				debug_overlay.toggle()
			return
		if event.keycode == KEY_ESCAPE:
			if _player != null:
				cam.position = _player.position
			return


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
	if not _player.request_move(g, 2):
		hud.text = "格(%d,%d) 无法到达" % [g.x, g.y]
		return
	if world != null:
		world.send_move(g)


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


# ==================== C9 登录链路 ====================

## --login：从 Logo 开始走完整登录链路
## 这是复刻 client.cpp 的进程状态机：LOGO → LOGIN → SELECTCHAR → RUN
## 内含 await，调用方必须用 await
func _start_login_flow() -> void:
	# 登录态不需要地图，先把渲染层与 HUD 收起来
	if map_view != null:
		map_view.visible = false
	if hud != null:
		hud.visible = false

	login_flow = LoginFlowRef.new()
	login_flow.name = "LoginFlow"
	add_child(login_flow)

	# 建网络对象并连服务端（默认 127.0.0.1:7000，mock 服务端）
	net = MirClient.new()
	net.name = "LoginNet"
	add_child(net)
	_capture_world_frames = true
	_pending_world_frames.clear()
	net.frame_received.connect(_capture_world_frame)
	login_flow.bind_net(net)

	var host := "127.0.0.1"
	var port := 7000
	var args := OS.get_cmdline_user_args()
	var hp := args.find("--server")
	if hp >= 0 and hp + 1 < args.size():
		var parts: PackedStringArray = args[hp + 1].split(":")
		host = parts[0]
		if parts.size() > 1:
			port = int(parts[1])

	var ok: bool = await net.connect_to(host, port)
	print("[C9] 登录链路启动，连服务端 %s:%d -> %s" % [host, port, str(ok)])

	# 进游戏后交棒
	login_flow.run_entered.connect(_on_run_entered)
	login_flow.conn_error.connect(func(reason): print("[C9] 连接错误: ", reason))


## SM_ONLINEOK 到达后接管进游戏
func _on_run_entered(online_body: PackedByteArray) -> void:
	print("[C9] 进入游戏：SM_ONLINEOK %d 字节" % online_body.size())
	if login_flow != null:
		login_flow.visible = false
		if login_flow._canvas != null:
			login_flow._canvas.visible = false
	if hud != null:
		hud.visible = true
	if map_view != null:
		map_view.visible = true
	_enter_world_from_online(online_body)


# ==================== 角色状态消息（processrunnet.cpp on_SM_*）====================

## 处理角色自身状态类消息，返回是否已消费
## 这些对应 processrunnet.cpp 里「不涉及场景实体增删」的那批处理器
func _on_hero_state_frame(code: int, body: PackedByteArray) -> bool:
	match code:
		MirClient.SM_HEALTH:
			var h := SdMsgParser.parse_health(body)
			if h.is_empty():
				return true
			hero_state["hp"] = int(h["hp"])
			hero_state["mp"] = int(h["mp"])
			hero_state["max_hp"] = int(h["max_hp"])
			hero_state["max_mp"] = int(h["max_mp"])
			hero_state["hp_recover"] = int(h["hp_recover"])
			hero_state["mp_recover"] = int(h["mp_recover"])
			if int(h.get("uid", 0)) != Game.self_uid and co_manager != null:
				co_manager.set_actor_health(int(h["uid"]), int(h["hp"]), int(h["max_hp"]))
			_sync_hud_bars()
			return true

		MirClient.SM_GOLD:
			var g := SdMsgParser.parse_gold(body)
			if g.is_empty():
				return true
			var last: int = int(hero_state["gold"])
			var now: int = int(g["gold"])
			hero_state["gold"] = now
			# processrunnet.cpp:695-700 的差异日志
			if last < now:
				_push_log("你获得了%d金币" % (now - last))
			elif last > now:
				_push_log("你失去了%d金币" % (last - now))
			return true

		MirClient.SM_EXP:
			var e := SdMsgParser.parse_exp(body)
			if e.is_empty():
				return true
			var last_exp: int = int(hero_state["exp"])
			var new_exp: int = int(e["exp"])
			hero_state["exp"] = new_exp
			if new_exp > last_exp and last_exp > 0:
				_push_log("你获得了经验值%d" % (new_exp - last_exp))
			return true

		MirClient.SM_INVENTORY:
			var inv := SdMsgParser.parse_inventory(body)
			if inv.is_empty():
				return true
			if _inventory != null:
				_inventory.set_items(inv["items"])
			_refresh_inventory_ui()
			return true

		MirClient.SM_BELT:
			var belt := SdMsgParser.parse_belt(body)
			hero_state["belt"] = belt["items"]
			if ui != null and ui.quick_access != null:
				ui.quick_access.set_belt(belt["items"])
			return true

		MirClient.SM_EQUIPWEAR:
			var ew := SdMsgParser.parse_equip_wear(body)
			if ew.get("item") != null:
				hero_state["wear"][int(ew["wltype"])] = ew["item"]
			else:
				hero_state["wear"].erase(int(ew["wltype"]))
			if ui != null and ui.my_state_board != null:
				ui.my_state_board.wear = hero_state["wear"]
			if world != null:
				world._refresh_player_appearance({"wear": hero_state["wear"], "gender": world.gender})
			return true

		MirClient.SM_GRABWEAR:
			var gw := SdMsgParser.parse_grab_wear(body)
			hero_state["wear"].erase(int(gw["wltype"]))
			if ui != null and ui.my_state_board != null:
				ui.my_state_board.wear = hero_state["wear"]
			if world != null:
				world._refresh_player_appearance({"wear": hero_state["wear"], "gender": world.gender})
			return true

		MirClient.SM_EQUIPBELT:
			var eb := SdMsgParser.parse_equip_belt(body)
			hero_state["belt"][int(eb["slot"])] = eb["item"]
			if ui != null and ui.quick_access != null:
				ui.quick_access.set_belt(hero_state["belt"])
			return true

		MirClient.SM_GRABBELT:
			var gb := SdMsgParser.parse_grab_belt(body)
			hero_state["belt"][int(gb["slot"])] = null
			if ui != null and ui.quick_access != null:
				ui.quick_access.set_belt(hero_state["belt"])
			return true

		MirClient.SM_UPDATEITEM:
			var ui_item := SdMsgParser.parse_update_item(body)
			if _inventory != null and ui_item.has("item"):
				var found = _inventory.find(int(ui_item["item_id"]), int(ui_item["seq_id"]))
				if found != null:
					found.count = int(ui_item["count"])
					found.duration = ui_item["item"].duration
			_refresh_inventory_ui()
			return true

		MirClient.SM_REMOVEITEM:
			var ri := SdMsgParser.parse_remove_item(body)
			if _inventory != null:
				_inventory.remove(int(ri["item_id"]), int(ri["seq_id"]), int(ri["count"]), true)
			_refresh_inventory_ui()
			return true

		MirClient.SM_REMOVESECUREDITEM:
			var rsi := SdMsgParser.parse_remove_secured_item(body)
			_push_log("寄存物品已移除：%d/%d" % [int(rsi["item_id"]), int(rsi["seq_id"])])
			return true

		MirClient.SM_LEARNEDMAGICLIST:
			var lm := SdMsgParser.parse_learned_magic(body)
			hero_state["magics"] = lm["magics"]
			for mid in hero_state["magics"]:
				var nm := MagicDBRef.name_of(int(mid))
				if not nm.is_empty() and ui != null and ui.skill_board != null:
					ui.skill_board.learn(nm)
			return true

		MirClient.SM_BUFFIDLIST:
			var bl := SdMsgParser.parse_buff_id_list(body)
			hero_state["buffs"] = bl["ids"]
			return true

		MirClient.SM_VIEWRANGE:
			var vr := SdMsgParser.parse_view_range(body)
			hero_state["view_range"] = int(vr["range"])
			return true

		MirClient.SM_GROUNDITEMIDLIST:
			var gl := SdMsgParser.parse_ground_item_list(body)
			if gl.is_empty():
				return true
			# 只处理当前地图（processrunnet.cpp:469 的 mapUID 校验）
			if int(gl["map_uid"]) != map_uid and map_uid != 0:
				return true
			if obj_layer != null:
				obj_layer.clear_ground_items()
				for cell in gl["cells"]:
					for iid in cell["items"]:
						obj_layer.add_ground_item(int(cell["x"]), int(cell["y"]),
							int(iid), 1, false)
			return true

		MirClient.SM_PICKUPERROR:
			# SMPickUpError { u32 error; }
			_push_log("无法拾取（错误码 %d）" % SdMsgParser.u32(body, 0))
			return true

		MirClient.SM_TEXT, MirClient.SM_PARTEXT:
			# 裸字符串 body（processrunnet.cpp:738-746）
			_push_log(SdMsgParser.parse_raw_text(body))
			return true

		MirClient.SM_PLAYERSAY:
			var ps := SdMsgParser.parse_player_say(body)
			if ps.is_empty():
				return true
			# 别人说话才进聊天框（原版 :112 的 uid 判断）
			if not ps["content"].is_empty():
				_push_log(str(ps["content"]))
			return true

		MirClient.SM_PLAYERBROADCAST:
			var pb := SdMsgParser.parse_player_say(body)
			if pb.is_empty():
				return true
			_push_log("[广播] " + str(pb["content"]))
			return true

		MirClient.SM_STRIKEGRID:
			# 打击格高亮（原版用 m_strikeGridList 记时间戳后短暂描边）
			var sg := SdMsgParser.parse_strike_grid(body)
			if sg.is_empty() or fx == null:
				return true
			fx.on_miss(int(sg["x"]), int(sg["y"]))
			return true

		MirClient.SM_NPCNAME:
			var nn := SdMsgParser.parse_npc_name(body)
			if co_manager != null:
				co_manager.set_actor_name(int(nn.get("uid", 0)), str(nn.get("name", "")))
			if ui != null and ui.ctrl_board != null and not str(nn["name"]).is_empty():
				ui.ctrl_board.add_log("NPC: " + str(nn["name"]))
			return true

		MirClient.SM_NPCXMLLAYOUT:
			var nx := SdMsgParser.parse_npc_xml(body)
			if ui != null and ui.npc_chat != null and not str(nx["xml"]).is_empty():
				ui.npc_chat.show_layout(int(nx["uid"]), str(nx["xml"]), str(nx.get("event_path", "")))
			return true

		MirClient.SM_LOGOUTOK:
			_push_log("已退出到选人界面")
			return true
	return false


## 把状态同步到 HUD 与各面板
func _sync_hud_bars() -> void:
	if _player != null:
		# 控制栏与头顶板都读这两个比例
		_player.set("hp_ratio", float(hero_state["hp"]) / maxf(1.0, float(hero_state["max_hp"])))
		_player.set("mp_ratio", float(hero_state["mp"]) / maxf(1.0, float(hero_state["max_mp"])))
		_player.set("level", int(hero_state.get("level", 1)))


func _refresh_inventory_ui() -> void:
	if ui != null and ui.inventory_board != null:
		ui.inventory_board.queue_redraw()


func _push_log(text: String) -> void:
	if ui != null:
		ui._push_sys_msg(text)
	elif ui != null and ui.ctrl_board != null:
		ui.ctrl_board.add_log(text)
	else:
		print("[log] ", text)
