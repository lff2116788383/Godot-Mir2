extends UiWidget
class_name UiControlBoard
## 底部控制栏（含日志窗口、命令行、HP/MP 条、右侧按钮矩阵、标题等级条）
##
## 【逐行翻译】client/src/gui/controlboard/
##   controlboard.hpp/.cpp   外壳：三段布局（左 178 + 中可伸缩 + 右 166）+ 标题条 21
##   cbleft.hpp/.cpp         左段 178x133：背景、HP、MP、等级条、负重条、快捷栏按钮、关闭/最小化、地图坐标
##   cbmiddle.hpp/.cpp       中段 131 高：日志窗口 344x84 + 命令行 344x15 + 头像 + 滚动条 + 模式切换
##   cbmiddleexpand.hpp/.cpp 中段展开态：日志 432x228 + 命令行 350x40 + 表情/静音按钮
##   cbright.hpp/.cpp        右段 166x133：三个圆形按钮 + 九宫格按钮 + AC/DC 面板
##   cbtitle.hpp/.cpp        标题条 127x41：弧形动画 + 等级按钮（双击切换最小化）
##   cbface.hpp/.cpp         头像 82x97：立绘 + 血条 3px
##
## ── 三段几何（controlboard.hpp 的 ASCII 图，逐值核对过）──
##   整宽 = 178(左) + 50 + 110 + 127 + 50 + 119(中) + 166(右) = 800
##   左段 w = 178，右段 w = 166，中段 w = W - 178 - 166
##   中段高 = 131（= 00000013.PNG 高），标题条高 = 21（UP_HEIGHT）
##   整栏高 = 131 + 21 = 152（最小化时 21 + 3 = 24）
##
## ── 贴图（proguse 图库，尺寸已实测吻合）──
##   0x00000012 800x133  左右段背景（左段裁左 178，右段裁右 166）
##   0x00000013 456x131  中段背景（横向拉伸）
##   0x00000022 127x41   标题条背景
##   0x00000018 36x88    HP 条（从下往上长）
##   0x00000019 36x88    MP 条
##   0x000000A0 3x89     等级条 / 负重条
##   按钮全部走 texAniOpt（0x0000001C 关闭、0x0000001E 退出、0x00000020 最小化…）
##
## ── 坐标注意（曾踩过的坑）──
##   原版 Widget 的 x/y 相对父控件，绘制时逐层累加父的绝对坐标。
##   这里用 Godot 的 position 相对定位，但 _draw 拿到的是「绝对 ROI」，
##   所以子控件绘制一律用 roi.x/roi.y 起点，不要再加 position（会加两次）。

# ==================== 几何常量（controlboard.hpp / cb*.hpp） ====================

const LEFT_W := 178          ## CBLeft w
const RIGHT_W := 166         ## CBRight w
const MIDDLE_H := 131        ## CBMiddle CB_MIDDLE_TEX_HEIGHT
const TITLE_H := 21          ## CBTitle UP_HEIGHT
const TITLE_H_EXTRA := 3     ## CBTitle UP_HEIGHT_EXTRA
const TOTAL_H := MIDDLE_H + TITLE_H   ## 152

## 中段（CBMiddle）内部窗口
const LOG_WIN_X := 7
const LOG_WIN_Y := 15
const LOG_WIN_W_ORIG := 344
const LOG_WIN_H := 84
const CMD_WIN_X := 7
const CMD_WIN_Y := 106
const CMD_WIN_W_ORIG := 344
const CMD_WIN_H := 15

## 中段展开（CBMiddleExpand）内部窗口
const EXP_LOG_W_ORIG := 432
const EXP_LOG_H_ORIG := 228
const EXP_LOG_X := 7
const EXP_LOG_Y := 15
const EXP_CMD_W_ORIG := 350
const EXP_CMD_H := 40
const EXP_CMD_X := 6
const EXP_CMD_Y := 248

## 左段（CBLeft）内部坐标
const HP_X := 33
const HP_Y := 95
const MP_X := 73
const MP_Y := 95
const LEVELBAR_X := 153
const LEVELBAR_Y := 115
const INVBAR_X := 166
const INVBAR_Y := 115
const MAPGLOC_X := 73
const MAPGLOC_Y := 117
const MAPGLOC_MAX_W := 120
const MAPGLOC_PIXEL_SPEED := 20

