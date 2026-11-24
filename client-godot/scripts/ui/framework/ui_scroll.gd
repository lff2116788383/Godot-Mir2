extends UiWidget
class_name UiScroll
## 滚动容器
##
## 【逐行翻译】client/src/scrollcontainer.hpp（187 行）+ scrollcontainer.cpp（550 行）
## 对照：
##   - scrollcontainer.cpp:376-550  processEventDefault（拖动状态机）
##   - scrollcontainer.cpp:vTrackROI/vThumbROI  滚动条几何
##
## 核心语义：
##   1. 内容区可超出可视区，超出部分被 ROI 裁掉（不发散到父级）
##   2. 竖向/横向滚动条：轨道 + 滑块，滑块长度按内容比计算
##   3. 三种拖动状态：DRAG_NONE / DRAG_V_THUMB / DRAG_H_THUMB / DRAG_VIEWPORT
##      —— 拖滑块与拖内容区（viewport）都支持
##
## Godot 差异：不依赖 Godot 的 ScrollContainer，因为原版的滚动条要自己画贴图。

const RoiRef := preload("res://scripts/ui/framework/ui_roi.gd")

const DRAG_NONE := 0
const DRAG_V_THUMB := 1
const DRAG_H_THUMB := 2
const DRAG_VIEWPORT := 3

## 可视区尺寸（不设则取 w/h）
var view_w = null
var view_h = null

## 内容总尺寸
var content_w := 0
var content_h := 0

## 当前滚动偏移
var scroll_x := 0
var scroll_y := 0

## 滚动条宽度
var bar_w := 8
## 滚动条贴图：轨道与滑块（tex_id）
var track_tex_id := -1
var thumb_tex_id := -1
## 滚动条颜色（无贴图时用纯色画）
var track_color := Color(0.1, 0.1, 0.1, 0.6)
var thumb_color := Color(0.55, 0.55, 0.6, 0.9)

## 是否启用
var v_enabled := true
var h_enabled := false

## 贴图提供者
var tex_provider: Callable = Callable()

var _drag := DRAG_NONE
var _drag_start_pos := Vector2.ZERO
var _drag_start_scroll := Vector2i.ZERO


func _init(p_name: String = "", p_vw = 0, p_vh = 0) -> void:
	widget_name = p_name
	view_w = p_vw
	view_h = p_vh
	on_event = _handle_event


func size_w() -> int:
	if w != null:
		return eval_size(w, self, 0)
	if view_w != null:
		return eval_size(view_w, self, 0)
	return content_w


func size_h() -> int:
	if h != null:
		return eval_size(h, self, 0)
	if view_h != null:
		return eval_size(view_h, self, 0)
	return content_h


## 可视区（不含滚动条）
func viewport_roi(my_roi) -> UiRoi:
	var ww: int = my_roi.w
	if v_enabled and content_h > my_roi.h:
		ww -= bar_w
	var hh: int = my_roi.h
	if h_enabled and content_w > my_roi.w:
		hh -= bar_w
	return UiRoi.new(my_roi.x, my_roi.y, maxi(0, ww), maxi(0, hh))


## 竖向滑块矩形的 y 与高度
func v_thumb_geo(my_roi) -> Dictionary:
	var vp := viewport_roi(my_roi)
	if vp.h <= 0:
		return {}
	if content_h <= vp.h:
		return {}
	var ratio := float(vp.h) / float(content_h)
	var thumb_h := maxi(20, int(vp.h * ratio))
	var max_scroll := content_h - vp.h
	var t := 0.0 if max_scroll <= 0 else float(scroll_y) / float(max_scroll)
	var thumb_y := vp.y + int((vp.h - thumb_h) * clampf(t, 0.0, 1.0))
	return {"y": thumb_y, "h": thumb_h, "x": my_roi.x + my_roi.w - bar_w, "w": bar_w}


func max_scroll_y(my_roi) -> int:
	var vp := viewport_roi(my_roi)
	return maxi(0, content_h - vp.h)


func max_scroll_x(my_roi) -> int:
	var vp := viewport_roi(my_roi)
	return maxi(0, content_w - vp.w)


func clamp_scroll() -> void:
	scroll_x = maxi(0, scroll_x)
	scroll_y = maxi(0, scroll_y)


## 绘制：先画滚动条，子控件由外层按 viewport 裁剪后绘制
func draw_self(canvas, my_roi) -> void:
	if not v_enabled:
		return
	var geo := v_thumb_geo(my_roi)
	if geo.is_empty():
		return
	# 轨道
	canvas.draw_rect_c(track_color, my_roi.x + my_roi.w - bar_w, my_roi.y, bar_w, my_roi.h, true)
	# 滑块
	canvas.draw_rect_c(thumb_color, int(geo["x"]), int(geo["y"]), int(geo["w"]), int(geo["h"]), true)


## scrollcontainer.cpp:376
func _handle_event(self_widget, event: InputEvent, valid: bool, my_roi) -> bool:
	# 拖动中
	if _drag != DRAG_NONE:
		if event is InputEventMouseMotion:
			var d: Vector2 = event.position - _drag_start_pos
			match _drag:
				DRAG_V_THUMB:
					var geo := v_thumb_geo(my_roi)
					if geo.is_empty():
						_drag = DRAG_NONE
						return false
					var track_h := viewport_roi(my_roi).h - int(geo["h"])
					if track_h > 0:
						var msy := max_scroll_y(my_roi)
						scroll_y = clampi(_drag_start_scroll.y + int(d.y * float(msy) / float(track_h)), 0, msy)
					return true
				DRAG_VIEWPORT:
					scroll_x = clampi(_drag_start_scroll.x - int(d.x), 0, max_scroll_x(my_roi))
					scroll_y = clampi(_drag_start_scroll.y - int(d.y), 0, max_scroll_y(my_roi))
					return true
			return true
		if event is InputEventMouseButton and not event.pressed:
			_drag = DRAG_NONE
			return true
		return true

	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_LEFT:
		var mpos: Vector2 = event.position
		if event.pressed:
			var geo := v_thumb_geo(my_roi)
			if not geo.is_empty():
				var thumb_rect := Rect2(int(geo["x"]), int(geo["y"]), int(geo["w"]), int(geo["h"]))
				if thumb_rect.has_point(mpos):
					_drag = DRAG_V_THUMB
					_drag_start_pos = mpos
					_drag_start_scroll = Vector2i(scroll_x, scroll_y)
					return true
			if my_roi.contains(int(mpos.x), int(mpos.y)):
				_drag = DRAG_VIEWPORT
				_drag_start_pos = mpos
				_drag_start_scroll = Vector2i(scroll_x, scroll_y)
				return true

	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_WHEEL_DOWN:
		scroll_y = mini(scroll_y + 30, max_scroll_y(my_roi))
		scroll_y = maxi(scroll_y, 0)
		return true
	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_WHEEL_UP:
		scroll_y = maxi(scroll_y - 30, 0)
		return true

	return false
