extends RefCounted
class_name MiniMap
## 小地图坐标换算与缩放
##
## 【逐行翻译】client/src/minimapboard.cpp:514-562
## 对照：
##   - minimapboard.cpp:514  getMiniMapTexture  查 miniMapID
##   - minimapboard.cpp:521  zoomOnCanvasAt     以光标为锚点缩放
##   - minimapboard.cpp:543  fixMapImagePLoc    边界修正（防拖出画布）
##
## 说明：小地图贴图是一张整图（miniMapID 指向 texture/proguse 里的一张图），
## 不是瓦片拼接。所以这里只有「缩放 + 平移 + 边界夹取」三件事。

const SysConstRef10 := preload("res://scripts/core/sys_const.gd")

## 缩放范围（minimapboard.cpp:532）
const ZOOM_MIN := 0.1
const ZOOM_MAX := 10.0

var zoom_factor: float = 1.0
var auto_center: bool = true

## 画布尺寸
var canvas_w: int = 0
var canvas_h: int = 0

## 地图图像尺寸
var map_w: int = 0
var map_h: int = 0

## 图像左上角在画布内的位置
var map_dx: int = 0
var map_dy: int = 0


func setup(cw: int, ch: int, mw: int, mh: int) -> void:
	canvas_w = cw
	canvas_h = ch
	map_w = mw
	map_h = mh
	if auto_center:
		_fix_loc(true)


## 画布坐标 -> 图像坐标（对应 onMapImagePLoc_from_onCanvasPLoc）
func canvas_to_image(px: int, py: int) -> Vector2i:
	return Vector2i(px - map_dx, py - map_dy)


## 图像坐标 -> 画布坐标
func image_to_canvas(px: int, py: int) -> Vector2i:
	return Vector2i(px + map_dx, py + map_dy)


## 图像坐标 -> 地图格坐标（小地图按比例映射）
func image_to_grid(px: int, py: int, map_grid_w: int, map_grid_h: int) -> Vector2i:
	if map_w <= 0 or map_h <= 0:
		return Vector2i.ZERO
	return Vector2i(
		int(float(px) / float(map_w) * float(map_grid_w)),
		int(float(py) / float(map_h) * float(map_grid_h))
	)


## 地图格坐标 -> 图像坐标
func grid_to_image(gx: int, gy: int, map_grid_w: int, map_grid_h: int) -> Vector2i:
	if map_grid_w <= 0 or map_grid_h <= 0:
		return Vector2i.ZERO
	return Vector2i(
		int(float(gx) / float(map_grid_w) * float(map_w)),
		int(float(gy) / float(map_grid_h) * float(map_h))
	)


## 地图格坐标 -> 画布坐标（画玩家/怪物点位用）
func grid_to_canvas(gx: int, gy: int, map_grid_w: int, map_grid_h: int) -> Vector2i:
	var p := grid_to_image(gx, gy, map_grid_w, map_grid_h)
	return image_to_canvas(p.x, p.y)


## zoomOnCanvasAt（minimapboard.cpp:521）
## 以画布上某点为锚缩放：缩放前后该点对应的图像位置保持不变
func zoom_on_canvas_at(on_px: int, on_py: int, new_zoom: float) -> void:
	var old_dx := map_dx
	var old_dy := map_dy
	var old_img := canvas_to_image(on_px, on_py)

	# 记录相对位置比例
	var rx := 0.0 if map_w <= 0 else float(old_img.x) / float(map_w)
	var ry := 0.0 if map_h <= 0 else float(old_img.y) / float(map_h)

	var z := clampf(new_zoom, ZOOM_MIN, ZOOM_MAX)
	# 先按新缩放算出新的图像尺寸
	var new_w := int(round(float(map_w) * (z / zoom_factor)))
	var new_h := int(round(float(map_h) * (z / zoom_factor)))
	zoom_factor = z

	var new_px := int(round(float(new_w) * rx))
	var new_py := int(round(float(new_h) * ry))

	map_w = new_w
	map_h = new_h

	auto_center = false
	map_dx = old_dx + (old_img.x - new_px)
	map_dy = old_dy + (old_img.y - new_py)
	fix_map_image_p_loc()


## fixMapImagePLoc（minimapboard.cpp:543）
## 图像小于画布时居中；大于画布时夹取，保证不露出画布外
func fix_map_image_p_loc(force_center: bool = false) -> void:
	if force_center:
		auto_center = false

	if map_w <= canvas_w:
		map_dx = (canvas_w - map_w) / 2
	else:
		map_dx = clampi(map_dx, canvas_w - map_w, 0)

	if map_h <= canvas_h:
		map_dy = (canvas_h - map_h) / 2
	else:
		map_dy = clampi(map_dy, canvas_h - map_h, 0)


func _fix_loc(center: bool) -> void:
	fix_map_image_p_loc(center)


## 拖动（鼠标拖拽平移）
func pan(dx: int, dy: int) -> void:
	map_dx += dx
	map_dy += dy
	fix_map_image_p_loc()


## 自动居中到某个地图格（玩家所在位置）
func center_on_grid(gx: int, gy: int, map_grid_w: int, map_grid_h: int) -> void:
	if not auto_center:
		return
	var img := grid_to_image(gx, gy, map_grid_w, map_grid_h)
	map_dx = canvas_w / 2 - img.x
	map_dy = canvas_h / 2 - img.y
	fix_map_image_p_loc()