## 右段（CBRight）按钮坐标
## 三个圆形按钮用 onOffX/onOffY=1、onRadius=10 的 off-center 绘制
const ROUND_BTN := [
	{"x": 4, "y": 6, "tex": 0x00000042, "act": "stall"},      ## 交易/摆摊
	{"x": 4, "y": 40, "tex": 0x00000043, "act": "minimap"},   ## 小地图
	{"x": 4, "y": 75, "tex": 0x00000044, "act": "magickey"},  ## 魔法快捷键显示
]
## 九宫格按钮（texAniOpt 决定贴图）
const GRID_BTNS := [
	{"x": 48, "y": 33, "tex": 0x00000030, "act": "inventory", "name": "背包"},
	{"x": 77, "y": 31, "tex": 0x00000032, "act": "herostate", "name": "人物"},
	{"x": 105, "y": 33, "tex": 0x00000034, "act": "skill", "name": "技能"},
	{"x": 40, "y": 11, "tex": 0x00000036, "act": "guild", "name": "行会"},
	{"x": 72, "y": 8, "tex": 0x00000038, "act": "team", "name": "队伍"},
	{"x": 108, "y": 11, "tex": 0x0000003A, "act": "quest", "name": "任务"},
	{"x": 40, "y": 61, "tex": 0x0000003C, "act": "horse", "name": "马匹"},
	{"x": 72, "y": 72, "tex": 0x0000003E, "act": "config", "name": "设置"},
	{"x": 108, "y": 61, "tex": 0x00000040, "act": "friendchat", "name": "好友"},
]

## 日志类型（controlboard.hpp:CBLOG_*）
const CBLOG_DEF := 0
const CBLOG_SYS := 1
const CBLOG_DBG := 2
const CBLOG_ERR := 3
## 日志保留上限（controlboard.cpp:maxLogParCount）
const MAX_LOG_PAR := 1000

## 模式
const MODE_NORMAL := 0
const MODE_EXPAND := 1
const MODE_MINIMIZE := 2

# ==================== 状态 ====================

## 当前模式
var mode: int = MODE_NORMAL
## 日志行：{ "text": String, "type": int }
var log_lines: Array = []
## 命令输入
var cmd_text: String = ""
## 日志滚动比例 0..1（1 = 滚到最底，最新）
var log_scroll: float = 1.0
## 命令行横向裁剪偏移（光标跟随时移动）
var cmd_crop_x: int = 0
## 地图坐标滚动累计时间（原版 m_mapGLocAccuTime）
var _maploc_acc: float = 0.0

## 视口宽（由外部每帧同步）
var vw := 1280

## 引用（由外部注入）
var player = null            ## 主角（读 hp/mp/level/负重/坐标）
var map_id := 0              ## 当前地图 id（查 MapRecord 名字）
var map_gx := 0
var map_gy := 0

## 回调
var on_action: Callable = Callable()      ## Callable(action_name)
var on_command: Callable = Callable()     ## Callable(text) —— 回车提交
var on_mode_change: Callable = Callable() ## Callable(mode)


func _init(p_name: String = "ControlBoard") -> void:
	widget_name = p_name
	w = 800
	h = TOTAL_H
	on_draw = _draw_control_board
	on_update = _update_control


# ==================== 尺寸 / 布局 ====================

## 整栏高度（controlboard.cpp:h 的三个分支）
func bar_height() -> int:
	match mode:
		MODE_MINIMIZE:
			return TITLE_H + TITLE_H_EXTRA
		MODE_EXPAND:
			return EXP_LOG_H_ORIG + TITLE_H + 20   ## 展开态：228 + 21 + 底部余量
		_:
			return TOTAL_H


## 中段宽度 = 整宽 - 左 - 右
func middle_width() -> int:
	return maxi(0, vw - LEFT_W - RIGHT_W)


## 中段高度（展开态更高）
func middle_height() -> int:
	return EXP_LOG_H_ORIG + 20 if mode == MODE_EXPAND else MIDDLE_H


## 日志窗口宽度（cbmiddle.hpp:getLogWindowWidth）
## = 中段宽 - 背景满宽 + 344
func log_window_width() -> int:
	var full_w := 456   ## 0x00000013 宽
	return middle_width() - full_w + LOG_WIN_W_ORIG


