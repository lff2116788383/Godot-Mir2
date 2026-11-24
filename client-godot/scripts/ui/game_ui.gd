extends CanvasLayer
class_name GameUi
## 游戏主界面
##
## 【逐行翻译】client/src/guimanager.hpp + guimanager.cpp（392 行）
## 对照：
##   - guimanager.cpp:26-160  各面板的初始位置与尺寸
##   - guimanager.cpp:195      drawRoot 逐面板绘制
##   - guimanager.cpp:267      processEventRoot 逐面板事件
##   - guimanager.cpp:276-345  getWidget 按名索引
##
## ── 素材限制（必须说明）──
## 原版 UI 贴图来自 ID 0x07xxxxxx 的环境图库，而 mir2x_assets-main 里
## **只有 12 个贴图目录（equip/hair/helmet/hero/item/magic/map/monster/npc/proguse/selectchar/weapon），
## 没有任何 UI 图库**。
## 因此本阶段的界面用「纯色 + 文字 + 边框」绘制，
## 布局尺寸与层级严格按原版，贴图待素材补齐后接入（tex_provider 已预留接口）。
##
## 面板位置（guimanager.cpp 原文数值，1280x720 下）：
##   ControlBoard      : 底部贴边
##   QuickAccessBoard  : 底部，y = H - controlBoard.h() - 48
##   MyHeroStateBoard  : 居中，x = W/2 - 164, y = H/2 - 233
##   MiniMapBoard      : 右上
##   InventoryBoard    : 居中，x = W/2 - 179, y = H/2 - 134

const RoiRef := preload("res://scripts/ui/framework/ui_roi.gd")
const WidgetRef := preload("res://scripts/ui/framework/ui_widget.gd")
const CanvasRef := preload("res://scripts/ui/framework/ui_canvas.gd")
const ButtonRef := preload("res://scripts/ui/framework/ui_button.gd")
const LabelRef := preload("res://scripts/ui/framework/ui_label.gd")
const ImageRef := preload("res://scripts/ui/framework/ui_image.gd")
const ScrollRef := preload("res://scripts/ui/framework/ui_scroll.gd")
const CheckboxRef := preload("res://scripts/ui/framework/ui_checkbox.gd")
const SliderRef := preload("res://scripts/ui/framework/ui_slider.gd")
const InputRef := preload("res://scripts/ui/framework/ui_input_line.gd")

## 面板
var control_board: WidgetRef = null      ## 根控件（铺满屏幕）
var control_bar: WidgetRef = null        ## 底部功能按钮栏
var quick_board: WidgetRef = null        ## 快捷栏
var hero_state_board: WidgetRef = null   ## 人物状态（装备+属性）
var mini_map_board: WidgetRef = null     ## 小地图
var inventory_board: WidgetRef = null    ## 背包
var msg_stack_board: WidgetRef = null    ## 消息/聊天

## 绘制器
var _canvas: CanvasRef = null
var _diag_done := false

## 视口尺寸（每帧同步，原版用 getWindowLogicalWidth/Height）
var vw := 1280
var vh := 720

## 引用的游戏数据（由 main 注入）
var player_actor = null
var co_manager = null
var message_stack = null
var inventory = null


func _ready() -> void:
	layer = 10
	_canvas = CanvasRef.new()
	_canvas.name = "UiCanvas"
	add_child(_canvas)
	_canvas.setup()
	_build_ui()


func _process(_dt: float) -> void:
	if _canvas == null:
		return
	var vp := get_viewport()
	if vp != null:
		vw = int(vp.get_visible_rect().size.x)
		vh = int(vp.get_visible_rect().size.y)

	_layout()
	_canvas.queue_redraw()


