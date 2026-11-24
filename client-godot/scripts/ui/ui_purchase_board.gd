extends UiWidget
class_name UiPurchaseBoard
## NPC 商店购买面板（小格列表 + 展开详情）
##
## 【逐行翻译】client/src/gui/purchaseboard/purchaseboard.cpp（901 行）
##            client/src/gui/purchaseboard/purchaseboard.hpp（117 行）
##
## ── 布局（purchaseboard.hpp 的 ASCII 图，逐值）──
##   m_startX = 19, m_startY = 15
##   m_boxW = 57 - 19 = 38,  m_boxH = 53 - 15 = 38      单个小格
##   m_lineW = 252 - 19 = 233, m_lineH = 57 - 15 = 42   行距
##   主背景 = 0x08000000 + extendedBoardGfxID()
##     extendedBoardGfxID(): Ext1(不可打包) = 1, Ext2(可打包) = 2
##     实测 08000000 = 290x224（无扩展）
##          08000001 = 488x224（Ext1 展开）
##          08000002 = 514x224（Ext2 展开）
##   关闭按钮 (257, 183) 0x0000001C
##   选定按钮 (105, 185) texAniOpt = 0x08000003（58x22）
##
## ── 展开面板区域（hpp 的 Ext1 / Ext2）──
##   Ext1 格子区 { x=313, y=41, w=152, h=114 }
##   Ext2 格子区 { x=303, y=16, w=38,  h=38 }
##   Ext2 价格区 { x=350, y=16, w=150, h=38 }
##
## ── 分页 ──
##   getStartIndex()：每页格子数由列表区高/行距决定，原版按 m_lineH 计算
##   这里按「主区可容纳行数」算：rows = (224 - 15 - 15) / 42 ≈ 4 行，
##   每行 6 列（233/38 ≈ 6）-> 每页 24 格

const ItemGfx := preload("res://scripts/item/item_gfx.gd")
const ItemRecord := preload("res://scripts/item/item_record.gd")
const ItemRecordRef := preload("res://scripts/item/item_record.gd")

## 主区格子
const START_X := 19
const START_Y := 15
const BOX_W := 38
const BOX_H := 38
const LINE_W := 233
const LINE_H := 42

## 主区背景尺寸（08000000）
const BOARD_W := 290
const BOARD_H := 224

## 扩展区（hpp 的 m_ext1GridArea / m_ext2GridArea / m_ext2PriceArea）
const EXT1_GRID_X := 313
const EXT1_GRID_Y := 41
const EXT1_GRID_W := 152
const EXT1_GRID_H := 114
const EXT2_GRID_X := 303
const EXT2_GRID_Y := 16
const EXT2_GRID_W := 38
const EXT2_GRID_H := 38
const EXT2_PRICE_X := 350
const EXT2_PRICE_Y := 16
const EXT2_PRICE_W := 150
const EXT2_PRICE_H := 38

## 按钮
const BTN_CLOSE := 0x0000001C
const BTN_SELECT := 0x08000003

## 每页格数：4 行 x 6 列
const COLS := 6
const ROWS := 4
const PAGE_SIZE := COLS * ROWS

# ==================== 状态 ====================

## 出售列表：[{ item_id, count, price }]
var items: Array = []
var npc_uid := 0
var selected := 0
var page := 0

## 扩展面板：0=无, 1=Ext1(不可打包), 2=Ext2(可打包)
var ext_mode := 0
## Ext1 选中格
var ext1_selected := -1
var ext1_page := 0

var on_close: Callable = Callable()
var on_buy: Callable = Callable()     ## Callable(item_id, seq_id, price)
var on_select: Callable = Callable()  ## Callable(item_id)

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init(p_name: String = "PurchaseBoard") -> void:
	widget_name = p_name
	w = BOARD_W
	h = BOARD_H
	show = false
	on_draw = _draw_board


func set_canvas(c) -> void:
	_canvas_ref = c


func load_sell(p_npc_uid: int, p_items: Array) -> void:
	npc_uid = p_npc_uid
	items = p_items.duplicate(true)
	selected = 0
	page = 0
	ext_mode = 0
	ext1_selected = -1
	show = true


func page_count() -> int:
	return maxi(1, (items.size() + PAGE_SIZE - 1) / PAGE_SIZE)


func page_items() -> Array:
	var s := page * PAGE_SIZE
	return items.slice(s, mini(s + PAGE_SIZE, items.size()))


func selected_item_id() -> int:
	var pi := page_items()
	if selected < 0 or selected >= pi.size():
		return 0
	return int(pi[selected].get("item_id", 0))