func cmd_window_width() -> int:
	var full_w := 456
	return middle_width() - full_w + CMD_WIN_W_ORIG


## 标题条 x（controlboard.cpp：leftW + (fullW - leftW - rightW) / 2）
func title_x() -> int:
	return LEFT_W + (vw - LEFT_W - RIGHT_W) / 2


## 标题条 y（贴栏顶）
func title_y() -> int:
	return 0


## 中段 y（标题条下）
func middle_y() -> int:
	return TITLE_H


# ==================== 数据：进度比例 ====================

func _hero_hp_ratio() -> float:
	if player == null:
		return 1.0
	if "hp_ratio" in player:
		return clampf(float(player.hp_ratio), 0.0, 1.0)
	return 1.0


func _hero_mp_ratio() -> float:
	if player == null:
		return 1.0
	if "mp_ratio" in player:
		return clampf(float(player.mp_ratio), 0.0, 1.0)
	return 1.0


func _hero_level_ratio() -> float:
	if player == null:
		return 0.0
	if "level_ratio" in player:
		return clampf(float(player.level_ratio), 0.0, 1.0)
	return 0.0


## 负重比例（原版 getInventoryRatio）
func _hero_inv_ratio() -> float:
	if player == null:
		return 0.0
	if "inventory_ratio" in player:
		return clampf(float(player.inventory_ratio), 0.0, 1.0)
	return 0.0


func _hero_level() -> int:
	if player == null:
		return 1
	if "level" in player:
		return int(player.level)
	return 1


## 地图坐标串（cbleft.cpp:getMapGLocStr）
## 格式 "%s: %d %d"，地图名取 '_' 之前的部分
func map_gloc_str() -> String:
	var base := ""
	if map_id > 0:
		var mr = _map_record()
		if mr != null:
			base = str(mr.get("name", ""))
			var us := base.find("_")
			if us >= 0:
				base = base.substr(0, us)
	if base.is_empty():
		return ""
	if player != null:
		return "%s: %d %d" % [base, map_gx, map_gy]
	return base


func _map_record():
	var MapRecordRef = load("res://scripts/item/map_record.gd")
	if MapRecordRef == null:
		return null
	if MapRecordRef.has_method("find"):
		return MapRecordRef.find(map_id)
	return null


# ==================== 更新 ====================

func _update_control(_self, dt: float) -> void:
	_maploc_acc += dt


# ==================== 绘制入口 ====================

func _draw_control_board(self_widget, roi) -> void:
	var canvas = _canvas_of(self_widget)
	if canvas == null:
		return

	var bh := bar_height()
	# 整栏贴在 roi 底部（原版 dir=DIR_DOWNLEFT，y = H - 1）
	var top: int = roi.y + roi.h - bh

	_draw_left(canvas, roi.x, top + (0 if mode == MODE_MINIMIZE else TITLE_H))
	if mode != MODE_MINIMIZE:
		_draw_middle(canvas, roi.x + LEFT_W, top + TITLE_H)
		_draw_right(canvas, roi.x + vw - RIGHT_W, top + TITLE_H)
	_draw_title(canvas, roi.x + title_x(), top)


## 从任意控件找到绘制器（GameUi 注入）
var _canvas_ref = null


func set_canvas(c) -> void:
	_canvas_ref = c


func _canvas_of(_w):
	return _canvas_ref


# ==================== 左段（CBLeft）====================

