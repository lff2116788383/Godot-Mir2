extends RefCounted
class_name UiMenuBoard
## 弹出菜单（右键菜单 / 下拉选项，ItemFlex + MenuItem 组合）
##
## 【逐行翻译】client/src/menuboard.cpp（165 行）
##            client/src/menuitem.cpp（206 行）
##            client/src/menubutton.cpp（36 行）
##            client/src/pullmenu.cpp（143 行）
##
## ── 结构（menuboard.cpp）──
##   MarginWrapper（圆角黑底 + 灰框，corner 由调用方给）
##     MenuCanvas（ItemFlex 纵向排列 MenuItem）
##       MenuItem * n
##   隔项插入 separator（m_separatorSpace 控制间距）
##   itemSpace 控制项间距
##
## ── MenuItem（menuitem.cpp）──
##   一个 MenuItem = gfxWidget（图标或文字）+ 三态（off/on/down）
##   点击时回调 onClickMenu(itemIndex)
##
## ── PullMenu（pullmenu.cpp）──
##   一个带箭头的小标题 + 展开列表
##   箭头三态贴图 0x00000301 的第 0/1/2 帧（22x22，需要 rotate 1）
##
## 本类不依赖控件树，用纯数据 + 绘制函数实现，便于复用。

const ARROW_TEX := 0x00000301
const ARROW_SIZE := 22

# ==================== 单条菜单项 ====================

class MenuItem:
	var text: String = ""
	var enabled: bool = true
	var checked: bool = false
	var separator_before: bool = false
	var icon_tex: int = -1
	var index: int = 0

	func _init(p_text: String = "", p_index: int = 0) -> void:
		text = p_text
		index = p_index


# ==================== 菜单板 ====================

var items: Array = []
## 项间距 / 分隔条间距（menuboard.cpp 的 m_itemSpace / m_separatorSpace）
var item_space := 2
var separator_space := 6
## 圆角
var corner := 3
## 内边距
var margin := 4
## 项高
var row_h := 20
## 最小宽度
var min_width := 90

var on_click: Callable = Callable()  ## Callable(item_index)

var _canvas_ref = null
var hover := -1
var hit_areas: Dictionary = {}


func _init() -> void:
	pass


func set_canvas(c) -> void:
	_canvas_ref = c


## 加一项，返回索引
func add_item(text: String, enabled: bool = true, checked: bool = false,
		separator_before: bool = false) -> int:
	var it := MenuItem.new(text, items.size())
	it.enabled = enabled
	it.checked = checked
	it.separator_before = separator_before
	items.append(it)
	return it.index


func count() -> int:
	return items.size()


func clear() -> void:
	items.clear()
	hover = -1


## 计算面板尺寸
func board_size() -> Vector2i:
	var w: int = min_width
	for it in items:
		var tw: int = it.text.length() * 13 + 24
		w = maxi(w, tw)
	var h: int = margin * 2
	for i in range(items.size()):
		var it: MenuItem = items[i]
		if it.separator_before and i > 0:
			h += separator_space
		h += row_h + item_space
	if h > margin * 2:
		h -= item_space   # 最后一项后面不加间距
	return Vector2i(w, h)


## 绘制；返回面板矩形
func draw(canvas, x: int, y: int) -> Rect2:
	if _canvas_ref != null:
		canvas = _canvas_ref
	var sz := board_size()
	var w := sz.x
	var h := sz.y
	hit_areas.clear()

	# 黑底 + 灰框（menuboard.cpp 的 bgDrawFunc）
	canvas.draw_rect_c(Color(0, 0, 0, 1), x, y, w, h, true)
	canvas.draw_border(Color(0.5, 0.5, 0.5, 1.0), x, y, w, h, 1)

	var cy: int = y + margin
	for i in range(items.size()):
		var it: MenuItem = items[i]
		if it.separator_before and i > 0:
			# 分隔线
			canvas.draw_line_c(Color(0.35, 0.35, 0.35, 1.0), x + 4, cy + separator_space / 2,
				x + w - 4, cy + separator_space / 2, 1)
			cy += separator_space

		# 悬停高亮
		if i == hover and it.enabled:
			canvas.draw_rect_c(Color(0.25, 0.25, 0.3, 1.0), x + 2, cy, w - 4, row_h, true)

		# 勾选标记
		var tx: int = x + margin
		if it.checked:
			canvas.draw_string_c("✓", tx, cy + 2, Color(0.4, 0.95, 0.4), null, 13)
			tx += 14
		elif it.icon_tex >= 0:
			var t := ProgUseTex.retrieve(it.icon_tex)
			if t != null:
				canvas.tex_draw_region(t, Rect2(0, 0, t.get_width(), t.get_height()),
					tx, cy + 2, 16, 16)
			tx += 18

		# 文字（禁用置灰）
		var col := Color(0.92, 0.9, 0.84) if it.enabled else Color(0.45, 0.45, 0.45)
		canvas.draw_string_c(it.text, tx, cy + 2, col, null, 13)

		hit_areas["item_%d" % i] = Rect2(x + 2, cy, w - 4, row_h)
		cy += row_h + item_space

	return Rect2(x, y, w, h)


## 鼠标移动：更新 hover 并返回是否落在菜单内
func handle_move(px: int, py: int) -> bool:
	hover = -1
	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			hover = int(str(key).substr(5))
			return true
	return false


## 点击：命中则回调，返回是否消费
func handle_click(px: int, py: int) -> bool:
	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			var idx: int = int(str(key).substr(5))
			if idx < items.size() and items[idx].enabled:
				if on_click.is_valid():
					on_click.call(idx)
				return true
	return false


## PullMenu 箭头贴图帧（0 关 / 1 开 / 2 按下）
static func arrow_frame(open: bool, down: bool) -> int:
	if down:
		return 2
	if open:
		return 1
	return 0


## 画一个下拉菜单的标题条 + 箭头
## 返回标题条矩形
static func draw_pull_title(canvas, x: int, y: int, title: String,
		w: int, h: int, open: bool) -> Rect2:
	canvas.draw_rect_c(Color(0.14, 0.14, 0.17, 0.96), x, y, w, h, true)
	canvas.draw_border(Color(0.42, 0.42, 0.48, 1.0), x, y, w, h, 1)
	canvas.draw_string_c(title, x + 4, y + (h - 12) / 2, Color(0.92, 0.9, 0.84), null, 12)
	# 箭头 22x22 靠右
	var ax: int = x + w - ARROW_SIZE
	var ay: int = y + (h - ARROW_SIZE) / 2
	var t := ProgUseTex.retrieve(ARROW_TEX)
	if t != null:
		var frame := arrow_frame(open, false)
		# 动画帧图：横向或纵向排布，这里按纵向取第 frame 帧
		var fh: int = t.get_height() / 3
		canvas.tex_draw_region(t, Rect2(0, frame * fh, t.get_width(), fh),
			ax, ay, ARROW_SIZE, ARROW_SIZE)
	else:
		canvas.draw_string_c("▼" if open else "▶", ax + 4, ay + 4, Color(0.9, 0.85, 0.6), null, 12)
	return Rect2(x, y, w, h)