func selected_price() -> int:
	var pi := page_items()
	if selected < 0 or selected >= pi.size():
		return 0
	return int(pi[selected].get("price", 0))


## 扩展面板 gfx（purchaseboard.cpp:extendedBoardGfxID）
func board_tex_id() -> int:
	return 0x08000000 + ext_mode


func board_w() -> int:
	match ext_mode:
		1: return 488
		2: return 514
		_: return BOARD_W


## 格子坐标（getExt1PageGridLoc，按 4 列 x 3 行排）
static func ext1_grid_loc(idx: int) -> Rect2:
	var col: int = idx % 4
	var row: int = idx / 4
	var cw: int = EXT1_GRID_W / 4
	var ch: int = EXT1_GRID_H / 3
	return Rect2(EXT1_GRID_X + col * cw, EXT1_GRID_Y + row * ch, cw, ch)


# ==================== 绘制 ====================

func _draw_board(_w, roi) -> void:
	var canvas = _canvas_ref
	if canvas == null:
		return
	hit_areas.clear()

	var bw := board_w()
	var bg := ProgUseTex.retrieve(board_tex_id())
	if bg != null:
		canvas.tex_draw(bg, roi.x, roi.y)
	else:
		canvas.draw_rect_c(Color(0.1, 0.09, 0.07, 1.0), roi.x, roi.y, bw, BOARD_H, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), roi.x, roi.y, bw, BOARD_H, 2)

	# 主区格子列表
	var pi := page_items()
	for i in range(PAGE_SIZE):
		var col: int = i % COLS
		var row: int = i / COLS
		var cx: int = roi.x + START_X + col * LINE_W / COLS
		var cy: int = roi.y + START_Y + row * LINE_H
		if i == selected:
			canvas.draw_rect_c(Color(0.9, 0.75, 0.2, 0.32), cx, cy, BOX_W, BOX_H, true)
		if i < pi.size():
			var it: Dictionary = pi[i]
			var iid := int(it.get("item_id", 0))
			var cnt := int(it.get("count", 0))
			var tex := ItemGfx.icon_of_item(iid)
			if tex != null:
				var off := ItemGfx.center_offset(tex, BOX_W, BOX_H)
				canvas.tex_draw(tex, cx + off.x, cy + off.y)
			else:
				var nm := ItemRecord.name(iid)
				if not nm.is_empty():
					canvas.draw_string_c(nm.substr(0, 1), cx + 12, cy + 10,
						Color(0.9, 0.85, 0.7), null, 13)
			if cnt > 1:
				canvas.draw_string_shadow(str(cnt), cx + BOX_W - 14, cy + BOX_H - 14,
					Color(1, 1, 0.6), Color(0, 0, 0, 0.9), 1, null, 10)
		hit_areas["cell_%d" % i] = Rect2(cx, cy, BOX_W, BOX_H)

	# 分页信息
	canvas.draw_string_c("%d/%d" % [page + 1, page_count()],
		roi.x + 19, roi.y + BOARD_H - 18, Color(0.85, 0.82, 0.7), null, 12)

	# 选定按钮（105,185）58x22
	var stex := ProgUseTex.retrieve(BTN_SELECT)
	if stex != null:
		canvas.tex_draw(stex, roi.x + 105, roi.y + 185)
		hit_areas["select"] = Rect2(roi.x + 105, roi.y + 185, stex.get_width(), stex.get_height())
	else:
		canvas.draw_rect_c(Color(0.3, 0.26, 0.14, 0.95), roi.x + 105, roi.y + 185, 58, 22, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), roi.x + 105, roi.y + 185, 58, 22, 1)
		canvas.draw_string_c("选定", roi.x + 113, roi.y + 189, Color(1, 0.95, 0.6), null, 13)
		hit_areas["select"] = Rect2(roi.x + 105, roi.y + 185, 58, 22)

	# 关闭（257,183）
	var ctex := ProgUseTex.retrieve(BTN_CLOSE)
	if ctex != null:
		canvas.tex_draw(ctex, roi.x + 257, roi.y + 183)
		hit_areas["close"] = Rect2(roi.x + 257, roi.y + 183, 32, 30)
	else:
		canvas.draw_string_c("×", roi.x + 257, roi.y + 183, Color(0.9, 0.75, 0.5), null, 18)
		hit_areas["close"] = Rect2(roi.x + 257, roi.y + 183, 20, 20)

	# 扩展面板
	if ext_mode == 1:
		_draw_ext1(canvas, roi)
	elif ext_mode == 2:
		_draw_ext2(canvas, roi)

	# 选中物品的说明
	if selected_item_id() > 0:
		var nm2 := ItemRecord.name(selected_item_id())
		var pr := selected_price()
		canvas.draw_string_c(nm2, roi.x + 19, roi.y + BOARD_H - 38, Color(1, 1, 0.85), null, 13)
		canvas.draw_string_c("价格 %s" % UiDirectTradeBoard.ksep(pr),
			roi.x + 19, roi.y + BOARD_H - 24, Color(1, 1, 0), null, 13)


