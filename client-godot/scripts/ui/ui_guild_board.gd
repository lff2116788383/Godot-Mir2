extends UiWidget
class_name UiGuildBoard
## 行会面板
##
## 【逐行翻译】client/src/guildboard.hpp + guildboard.cpp（1268 行）
## 对照：
##   - guildboard.hpp:22-27   PageType 三页：NOTICE / MEMBER / CHAT
##   - guildboard.hpp:30-38   公告区 (20,44,526,296)、聊天输入 (x,y=336,h=18)
##   - guildboard.hpp:40-47   成员列表 (14,40,538)，表头 22px，每行 20px，13 行

const PAGE_NOTICE := 0
const PAGE_MEMBER := 1
const PAGE_CHAT := 2

## 布局（guildboard.hpp 原文数值）
const TEXT_X := 20
const TEXT_Y := 44
const TEXT_W := 526
const NOTICE_H := 296
const CHAT_INPUT_Y := 336
const CHAT_INPUT_H := 18
const MEMBER_X := 14
const MEMBER_Y := 40
const MEMBER_W := 538
const MEMBER_HEADER_H := 22
const MEMBER_ROW_H := 20
const MEMBER_ROW_COUNT := 13

const BOARD_W := 566
const BOARD_H := 380

var cur_page := PAGE_NOTICE

## 行会信息
var guild_name: String = ""
var guild_rank: int = 0
## 公告文本
var notice: String = ""
## 成员：[{ name, level, job, rank, online }]
var members: Array = []
## 聊天消息：[{ name, text }]
var chat_lines: Array = []

## 输入
var input_text: String = ""
var input_active := false

## 回调
var on_send_chat: Callable = Callable()
var on_set_notice: Callable = Callable()


func _init(p_name: String = "GuildBoard") -> void:
	widget_name = p_name
	w = BOARD_W
	h = BOARD_H
	show = false
	on_event = _handle_event


func draw_self(canvas, my_roi) -> void:
	var t = canvas
	t.draw_rect_c(Color(0.07, 0.07, 0.1, 0.96), my_roi.x, my_roi.y, my_roi.w, my_roi.h, true)
	t.draw_border(Color(0.5, 0.45, 0.3, 1.0), my_roi.x, my_roi.y, my_roi.w, my_roi.h, 2)
	t.draw_string_c("行会", my_roi.x + 10, my_roi.y + 6, Color(0.95, 0.85, 0.5), null, 16)
	t.draw_string_c(guild_name if not guild_name.is_empty() else "（未加入行会）",
		my_roi.x + 60, my_roi.y + 8, Color(0.85, 0.82, 0.75), null, 13)
	t.draw_string_c("×", my_roi.x + my_roi.w - 20, my_roi.y + 6, Color(0.9, 0.75, 0.5), null, 16)

	# 三个页签
	var tabs := ["公告", "成员", "聊天"]
	var tx: int = my_roi.x + 20
	for i in range(tabs.size()):
		var tw := 56
		var bg := Color(0.22, 0.22, 0.28, 0.95) if i == cur_page else Color(0.13, 0.13, 0.17, 0.9)
		t.draw_rect_c(bg, tx + i * (tw + 4), my_roi.y + 28, tw, 20, true)
		t.draw_border(Color(0.38, 0.38, 0.45, 1.0), tx + i * (tw + 4), my_roi.y + 28, tw, 20, 1)
		t.draw_string_c(tabs[i], tx + i * (tw + 4) + 12, my_roi.y + 31,
			Color(0.9, 0.88, 0.82) if i == cur_page else Color(0.62, 0.62, 0.68), null, 12)

	match cur_page:
		PAGE_NOTICE:
			_draw_notice(t, my_roi)
		PAGE_MEMBER:
			_draw_members(t, my_roi)
		PAGE_CHAT:
			_draw_chat(t, my_roi)


func _draw_notice(t, my_roi) -> void:
	var nx: int = my_roi.x + TEXT_X
	var ny: int = my_roi.y + TEXT_Y
	t.draw_rect_c(Color(0.1, 0.1, 0.13, 0.9), nx, ny, TEXT_W, NOTICE_H, true)
	t.draw_border(Color(0.3, 0.3, 0.36, 1.0), nx, ny, TEXT_W, NOTICE_H, 1)

	var txt := notice if not notice.is_empty() else "（暂无公告）"
	# 逐行简单折行（每行约 38 字）
	var col := Color(0.88, 0.86, 0.8) if not notice.is_empty() else Color(0.5, 0.5, 0.55)
	var y := ny + 8
	var line := ""
	for i in range(txt.length()):
		line += txt[i]
		if line.length() >= 38 or txt[i] == "\n":
			t.draw_string_c(line.strip_edges(), nx + 8, y, col, null, 13)
			y += 20
			line = ""
			if y > ny + NOTICE_H - 20:
				break
	if not line.is_empty() and y <= ny + NOTICE_H - 20:
		t.draw_string_c(line, nx + 8, y, col, null, 13)


