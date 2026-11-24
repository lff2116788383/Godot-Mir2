extends UiWidget
class_name UiTeamBoard
## 队伍面板
##
## 【逐行翻译】client/src/gui/teamstateboard/teamstateboard.hpp
## 对照：
##   - teamstateboard.hpp:35-40    m_boardW=600, m_boardH=440
##   - teamstateboard.hpp:42-46    m_menuX=30, m_menuY=30, m_menuW=90, m_tabH=26
##   - teamstateboard.hpp:27-35    PageType 五个页：附近/创建/成员/设置/申请
##
## 五个页签（原版 PAGE_NEARBY / PAGE_CREATE / PAGE_MEMBER / PAGE_SETTING / PAGE_APPLICANT）

const BOARD_W := 600
const BOARD_H := 440
const MENU_X := 30
const MENU_Y := 30
const MENU_W := 90
const TAB_H := 26

const PAGE_NAMES := ["附近队伍", "创建队伍", "队伍成员", "队伍设置", "入队申请"]

## 当前页
var cur_page := 0
## 队伍成员：{ name, level, hp_ratio, map }
var members: Array = []
## 附近队伍列表
var nearby: Array = []
## 入队申请
var applicants: Array = []

var on_page_change: Callable = Callable()
var on_member_click: Callable = Callable()


func _init(p_name: String = "TeamStateBoard") -> void:
	widget_name = p_name
	w = BOARD_W
	h = BOARD_H
	show = false
	on_event = _handle_event


func draw_self(canvas, my_roi) -> void:
	var t = canvas
	t.draw_rect_c(Color(0.07, 0.07, 0.1, 0.96), my_roi.x, my_roi.y, my_roi.w, my_roi.h, true)
	t.draw_border(Color(0.5, 0.45, 0.3, 1.0), my_roi.x, my_roi.y, my_roi.w, my_roi.h, 2)
	t.draw_string_c("队伍", my_roi.x + 10, my_roi.y + 6, Color(0.95, 0.85, 0.5), null, 16)
	t.draw_string_c("×", my_roi.x + my_roi.w - 20, my_roi.y + 6, Color(0.9, 0.75, 0.5), null, 16)

	# 左侧页签列
	for i in range(PAGE_NAMES.size()):
		var tx: int = my_roi.x + MENU_X
		var ty: int = my_roi.y + MENU_Y + i * (TAB_H + 4)
		var bg := Color(0.22, 0.22, 0.28, 0.95) if i == cur_page else Color(0.13, 0.13, 0.17, 0.9)
		t.draw_rect_c(bg, tx, ty, MENU_W, TAB_H, true)
		t.draw_border(Color(0.38, 0.38, 0.45, 1.0), tx, ty, MENU_W, TAB_H, 1)
		t.draw_string_c(PAGE_NAMES[i], tx + 6, ty + 5,
			Color(0.92, 0.9, 0.84) if i == cur_page else Color(0.65, 0.65, 0.7), null, 12)

	# 右侧内容区
	var cx: int = my_roi.x + MENU_X + MENU_W + 10
	var cy: int = my_roi.y + MENU_Y
	var cw: int = my_roi.w - (cx - my_roi.x) - 20
	var ch: int = my_roi.h - MENU_Y - 20
	t.draw_rect_c(Color(0.1, 0.1, 0.13, 0.9), cx, cy, cw, ch, true)
	t.draw_border(Color(0.3, 0.3, 0.36, 1.0), cx, cy, cw, ch, 1)

	match cur_page:
		0:
			_draw_nearby(t, cx, cy, cw, ch)
		1:
			_draw_create(t, cx, cy, cw, ch)
		2:
			_draw_members(t, cx, cy, cw, ch)
		3:
			_draw_settings(t, cx, cy, cw, ch)
		4:
			_draw_applicants(t, cx, cy, cw, ch)


func _draw_nearby(t, cx: int, cy: int, cw: int, ch: int) -> void:
	if nearby.is_empty():
		t.draw_string_c("附近没有队伍", cx + 12, cy + 12, Color(0.55, 0.55, 0.6), null, 13)
		return
	var y := cy + 8
	for i in range(mini(nearby.size(), 12)):
		var it = nearby[i]
		var line := "%s  队长:%s  %d/%d 人" % [
			str(it.get("name", "?")), str(it.get("leader", "?")),
			int(it.get("count", 0)), int(it.get("max", 5))]
		t.draw_string_c(line, cx + 10, y, Color(0.88, 0.86, 0.8), null, 13)
		y += 22


