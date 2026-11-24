extends RefCounted
class_name UiOverHead
## 角色头顶信息板（血条 + 数字显血 + 喊话气泡 + 队长标识 + buff 图标 + 摊位招牌）
##
## 【逐行翻译】client/src/gui/overheadboard/overheadboard.hpp + .cpp（502 行）
##
## ── 纵向堆叠顺序（overheadboard.cpp:m_flex 的 childList 顺序）──
##   1. m_sayBoard    喊话气泡（MessageStackBoard，宽 160，corner 3，留 5000ms，最多 10 条）
##   2. m_teamToken   队长标识（TexAniBoard 0x00000220 = 29x38）
##   3. m_iconArea    buff 图标区（每格 10x10）
##   4. 血条组        数值文字 + 空条 + 满条裁剪
##   5. m_stallBar    摊位招牌
##   全部居中（ItemAlign::CENTER）纵向排列。
##
## ── 血条尺寸规则（关键，容易算错）──
##   BAR_W_MIN = 40, BAR_W_MARGIN = 0（源码注释：disable inset currently）
##   buffIconDrawW = 10
##   buffIconsPerRow():
##     数字显血开 -> (max(40, numbers.w) + 9) / 10   // 向上取整
##     数字显血关 -> 3
##   barWidth() = buffIconsPerRow() * 10 + 2 * max(1, BAR_W_MARGIN)
##     注意 MARGIN=0 时仍按 max(1,0)=1 算，所以是 +2，源码就是这么写的
##   barWidthClip() = barWidth() * hpRatio
##   barHeight() = 空条贴图高
##
## ── 空/满条贴图（按是否数字显血选）──
##   数字显血: empty=0x00000016(101x7) full=0x00000017(101x7)
##   不显血  : empty=0x00000014(32x4)  full=0x00000015(32x4)
##
## ── 显隐规则（overheadboard.cpp:showBar）──
##   摊位中 -> 不显血条
##   RTCFG_显示血条     -> 总是显
##   RTCFG_满血隐藏血条 -> 只在 hp < maxHP 时显
##
## 本类不改控件树，只提供 draw()：由 actor 在头顶位置调用。

const BAR_W_MIN := 40
const BAR_W_MARGIN := 0
const BUFF_ICON_W := 10
const BUFF_ICON_H := 10
const BUFF_ICONS_PER_ROW := 3
const SAY_WIDTH := 160
const SAY_SHOW_TIME := 5000
const SAY_ENTRY_LIMIT := 10
const SAY_MARGIN := 2

## 数字显血 / 显示血条 / 满血隐藏 —— 对应三个运行时配置项
var cfg_show_number := true
var cfg_show_bar := true
var cfg_hide_full_bar := false

## 数据
var hp := 0
var max_hp := 0
var has_health := false
var is_team_leader := false
var stall_name: String = ""     ## 非空表示正在摆摊
var buff_list: Array = []       ## [{ "gfx_id": int, "favor": int }]
## 喊话：{ "text": String, "ms": int 剩余毫秒 }
var says: Array = []

var _say_w := SAY_WIDTH


func set_health(p_hp: int, p_max_hp: int) -> void:
	hp = p_hp
	max_hp = p_max_hp
	has_health = p_max_hp > 0


func hp_ratio() -> float:
	if max_hp <= 0:
		return 1.0
	return clampf(float(hp) / float(max_hp), 0.0, 1.0)


## 喊一根话（对应 addSay，带存活时间）
func add_say(text: String) -> void:
	says.append({"text": text, "ms": SAY_SHOW_TIME})
	if says.size() > SAY_ENTRY_LIMIT:
		says = says.slice(says.size() - SAY_ENTRY_LIMIT)


func update(dt: float) -> void:
	var ms := int(dt * 1000.0)
	var kept: Array = []
	for s in says:
		var left: int = int(s["ms"]) - ms
		if left > 0:
			s["ms"] = left
			kept.append(s)
	says = kept


# ==================== 尺寸计算（逐条对应 cpp）====================

func show_number() -> bool:
	return cfg_show_number


func show_stall() -> bool:
	return not stall_name.is_empty()


## overheadboard.cpp:showBar
func show_bar() -> bool:
	if show_stall():
		return false
	if cfg_show_bar:
		return true
	if cfg_hide_full_bar:
		return has_health and hp < max_hp
	return false


func bar_tex_empty() -> int:
	return 0x00000016 if show_number() else 0x00000014


func bar_tex_full() -> int:
	return 0x00000017 if show_number() else 0x00000015


## overheadboard.cpp:buffIconsPerRow
func buff_icons_per_row() -> int:
	if show_number():
		var nw: int = 0
		if has_health:
			nw = maxi(BAR_W_MIN, _numbers_text().length() * 8)
		else:
			nw = BAR_W_MIN
		return (maxi(BAR_W_MIN, nw) + BUFF_ICON_W - 1) / BUFF_ICON_W
	return BUFF_ICONS_PER_ROW


## overheadboard.cpp:barWidth
func bar_width() -> int:
	return buff_icons_per_row() * BUFF_ICON_W + 2 * maxi(1, BAR_W_MARGIN)


## overheadboard.cpp:barWidthClip
func bar_width_clip() -> int:
	if not has_health:
		return bar_width()
	return int(round(bar_width() * hp_ratio()))


func bar_height() -> int:
	var t := ProgUseTex.retrieve(bar_tex_empty())
	if t == null:
		return 7 if show_number() else 4
	return t.get_height()


## 数值文字 "%*d/%d"（右对齐到 maxHP 的宽度）
func _numbers_text() -> String:
	if not has_health:
		return ""
	var digits: int = str(max_hp).length()
	return "%*d/%d" % [maxi(1, digits), hp, max_hp]


