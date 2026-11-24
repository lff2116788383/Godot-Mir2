extends UiWidget
class_name UiButton
## 按钮控件
##
## 【逐行翻译】client/src/buttonbase.hpp + buttonbase.cpp:51-130
##            client/src/tritexbutton.hpp（三态贴图）
##
## 状态机（client/src/bevent.hpp）：
##   BEVENT_OFF  = 0  普通
##   BEVENT_ON   = 1  悬停
##   BEVENT_DOWN = 2  按下
##
## 交互语义（buttonbase.cpp:87-130）：
##   - 鼠标松开时若在区域内 -> ON + setFocus，触发 onClick
##   - 离开区域 -> OFF + onOverOut
##   - radioMode 与普通模式的区别：radio 模式保持 ON 直到被明确关闭
##
## 三态贴图（tritexbutton.hpp:52）：
##   texList = { off, on, down }，按状态选一张画

const RoiRef := preload("res://scripts/ui/framework/ui_roi.gd")

const BEVENT_OFF := 0
const BEVENT_ON := 1
const BEVENT_DOWN := 2

## 三态贴图 id（对应 TritexIDList 的三个 AniFrameOpt）
## 每项为 null 或 { "tex_id": int, "frame": int }
var tex_off = null
var tex_on = null
var tex_down = null

## 贴图查询回调：Callable(widget, tex_id, frame) -> Texture2D
var tex_provider: Callable = Callable()

## 点击回调 Callable(widget)
var on_click: Callable = Callable()
## 悬停进入/离开 Callable(widget)
var on_over_in: Callable = Callable()
var on_over_out: Callable = Callable()
## 触发回调（按下瞬间）Callable(widget)
var on_trigger: Callable = Callable()

## radio 模式：保持选中
var radio_mode := false

## 悬停/点击时的像素偏移（原版 offXOnOver / offXOnClick）
var off_x_on_over := 0
var off_y_on_over := 0
var off_x_on_click := 0
var off_y_on_click := 0

var _state := BEVENT_OFF
## 鼠标是否在区域内
var _hover := false


func _init(p_name: String = "", p_w = 0, p_h = 0) -> void:
	widget_name = p_name
	w = p_w
	h = p_h
	on_event = _handle_event


func get_state() -> int:
	return _state


func set_state(s: int) -> void:
	if _state == s:
		return
	var was_off := _state == BEVENT_OFF
	_state = s
	# buttonbase.cpp:57-70  离开 OFF 与进入 OFF 分别触发回调
	if was_off and s != BEVENT_OFF:
		if on_over_in.is_valid():
			on_over_in.call(self)
	elif not was_off and s == BEVENT_OFF:
		if on_over_out.is_valid():
			on_over_out.call(self)


## 当前状态下应绘制的贴图项
func _current_tex() -> Dictionary:
	var item = tex_off
	match _state:
		BEVENT_ON:
			if tex_on != null:
				item = tex_on
		BEVENT_DOWN:
			if tex_down != null:
				item = tex_down
			elif tex_on != null:
				item = tex_on
	if item == null:
		return {}
	if item is Dictionary:
		return item
	return {}


## 绘制：按状态画贴图（tritexbutton.cpp drawDefault）
func draw_self(canvas, my_roi) -> void:
	if not tex_provider.is_valid():
		return
	var item := _current_tex()
	if item.is_empty():
		return
	var tex = tex_provider.call(self, int(item.get("tex_id", 0)), int(item.get("frame", 0)))
	if tex == null:
		return

	var ox := 0
	var oy := 0
	if _hover:
		ox += off_x_on_over
		oy += off_y_on_over
	if _state == BEVENT_DOWN:
		ox += off_x_on_click
		oy += off_y_on_click

	canvas.tex_draw(tex, my_roi.x + ox, my_roi.y + oy)


## buttonbase.cpp:51  processEventDefault
func _handle_event(self_widget, event: InputEvent, valid: bool, my_roi) -> bool:
	if not is_active():
		if _state != BEVENT_OFF:
			set_state(BEVENT_OFF)
			_hover = false
		return false

	if event is InputEventMouseMotion:
		var mpos: Vector2 = event.position
		var inside: bool = my_roi.contains(int(mpos.x), int(mpos.y))
		if inside != _hover:
			_hover = inside
			if inside:
				if on_over_in.is_valid():
					on_over_in.call(self)
			else:
				if on_over_out.is_valid():
					on_over_out.call(self)
		if inside and _state == BEVENT_OFF:
			set_state(BEVENT_ON)
		elif not inside and _state == BEVENT_ON and not radio_mode:
			set_state(BEVENT_OFF)
		return false

	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_LEFT:
		var mpos: Vector2 = event.position
		var inside: bool = my_roi.contains(int(mpos.x), int(mpos.y))
		if event.pressed:
			if inside:
				set_state(BEVENT_DOWN)
				if on_trigger.is_valid():
					on_trigger.call(self)
				return true
		else:
			# 松开
			if inside:
				if not radio_mode:
					set_state(BEVENT_ON)
				else:
					set_state(BEVENT_ON)
				if on_click.is_valid():
					on_click.call(self)
				return true
			else:
				if not radio_mode:
					set_state(BEVENT_OFF)
				return false
	return false
