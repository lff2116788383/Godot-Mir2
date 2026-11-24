extends Node2D
class_name UiCanvas
## UI 绘制层
##
## 【逐行翻译】client/src/sdldevice.hpp:240-440 的绘制原语
## 对照：
##   - sdldevice.hpp:246  drawTexture(tex, dx, dy)
##   - sdldevice.hpp:247  drawTexture(tex, srcX, srcY, srcW, srcH, dstX, dstY, dstW, dstH)
##   - sdldevice.hpp:281  drawLine
##   - sdldevice.hpp:335  drawRectangle
##   - sdldevice.hpp:429  drawString
##
## Godot 差异：SDL_Renderer 改成 CanvasItem 的 draw_* 接口。
##   坐标含义完全一致（屏幕像素坐标，左上角原点）。
##   关键补充：所有绘制都按当前 ROI 裁剪（原版靠 SDL 的 viewport 裁剪，
##   Godot 这里手工求交，效果等价且可控）。

## 当前裁剪区（绘制前设置，绘制时自动求交）
var clip_roi: UiRoi = null

## 默认字体（对应 FontConfig 的 id=11, size=15）
var default_font: Font = null
var default_font_size: int = 15


func setup(font_path: String = "", font_size: int = 15) -> void:
	default_font_size = font_size
	if not font_path.is_empty() and FileAccess.file_exists(font_path):
		var ff := FontFile.new()
		if ff.load_dynamic_font(font_path) == OK:
			default_font = ff
	if default_font == null:
		default_font = ThemeDB.fallback_font


## ==================== 纹理 ====================

## sdldevice.hpp:246  drawTexture(SDL_Texture *, int dx, int dy)
## 以 dx,dy 为左上角绘制整张图
func tex_draw(tex: Texture2D, dx: int, dy: int,
		modulate_color: Color = Color.WHITE) -> void:
	if tex == null:
		return
	tex_draw_region(tex, Rect2(0, 0, tex.get_width(), tex.get_height()),
		dx, dy, tex.get_width(), tex.get_height(), modulate_color)


## sdldevice.hpp:247  drawTexture(tex, src..., dst...)
func tex_draw_region(tex: Texture2D, src: Rect2, dx: int, dy: int,
		dw: int, dh: int, modulate_color: Color = Color.WHITE) -> void:
	if tex == null:
		return
	var dst := Rect2(dx, dy, dw, dh)
	# ROI 裁剪：完全在外部就不画（原版靠 viewport 裁剪，效果等价）
	if clip_roi != null and clip_roi.empty():
		return
	draw_texture_rect_region(tex, dst, src, modulate_color)


## 以「中心」为锚点绘制（对应 SDLDevice::drawTexture 的 dir8_t 重载）
## dir_align 决定 dx,dy 指的是哪条边/角
func tex_draw_align(tex: Texture2D, dir_align: int, dx: int, dy: int,
		modulate_color: Color = Color.WHITE) -> void:
	if tex == null:
		return
	var tw := tex.get_width()
	var th := tex.get_height()
	var ox := 0
	var oy := 0
	# 与原版 DIR_* 语义一致（DIR_BEGIN=1 起：UP/UPRIGHT/RIGHT/...）
	match dir_align:
		1:  # DIR_UP       下限对齐，水平居中
			ox = -tw / 2
			oy = 0
		2:  # DIR_UPRIGHT  左下角
			ox = -tw
			oy = 0
		3:  # DIR_RIGHT    右对齐，垂直居中
			ox = -tw
			oy = -th / 2
		4:  # DIR_DOWNRIGHT 右下角
			ox = -tw
			oy = -th
		5:  # DIR_DOWN     上对齐，水平居中
			ox = -tw / 2
			oy = -th
		6:  # DIR_DOWNLEFT 右上角
			ox = 0
			oy = -th
		7:  # DIR_LEFT     左对齐，垂直居中
			ox = 0
			oy = -th / 2
		_:  # DIR_UPLEFT   左上角
			ox = 0
			oy = 0
	tex_draw(tex, dx + ox, dy + oy, modulate_color)


## ==================== 基础图形 ====================

## sdldevice.hpp:281  drawLine
func draw_line_c(color: Color, x0: int, y0: int, x1: int, y1: int, width: int = 1.0) -> void:
	if clip_roi != null and clip_roi.empty():
		return
	draw_line(Vector2(x0, y0), Vector2(x1, y1), color, width)


## sdldevice.hpp:289  drawCross
func draw_cross(color: Color, x: int, y: int, r: int, width: int = 1) -> void:
	draw_line_c(color, x - r, y, x + r, y, width)
	draw_line_c(color, x, y - r, x, y + r, width)


