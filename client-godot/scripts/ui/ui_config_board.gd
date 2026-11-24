extends UiWidget
class_name UiConfigBoard
## 设置面板
##
## 【逐行翻译】client/src/gui/runtimeconfigboard/runtimeconfigboard.hpp（1169 行）
##            common/src/protocoldef.hpp 的 RTCFG_* 运行时配置项
##
## 原版把配置项做成页签 + 滑块/复选的组合。
## 这里保留「分页 + 复选/滑块」的结构，配置项按 RTCFG 枚举组织。
##
## 对应 CMSetRuntimeConfig（客户端消息，把改动下发给服务端）

## 配置项（对应 RTCFG_*）
## 类型："bool" = 复选，"int" = 滑块
const CONFIG_ITEMS := [
	# 显示
	{"key": "平滑移动", "type": "bool", "default": true, "group": "显示"},
	{"key": "显示伤害数字", "type": "bool", "default": true, "group": "显示"},
	{"key": "显示名字", "type": "bool", "default": true, "group": "显示"},
	{"key": "显示血条", "type": "bool", "default": true, "group": "显示"},
	# 音效
	{"key": "背景音乐音量", "type": "int", "default": 60, "min": 0, "max": 100, "group": "音效"},
	{"key": "音效音量", "type": "int", "default": 80, "min": 0, "max": 100, "group": "音效"},
	# 操作
	{"key": "自动拾取", "type": "bool", "default": false, "group": "操作"},
	{"key": "点击移动", "type": "bool", "default": true, "group": "操作"},
	# 其他
	{"key": "显示帧率", "type": "bool", "default": false, "group": "其他"},
]

const BOARD_W := 380
const BOARD_H := 400
const ROW_H := 30

var cur_group := 0
var groups: Array = []
## 当前值：key -> value
var values: Dictionary = {}

## 改动回调：Callable(widget, key, value)
var on_change: Callable = Callable()

## 拖动中的滑块 key
var _drag_key: String = ""


func _init(p_name: String = "RuntimeConfigBoard") -> void:
	widget_name = p_name
	w = BOARD_W
	h = BOARD_H
	show = false
	on_event = _handle_event
	_build_groups()


func _build_groups() -> void:
	groups.clear()
	for it in CONFIG_ITEMS:
		var g := str(it.get("group", "其他"))
		if not groups.has(g):
			groups.append(g)
		values[str(it["key"])] = it.get("default")


func items_of_group(g: String) -> Array:
	var out: Array = []
	for it in CONFIG_ITEMS:
		if str(it.get("group", "")) == g:
			out.append(it)
	return out


func draw_self(canvas, my_roi) -> void:
	var t = canvas
	t.draw_rect_c(Color(0.07, 0.07, 0.1, 0.96), my_roi.x, my_roi.y, my_roi.w, my_roi.h, true)
	t.draw_border(Color(0.5, 0.45, 0.3, 1.0), my_roi.x, my_roi.y, my_roi.w, my_roi.h, 2)
	t.draw_string_c("设置", my_roi.x + 10, my_roi.y + 6, Color(0.95, 0.85, 0.5), null, 16)
	t.draw_string_c("×", my_roi.x + my_roi.w - 20, my_roi.y + 6, Color(0.9, 0.75, 0.5), null, 16)

	# 左侧分组
	var gx: int = my_roi.x + 10
	for i in range(groups.size()):
		var gy: int = my_roi.y + 36 + i * 28
		var bg := Color(0.22, 0.22, 0.28, 0.95) if i == cur_group else Color(0.13, 0.13, 0.17, 0.9)
		t.draw_rect_c(bg, gx, gy, 70, 24, true)
		t.draw_border(Color(0.38, 0.38, 0.45, 1.0), gx, gy, 70, 24, 1)
		t.draw_string_c(str(groups[i]), gx + 8, gy + 4,
			Color(0.9, 0.88, 0.82) if i == cur_group else Color(0.62, 0.62, 0.68), null, 12)

	# 右侧配置项
	var ix: int = my_roi.x + 92
	var iy0: int = my_roi.y + 40
	var cur_group_name: String = groups[cur_group] if cur_group < groups.size() else ""
	var items := items_of_group(cur_group_name)
	for i in range(items.size()):
		var it: Dictionary = items[i]
		var key := str(it["key"])
		var ry: int = iy0 + i * (ROW_H + 8)
		t.draw_string_c(key, ix, ry + 4, Color(0.86, 0.84, 0.78), null, 13)

		if str(it["type"]) == "bool":
			var v := bool(values.get(key, false))
			var bx: int = ix + 180
			t.draw_rect_c(Color(0.16, 0.16, 0.2, 0.9), bx, ry, 16, 16, true)
			t.draw_border(Color(0.42, 0.42, 0.48, 1.0), bx, ry, 16, 16, 1)
			if v:
				# 勾
				t.draw_line_c(Color(0.5, 0.95, 0.5), bx + 3, ry + 8, bx + 7, ry + 12, 2)
				t.draw_line_c(Color(0.5, 0.95, 0.5), bx + 7, ry + 12, bx + 13, ry + 4, 2)
		else:
			# 滑块
			var val := int(values.get(key, 0))
			var vmin := int(it.get("min", 0))
			var vmax := int(it.get("max", 100))
			var sx: int = ix + 140
			var sw := 120
			t.draw_rect_c(Color(0.15, 0.15, 0.19, 0.9), sx, ry + 6, sw, 6, true)
			var ratio := 0.0 if vmax <= vmin else float(val - vmin) / float(vmax - vmin)
			t.draw_rect_c(Color(0.5, 0.5, 0.58, 0.95), sx, ry + 6, int(sw * ratio), 6, true)
			var knob_x: int = sx + int((sw - 10) * ratio)
			t.draw_rect_c(Color(0.85, 0.82, 0.72), knob_x, ry + 2, 10, 14, true)
			t.draw_string_c(str(val), sx + sw + 8, ry + 3, Color(0.82, 0.8, 0.75), null, 12)


