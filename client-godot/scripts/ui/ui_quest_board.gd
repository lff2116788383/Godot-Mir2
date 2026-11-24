extends UiWidget
class_name UiQuestBoard
## 任务状态板
##
## 【逐行翻译】client/src/queststateboard.cpp（247 行）+ queststateboard.hpp
##
## ── 布局 ──
##   背景 0x00000350（363x446，已实测）
##   描述区 m_despX=40, m_despY=100, m_despW=270, m_despH=300
##   字体 id=1 size=12，lineAlign = LALIGN_JUSTIFY，下边距 5
##   左/右翻页按钮 (315, 76) 与 (316, 108)
##   关闭 (326, 160)
##
## ── 折叠语义（queststateboard.cpp:70 的 onClickText）──
##   点描述区里的 <text id="xxx"> 会翻转对应任务的 folded 状态，
##   然后置 m_loadRequested = true 触发重新排版。
##   这里用「段落标题行点一下折叠/展开」等价实现。
##
## ── 任务数据结构 ──
##   m_questDesp: map<string, QuestDespState>
##   QuestDespState { xml: string, folded: bool }
##   排版用 XML 引擎（本工程已有 xml_layout/xml_typeset），
##   这里把每条任务的 xml 排版成文本行后逐行绘制。

const BG_TEX := 0x00000350
const DESP_X := 40
const DESP_Y := 100
const DESP_W := 270
const DESP_H := 300
const LINE_H := 16
const FONT_SIZE := 12
const BTN_CLOSE := 0x0000001C

# ==================== 状态 ====================

## 任务列表：[{ id, title, xml, folded, lines: Array }]
var quests: Array = []
var scroll := 0.0   ## 0..1
var left_side := true  ## 原版 m_left：显示左半还是右半（任务列表切换）

var on_close: Callable = Callable()
var on_toggle: Callable = Callable()   ## Callable(quest_id)
var on_page_left: Callable = Callable()
var on_page_right: Callable = Callable()

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init(p_name: String = "QuestStateBoard") -> void:
	widget_name = p_name
	w = 363
	h = 446
	show = false
	on_draw = _draw_board


func set_canvas(c) -> void:
	_canvas_ref = c


## 设置任务列表（xml 是原版的富文本描述）
func set_quests(list: Array) -> void:
	quests = list.duplicate(true)
	for q in quests:
		if not q.has("folded"):
			q["folded"] = false
		_rebuild_lines(q)


## 把一条任务的 xml 折行成文本行（原版走 xmltypeset，这里做等价简化：
## 去标签 + 按宽度折行，保留 <par> 产生的段落边界）
func _rebuild_lines(q: Dictionary) -> void:
	var xml := str(q.get("xml", ""))
	var text := _strip_tags(xml)
	var lines: Array = []
	if bool(q.get("folded", false)):
		# 折叠时只显示标题
		lines.append(str(q.get("title", "")))
	else:
		lines.append(str(q.get("title", "")))
		# 按 22 个字符折行（270 宽 / 12pt 字 ≈ 每行 22 汉字）
		var rest := text
		while not rest.is_empty():
			var take: int = mini(22, rest.length())
			lines.append(rest.substr(0, take))
			rest = rest.substr(take)
	q["lines"] = lines


## 去掉 xml 标签（对应 tinyxml 的取文本）
static func _strip_tags(xml: String) -> String:
	var out := ""
	var depth := 0
	for i in range(xml.length()):
		var ch := xml[i]
		if ch == "<":
			depth += 1
		elif ch == ">":
			if depth > 0:
				depth -= 1
		elif depth == 0:
			out += ch
	return out.strip_edges()


