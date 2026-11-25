extends UiWidget
class_name UiDirectTradeBoard
## 直接交易面板（面对面交易）
##
## 【逐行翻译】client/src/gui/directtradeboard/directtradeboard.cpp（1174 行）
##            client/src/gui/directtradeboard/directtradeitemlist.cpp
##
## ── 布局（背景 0x00002010 = 494x330，已实测）──
##   对方名字    (178,  20)  黄色 dir=NONE
##   对方状态    ( 80,  20)  已确认=红 / 已锁定=绿 / 编辑中=灰
##   对方金钱    (124, 300)  千分位分隔
##   对方物品格  ( 13,  47)  5x5
##   自己名字    (320,  20)  黄色
##   自己状态    (415,  20)  已确认=红 / 已锁定=绿 / 锁定中=黄 / 编辑中=灰
##   自己金钱    (375, 300)  96x20 输入框
##   自己物品格  (264,  47)  5x5
##   交易按钮    (227, 278) 未锁 0x000000B5 / 已锁 0x00000190（都是 34x34，4 帧）
##   关闭按钮    (454, 282)
##
## ── 状态机（directtradeboard.cpp 的 5 个 bool）──
##   m_lockPending  自己点了锁定、等服务器回执
##   m_locked       自己已锁定
##   m_confirmed    自己已确认（双方都锁后进入确认）
##   m_peerLocked   对方已锁定
##   m_peerConfirmed 对方已确认
##
## ── 金钱解析（parseGold）──
##   只接受纯数字，输入过程中 validate 校验「当前+新输入」仍是合法数字。
##   显示时用千分位（str_ksep）。

const ItemGfx := preload("res://scripts/item/item_gfx.gd")
const BG_TEX := 0x00002010
const GRID_COLS := 5
const GRID_ROWS := 5

## 物品格尺寸（DirectTradeItemList：m_gridW=5, m_gridH=5，格子由模板排布决定）
## 每个格子按 44x44 走，5 列 = 220 宽，正好覆盖 (13,47)-(233,267) 与 (264,47)-(484,267)
const CELL_W := 44
const CELL_H := 44

## 按钮
const BTN_TRADE_OFF := 0x000000B5
const BTN_TRADE_ON := 0x00000190
const BTN_CLOSE := 0x0000001C

## 状态文字与颜色（两个状态板的差异：自己多一个「锁定中」）
const STATE_EDIT := "编辑中"
const STATE_LOCKING := "锁定中"
const STATE_LOCKED := "已锁定"
const STATE_CONFIRM := "已确认"


# ==================== 状态 ====================

var peer_uid: int = 0
var peer_name: String = ""
var peer_locked := false
var peer_confirmed := false
var peer_gold := 0
var peer_items: Array = []      ## [{ item_id, count }]

var my_name: String = ""
var locked := false
var lock_pending := false
var confirmed := false
var my_gold_text: String = ""
var my_items: Array = []        ## [{ item_id, count }]

var on_trade: Callable = Callable()
var on_close: Callable = Callable()
var on_gold_change: Callable = Callable()

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init(p_name: String = "DirectTradeBoard") -> void:
	widget_name = p_name
	w = 494
	h = 330
	show = false
	on_draw = _draw_board


func set_canvas(c) -> void:
	_canvas_ref = c


func begin_trade(p_peer_uid: int, p_peer_name: String) -> void:
	peer_name = p_peer_name
	peer_gold = 0
	peer_items = []
	peer_locked = false
	peer_confirmed = false
	locked = false
	lock_pending = false
	confirmed = false
	my_gold_text = ""
	my_items = []
	show = true


func close_trade() -> void:
	show = false
	peer_uid = 0
	if on_close.is_valid():
		on_close.call(self)


func complete_trade() -> void:
	peer_uid = 0
	my_items = []
	peer_items = []
	my_gold_text = ""
	peer_gold = 0
	locked = false
	confirmed = false
	peer_locked = false
	peer_confirmed = false


# ==================== 金钱解析 ====================

