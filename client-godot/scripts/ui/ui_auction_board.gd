extends UiWidget
class_name UiAuctionBoard
## 拍卖行主面板（浏览 / 登记 / 我的拍卖 / 查询）
##
## 【逐行翻译】client/src/gui/auctionboard/auctionboard.cpp（2609 行，含子件）
##
## ── 布局（背景 0x00001400 = 720x440，已实测）──
##   列表区    ( 10,  82)  13 行 x 4 列（auctionitemlist.hpp:m_rowCount=13）
##   分类文字  ( 61,  24)  所有物品/衣服/武器/项链/头盔/戒指/手镯/…
##   其他文字  (240,  24) (419, 24)
##   列头      ( 64,  62) (188, 62) (293, 62) (399, 62)
##   关闭      (486,  23)
##   重新读    (486, 168)
##   翻页按钮  ( 21, 360) ( 72, 360) (122, 361)
##   日期区间  (412, 360) (412, 390)
##   搜索      (513, 370) (563, 370)
##   撤拍      (643, 370)  0x00000850
##
## ── 行尺寸（auctionitemrow.hpp，逐值）──
##   列宽 item=108 seller=140 time=70 price=142  -> 行宽 = 460
##   行高 19
##   列中心 = 108/2, 108+70, 108+140+35, 108+140+70+71
##
## ── 拍卖分类（AUCTIONCAT_*）──
##   0 所有 1 衣服 2 武器 3 项链 4 头盔 5 戒指 6 手镯 7 药水 8 其他

const ItemGfx := preload("res://scripts/item/item_gfx.gd")
const ItemRecord := preload("res://scripts/item/item_record.gd")
const ProtocolDefRef := preload("res://scripts/core/protocol_def.gd")

const BG_TEX := 0x00001400
const BTN_CANCEL := 0x00000850
const BTN_CLOSE := 0x0000001C

## 列表几何
const LIST_X := 10
const LIST_Y := 82
const ROW_COUNT := 13
const COL_ITEM_W := 108
const COL_SELLER_W := 140
const COL_TIME_W := 70
const COL_PRICE_W := 142
const ROW_W := 460
const ROW_H := 19

## 列中心（auctionitemrow.hpp:25-28）
const COL_C_ITEM := 54        ## 108 / 2
const COL_C_SELLER := 178     ## 108 + 140/2
const COL_C_TIME := 283       ## 108 + 140 + 70/2
const COL_C_PRICE := 399      ## 108 + 140 + 70 + 142/2

## 分类（协议枚举）
const CAT_ALL := 0
const CAT_DRESS := 1
const CAT_WEAPON := 2
const CAT_NECKLACE := 3
const CAT_HELMET := 4
const CAT_RING := 5
const CAT_ARMRING := 6
const CAT_POTION := 7
const CAT_OTHER := 8

const CAT_NAMES := ["所有物品", "衣服", "武器", "项链", "头盔", "戒指", "手镯", "药水", "其他"]

## 每页行数 = 13
const PAGE_SIZE := 13

# ==================== 状态 ====================

## 拍卖条目：[{ item_id, count, price, seller_name, seller_id, expire_ts }]
var entries: Array = []
var category := CAT_ALL
var page := 0
var selected := -1
var my_db_id := 0

## 搜索关键字
var search_text: String = ""
## 日期过滤（原版两个输入框，这里简化成 ts 区间）
var date_from := 0
var date_to := 0

var on_close: Callable = Callable()
var on_register: Callable = Callable()
var on_cancel: Callable = Callable()
var on_buy: Callable = Callable()      ## Callable(idx)
var on_search: Callable = Callable()   ## Callable(keyword, cat, page)

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init(p_name: String = "AuctionBoard") -> void:
	widget_name = p_name
	w = 720
	h = 440
	show = false
	on_draw = _draw_board


func set_canvas(c) -> void:
	_canvas_ref = c


func set_entries(list: Array, p_my_db_id: int) -> void:
	entries = list.duplicate(true)
	my_db_id = p_my_db_id
	selected = -1


func page_count() -> int:
	return maxi(1, (entries.size() + PAGE_SIZE - 1) / PAGE_SIZE)


func page_entries() -> Array:
	var s := page * PAGE_SIZE
	return entries.slice(s, mini(s + PAGE_SIZE, entries.size()))


func selected_entry():
	var pe := page_entries()
	if selected < 0 or selected >= pe.size():
		return null
	return pe[selected]


## 当前选中的是否可以撤拍（自己的且非待处理）
func can_cancel() -> bool:
	var e = selected_entry()
	if e == null:
		return false
	return int(e.get("seller_id", 0)) == my_db_id and my_db_id != 0