## Ext1：不可打包物品（如装备）—— 格子 4x3 区
func _draw_ext1(canvas, roi) -> void:
	canvas.draw_rect_c(Color(0.06, 0.06, 0.08, 0.9), roi.x + EXT1_GRID_X, roi.y + EXT1_GRID_Y,
		EXT1_GRID_W, EXT1_GRID_H, true)
	canvas.draw_border(Color(0.4, 0.34, 0.22, 1.0), roi.x + EXT1_GRID_X, roi.y + EXT1_GRID_Y,
		EXT1_GRID_W, EXT1_GRID_H, 1)
	for i in range(12):
		var r := ext1_grid_loc(i)
		var gx: int = roi.x + int(r.position.x)
		var gy: int = roi.y + int(r.position.y)
		if i == ext1_selected:
			canvas.draw_rect_c(Color(0.9, 0.75, 0.2, 0.3), gx, gy, int(r.size.x), int(r.size.y), true)
		hit_areas["ext1_%d" % i] = Rect2(gx, gy, int(r.size.x), int(r.size.y))
	# 关闭
	_draw_small_close(canvas, roi.x + 448, roi.y + 159, "ext1_close")


## Ext2：可打包物品（药水等）—— 单格 + 价格区
func _draw_ext2(canvas, roi) -> void:
	var gx: int = roi.x + EXT2_GRID_X
	var gy: int = roi.y + EXT2_GRID_Y
	canvas.draw_rect_c(Color(0.06, 0.06, 0.08, 0.9), gx, gy, EXT2_GRID_W, EXT2_GRID_H, true)
	canvas.draw_border(Color(0.4, 0.34, 0.22, 1.0), gx, gy, EXT2_GRID_W, EXT2_GRID_H, 1)
	hit_areas["ext2_grid"] = Rect2(gx, gy, EXT2_GRID_W, EXT2_GRID_H)

	var px: int = roi.x + EXT2_PRICE_X
	var py: int = roi.y + EXT2_PRICE_Y
	canvas.draw_rect_c(Color(0.06, 0.06, 0.08, 0.9), px, py, EXT2_PRICE_W, EXT2_PRICE_H, true)
	canvas.draw_border(Color(0.4, 0.34, 0.22, 1.0), px, py, EXT2_PRICE_W, EXT2_PRICE_H, 1)
	canvas.draw_string_c("价格 %s" % UiDirectTradeBoard.ksep(selected_price()),
		px + 4, py + 10, Color(1, 1, 0), null, 13)

	_draw_small_close(canvas, roi.x + 448, roi.y + 159, "ext2_close")


func _draw_small_close(canvas, x: int, y: int, act: String) -> void:
	var t := ProgUseTex.retrieve(BTN_CLOSE)
	if t != null:
		canvas.tex_draw(t, x, y)
		hit_areas[act] = Rect2(x, y, t.get_width(), t.get_height())
	else:
		canvas.draw_string_c("×", x, y, Color(0.9, 0.75, 0.5), null, 16)
		hit_areas[act] = Rect2(x, y, 18, 18)


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
			if k == "select":
				_on_select()
				return true
			if k == "ext1_close" or k == "ext2_close":
				ext_mode = 0
				ext1_selected = -1
				return true
			if k.begins_with("cell_"):
				selected = int(k.substr(5))
				return true
			if k.begins_with("ext1_"):
				ext1_selected = int(k.substr(5))
				return true
	return false


func _on_select() -> void:
	var iid := selected_item_id()
	if iid <= 0:
		return
	if on_select.is_valid():
		on_select.call(iid)
	# 买的物品可打包走 Ext2，否则 Ext1（原版 extendedBoardGfxID 的判定）
	var packable := false
	packable = ItemRecordRef.packable(iid)
	ext_mode = 2 if packable else 1
	if packable:
		ext1_selected = -1


## 确认购买（扩展面板里的「购买」按钮）
func confirm_buy() -> void:
	var iid := selected_item_id()
	if iid <= 0:
		return
	if on_buy.is_valid():
		on_buy.call(iid, 0, selected_price())
	ext_mode = 0


## 翻页
func next_page() -> void:
	page = clampi(page + 1, 0, page_count() - 1)
	selected = 0


func prev_page() -> void:
	page = clampi(page - 1, 0, page_count() - 1)
	selected = 0
