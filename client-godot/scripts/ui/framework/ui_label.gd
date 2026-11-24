extends UiWidget
class_name UiLabel
## 文字标签
##
## 【逐行翻译】client/src/labelboard.hpp + labelboard.cpp（88 行）
## 原版内部用 XMLTypeset 做排版，这里直接复用已翻译的 XmlTypeset（B8 产物）。
##
## 支持：
##   - 单行/多行文字
##   - 对齐（左/右/居中）
##   - 字体与颜色
##   - 阴影

const TypesetRef := preload("res://scripts/ui/xml_typeset.gd")
const TokenRef := preload("res://scripts/ui/xml_token.gd")

var text: String = ""
var font: Font = null
var font_size: int = 15
var color: Color = Color.WHITE
var align: int = 0   ## 0=LEFT 1=RIGHT 2=CENTER

## 阴影
var shadow := false
var shadow_color: Color = Color(0, 0, 0, 0.85)
var shadow_offset := 1

## 行间距
var line_space := 0


func _init(p_name: String = "", p_text: String = "") -> void:
	widget_name = p_name
	text = p_text
	on_draw = _draw_self


## 自动尺寸：按文字实际宽度
func measure(canvas) -> Vector2:
	if canvas == null:
		return Vector2.ZERO
	return canvas.measure_text(text, font, font_size)


func _draw_self(self_widget, my_roi) -> void:
	pass  # 实际绘制由 UiCanvas 调 draw_text_of 完成


## 供 UiCanvas 调用的绘制入口
## 因为需要用 canvas 的字体测量，所以绘制放在 canvas 侧调度
func paint(canvas, my_roi) -> void:
	if text.is_empty():
		return
	var box_w: int = my_roi.w if my_roi.w > 0 else size_w()
	var tw: float = canvas.measure_text(text, font, font_size).x
	var ox := 0.0
	match align:
		1:  # RIGHT
			ox = box_w - tw
		2:  # CENTER
			ox = (box_w - tw) * 0.5

	if shadow:
		canvas.draw_string_shadow(text, my_roi.x + int(ox), my_roi.y,
			color, shadow_color, shadow_offset, font, font_size)
	else:
		canvas.draw_string_c(text, my_roi.x + int(ox), my_roi.y, color, font, font_size)
