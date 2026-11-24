extends RefCounted
class_name MapObjects
## 地图物件层（地面物件 + 上层物件 + 地面物品 + 光照）
##
## 【逐行翻译】client/src/processrun.cpp:2388-2436 drawObject
##            client/src/processrun.cpp:295-425 draw（分层顺序）
##
## ══════════════ 原版绘制顺序（这是「传奇」画面层次的核心）══════════════
##   1. drawTile                          瓦片
##   2. drawObject(OBJD_GROUND)           地面物件（人踩在上面）
##   3. 火焰灰烬等地面魔法特效
##   4. drawGroundItem                    地面掉落物（金币/装备）
##   5. 角色（按 y 排序，同 y 再按 x）      —— 主循环里夹在两批物件之间
##   6. drawObject(OBJD_OVERGROUND0)      上层物件（低于玩家）
##   7. 火墙
##   8. drawObject(OBJD_OVERGROUND1)      上层物件（高于玩家，如树冠/屋檐）
##
##   所以「物件分两批画、角色夹在中间」是必须的，
##   否则树会盖住人、或者人被门槛挡住。
##
## ══════════════ 物件绘制细节（drawObject，逐条）══════════════
##   ★ 物件是「底部对齐格底」：dstY = (y+1) * GRID_Y - texH
##     不是顶部对齐。这就是为什么树冠能罩住下方的格子。
##   ★ 动画只有特定图库才推进（源码 TODO 注释明确列了 5 个 fileIndex）：
##       fileIndex = texID >> 16 ∈ {11, 26, 41, 56, 71}
##     其它带 animated 标记的图不动 —— 这是原版的实际情况，不是偷懒。
##   ★ 透明度：obj.alpha 非 0 -> 96；传参 alpha=true -> 128；否则 255
##
## ══════════════ 光照（LIGHT 位域）══════════════
##   valid:1 | radius:2 | alpha:2 | color:3
##   radius 分 4 档，color 分 8 档。原版用 proguse 的 0x00000090 系列贴图做叠加，
##   这里用同色圆形渐隐近似（素材里没有标准光照图，先保证视觉上"暗处更暗"）。

const GRID_X := 48
const GRID_Y := 32

## 参与动画推进的图库索引（processrun.cpp:2408-2412 的 TODO 列表）
const ANI_FILE_INDEX := [11, 26, 41, 56, 71]

## 物件动画节拍（m_aniTimer）；原版按 tickType 分组的全局计时器
## 这里用一个统一的累积时间 + tickType 分组偏移，效果等价
const TICK_MS := 120.0

## 光照颜色（8 档，按 color 位取）
const LIGHT_COLORS := [
	Color(1.00, 1.00, 1.00),   # 0 白
	Color(1.00, 0.85, 0.55),   # 1 暖黄
	Color(1.00, 0.55, 0.35),   # 2 橙红
	Color(0.85, 1.00, 0.75),   # 3 淡绿
	Color(0.60, 0.85, 1.00),   # 4 淡蓝
	Color(0.75, 0.65, 1.00),   # 5 紫
	Color(1.00, 0.45, 0.45),   # 6 红
	Color(0.90, 0.90, 0.75),   # 7 米白
]

## 光照半径（4 档，单位：格）
const LIGHT_RADII := [2, 4, 7, 11]

static var _anim_ms := 0.0


static func tick(dt: float) -> void:
	_anim_ms += dt * 1000.0


## 动画帧号（processrun.cpp:2413 m_aniTimer.frame(tickType, frameCount)）
static func ani_frame(tick_type: int, frame_count: int) -> int:
	if frame_count <= 1:
		return 0
	# 各 tickType 错开相位，避免整屏物件同步闪烁
	var phase: float = float(tick_type % 8) * 0.125
	var step: int = int((_anim_ms / TICK_MS) + phase)
	return step % frame_count


## texID 是否需要推进动画（只看图库索引）
static func is_animated_tex(tex_id: int) -> bool:
	var file_index: int = tex_id >> 16
	return ANI_FILE_INDEX.has(file_index)


## 取物件的实际贴图 id（含动画推进）
## 返回 -1 表示该物件本帧不画
static func obj_draw_tex(obj) -> int:
	if obj == null or not obj.valid:
		return -1
	var tid: int = obj.tex_id
	if obj.animated and is_animated_tex(tid):
		tid += ani_frame(obj.tick_type, obj.frame_count)
	return tid


## 物件透明度（drawObject 的 drawAlphaObj 三分支）
static func obj_alpha(obj, dim: bool = false) -> float:
	if obj != null and obj.alpha != 0:
		return 96.0 / 255.0
	if dim:
		return 128.0 / 255.0
	return 1.0


