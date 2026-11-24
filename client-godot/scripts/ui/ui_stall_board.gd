extends UiWidget
class_name UiStallBoard
## 摆摊面板（卖家布置 / 卖家在售 / 买家浏览 三态）
##
## 【逐行翻译】client/src/gui/stallboard/stallboard.cpp（1007 行）
##            client/src/gui/stallboard/stallitemgrid.cpp
##
## ── 三种模式（stallboard.hpp 注释）──
##   1. 卖家布置：从背包装入物品，可取出，0x910（开始摆摊）
##   2. 卖家在售：可取回物品（需确认），0x860（收摊）
##   3. 买家浏览：选中物品点 0xB7（购买）
##   操作槽同一时间只显示一个按揭
##
## ── 布局（背景 0x00000230 = 282x390，已实测）──
##   卖家名   ( 49,  26) 白色 12pt
##   摊名     (157,  26) 黄色 12pt
##   物品格   ( 17,  58) 6x6，格 249x(6*SYS_INVGRIDPH)=249x192
##   信息行0  ( 60, 330) dir=LEFT 白 12pt：选中时「单价：x」否则「物品：n/36」
##   信息行1  ( 60, 351) dir=LEFT 黄 12pt：卖家「总值：x」买家「金币：x」
##   操作底图 (196, 324) 0x000000B0 = 40x40
##   操作按钮 (199, 327) 三选一：0x000000B7 / 0x00000910 / 0x00000860
##   关闭     (242, 346) 0x0000001C
##
## ── 按钮显隐（stallboard.cpp:296-309，逐条照抄）──
##   showItemOp    = 有选中 && (是卖家 || 在售)
##   showStartStall= 是卖家 && 未在售 && 有物品 && 无选中
##   showCloseStall= 是卖家 && 在售 && 无选中

const ItemGfx := preload("res://scripts/item/item_gfx.gd")
const ItemRecord := preload("res://scripts/item/item_record.gd")

const BG_TEX := 0x00000230
const OP_BG_TEX := 0x000000B0
const BTN_OP := 0x000000B7
const BTN_START := 0x00000910
const BTN_CLOSE_STALL := 0x00000860
const BTN_CLOSE := 0x0000001C

const GRID_COLS := 6
const GRID_ROWS := 6
const GRID_W := 249
const GRID_X := 17
const GRID_Y := 58

## SYS_STALLMAXITEM
const STALL_MAX_ITEM := 36
## SYS_INVGRIDPW / PH
const INV_GRID_W := 38
const INV_GRID_H := 32

# ==================== 状态 ====================

## 摊位数据：{ dbid, name, seller_uid, selling, item_list: [{ item_id, count, price }] }
var stall: Dictionary = {
	"dbid": 0, "name": "", "seller_uid": 0, "selling": false, "item_list": [],
}
var seller_name: String = ""
var my_gold := 0
var my_db_id := 0

## 选中格（-1 无）
var selected := -1
var hover := -1

var on_item_op: Callable = Callable()
var on_start_stall: Callable = Callable()
var on_close_stall: Callable = Callable()
var on_close: Callable = Callable()
var on_buy: Callable = Callable()      ## Callable(idx, price)

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init(p_name: String = "StallBoard") -> void:
	widget_name = p_name
	w = 282
	h = 390
	show = false
	on_draw = _draw_board


func set_canvas(c) -> void:
	_canvas_ref = c


func load_stall(p_stall: Dictionary, p_seller_name: String, p_my_db_id: int) -> void:
	stall = p_stall.duplicate(true)
	if not stall.has("item_list"):
		stall["item_list"] = []
	seller_name = p_seller_name
	my_db_id = p_my_db_id
	selected = -1
	hover = -1
	show = true


## 是否是摊主（isSeller：比对 dbid 与自己的 dbid）
func is_seller() -> bool:
	return int(stall.get("seller_uid", 0)) == my_db_id and my_db_id != 0


func selling() -> bool:
	return bool(stall.get("selling", false))


func item_list() -> Array:
	return stall.get("item_list", [])


func selected_item():
	var lst := item_list()
	if selected < 0 or selected >= lst.size():
		return null
	return lst[selected]


func selected_price() -> int:
	var it = selected_item()
	if it == null:
		return 0
	return int(it.get("price", 0))


# ==================== 按钮显隐 ====================

func show_item_op() -> bool:
	return selected_item() != null and (is_seller() or selling())


func show_start_stall() -> bool:
	return is_seller() and not selling() and not item_list().is_empty() \
		and selected_item() == null


func show_close_stall() -> bool:
	return is_seller() and selling() and selected_item() == null


func show_op_any() -> bool:
	return show_item_op() or show_start_stall() or show_close_stall()


# ==================== 总价 ====================

