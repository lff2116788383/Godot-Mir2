extends Node2D
class_name MapObjectLayer
## 地图物件层（地面物件 / 上层物件 / 地面物品 / 光照）
##
## 【逐行翻译】client/src/processrun.cpp:295-425 的绘制顺序
##
## ── 为什么单独一个节点 ──
##   瓦片层用 Sprite2D 池化（只有换块时才更新纹理，开销低）；
##   物件层每帧都要动（动画帧推进、地面物品浮动），
##   用 Node2D 的 _draw 即时绘制更合适。
##
## ── z_index 约定（对应原版"物件分两批、角色夹中间"）──
##   GROUND 地面物件     : -90   （瓦片 -100 之上，角色之下）
##   地面物品            : -85
##   角色                :   0   （由 PlayerActor 等使用默认 z）
##   OVERGROUND0 上层0   :  10   （低于玩家的物件，其实仍在角色之上）
##   OVERGROUND1 上层1   :  20   （树冠、屋檐，压住一切）
##   光照                :  30
##
##   注意原版 OVERGROUND0/1 都在角色之后画，所以都是压角色的，
##   区别只是"低于玩家"和"高于玩家"两批的相对顺序（0 在 1 下面）。

const ObjRef := preload("res://scripts/data/map_objects.gd")
## Node2D -> UiCanvas 风格的绘制适配器
## map_objects.gd 里的绘制函数是按 UiCanvas 接口写的（tex_draw / draw_border /
## draw_circle_c），但这里是裸 Node2D。用一个内联适配对象转接，
## 避免为了物件层再去改一遍 UiCanvas。
const DrawAdapterRef := preload("res://scripts/data/draw_adapter.gd")
const GRID_X := 48
const GRID_Y := 32

const Z_GROUND := -90
const Z_GROUND_ITEM := -85
const Z_OVER0 := 10
const Z_OVER1 := 20
const Z_LIGHT := 30

## 当前地图
var map_data: MapLoader.MapData = null
## 相机
var _cam: Camera2D = null
## 视野（格）
var view_w := 44
var view_h := 28

## 地面物品：[{ gx, gy, item_id, count, highlighted }]
var ground_items: Array = []

## 物件贴图提供者（texID -> Texture2D）
var obj_tex_fn: Callable = Callable()
## 物品图标提供者
var item_icon_fn: Callable = Callable()

## 分层的子节点
var _layer_ground: Node2D = null
var _layer_item: Node2D = null
var _layer_over0: Node2D = null
var _layer_over1: Node2D = null
var _layer_light: Node2D = null

var _now_ms := 0.0
## 统计（自检用）
var drawn_ground := 0
var drawn_over0 := 0
var drawn_over1 := 0
var drawn_items := 0
var drawn_lights := 0


func _ready() -> void:
	_layer_ground = _make_layer("ObjGround", Z_GROUND)
	_layer_item = _make_layer("GroundItems", Z_GROUND_ITEM)
	_layer_over0 = _make_layer("ObjOver0", Z_OVER0)
	_layer_over1 = _make_layer("ObjOver1", Z_OVER1)
	_layer_light = _make_layer("MapLight", Z_LIGHT)


func _make_layer(nm: String, z: int) -> Node2D:
	var n := Node2D.new()
	n.name = nm
	n.z_index = z
	n.draw.connect(_on_layer_draw.bind(n))
	add_child(n)
	# 记录每个 layer 的绘制适配器
	_adapters[n] = DrawAdapterRef.new(n)
	return n


## layer -> 适配器
var _adapters: Dictionary = {}


func setup(md: MapLoader.MapData, cam: Camera2D) -> void:
	map_data = md
	_cam = cam


func _process(dt: float) -> void:
	ObjRef.tick(dt)
	_now_ms += dt * 1000.0
	# 物件会动（动画帧 + 物品浮动），每帧都要重画
	for n in [_layer_ground, _layer_item, _layer_over0, _layer_over1, _layer_light]:
		n.queue_redraw()