func _draw_left(canvas, bx: int, by: int) -> void:
	var w := LEFT_W
	var h := MIDDLE_H

	# 背景：0x00000012 是 800 宽整条，左段裁左边 178
	var bg := ProgUseTex.retrieve(0x00000012)
	if bg != null:
		ProgUseTex.blit_crop(canvas, bg, Rect2(0, 0, w, h), bx, by, w, h)
	else:
		canvas.draw_rect_c(Color(0.12, 0.1, 0.08, 0.96), bx, by, w, h, true)
		canvas.draw_border(Color(0.45, 0.38, 0.24, 1.0), bx, by, w, h, 1)

	# HP 条 36x88，从下往上长（dir=DIR_DOWNLEFT）
	_draw_vbar(canvas, 0x00000018, bx + HP_X, by + HP_Y, _hero_hp_ratio(),
		Color(0.85, 0.1, 0.1, 0.9))
	# MP 条
	_draw_vbar(canvas, 0x00000019, bx + MP_X, by + MP_Y, _hero_mp_ratio(),
		Color(0.15, 0.3, 0.9, 0.9))
	# 等级条 3x89（dir=DIR_DOWN）
	_draw_vbar(canvas, 0x000000A0, bx + LEVELBAR_X, by + LEVELBAR_Y, _hero_level_ratio(),
		Color(0.95, 0.8, 0.2, 0.9))
	# 负重条 3x89
	_draw_vbar(canvas, 0x000000A0, bx + INVBAR_X, by + INVBAR_Y, _hero_inv_ratio(),
		Color(0.3, 0.85, 0.4, 0.9))

	# 快捷栏按钮 0x0B000000（24x16）在 148,2
	_draw_tex_button(canvas, 0x0B000000, bx + 148, by + 2, LEFT_W, MIDDLE_H, "quickaccess")

	# 关闭按钮 0x0000001E（24x24）在 8,72
	_draw_tex_button(canvas, 0x0000001E, bx + 8, by + 72, LEFT_W, MIDDLE_H, "close")
	# 最小化按钮 0x00000020 在 109,72
	_draw_tex_button(canvas, 0x00000020, bx + 109, by + 72, LEFT_W, MIDDLE_H, "minimize")

	# 地图坐标（10 号字 15pt），超宽横向滚动（m_mapGLocMaxWidth=120, speed=20px/s）
	var s := map_gloc_str()
	if not s.is_empty():
		var full_w: int = int(canvas.measure_text(s, null, 15).x)
		var crop_w: int = mini(full_w, MAPGLOC_MAX_W)
		var off := 0
		if full_w > MAPGLOC_MAX_W:
			off = int(MAPGLOC_PIXEL_SPEED * _maploc_acc) % maxi(1, full_w - MAPGLOC_MAX_W)
		# ROI 裁剪交给 draw_* 的 clip_roi；这里手工控制可见段
		canvas.draw_string_c(s.substr(0, 24), bx + MAPGLOC_X - off, by + MAPGLOC_Y,
			Color(0.95, 0.92, 0.7), null, 15)
		if crop_w <= 0:
			pass


## 竖向进度条（贴图从底往上长）
func _draw_vbar(canvas, tex_id: int, x: int, y: int, ratio: float, fallback_color: Color) -> void:
	var tex := ProgUseTex.retrieve(tex_id)
	if tex == null:
		# 无贴图时画纯色条（尺寸取原版实测值：HP/MP 36x88，等级/负重 3x89）
		var bw: int = 36 if tex_id != 0x000000A0 else 3
		var bh: int = 88 if tex_id != 0x000000A0 else 89
		canvas.draw_rect_c(Color(0, 0, 0, 0.5), x, y - bh, bw, bh, true)
		var fill_h: int = int(bh * clampf(ratio, 0.0, 1.0))
		if fill_h > 0:
			canvas.draw_rect_c(fallback_color, x, y - fill_h, bw, fill_h, true)
		return

	var tw := tex.get_width()
	var th := tex.get_height()
	var fill_h2: int = int(round(th * clampf(ratio, 0.0, 1.0)))
	if fill_h2 <= 0:
		return
	# 从整图底部裁 fill_h2 高度，画在「底线 y」上方
	var src := Rect2(0, th - fill_h2, tw, fill_h2)
	ProgUseTex.blit_crop(canvas, tex, src, x, y - fill_h2, tw, fill_h2)


## 贴图按钮（带 hover 检测）
func _draw_tex_button(canvas, tex_id: int, x: int, y: int, _bw: int, _bh: int, act: String) -> void:
	var tex := ProgUseTex.retrieve(tex_id)
	if tex == null:
		canvas.draw_rect_c(Color(0.2, 0.2, 0.25, 0.9), x, y, 24, 24, true)
		return
	var hover := _mouse_in(x, y, tex.get_width(), tex.get_height())
	if hover:
		canvas.tex_draw(tex, x, y, Color(1.15, 1.15, 1.15))
	else:
		canvas.tex_draw(tex, x, y)
	btn_hit_areas[act] = Rect2(x, y, tex.get_width(), tex.get_height())