## 面板跟随视口重排（对应 guimanager.cpp 的 afterResizeDefault）
func _layout() -> void:
	if control_board == null or control_bar == null:
		return

	# ControlBar 贴底（原版：y = H - controlBoard.h()）
	var cbh: int = control_bar.size_h()
	control_bar.position = Vector2(0, vh - cbh)

	# QuickAccessBoard（原版：y = H - controlBoard.h() - 48）
	if quick_board != null:
		quick_board.position = Vector2(0, vh - cbh - 48)

	# MyHeroStateBoard（原版：x = W/2 - 164, y = H/2 - 233）
	if hero_state_board != null:
		hero_state_board.position = Vector2(vw / 2 - 164, vh / 2 - 233)

	# InventoryBoard（原版：x = W/2 - 179, y = H/2 - 134）
	if inventory_board != null:
		inventory_board.position = Vector2(vw / 2 - 179, vh / 2 - 134)

	# MiniMapBoard：右上角
	if mini_map_board != null:
		mini_map_board.position = Vector2(vw - 158, 8)

	# 消息栈：左下，快捷栏上方
	if msg_stack_board != null:
		msg_stack_board.position = Vector2(8, vh - cbh - 48 - 138)


# ==================== 构建 ====================

func _build_ui() -> void:
	# 根控件（透明，铺满屏幕）
	control_board = WidgetRef.new()
	control_board.widget_name = "Root"
	control_board.w = func(_s): return vw
	control_board.h = func(_s): return vh
	control_board.on_draw = _draw_root_empty
	add_child(control_board)
	_canvas.root_widget = control_board

	_build_control_bar()
	_build_hero_state()
	_build_inventory()
	_build_msg_stack()
	_build_mini_map()

	# 立即布局一次，避免首帧 _draw 早于 _process 导致位置为 0
	_layout()


func _draw_root_empty(_w, _roi) -> void:
	pass


## 底部功能栏（对应 gui/controlboard）
## 原版按钮：背包/技能/行会/队伍/好友/设置/小地图/拍卖/摆摊...
func _build_control_bar() -> void:
	var bar := WidgetRef.new()
	bar.widget_name = "ControlBar"
	bar.w = func(_s): return vw
	bar.h = 44
	bar.on_draw = _draw_control_bar
	control_bar = bar
	control_board.add_widget(bar, 0, 0)

	var labels := ["背包", "技能", "行会", "队伍", "好友", "设置", "地图", "拍卖", "摊位", "帮助"]
	var bx := 6
	for i in range(labels.size()):
		var b := ButtonRef.new("Btn_" + labels[i], 52, 36)
		b.tex_provider = Callable()
		b.on_draw = _make_button_draw(labels[i])
		b.on_click = _make_button_click(i, labels[i])
		bar.add_widget(b, bx + i * 56, 4)

	# 快捷栏（对应 quickaccessboard）：6 格
	quick_board = WidgetRef.new()
	quick_board.widget_name = "QuickAccess"
	quick_board.w = 6 * 40 + 4
	quick_board.h = 40
	quick_board.on_draw = _draw_quick_bar
	control_board.add_widget(quick_board, 0, 0)


func _make_button_draw(text: String) -> Callable:
	return func(w, roi):
		var canvas := _canvas
		var bg: Color = Color(0.16, 0.16, 0.2, 0.92)
		if w.get_state() == ButtonRef.BEVENT_ON:
			bg = Color(0.24, 0.24, 0.3, 0.95)
		elif w.get_state() == ButtonRef.BEVENT_DOWN:
			bg = Color(0.3, 0.3, 0.36, 0.98)
		canvas.draw_rect_c(bg, roi.x, roi.y, roi.w, roi.h, true)
		canvas.draw_border(Color(0.42, 0.42, 0.5, 1.0), roi.x, roi.y, roi.w, roi.h, 1)
		canvas.draw_string_c(text, roi.x + 6, roi.y + 9, Color(0.92, 0.9, 0.86), null, 14)


func _make_button_click(idx: int, text: String) -> Callable:
	return func(_w):
		match idx:
			0:
				if inventory_board != null:
					inventory_board.show = not WidgetRef.eval_bool(inventory_board.show, inventory_board, true)
			2:
				_push_sys_msg("行会系统待实现")
			3:
				_push_sys_msg("队伍系统待实现")
			8:
				_push_sys_msg("摆摊系统待实现")
			_:
				_push_sys_msg("点击：" + text)


