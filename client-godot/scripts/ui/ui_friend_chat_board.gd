extends UiWidget
class_name UiFriendChatBoard
## 好友聊天板（可拖拽缩放的多页面板）
##
## 【逐行翻译】client/src/gui/friendchatboard/（4584 行，17 个文件）
##
## ── 常量（friendchatboardconst.hpp，逐值）──
##   UIPage_DRAGBORDER = {10, 10, 12, 10}   拖拽边距（上左下右）
##   UIPage_BORDER     = {54, 10, 13, 38}   内容边框（上左下右）
##   UIPage_MARGIN     = 4                  绘制区边距
##   UIPage_MIN_WIDTH  = 400
##   UIPage_MIN_HEIGHT = 400
##
## ── 五个页面（UIPageType）──
##   UIPage_CHAT          0  聊天（气泡消息流）
##   UIPage_CHATPREVIEW   1  会话预览（默认页）
##   UIPage_FRIENDLIST    2  好友列表
##   UIPage_FRIENDSEARCH  3  搜索添加
##   UIPage_CREATEGROUP   4  建群
##
## ── 背景缩放（friendchatboard.cpp 的 cropDup 技巧）──
##   m_frame      0x00000800 = 491x437
##     cropDup 源区 (55,95,230,250)，目标 (w-261, h-187)
##     —— 即把中间那块拉伸，边框保持原宽
##   m_background 0x00000810 = 510x187，modColor = (160,160,160,255)
##     cropDup 源区 (0,0,510,187)，目标 (w, h)
##
## ── 头像素材 ──
##   0x010007CF 兜底头像（CBFace 也用它）
##   0x00001100 系统助手图标（200x200）
##   0x00001300 群助手图标（200x200）
##
## ── 消息项尺寸（chatitem.hpp，逐值）──
##   AVATAR_WIDTH  = 35
##   AVATAR_HEIGHT = 35 * 94 / 84 = 39
##   GAP = 5, NAME_HEIGHT = 20
##   TRIANGLE_WIDTH = 4, TRIANGLE_HEIGHT = 6
##   MESSAGE_MARGIN = 5, MESSAGE_CORNER = 3
##   MESSAGE_MIN_WIDTH = 10, MESSAGE_MIN_HEIGHT = 10
##   REF_GAP = 10

const ProtocolDefRef := preload("res://scripts/core/protocol_def.gd")

## 拖拽边距
const DRAG_TOP := 10
const DRAG_LEFT := 10
const DRAG_BOTTOM := 12
const DRAG_RIGHT := 10
## 内容边框
const BORDER_TOP := 54
const BORDER_LEFT := 10
const BORDER_BOTTOM := 13
const BORDER_RIGHT := 38
const PAGE_MARGIN := 4
const MIN_W := 400
const MIN_H := 400

## 页类型
const PAGE_CHAT := 0
const PAGE_CHATPREVIEW := 1
const PAGE_FRIENDLIST := 2
const PAGE_FRIENDSEARCH := 3
const PAGE_CREATEGROUP := 4
const PAGE_NAMES := ["聊天", "会话", "好友", "搜索", "建群"]

## 消息项尺寸
const AVATAR_W := 35
const AVATAR_H := 39          ## 35 * 94 / 84
const ITEM_GAP := 5
const NAME_HEIGHT := 20
const TRIANGLE_W := 4
const TRIANGLE_H := 6
const MSG_MARGIN := 5
const MSG_CORNER := 3
const MSG_MIN_W := 10
const MSG_MIN_H := 10
const REF_GAP := 10

## 贴图
const FRAME_TEX := 0x00000800
const BG_TEX := 0x00000810
const AVATAR_FALLBACK := 0x010007CF
const ICON_SYSTEM := 0x00001100
const ICON_GROUP := 0x00001300
const BTN_CLOSE := 0x0000001C

# ==================== 状态 ====================

var cur_page := PAGE_CHATPREVIEW

