extends UiWidget
class_name UiItemListBoard
## 通用物品列表面板
##
## 【逐行翻译】client/src/itemlistboard.hpp + itemlistboard.cpp
## 对照（itemlistboard.hpp:8-19）：
##   m_boxW = 38, m_boxH = 38        格子尺寸
##   m_startX = 23, m_startY = 41    起始坐标
##   m_gfxSrcX = 290, m_gfxSrcY = 0, m_gfxSrcW = 198, m_gfxSrcH = 204
##                                   右侧物品详情图源区
##
## ── 复用范围 ──
## 原版这些面板是同一套「格子列表 + 翻页 + 选择」的实现：
##   itemlistboard      (物品列表)
##   purchaseboard      (NPC 商店，格 38x38 起点 19,15)
##   auctionboard       (拍卖行)
##   stallboard         (摆摊)
##   secureditemlistboard (寄存物品)
## 差异只在格子尺寸、起始坐标、按钮文案。
## 所以做一个参数化的通用面板，用配置区分，避免写 5 份几乎相同的代码。

const RoiRef := preload("res://scripts/ui/framework/ui_roi.gd")
const ItemGfxRef := preload("res://scripts/item/item_gfx.gd")
const ItemRecordRef := preload("res://scripts/item/item_record.gd")

## 布局配置（默认按 itemlistboard 的数值）
var box_w := 38
var box_h := 38
var start_x := 23
var start_y := 41
## 每行/每列格数
var cols := 6
var rows := 5
## 面板尺寸
var board_w := 360
var board_h := 320

## 面板标题
var title: String = "物品"

## 物品列表：[{ item_id, count, price, desc }]
var items: Array = []
## 当前页
var page := 0
## 选中格
var selected: int = -1

## 按钮文案
var btn_left := "上一页"
var btn_right := "下一页"
var btn_action := "选定"

## 回调
## Callable(widget, item_index, item_dict)
var on_select: Callable = Callable()
## Callable(widget, item_index)  点"选定/购买"
var on_action: Callable = Callable()


func _init(p_name: String = "ItemListBoard") -> void:
	widget_name = p_name
	w = board_w
	h = board_h
	show = false
	on_event = _handle_event


## 配置成某个面板的样式
## kind: "list" / "purchase" / "auction" / "stall" / "secured"
func configure(kind: String) -> void:
	match kind:
		"purchase":
			# purchaseboard 的数值（hpp:28-32）
			start_x = 19
			start_y = 15
			box_w = 38
			box_h = 38
			cols = 6
			rows = 4
			board_w = 300
			board_h = 260
			title = "购买"
			btn_action = "购买"
		"auction":
			start_x = 23
			start_y = 41
			box_w = 38
			box_h = 38
			cols = 6
			rows = 5
			board_w = 520
			board_h = 420
			title = "拍卖行"
			btn_action = "竞拍"
		"stall":
			start_x = 23
			start_y = 41
			box_w = 38
			box_h = 38
			cols = 6
			rows = 5
			board_w = 400
			board_h = 380
			title = "摆摊"
			btn_action = "购买"
		"secured":
			start_x = 23
			start_y = 41
			box_w = 38
			box_h = 38
			cols = 6
			rows = 5
			board_w = 360
			board_h = 340
			title = "寄存物品"
			btn_action = "取出"
		_:
			title = "物品列表"
			btn_action = "选定"
	w = board_w
	h = board_h


func set_items(list: Array) -> void:
	items = list
	page = 0
	selected = -1


func page_count() -> int:
	var per := cols * rows
	if per <= 0:
		return 0
	return int(ceil(float(items.size()) / float(per)))


func page_items() -> Array:
	var per := cols * rows
	var s := page * per
	return items.slice(s, mini(s + per, items.size()))