## 全部行（用于滚动）
func all_lines() -> Array:
	var out: Array = []
	for q in quests:
		for ln in q.get("lines", []):
			out.append({"text": ln, "qid": str(q.get("id", "")),
				"is_title": true})
	return out


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
		canvas.draw_rect_c(Color(0.09, 0.08, 0.07, 1.0), roi.x, roi.y, 363, 446, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), roi.x, roi.y, 363, 446, 2)

	# 描述区
	var dx: int = roi.x + DESP_X
	var dy: int = roi.y + DESP_Y
	var lines := all_lines()
	var max_lines: int = DESP_H / LINE_H
	var total := lines.size()
	var start := 0
	if total > max_lines:
		start = int(round((1.0 - clampf(scroll, 0.0, 1.0)) * (total - max_lines)))
		start = clampi(start, 0, total - max_lines)

	var ly := dy
	for i in range(start, mini(start + max_lines, total)):
		var ln: Dictionary = lines[i]
		var is_title: bool = bool(ln.get("is_title", false))
		# 标题行加粗感（用亮色 + 折叠标记）
		var txt := str(ln["text"])
		# 如果是某任务的标题行，前面加折叠符
		var qid := str(ln["qid"])
		var q = _find_quest(qid)
		var is_first := q != null and str(q.get("title", "")) == txt
		if is_first:
			var mark := "▼ " if not bool(q.get("folded", false)) else "▶ "
			canvas.draw_string_c(mark, dx, ly, Color(1, 0.85, 0.4), null, FONT_SIZE)
			canvas.draw_string_c(txt, dx + 14, ly, Color(1, 0.9, 0.6), null, FONT_SIZE)
			hit_areas["q_%s" % qid] = Rect2(dx, ly, DESP_W, LINE_H)
		else:
			canvas.draw_string_c(txt, dx, ly, Color(0.9, 0.88, 0.8), null, FONT_SIZE)
		ly += LINE_H

	if total == 0:
		canvas.draw_string_c("暂无任务", dx, dy, Color(0.55, 0.55, 0.55), null, FONT_SIZE)

	# 滚动条（右侧，DESP 区内）
	if total > max_lines:
		var sbx: int = dx + DESP_W + 4
		var sbh: int = DESP_H
		canvas.draw_rect_c(Color(0.08, 0.08, 0.1, 0.8), sbx, dy, 6, sbh, true)
		var th: int = maxi(20, sbh * max_lines / total)
		var ty: int = dy + int(round((1.0 - scroll) * (sbh - th)))
		canvas.draw_rect_c(Color(0.55, 0.5, 0.38, 0.95), sbx, ty, 6, th, true)

	# 翻页按钮 (315,76) / (316,108)
	_draw_btn(canvas, roi.x + 315, roi.y + 76, "◀", "page_left")
	_draw_btn(canvas, roi.x + 316, roi.y + 108, "▶", "page_right")

	# 关闭 (326,160)
	var ctex := ProgUseTex.retrieve(BTN_CLOSE)
	if ctex != null:
		canvas.tex_draw(ctex, roi.x + 326, roi.y + 160)
		hit_areas["close"] = Rect2(roi.x + 326, roi.y + 160, 32, 30)
	else:
		canvas.draw_string_c("×", roi.x + 326, roi.y + 160, Color(0.9, 0.75, 0.5), null, 18)
		hit_areas["close"] = Rect2(roi.x + 326, roi.y + 160, 20, 20)


func _find_quest(qid: String) -> Variant:
	for q in quests:
		if str(q.get("id", "")) == qid:
			return q
	return null


func _draw_btn(canvas, x: int, y: int, text: String, act: String) -> void:
	canvas.draw_rect_c(Color(0.22, 0.19, 0.12, 0.95), x, y, 22, 22, true)
	canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), x, y, 22, 22, 1)
	canvas.draw_string_c(text, x + 4, y + 3, Color(1, 0.95, 0.72), null, 13)
	hit_areas[act] = Rect2(x, y, 22, 22)


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
			if k == "page_left":
				if on_page_left.is_valid():
					on_page_left.call()
				return true
			if k == "page_right":
				if on_page_right.is_valid():
					on_page_right.call()
				return true
			if k.begins_with("q_"):
				var qid := k.substr(2)
				var q = _find_quest(qid)
				if q != null:
					q["folded"] = not bool(q.get("folded", false))
					_rebuild_lines(q)
					if on_toggle.is_valid():
						on_toggle.call(qid)
				return true
	return false


func scroll_by(d: float) -> void:
	scroll = clampf(scroll + d, 0.0, 1.0)
