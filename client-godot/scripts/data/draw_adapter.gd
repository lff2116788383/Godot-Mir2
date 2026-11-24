extends RefCounted
class_name DrawAdapter
## 把 Godot 的 CanvasItem 绘制接口包装成 UiCanvas 风格的 API
##
## ── 为什么需要 ──
##   UI 层的绘制函数统一按 UiCanvas 的接口写（tex_draw / tex_draw_region /
##   draw_rect_c / draw_border / draw_string_c / draw_circle_c …），
##   但地图物件层是裸 Node2D（每帧 _draw）。
##   与其把物件绘制函数改成 Node2D 方言，不如在最薄的一层做转接 ——
##   这样 map_objects.gd 里的绘制逻辑和 UI 复用同一套写法，不容易写错。
##
##   注意：Node2D 的 draw_* 只能在 _draw 回调里调用，
##   本适配器实例只在对应 layer 的 draw 信号处理期间可用。

var _node: CanvasItem = null


func _init(p_node: CanvasItem) -> void:
	_node = p_node


func tex_draw(tex: Texture2D, dx: int, dy: int,
		modulate_color: Color = Color.WHITE) -> void:
	if tex == null or _node == null:
		return
	_node.draw_texture(tex, Vector2(dx, dy), modulate_color)


func tex_draw_region(tex: Texture2D, src: Rect2, dx: int, dy: int,
		dw: int, dh: int, modulate_color: Color = Color.WHITE) -> void:
	if tex == null or _node == null:
		return
	if dw <= 0 or dh <= 0:
		return
	_node.draw_texture_rect_region(tex, Rect2(dx, dy, dw, dh), src, modulate_color)


func draw_rect_c(color: Color, x: int, y: int, w: int, h: int,
		filled: bool = true) -> void:
	if _node == null or w <= 0 or h <= 0:
		return
	_node.draw_rect(Rect2(x, y, w, h), color, filled, 1.0)


func draw_border(color: Color, x: int, y: int, w: int, h: int,
		width: int = 1) -> void:
	if _node == null:
		return
	var pts := [
		Vector2(x, y), Vector2(x + w - 1, y),
		Vector2(x + w - 1, y + h - 1), Vector2(x, y + h - 1),
	]
	for i in range(4):
		_node.draw_line(pts[i], pts[(i + 1) % 4], color, float(width))


func draw_line_c(color: Color, x0: int, y0: int, x1: int, y1: int,
		width: int = 1) -> void:
	if _node == null:
		return
	_node.draw_line(Vector2(x0, y0), Vector2(x1, y1), color, float(width))


func draw_circle_c(color: Color, cx: int, cy: int, r: int,
		filled: bool = true) -> void:
	if _node == null or r <= 0:
		return
	if filled:
		_node.draw_circle(Vector2(cx, cy), float(r), color)
	else:
		_node.draw_arc(Vector2(cx, cy), float(r), 0.0, TAU, maxi(12, r * 2), color, 1.0)


func draw_string_c(text: String, x: int, y: int, color: Color = Color.WHITE,
		font: Font = null, font_size: int = 15) -> void:
	if _node == null or text.is_empty():
		return
	var f: Font = font if font != null else ThemeDB.fallback_font
	_node.draw_string(f, Vector2(x, y + font_size), text,
		HORIZONTAL_ALIGNMENT_LEFT, -1, font_size, color)


func measure_text(text: String, font: Font = null, font_size: int = 15) -> Vector2:
	var f: Font = font if font != null else ThemeDB.fallback_font
	return f.get_string_size(text, HORIZONTAL_ALIGNMENT_LEFT, -1, font_size)
