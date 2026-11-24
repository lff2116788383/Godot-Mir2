extends RefCounted
class_name UiStallBar
## 摊位招牌条（角色头顶的「某某的摊位」）
##
## 【逐行翻译】client/src/gui/stallbar/stallbar.hpp + stallbar.cpp（150 行）
##
## 结构（stallbar.cpp 的部件树）：
##   m_name   TextBoard  黄色字，文本来自 hero->getStallName()
##   m_bg     ImageBoard 0x00000235（90x20），宽 = max(name.w, 60) + 10
##   m_frame  GfxResizeBoard 取 m_frameImage(0x00000234, 100x20) 的 (10,0,80,20) 区域
##            再拉伸到 (nameSpaceWidth, 20) —— 即九宫格中间那段可伸缩
##   m_gfx    GfxCenterBoard  把 bg/name/frame 居中拼起来
##   m_button TrigfxButton    三态用同一个 gfx（原版 gfxList 里三项相同）
##
## 关键细节：nameSpaceWidth() = max(m_name.w(), 60)，最小 60 宽。

const MIN_NAME_SPACE := 60


## 计算招牌宽度（stallbar.hpp:nameSpaceWidth）
static func name_space_width(name_w: int) -> int:
	return maxi(name_w, MIN_NAME_SPACE)


## 招牌总宽 = nameSpaceWidth + 10
static func bar_width(name_w: int) -> int:
	return name_space_width(name_w) + 10


## 在 (x,y) 画招牌
## 返回命中矩形（调用方登记点击区）
static func draw(canvas, x: int, y: int, name: String, hover: bool = false) -> Rect2:
	var nw: int = int(canvas.measure_text(name, null, 14).x)
	var space := name_space_width(nw)
	var total_w := space + 10
	var h := 20

	# 底板 0x00000235（90x20），横向拉伸到 total_w
	var bg := ProgUseTex.retrieve(0x00000235)
	if bg != null:
		canvas.tex_draw_region(bg, Rect2(0, 0, bg.get_width(), bg.get_height()),
			x, y, total_w, h)
	else:
		canvas.draw_rect_c(Color(0.35, 0.28, 0.1, 0.92), x, y, total_w, h, true)
		canvas.draw_border(Color(0.7, 0.58, 0.28, 1.0), x, y, total_w, h, 1)

	# 边框：取 0x00000234 的 (10,0,80,20) 段再横向拉伸
	var frame := ProgUseTex.retrieve(0x00000234)
	if frame != null:
		canvas.tex_draw_region(frame, Rect2(10, 0, 80, 20),
			x + 5, y, space, h)

	# 名字（原版 font.color = YELLOW_A255）
	if hover:
		canvas.draw_string_c(name, x + 5 + (space - nw) / 2, y + 3,
			Color(1, 1, 0.8), null, 14)
	else:
		canvas.draw_string_c(name, x + 5 + (space - nw) / 2, y + 3,
			Color(1, 1, 0, 1), null, 14)

	return Rect2(x, y, total_w, h)