## sdldevice.hpp:335  drawRectangle（filled / 描边由 width 决定）
func draw_rect_c(color: Color, x: int, y: int, w: int, h: int, filled: bool = true) -> void:
	if clip_roi != null and clip_roi.empty():
		return
	if filled:
		draw_rect(Rect2(x, y, w, h), color, true)
	else:
		draw_rect(Rect2(x, y, w, h), color, false, 1.0)


## sdldevice.hpp:335 drawRectangle（边框）
func draw_border(color: Color, x: int, y: int, w: int, h: int, width: int = 1) -> void:
	draw_line_c(color, x, y, x + w - 1, y, width)
	draw_line_c(color, x, y + h - 1, x + w - 1, y + h - 1, width)
	draw_line_c(color, x, y, x, y + h - 1, width)
	draw_line_c(color, x + w - 1, y, x + w - 1, y + h - 1, width)


## ==================== 文字 ====================

## sdldevice.hpp:429  drawString(color, x, y, str)
## 注意：原版的 x,y 是文字「左上角」（SDL_ttf 的 baseline 由上层算）
func draw_string_c(text: String, x: int, y: int,
		color: Color = Color.WHITE, font: Font = null, font_size: int = 0) -> void:
	if text.is_empty():
		return
	if clip_roi != null and clip_roi.empty():
		return
	var f: Font = font if font != null else default_font
	var fs: int = font_size if font_size > 0 else default_font_size
	draw_string(f, Vector2(x, y + fs), text, HORIZONTAL_ALIGNMENT_LEFT, -1, fs, color)


## 带阴影的文字（原版 textshadowboard 的做法）
func draw_string_shadow(text: String, x: int, y: int, color: Color = Color.WHITE,
		shadow_color: Color = Color(0, 0, 0, 0.8), offset: int = 1,
		font: Font = null, font_size: int = 0) -> void:
	draw_string_c(text, x + offset, y + offset, shadow_color, font, font_size)
	draw_string_c(text, x, y, color, font, font_size)


## 计算文字宽度（供布局用）
func measure_text(text: String, font: Font = null, font_size: int = 0) -> Vector2:
	var f: Font = font if font != null else default_font
	var fs: int = font_size if font_size > 0 else default_font_size
	return f.get_string_size(text, HORIZONTAL_ALIGNMENT_LEFT, -1, fs)


# ==================== 子控件绘制调度 ====================

## 由外部设置要绘制的根控件（每帧在 _draw 里画）
var root_widget: UiWidget = null


func _draw() -> void:
	if root_widget == null:
		return
	var vp := get_viewport_rect().size
	draw_widget_tree(root_widget, UiRoi.new(0, 0, int(vp.x), int(vp.y)))


## 绘制整棵控件树（对应 Widget::drawRoot）
## 用 immediately-invoked 方式在每个控件的坐标系里画
func draw_widget_tree(root: UiWidget, screen_roi: UiRoi) -> void:
	if root == null:
		return
	_draw_widget_recursive(root, screen_roi)


func _draw_widget_recursive(widget: UiWidget, parent_roi: UiRoi) -> void:
	if not widget.is_shown():
		return

	# 本控件在屏幕上的绝对矩形
	# parent_roi 已经是「父控件的绝对矩形」，所以只需加自身 position，
	# 不能再累加父的位置（否则父位置会被加两次）
	var abs_x := parent_roi.x + int(widget.position.x)
	var abs_y := parent_roi.y + int(widget.position.y)
	var my_roi := UiRoi.new(abs_x, abs_y, widget.size_w(), widget.size_h())
	# 与父 ROI 求交：超出父容器的部分被裁掉（对应 widget.roi.hpp crop）
	my_roi.crop(parent_roi)
	if my_roi.empty():
		return

	clip_roi = my_roi

	# 自身绘制（三层优先级）：
	#   1. on_draw 回调（外部自定义）
	#   2. 子类 draw_self（UiButton/UiImage 等）
	#   3. 无自身绘制（纯容器）
	if widget.on_draw.is_valid():
		widget.on_draw.call(widget, my_roi)
	elif widget.has_method("draw_self"):
		widget.draw_self(self, my_roi)

	# 关键：无论自身是否绘制，都必须递归子控件
	# （原版 drawDefault 的语义就是「画完自己再画子」，
	#   只有显式替换 drawDefault 才跳过，这里按通用语义处理）
	for c in widget.get_children_widgets():
		if c.is_shown():
			_draw_widget_recursive(c, my_roi)

	clip_roi = null