func _draw_members(t, my_roi) -> void:
	var mx: int = my_roi.x + MEMBER_X
	var my: int = my_roi.y + MEMBER_Y
	t.draw_rect_c(Color(0.1, 0.1, 0.13, 0.9), mx, my, MEMBER_W,
		MEMBER_HEADER_H + MEMBER_ROW_COUNT * MEMBER_ROW_H, true)
	t.draw_border(Color(0.3, 0.3, 0.36, 1.0), mx, my, MEMBER_W,
		MEMBER_HEADER_H + MEMBER_ROW_COUNT * MEMBER_ROW_H, 1)

	# 表头
	var heads := ["名字", "等级", "职业", "职位", "状态"]
	var colx := [8, 150, 210, 300, 400]
	for i in range(heads.size()):
		t.draw_string_c(str(heads[i]), mx + int(colx[i]), my + 5, Color(0.85, 0.8, 0.6), null, 12)
	t.draw_line_c(Color(0.3, 0.3, 0.36, 1.0), mx, my + MEMBER_HEADER_H, mx + MEMBER_W, my + MEMBER_HEADER_H, 1)

	# 行
	var ranks := ["会长", "副会长", "长老", "成员"]
	var jobs := ["战士", "法师", "道士"]
	for i in range(mini(members.size(), MEMBER_ROW_COUNT)):
		var m = members[i]
		var ry: int = my + MEMBER_HEADER_H + i * MEMBER_ROW_H
		var nm := str(m.get("name", "?"))
		var lv := int(m.get("level", 0))
		var job := int(m.get("job", 0))
		var rank := int(m.get("rank", 3))
		var online := bool(m.get("online", false))
		t.draw_string_c(nm, mx + 8, ry + 3, Color(0.9, 0.87, 0.8), null, 12)
		t.draw_string_c(str(lv), mx + 150, ry + 3, Color(0.82, 0.8, 0.75), null, 12)
		t.draw_string_c(jobs[clampi(job, 0, 2)], mx + 210, ry + 3, Color(0.82, 0.8, 0.75), null, 12)
		t.draw_string_c(ranks[clampi(rank, 0, 3)], mx + 300, ry + 3,
			Color(0.95, 0.85, 0.5) if rank == 0 else Color(0.82, 0.8, 0.75), null, 12)
		t.draw_string_c("在线" if online else "离线", mx + 400, ry + 3,
			Color(0.5, 0.9, 0.5) if online else Color(0.55, 0.55, 0.6), null, 12)


func _draw_chat(t, my_roi) -> void:
	var cx: int = my_roi.x + TEXT_X
	var cy: int = my_roi.y + TEXT_Y
	t.draw_rect_c(Color(0.1, 0.1, 0.13, 0.9), cx, cy, TEXT_W, 284, true)
	t.draw_border(Color(0.3, 0.3, 0.36, 1.0), cx, cy, TEXT_W, 284, 1)

	var y := cy + 6
	for m in chat_lines:
		if y > cy + 284 - 18:
			break
		t.draw_string_c("%s: %s" % [str(m.get("name", "?")), str(m.get("text", ""))],
			cx + 8, y, Color(0.86, 0.84, 0.78), null, 13)
		y += 20

	# 输入框
	var iy: int = my_roi.y + 336
	t.draw_rect_c(Color(0.06, 0.06, 0.08, 0.9), cx, iy, TEXT_W, 26, true)
	t.draw_border(Color(0.5, 0.45, 0.3, 1.0) if input_active else Color(0.32, 0.32, 0.38, 1.0),
		cx, iy, TEXT_W, 26, 1)
	var show_txt := input_text if not input_text.is_empty() else "（点击输入行会聊天）"
	t.draw_string_c(show_txt, cx + 6, iy + 5,
		Color(0.9, 0.88, 0.82) if not input_text.is_empty() else Color(0.45, 0.45, 0.5), null, 13)


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
		if mpos.y >= my_roi.y + 28 and mpos.y < my_roi.y + 48:
			var tx: int = my_roi.x + 20
			for i in range(3):
				if mpos.x >= tx + i * 60 and mpos.x < tx + i * 60 + 56:
					cur_page = i
					return true
		# 聊天输入框
		if cur_page == PAGE_CHAT:
			var iy: int = my_roi.y + 336
			if mpos.y >= iy and mpos.y < iy + 26:
				input_active = true
				return true
		return true

	# 输入
	if input_active and event is InputEventKey and event.pressed and cur_page == PAGE_CHAT:
		if event.keycode == KEY_ENTER or event.keycode == KEY_KP_ENTER:
			if not input_text.is_empty() and on_send_chat.is_valid():
				on_send_chat.call(self, input_text)
			input_text = ""
			return true
		elif event.keycode == KEY_BACKSPACE:
			if input_text.length() > 0:
				input_text = input_text.substr(0, input_text.length() - 1)
			return true
		elif event.unicode > 0 and event.unicode != 127:
			input_text += char(event.unicode)
			return true
	return false
