extends UiWidget
class_name UiHeroStateBoard
## 人物状态板（自身 / 他人共用）
##
## 【逐行翻译】client/src/gui/myherostateboard/  +  client/src/gui/herostateboard/
##
## ── 两版的差异（对照两份 hpp 的 WearGrid 定义）──
##   myherostateboard   (自身)  m_equipCharX =  90, m_equipCharY = 200
##                              框贴图 0x06000000 = 328x466，关闭按钮 (288,13)
##   herostateboard     (他人)  m_equipCharX = 103, m_equipCharY = 229
##                              框贴图 0x00002020 = 284x348，关闭按钮 (249,312)
##                              另有「交易申请」(208,15) 与「加为好友」(208,30) 两个文字按钮
##   装备格布局两者相同结构、不同坐标，所以做成数据表 + 两套预设。
##
## ── 装备格尺寸语义（src 里逐条抄的）──
##   衣服   60x110   头盔 30x25   武器 45x90
##   鞋     默认 SYS_INVGRIDPW x SYS_INVGRIDPH（他人版 43x61）
##   项链/手镯/戒指/火把/魅力  默认格
##   注意 WLG_DRESS / HELMET / WEAPON 三格位置是「相对 m_equipCharX/Y 的偏移」，
##   其余格是面板绝对坐标 —— 原版就是这么写的，别统一。
##
## ── 物品落点（herostateboard.cpp:206-208）──
##   dstX = grid.x + (grid.w - texW) / 2
##   dstY = grid.y + (grid.h - texH) / (i == WLG_SHOES ? 1 : 2)
##   鞋用 1、其它用 2 —— 因为鞋是要踩在格子底边的。

const ProtocolDefRef := preload("res://scripts/core/protocol_def.gd")
const ItemGfxRef := preload("res://scripts/item/item_gfx.gd")
const ItemRecordRef := preload("res://scripts/item/item_record.gd")

## 格子默认尺寸（SYS_INVGRIDPW/PH）
const INV_GRID_W := 36
const INV_GRID_H := 32

## 预设：自身
const PRESET_SELF := {
	"w": 328, "h": 466,
	"frame_tex": 0x06000000,
	"char_x": 90, "char_y": 200,
	"close_x": 288, "close_y": 13,
	"has_peer_buttons": false,
}
## 预设：他人
const PRESET_PEER := {
	"w": 284, "h": 348,
	"frame_tex": 0x00002020,
	"char_x": 103, "char_y": 229,
	"close_x": 249, "close_y": 312,
	"has_peer_buttons": true,
}

## 装备格布局表
## key = WLG 常量，值 = [x, y, w, h, 相对角色锚点?]
## 相对锚点的三格（衣服/头盔/武器）在 myherostateboard.cpp:40-63 里就是
## m_equipCharX / m_equipCharY 的偏移，其余是绝对坐标
static func grid_layout_self() -> Dictionary:
	var ax := int(PRESET_SELF["char_x"])
	var ay := int(PRESET_SELF["char_y"])
	return {
		ProtocolDefRef.WLG_DRESS:    [ax, ay - 100, 60, 110],
		ProtocolDefRef.WLG_HELMET:   [ax + 10, ay - 135, 30, 25],
		ProtocolDefRef.WLG_WEAPON:   [ax - 50, ay - 120, 45, 90],
		ProtocolDefRef.WLG_SHOES:    [10, 240, INV_GRID_W, 56],
		ProtocolDefRef.WLG_NECKLACE: [168, 88, INV_GRID_W, INV_GRID_H],
		ProtocolDefRef.WLG_ARMRING0: [10, 155, INV_GRID_W, INV_GRID_H],
		ProtocolDefRef.WLG_ARMRING1: [168, 155, INV_GRID_W, INV_GRID_H],
		ProtocolDefRef.WLG_RING0:    [10, 195, INV_GRID_W, INV_GRID_H],
		ProtocolDefRef.WLG_RING1:    [168, 195, INV_GRID_W, INV_GRID_H],
		ProtocolDefRef.WLG_TORCH:    [88, 265, INV_GRID_W, INV_GRID_H],
		ProtocolDefRef.WLG_CHARM:    [128, 265, INV_GRID_W, INV_GRID_H],
	}