## 画一个物件（底部对齐格底）
## view: Vector2i 视口左上角像素偏移
## 返回是否有绘制
static func draw_one(canvas, md, x: int, y: int, idx: int, view: Vector2i,
		depth_filter: int, tex_fn: Callable) -> bool:
	var obj = md.obj_at(x, y, idx)
	if obj == null or not obj.valid:
		return false
	if obj.depth != depth_filter:
		return false

	var tid: int = obj_draw_tex(obj)
	if tid < 0:
		return false

	var tex: Texture2D = tex_fn.call(tid)
	if tex == null:
		return false

	var a: float = obj_alpha(obj)
	var dx: int = x * GRID_X - view.x
	# ★ 底部对齐：(y+1) * GRID_Y - texH
	var dy: int = (y + 1) * GRID_Y - view.y - tex.get_height()
	canvas.tex_draw(tex, dx, dy, Color(1, 1, 1, a))
	return true


## 画某一深度的所有物件（矩形区域）
## 返回绘制数量（供统计）
static func draw_layer(canvas, md, x0: int, y0: int, x1: int, y1: int,
		view: Vector2i, depth: int, tex_fn: Callable) -> int:
	if md == null:
		return 0
	var n := 0
	var y: int = y0
	while y <= y1:
		var x: int = x0
		while x <= x1:
			# 该格无物件直接跳过（比逐个查 obj_at 快）
			if md.obj_count[y * md.w + x] > 0:
				if draw_one(canvas, md, x, y, 0, view, depth, tex_fn):
					n += 1
				if draw_one(canvas, md, x, y, 1, view, depth, tex_fn):
					n += 1
			x += 1
		y += 1
	return n


# ==================== 地面物品 ====================

## 地面掉落物：{ gx, gy, item_id, count, highlighted }
## 对应 processrun.cpp:drawGroundItem（在角色之前画）
## 原版：物品图标带上下浮动 + 可拾取高亮（黄色描边）
const GROUND_ITEM_FLOAT_MS := 1200.0


static func draw_ground_items(canvas, items: Array, view: Vector2i,
		icon_fn: Callable, now_ms: float) -> int:
	var n := 0
	for it in items:
		var tex: Texture2D = icon_fn.call(int(it.get("item_id", 0)))
		if tex == null:
			continue
		var gx: int = int(it.get("gx", 0))
		var gy: int = int(it.get("gy", 0))
		# 上下浮动 3px（原版用 sin 做轻微起伏）
		var phase: float = fmod(now_ms / GROUND_ITEM_FLOAT_MS, 1.0)
		var fl: int = int(round(sin(phase * TAU) * 3.0))

		var cx: int = gx * GRID_X - view.x + GRID_X / 2
		var cy: int = gy * GRID_Y - view.y + GRID_Y / 2 - tex.get_height() / 2 + fl
		canvas.tex_draw(tex, cx - tex.get_width() / 2, cy)

		# 可拾取高亮（原版 hasHighlightedItem 用黄色 alpha 220）
		if bool(it.get("highlighted", false)):
			var w := tex.get_width()
			var h := tex.get_height()
			canvas.draw_border(Color(1, 1, 0, 220.0 / 255.0),
				cx - w / 2 - 1, cy - 1, w + 2, h + 2, 1)
		n += 1
	return n


# ==================== 光照 ====================

## 画光照（叠加层）
## 原版在物件之后统一压一层，这里按格子逐个画圆形渐隐
static func draw_light(canvas, md, x0: int, y0: int, x1: int, y1: int,
		view: Vector2i) -> int:
	if md == null:
		return 0
	var n := 0
	var y: int = y0
	while y <= y1:
		var x: int = x0
		while x <= x1:
			var lv: int = md.light[y * md.w + x]
			if (lv & 1) != 0:
				var radius: int = LIGHT_RADII[(lv >> 1) & 3]
				var alpha: int = (lv >> 3) & 3
				var color_idx: int = (lv >> 5) & 7
				var col: Color = LIGHT_COLORS[color_idx]
				# alpha 档位 0..3 映射成 40..160
				var a: float = (40.0 + alpha * 40.0) / 255.0
				var cx: int = x * GRID_X - view.x + GRID_X / 2
				var cy: int = y * GRID_Y - view.y + GRID_Y / 2
				# 用多层同心圆近似径向渐隐
				for ring in range(radius, 0, -1):
					var r: int = ring * GRID_X / 2
					var ra: float = a * (1.0 - float(radius - ring) / float(radius + 1))
					canvas.draw_circle_c(Color(col.r, col.g, col.b, ra), cx, cy, r)
				n += 1
			x += 1
		y += 1
	return n