func category_name() -> String:
	return CAT_NAMES[clampi(category, 0, CAT_NAMES.size() - 1)]


## 时间格式：剩余秒数 -> "Xd Yh"
static func fmt_remain(sec: int) -> String:
	if sec <= 0:
		return "已过期"
	var d := sec / 86400
	var h := (sec % 86400) / 3600
	if d > 0:
		return "%dd %dh" % [d, h]
	var m := (sec % 3600) / 60
	return "%dh %dm" % [h, m]


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
		canvas.draw_rect_c(Color(0.09, 0.08, 0.07, 1.0), roi.x, roi.y, 720, 440, true)
		canvas.draw_border(Color(0.52, 0.44, 0.28, 1.0), roi.x, roi.y, 720, 440, 2)

	# 分类
	canvas.draw_string_c(category_name(), roi.x + 61, roi.y + 24, Color(1, 1, 0), null, 14)
	# 页脚：第几页 / 共几页
	canvas.draw_string_c("第 %d / %d 页" % [page + 1, page_count()],
		roi.x + 240, roi.y + 24, Color(0.9, 0.88, 0.8), null, 13)
	canvas.draw_string_c("共 %d 条" % entries.size(),
		roi.x + 419, roi.y + 24, Color(0.9, 0.88, 0.8), null, 13)

	# 列头
	var heads := [
		[64, "物品"], [188, "卖家"], [293, "剩余时间"], [399, "价格"],
	]
	for hd in heads:
		canvas.draw_string_c(str(hd[1]), roi.x + int(hd[0]), roi.y + 62,
			Color(0.95, 0.9, 0.65), null, 13)

	# 13 行
	var pe := page_entries()
	for i in range(ROW_COUNT):
		var ry: int = roi.y + LIST_Y + i * ROW_H
		var rx: int = roi.x + LIST_X
		# 斑马纹 + 选中高亮
		if i == selected:
			canvas.draw_rect_c(Color(0.9, 0.75, 0.2, 0.3), rx, ry, ROW_W, ROW_H, true)
		elif i % 2 == 1:
			canvas.draw_rect_c(Color(1, 1, 1, 0.04), rx, ry, ROW_W, ROW_H, true)

		if i < pe.size():
			var e: Dictionary = pe[i]
			var iid := int(e.get("item_id", 0))
			var cnt := int(e.get("count", 0))
			# 物品列：图标 + 名字
			var tex := ItemGfx.icon_of_item(iid)
			var nm := ItemRecord.name(iid)
			var tx: int = rx + 4
			if tex != null:
				# 图标限制在行高内
				var iw: int = tex.get_width()
				var ih: int = tex.get_height()
				var sc: int = 1
				if ih > ROW_H:
					sc = 1  # 不做缩放，直接按原尺寸（可能略溢出）
				canvas.tex_draw(tex, tx, ry + (ROW_H - ih) / 2)
				tx += iw + 4
			var label := nm if cnt <= 1 else "%s x%d" % [nm, cnt]
			canvas.draw_string_c(label.substr(0, 11), tx, ry + 2,
				Color(0.93, 0.92, 0.85), null, 12)
			# 卖家
			canvas.draw_string_c(str(e.get("seller_name", "")).substr(0, 12),
				rx + COL_C_SELLER - COL_SELLER_W / 2 + 4, ry + 2,
				Color(0.8, 0.85, 0.95), null, 12)
			# 剩余时间
			canvas.draw_string_c(fmt_remain(int(e.get("remain_sec", 0))),
				rx + COL_C_TIME - 30, ry + 2, Color(0.85, 0.82, 0.7), null, 12)
			# 价格（黄色，右对齐到列右端）
			var ps := UiDirectTradeBoard.ksep(int(e.get("price", 0)))
			var pw: int = int(canvas.measure_text(ps, null, 12).x)
			canvas.draw_string_c(ps, rx + COL_C_PRICE + COL_PRICE_W / 2 - pw - 4, ry + 2,
				Color(1, 1, 0), null, 12)

		hit_areas["row_%d" % i] = Rect2(rx, ry, ROW_W, ROW_H)

	# 翻页按钮（21/72/122, 360）
	_draw_text_btn(canvas, roi.x + 21, roi.y + 360, "首页", "first")
	_draw_text_btn(canvas, roi.x + 72, roi.y + 360, "上一页", "prev")
	_draw_text_btn(canvas, roi.x + 122, roi.y + 361, "下一页", "next")

	# 日期区间（412,360 / 412,390）
	_draw_input_box(canvas, roi.x + 412, roi.y + 360, "起始日期", str(date_from))
	_draw_input_box(canvas, roi.x + 412, roi.y + 390, "结束日期", str(date_to))

	# 搜索框 + 按钮（513/563, 370）
	_draw_input_box(canvas, roi.x + 513, roi.y + 370, "关键字", search_text)
	_draw_text_btn(canvas, roi.x + 563, roi.y + 370, "查询", "search")

	# 去掉重复覆盖：搜索按钮重定位（原版 513 是输入框，563 是按钮）
	# —— 已在上面分别登记

	# 撤拍按钮（643,370）只有自己的东西才能点
	var ctex := ProgUseTex.retrieve(BTN_CANCEL)
	var c_enabled := can_cancel()
	if ctex != null:
		canvas.tex_draw(ctex, roi.x + 643, roi.y + 370,
			Color(1, 1, 1, 1.0) if c_enabled else Color(0.5, 0.5, 0.5, 0.6))
	else:
		canvas.draw_rect_c(Color(0.3, 0.26, 0.14, 0.95), roi.x + 643, roi.y + 370, 46, 24, true)
		canvas.draw_string_c("撤拍", roi.x + 652, roi.y + 375,
			Color(1, 0.95, 0.6) if c_enabled else Color(0.5, 0.5, 0.5), null, 12)
	if c_enabled:
		hit_areas["cancel"] = Rect2(roi.x + 643, roi.y + 370, 46, 24)

	# 登记按钮（486,168 区域，原版是 m_buttonRegister 一类）
	_draw_text_btn(canvas, roi.x + 486, roi.y + 168, "登记拍卖", "register")

	# 关闭（486,23）
	var cl := ProgUseTex.retrieve(BTN_CLOSE)
	if cl != null:
		canvas.tex_draw(cl, roi.x + 486, roi.y + 23)
		hit_areas["close"] = Rect2(roi.x + 486, roi.y + 23, 32, 30)
	else:
		canvas.draw_string_c("×", roi.x + 486, roi.y + 23, Color(0.9, 0.75, 0.5), null, 18)
		hit_areas["close"] = Rect2(roi.x + 486, roi.y + 23, 20, 20)


