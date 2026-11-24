extends RefCounted
class_name UiRoi
## 矩形区域 + 裁剪
##
## 【逐行翻译】client/src/widget.roi.hpp:1-60  Widget::ROI
##
## 作用：UI 绘制时逐层向下传递可用区域。
##      父容器把自己的 ROI 传给子控件，子控件再与自身区域求交，
##      最终超出父容器的部分被裁掉（不是靠 clip_children，而是靠坐标与尺寸求交）。
##
## 原版：
##   struct ROI { int x, y, w, h; }
##   crop(r) -> 与 r 求交（就地修改）

var x: int = 0
var y: int = 0
var w: int = 0
var h: int = 0


func _init(p_x: int = 0, p_y: int = 0, p_w: int = 0, p_h: int = 0) -> void:
	x = p_x
	y = p_y
	w = p_w
	h = p_h


## widget.roi.hpp:33 empty()
func empty() -> bool:
	return w <= 0 or h <= 0


## widget.roi.hpp:28 in()
func contains(px: int, py: int) -> bool:
	return px >= x and px < x + w and py >= y and py < y + h


## widget.roi.hpp:33 overlap()
func overlap(r: UiRoi) -> bool:
	return x < r.x + r.w and r.x < x + w and y < r.y + r.h and r.y < y + h


## widget.roi.hpp:38 crop() —— 与 r 求交，就地修改
## 对应 mathf::cropSegment
func crop(r: UiRoi) -> UiRoi:
	var x1 := maxi(x, r.x)
	var y1 := maxi(y, r.y)
	var x2 := mini(x + w, r.x + r.w)
	var y2 := mini(y + h, r.y + r.h)
	x = x1
	y = y1
	w = maxi(0, x2 - x1)
	h = maxi(0, y2 - y1)
	return self


func clone() -> UiRoi:
	return UiRoi.new(x, y, w, h)


func to_rect2() -> Rect2:
	return Rect2(x, y, w, h)


func to_str() -> String:
	return "ROI(%d,%d %dx%d)" % [x, y, w, h]


## 便捷：构造子区域
static func make(px: int, py: int, pw: int, ph: int) -> UiRoi:
	return UiRoi.new(px, py, pw, ph)