static func grid_layout_peer() -> Dictionary:
	var ax := int(PRESET_PEER["char_x"])
	var ay := int(PRESET_PEER["char_y"])
	return {
		ProtocolDefRef.WLG_DRESS:    [ax, ay - 100, 60, 110],
		ProtocolDefRef.WLG_HELMET:   [ax + 10, ay - 135, 30, 25],
		ProtocolDefRef.WLG_WEAPON:   [ax - 50, ay - 120, 45, 90],
		ProtocolDefRef.WLG_SHOES:    [20, 268, 43, 61],
		ProtocolDefRef.WLG_NECKLACE: [221, 94, 43, 43],
		ProtocolDefRef.WLG_ARMRING0: [20, 184, 43, 43],
		ProtocolDefRef.WLG_ARMRING1: [222, 184, 43, 43],
		ProtocolDefRef.WLG_RING0:    [20, 224, 43, 43],
		ProtocolDefRef.WLG_RING1:    [222, 224, 43, 43],
		ProtocolDefRef.WLG_TORCH:    [100, 294, 42, 43],
		ProtocolDefRef.WLG_CHARM:    [140, 294, 42, 43],
	}


## 槽位显示名（原版 WearGrid.type 字段）
const SLOT_NAMES := {
	ProtocolDefRef.WLG_DRESS: "衣服",
	ProtocolDefRef.WLG_HELMET: "头盔",
	ProtocolDefRef.WLG_WEAPON: "武器",
	ProtocolDefRef.WLG_SHOES: "鞋",
	ProtocolDefRef.WLG_NECKLACE: "项链",
	ProtocolDefRef.WLG_ARMRING0: "手镯",
	ProtocolDefRef.WLG_ARMRING1: "手镯",
	ProtocolDefRef.WLG_RING0: "戒指",
	ProtocolDefRef.WLG_RING1: "戒指",
	ProtocolDefRef.WLG_TORCH: "火把",
	ProtocolDefRef.WLG_CHARM: "护身符",
}

# ==================== 实例状态 ====================

## true = 自身，false = 他人
var is_self := true
## 装备槽 -> 物品 id（0 或 -1 表示空）
var wear: Dictionary = {}
## 角色信息
var hero_name: String = ""
var hero_level := 1
var hero_gender := 0        ## 0=男 1=女（原版 gender() 返回 bool）
var hair := 0               ## HAIR_BEGIN 起的发型序号
var name_color := Color(0.95, 0.95, 0.9)

## 悬停槽位（-1 无）
var hover_slot := -1

## 回调
var on_close: Callable = Callable()
var on_trade: Callable = Callable()
var on_add_friend: Callable = Callable()

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init(p_name: String = "MyHeroStateBoard", p_is_self: bool = true) -> void:
	widget_name = p_name
	is_self = p_is_self
	var p: Dictionary = PRESET_SELF if p_is_self else PRESET_PEER
	w = int(p["w"])
	h = int(p["h"])
	show = false
	on_draw = _draw_board


func set_canvas(c) -> void:
	_canvas_ref = c


func layout_table() -> Dictionary:
	return grid_layout_self() if is_self else grid_layout_peer()


func preset() -> Dictionary:
	return PRESET_SELF if is_self else PRESET_PEER


## 打开时填入目标角色
func inspect(p_name: String, p_level: int, p_gender: int, p_wear: Dictionary) -> void:
	hero_name = p_name
	hero_level = p_level
	hero_gender = p_gender
	wear = p_wear.duplicate()
	show = true


func close() -> void:
	show = false
	if on_close.is_valid():
		on_close.call(self)


func get_wear(slot: int) -> int:
	return int(wear.get(slot, 0))


# ==================== 绘制 ====================

