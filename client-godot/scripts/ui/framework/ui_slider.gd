extends UiWidget
class_name UiSlider
## 滑块
##
## 【逐行翻译】client/src/sliderbase.hpp + sliderbase.cpp（301 行）
##            client/src/texslider.hpp（114 行）+ texsliderbar.hpp（55 行）
##
## 核心语义（sliderbase.cpp）：
##   - 值域 [min_value, max_value]，可设步长
##   - 拖动滑块或点击轨道都会改变值
##   - 横向/纵向
##   - 滑块位置由 value 在值域中的比例决定
##
## texslider 是「用贴图做轨道和滑块」的版本，
## 这里统一支持「贴图」与「纯色」两种画法。

const DRAG_NONE := 0
const DRAG_THUMB := 1

## 值域
var min_value := 0
var max_value := 100
var value := 0
var step := 0        ## 0 = 连续

## 方向：true = 横向
var horizontal := true

## 轨道与滑块的贴图 id（<0 则用纯色）
var track_tex_id := -1
var thumb_tex_id := -1
var track_color := Color(0.15, 0.15, 0.18, 0.9)
var thumb_color := Color(0.75, 0.75, 0.8, 1.0)

## 滑块尺寸
var thumb_w := 10
var thumb_h := 18

## 贴图提供者
var tex_provider: Callable = Callable()

## 变化回调 Callable(widget, value)
var on_change: Callable = Callable()

var _drag := DRAG_NONE


func _init(p_name: String = "", p_min: int = 0, p_max: int = 100, p_value: int = 0) -> void:
	widget_name = p_name
	min_value = p_min
	max_value = p_max
	value = p_value
	on_event = _handle_event


func set_value(v: int) -> void:
	var nv := clampi(v, min_value, max_value)
	if step > 0:
		nv = min_value + int(round(float(nv - min_value) / float(step))) * step
		nv = clampi(nv, min_value, max_value)
	if nv == value:
		return
	value = nv
	if on_change.is_valid():
		on_change.call(self, value)


## 值 -> 比例 [0,1]
func value_ratio() -> float:
	if max_value <= min_value:
		return 0.0
	return float(value - min_value) / float(max_value - min_value)


## 滑块在轨道上的矩形
func thumb_rect(my_roi) -> Rect2:
	var r := value_ratio()
	if horizontal:
		var usable := maxi(1, my_roi.w - thumb_w)
		return Rect2(my_roi.x + int(usable * r), my_roi.y + (my_roi.h - thumb_h) / 2,
			thumb_w, thumb_h)
	else:
		var usable := maxi(1, my_roi.h - thumb_h)
		# 纵向：值大在上
		return Rect2(my_roi.x + (my_roi.w - thumb_w) / 2,
			my_roi.y + int(usable * (1.0 - r)), thumb_w, thumb_h)


## 绘制
func draw_self(canvas, my_roi) -> void:
	# 轨道
	if tex_provider.is_valid() and track_tex_id >= 0:
		var tt = tex_provider.call(self, track_tex_id, 0)
		if tt != null:
			canvas.tex_draw_region(tt, Rect2(0, 0, tt.get_width(), tt.get_height()),
				my_roi.x, my_roi.y, my_roi.w, my_roi.h)
	else:
		var th: int = 4 if horizontal else my_roi.w
		if horizontal:
			canvas.draw_rect_c(track_color, my_roi.x, my_roi.y + (my_roi.h - th) / 2,
				my_roi.w, th, true)
		else:
			canvas.draw_rect_c(track_color, my_roi.x + (my_roi.w - th) / 2, my_roi.y,
				th, my_roi.h, true)

	# 滑块
	var tr := thumb_rect(my_roi)
	if tex_provider.is_valid() and thumb_tex_id >= 0:
		var tm = tex_provider.call(self, thumb_tex_id, 0)
		if tm != null:
			canvas.tex_draw_region(tm, Rect2(0, 0, tm.get_width(), tm.get_height()),
				int(tr.position.x), int(tr.position.y), int(tr.size.x), int(tr.size.y))
			return
	canvas.draw_rect_c(thumb_color, int(tr.position.x), int(tr.position.y),
		int(tr.size.x), int(tr.size.y), true)


## sliderbase.cpp 事件处理
func _handle_event(self_widget, event: InputEvent, valid: bool, my_roi) -> bool:
	if not is_active():
		return false

	if event is InputEventMouseMotion and _drag == DRAG_THUMB:
		_apply_mouse(event.position, my_roi)
		return true

	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_LEFT:
		var mpos: Vector2 = event.position
		if event.pressed:
			var tr := thumb_rect(my_roi)
			if tr.has_point(mpos):
				_drag = DRAG_THUMB
				return true
			if my_roi.contains(int(mpos.x), int(mpos.y)):
				# 点轨道直接跳
				_apply_mouse(mpos, my_roi)
				_drag = DRAG_THUMB
				return true
		else:
			if _drag != DRAG_NONE:
				_drag = DRAG_NONE
				return true
	return false


func _apply_mouse(mpos: Vector2, my_roi) -> void:
	var t: float
	if horizontal:
		var usable := maxi(1, my_roi.w - thumb_w)
		t = (mpos.x - my_roi.x - thumb_w * 0.5) / float(usable)
	else:
		var usable := maxi(1, my_roi.h - thumb_h)
		t = 1.0 - (mpos.y - my_roi.y - thumb_h * 0.5) / float(usable)
	t = clampf(t, 0.0, 1.0)
	set_value(min_value + int(round(float(max_value - min_value) * t)))
