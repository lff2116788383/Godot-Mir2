extends RefCounted
class_name ItemLayout
## 物品列表布局算法
##
## 【逐行翻译】client/src/itemflex.cpp + itemflex.hpp + itembox.hpp
##
## 原版 ItemFlex / ItemBox 是 Widget 子类（渲染 + 布局混在一起）。
## 按分层方案（docs/translation-layers.md），这里只翻译**布局算法**，
## 实际绘制交给 Godot Control，见 L4。
##
## 原版三种对齐方式（itemalign.hpp:3-22）：
##
##   UPLEFT         +---+ +------+         +---------+
##                  |   | |      |         |         |
##                  |   | +------+         +---------+
##                  |   |                  +---+
##                  +---+                  |   |
##                                         |   |
##                                         +---+
##
##   CENTER         +---+                  +---------+
##                  |   | +-------+        |         |
##                  |   | |       |        +---------+
##                  |   | +-------+           +---+
##                  +---+                     |   |
##                                            |   |
##                                            +---+
##
##   DOWNRIGHT      +---+                  +---------+
##                  |   |                  |         |
##                  |   | +------+         +---------+
##                  |   | |      |               +---+
##                  +---+ +------+               |   |
##                                               |   |
##                                               +---+

## ItemAlign（itemalign.hpp:17）
const ALIGN_UPLEFT := 0
const ALIGN_CENTER := 1
const ALIGN_DOWNRIGHT := 2

## 方向常量（用于对齐锚点）
const DIR_UPLEFT := 8
const DIR_UPRIGHT := 2
const DIR_UP := 1
const DIR_LEFT := 7
const DIR_DOWNLEFT := 6


## 一个已布局物品的结果
class ItemPlacement:
	var index: int = 0
	## 是否可见（原版 localShow()）
	var visible: bool = true
	## 锚点方向（决定从哪条边对齐）
	var dir: int = DIR_UPLEFT
	## 锚点坐标
	var x: int = 0
	var y: int = 0
	## 宽高
	var w: int = 0
	var h: int = 0


## 布局计算器：对应 ItemFlex 的 addItem 偏移累积逻辑
##   vbox       : true 竖排 / false 横排
##   align      : ALIGN_*
##   head_space : 首个物品前的留白
##   item_space : 物品间距
##   canvas_w/h : 容器尺寸（CENTER/DOWNRIGHT 对齐需要）
static func compute_layout(vbox: bool, align: int,
		items: Array, head_space: int, item_space: int,
		canvas_w: int, canvas_h: int) -> Array:

	var result: Array = []
	var offset := head_space

	for i in range(items.size()):
		var it: Dictionary = items[i]
		var w := int(it.get("w", 0))
		var h := int(it.get("h", 0))
		var visible := bool(it.get("visible", true))

		var p := ItemPlacement.new()
		p.index = i
		p.visible = visible
		p.w = w
		p.h = h

		# itemflex.cpp:107-133 的对齐分支
		match align:
			ALIGN_UPLEFT:
				p.dir = DIR_UPLEFT
				p.x = 0 if vbox else offset
				p.y = offset if vbox else 0

			ALIGN_DOWNRIGHT:
				p.dir = DIR_UPRIGHT if vbox else DIR_DOWNLEFT
				p.x = (canvas_w - 1) if vbox else offset
				p.y = offset if vbox else (canvas_h - 1)

			ALIGN_CENTER:
				p.dir = DIR_UP if vbox else DIR_LEFT
				p.x = (canvas_w / 2) if vbox else offset
				p.y = offset if vbox else (canvas_h / 2)

			_:
				p.dir = DIR_UPLEFT
				p.x = 0 if vbox else offset
				p.y = offset if vbox else 0

		result.append(p)

		# 只有可见物品推进偏移（itemflex.cpp:81-95）
		if visible:
			offset += h if vbox else w
			offset += item_space

	return result


## 容器总尺寸（itemflex.cpp 的 canvasW/canvasH）
static func canvas_size(vbox: bool, items: Array, head_space: int, item_space: int,
		tail_space: int = 0) -> Vector2i:
	var main := head_space + tail_space
	var cross := 0
	var visible_count := 0
	for it in items:
		if not bool(it.get("visible", true)):
			continue
		visible_count += 1
		var w := int(it.get("w", 0))
		var h := int(it.get("h", 0))
		main += h if vbox else w
		cross = maxi(cross, w if vbox else h)
	if visible_count > 1:
		main += item_space * (visible_count - 1)
	if vbox:
		return Vector2i(cross, main)
	return Vector2i(main, cross)


## 首个/末个可见物品的索引（itembox.hpp 的 firstShowContainer/lastShowContainer）
static func first_visible(items: Array) -> int:
	for i in range(items.size()):
		if bool(items[i].get("visible", true)):
			return i
	return -1


static func last_visible(items: Array) -> int:
	for i in range(items.size() - 1, -1, -1):
		if bool(items[i].get("visible", true)):
			return i
	return -1


# ==================== 背包格坐标 ====================

## 背包网格（sysconst.hpp:83-87）
const SYS_INVGRIDGW := 10
const SYS_INVGRIDGH := 10
const SYS_INVGRIDPW := 38
const SYS_INVGRIDPH := 38

## 背包格索引 -> 网格坐标（10x10 背包）
static func inv_grid_pos(index: int) -> Vector2i:
	return Vector2i(index % SYS_INVGRIDGW, index / SYS_INVGRIDGW)


## 网格坐标 -> 背包格索引
static func inv_grid_index(gx: int, gy: int) -> int:
	if gx < 0 or gy < 0 or gx >= SYS_INVGRIDGW or gy >= SYS_INVGRIDGH:
		return -1
	return gy * SYS_INVGRIDGW + gx


## 背包格像素坐标（左上角）
static func inv_grid_pixel(index: int, origin: Vector2i = Vector2i.ZERO) -> Vector2i:
	var g := inv_grid_pos(index)
	return origin + Vector2i(g.x * SYS_INVGRIDPW, g.y * SYS_INVGRIDPH)


## 背包容量
static func inv_capacity() -> int:
	return SYS_INVGRIDGW * SYS_INVGRIDGH


# ==================== 腰带（6 格） ====================

## SDBelt 固定 6 格（serdesmsg.hpp:461）
const BELT_SLOT_COUNT := 6


static func belt_slot_valid(slot: int) -> bool:
	return slot >= 0 and slot < BELT_SLOT_COUNT