## 整板高度（用于摆放上方元素）
func total_height() -> int:
	var h := 0
	if not says.is_empty():
		h += says.size() * 16 + SAY_MARGIN * 2
	if is_team_leader:
		h += 38
	if not buff_list.is_empty():
		var rows: int = (buff_list.size() + buff_icons_per_row() - 1) / buff_icons_per_row()
		h += rows * BUFF_ICON_H
	if show_bar():
		h += bar_height()
	if show_stall():
		h += 20
	return h


# ==================== 绘制 ====================
## 在锚点 (cx, bottom_y) 处向上堆叠绘制
## 返回整板的命中矩形（含躺位招牌的热区）
func draw(canvas, cx: int, bottom_y: int) -> Rect2:
	var y := bottom_y
	var half_w := bar_width() / 2

	# 5. 摊位招牌（最下）
	if show_stall():
		y -= 20
		UiStallBar.draw(canvas, cx - UiStallBar.bar_width(
			int(canvas.measure_text(stall_name, null, 14).x)) / 2, y, stall_name)

	# 4. 血条组
	if show_bar():
		var bh := bar_height()
		y -= bh
		var bw := bar_width()
		var bx := cx - bw / 2

		# 空条（数字显血时先铺黑底）
		var empty_tex := ProgUseTex.retrieve(bar_tex_empty())
		if empty_tex != null:
			if show_number():
				canvas.draw_rect_c(Color(0, 0, 0, 1), bx, y, bw, bh, true)
			# 源图裁掉左右各 margin（max(1,MARGIN)=1）
			var m := maxi(1, BAR_W_MARGIN)
			var src_w: int = empty_tex.get_width() - 2 * m
			canvas.tex_draw_region(empty_tex, Rect2(m, 0, src_w, empty_tex.get_height()),
				bx + m, y, bw - 2 * m, bh)
		else:
			canvas.draw_rect_c(Color(0.1, 0.1, 0.1, 0.85), bx, y, bw, bh, true)

		# 满条按血量比例裁剪
		var full_tex := ProgUseTex.retrieve(bar_tex_full())
		var clip_w := bar_width_clip()
		if full_tex != null and clip_w > 0:
			var m2 := maxi(1, BAR_W_MARGIN)
			var src_w2: int = full_tex.get_width() - 2 * m2
			canvas.tex_draw_region(full_tex, Rect2(m2, 0, src_w2, full_tex.get_height()),
				bx + m2, y, mini(clip_w, bw - 2 * m2), bh)
		elif clip_w > 0:
			canvas.draw_rect_c(Color(0.75, 0.12, 0.12, 0.95), bx, y, clip_w, bh, true)

		# 数字显血（居中压在条上）
		if show_number() and has_health:
			var s := _numbers_text()
			var tw: int = int(canvas.measure_text(s, null, 16).x)
			canvas.draw_string_c(s, cx - tw / 2, y - 1, Color(1, 1, 1), null, 16)

	# 3. buff 图标（10x10 网格）
	if not buff_list.is_empty():
		var per_row := buff_icons_per_row()
		var n := buff_list.size()
		var rows: int = (n + per_row - 1) / per_row
		var area_h: int = rows * BUFF_ICON_H
		y -= area_h
		var area_w: int = per_row * BUFF_ICON_W
		var ax: int = cx - area_w / 2
		for i in range(n):
			var b: Dictionary = buff_list[i]
			var col: int = i % per_row
			var row: int = i / per_row
			# 最后一行靠下（原版用 lastChildRow - childRow）
			var draw_row: int = rows - 1 - (rows - 1 - row)
			var ix: int = ax + col * BUFF_ICON_W
			var iy: int = y + draw_row * BUFF_ICON_H
			var gfx := ProgUseTex.retrieve(int(b.get("gfx_id", -1)))
			if gfx != null:
				canvas.tex_draw_region(gfx, Rect2(0, 0, gfx.get_width(), gfx.get_height()),
					ix, iy, BUFF_ICON_W, BUFF_ICON_H)
			# 边框：favor<0 红 / >0 绿 / =0 黄
			var fc := Color(1, 1, 0, 1)
			var fv := int(b.get("favor", 0))
			if fv < 0:
				fc = Color(1, 0, 0, 1)
			elif fv > 0:
				fc = Color(0, 1, 0, 1)
			canvas.draw_border(fc, ix, iy, BUFF_ICON_W, BUFF_ICON_H, 1)

	# 2. 队长标识 0x00000220（29x38）
	if is_team_leader:
		var token := ProgUseTex.retrieve(0x00000220)
		y -= 38
		if token != null:
			canvas.tex_draw(token, cx - token.get_width() / 2, y)
		else:
			canvas.draw_rect_c(Color(0.3, 0.25, 0.6, 0.9), cx - 14, y, 28, 38, true)
			canvas.draw_string_c("队长", cx - 12, y + 10, Color(1, 1, 0.7), null, 12)

	# 1. 喊话气泡（最上）
	if not says.is_empty():
		var box_h: int = says.size() * 16 + SAY_MARGIN * 2
		y -= box_h
		var bx2: int = cx - _say_w / 2
		canvas.draw_rect_c(Color(0, 0, 0, 128.0 / 255.0), bx2, y, _say_w, box_h, true)
		canvas.draw_border(Color(1, 1, 1, 0.3), bx2, y, _say_w, box_h, 1)
		var ly := y + SAY_MARGIN
		for s in says:
			canvas.draw_string_c(str(s["text"]).substr(0, 20), bx2 + SAY_MARGIN, ly,
				Color(1, 1, 0.85), null, 13)
			ly += 16

	var total_h: int = bottom_y - y
	return Rect2(cx - half_w, y, bar_width(), total_h)
