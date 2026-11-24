extends UiWidget
class_name UiHorseBoard
## 马匹面板
##
## 【逐行翻译】client/src/horseboard.cpp（194 行）+ horseboard.hpp
##
## ── 布局（horseboard.cpp，逐值）──
##   面板 257x322
##   背景 0x00000700（257x322，已实测）
##   关闭按钮 (217, 278) 0x0000001C
##   四个操作按钮 y=231，x = 20 / 77 / 134 / 191：
##     0x00000710 上马
##     0x00000720 下马
##     0x00000730 隐藏
##     0x00000740 显示
##   （原版这四颗是「马匹展示模式」的上下/隐藏/显示控制，
##     马匹形象本身由 ProcessRun 在世界里渲染，面板只做开关）

const BG_TEX := 0x00000700
const BTN_CLOSE := 0x0000001C

const BTN_UP := 0x00000710
const BTN_DOWN := 0x00000720
const BTN_HIDE := 0x00000730
const BTN_SHOW := 0x00000740
const BTN_Y := 231
const BTN_XS := [20, 77, 134, 191]

const BOARD_W := 257
const BOARD_H := 322

# ==================== 状态 ====================

## 骑乘状态：0=未骑马 1=骑乘 2=隐藏（原版 m_hide/m_show 两个开关）
var riding := false
var hide_mode := false
## 马匹外观 id（用于显示）
var horse_look := 0

var on_close: Callable = Callable()
var on_mount: Callable = Callable()
var on_dismount: Callable = Callable()
var on_hide: Callable = Callable()
var on_show: Callable = Callable()

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init(p_name: String = "HorseBoard") -> void:
	widget_name = p_name
	w = BOARD_W
	h = BOARD_H
	show = false
	on_draw = _draw_board


func set_canvas(c) -> void:
	_canvas_ref = c


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
		canvas.draw_rect_c(Color(0.09, 0.08, 0.07, 1.0), roi.x, roi.y, BOARD_W, BOARD_H, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), roi.x, roi.y, BOARD_W, BOARD_H, 2)

	# 状态文字
	var state := "未骑马"
	if hide_mode:
		state = "已隐藏"
	elif riding:
		state = "骑乘中"
	canvas.draw_string_c(state, roi.x + 20, roi.y + 20, Color(1, 0.9, 0.55), null, 15)
	if horse_look > 0:
		canvas.draw_string_c("外观 #%d" % horse_look, roi.x + 20, roi.y + 42,
			Color(0.8, 0.78, 0.7), null, 12)

	# 四颗按钮
	var texs := [BTN_UP, BTN_DOWN, BTN_HIDE, BTN_SHOW]
	var acts := ["mount", "dismount", "hide", "show"]
	var labels := ["上马", "下马", "隐藏", "显示"]
	for i in range(4):
		var bx: int = roi.x + int(BTN_XS[i])
		var by: int = roi.y + BTN_Y
		var tex := ProgUseTex.retrieve(int(texs[i]))
		# 不可用的置灰（原版按状态控制 active）
		var enabled := _is_enabled(acts[i])
		if tex != null:
			canvas.tex_draw(tex, bx, by, Color(1, 1, 1, 1) if enabled else Color(0.5, 0.5, 0.5, 0.5))
			hit_areas[acts[i]] = Rect2(bx, by, tex.get_width(), tex.get_height())
		else:
			canvas.draw_rect_c(Color(0.22, 0.19, 0.12, 0.95), bx, by, 40, 20, true)
			canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), bx, by, 40, 20, 1)
			canvas.draw_string_c(labels[i], bx + 6, by + 3,
				Color(1, 0.95, 0.72) if enabled else Color(0.5, 0.5, 0.5), null, 12)
			hit_areas[acts[i]] = Rect2(bx, by, 40, 20)

	# 关闭
	var ctex := ProgUseTex.retrieve(BTN_CLOSE)
	if ctex != null:
		canvas.tex_draw(ctex, roi.x + 217, roi.y + 278)
		hit_areas["close"] = Rect2(roi.x + 217, roi.y + 278, 32, 30)
	else:
		canvas.draw_string_c("×", roi.x + 217, roi.y + 278, Color(0.9, 0.75, 0.5), null, 18)
		hit_areas["close"] = Rect2(roi.x + 217, roi.y + 278, 20, 20)


## 按钮可用性：上马要未骑，下马要已骑，隐藏要可见，显示要已隐藏
func _is_enabled(act: String) -> bool:
	match act:
		"mount":
			return not riding and not hide_mode
		"dismount":
			return riding and not hide_mode
		"hide":
			return not hide_mode
		"show":
			return hide_mode
	return true


# ==================== 事件 ====================

func handle_click(px: int, py: int) -> bool:
	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			var k := str(key)
			if k == "close":
				show = false
				if on_close.is_valid():
					on_close.call(self)
				return true
			if not _is_enabled(k):
				return true
			match k:
				"mount":
					riding = true
					if on_mount.is_valid():
						on_mount.call()
				"dismount":
					riding = false
					if on_dismount.is_valid():
						on_dismount.call()
				"hide":
					hide_mode = true
					if on_hide.is_valid():
						on_hide.call()
				"show":
					hide_mode = false
					if on_show.is_valid():
						on_show.call()
			return true
	return false
