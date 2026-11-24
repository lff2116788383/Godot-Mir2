extends UiWidget
class_name UiCheckBox
## 复选框 / 单选框
##
## 【逐行翻译】client/src/checkbox.hpp + checkbox.cpp（224 行）
##            client/src/checklabel.hpp + checklabel.cpp（202 行）
##            client/src/radioselector.hpp + radioselector.cpp（176 行）
##
## 核心语义（checkbox.cpp）：
##   - 三态贴图：未选 / 已选 / 悬停
##   - 点击切换 checked
##   - radio 模式下，同组只能有一个选中（由外部组管理）

const BEVENT_OFF := 0
const BEVENT_ON := 1
const BEVENT_DOWN := 2

## 三态贴图 id
var tex_off: int = -1
var tex_on: int = -1
var tex_down: int = -1

## 贴图提供者
var tex_provider: Callable = Callable()

## 当前是否选中
var checked := false
## radio 模式（互斥）
var radio_mode := false

## 文字（checklabel 用）
var text: String = ""
var font: Font = null
var font_size: int = 15
var color: Color = Color.WHITE
## 文字与图标的间距
var text_gap := 4

## 状态
var _hover := false
var _down := false

## 变化回调 Callable(widget, checked)
var on_toggle: Callable = Callable()
## 点击回调
var on_click: Callable = Callable()


func _init(p_name: String = "", p_checked: bool = false, p_text: String = "") -> void:
	widget_name = p_name
	checked = p_checked
	text = p_text
	on_event = _handle_event


## 图标尺寸（取当前贴图）
func icon_size() -> Vector2i:
	if not tex_provider.is_valid():
		return Vector2i(0, 0)
	var tid := tex_on if checked else tex_off
	if tid < 0:
		return Vector2i(0, 0)
	var t = tex_provider.call(self, tid, 0)
	if t == null:
		return Vector2i(0, 0)
	return Vector2i(t.get_width(), t.get_height())


func size_w() -> int:
	if w != null:
		return eval_size(w, self, 0)
	var isz := icon_size()
	if text.is_empty():
		return isz.x
	# 文字宽度需要字体测量，这里退回估算（真正绘制时以 canvas 测量为准）
	return isz.x + text_gap + text.length() * font_size


func size_h() -> int:
	if h != null:
		return eval_size(h, self, 0)
	var isz := icon_size()
	return maxi(isz.y, font_size)


## 绘制
func draw_self(canvas, my_roi) -> void:
	if tex_provider.is_valid():
		var tid := tex_off
		if _down and tex_down >= 0:
			tid = tex_down
		elif checked and tex_on >= 0:
			tid = tex_on
		elif _hover and tex_on >= 0:
			tid = tex_on
		var t = tex_provider.call(self, tid, 0)
		if t != null:
			canvas.tex_draw(t, my_roi.x, my_roi.y)
			# 文字画在图标右侧
			if not text.is_empty():
				canvas.draw_string_c(text, my_roi.x + t.get_width() + text_gap, my_roi.y,
					color, font, font_size)
			return
	# 无贴图时纯文字
	if not text.is_empty():
		canvas.draw_string_c(text, my_roi.x, my_roi.y, color, font, font_size)


## checkbox.cpp 事件处理
func _handle_event(self_widget, event: InputEvent, valid: bool, my_roi) -> bool:
	if not is_active():
		return false

	if event is InputEventMouseMotion:
		var mpos: Vector2 = event.position
		_hover = my_roi.contains(int(mpos.x), int(mpos.y))
		return false

	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_LEFT:
		var mpos: Vector2 = event.position
		var inside: bool = my_roi.contains(int(mpos.x), int(mpos.y))
		if event.pressed:
			if inside:
				_down = true
				return true
		else:
			if _down and inside:
				_down = false
				if radio_mode:
					# radio 模式：只能被选中，不能被取消
					if not checked:
						checked = true
						if on_toggle.is_valid():
							on_toggle.call(self, checked)
				else:
					checked = not checked
					if on_toggle.is_valid():
						on_toggle.call(self, checked)
				if on_click.is_valid():
					on_click.call(self)
				return true
			_down = false
	return false