## 按钮命中区（绘制时顺便登记，事件里查）
var btn_hit_areas: Dictionary = {}


func _mouse_in(x: int, y: int, w: int, h: int) -> bool:
	var mp := _last_mouse
	return mp.x >= x and mp.x < x + w and mp.y >= y and mp.y < y + h


## 鼠标位置由 GameUi 每帧写入（避免每帧调 Input 查询）
var _last_mouse := Vector2(-1, -1)


func set_mouse(px: int, py: int) -> void:
	_last_mouse = Vector2(px, py)


# ==================== 中段（CBMiddle）====================

func _draw_middle(canvas, bx: int, by: int) -> void:
	var mw := middle_width()
	var mh := middle_height()
	if mw <= 0:
		return

	# 背景：0x00000013 456x131，横向拉伸到 mw
	# 先铺不透明黑底 —— 原版 cbmiddle.cpp 的 m_bg 就是 fillRectangle(A_SHF(0xFF))，
	# 否则背景图中间是半透的，会透出地图（第一版截图就是这个毛病）
	canvas.draw_rect_c(Color(0, 0, 0, 1), bx, by, mw, mh, true)
	var bg := ProgUseTex.retrieve(0x00000013)
	if bg != null:
		canvas.tex_draw_region(bg, Rect2(0, 0, 456, MIDDLE_H),
			bx, by, mw, MIDDLE_H)
		# 展开态下把背景竖向复制补足
		if mh > MIDDLE_H:
			var extra: int = mh - MIDDLE_H
			canvas.tex_draw_region(bg, Rect2(0, MIDDLE_H - extra, 456, extra),
				bx, by + MIDDLE_H, mw, extra)
	else:
		canvas.draw_rect_c(Color(0.1, 0.09, 0.07, 0.96), bx, by, mw, mh, true)
		canvas.draw_border(Color(0.4, 0.34, 0.22, 1.0), bx, by, mw, mh, 1)

	if mode == MODE_EXPAND:
		_draw_log_window(canvas, bx + EXP_LOG_X, by + EXP_LOG_Y, middle_width() - 20, EXP_LOG_H_ORIG)
		_draw_cmd_line(canvas, bx + EXP_CMD_X, by + EXP_CMD_Y, middle_width() - 20, EXP_CMD_H)
		# 表情 / 静音按钮（展开态才有）
		canvas.draw_rect_c(Color(0.2, 0.2, 0.24, 0.9), bx + mw - 44, by + 4, 18, 18, true)
		canvas.draw_string_c("☺", bx + mw - 41, by + 4, Color(0.9, 0.85, 0.6), null, 14)
		canvas.draw_rect_c(Color(0.2, 0.2, 0.24, 0.9), bx + mw - 22, by + 4, 18, 18, true)
		canvas.draw_string_c("♪", bx + mw - 19, by + 4, Color(0.9, 0.85, 0.6), null, 14)
	else:
		_draw_log_window(canvas, bx + LOG_WIN_X, by + LOG_WIN_Y,
			log_window_width(), LOG_WIN_H)
		_draw_cmd_line(canvas, bx + CMD_WIN_X, by + CMD_WIN_Y,
			cmd_window_width(), CMD_WIN_H)

	# 头像 82x97（DIR_DOWNRIGHT：相对中段右下角内缩 19,20）
	_draw_face(canvas, bx + mw - 19 - 82, by + mh - 20 - 97)

	# 滚动条：5 宽，在 w-10, y=40, h=60（普通）/ 更长（展开）
	if mode == MODE_EXPAND:
		_draw_scrollbar(canvas, bx + mw - 10, by + 40, 5, mh - 60)
	else:
		_draw_scrollbar(canvas, bx + mw - 10, by + 40, 5, 60)

	# 模式切换按钮 0x00000028（13x17）在 w-15, 3
	_draw_tex_button(canvas, 0x00000028, bx + mw - 15, by + 3, mw, mh, "switchmode")


