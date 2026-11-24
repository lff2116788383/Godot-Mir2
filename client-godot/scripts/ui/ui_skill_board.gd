extends UiWidget
class_name UiSkillBoard
## 技能面板
##
## 【逐行翻译】client/src/gui/skillboard/skillboard.hpp + skillboard.cpp
## 对照：
##   - skillboard.hpp:22-146   m_iconGfxList：63 个魔法的图标 ID 与网格坐标
##   - skillboard.hpp:167      getPageRectange() = {18, 44, 304, 329}
##   - skillboard.hpp:167-180  tabElem(tabIndex) 映射到元素类型
##
## ── 布局（照搬原版数值）──
##   右侧页签列，8 个页签（火/冰/雷/风/神圣/暗黑/幻影/无）
##   图标区 (18, 44, 304, 329)，每格 42x42
##   图标坐标由 m_iconGfxList 的 col/row 决定
##
## 数据来源：data/skill_icons.json（63 条，含分组）

const RoiRef := preload("res://scripts/ui/framework/ui_roi.gd")

## 图标区（getPageRectange）
const PAGE_X := 18
const PAGE_Y := 44
const PAGE_W := 304
const PAGE_H := 329

## 每个图标的格子尺寸（原版 42x42）
const CELL := 42

## 面板尺寸
const BOARD_W := 360
const BOARD_H := 400

## 页签（对应 MET_* 元素类型：火冰雷风神圣暗黑幻影无）
const TABS := ["火", "冰", "雷", "风", "神圣", "暗黑", "幻影", "无"]

var _data: Dictionary = {}
var _groups: Dictionary = {}
var _loaded := false

## 当前页签
var cur_tab := 0
## 悬停格
var _hover: Vector2i = Vector2i(-1, -1)

## 已学技能（名称集合，由服务端下发填充）
var learned: Dictionary = {}

## 贴图提供者：Callable(widget, tex_id, frame) -> Texture2D
var tex_provider: Callable = Callable()
## 点击技能回调：Callable(widget, magic_name)
var on_skill_click: Callable = Callable()


func _init(p_name: String = "SkillBoard") -> void:
	widget_name = p_name
	w = BOARD_W
	h = BOARD_H
	show = false
	on_event = _handle_event
	_load_data()


func _load_data() -> void:
	if _loaded:
		return
	var p := "res://data/skill_icons.json"
	if not FileAccess.file_exists(p):
		push_warning("技能图标表不存在")
		return
	var f := FileAccess.open(p, FileAccess.READ)
	if f == null:
		return
	var parsed = JSON.parse_string(f.get_as_text())
	f.close()
	if parsed == null:
		return
	_data = parsed.get("magics", {})
	_groups = parsed.get("groups", {})
	_loaded = true


## 当前页签的技能名列表
func tab_skills() -> Array:
	if not _loaded:
		return []
	var tab_name: String = TABS[cur_tab] if cur_tab < TABS.size() else "无"
	return _groups.get(tab_name, [])


## 技能图标信息
func magic_info(name_: String) -> Dictionary:
	return _data.get(name_, {})