## 人物状态面板（对应 gui/myherostateboard，尺寸按原版 .w/.h）
## 原版：装备格 12 个 + 角色预览 + 属性文字
func _build_hero_state() -> void:
	hero_state_board = WidgetRef.new()
	hero_state_board.widget_name = "MyHeroStateBoard"
	hero_state_board.w = 328
	hero_state_board.h = 466
	hero_state_board.show = false     # 默认收起，点底部按钮打开
	hero_state_board.on_draw = _draw_hero_state
	control_board.add_widget(hero_state_board, 0, 0)


func _draw_hero_state(_w, roi) -> void:
	var canvas := _canvas
	canvas.draw_rect_c(Color(0.08, 0.08, 0.11, 0.95), roi.x, roi.y, roi.w, roi.h, true)
	canvas.draw_border(Color(0.5, 0.45, 0.3, 1.0), roi.x, roi.y, roi.w, roi.h, 2)
	canvas.draw_string_c("人物状态", roi.x + 10, roi.y + 8, Color(0.95, 0.85, 0.5), null, 16)

	# 12 个装备槽（原版 WLG_* 顺序）
	# 位置按 myherostateboard.hpp 的 WearGrid 布局
	var slots := [
		["武器", 10, 40], ["衣服", 250, 40],
		["头盔", 10, 84], ["项链", 250, 84],
		["手镯", 10, 128], ["手镯", 250, 128],
		["戒指", 10, 172], ["戒指", 250, 172],
		["护身符", 10, 216], ["腰带", 250, 216],
		["靴子", 10, 260], ["宝石", 250, 260],
	]
	for s in slots:
		var sx: int = roi.x + int(s[1])
		var sy: int = roi.y + int(s[2])
		canvas.draw_rect_c(Color(0.14, 0.14, 0.18, 0.9), sx, sy, 34, 34, true)
		canvas.draw_border(Color(0.35, 0.35, 0.42, 1.0), sx, sy, 34, 34, 1)
		canvas.draw_string_c(str(s[0]), sx + 2, sy + 36, Color(0.65, 0.65, 0.7), null, 11)

	# 角色名牌
	canvas.draw_string_c("测试角色  Lv.1", roi.x + 120, roi.y + 120, Color(0.9, 0.9, 0.95), null, 14)


## 背包（对应 inventoryboard，90 行 × 775 行）
## 原版：10x10 格，每格 36x32（SYS_INVGRIDPW/PH）
func _build_inventory() -> void:
	inventory_board = WidgetRef.new()
	inventory_board.widget_name = "InventoryBoard"
	inventory_board.w = 358
	inventory_board.h = 268
	inventory_board.show = false
	inventory_board.on_draw = _draw_inventory
	control_board.add_widget(inventory_board, 0, 0)


func _draw_inventory(_w, roi) -> void:
	var canvas := _canvas
	canvas.draw_rect_c(Color(0.07, 0.07, 0.1, 0.95), roi.x, roi.y, roi.w, roi.h, true)
	canvas.draw_border(Color(0.5, 0.45, 0.3, 1.0), roi.x, roi.y, roi.w, roi.h, 2)
	canvas.draw_string_c("背包", roi.x + 10, roi.y + 6, Color(0.95, 0.85, 0.5), null, 14)

	# 10x10 格（原版 SYS_INVGRIDW=10, H=10，格 36x32）
	var grid_x: int = roi.x + 12
	var grid_y: int = roi.y + 30
	var cw := 33
	var ch := 32
	for gy in range(10):
		for gx in range(10):
			var cx: int = grid_x + gx * cw
			var cy: int = grid_y + gy * ch
			canvas.draw_rect_c(Color(0.12, 0.12, 0.16, 0.85), cx, cy, cw - 1, ch - 1, true)
			canvas.draw_border(Color(0.25, 0.25, 0.3, 0.8), cx, cy, cw - 1, ch - 1, 1)


