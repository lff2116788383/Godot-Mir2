extends Node2D
class_name FightEffects
## 战斗表现层（飘字 + 死亡淡出 + 受击闪烁）
##
## 【逐行翻译】client/src/ascendstr.hpp + ascendstr.cpp（88 行）
##            client/src/processrunnet.cpp:373-414   on_SM_DEADFADEOUT / on_SM_MISS
##            client/src/clientmonster.cpp:210-275   死亡 fadeOut
##
## ── 飘字（ascendstr.cpp）──
##   类型：ASCENDSTR_MISS=0 / RED=1 / BLUE=2 / GREEN=3
##   轨迹：ratio = tick / 3000，x += ratio*50，y -= ratio*50（右上飘）
##   透明度：255 * (1 - ratio)
##   文字：红色伤害数字用 proguse 图库的 0x03000000 系列；
##         这里用 Godot 文字绘制代替（素材缺 proguse 图库），颜色对应原版。
##
## ── 死亡淡出（clientmonster.cpp:210-275）──
##   fadeOut 每次 +10，到 255 时移除
##   bodyAlpha = 255 - fadeOut，shadowAlpha = bodyAlpha / 2

# 飘字类型（ascendstr.hpp:7-13）
const ASCENDSTR_MISS := 0
const ASCENDSTR_RED := 1
const ASCENDSTR_BLUE := 2
const ASCENDSTR_GREEN := 3

## 飘字存活时长（ascendstr.hpp:66）
const ASCEND_MS := 3000.0
## 飘动距离（ascendstr.hpp:55-62）
const ASCEND_DIST := 50.0

## 格尺寸
const GRID_X := 48
const GRID_Y := 32

## 飘字条目：{ type, value, x, y, tick }
var _ascend_list: Array = []

## 死亡淡出中的实体：{ node, fade }
var _fade_list: Array = []

## 字体
var font: Font = null
var font_size := 14


func _ready() -> void:
	z_index = 500
	# 显式启用两个处理循环，避免被上游默认值影响
	set_process(true)
	set_physics_process(true)


## 添加飘字（processrun.cpp:1726 addAscendStr）
func add_ascend_str(str_type: int, value: int, x: int, y: int) -> void:
	_ascend_list.append({
		"type": str_type, "value": value,
		"x": x, "y": y, "tick": 0.0,
	})


## 对应 on_SM_MISS（processrunnet.cpp:404）
## 位置：(x*48 + 24 - 20, y*32 - 32)
func on_miss(uid_x: int, uid_y: int) -> void:
	var nx := uid_x * GRID_X + GRID_X / 2 - 20
	var ny := uid_y * GRID_Y - GRID_Y
	add_ascend_str(ASCENDSTR_MISS, 0, nx, ny)


## 伤害数字（红）
func on_damage(uid_x: int, uid_y: int, value: int) -> void:
	add_ascend_str(ASCENDSTR_RED, value, uid_x * GRID_X, uid_y * GRID_Y - GRID_Y)


## 魔法伤害（蓝）
func on_magic_damage(uid_x: int, uid_y: int, value: int) -> void:
	add_ascend_str(ASCENDSTR_BLUE, value, uid_x * GRID_X, uid_y * GRID_Y - GRID_Y)


## 治疗（绿）
func on_heal(uid_x: int, uid_y: int, value: int) -> void:
	add_ascend_str(ASCENDSTR_GREEN, value, uid_x * GRID_X, uid_y * GRID_Y - GRID_Y)


## 开始死亡淡出（对应 deadFadeOut）
func start_fade_out(node) -> void:
	if node == null:
		return
	# 避免重复
	for e in _fade_list:
		if e["node"] == node:
			return
	_fade_list.append({"node": node, "fade": 0})


## 受击闪白
func flash_hitted(node) -> void:
	if node == null or not is_instance_valid(node):
		return
	var spr := _find_sprite(node)
	if spr == null:
		return
	spr.modulate = Color(1.6, 1.2, 1.2, 1.0)
	var tw := create_tween()
	tw.tween_property(spr, "modulate", Color.WHITE, 0.15)


func _find_sprite(node) -> Sprite2D:
	if node is Sprite2D:
		return node
	for c in node.get_children():
		if c is Sprite2D:
			return c
	return null


func _physics_process(dt: float) -> void:
	# 飘字推进
	var keep: Array = []
	for e in _ascend_list:
		e["tick"] += dt * 1000.0
		if float(e["tick"]) < ASCEND_MS:
			keep.append(e)
	_ascend_list = keep

	# 死亡淡出推进（clientmonster.cpp:227 每次 +10）
	var keep2: Array = []
	for e in _fade_list:
		var node = e["node"]
		if node == null or not is_instance_valid(node):
			continue
		var fade: int = int(e["fade"]) + 10
		if fade >= 255:
			node.queue_free()
			continue
		e["fade"] = fade
		var alpha := float(255 - fade) / 255.0
		for c in node.get_children():
			if c is Sprite2D:
				c.modulate.a = alpha
		keep2.append(e)
	_fade_list = keep2

	queue_redraw()


func _draw() -> void:
	for e in _ascend_list:
		var ratio: float = float(e["tick"]) / ASCEND_MS
		var px: int = int(e["x"]) + int(round(ratio * ASCEND_DIST))
		var py: int = int(e["y"]) - int(round(ratio * ASCEND_DIST))
		var alpha: float = 1.0 - ratio
		var txt := ""
		var col := Color.WHITE

		match int(e["type"]):
			ASCENDSTR_MISS:
				txt = "MISS"
				col = Color(1, 1, 1, alpha)
			ASCENDSTR_RED:
				var v := int(e["value"])
				txt = str(v)
				col = Color(1.0, 0.25, 0.2, alpha) if v > 0 else Color(0.6, 1.0, 0.4, alpha)
			ASCENDSTR_BLUE:
				var v2 := int(e["value"])
				txt = str(v2)
				col = Color(0.45, 0.7, 1.0, alpha)
			ASCENDSTR_GREEN:
				var v3 := int(e["value"])
				txt = "+" + str(absi(v3))
				col = Color(0.4, 1.0, 0.45, alpha)

		if txt.is_empty():
			continue
		var f: Font = font if font != null else ThemeDB.fallback_font
		# 描边加阴影，保证在复杂地形上也看得清
		draw_string(f, Vector2(px + 1, py + 1), txt, HORIZONTAL_ALIGNMENT_LEFT, -1, font_size, Color(0, 0, 0, alpha * 0.9))
		draw_string(f, Vector2(px, py), txt, HORIZONTAL_ALIGNMENT_LEFT, -1, font_size, col)


func ascend_count() -> int:
	return _ascend_list.size()


func fade_count() -> int:
	return _fade_list.size()
