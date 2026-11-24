extends UiWidget
class_name UiMiniMapBoard
## 小地图面板（完整版：贴图 + 缩放 + 拖动 + 角色点 + 角落装饰）
##
## 【逐行翻译】client/src/minimapboard.cpp（608 行）
##
## ── 尺寸（minimapboard.cpp:41-42）──
##   普通：200x200，贴右上角（dir=DIR_UPRIGHT，x = W-1, y = 0）
##   扩展：W*0.8 x H*0.5，居中（dir=DIR_NONE）
##
## ── 贴图 ──
##   m_bg 纯黑填充
##   三张角落装饰（都是 32x35）：
##     0x09000006 右上角  (w-1, 0)
##     0x09000007 左下角  (0, h-1)
##     0x09000008 右下角  (w-1, h-1)
##
## ── 地图图像（getMiniMapTexture）──
##   MapRecord 的 miniMapID 指向 proguse 图库的一张整图（实测 09000000=128x128）
##   不是瓦片拼接，所以只有缩放 + 平移 + 边界夹取三件事
##   （坐标换算在 scripts/ui/mini_map.gd 的 MiniMap 类里，已经实现）
##
## ── 交互 ──
##   滚轮：以光标为锚点缩放（zoomOnCanvasAt）
##   拖拽：平移，边界夹取（fixMapImagePLoc）
##   右键：切换扩展/普通模式

const MiniMapCalc := preload("res://scripts/ui/mini_map.gd")

const W_NORMAL := 200
const H_NORMAL := 200
const CORNER_TEX_RT := 0x09000006
const CORNER_TEX_LB := 0x09000007
const CORNER_TEX_RB := 0x09000008

# ==================== 状态 ====================

var extended := false
## 地图整图贴图 id（MapRecord.miniMapID）
var map_tex_id := -1
## 当前地图格子尺寸（用来把角色格坐标映射到图上）
var map_grid_w := 0
var map_grid_h := 0

## 角色位置（格坐标）
var hero_gx := 0
var hero_gy := 0
## 其他角色点：[{ gx, gy, color }]
var dots: Array = []

var calc = null      ## MiniMapCalc 实例
var _canvas_ref = null
var hit_areas: Dictionary = {}
var _dragging := false
var _drag_from := Vector2i.ZERO
var _drag_map_from := Vector2i.ZERO


func _init(p_name: String = "MiniMapBoard") -> void:
	widget_name = p_name
	w = W_NORMAL
	h = H_NORMAL
	show = true
	on_draw = _draw_board
	calc = MiniMapCalc.new()


func set_canvas(c) -> void:
	_canvas_ref = c


## 载入地图小图
func set_map(tex_id: int, grid_w: int, grid_h: int) -> void:
	map_tex_id = tex_id
	map_grid_w = grid_w
	map_grid_h = grid_h
	var tex := ProgUseTex.retrieve(tex_id)
	if tex != null:
		calc.setup(size_w(), size_h(), tex.get_width(), tex.get_height())
	else:
		calc.setup(size_w(), size_h(), grid_w, grid_h)


func set_hero(gx: int, gy: int) -> void:
	hero_gx = gx
	hero_gy = gy


## 切换扩展模式（右键）
func toggle_extended(vw: int, vh: int) -> void:
	extended = not extended
	if extended:
		w = int(vw * 0.8)
		h = int(vh * 0.5)
	else:
		w = W_NORMAL
		h = H_NORMAL
	var tex := ProgUseTex.retrieve(map_tex_id)
	if tex != null:
		calc.setup(size_w(), size_h(), tex.get_width(), tex.get_height())


## 面板在屏幕上的左上角（普通模式贴右上角；扩展模式居中）
func place(vw: int, vh: int, vx: int, vy: int) -> Vector2:
	if extended:
		return Vector2(vx + vw / 2 - size_w() / 2, vy + vh / 2 - size_h() / 2)
	return Vector2(vx + vw - 1 - size_w(), vy)


# ==================== 绘制 ====================