## 会话列表：[{ id, name, icon_tex, unread, last_text }]
var previews: Array = []
## 好友列表：[{ id, name, online }]
var friends: Array = []
## 当前会话 id
var cur_peer: String = ""
var cur_peer_name: String = ""
## 当前会话消息：[{ from_me, name, text, ts }]
var messages: Array = []
## 输入框内容
var input_text: String = ""
## 搜索关键字
var search_text: String = ""

var on_close: Callable = Callable()
var on_send: Callable = Callable()       ## Callable(text)
var on_open_chat: Callable = Callable()  ## Callable(peer_id)
var on_search: Callable = Callable()     ## Callable(keyword)
var on_add_friend: Callable = Callable() ## Callable(peer)

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init(p_name: String = "FriendChatBoard") -> void:
	widget_name = p_name
	w = MIN_W + DRAG_LEFT + DRAG_RIGHT
	h = MIN_H + DRAG_TOP + DRAG_BOTTOM
	show = false
	on_draw = _draw_board


func set_canvas(c) -> void:
	_canvas_ref = c


func set_ui_page(p: int) -> void:
	cur_page = clampi(p, 0, PAGE_NAMES.size() - 1)


func open_chat(peer_id: String, peer_name: String) -> void:
	cur_peer = peer_id
	cur_peer_name = peer_name
	cur_page = PAGE_CHAT
	messages = []


func add_message(from_me: bool, name: String, text: String) -> void:
	messages.append({"from_me": from_me, "name": name, "text": text})
	# 保留最近 200 条
	if messages.size() > 200:
		messages = messages.slice(messages.size() - 200)


## 内容区（去掉边框后的可用矩形，相对面板左上角）
func content_rect() -> Rect2:
	return Rect2(BORDER_LEFT + PAGE_MARGIN, BORDER_TOP + PAGE_MARGIN,
		size_w() - BORDER_LEFT - BORDER_RIGHT - PAGE_MARGIN * 2,
		size_h() - BORDER_TOP - BORDER_BOTTOM - PAGE_MARGIN * 2)


## 标题栏中心 x（原版 45 + (w - 45 - 190) / 2）
func title_cx() -> int:
	return 45 + (size_w() - 45 - 190) / 2


# ==================== 绘制 ====================

func _draw_board(_w, roi) -> void:
	var canvas = _canvas_ref
	if canvas == null:
		return
	hit_areas.clear()

	var bw := size_w()
	var bh := size_h()

	# 背景（0x00000810，灰度调制，拉伸铺满）
	var bg := ProgUseTex.retrieve(BG_TEX)
	if bg != null:
		canvas.tex_draw_region(bg, Rect2(0, 0, bg.get_width(), bg.get_height()),
			roi.x, roi.y, bw, bh, Color(160.0 / 255.0, 160.0 / 255.0, 160.0 / 255.0, 1.0))
	else:
		canvas.draw_rect_c(Color(0.1, 0.1, 0.12, 1.0), roi.x, roi.y, bw, bh, true)

	# 边框（0x00000800，九宫格：中间 (55,95,230,250) 拉伸）
	var frame := ProgUseTex.retrieve(FRAME_TEX)
	if frame != null:
		_draw_9slice(canvas, frame, roi.x, roi.y, bw, bh,
			55, 95, 230, 250)
	else:
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), roi.x, roi.y, bw, bh, 2)

	# 标题（当前页名 or 会话名）
	var title: String = PAGE_NAMES[cur_page]
	if cur_page == PAGE_CHAT and not cur_peer_name.is_empty():
		title = cur_peer_name
	var tx: int = roi.x + title_cx()
	var tw: int = int(canvas.measure_text(title, null, 14).x)
	canvas.draw_string_c(title, tx - tw / 2, roi.y + 29, Color(0.95, 0.9, 0.7), null, 14)

	# 右上角页切换按钮（4 个，原版 PageControl 在 w-42, 29）
	var pcb_x: int = roi.x + bw - 42
	_draw_page_btns(canvas, pcb_x, roi.y + 29)

	# 内容区
	var cr := content_rect()
	var cx: int = roi.x + int(cr.position.x)
	var cy: int = roi.y + int(cr.position.y)
	var cw: int = int(cr.size.x)
	var ch: int = int(cr.size.y)

	match cur_page:
		PAGE_CHAT:
			_draw_chat(canvas, cx, cy, cw, ch)
		PAGE_CHATPREVIEW:
			_draw_previews(canvas, cx, cy, cw, ch)
		PAGE_FRIENDLIST:
			_draw_friend_list(canvas, cx, cy, cw, ch)
		PAGE_FRIENDSEARCH:
			_draw_search(canvas, cx, cy, cw, ch)
		PAGE_CREATEGROUP:
			_draw_create_group(canvas, cx, cy, cw, ch)

	# 关闭（w-38, h-40）
	var clx: int = roi.x + bw - 38
	var cly: int = roi.y + bh - 40
	var ctex := ProgUseTex.retrieve(BTN_CLOSE)
	if ctex != null:
		canvas.tex_draw(ctex, clx, cly)
		hit_areas["close"] = Rect2(clx, cly, ctex.get_width(), ctex.get_height())
	else:
		canvas.draw_string_c("×", clx, cly, Color(0.9, 0.75, 0.5), null, 18)
		hit_areas["close"] = Rect2(clx, cly, 20, 20)