func draw_self(canvas, my_roi) -> void:
	var t = canvas
	t.draw_rect_c(Color(0.07, 0.07, 0.1, 0.96), my_roi.x, my_roi.y, my_roi.w, my_roi.h, true)
	t.draw_border(Color(0.5, 0.45, 0.3, 1.0), my_roi.x, my_roi.y, my_roi.w, my_roi.h, 2)
	t.draw_string_c(title, my_roi.x + 10, my_roi.y + 6, Color(0.95, 0.85, 0.5), null, 16)
	t.draw_string_c("×", my_roi.x + my_roi.w - 20, my_roi.y + 6, Color(0.9, 0.75, 0.5), null, 16)

	# 页码
	var pc := page_count()
	t.draw_string_c("%d / %d" % [page + 1, maxi(pc, 1)],
		my_roi.x + my_roi.w - 70, my_roi.y + 8, Color(0.7, 0.68, 0.6), null, 12)

	# 格子
	var gx0: int = my_roi.x + start_x
	var gy0: int = my_roi.y + start_y
	var pitems := page_items()
	for i in range(cols * rows):
		var cx: int = gx0 + (i % cols) * box_w
		var cy: int = gy0 + (i / cols) * box_h
		var is_sel := i == selected
		t.draw_rect_c(Color(0.12, 0.12, 0.16, 0.85), cx, cy, box_w - 1, box_h - 1, true)
		t.draw_border(Color(0.55, 0.5, 0.35, 1.0) if is_sel else Color(0.25, 0.25, 0.3, 0.8),
			cx, cy, box_w - 1, box_h - 1, 2 if is_sel else 1)

		if i < pitems.size():
			var it = pitems[i]
			var iid := int(it.get("item_id", 0))
			var cnt := int(it.get("count", 1))
			var tex := ItemGfxRef.icon_of_item(iid)
			if tex != null:
				var off := ItemGfxRef.center_offset(tex, box_w - 1, box_h - 1)
				t.tex_draw(tex, cx + off.x, cy + off.y)
			else:
				var nm := ItemRecordRef.name(iid)
				if not nm.is_empty():
					t.draw_string_c(nm.substr(0, 1), cx + 12, cy + 12, Color(0.85, 0.8, 0.7), null, 14)
			if cnt > 1:
				t.draw_string_shadow(str(cnt), cx + box_w - 16, cy + box_h - 16,
					Color(1, 1, 0.7), Color(0, 0, 0, 0.9), 1, null, 11)

	# 选中物品详情
	var detail_y: int = gy0 + rows * box_h + 6
	if selected >= 0 and selected < pitems.size():
		var sel = pitems[selected]
		var nm2 := ItemRecordRef.name(int(sel.get("item_id", 0)))
		t.draw_string_c(nm2, my_roi.x + start_x, detail_y, Color(0.95, 0.9, 0.75), null, 13)
		var price := int(sel.get("price", 0))
		if price > 0:
			t.draw_string_c("价格: %d" % price, my_roi.x + start_x + 160, detail_y,
				Color(0.9, 0.85, 0.6), null, 13)

	# 底部按钮
	var btn_y: int = my_roi.y + my_roi.h - 34
	var bx: int = my_roi.x + 12
	for txt in [btn_left, btn_action, btn_right]:
		var bw := 70
		t.draw_rect_c(Color(0.2, 0.2, 0.26, 0.95), bx, btn_y, bw, 24, true)
		t.draw_border(Color(0.4, 0.4, 0.46, 1.0), bx, btn_y, bw, 24, 1)
		t.draw_string_c(txt, bx + 10, btn_y + 5, Color(0.9, 0.88, 0.82), null, 13)
		bx += bw + 8


func _handle_event(self_widget, event: InputEvent, valid: bool, my_roi) -> bool:
	if not show:
		return false
	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_LEFT and event.pressed:
		var mpos: Vector2 = event.position
		if not my_roi.contains(int(mpos.x), int(mpos.y)):
			return false
		if mpos.x > my_roi.x + my_roi.w - 28 and mpos.y < my_roi.y + 28:
			show = false
			return true

		# 底部按钮
		var btn_y: int = my_roi.y + my_roi.h - 34
		if mpos.y >= btn_y and mpos.y < btn_y + 24:
			var bx: int = my_roi.x + 12
			for k in range(3):
				if mpos.x >= bx and mpos.x < bx + 70:
					match k:
						0:
							page = maxi(0, page - 1)
						1:
							if selected >= 0:
								var pitems := page_items()
								if selected < pitems.size() and on_action.is_valid():
									on_action.call(self, page * cols * rows + selected)
						2:
							page = mini(maxi(page_count() - 1, 0), page + 1)
					return true
				bx += 78

		# 格子
		var gx0: int = my_roi.x + start_x
		var gy0: int = my_roi.y + start_y
		if mpos.x >= gx0 and mpos.y >= gy0:
			var lx: int = int((mpos.x - gx0) / box_w)
			var ly: int = int((mpos.y - gy0) / box_h)
			if lx >= 0 and lx < cols and ly >= 0 and ly < rows:
				var idx: int = ly * cols + lx
				var pitems2 := page_items()
				if idx < pitems2.size():
					selected = idx
					if on_select.is_valid():
						on_select.call(self, page * cols * rows + idx, pitems2[idx])
				return true
		return true
	return false