func _handle_event(self_widget, event: InputEvent, valid: bool, my_roi) -> bool:
	if not show:
		return false

	# 拖动滑块
	if _drag_key != "" and event is InputEventMouseMotion:
		_apply_slider(_drag_key, event.position, my_roi)
		return true
	if _drag_key != "" and event is InputEventMouseButton and not event.pressed:
		_drag_key = ""
		return true

	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_LEFT and event.pressed:
		var mpos: Vector2 = event.position
		if not my_roi.contains(int(mpos.x), int(mpos.y)):
			return false
		if mpos.x > my_roi.x + my_roi.w - 28 and mpos.y < my_roi.y + 28:
			show = false
			return true

		# 分组
		if mpos.x >= my_roi.x + 10 and mpos.x < my_roi.x + 80:
			var idx: int = int((mpos.y - (my_roi.y + 36)) / 28)
			if idx >= 0 and idx < groups.size():
				cur_group = idx
				return true

		# 配置项
		var ix: int = my_roi.x + 92
		var iy0: int = my_roi.y + 40
		var cur_group_name: String = groups[cur_group] if cur_group < groups.size() else ""
		var items := items_of_group(cur_group_name)
		for i in range(items.size()):
			var it: Dictionary = items[i]
			var ry: int = iy0 + i * (ROW_H + 8)
			if mpos.y < ry or mpos.y > ry + ROW_H:
				continue
			var key := str(it["key"])
			if str(it["type"]) == "bool":
				if mpos.x >= ix + 180 and mpos.x < ix + 200:
					var nv := not bool(values.get(key, false))
					values[key] = nv
					if on_change.is_valid():
						on_change.call(self, key, nv)
					return true
			else:
				var sx: int = ix + 140
				if mpos.x >= sx - 8 and mpos.x < sx + 132:
					_drag_key = key
					_apply_slider(key, mpos, my_roi)
					return true
		return true
	return false


func _apply_slider(key: String, mpos: Vector2, my_roi) -> void:
	var ix: int = my_roi.x + 92
	var sx: int = ix + 140
	var sw := 120
	var it: Dictionary = {}
	for c in CONFIG_ITEMS:
		if str(c["key"]) == key:
			it = c
			break
	if it.is_empty():
		return
	var vmin := int(it.get("min", 0))
	var vmax := int(it.get("max", 100))
	var t := clampf((mpos.x - sx) / float(sw), 0.0, 1.0)
	var nv: int = vmin + int(round(float(vmax - vmin) * t))
	if int(values.get(key, -1)) == nv:
		return
	values[key] = nv
	if on_change.is_valid():
		on_change.call(self, key, nv)