## 九宫格绘制：四边保持原尺寸，中间拉伸
func _draw_9slice(canvas, tex: Texture2D, dx: int, dy: int, dw: int, dh: int,
		sx: int, sy: int, sw: int, sh: int) -> void:
	var tw := tex.get_width()
	var th := tex.get_height()
	# 九宫格切分：上 sy / 中 sh / 下 th-sy-sh
	var top_h := sy
	var bot_h := th - sy - sh
	var left_w := sx
	var right_w := tw - sx - sw

	# 四角（原尺寸）
	canvas.tex_draw_region(tex, Rect2(0, 0, left_w, top_h), dx, dy, left_w, top_h)
	canvas.tex_draw_region(tex, Rect2(tw - right_w, 0, right_w, top_h),
		dx + dw - right_w, dy, right_w, top_h)
	canvas.tex_draw_region(tex, Rect2(0, th - bot_h, left_w, bot_h),
		dx, dy + dh - bot_h, left_w, bot_h)
	canvas.tex_draw_region(tex, Rect2(tw - right_w, th - bot_h, right_w, bot_h),
		dx + dw - right_w, dy + dh - bot_h, right_w, bot_h)

	# 四边拉伸
	var mid_w: int = maxi(0, dw - left_w - right_w)
	var mid_h: int = maxi(0, dh - top_h - bot_h)
	if mid_w > 0 and top_h > 0:
		canvas.tex_draw_region(tex, Rect2(left_w, 0, sw, top_h),
			dx + left_w, dy, mid_w, top_h)
		canvas.tex_draw_region(tex, Rect2(left_w, th - bot_h, sw, bot_h),
			dx + left_w, dy + dh - bot_h, mid_w, bot_h)
	if mid_h > 0 and left_w > 0:
		canvas.tex_draw_region(tex, Rect2(0, top_h, left_w, sh),
			dx, dy + top_h, left_w, mid_h)
		canvas.tex_draw_region(tex, Rect2(tw - right_w, top_h, right_w, sh),
			dx + dw - right_w, dy + top_h, right_w, mid_h)

	# 中心拉伸
	if mid_w > 0 and mid_h > 0:
		canvas.tex_draw_region(tex, Rect2(left_w, top_h, sw, sh),
			dx + left_w, dy + top_h, mid_w, mid_h)


## 4 个页切换按钮（原版 texAniOpt 0x8F0 / 0x23 / 0x8B0 / 0x590）
const PAGE_BTNS := [
	{"tex": 0x000008F0, "page": PAGE_CHATPREVIEW},
	{"tex": 0x00000023, "page": PAGE_FRIENDLIST},
	{"tex": 0x000008B0, "page": PAGE_FRIENDSEARCH},
	{"tex": 0x00000590, "page": PAGE_CREATEGROUP},
]


