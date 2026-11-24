extends UiWidget
class_name UiQuickAccessBoard
## 快捷栏（腰带 6 格）
##
## 【逐行翻译】client/src/gui/quickaccessboard/quickaccessboard.cpp（369 行）
##            client/src/gui/quickaccessboard/quickaccessgrid.cpp
##
## ── 布局常量 ──
##   背景贴图 0x00000060 = 280x48（已实测）
##   getGridLoc(slot) = { 17 + 42 * slot, 6, 36, 36 }   共 6 格
##   关闭按钮 0x00000061（16x16）在 (263, 32)
##   面板默认 show = false
##
## ── 每格内部（quickaccessgrid.cpp）──
##   bg    ：悬停时填充 WHITE + A_SHF(64)，即 25% 白
##   item  ：dir=DIR_NONE，锚点在格中心 (w/2, h/2)；贴图 = pkgGfxID | 0x01000000
##   count ：dir=DIR_UPRIGHT，锚点 (w-1, 0)，字号 10，count > 1 才显示
##
## ── 数据来源 ──
##   proc->getMyHero()->getBelt(slot) -> SDItem
##   belt 是腰带容器，6 格，对应键盘 1..6

const BG_TEX := 0x00000060
const CLOSE_TEX := 0x00000061
const SLOT_COUNT := 6
const GRID_W := 36
const GRID_H := 36

## 背景实测尺寸
const BOARD_W := 280
const BOARD_H := 48

## 关闭按钮坐标
const CLOSE_X := 263
const CLOSE_Y := 32

const ItemGfxRef := preload("res://scripts/item/item_gfx.gd")
const ItemRecordRef := preload("res://scripts/item/item_record.gd")

## 腰带物品：[{ "item_id": int, "count": int }, ...] 共 6 项，空位用 null
var belt: Array = []

## 悬停格（-1 无）
var hover_slot := -1
## 回调
var on_slot_click: Callable = Callable()   ## Callable(slot)
var on_close: Callable = Callable()

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init(p_name: String = "QuickAccessBoard") -> void:
	widget_name = p_name
	w = BOARD_W
	h = BOARD_H
	show = false
	on_draw = _draw_board


func set_canvas(c) -> void:
	_canvas_ref = c


## quickaccessboard.hpp:getGridLoc
static func grid_loc(slot: int) -> Rect2:
	if slot < 0 or slot >= SLOT_COUNT:
		return Rect2(0, 0, 0, 0)
	return Rect2(17 + 42 * slot, 6, GRID_W, GRID_H)


func set_belt(items: Array) -> void:
	belt = items.duplicate()
	while belt.size() < SLOT_COUNT:
		belt.append(null)


func get_belt(slot: int):
	if slot < 0 or slot >= belt.size():
		return null
	return belt[slot]


# ==================== 绘制 ====================

func _draw_board(_w, roi) -> void:
	var canvas = _canvas_ref
	if canvas == null:
		return
	hit_areas.clear()

	# 背景 280x48
	var bg := ProgUseTex.retrieve(BG_TEX)
	if bg != null:
		canvas.tex_draw(bg, roi.x, roi.y)
	else:
		canvas.draw_rect_c(Color(0.1, 0.09, 0.07, 0.94), roi.x, roi.y, BOARD_W, BOARD_H, true)
		canvas.draw_border(Color(0.48, 0.4, 0.25, 1.0), roi.x, roi.y, BOARD_W, BOARD_H, 1)

	# 6 格
	for slot in range(SLOT_COUNT):
		var gl := grid_loc(slot)
		var gx: int = roi.x + int(gl.position.x)
		var gy: int = roi.y + int(gl.position.y)

		# 悬停：25% 白填充
		if slot == hover_slot:
			canvas.draw_rect_c(Color(1, 1, 1, 64.0 / 255.0), gx, gy, GRID_W, GRID_H, true)

		var it = get_belt(slot)
		if it != null:
			var iid: int = int(it.get("item_id", 0)) if it is Dictionary else int(it.item_id)
			var cnt: int = int(it.get("count", 0)) if it is Dictionary else int(it.count)
			var tex := ItemGfxRef.icon_of_item(iid)
			if tex != null:
				# dir=DIR_NONE：锚点在格中心
				var off := ItemGfxRef.center_offset(tex, GRID_W, GRID_H)
				canvas.tex_draw(tex, gx + off.x, gy + off.y)
			else:
				var nm := ItemRecordRef.name(iid)
				if not nm.is_empty():
					canvas.draw_string_c(nm.substr(0, 1), gx + 12, gy + 10,
						Color(0.9, 0.85, 0.7), null, 13)

			# 数量：DIR_UPRIGHT 在 (w-1, 0)，字号 10
			if cnt > 1:
				var cs := str(cnt)
				var cw: int = int(canvas.measure_text(cs, null, 10).x)
				canvas.draw_string_shadow(cs, gx + GRID_W - 1 - cw, gy,
					Color(1, 1, 0.6), Color(0, 0, 0, 0.9), 1, null, 10)

		hit_areas["slot_%d" % slot] = Rect2(gx, gy, GRID_W, GRID_H)

	# 关闭按钮
	var ctex := ProgUseTex.retrieve(CLOSE_TEX)
	var cx: int = roi.x + CLOSE_X
	var cy: int = roi.y + CLOSE_Y
	if ctex != null:
		canvas.tex_draw(ctex, cx, cy)
		hit_areas["close"] = Rect2(cx, cy, ctex.get_width(), ctex.get_height())
	else:
		canvas.draw_string_c("×", cx, cy, Color(0.9, 0.75, 0.5), null, 14)
		hit_areas["close"] = Rect2(cx, cy, 16, 16)


# ==================== 事件 ====================

func handle_move(px: int, py: int) -> void:
	hover_slot = -1
	for slot in range(SLOT_COUNT):
		var r: Rect2 = hit_areas.get("slot_%d" % slot, Rect2())
		if r.size.x > 0 and px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			hover_slot = slot
			return


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
			if k.begins_with("slot_"):
				var slot: int = int(k.substr(5))
				if on_slot_click.is_valid():
					on_slot_click.call(slot)
				return true
	return false


## 使用某格（消耗一个）
func consume_slot(slot: int) -> bool:
	var it = get_belt(slot)
	if it == null:
		return false
	var cnt: int = int(it.get("count", 0)) if it is Dictionary else int(it.count)
	if cnt <= 1:
		belt[slot] = null
	else:
		if it is Dictionary:
			it["count"] = cnt - 1
		else:
			it.count = cnt - 1
	return true