func _draw_create(t, cx: int, cy: int, cw: int, ch: int) -> void:
	t.draw_string_c("队伍名称", cx + 12, cy + 12, Color(0.8, 0.78, 0.72), null, 13)
	t.draw_rect_c(Color(0.06, 0.06, 0.08, 0.9), cx + 12, cy + 32, cw - 24, 26, true)
	t.draw_border(Color(0.35, 0.35, 0.42, 1.0), cx + 12, cy + 32, cw - 24, 26, 1)
	t.draw_string_c("（点击输入）", cx + 18, cy + 38, Color(0.45, 0.45, 0.5), null, 12)
	t.draw_string_c("最多人数: 5", cx + 12, cy + 70, Color(0.75, 0.73, 0.68), null, 13)
	t.draw_rect_c(Color(0.2, 0.3, 0.45, 0.95), cx + 12, cy + 96, 100, 30, true)
	t.draw_string_c("创建队伍", cx + 26, cy + 104, Color(0.95, 0.93, 0.88), null, 13)


func _draw_members(t, cx: int, cy: int, cw: int, ch: int) -> void:
	if members.is_empty():
		t.draw_string_c("暂无队伍成员", cx + 12, cy + 12, Color(0.55, 0.55, 0.6), null, 13)
		return
	var y := cy + 8
	for m in members:
		var nm := str(m.get("name", "?"))
		var lv := int(m.get("level", 0))
		var hp := float(m.get("hp_ratio", 1.0))
		t.draw_string_c("%s  Lv.%d" % [nm, lv], cx + 10, y, Color(0.9, 0.87, 0.8), null, 13)
		# 血条
		var bar_x: int = cx + 200
		var bar_w := 180
		t.draw_rect_c(Color(0.15, 0.05, 0.05, 0.9), bar_x, y + 3, bar_w, 10, true)
		t.draw_rect_c(Color(0.75, 0.2, 0.18, 0.95), bar_x, y + 3, int(bar_w * clampf(hp, 0, 1)), 10, true)
		y += 26


func _draw_settings(t, cx: int, cy: int, cw: int, ch: int) -> void:
	var opts := ["允许队友拾取", "自动分配经验", "队友可查看装备", "允许自由加入"]
	var y := cy + 12
	for i in range(opts.size()):
		t.draw_rect_c(Color(0.16, 0.16, 0.2, 0.9), cx + 12, y, 14, 14, true)
		t.draw_border(Color(0.4, 0.4, 0.46, 1.0), cx + 12, y, 14, 14, 1)
		t.draw_string_c(str(opts[i]), cx + 34, y, Color(0.85, 0.83, 0.78), null, 13)
		y += 28


func _draw_applicants(t, cx: int, cy: int, cw: int, ch: int) -> void:
	if applicants.is_empty():
		t.draw_string_c("没有入队申请", cx + 12, cy + 12, Color(0.55, 0.55, 0.6), null, 13)
		return
	var y := cy + 8
	for a in applicants:
		t.draw_string_c("%s  Lv.%d" % [str(a.get("name", "?")), int(a.get("level", 0))],
			cx + 10, y, Color(0.88, 0.86, 0.8), null, 13)
		t.draw_rect_c(Color(0.2, 0.35, 0.25, 0.95), cx + 260, y, 46, 20, true)
		t.draw_string_c("同意", cx + 268, y + 3, Color(0.92, 0.95, 0.9), null, 12)
		t.draw_rect_c(Color(0.4, 0.2, 0.2, 0.95), cx + 314, y, 46, 20, true)
		t.draw_string_c("拒绝", cx + 322, y + 3, Color(0.95, 0.9, 0.9), null, 12)
		y += 26


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
		# 页签
		var tx: int = my_roi.x + MENU_X
		var ty0: int = my_roi.y + MENU_Y
		if mpos.x >= tx and mpos.x < tx + MENU_W:
			var idx: int = int((mpos.y - ty0) / (TAB_H + 4))
			if idx >= 0 and idx < PAGE_NAMES.size():
				cur_page = idx
				if on_page_change.is_valid():
					on_page_change.call(self, idx)
				return true
		return true
	return false