func _draw_page_btns(canvas, x: int, y: int) -> void:
	var bx := x
	for i in range(PAGE_BTNS.size()):
		var b: Dictionary = PAGE_BTNS[i]
		var tex := ProgUseTex.retrieve(int(b["tex"]))
		var bw := 16
		if tex != null:
			bw = tex.get_width()
			canvas.tex_draw(tex, bx, y)
		else:
			canvas.draw_rect_c(Color(0.22, 0.2, 0.14, 0.95), bx, y, 16, 16, true)
			canvas.draw_string_c(str(i + 1), bx + 4, y + 2, Color(1, 0.95, 0.7), null, 11)
		# 当前页高亮
		if int(b["page"]) == cur_page:
			canvas.draw_border(Color(1, 0.85, 0.3, 1.0), bx, y, bw, 16, 1)
		hit_areas["page_%d" % int(b["page"])] = Rect2(bx, y, bw, 16)
		bx += bw + 2


## 聊天页：气泡列表 + 底部输入
func _draw_chat(canvas, x: int, y: int, w: int, h: int) -> void:
	var input_h := 24
	var list_h: int = h - input_h - ITEM_GAP

	# 从最新往回排，累积高度不超过 list_h
	var by := y + list_h
	var shown := 0
	for i in range(messages.size() - 1, -1, -1):
		var m: Dictionary = messages[i]
		var text := str(m.get("text", ""))
		var nm := str(m.get("name", ""))
		var from_me := bool(m.get("from_me", false))
		var tw: int = int(canvas.measure_text(text, null, 13).x)
		var bubble_w: int = mini(tw + MSG_MARGIN * 2 + TRIANGLE_W, w - AVATAR_W - ITEM_GAP * 2)
		var bubble_h: int = NAME_HEIGHT + MSG_MARGIN * 2 + 16

		var item_h: int = bubble_h + ITEM_GAP
		if by - item_h < y:
			break
		by -= item_h

		# 头像
		var ax: int = x + (w - AVATAR_W if from_me else 0)
		var atex := ProgUseTex.retrieve(AVATAR_FALLBACK)
		if atex != null:
			canvas.tex_draw_region(atex, Rect2(0, 0, atex.get_width(), atex.get_height()),
				ax, by, AVATAR_W, AVATAR_H)
		else:
			canvas.draw_rect_c(Color(0.2, 0.18, 0.16, 0.9), ax, by, AVATAR_W, AVATAR_H, true)
			canvas.draw_string_c("头", ax + 10, by + 12, Color(0.7, 0.65, 0.55), null, 12)

		# 气泡（自己靠右，别人靠左）
		var bx: int = x + AVATAR_W + ITEM_GAP if not from_me else x + w - AVATAR_W - ITEM_GAP - bubble_w
		canvas.draw_rect_c(Color(0.18, 0.22, 0.3, 0.95) if from_me else Color(0.15, 0.15, 0.17, 0.95),
			bx, by, bubble_w, bubble_h, true)
		canvas.draw_border(Color(0.4, 0.45, 0.55, 1.0) if from_me else Color(0.35, 0.35, 0.38, 1.0),
			bx, by, bubble_w, bubble_h, 1)

		# 名字（小字）+ 正文
		canvas.draw_string_c(nm.substr(0, 10), bx + MSG_MARGIN, by + MSG_MARGIN,
			Color(0.75, 0.72, 0.62), null, 11)
		canvas.draw_string_c(text.substr(0, 22), bx + MSG_MARGIN, by + MSG_MARGIN + NAME_HEIGHT,
			Color(0.95, 0.93, 0.86), null, 13)
		shown += 1

	# 输入框（底部）
	var iy: int = y + list_h + ITEM_GAP
	canvas.draw_rect_c(Color(0, 0, 0, 0.6), x, iy, w, input_h, true)
	canvas.draw_border(Color(0.42, 0.36, 0.22, 1.0), x, iy, w, input_h, 1)
	var shown_text := input_text if not input_text.is_empty() else "输入消息，回车发送"
	canvas.draw_string_c(shown_text, x + 4, iy + 4,
		Color(0.95, 0.93, 0.86) if not input_text.is_empty() else Color(0.5, 0.5, 0.5), null, 13)
	hit_areas["chat_input"] = Rect2(x, iy, w, input_h)