func _draw_board(_w, roi) -> void:
	var canvas = _canvas_ref
	if canvas == null:
		return
	hit_areas.clear()

	var p := preset()
	var fw: int = int(p["w"])
	var fh: int = int(p["h"])

	# 框贴图（原版优先用图库，缺图时画兜底边框）
	var frame := ProgUseTex.retrieve(int(p["frame_tex"]))
	if frame != null:
		canvas.tex_draw(frame, roi.x, roi.y)
	else:
		canvas.draw_rect_c(Color(0.08, 0.07, 0.06, 0.96), roi.x, roi.y, fw, fh, true)
		canvas.draw_border(Color(0.52, 0.44, 0.28, 1.0), roi.x, roi.y, fw, fh, 2)

	var grid := layout_table()

	# 角色剪影（equip 图库：男 0x00000000 / 女 0x00000001）
	var cx: int = roi.x + int(p["char_x"])
	var cy: int = roi.y + int(p["char_y"])
	var body := _equip_tex(0x00000000 if hero_gender == 0 else 0x00000001)
	if body != null:
		canvas.tex_draw(body, cx, cy)
	else:
		# 无剪影时画一个占位人形
		canvas.draw_rect_c(Color(0.22, 0.2, 0.18, 0.5), cx - 18, cy - 60, 36, 110, true)
		canvas.draw_string_c("人物", cx - 12, cy - 20, Color(0.5, 0.46, 0.4), null, 12)

	# 角色名（原版 LabelBoard 在 m.x+142, m.y+38；他人版在面板上方区域）
	var name_x: int = roi.x + (142 if is_self else 142)
	var name_y: int = roi.y + (38 if is_self else 38)
	if not hero_name.is_empty():
		canvas.draw_string_c(hero_name, name_x, name_y, name_color, null, 14)
		canvas.draw_string_c("Lv.%d" % hero_level, name_x, name_y + 18,
			Color(0.85, 0.8, 0.5), null, 12)

	# 衣服/武器/头盔三层贴到角色位置（equip 图库，带 12 位补码偏移）
	var dress_id := get_wear(ProtocolDefRef.WLG_DRESS)
	if dress_id > 0:
		_draw_equip_layer(canvas, cx, cy, _pkg_gfx(dress_id))
	var weapon_id := get_wear(ProtocolDefRef.WLG_WEAPON)
	if weapon_id > 0:
		_draw_equip_layer(canvas, cx, cy, 0x01000000 + _pkg_gfx(weapon_id))
	var helmet_id := get_wear(ProtocolDefRef.WLG_HELMET)
	if helmet_id > 0:
		_draw_equip_layer(canvas, cx, cy, 0x01000000 + _pkg_gfx(helmet_id))

	# 各装备格物品图标（WLG_W_BEGIN..WLG_W_END 这一段原版会画）
	var w_begin := ProtocolDefRef.WLG_W_BEGIN
	var w_end := ProtocolDefRef.WLG_W_END
	for slot in range(w_begin, w_end):
		var iid := get_wear(slot)
		if iid <= 0:
			continue
		if not grid.has(slot):
			continue
		var g: Array = grid[slot]
		var tex := ItemGfxRef.icon_of_item(iid)
		if tex == null:
			continue
		var gw: int = int(g[2])
		var gh: int = int(g[3])
		var dx: int = roi.x + int(g[0]) + (gw - tex.get_width()) / 2
		# 鞋用 1，其它用 2（herostateboard.cpp:208）
		var div: int = 1 if slot == ProtocolDefRef.WLG_SHOES else 2
		var dy: int = roi.y + int(g[1]) + (gh - tex.get_height()) / div
		canvas.tex_draw(tex, dx, dy)

	# 悬停高亮：0x06000001（普通格 42x42）/ 0x06000002（鞋格 42x71），带 50% 白
	if hover_slot > 0 and grid.has(hover_slot):
		var hg: Array = grid[hover_slot]
		var htex_id: int = 0x06000002 if hover_slot == ProtocolDefRef.WLG_SHOES else 0x06000001
		var htex := ProgUseTex.retrieve(htex_id)
		var hdx: int = roi.x + int(hg[0]) + (-1)
		var hdy: int = roi.y + int(hg[1]) + (-6 if hover_slot == ProtocolDefRef.WLG_SHOES else -3)
		if htex != null:
			canvas.tex_draw(htex, hdx, hdy, Color(1, 1, 1, 0.5))
		# 悬停文字
		_draw_hover_text(canvas, roi, hover_slot, int(hg[0]), int(hg[1]), int(hg[2]), int(hg[3]))

	# 登记格子命中区（供悬停判定）
	for slot in grid.keys():
		var g2: Array = grid[slot]
		hit_areas["slot_%d" % slot] = Rect2(roi.x + int(g2[0]), roi.y + int(g2[1]),
			int(g2[2]), int(g2[3]))

	# 关闭按钮 0x0000001C（32x30）
	var close_x: int = roi.x + int(p["close_x"])
	var close_y: int = roi.y + int(p["close_y"])
	var ctex := ProgUseTex.retrieve(0x0000001C)
	if ctex != null:
		canvas.tex_draw(ctex, close_x, close_y)
		hit_areas["close"] = Rect2(close_x, close_y, ctex.get_width(), ctex.get_height())
	else:
		canvas.draw_string_c("×", close_x, close_y, Color(0.9, 0.75, 0.5), null, 18)
		hit_areas["close"] = Rect2(close_x, close_y, 20, 20)

	# 他人版独有：交易申请 / 加为好友
	if bool(p["has_peer_buttons"]):
		_draw_text_button(canvas, roi.x + 208, roi.y + 15, "交易申请", "trade")
		_draw_text_button(canvas, roi.x + 208, roi.y + 30, "加为好友", "addfriend")


