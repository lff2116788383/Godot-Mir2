extends Node2D
## 主场景：加载一张地图并渲染，支持点击走路
## 阶段 2 的验证入口：不依赖服务端

const GRID_X := 48
const GRID_Y := 32

@onready var map_view: MapView = $MapView
@onready var cam: Camera2D = $Camera2D
@onready var hud: Label = $UI/HUD

var _map: MapLoader.MapData = null
var _player_grid := Vector2i.ZERO
var _player_node: Node2D = null
var _spawn := Vector2i.ZERO


func _ready() -> void:
	map_view.set_camera(cam)
	# 按 maprecord.inc 第 1 条：比奇县_0 -> 0.bin
	var path := "%s/map/0.bin" % Game.assets_root
	var t0 := Time.get_ticks_msec()
	_map = MapLoader.load_file(path)
	var t1 := Time.get_ticks_msec()

	if _map == null:
		hud.text = "地图加载失败: " + path
		push_error(hud.text)
		return

	# 找一个可行走的出生点：地图中心附近螺旋搜索
	_spawn = _find_spawn(Vector2i(_map.w / 2, _map.h / 2))
	_player_grid = _spawn
	map_view.load_map(_map)
	_create_player()
	_update_camera()
	_update_hud(t1 - t0)
	print("[Main] 地图 %dx%d 格, 块=%d, 解析 %d ms" % [_map.w, _map.h, _map.tiles.size(), t1 - t0])
	print("[Main] 出生点 = (%d,%d) 可走=%s" % [_spawn.x, _spawn.y, str(map_view.is_walkable(_spawn))])
	# 统计有效瓦片
	var valid := 0
	for t in _map.tiles:
		if t >= 0:
			valid += 1
	print("[Main] 有效瓦片块=%d / %d (%.1f%%)" % [valid, _map.tiles.size(), 100.0 * valid / _map.tiles.size()])
	await get_tree().process_frame
	await get_tree().process_frame
	var st := TexCache.stats()
	print("[Main] 纹理解析后: %s" % str(st))

	_maybe_autoshot()


## 支持 --shot <路径> : 若干帧后截图并退出（用于自动化验证）
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


func _find_spawn(center: Vector2i) -> Vector2i:
	for r in range(0, 60):
		for dy in range(-r, r + 1):
			for dx in range(-r, r + 1):
				if absi(dx) != r and absi(dy) != r:
					continue
				var g := center + Vector2i(dx, dy)
				if map_view.is_walkable(g) or MapLoader.is_walkable(_map, g.x, g.y):
					return g
	return center


func _create_player() -> void:
	_player_node = Node2D.new()
	var dot := Polygon2D.new()
	dot.polygon = PackedVector2Array([
		Vector2(-7, 6), Vector2(7, 6), Vector2(7, -6), Vector2(-7, -6)
	])
	dot.color = Color(0.95, 0.35, 0.25)
	_player_node.add_child(dot)
	add_child(_player_node)
	_player_node.position = map_view.grid_to_world(_player_grid)


func _update_camera() -> void:
	cam.position = map_view.grid_to_world(_player_grid)


func _update_hud(load_ms: int) -> void:
	var st := TexCache.stats()
	hud.text = "地图 %dx%d | 格(%d,%d) | 加载 %d ms | 纹理缓存 %d 张 | 命中率 %.1f%%" % [
		_map.w, _map.h, _player_grid.x, _player_grid.y, load_ms,
		st.cached, st.hit_rate * 100.0
	]


func _unhandled_input(event: InputEvent) -> void:
	if _map == null:
		return
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		var world: Vector2 = get_global_mouse_position()
		var g := map_view.world_to_grid(world)
		_try_move_to(g)


func _try_move_to(g: Vector2i) -> void:
	if _map == null:
		return
	if g.x < 0 or g.y < 0 or g.x >= _map.w or g.y >= _map.h:
		return
	if not map_view.is_walkable(g):
		hud.text = "格(%d,%d) 不可行走" % [g.x, g.y]
		return
	_player_grid = g
	_player_node.position = map_view.grid_to_world(g)
	_update_camera()
	_update_hud(0)