## 日志窗口：按 log_scroll 决定可见段
func _draw_log_window(canvas, x: int, y: int, w: int, h: int) -> void:
	canvas.draw_rect_c(Color(0, 0, 0, 0.45), x, y, w, h, true)
	var line_h := 16
	var max_lines: int = maxi(1, h / line_h)
	var total := log_lines.size()
	# 滚动：1.0 = 最底（最新），0.0 = 最顶
	var start: int = 0
	if total > max_lines:
		var max_start: int = total - max_lines
		start = int(round((1.0 - clampf(log_scroll, 0.0, 1.0)) * max_start))
		start = clampi(start, 0, max_start)
	var visible := log_lines.slice(start, mini(start + max_lines, total))
	var ly := y + 1
	for ln in visible:
		var txt := str(ln.get("text", ""))
		var lt: int = int(ln.get("type", CBLOG_DEF))
		# 日志底色（controlboard.cpp:addLog 的 par bgcolor）
		match lt:
			CBLOG_SYS:
				canvas.draw_rect_c(Color(0, 0.5, 0, 0.35), x, ly, w, line_h, true)
			CBLOG_DBG:
				canvas.draw_rect_c(Color(0, 0, 1, 0.35), x, ly, w, line_h, true)
			CBLOG_ERR:
				canvas.draw_rect_c(Color(1, 0, 0, 0.35), x, ly, w, line_h, true)
		canvas.draw_string_c(txt.substr(0, 40), x + 2, ly + 1,
			Color(0.93, 0.92, 0.86), null, 13)
		ly += line_h


## 命令行：显示 cmd_text + 光标
func _draw_cmd_line(canvas, x: int, y: int, w: int, h: int) -> void:
	canvas.draw_rect_c(Color(0, 0, 0, 0.55), x, y, w, h, true)
	var shown := cmd_text
	if shown.is_empty():
		canvas.draw_string_c("输入聊天内容，回车发送（! 广播 / @ 命令 / $ lua / !~ 行会）",
			x + 2, y + 1, Color(0.5, 0.5, 0.5, 0.9), null, 13)
	else:
		canvas.draw_string_c(shown.substr(0, 48), x + 2 - cmd_crop_x, y + 1,
			Color(0.95, 0.95, 0.9), null, 13)
	# 光标
	var tw: int = int(canvas.measure_text(shown, null, 13).x)
	canvas.draw_rect_c(Color(1, 1, 1, 0.85), x + 2 + tw - cmd_crop_x, y + 1, 1, h - 3, true)


## 头像（CBFace）：82x97，血条 3px 在顶部
##
## 贴图 id 规则（hero.hpp:324 faceGfxID）：
##   0x02000000 + jobGfxIndex * 2 + (gender ? 0 : 1)
## 实测 proguse 里 02000000..02000005 共 6 张，全是 84x94，正好 3 职业 × 2 性别。
## 原版 m_face 把 84 裁成 82（右边留 2px 空白），高度不减。
func _draw_face(canvas, x: int, y: int) -> void:
	var fw := 82
	var fh := 97
	var bar_h := 3

	# 优先按职业/性别取，取不到再用 0x010007CF 兜底（那张是问号占位图）
	var job: int = 0
	var gender: int = 0
	if player != null:
		if "job" in player:
			job = int(player.job)
		if "gender" in player:
			gender = int(player.gender)
	var face_id := 0x02000000 + job * 2 + (0 if gender != 0 else 1)
	var face := ProgUseTex.retrieve(face_id)
	if face == null:
		face = ProgUseTex.retrieve(0x000007CF)
	if face == null:
		face = ProgUseTex.retrieve(0x00010000)

	if face != null:
		# 原版 crop 到 w-2 宽（84 -> 82）
		var sw: int = mini(fw, face.get_width())
		var sh: int = face.get_height()
		canvas.tex_draw_region(face, Rect2(0, 0, sw, sh),
			x, y + bar_h, fw, sh)
	else:
		canvas.draw_rect_c(Color(0.18, 0.16, 0.14, 0.95), x, y + bar_h, fw, fh - bar_h, true)
		canvas.draw_border(Color(0.4, 0.34, 0.24, 1.0), x, y + bar_h, fw, fh - bar_h, 1)
		canvas.draw_string_c("头像", x + 24, y + 40, Color(0.6, 0.55, 0.45), null, 13)

	# 血条：宽 = 血量比例 * fw，高 3（原版 m_hpBar 用 RED_A255 填充）
	var hp_w: int = int(round(_hero_hp_ratio() * fw))
	canvas.draw_rect_c(Color(0.15, 0.05, 0.05, 0.9), x, y, fw, bar_h, true)
	if hp_w > 0:
		canvas.draw_rect_c(Color(1, 0, 0, 1), x, y, hp_w, bar_h, true)