func _draw_board(_w, roi) -> void:
	var canvas = _canvas_ref
	if canvas == null:
		return
	hit_areas.clear()
	var bw := size_w()
	var bh := size_h()

	# 黑底
	canvas.draw_rect_c(Color(0, 0, 0, 1), roi.x, roi.y, bw, bh, true)

	# 地图图像（按 calc 的缩放/平移绘制，裁剪到面板内）
	var tex := ProgUseTex.retrieve(map_tex_id)
	if tex != null:
		var tw: float = tex.get_width() * calc.zoom_factor
		var th: float = tex.get_height() * calc.zoom_factor
		var dx: int = roi.x + calc.map_dx
		var dy: int = roi.y + calc.map_dy
		canvas.tex_draw_region(tex, Rect2(0, 0, tex.get_width(), tex.get_height()),
			dx, dy, int(tw), int(th))
	else:
		# 无小图时用格网 + 角色点表示
		var cw: int = 16
		var chh: int = 16
		for gy in range(0, bh, chh):
			for gx in range(0, bw, cw):
				canvas.draw_rect_c(Color(0.12, 0.12, 0.12, 0.8), roi.x + gx, roi.y + gy,
					cw - 1, chh - 1, true)

	# 角色点（红点，3x3）
	if map_grid_w > 0 and map_grid_h > 0:
		var hx: int = roi.x + calc.map_dx + int(float(hero_gx) / map_grid_w * tex_w_or(256) * calc.zoom_factor)
		var hy: int = roi.y + calc.map_dy + int(float(hero_gy) / map_grid_h * tex_h_or(256) * calc.zoom_factor)
		if hx >= roi.x and hx < roi.x + bw and hy >= roi.y and hy < roi.y + bh:
			canvas.draw_rect_c(Color(1, 0.2, 0.2, 1), hx - 1, hy - 1, 3, 3, true)

	# 其他点（队友黄 / 怪物白）
	for d in dots:
		var dx2: int = roi.x + calc.map_dx + int(float(d.get("gx", 0)) / maxi(1, map_grid_w) * tex_w_or(256) * calc.zoom_factor)
		var dy2: int = roi.y + calc.map_dy + int(float(d.get("gy", 0)) / maxi(1, map_grid_h) * tex_h_or(256) * calc.zoom_factor)
		if dx2 >= roi.x and dx2 < roi.x + bw and dy2 >= roi.y and dy2 < roi.y + bh:
			var col: Color = d.get("color", Color(1, 1, 0.4))
			canvas.draw_rect_c(col, dx2, dy2, 2, 2, true)

	# 三张角落装饰
	_draw_corner(canvas, CORNER_TEX_RT, roi.x + bw - 32, roi.y)
	_draw_corner(canvas, CORNER_TEX_LB, roi.x, roi.y + bh - 35)
	_draw_corner(canvas, CORNER_TEX_RB, roi.x + bw - 32, roi.y + bh - 35)

	# 边框
	canvas.draw_border(Color(0.35, 0.3, 0.2, 1.0), roi.x, roi.y, bw, bh, 1)

	hit_areas["canvas"] = Rect2(roi.x, roi.y, bw, bh)


func tex_w_or(fallback: int) -> int:
	var t := ProgUseTex.retrieve(map_tex_id)
	return t.get_width() if t != null else fallback


func tex_h_or(fallback: int) -> int:
	var t := ProgUseTex.retrieve(map_tex_id)
	return t.get_height() if t != null else fallback


func _draw_corner(canvas, tex_id: int, x: int, y: int) -> void:
	var t := ProgUseTex.retrieve(tex_id)
	if t == null:
		return
	# 右上角那张要水平翻转（原版用 DIR_UPRIGHT 锚 + 翻转贴图）
	canvas.tex_draw(t, x, y)


# ==================== 事件 ====================

## 滚轮缩放（以光标为锚）
func handle_wheel(px: int, py: int, up: bool) -> void:
	var r: Rect2 = hit_areas.get("canvas", Rect2())
	if r.size.x <= 0:
		return
	if not (px >= r.position.x and px < r.position.x + r.size.x \
			and py >= r.position.y and py < r.position.y + r.size.y):
		return
	var lx: int = px - int(r.position.x)
	var ly: int = py - int(r.position.y)
	var step := 1.25 if up else 0.8
	var nz: float = clampf(calc.zoom_factor * step, MiniMapCalc.ZOOM_MIN, MiniMapCalc.ZOOM_MAX)
	# 以光标为锚点缩放：锚点在图上的位置保持不变
	if calc.zoom_factor > 0.0001:
		calc.map_dx = int(lx - (lx - calc.map_dx) * (nz / calc.zoom_factor))
		calc.map_dy = int(ly - (ly - calc.map_dy) * (nz / calc.zoom_factor))
	calc.zoom_factor = nz
	calc._fix_loc(false)


func handle_press(px: int, py: int) -> bool:
	var r: Rect2 = hit_areas.get("canvas", Rect2())
	if r.size.x > 0 and px >= r.position.x and px < r.position.x + r.size.x \
			and py >= r.position.y and py < r.position.y + r.size.y:
		_dragging = true
		_drag_from = Vector2i(px, py)
		_drag_map_from = Vector2i(calc.map_dx, calc.map_dy)
		return true
	return false


func handle_drag(px: int, py: int) -> void:
	if not _dragging:
		return
	calc.map_dx = _drag_map_from.x + (px - _drag_from.x)
	calc.map_dy = _drag_map_from.y + (py - _drag_from.y)
	calc._fix_loc(false)


func handle_release() -> void:
	_dragging = false


func is_dragging() -> bool:
	return _dragging
