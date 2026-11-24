extends UiWidget
class_name UiInputLine
## 单行输入框
##
## 【逐行翻译】client/src/inputline.hpp + inputline.cpp（490 行）
##            client/src/textinput.hpp（108 行）
##
## 核心语义（inputline.cpp:46-170）：
##   - 获得焦点后可输入
##   - TEXT_INPUT 事件插入文字（系统 IME 已处理完的成品）
##   - 光标位置可左右移动，退格删除光标前一个字符
##   - 光标闪烁（m_cursorBlink）
##   - Enter 触发 onCR 回调
##
## IME 三档（protocoldef.hpp:557-562）：
##   IME_DISABLE = 1  不启用输入法（只接受 ASCII）
##   IME_EMBEDED = 2  用游戏内置输入法（imeboard）
##   IME_SYSTEM  = 3  用系统输入法
##
## Godot 差异：IME 交给 Godot（自动处理 TEXT_INPUT），
##   内置输入法面板（imeboard）留待后续；此处按 IME_SYSTEM 行为实现。

const IME_DISABLE := 1
const IME_EMBEDED := 2
const IME_SYSTEM := 3

## 内容
var text: String = ""
## 光标位置（字符索引）
var cursor: int = 0
## 最大长度（0 = 不限）
var max_length := 0
## IME 模式
var ime_mode := IME_SYSTEM
## 密码模式（显示为圆点）
var password_mode := false

var font: Font = null
var font_size: int = 15
var color: Color = Color.WHITE
## 背景与边框
var bg_color := Color(0, 0, 0, 0.6)
var border_color := Color(0.4, 0.4, 0.45, 1.0)
## 光标
var cursor_color := Color(1, 1, 1, 0.9)
var cursor_width := 2
var _cursor_blink := 0.0

## 占位提示（空内容时显示）
var placeholder: String = ""
var placeholder_color := Color(0.6, 0.6, 0.6, 0.8)

## 事件回调
## Callable(widget, text) 内容变化
var on_change: Callable = Callable()
## Callable(widget, text) 回车
var on_enter: Callable = Callable()
## Callable(widget) 点击聚焦
var on_focus: Callable = Callable()

var _focused := false


func _init(p_name: String = "", p_text: String = "") -> void:
	widget_name = p_name
	text = p_text
	cursor = text.length()
	on_event = _handle_event


func set_text(t: String) -> void:
	text = t if max_length <= 0 else t.substr(0, max_length)
	cursor = clampi(cursor, 0, text.length())


func insert_text(t: String) -> bool:
	if t.is_empty():
		return false
	if max_length > 0 and text.length() >= max_length:
		return false
	var add := t
	if max_length > 0:
		add = t.substr(0, max_length - text.length())
	text = text.substr(0, cursor) + add + text.substr(cursor)
	cursor += add.length()
	_cursor_blink = 0.0
	if on_change.is_valid():
		on_change.call(self, text)
	return true


func backspace() -> bool:
	if cursor <= 0:
		return false
	text = text.substr(0, cursor - 1) + text.substr(cursor)
	cursor -= 1
	_cursor_blink = 0.0
	if on_change.is_valid():
		on_change.call(self, text)
	return true


func delete_forward() -> bool:
	if cursor >= text.length():
		return false
	text = text.substr(0, cursor) + text.substr(cursor + 1)
	_cursor_blink = 0.0
	if on_change.is_valid():
		on_change.call(self, text)
	return true


func is_focused() -> bool:
	return _focused


func set_focused(f: bool) -> void:
	if _focused == f:
		return
	_focused = f
	_cursor_blink = 0.0
	if f and on_focus.is_valid():
		on_focus.call(self)


## 光标闪烁推进
func update_widget(dt: float) -> void:
	super.update_widget(dt)
	if _focused:
		_cursor_blink = fmod(_cursor_blink + dt, 1.0)


## 绘制
func draw_self(canvas, my_roi) -> void:
	# 背景
	if bg_color.a > 0.0:
		canvas.draw_rect_c(bg_color, my_roi.x, my_roi.y, my_roi.w, my_roi.h, true)
	# 边框（聚焦时高亮）
	var bc := border_color
	if _focused:
		bc = Color(0.85, 0.85, 0.9, 1.0)
	canvas.draw_border(bc, my_roi.x, my_roi.y, my_roi.w, my_roi.h, 1)

	var show_text := text
	if password_mode and not text.is_empty():
		show_text = "*".repeat(text.length())

	# 空内容显示占位
	if show_text.is_empty() and not placeholder.is_empty():
		canvas.draw_string_c(placeholder, my_roi.x + 4, my_roi.y + 3,
			placeholder_color, font, font_size)
		return

	canvas.draw_string_c(show_text, my_roi.x + 4, my_roi.y + 3, color, font, font_size)

	# 光标
	if _focused and _cursor_blink < 0.5:
		var pre := text.substr(0, cursor)
		if password_mode:
			pre = "*".repeat(cursor)
		var cx: float = canvas.measure_text(pre, font, font_size).x
		canvas.draw_rect_c(cursor_color, my_roi.x + 4 + int(cx), my_roi.y + 3,
			cursor_width, font_size, true)


## inputline.cpp:46
func _handle_event(self_widget, event: InputEvent, valid: bool, my_roi) -> bool:
	# 点击聚焦
	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_LEFT and event.pressed:
		var mpos: Vector2 = event.position
		var inside: bool = my_roi.contains(int(mpos.x), int(mpos.y))
		set_focused(inside)
		if inside:
			# 点击后光标移到末尾（原版支持按 x 定位光标，此处简化）
			cursor = text.length()
			return true
		return false

	if not _focused:
		return false

	# 文本输入（IME 成品）
	if event is InputEventKey and event.pressed:
		if ime_mode == IME_DISABLE and event.unicode > 0x7F:
			return true   # 非 ASCII 不接受
		match event.keycode:
			KEY_BACKSPACE:
				backspace()
				return true
			KEY_DELETE:
				delete_forward()
				return true
			KEY_LEFT:
				cursor = maxi(0, cursor - 1)
				_cursor_blink = 0.0
				return true
			KEY_RIGHT:
				cursor = mini(text.length(), cursor + 1)
				_cursor_blink = 0.0
				return true
			KEY_HOME:
				cursor = 0
				return true
			KEY_END:
				cursor = text.length()
				return true
			KEY_ENTER, KEY_KP_ENTER:
				if on_enter.is_valid():
					on_enter.call(self, text)
				return true
		# 普通字符
		if event.unicode > 0 and event.unicode != 127:
			var ch := char(event.unicode)
			if insert_text(ch):
				return true
	return false