## 滚动条（TexSlider，index=2 走竖向）
func _draw_scrollbar(canvas, x: int, y: int, w: int, h: int) -> void:
	canvas.draw_rect_c(Color(0.08, 0.08, 0.1, 0.8), x, y, w, h, true)
	var thumb_h := 24
	var ty: int = y + int(round((1.0 - clampf(log_scroll, 0.0, 1.0)) * maxi(0, h - thumb_h)))
	canvas.draw_rect_c(Color(0.55, 0.5, 0.38, 0.95), x, ty, w, thumb_h, true)
	canvas.draw_border(Color(0.75, 0.68, 0.45, 1.0), x, ty, w, thumb_h, 1)


# ==================== 右段（CBRight）====================

func _draw_right(canvas, bx: int, by: int) -> void:
	var w := RIGHT_W
	var h := MIDDLE_H

	# 背景：0x00000012 右段（源 x 从 800-166 起）
	var bg := ProgUseTex.retrieve(0x00000012)
	if bg != null:
		ProgUseTex.blit_crop(canvas, bg, Rect2(bg.get_width() - w, 0, w, h),
			bx, by, w, h)
	else:
		canvas.draw_rect_c(Color(0.12, 0.1, 0.08, 0.96), bx, by, w, h, true)
		canvas.draw_border(Color(0.45, 0.38, 0.24, 1.0), bx, by, w, h, 1)

	# 三个圆形按钮（AlphaOnButton：半透明 + 悬停偏 1px）
	for b in ROUND_BTN:
		var tex := ProgUseTex.retrieve(int(b["tex"]))
		if tex != null:
			canvas.tex_draw(tex, bx + int(b["x"]), by + int(b["y"]), Color(1, 1, 1, 0.8))
			btn_hit_areas[str(b["act"])] = Rect2(bx + int(b["x"]), by + int(b["y"]),
				tex.get_width(), tex.get_height())

	# 九宫格按钮
	for b in GRID_BTNS:
		var gx: int = bx + int(b["x"])
		var gy: int = by + int(b["y"])
		var tex2 := ProgUseTex.retrieve(int(b["tex"]))
		if tex2 != null:
			canvas.tex_draw(tex2, gx, gy)
			btn_hit_areas[str(b["act"])] = Rect2(gx, gy, tex2.get_width(), tex2.get_height())
		else:
			# 无贴图时画带文字的小方块（保证可点）
			var bw := 32
			var bh := 28
			canvas.draw_rect_c(Color(0.2, 0.18, 0.14, 0.95), gx, gy, bw, bh, true)
			canvas.draw_border(Color(0.45, 0.38, 0.24, 1.0), gx, gy, bw, bh, 1)
			canvas.draw_string_c(str(b["name"]), gx + 2, gy + 7,
				Color(0.9, 0.85, 0.65), null, 12)
			btn_hit_areas[str(b["act"])] = Rect2(gx, gy, bw, bh)

	# AC/DC 面板（ACButton：两行数值，点击切换 AC/MA 与 DC/MC）
	_draw_ac_panel(canvas, bx + 1, by + 105, "AC", "MA")
	_draw_ac_panel(canvas, bx + 84, by + 105, "DC", "MC")


func _draw_ac_panel(canvas, x: int, y: int, n0: String, n1: String) -> void:
	var pw := 80
	var ph := 24
	canvas.draw_rect_c(Color(0, 0, 0, 0.55), x, y, pw, ph, true)
	canvas.draw_border(Color(0.4, 0.34, 0.22, 1.0), x, y, pw, ph, 1)
	canvas.draw_string_c("%s / %s" % [n0, n1], x + 3, y + 2, Color(0.85, 0.82, 0.6), null, 12)
	# 数值行（暂用占位，属性面板接好后填真实值）
	canvas.draw_string_c("0 - 0", x + 3, y + 12, Color(0.95, 0.92, 0.8), null, 12)