func _draw_text_btn(canvas, x: int, y: int, text: String, act: String) -> void:
	var tw: int = int(canvas.measure_text(text, null, 12).x)
	canvas.draw_rect_c(Color(0.22, 0.19, 0.12, 0.95), x, y, tw + 8, 18, true)
	canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), x, y, tw + 8, 18, 1)
	canvas.draw_string_c(text, x + 4, y + 2, Color(1, 0.95, 0.72), null, 12)
	hit_areas[act] = Rect2(x, y, tw + 8, 18)


func _draw_input_box(canvas, x: int, y: int, ph: String, val: String) -> void:
	var bw := 0
	if ph == "关键字":
		bw = 0  # 搜索框宽度由按钮位置推算
	var box_w: int = 96 if ph != "关键字" else 0
	if box_w == 0:
		return
	canvas.draw_rect_c(Color(0, 0, 0, 0.6), x, y, box_w, 18, true)
	canvas.draw_border(Color(0.42, 0.36, 0.22, 1.0), x, y, box_w, 18, 1)
	var show := val if not val.is_empty() else ph
	canvas.draw_string_c(show, x + 3, y + 2,
		Color(0.95, 0.92, 0.8) if not val.is_empty() else Color(0.5, 0.5, 0.5), null, 12)


# ==================== 事件 ====================

func handle_click(px: int, py: int) -> bool:
	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			var k := str(key)
			match k:
				"close":
					show = false
					if on_close.is_valid():
						on_close.call(self)
					return true
				"first":
					_set_page(0)
					return true
				"prev":
					_set_page(page - 1)
					return true
				"next":
					_set_page(page + 1)
					return true
				"search":
					if on_search.is_valid():
						on_search.call(search_text, category, 0)
					return true
				"register":
					if on_register.is_valid():
						on_register.call(self)
					return true
				"cancel":
					if can_cancel() and on_cancel.is_valid():
						on_cancel.call(self)
					return true
				_:
					if k.begins_with("row_"):
						var idx: int = int(k.substr(4))
						if idx < page_entries().size():
							if selected == idx:
								# 双击/再点 = 购买
								if on_buy.is_valid():
									on_buy.call(page * PAGE_SIZE + idx)
							else:
								selected = idx
						return true
	return false


func _set_page(p: int) -> void:
	page = clampi(p, 0, page_count() - 1)
	selected = -1


func set_category(c: int) -> void:
	category = clampi(c, 0, CAT_NAMES.size() - 1)
	page = 0
	selected = -1