## directtradeboard.cpp:parseGold —— 只接受纯数字
static func parse_gold(s: String) -> Variant:
	var t := s.strip_edges()
	if t.is_empty():
		return null
	for i in range(t.length()):
		var ch := t[i]
		if ch < "0" or ch > "9":
			return null
	return int(t)


## 千分位（原版 str_ksep(n, ',')）
static func ksep(n: int) -> String:
	var s := str(absi(n))
	var out := ""
	var cnt := 0
	for i in range(s.length() - 1, -1, -1):
		out = s[i] + out
		cnt += 1
		if cnt % 3 == 0 and i > 0:
			out = "," + out
	return ("-" if n < 0 else "") + out


func my_gold() -> int:
	var v = parse_gold(my_gold_text)
	return 0 if v == null else int(v)


## 输入校验：当前 + 新输入仍是合法数字（directtradeboard.cpp 的 validate）
func validate_gold_input(new_input: String) -> bool:
	return parse_gold(my_gold_text + new_input) != null


# ==================== 状态文字 ====================

## 对方状态
func peer_state_text() -> String:
	if peer_confirmed:
		return STATE_CONFIRM
	if peer_locked:
		return STATE_LOCKED
	return STATE_EDIT


func peer_state_color() -> Color:
	if peer_confirmed:
		return Color(1, 0, 0)
	if peer_locked:
		return Color(0, 1, 0)
	return Color(0.5, 0.5, 0.5)


## 自己状态
func my_state_text() -> String:
	if confirmed:
		return STATE_CONFIRM
	if locked:
		return STATE_LOCKED
	if lock_pending:
		return STATE_LOCKING
	return STATE_EDIT


func my_state_color() -> Color:
	if confirmed:
		return Color(1, 0, 0)
	if locked:
		return Color(0, 1, 0)
	if lock_pending:
		return Color(1, 1, 0)
	return Color(0.5, 0.5, 0.5)


# ==================== 绘制 ====================

func _draw_board(_w, roi) -> void:
	var canvas = _canvas_ref
	if canvas == null:
		return
	hit_areas.clear()

	# 背景（blendMode = NONE，不透明）
	var bg := ProgUseTex.retrieve(BG_TEX)
	if bg != null:
		canvas.tex_draw(bg, roi.x, roi.y)
	else:
		canvas.draw_rect_c(Color(0.1, 0.09, 0.07, 1.0), roi.x, roi.y, 494, 330, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), roi.x, roi.y, 494, 330, 2)
		canvas.draw_string_c("直接交易", roi.x + 8, roi.y + 6, Color(0.95, 0.85, 0.5), null, 15)

	# 左半：对方
	canvas.draw_string_c(peer_name, roi.x + 178, roi.y + 20, Color(1, 1, 0), null, 14)
	canvas.draw_string_c(peer_state_text(), roi.x + 80, roi.y + 20, peer_state_color(), null, 14)
	canvas.draw_string_c(ksep(peer_gold), roi.x + 124, roi.y + 300, Color(1, 1, 0), null, 14)
	_draw_items(canvas, roi, 13, 47, peer_items, "peer")

	# 右半：自己
	canvas.draw_string_c(my_name, roi.x + 320, roi.y + 20, Color(1, 1, 0), null, 14)
	canvas.draw_string_c(my_state_text(), roi.x + 415, roi.y + 20, my_state_color(), null, 14)

	# 自己金钱输入框 96x20 在 (375,300)
	var gx: int = roi.x + 375
	var gy: int = roi.y + 300
	canvas.draw_rect_c(Color(0, 0, 0, 0.6), gx, gy, 96, 20, true)
	canvas.draw_border(Color(0.45, 0.38, 0.22, 1.0), gx, gy, 96, 20, 1)
	canvas.draw_string_c(my_gold_text, gx + 3, gy + 2, Color(1, 1, 0), null, 14)
	hit_areas["gold_input"] = Rect2(gx, gy, 96, 20)
	_draw_items(canvas, roi, 264, 47, my_items, "mine")

	# 交易按钮（227,278）34x34，锁定后换图
	var tex_id := BTN_TRADE_ON if locked else BTN_TRADE_OFF
	var btex := ProgUseTex.retrieve(tex_id)
	if btex != null:
		canvas.tex_draw(btex, roi.x + 227, roi.y + 278)
		hit_areas["trade"] = Rect2(roi.x + 227, roi.y + 278, 34, 34)
	else:
		canvas.draw_rect_c(Color(0.3, 0.26, 0.14, 0.95), roi.x + 227, roi.y + 278, 34, 34, true)
		canvas.draw_string_c("锁定" if not locked else "确认", roi.x + 229, roi.y + 283,
			Color(1, 0.95, 0.6), null, 12)
		hit_areas["trade"] = Rect2(roi.x + 227, roi.y + 278, 34, 34)

	# 关闭按钮（454,282）
	var ctex := ProgUseTex.retrieve(BTN_CLOSE)
	if ctex != null:
		canvas.tex_draw(ctex, roi.x + 454, roi.y + 282)
		hit_areas["close"] = Rect2(roi.x + 454, roi.y + 282, 32, 30)
	else:
		canvas.draw_string_c("×", roi.x + 454, roi.y + 282, Color(0.9, 0.75, 0.5), null, 18)
		hit_areas["close"] = Rect2(roi.x + 454, roi.y + 282, 20, 20)