# ==================== 标题条（CBTitle）====================

func _draw_title(canvas, bx: int, by: int) -> void:
	# 背景 127x41（dir=DIR_UP 居中，所以左上角要左移一半）
	var bg := ProgUseTex.retrieve(0x00000022)
	var tw := 127
	var th := 41
	if bg != null:
		tw = bg.get_width()
		th = bg.get_height()

	var x: int = bx - tw / 2
	var y: int = by

	if bg != null:
		canvas.tex_draw(bg, x, y)
	else:
		canvas.draw_rect_c(Color(0.14, 0.12, 0.09, 0.95), x, y, tw, th, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), x, y, tw, th, 1)

	# 弧光动画 0x04000000（36x20）在 46,8，fps=1 交叉淡入
	var arc := ProgUseTex.retrieve(0x04000000)
	if arc != null:
		canvas.tex_draw(arc, x + 46, y + 8, Color(1, 1, 1, 0.9))

	# 等级按钮在 63,25（双击切最小化）
	var lv := _hero_level()
	canvas.draw_string_c("Lv.%d" % lv, x + 63, y + 25, Color(0.98, 0.9, 0.55), null, 14)
	btn_hit_areas["title_level"] = Rect2(x + 63, y + 25, 44, 16)
	btn_hit_areas["title_bar"] = Rect2(x, y, tw, th)


# ==================== 事件 ====================

## 由 GameUi 转发鼠标点击
func handle_click(px: int, py: int) -> bool:
	for act in btn_hit_areas.keys():
		var r: Rect2 = btn_hit_areas[act]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			_on_button(str(act))
			return true
	return false


func _on_button(act: String) -> void:
	match act:
		"close":
			# 原版弹询问框：退出游戏 / 离线挂机
			_emit_action("request_exit")
		"minimize":
			_emit_action("request_logout")
		"quickaccess":
			_emit_action("toggle_quickaccess")
		"switchmode":
			if mode == MODE_EXPAND:
				mode = MODE_NORMAL
			else:
				mode = MODE_EXPAND
			if on_mode_change.is_valid():
				on_mode_change.call(mode)
			_emit_action("mode_changed")
		"title_level", "title_bar":
			# 双击标题条切最小化
			mode = MODE_NORMAL if mode == MODE_MINIMIZE else MODE_MINIMIZE
			if on_mode_change.is_valid():
				on_mode_change.call(mode)
			_emit_action("mode_changed")
		_:
			_emit_action(act)


func _emit_action(act: String) -> void:
	if on_action.is_valid():
		on_action.call(act)


## 提交命令（controlboard.cpp:onInputDone 的分发规则）
##   !~xxx  行会聊天
##   !xxx   全服广播
##   @xxx   用户命令
##   $xxx   lua 命令（超管）
##   其它   普通说话
func submit_command() -> String:
	var full: String = cmd_text.strip_edges()
	cmd_text = ""
	cmd_crop_x = 0
	log_scroll = 1.0
	if full.is_empty():
		return ""
	if on_command.is_valid():
		on_command.call(full)
	return full


## 命令类型判定（供测试与上层分发）
static func command_kind(text: String) -> String:
	var s: String = text.strip_edges()
	if s.is_empty():
		return "empty"
	if s.begins_with("!~"):
		return "guild"
	if s.begins_with("!"):
		return "broadcast"
	if s.begins_with("@"):
		return "user"
	if s.begins_with("$"):
		return "lua"
	return "talk"


## 加一条日志（controlboard.cpp:addLog）
func add_log(text: String, log_type: int = CBLOG_DEF) -> void:
	log_lines.append({"text": text, "type": log_type})
	# 超出上限从头砍（onLogAdded: removePar(0, count - 1000)）
	if log_lines.size() > MAX_LOG_PAR:
		log_lines = log_lines.slice(log_lines.size() - MAX_LOG_PAR)
	# 加日志后自动滚到最底
	log_scroll = 1.0


func clear_log() -> void:
	log_lines.clear()


## 滚动日志（delta > 0 向新，< 0 向旧）
func scroll_log(delta: float) -> void:
	log_scroll = clampf(log_scroll + delta, 0.0, 1.0)