## 会话预览页
func _draw_previews(canvas, x: int, y: int, w: int, h: int) -> void:
	if previews.is_empty():
		canvas.draw_string_c("暂无会话", x + 10, y + 10, Color(0.55, 0.55, 0.55), null, 13)
		return
	var ry := y
	for i in range(previews.size()):
		var p: Dictionary = previews[i]
		var row_h := 46
		if ry + row_h > y + h:
			break
		# 悬停/选中底色
		if str(p.get("id", "")) == cur_peer:
			canvas.draw_rect_c(Color(0.9, 0.75, 0.2, 0.2), x, ry, w, row_h, true)
		# 图标（系统/群/玩家）
		var icon_id := int(p.get("icon_tex", AVATAR_FALLBACK))
		var itex := ProgUseTex.retrieve(icon_id)
		if itex != null:
			var sz := 36
			canvas.tex_draw_region(itex, Rect2(0, 0, itex.get_width(), itex.get_height()),
				x + 4, ry + 4, sz, sz)
		# 名字 + 最后一条消息
		canvas.draw_string_c(str(p.get("name", "")).substr(0, 12),
			x + 46, ry + 5, Color(0.95, 0.9, 0.72), null, 13)
		canvas.draw_string_c(str(p.get("last_text", "")).substr(0, 20),
			x + 46, ry + 24, Color(0.7, 0.68, 0.6), null, 12)
		# 未读数
		var unread := int(p.get("unread", 0))
		if unread > 0:
			canvas.draw_rect_c(Color(0.85, 0.15, 0.15, 1.0), x + w - 26, ry + 6, 20, 16, true)
			canvas.draw_string_c(str(mini(unread, 99)), x + w - 22, ry + 7,
				Color(1, 1, 1), null, 11)
		hit_areas["peer_%d" % i] = Rect2(x, ry, w, row_h)
		ry += row_h + 2


## 好友列表页
func _draw_friend_list(canvas, x: int, y: int, w: int, h: int) -> void:
	if friends.is_empty():
		canvas.draw_string_c("暂无好友", x + 10, y + 10, Color(0.55, 0.55, 0.55), null, 13)
		return
	var ry := y
	for i in range(friends.size()):
		var f: Dictionary = friends[i]
		var row_h := 24
		if ry + row_h > y + h:
			break
		# 在线绿点
		var online := bool(f.get("online", false))
		canvas.draw_rect_c(Color(0.2, 0.85, 0.3, 1.0) if online else Color(0.4, 0.4, 0.4, 1.0),
			x + 4, ry + 8, 6, 6, true)
		canvas.draw_string_c(str(f.get("name", "")).substr(0, 14),
			x + 16, ry + 4,
			Color(0.92, 0.9, 0.82) if online else Color(0.6, 0.6, 0.6), null, 13)
		hit_areas["friend_%d" % i] = Rect2(x, ry, w, row_h)
		ry += row_h


## 搜索页
func _draw_search(canvas, x: int, y: int, w: int, h: int) -> void:
	var iy := y
	canvas.draw_rect_c(Color(0, 0, 0, 0.6), x, iy, w - 60, 22, true)
	canvas.draw_border(Color(0.42, 0.36, 0.22, 1.0), x, iy, w - 60, 22, 1)
	canvas.draw_string_c(search_text if not search_text.is_empty() else "输入名字搜索",
		x + 4, iy + 3, Color(0.95, 0.93, 0.86) if not search_text.is_empty() else Color(0.5, 0.5, 0.5),
		null, 13)
	hit_areas["search_input"] = Rect2(x, iy, w - 60, 22)
	_draw_btn(canvas, x + w - 56, iy, "搜索", "search_go")

	# 搜索结果（复用 friends 数组）
	var ry := iy + 28
	for i in range(mini(friends.size(), 12)):
		var f: Dictionary = friends[i]
		canvas.draw_string_c(str(f.get("name", "")).substr(0, 14), x + 4, ry,
			Color(0.9, 0.88, 0.8), null, 13)
		_draw_btn(canvas, x + w - 56, ry - 2, "加好友", "add_%d" % i)
		ry += 24


