extends Node2D
class_name MapView
## 地图渲染：视野裁剪 + 瓦片复用
##
## 坐标体系（关键）：
##   1 个地图格 = GRID_X(48) × GRID_Y(32) 像素
##   1 个 BLOCK 覆盖 2×2 格
##   1 张瓦片恰好绘制 1 个 BLOCK 的区域
##
## 素材分辨率陷阱：
##   原版 mir2map.hpp:167 要求瓦片 == 48×32，但 mir2x_assets-main 的
##   texture/map/*.PNG 实测是 96×64（2 倍高清导出）。
##   因此必须按 tile_scale 缩放，否则瓦片会盖住 2×2 格造成棋盘错位。

const GRID_X := 48
const GRID_Y := 32
const BLOCK_W := 2
const BLOCK_H := 2

## 瓦片像素 / BLOCK 像素。
## 素材实测 96×64，BLOCK = 2 格 × (48,32) = (96,64)，两者相等，故取 1.0。
@export var tile_scale: float = 1.0

## 视野格数（含边界冗余）
@export var view_w: int = 44
@export var view_h: int = 28

var map_data: MapLoader.MapData = null

var _sprites: Array[Sprite2D] = []
var _cam: Camera2D = null
var _last_block := Vector2i(-99999, -99999)


func _ready() -> void:
	z_index = -100
	var total := (view_w / BLOCK_W + 3) * (view_h / BLOCK_H + 3)
	_sprites.resize(total)
	for i in range(total):
		var s := Sprite2D.new()
		s.centered = false
		s.texture_filter = CanvasItem.TEXTURE_FILTER_NEAREST
		s.scale = Vector2(tile_scale, tile_scale)
		s.visible = false
		add_child(s)
		_sprites[i] = s


func set_camera(cam: Camera2D) -> void:
	_cam = cam
	_last_block = Vector2i(-99999, -99999)


func load_map(md: MapLoader.MapData) -> void:
	map_data = md
	_last_block = Vector2i(-99999, -99999)
	refresh()


func _process(_dt: float) -> void:
	if map_data == null or _cam == null:
		return
	var b := _cam_block()
	if b == _last_block:
		return
	_last_block = b
	refresh()


func _cam_block() -> Vector2i:
	var c := _cam.get_screen_center_position()
	return Vector2i(int(c.x / (GRID_X * BLOCK_W)), int(c.y / (GRID_Y * BLOCK_H)))


func refresh() -> void:
	if map_data == null or _cam == null:
		return

	var cb := _cam_block()
	var half_w := view_w / BLOCK_W / 2
	var half_h := view_h / BLOCK_H / 2

	var idx := 0
	for dy in range(-half_h - 1, half_h + 2):
		for dx in range(-half_w - 1, half_w + 2):
			if idx >= _sprites.size():
				break
			var bx := cb.x + dx
			var by := cb.y + dy
			var s := _sprites[idx]
			idx += 1

			var gx := bx * BLOCK_W
			var gy := by * BLOCK_H
			var tex_id := map_data.tile_tex_id(gx, gy)
			if tex_id < 0:
				s.visible = false
				continue

			var tex := TexCache.get_texture(_tex_path(tex_id))
			if tex == null:
				s.visible = false
				continue
			s.texture = tex
			s.position = Vector2(gx * GRID_X, gy * GRID_Y)
			s.visible = true

	for i in range(idx, _sprites.size()):
		_sprites[i].visible = false


func _tex_path(tex_id: int) -> String:
	return "%s/texture/map/%08X.PNG" % [Game.assets_root, tex_id]


func world_to_grid(p: Vector2) -> Vector2i:
	return Vector2i(int(p.x / GRID_X), int(p.y / GRID_Y))


func grid_to_world(g: Vector2i) -> Vector2:
	return Vector2(g.x * GRID_X + GRID_X / 2, g.y * GRID_Y + GRID_Y / 2)


func is_walkable(g: Vector2i) -> bool:
	if map_data == null:
		return false
	return MapLoader.is_walkable(map_data, g.x, g.y)
