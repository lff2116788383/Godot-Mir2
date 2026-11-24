extends UiWidget
class_name UiInputStringBoard
## 输入串面板（重命名 / 输入文本 / 确认对话框的载体）
##
## 【逐行翻译】client/src/inputstringboard.cpp（333 行）
##
## ── 布局（inputstringboard.cpp，逐值）──
##   背景 0x07000000（358x268，已实测）
##   标题文字在 (w/2, contentY + textInfoH/2)，居中
##   输入框 m_inputBg 在 (22, 225)，315x23
##   两个选项按钮 y=190：x 由 w 决定（左右各一个）
##
## ── 三种用法（原版同一个类）──
##   1. waitInput：输入一行文本，回调提交
##   2. waitChoice：给一组选项（XML 排版），选一个回调
##   3. waitConfirm：确认/取消二选一
##
## ── 与原版等的关键点 ──
##   按钮回调带 choice 索引；取消时回调参数为空
##   waitChoice 的 xmlList.size() 决定按钮数量（原版最多显示若干个）

const BG_TEX := 0x07000000
const BOARD_W := 358
const BOARD_H := 268

## 输入框
const INPUT_X := 22
const INPUT_Y := 225
const INPUT_W := 315
const INPUT_H := 23

## 按钮 y
const BTN_Y := 190

# ==================== 状态 ====================

## 模式
const MODE_INPUT := 0
const MODE_CHOICE := 1
const MODE_CONFIRM := 2

var mode := MODE_INPUT
## 标题（居中显示）
var title: String = ""
## 输入内容
var input_text: String = ""
## 选项文本
var choices: Array = []
## 是否允许取消
var cancelable := true

var on_submit: Callable = Callable()    ## Callable(text)
var on_choice: Callable = Callable()    ## Callable(index)
var on_cancel: Callable = Callable()

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init(p_name: String = "InputStringBoard") -> void:
	widget_name = p_name
	w = BOARD_W
	h = BOARD_H
	show = false
	on_draw = _draw_board


func set_canvas(c) -> void:
	_canvas_ref = c


## 对应 waitInput：要一行文本
func wait_input(p_title: String, default_text: String = "", p_cancelable: bool = true) -> void:
	mode = MODE_INPUT
	title = p_title
	input_text = default_text
	choices = []
	cancelable = p_cancelable
	show = true


## 对应 waitChoice：从若干选项里选一个
func wait_choice(p_title: String, p_choices: Array, p_cancelable: bool = true) -> void:
	mode = MODE_CHOICE
	title = p_title
	choices = p_choices.duplicate()
	input_text = ""
	cancelable = p_cancelable
	show = true


## 对应 waitConfirm：确认 / 取消
func wait_confirm(p_title: String) -> void:
	mode = MODE_CONFIRM
	title = p_title
	choices = ["确定", "取消"]
	input_text = ""
	cancelable = true
	show = true


func hide_board() -> void:
	show = false
	if on_cancel.is_valid():
		on_cancel.call()


# ==================== 绘制 ====================

func _draw_board(_w, roi) -> void:
	var canvas = _canvas_ref
	if canvas == null:
		return
	hit_areas.clear()

	var bg := ProgUseTex.retrieve(BG_TEX)
	if bg != null:
		canvas.tex_draw(bg, roi.x, roi.y)
	else:
		canvas.draw_rect_c(Color(0.1, 0.09, 0.07, 0.98), roi.x, roi.y, BOARD_W, BOARD_H, true)
		canvas.draw_border(Color(0.52, 0.44, 0.28, 1.0), roi.x, roi.y, BOARD_W, BOARD_H, 2)

	# 标题（居中，可能是多行 XML；这里按 \n 拆行）
	var tlines := title.split("\n")
	var ty: int = roi.y + 40
	for tl in tlines:
		var tw: int = int(canvas.measure_text(tl, null, 14).x)
		canvas.draw_string_c(tl, roi.x + BOARD_W / 2 - tw / 2, ty,
			Color(0.95, 0.92, 0.8), null, 14)
		ty += 20

	# 输入框（MODE_INPUT 才有）
	if mode == MODE_INPUT:
		var ix: int = roi.x + INPUT_X
		var iy: int = roi.y + INPUT_Y
		canvas.draw_rect_c(Color(0, 0, 0, 0.65), ix, iy, INPUT_W, INPUT_H, true)
		canvas.draw_border(Color(0.45, 0.38, 0.22, 1.0), ix, iy, INPUT_W, INPUT_H, 1)
		canvas.draw_string_c(input_text, ix + 4, iy + 4, Color(0.95, 0.93, 0.86), null, 14)
		# 光标
		var tw2: int = int(canvas.measure_text(input_text, null, 14).x)
		canvas.draw_rect_c(Color(1, 1, 1, 0.85), ix + 5 + tw2, iy + 4, 1, INPUT_H - 8, true)
		hit_areas["input"] = Rect2(ix, iy, INPUT_W, INPUT_H)

	# 选项按钮
	if not choices.is_empty():
		var n := choices.size()
		var btn_w := 80
		var gap := 24
		var total_w: int = n * btn_w + (n - 1) * gap
		var start_x: int = roi.x + (BOARD_W - total_w) / 2
		for i in range(n):
			var bx: int = start_x + i * (btn_w + gap)
			var by: int = roi.y + BTN_Y
			canvas.draw_rect_c(Color(0.24, 0.2, 0.12, 0.95), bx, by, btn_w, 24, true)
			canvas.draw_border(Color(0.55, 0.46, 0.26, 1.0), bx, by, btn_w, 24, 1)
			var label := str(choices[i])
			var lw: int = int(canvas.measure_text(label, null, 13).x)
			canvas.draw_string_c(label, bx + (btn_w - lw) / 2, by + 4,
				Color(1, 0.95, 0.72), null, 13)
			hit_areas["choice_%d" % i] = Rect2(bx, by, btn_w, 24)

	# 取消按钮（右下角小叉）
	if cancelable and mode != MODE_CONFIRM:
		var ctex := ProgUseTex.retrieve(0x0000001C)
		var cx: int = roi.x + BOARD_W - 40
		var cy: int = roi.y + BOARD_H - 40
		if ctex != null:
			canvas.tex_draw(ctex, cx, cy)
			hit_areas["cancel"] = Rect2(cx, cy, 32, 30)
		else:
			canvas.draw_string_c("×", cx, cy, Color(0.9, 0.75, 0.5), null, 18)
			hit_areas["cancel"] = Rect2(cx, cy, 20, 20)


# ==================== 事件 ====================

func handle_click(px: int, py: int) -> bool:
	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			var k := str(key)
			if k == "cancel":
				show = false
				if on_cancel.is_valid():
					on_cancel.call()
				return true
			if k.begins_with("choice_"):
				var idx: int = int(k.substr(7))
				show = false
				if mode == MODE_CONFIRM and idx == 1:
					if on_cancel.is_valid():
						on_cancel.call()
				elif on_choice.is_valid():
					on_choice.call(idx)
				return true
	return false


## 回车提交（MODE_INPUT）
func submit() -> bool:
	if mode != MODE_INPUT:
		return false
	var t := input_text
	show = false
	if on_submit.is_valid():
		on_submit.call(t)
	return true


## 输入字符（外部转发键盘）
func type_text(s: String) -> void:
	if mode != MODE_INPUT:
		return
	input_text += s


func backspace() -> void:
	if mode != MODE_INPUT or input_text.is_empty():
		return
	input_text = input_text.substr(0, input_text.length() - 1)