func total_value() -> int:
	var t := 0
	for it in item_list():
		t += int(it.get("price", 0)) * int(it.get("count", 0))
	return t


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
		canvas.draw_rect_c(Color(0.1, 0.09, 0.07, 1.0), roi.x, roi.y, 282, 390, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), roi.x, roi.y, 282, 390, 2)

	# 卖家名（白）
	canvas.draw_string_c(seller_name, roi.x + 49, roi.y + 26, Color(1, 1, 1), null, 12)
	# 摊名（黄）
	canvas.draw_string_c(str(stall.get("name", "")), roi.x + 157, roi.y + 26,
		Color(1, 1, 0), null, 12)

	# 物品格 6x6
	var lst := item_list()
	for i in range(GRID_COLS * GRID_ROWS):
		var col: int = i % GRID_COLS
		var row: int = i / GRID_COLS
		var cx: int = roi.x + GRID_X + col * INV_GRID_W
		var cy: int = roi.y + GRID_Y + row * INV_GRID_H
		var cell_w := INV_GRID_W - 2
		var cell_h := INV_GRID_H - 2

		# 选中/悬停底色
		if i == selected:
			canvas.draw_rect_c(Color(0.9, 0.75, 0.2, 0.35), cx, cy, cell_w, cell_h, true)
		elif i == hover:
			canvas.draw_rect_c(Color(1, 1, 1, 0.12), cx, cy, cell_w, cell_h, true)

		if i < lst.size():
			var it: Dictionary = lst[i]
			var iid := int(it.get("item_id", 0))
			var cnt := int(it.get("count", 0))
			var tex := ItemGfx.icon_of_item(iid)
			if tex != null:
				var off := ItemGfx.center_offset(tex, cell_w, cell_h)
				canvas.tex_draw(tex, cx + off.x, cy + off.y)
			if cnt > 1:
				canvas.draw_string_shadow(str(cnt), cx + cell_w - 14, cy + cell_h - 14,
					Color(1, 1, 0.6), Color(0, 0, 0, 0.9), 1, null, 10)
		hit_areas["cell_%d" % i] = Rect2(cx, cy, cell_w, cell_h)

	# 信息行 0（dir=LEFT：右对齐到 x=60，这里直接左起）
	var line0 := ""
	if not is_seller() and not selling():
		line0 = "摊位已关闭"
	elif selected_item() != null:
		line0 = "单价：%s" % UiDirectTradeBoard.ksep(selected_price())
	else:
		line0 = "物品：%d/%d" % [lst.size(), STALL_MAX_ITEM]
	canvas.draw_string_c(line0, roi.x + 60, roi.y + 330, Color(1, 1, 1), null, 12)

	# 信息行 1（黄）
	var line1 := ""
	if is_seller():
		line1 = "总值：%s" % UiDirectTradeBoard.ksep(total_value())
	else:
		line1 = "金币：%s" % UiDirectTradeBoard.ksep(my_gold)
	canvas.draw_string_c(line1, roi.x + 60, roi.y + 351, Color(1, 1, 0), null, 12)

	# 操作底图 + 按钮（三选一）
	if show_op_any():
		var opbg := ProgUseTex.retrieve(OP_BG_TEX)
		if opbg != null:
			canvas.tex_draw(opbg, roi.x + 196, roi.y + 324)
		var btn_tex := 0
		var act := ""
		if show_item_op():
			btn_tex = BTN_OP
			act = "itemop"
		elif show_start_stall():
			btn_tex = BTN_START
			act = "startstall"
		elif show_close_stall():
			btn_tex = BTN_CLOSE_STALL
			act = "closestall"
		var btex := ProgUseTex.retrieve(btn_tex)
		if btex != null:
			canvas.tex_draw(btex, roi.x + 199, roi.y + 327)
			hit_areas[act] = Rect2(roi.x + 199, roi.y + 327, 34, 34)
		else:
			var lbl: String = str({"itemop": "购买" if not is_seller() else "取出",
				"startstall": "摆摊", "closestall": "收摊"}.get(act, "操作"))
			canvas.draw_rect_c(Color(0.3, 0.26, 0.14, 0.95), roi.x + 199, roi.y + 327, 34, 34, true)
			canvas.draw_string_c(str(lbl), roi.x + 201, roi.y + 332, Color(1, 0.95, 0.6), null, 12)
			hit_areas[act] = Rect2(roi.x + 199, roi.y + 327, 34, 34)

	# 关闭
	var ctex := ProgUseTex.retrieve(BTN_CLOSE)
	if ctex != null:
		canvas.tex_draw(ctex, roi.x + 242, roi.y + 346)
		hit_areas["close"] = Rect2(roi.x + 242, roi.y + 346, 32, 30)
	else:
		canvas.draw_string_c("×", roi.x + 242, roi.y + 346, Color(0.9, 0.75, 0.5), null, 18)
		hit_areas["close"] = Rect2(roi.x + 242, roi.y + 346, 20, 20)

	# 选中物品时画悬停说明（原版 drawItemHoverText 在 drawBoardPX+10, +10）
	if selected_item() != null:
		var it2: Dictionary = selected_item()
		var nm := ItemRecord.name(int(it2.get("item_id", 0)))
		canvas.draw_string_shadow(nm, roi.x + 10, roi.y + 10,
			Color(1, 1, 0.85), Color(0, 0, 0, 0.9), 1, null, 13)



# ==================== 事件 ====================

func handle_move(px: int, py: int) -> void:
	hover = -1
	for i in range(GRID_COLS * GRID_ROWS):
		var r: Rect2 = hit_areas.get("cell_%d" % i, Rect2())
		if r.size.x > 0 and px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			hover = i
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
			if k.begins_with("cell_"):
				var idx: int = int(k.substr(5))
				# 空格不改变选中；有物品则切换选中
				if idx < item_list().size():
					selected = -1 if selected == idx else idx
				return true
			if k == "itemop":
				if show_item_op():
					# 买家 -> 购买；卖家 -> 取出
					if not is_seller() and on_buy.is_valid():
						on_buy.call(selected, selected_price())
					elif on_item_op.is_valid():
						on_item_op.call(self)
				return true
			if k == "startstall":
				if on_start_stall.is_valid():
					on_start_stall.call(self)
				return true
			if k == "closestall":
				if on_close_stall.is_valid():
					on_close_stall.call(self)
				return true
	return false