## 建群页
func _draw_create_group(canvas, x: int, y: int, w: int, h: int) -> void:
	canvas.draw_string_c("群名称：", x + 4, y + 4, Color(0.9, 0.88, 0.8), null, 13)
	canvas.draw_rect_c(Color(0, 0, 0, 0.6), x + 56, y, w - 60, 22, true)
	canvas.draw_border(Color(0.42, 0.36, 0.22, 1.0), x + 56, y, w - 60, 22, 1)
	canvas.draw_string_c(input_text, x + 60, y + 3, Color(0.95, 0.93, 0.86), null, 13)
	hit_areas["group_input"] = Rect2(x + 56, y, w - 60, 22)

	canvas.draw_string_c("选择成员（从好友列表）：", x + 4, y + 32, Color(0.8, 0.78, 0.7), null, 12)
	var ry := y + 52
	for i in range(mini(friends.size(), 12)):
		var f: Dictionary = friends[i]
		canvas.draw_rect_c(Color(0.15, 0.15, 0.17, 0.9), x + 4, ry - 2, 12, 12, true)
		canvas.draw_border(Color(0.4, 0.36, 0.24, 1.0), x + 4, ry - 2, 12, 12, 1)
		canvas.draw_string_c(str(f.get("name", "")).substr(0, 14), x + 22, ry,
			Color(0.9, 0.88, 0.8), null, 12)
		hit_areas["pick_%d" % i] = Rect2(x + 4, ry - 2, 12, 12)
		ry += 22

	_draw_btn(canvas, x, y + h - 26, "创建群", "create_group")


func _draw_btn(canvas, x: int, y: int, text: String, act: String) -> void:
	var tw: int = int(canvas.measure_text(text, null, 12).x)
	canvas.draw_rect_c(Color(0.22, 0.19, 0.12, 0.95), x, y, tw + 8, 20, true)
	canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), x, y, tw + 8, 20, 1)
	canvas.draw_string_c(text, x + 4, y + 3, Color(1, 0.95, 0.72), null, 12)
	hit_areas[act] = Rect2(x, y, tw + 8, 20)


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
			if k.begins_with("page_"):
				set_ui_page(int(k.substr(5)))
				return true
			if k.begins_with("peer_"):
				var idx: int = int(k.substr(5))
				if idx < previews.size():
					var p: Dictionary = previews[idx]
					open_chat(str(p.get("id", "")), str(p.get("name", "")))
					if on_open_chat.is_valid():
						on_open_chat.call(str(p.get("id", "")))
				return true
			if k.begins_with("friend_"):
				var fi: int = int(k.substr(7))
				if fi < friends.size():
					var f: Dictionary = friends[fi]
					open_chat(str(f.get("name", "")), str(f.get("name", "")))
				return true
			if k == "search_go":
				if on_search.is_valid():
					on_search.call(search_text)
				return true
			if k.begins_with("add_"):
				var ai: int = int(k.substr(4))
				if ai < friends.size() and on_add_friend.is_valid():
					on_add_friend.call(friends[ai])
				return true
			if k == "create_group":
				return true
	return false


## 回车发送
func submit_input() -> String:
	var t := input_text.strip_edges()
	input_text = ""
	if t.is_empty():
		return ""
	if on_send.is_valid():
		on_send.call(t)
	return t


## 未读总数（供控制栏提示）
func unread_total() -> int:
	var n := 0
	for p in previews:
		n += int(p.get("unread", 0))
	return n