## 绘制
func draw_self(canvas, my_roi) -> void:
	var t = canvas
	# 背景
	t.draw_rect_c(Color(0.07, 0.07, 0.1, 0.96), my_roi.x, my_roi.y, my_roi.w, my_roi.h, true)
	t.draw_border(Color(0.5, 0.45, 0.3, 1.0), my_roi.x, my_roi.y, my_roi.w, my_roi.h, 2)
	t.draw_string_c("技能", my_roi.x + 10, my_roi.y + 6, Color(0.95, 0.85, 0.5), null, 16)
	t.draw_string_c("×", my_roi.x + my_roi.w - 20, my_roi.y + 6, Color(0.9, 0.75, 0.5), null, 16)

	# 图标区边框
	var px: int = my_roi.x + PAGE_X
	var py: int = my_roi.y + PAGE_Y
	t.draw_rect_c(Color(0.1, 0.1, 0.13, 0.9), px, py, PAGE_W, PAGE_H, true)
	t.draw_border(Color(0.3, 0.3, 0.36, 1.0), px, py, PAGE_W, PAGE_H, 1)

	# 画当前页签的技能图标
	var skills := tab_skills()
	for nm in skills:
		var info := magic_info(nm)
		if info.is_empty():
			continue
		var cx: int = px + int(info.get("col", 0)) * CELL
		var cy: int = py + int(info.get("row", 0)) * CELL
		if cx + CELL > px + PAGE_W or cy + CELL > py + PAGE_H:
			continue

		var is_learned := learned.has(nm)
		# 技能图标：素材 0x05xxxxxx 图库，缺失时画色块
		var icon_id := int(info.get("icon", 0))
		var drew := false
		if tex_provider.is_valid():
			var tex = tex_provider.call(self, icon_id, 0)
			if tex != null:
				t.tex_draw_region(tex, Rect2(0, 0, tex.get_width(), tex.get_height()),
					cx + 4, cy + 4, CELL - 8, CELL - 8,
					Color.WHITE if is_learned else Color(0.45, 0.45, 0.5, 1.0))
				drew = true
		if not drew:
			var col := Color(0.3, 0.45, 0.65, 0.85) if is_learned else Color(0.2, 0.2, 0.24, 0.9)
			if bool(info.get("passive", false)):
				col = Color(0.55, 0.45, 0.2, 0.85) if is_learned else Color(0.25, 0.22, 0.15, 0.9)
			t.draw_rect_c(col, cx + 3, cy + 3, CELL - 6, CELL - 6, true)
			t.draw_string_c(nm.substr(0, 1), cx + 12, cy + 10,
				Color(0.9, 0.9, 0.92) if is_learned else Color(0.5, 0.5, 0.55), null, 14)

		t.draw_border(Color(0.35, 0.35, 0.42, 1.0), cx, cy, CELL - 1, CELL - 1, 1)

	# 右侧页签
	var tab_x: int = px + PAGE_W + 4
	for i in range(TABS.size()):
		var ty: int = py + i * 40
		var bg := Color(0.2, 0.2, 0.26, 0.95) if i == cur_tab else Color(0.13, 0.13, 0.17, 0.9)
		t.draw_rect_c(bg, tab_x, ty, 26, 36, true)
		t.draw_border(Color(0.38, 0.38, 0.45, 1.0), tab_x, ty, 26, 36, 1)
		# 竖排文字
		var label: String = TABS[i]
		for k in range(label.length()):
			t.draw_string_c(label[k], tab_x + 8, ty + 2 + k * 15, Color(0.88, 0.85, 0.78), null, 12)


## 事件
func _handle_event(self_widget, event: InputEvent, valid: bool, my_roi) -> bool:
	if not show:
		return false
	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_LEFT and event.pressed:
		var mpos: Vector2 = event.position
		if not my_roi.contains(int(mpos.x), int(mpos.y)):
			return false
		# 关闭
		if mpos.x > my_roi.x + my_roi.w - 28 and mpos.y < my_roi.y + 28:
			show = false
			return true

		var px: int = my_roi.x + PAGE_X
		var py: int = my_roi.y + PAGE_Y
		var tab_x: int = px + PAGE_W + 4

		# 页签
		if mpos.x >= tab_x and mpos.x < tab_x + 26:
			var idx: int = int((mpos.y - py) / 40)
			if idx >= 0 and idx < TABS.size():
				cur_tab = idx
				return true

		# 图标区
		if mpos.x >= px and mpos.x < px + PAGE_W and mpos.y >= py and mpos.y < py + PAGE_H:
			var lx: int = int((mpos.x - px) / CELL)
			var ly: int = int((mpos.y - py) / CELL)
			for nm in tab_skills():
				var info := magic_info(nm)
				if int(info.get("col", -1)) == lx and int(info.get("row", -1)) == ly:
					if on_skill_click.is_valid():
						on_skill_click.call(self, nm)
					return true
		return true
	return false


## 学习技能（服务端下发改动）
func learn(name_: String) -> void:
	learned[name_] = true


func is_open() -> bool:
	return UiWidget.eval_bool(show, self, false)