## 消息栈（对应 messagestackboard，173 行）
func _build_msg_stack() -> void:
	msg_stack_board = WidgetRef.new()
	msg_stack_board.widget_name = "MessageStack"
	msg_stack_board.w = 420
	msg_stack_board.h = 130
	msg_stack_board.on_draw = _draw_msg_stack
	control_board.add_widget(msg_stack_board, 0, 0)


func _draw_msg_stack(_w, roi) -> void:
	var canvas := _canvas
	canvas.draw_rect_c(Color(0, 0, 0, 0.35), roi.x, roi.y, roi.w, roi.h, true)
	var lines: Array = []
	if message_stack != null:
		lines = message_stack.latest(6)
	var y: int = roi.y + 4
	for ln in lines:
		canvas.draw_string_shadow(str(ln), roi.x + 4, y, Color(0.95, 0.93, 0.86), Color(0, 0, 0, 0.9), 1, null, 13)
		y += 18


## 小地图（对应 minimapboard，608 行）
func _build_mini_map() -> void:
	mini_map_board = WidgetRef.new()
	mini_map_board.widget_name = "MiniMapBoard"
	mini_map_board.w = 150
	mini_map_board.h = 150
	mini_map_board.on_draw = _draw_mini_map
	control_board.add_widget(mini_map_board, 0, 0)


func _draw_mini_map(_w, roi) -> void:
	var canvas := _canvas
	canvas.draw_rect_c(Color(0, 0, 0, 0.5), roi.x, roi.y, roi.w, roi.h, true)
	canvas.draw_border(Color(0.4, 0.4, 0.45, 1.0), roi.x, roi.y, roi.w, roi.h, 1)
	canvas.draw_string_c("小地图", roi.x + 4, roi.y + 4, Color(0.8, 0.8, 0.85), null, 12)
	# 玩家位置点（居中红点）
	var cx: int = roi.x + roi.w / 2
	var cy: int = roi.y + roi.h / 2
	canvas.draw_rect_c(Color(1, 0.2, 0.2, 1), cx - 2, cy - 2, 4, 4, true)


func _draw_control_bar(_w, roi) -> void:
	_canvas.draw_rect_c(Color(0.06, 0.06, 0.08, 0.94), roi.x, roi.y, roi.w, roi.h, true)
	_canvas.draw_line_c(Color(0.35, 0.3, 0.2, 1.0), roi.x, roi.y, roi.x + roi.w, roi.y, 1)


func _draw_quick_bar(_w, roi) -> void:
	_canvas.draw_rect_c(Color(0.1, 0.1, 0.13, 0.9), roi.x, roi.y, roi.w, roi.h, true)
	for i in range(6):
		var x: int = roi.x + 2 + i * 40
		_canvas.draw_rect_c(Color(0.16, 0.16, 0.2, 0.9), x, roi.y + 2, 36, 36, true)
		_canvas.draw_border(Color(0.32, 0.32, 0.38, 1.0), x, roi.y + 2, 36, 36, 1)
		_canvas.draw_string_c(str(i + 1), x + 2, roi.y + 3, Color(0.55, 0.55, 0.6), null, 10)


# ==================== 对外接口 ====================

## 切换面板显示
func toggle_panel(name_: String) -> void:
	var w: WidgetRef = control_board.find_widget(name_)
	if w == null:
		return
	w.show = not WidgetRef.eval_bool(w.show, w, true)


## 系统消息（对应 addCBLog(CBLOG_SYS, ...)）
func _push_sys_msg(text: String) -> void:
	if message_stack != null:
		message_stack.add_message(text)
	else:
		print("[UI] ", text)


## 输入事件转发（main 调用）
func handle_input(event: InputEvent) -> bool:
	if control_board == null:
		return false
	return control_board.process_event(event, RoiRef.new(0, 0, vw, vh))
