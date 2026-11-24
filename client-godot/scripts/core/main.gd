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

	_update_hud()
	_maybe_run_test()
	_maybe_autoshot()


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
	if name == "net":
		var tn: Node = load("res://tools/net_selftest.gd").new()
		add_child(tn)
		await tn.run(self)
		return
	if name == "anim":
		var ta: Node = load("res://tools/anim_selftest.gd").new()
		add_child(ta)
		await ta.run(self)
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