func _draw_items(canvas, roi, bx: int, by: int, items: Array, side: String) -> void:
	for i in range(mini(items.size(), GRID_COLS * GRID_ROWS)):
		var it: Dictionary = items[i]
		var col: int = i % GRID_COLS
		var row: int = i / GRID_COLS
		var cx: int = roi.x + bx + col * CELL_W
		var cy: int = roi.y + by + row * CELL_H
		# 格子底
		canvas.draw_rect_c(Color(0.16, 0.14, 0.1, 0.55), cx, cy, CELL_W - 2, CELL_H - 2, true)
		canvas.draw_border(Color(0.32, 0.28, 0.16, 0.9), cx, cy, CELL_W - 2, CELL_H - 2, 1)
		var iid := int(it.get("item_id", 0))
		var cnt := int(it.get("count", 0))
		var tex := ItemGfx.icon_of_item(iid)
		if tex != null:
			var off := ItemGfx.center_offset(tex, CELL_W - 2, CELL_H - 2)
			canvas.tex_draw(tex, cx + off.x, cy + off.y)
		if cnt > 1:
			canvas.draw_string_shadow(str(cnt), cx + CELL_W - 16, cy + CELL_H - 16,
				Color(1, 1, 0.6), Color(0, 0, 0, 0.9), 1, null, 10)
		hit_areas["%s_%d" % [side, i]] = Rect2(cx, cy, CELL_W - 2, CELL_H - 2)



# ==================== 事件 ====================

func handle_click(px: int, py: int) -> bool:
	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			var k := str(key)
			if k == "close":
				close_trade()
				return true
			if k == "trade":
				_on_trade_button()
				return true
			if k == "gold_input":
				return true
			# 点自己格子 -> 移回背包（restoreLocalItems 的语义）
			if k.begins_with("mine_"):
				var idx: int = int(k.substr(5))
				if idx >= 0 and idx < my_items.size():
					my_items.remove_at(idx)
					confirmed = false
					return true
	return false


func _on_trade_button() -> void:
	if not locked:
		# 第一次点：锁定（syncLocalOffer(true)）
		if parse_gold(my_gold_text) == null and not my_gold_text.is_empty():
			return
		lock_pending = true
	else:
		# 已锁定：确认
		confirmed = true
	if on_trade.is_valid():
		on_trade.call(self)


## 服务器回执锁定成功
func apply_lock_ack() -> void:
	lock_pending = false
	locked = true


## 服务器拒绝
func reject_offer() -> void:
	lock_pending = false
	locked = false
	confirmed = false


## 对方出价同步
func set_peer_offer(p_gold: int, p_items: Array) -> void:
	peer_gold = p_gold
	peer_items = p_items.duplicate()