func _draw_text_button(canvas, x: int, y: int, text: String, act: String) -> void:
	var tw: int = int(canvas.measure_text(text, null, 13).x)
	canvas.draw_rect_c(Color(0.2, 0.17, 0.12, 0.9), x, y, tw + 6, 16, true)
	canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), x, y, tw + 6, 16, 1)
	canvas.draw_string_c(text, x + 3, y + 1, Color(0.95, 0.9, 0.7), null, 13)
	hit_areas[act] = Rect2(x, y, tw + 6, 16)


func _draw_hover_text(canvas, roi, slot: int, gx: int, gy: int, gw: int, gh: int) -> void:
	var iid := get_wear(slot)
	var lines: Array = []
	if iid > 0:
		lines.append(ItemRecordRef.name(iid))
		var nm: String = str(SLOT_NAMES.get(slot, "装备"))
		lines.append("［%s］" % nm)
	else:
		lines.append("[%s] 空" % str(SLOT_NAMES.get(slot, "装备")))
	if lines.is_empty():
		return
	var bw := 110
	var bh: int = 6 + lines.size() * 15
	var bx: int = roi.x + gx + gw + 4
	var by: int = roi.y + gy
	canvas.draw_rect_c(Color(0, 0, 0, 0.88), bx, by, bw, bh, true)
	canvas.draw_border(Color(0.6, 0.52, 0.32, 1.0), bx, by, bw, bh, 1)
	var ly := by + 3
	for ln in lines:
		canvas.draw_string_c(str(ln), bx + 4, ly, Color(0.95, 0.92, 0.8), null, 12)
		ly += 15


## 从 equip 图库取贴图（素材在 texture/equip/，文件名 8 位 hex）
func _equip_tex(tex_id: int) -> Texture2D:
	var root := ProgUseTex.assets_root()
	if root.is_empty():
		return null
	var p := "%s/texture/equip/%08X.PNG" % [root, tex_id]
	if not FileAccess.file_exists(p):
		return null
	var img := Image.load_from_file(p)
	if img == null:
		return null
	return ImageTexture.create_from_image(img)


## 衣服/武器/头盔整层（原版从 equipDB 取，带 dx/dy 偏移；这里按 0 偏移画）
func _draw_equip_layer(canvas, cx: int, cy: int, tex_id: int) -> void:
	var t := _equip_tex(tex_id)
	if t == null:
		return
	canvas.tex_draw(t, cx, cy)


func _pkg_gfx(item_id: int) -> int:
	return ItemRecordRef.pkg_gfx_id(item_id)


# ==================== 事件 ====================

## 鼠标移动：更新悬停槽
func mouse_move(px: int, py: int) -> void:
	hover_slot = -1
	for slot in layout_table().keys():
		var g: Array = layout_table()[slot]
		var p := preset()
		var x: int = _abs_x + int(g[0])
		var y: int = _abs_y + int(g[1])
		if px >= x and px < x + int(g[2]) and py >= y and py < y + int(g[3]):
			hover_slot = slot
			return


## 面板绝对坐标（由 GameUi 每帧写入，事件判定要用）
var _abs_x := 0
var _abs_y := 0


func set_abs(x: int, y: int) -> void:
	_abs_x = x
	_abs_y = y


func handle_click(px: int, py: int) -> bool:
	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			var k := str(key)
			if k == "close":
				close()
				return true
			if k == "trade" and on_trade.is_valid():
				on_trade.call(self)
				return true
			if k == "addfriend" and on_add_friend.is_valid():
				on_add_friend.call(self)
				return true
	return false