## 视口矩形（格坐标）
func _view_grid_rect() -> Array:
	if _cam == null:
		return [0, 0, 0, 0]
	var c := _cam.get_screen_center_position()
	var vp := get_viewport().get_visible_rect().size
	var zoom: float = _cam.zoom.x if _cam.zoom.x != 0.0 else 1.0
	var half_w: float = vp.x / zoom * 0.5
	var half_h: float = vp.y / zoom * 0.5
	var x0: int = int(floor((c.x - half_w) / GRID_X)) - 2
	var y0: int = int(floor((c.y - half_h) / GRID_Y)) - 2
	var x1: int = int(ceil((c.x + half_w) / GRID_X)) + 2
	var y1: int = int(ceil((c.y + half_h) / GRID_Y)) + 3
	return [x0, y0, x1, y1]


## 视口左上角的像素偏移（物件绘制要减去它）
func _view_px() -> Vector2i:
	if _cam == null:
		return Vector2i.ZERO
	var vp := get_viewport().get_visible_rect().size
	var zoom: float = _cam.zoom.x if _cam.zoom.x != 0.0 else 1.0
	var c := _cam.get_screen_center_position()
	return Vector2i(
		int(c.x - vp.x / zoom * 0.5),
		int(c.y - vp.y / zoom * 0.5))


func _clamp_rect(r: Array) -> Array:
	if map_data == null:
		return [0, 0, -1, -1]
	var x0: int = maxi(0, int(r[0]))
	var y0: int = maxi(0, int(r[1]))
	var x1: int = mini(map_data.w - 1, int(r[2]))
	var y1: int = mini(map_data.h - 1, int(r[3]))
	return [x0, y0, x1, y1]


func _on_layer_draw(layer: Node2D) -> void:
	if map_data == null or _cam == null:
		return
	var r := _clamp_rect(_view_grid_rect())
	if int(r[2]) < int(r[0]) or int(r[3]) < int(r[1]):
		return
	var view := _view_px()
	var tex_fn := obj_tex_fn
	if not tex_fn.is_valid():
		return

	var canvas = _adapters.get(layer, null)
	if canvas == null:
		return

	if layer == _layer_ground:
		drawn_ground = ObjRef.draw_layer(canvas, map_data,
			int(r[0]), int(r[1]), int(r[2]), int(r[3]), view,
			MapLoader.OBJD_GROUND, tex_fn)
	elif layer == _layer_over0:
		drawn_over0 = ObjRef.draw_layer(canvas, map_data,
			int(r[0]), int(r[1]), int(r[2]), int(r[3]), view,
			MapLoader.OBJD_OVERGROUND0, tex_fn)
	elif layer == _layer_over1:
		drawn_over1 = ObjRef.draw_layer(canvas, map_data,
			int(r[0]), int(r[1]), int(r[2]), int(r[3]), view,
			MapLoader.OBJD_OVERGROUND1, tex_fn)
	elif layer == _layer_item:
		if item_icon_fn.is_valid():
			drawn_items = ObjRef.draw_ground_items(canvas, ground_items, view,
				item_icon_fn, _now_ms)
	elif layer == _layer_light:
		drawn_lights = ObjRef.draw_light(canvas, map_data,
			int(r[0]), int(r[1]), int(r[2]), int(r[3]), view)


func add_ground_item(gx: int, gy: int, item_id: int, count: int = 1,
		highlighted: bool = false) -> void:
	ground_items.append({
		"gx": gx, "gy": gy, "item_id": item_id,
		"count": count, "highlighted": highlighted,
	})


func clear_ground_items() -> void:
	ground_items.clear()


func stats() -> Dictionary:
	return {
		"ground": drawn_ground, "over0": drawn_over0, "over1": drawn_over1,
		"items": drawn_items, "lights": drawn_lights,
		"ground_list": ground_items.size(),
	}