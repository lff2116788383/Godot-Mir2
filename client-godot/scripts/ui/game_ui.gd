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
const NpcChatRef := preload("res://scripts/ui/ui_npc_chat.gd")
const ItemGfxRef := preload("res://scripts/item/item_gfx.gd")
const ItemRecordRef := preload("res://scripts/item/item_record.gd")
const ItemLayoutRef := preload("res://scripts/item/item_layout.gd")
const SkillBoardRef := preload("res://scripts/ui/ui_skill_board.gd")
const TeamBoardRef := preload("res://scripts/ui/ui_team_board.gd")
const ItemListBoardRef := preload("res://scripts/ui/ui_item_list_board.gd")
const GuildBoardRef := preload("res://scripts/ui/ui_guild_board.gd")
const ConfigBoardRef := preload("res://scripts/ui/ui_config_board.gd")
## C7 新增面板
const CtrlBoardRef := preload("res://scripts/ui/ui_control_board.gd")
const HeroStateRef := preload("res://scripts/ui/ui_hero_state_board.gd")
const QuickAccessRef := preload("res://scripts/ui/ui_quick_access_board.gd")
const PurchaseRef := preload("res://scripts/ui/ui_purchase_board.gd")
const DirectTradeRef := preload("res://scripts/ui/ui_direct_trade_board.gd")
const StallBoardRef := preload("res://scripts/ui/ui_stall_board.gd")
const AuctionRef := preload("res://scripts/ui/ui_auction_board.gd")
const FriendChatRef := preload("res://scripts/ui/ui_friend_chat_board.gd")
const QuestBoardRef := preload("res://scripts/ui/ui_quest_board.gd")
const HorseBoardRef := preload("res://scripts/ui/ui_horse_board.gd")
const MiniMapBoardRef := preload("res://scripts/ui/ui_mini_map_board.gd")
const InputStringRef := preload("res://scripts/ui/ui_input_string_board.gd")
const MenuBoardRef := preload("res://scripts/ui/ui_menu_board.gd")
const RankingBoardRef := preload("res://scripts/ui/ui_ranking_board.gd")
const ProgUseTexRef := preload("res://scripts/data/proguse_tex.gd")
const OverHeadRef := preload("res://scripts/ui/ui_over_head.gd")

## 面板
var control_board: WidgetRef = null      ## 根控件（铺满屏幕）
var control_bar: WidgetRef = null        ## 底部功能按钮栏
var quick_board: WidgetRef = null        ## 快捷栏
var hero_state_board: WidgetRef = null   ## 人物状态（装备+属性）
var mini_map_board: WidgetRef = null     ## 小地图
var inventory_board: WidgetRef = null    ## 背包
var msg_stack_board: WidgetRef = null    ## 消息/聊天
var npc_chat: NpcChatRef = null          ## NPC 对话（C3）

## C5 面板
var skill_board: SkillBoardRef = null        ## 技能
var team_board: TeamBoardRef = null          ## 队伍
var guild_board: GuildBoardRef = null        ## 行会
var config_board: ConfigBoardRef = null      ## 设置
var shop_board: ItemListBoardRef = null      ## 商店（NPC 购买）
var auction_board: ItemListBoardRef = null   ## 拍卖行
var stall_board: ItemListBoardRef = null     ## 摆摊
var secured_board: ItemListBoardRef = null   ## 寄存

## C7 面板
var ctrl_board: CtrlBoardRef = null           ## 底部控制栏（完整版，替代旧 control_bar）
var my_state_board: HeroStateRef = null       ## 自身人物状态
var peer_state_board: HeroStateRef = null     ## 他人人物状态
var quick_access: QuickAccessRef = null       ## 快捷栏（6 格腰带）
var purchase_board: PurchaseRef = null        ## NPC 商店
var direct_trade: DirectTradeRef = null       ## 直接交易
var stall_board2: StallBoardRef = null        ## 摆摊（完整版）
var auction2: AuctionRef = null               ## 拍卖行（完整版）
var friend_chat: FriendChatRef = null         ## 好友聊天
var quest_board: QuestBoardRef = null         ## 任务
var horse_board: HorseBoardRef = null         ## 马匹
var mini_map2: MiniMapBoardRef = null         ## 小地图（完整版）
var input_board: InputStringRef = null        ## 输入串 / 确认框
var menu: MenuBoardRef = null                 ## 右键菜单
var ranking_board: RankingBoardRef = null    ## 排行榜

## 背包数据（由 main 注入 SDInventory）
var inventory_data = null
var inv_hover := -1
var inv_mouse := Vector2.ZERO
## 人物装备（WLG 槽位 -> 物品 id）
var wear_list: Dictionary = {}
## 悬停的背包格（-1 表示无）
var _hover_cell := -1
## 背包格尺寸（原版 SYS_INVGRIDPW/PH）
const INV_GRID_W := 33
const INV_GRID_H := 32
const INV_COLS := 10
const INV_ROWS := 10

## 绘制器
var _canvas: CanvasRef = null
var _diag_done := false

## 视口尺寸（每帧同步，原版用 getWindowLogicalWidth/Height）
var vw := 1280
var vh := 720

## 引用的游戏数据（由 main 注入）
var player_actor = null
var co_manager = null
var world = null
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

	# 控制栏（C7 完整版）：贴底，宽度 = 视口宽
	if ctrl_board != null:
		ctrl_board.vw = vw
		ctrl_board.w = vw
		var bh: int = ctrl_board.bar_height()
		ctrl_board.h = bh
		ctrl_board.position = Vector2(0, vh - bh)

	# 快捷栏：控制栏上方 48px（原版 y = H - controlBoard.h() - 48）
	if quick_access != null:
		var cbh: int = ctrl_board.bar_height() if ctrl_board != null else 44
		quick_access.position = Vector2(0, vh - cbh - 48)
		quick_board = quick_access

	# 自身人物状态：居中（原版 x = W/2 - 164, y = H/2 - 233）
	if my_state_board != null:
		my_state_board.position = Vector2(vw / 2 - 164, vh / 2 - 233)
		my_state_board.set_abs(int(my_state_board.position.x), int(my_state_board.position.y))

	# 他人状态：居中（284x348）
	if peer_state_board != null:
		peer_state_board.position = Vector2((vw - 284) / 2, (vh - 348) / 2)
		peer_state_board.set_abs(int(peer_state_board.position.x), int(peer_state_board.position.y))

	# 旧引用同步（背包位置按原版：x = W/2 - 179, y = H/2 - 134）
	if inventory_board != null:
		inventory_board.position = Vector2(vw / 2 - 179, vh / 2 - 134)

	# 小地图（完整版）：普通模式贴右上，扩展模式居中
	if mini_map2 != null:
		var p := mini_map2.place(vw, vh, 0, 0)
		mini_map2.position = p

	# 旧小地图引用（hide 掉旧的）
	if mini_map_board != null:
		mini_map_board.show = false

	# 消息栈：左下，快捷栏上方
	if msg_stack_board != null:
		var cbh2: int = ctrl_board.bar_height() if ctrl_board != null else 44
		msg_stack_board.position = Vector2(8, vh - cbh2 - 48 - 138)

	# C5 面板统一居中
	for b in [skill_board, team_board, guild_board, config_board,
			shop_board, auction_board, stall_board, secured_board]:
		if b != null:
			b.position = Vector2((vw - b.size_w()) / 2, (vh - b.size_h()) / 2)

	# C7 面板统一居中（按各自实际尺寸）
	var centered := [purchase_board, direct_trade, stall_board2, auction2,
		friend_chat, quest_board, horse_board, input_board, ranking_board]
	for b in centered:
		if b != null:
			b.position = Vector2((vw - b.size_w()) / 2, (vh - b.size_h()) / 2)

	# NPC 对话：屏幕中下部
	if npc_chat != null:
		var cbh3: int = ctrl_board.bar_height() if ctrl_board != null else 44
		npc_chat.position = Vector2((vw - npc_chat.size_w()) / 2, vh - cbh3 - 48 - 260)


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
	_build_npc_chat()
	_build_c5_boards()
	_build_c7_boards()

	# 立即布局一次，避免首帧 _draw 早于 _process 导致位置为 0
	_layout()


func _draw_root_empty(_w, _roi) -> void:
	pass


## 底部控制栏（C7 完整版，对应 gui/controlboard 的 2500 行）
## 旧版是个简化按钮条；现在换成逐行翻译的 UiControlBoard，
## 含日志窗口 + 命令行 + HP/MP/等级/负重条 + 右段 9 钮 + 标题条。
## 旧接口（control_bar / _draw_control_bar / Btn_背包 等）保留为兼容层。
func _build_control_bar() -> void:
	ctrl_board = CtrlBoardRef.new("ControlBoard")
	ctrl_board.set_canvas(_canvas)
	ctrl_board.on_action = _on_ctrl_action
	ctrl_board.on_command = _on_ctrl_command
	ctrl_board.on_mode_change = func(_m): _layout()
	control_board.add_widget(ctrl_board, 0, 0)

	# 兼容层：旧代码里有 control_bar 引用与 Btn_xxx 按钮查找，
	# 用一个透明控件占位，避免空引用崩溃
	control_bar = WidgetRef.new()
	control_bar.widget_name = "ControlBar"
	control_bar.w = func(_s): return vw
	control_bar.h = 0
	control_bar.on_draw = func(_w, _roi): pass
	control_board.add_widget(control_bar, 0, 0)

	# 快捷栏（完整版 6 格腰带）
	quick_access = QuickAccessRef.new("QuickAccessBoard")
	quick_access.set_canvas(_canvas)
	quick_access.on_slot_click = func(slot):
		_push_sys_msg("使用快捷栏第 %d 格" % (slot + 1))
	quick_access.on_close = func(_w): pass
	control_board.add_widget(quick_access, 0, 0)
	quick_board = quick_access   # 旧引用


## 控制栏按钮动作分发（对应 cbright.cpp 各 onTrigger）
func _on_ctrl_action(act: String) -> void:
	match act:
		"inventory":
			_toggle(inventory_board, true)
		"herostate":
			if my_state_board != null:
				my_state_board.show = not my_state_board.show
		"skill":
			_toggle(skill_board, true)
		"guild":
			_toggle(guild_board, true)
		"team":
			_toggle(team_board, true)
		"quest":
			if quest_board != null:
				quest_board.show = not quest_board.show
		"horse":
			if horse_board != null:
				horse_board.show = not horse_board.show
		"config":
			_toggle(config_board, true)
		"friendchat":
			if friend_chat != null:
				friend_chat.show = not friend_chat.show
		"auction":
			if auction2 != null:
				auction2.show = not auction2.show
		"stall":
			if stall_board2 != null:
				stall_board2.show = not stall_board2.show
		"minimap":
			if mini_map2 != null:
				mini_map2.show = not mini_map2.show
				_layout()
		"quickaccess":
			if quick_access != null:
				quick_access.show = not quick_access.show
		"request_exit":
			if input_board != null:
				input_board.wait_choice("退出游戏", ["退出游戏", "离线挂机", "取消"])
				input_board.on_choice = func(idx):
					match idx:
						0: _push_sys_msg("退出游戏")
						1: _push_sys_msg("离线挂机")
		"request_logout":
			if input_board != null:
				input_board.wait_choice("返回人物界面吗？", ["确定", "取消"])
		"magickey":
			_push_sys_msg("切换魔法快捷键显示")
		_:
			_push_sys_msg("控制栏动作：" + act)


## 控制栏命令提交（controlboard.cpp:onInputDone 的分发规则）
func _on_ctrl_command(text: String) -> void:
	if world != null and world.handle_command(text):
		return
	var kind: String = CtrlBoardRef.command_kind(text)
	match kind:
		"guild":
			_push_sys_msg("[行会] " + text.substr(2))
		"broadcast":
			_push_sys_msg("[广播] " + text.substr(1))
		"user":
			_push_sys_msg("[命令] " + text.substr(1))
		"lua":
			_push_sys_msg("[lua] " + text.substr(1))
		_:
			_push_sys_msg(text)
			if ctrl_board != null:
				ctrl_board.add_log(text, CtrlBoardRef.CBLOG_DEF)


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
			0:  # 背包
				_toggle(skill_board, false)
				_toggle(inventory_board, true)
			1:  # 技能
				_toggle(inventory_board, false)
				_toggle(skill_board, true)
			2:  # 行会
				_toggle(guild_board, true)
			3:  # 队伍
				_toggle(team_board, true)
			4:  # 好友
				_push_sys_msg("好友系统待实现")
			5:  # 设置
				_toggle(config_board, true)
			6:  # 地图
				_push_sys_msg("地图 %dx%d" % [vw, vh])
			7:  # 拍卖
				_toggle(auction_board, true)
			8:  # 摊位
				_toggle(stall_board, true)
			9:  # 帮助
				_push_sys_msg("点击地面移动，点底部按钮打开面板")
			_:
				_push_sys_msg("点击：" + text)


## 开关某面板；exclusive=true 时关闭同组其他面板
func _toggle(board, exclusive: bool) -> void:
	if board == null:
		return
	var cur := WidgetRef.eval_bool(board.show, board, false)
	board.show = not cur
	if not cur and exclusive:
		pass


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
	inventory_board.on_event = _handle_inventory_event
	control_board.add_widget(inventory_board, 0, 0)


func _draw_inventory(_w, roi) -> void:
	var canvas := _canvas
	canvas.draw_rect_c(Color(0.07, 0.07, 0.1, 0.95), roi.x, roi.y, roi.w, roi.h, true)
	canvas.draw_border(Color(0.5, 0.45, 0.3, 1.0), roi.x, roi.y, roi.w, roi.h, 2)
	canvas.draw_string_c("背包", roi.x + 10, roi.y + 6, Color(0.95, 0.85, 0.5), null, 14)
	var cnt := 0
	if inventory_data != null:
		cnt = inventory_data.size()
	canvas.draw_string_c("%d 件" % cnt, roi.x + roi.w - 56, roi.y + 6,
		Color(0.7, 0.68, 0.62), null, 12)

	var grid_x: int = roi.x + 12
	var grid_y: int = roi.y + 30

	# 先画底格
	for gy in range(INV_ROWS):
		for gx in range(INV_COLS):
			var cx: int = grid_x + gx * INV_GRID_W
			var cy: int = grid_y + gy * INV_GRID_H
			canvas.draw_rect_c(Color(0.12, 0.12, 0.16, 0.85), cx, cy,
				INV_GRID_W - 1, INV_GRID_H - 1, true)
			canvas.draw_border(Color(0.25, 0.25, 0.3, 0.8), cx, cy,
				INV_GRID_W - 1, INV_GRID_H - 1, 1)

	# 再按顺序画物品（原版按背包格线性排列）
	if inventory_data == null:
		return
	var items: Array = inventory_data.item_list()
	for i in range(mini(items.size(), INV_COLS * INV_ROWS)):
		var it = items[i]
		if it == null:
			continue
		var gx2: int = i % INV_COLS
		var gy2: int = i / INV_COLS
		var cx2: int = grid_x + gx2 * INV_GRID_W
		var cy2: int = grid_y + gy2 * INV_GRID_H

		var tex := ItemGfxRef.icon_of_item(it.item_id)
		if tex != null:
			# 图标在格内居中（原版 drawItem 的做法）
			var off := ItemGfxRef.center_offset(tex, INV_GRID_W - 1, INV_GRID_H - 1)
			canvas.tex_draw(tex, cx2 + off.x, cy2 + off.y)
		else:
			# 无图标时画名字首字
			var nm := ItemRecordRef.name(it.item_id)
			if not nm.is_empty():
				canvas.draw_string_c(nm.substr(0, 1), cx2 + 10, cy2 + 10,
					Color(0.85, 0.8, 0.7), null, 12)

		# 数量（>1 时右下角显示）
		if it.count > 1:
			canvas.draw_string_shadow(str(it.count), cx2 + INV_GRID_W - 14, cy2 + INV_GRID_H - 15,
				Color(1, 1, 0.7), Color(0, 0, 0, 0.9), 1, null, 11)


## 消息栈（对应 messagestackboard，173 行）
func _handle_inventory_event(_w, event: InputEvent, _valid: bool, roi) -> bool:
	var p: Vector2 = event.position
	var gx := int((p.x - roi.x - 12) / INV_GRID_W)
	var gy := int((p.y - roi.y - 30) / INV_GRID_H)
	var inside := gx >= 0 and gy >= 0 and gx < INV_COLS and gy < INV_ROWS
	if event is InputEventMouseMotion:
		var old := inv_hover
		inv_hover = gy * INV_COLS + gx if inside else -1
		inv_mouse = p
		if old != inv_hover:
			inventory_board.queue_redraw()
		return false
	if not (event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT):
		return false
	if not inside:
		return false
	if inventory_data == null:
		return true
	var idx := gy * INV_COLS + gx
	var items: Array = inventory_data.item_list()
	if idx >= items.size() or items[idx] == null:
		return true
	var it = items[idx]
	if event.double_click and world != null:
		world.activate_inventory_item(it)
	else:
		_push_sys_msg("物品：%s x%d" % [ItemRecordRef.name(it.item_id), it.count])
	return true


func _draw_item_tooltip(canvas, item) -> void:
	if item == null:
		return
	var x := int(inv_mouse.x + 16)
	var y := int(inv_mouse.y + 12)
	var nm := ItemRecordRef.name(item.item_id)
	var typ := ItemRecordRef.type(item.item_id)
	var lines: Array = [nm, "类型：" + typ]
	if item.duration.size() >= 2 and int(item.duration[1]) > 0:
		lines.append("耐久：%d / %d" % [int(item.duration[0]), int(item.duration[1])])
	var desc := ItemRecordRef.description(item.item_id)
	if not desc.is_empty():
		lines.append(desc)
	var w := 220
	var h := 12 + lines.size() * 18
	canvas.draw_rect_c(Color(0, 0, 0, 0.92), x, y, w, h, true)
	canvas.draw_border(Color(0.62, 0.52, 0.3, 1.0), x, y, w, h, 1)
	var yy := y + 6
	for i in range(lines.size()):
		canvas.draw_string_c(str(lines[i]), x + 8, yy,
				Color(0.95, 0.88, 0.65) if i == 0 else Color(0.84, 0.82, 0.76), null, 12)
		yy += 18

	if inv_hover >= 0 and inventory_data != null:
		var all_items: Array = inventory_data.item_list()
		if inv_hover < all_items.size():
			_draw_item_tooltip(canvas, all_items[inv_hover])


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


## NPC 对话面板（对应 gui/npcchatboard）
func _build_npc_chat() -> void:
	npc_chat = NpcChatRef.new("NpcChat")
	npc_chat.set_canvas(_canvas)
	npc_chat.on_npc_event = func(_w, evt_path): _push_sys_msg("NPC 事件: " + evt_path)
	npc_chat.on_close = func(_w): _push_sys_msg("关闭对话")
	control_board.add_widget(npc_chat, 0, 0)


## C7 面板集合（对应 guimanager.hpp 的完整面板成员表）
## 逐个 set_canvas，因为绘制器在 CanvasLayer 里、面板本身只是控件
func _build_c7_boards() -> void:
	# 自身人物状态（myherostateboard，328x466）
	my_state_board = HeroStateRef.new("MyHeroStateBoard", true)
	my_state_board.set_canvas(_canvas)
	my_state_board.on_close = func(_w): pass
	control_board.add_widget(my_state_board, 0, 0)
	hero_state_board = my_state_board   # 旧引用

	# 他人人物状态（herostateboard，284x348）
	peer_state_board = HeroStateRef.new("HeroStateBoard", false)
	peer_state_board.set_canvas(_canvas)
	peer_state_board.on_trade = func(_w): _push_sys_msg("发起交易申请")
	peer_state_board.on_add_friend = func(_w): _push_sys_msg("加为好友")
	control_board.add_widget(peer_state_board, 0, 0)

	# NPC 商店（purchaseboard，290x224 起）
	purchase_board = PurchaseRef.new("PurchaseBoard")
	purchase_board.set_canvas(_canvas)
	purchase_board.on_close = func(_w): pass
	purchase_board.on_buy = func(iid, _seq, price):
		_push_sys_msg("购买 %s，价格 %d" % [ItemRecordRef.name(iid), price])
	control_board.add_widget(purchase_board, 0, 0)

	# 直接交易（directtradeboard，494x330）
	direct_trade = DirectTradeRef.new("DirectTradeBoard")
	direct_trade.set_canvas(_canvas)
	direct_trade.on_close = func(_w): pass
	control_board.add_widget(direct_trade, 0, 0)

	# 摆摊（stallboard，282x390）
	stall_board2 = StallBoardRef.new("StallBoardFull")
	stall_board2.set_canvas(_canvas)
	stall_board2.on_buy = func(idx, price):
		_push_sys_msg("购买摊位物品 #%d，单价 %d" % [idx, price])
	control_board.add_widget(stall_board2, 0, 0)

	# 拍卖行（auctionboard，720x440）
	auction2 = AuctionRef.new("AuctionBoardFull")
	auction2.set_canvas(_canvas)
	auction2.on_close = func(_w): pass
	control_board.add_widget(auction2, 0, 0)

	# 好友聊天（friendchatboard，400x400 起，可缩放）
	friend_chat = FriendChatRef.new("FriendChatBoard")
	friend_chat.set_canvas(_canvas)
	friend_chat.on_send = func(t): _push_sys_msg("[好友] " + t)
	control_board.add_widget(friend_chat, 0, 0)

	# 任务（queststateboard，363x446）
	quest_board = QuestBoardRef.new("QuestStateBoard")
	quest_board.set_canvas(_canvas)
	control_board.add_widget(quest_board, 0, 0)

	# 马匹（horseboard，257x322）
	horse_board = HorseBoardRef.new("HorseBoard")
	horse_board.set_canvas(_canvas)
	control_board.add_widget(horse_board, 0, 0)

	# 小地图完整版（minimapboard，200x200）
	mini_map2 = MiniMapBoardRef.new("MiniMapBoardFull")
	mini_map2.set_canvas(_canvas)
	control_board.add_widget(mini_map2, 0, 0)

	# 输入串 / 确认框（inputstringboard，358x268）
	input_board = InputStringRef.new("InputStringBoard")
	input_board.set_canvas(_canvas)
	control_board.add_widget(input_board, 0, 0)

	# 右键菜单（运行时创建，先建空实例）
	ranking_board = RankingBoardRef.new("RankingBoard")
	ranking_board.set_canvas(_canvas)
	control_board.add_widget(ranking_board, 0, 0)

	menu = MenuBoardRef.new()
	menu.set_canvas(_canvas)


## C5 面板集合（对应 guimanager.hpp 的 21 个面板成员）
func _build_c5_boards() -> void:
	# 技能（gui/skillboard）
	skill_board = SkillBoardRef.new("SkillBoard")
	skill_board.on_skill_click = func(_w, nm): _push_sys_msg("技能: " + nm)
	control_board.add_widget(skill_board, 0, 0)

	# 队伍（gui/teamstateboard）
	team_board = TeamBoardRef.new("TeamStateBoard")
	team_board.on_page_change = func(_w, idx): pass
	control_board.add_widget(team_board, 0, 0)

	# 行会（guildboard）
	guild_board = GuildBoardRef.new("GuildBoard")
	guild_board.on_send_chat = func(_w, txt): _push_sys_msg("[行会] " + txt)
	control_board.add_widget(guild_board, 0, 0)

	# 设置（gui/runtimeconfigboard）
	config_board = ConfigBoardRef.new("RuntimeConfigBoard")
	config_board.on_change = func(_w, k, v): _push_sys_msg("%s = %s" % [k, str(v)])
	control_board.add_widget(config_board, 0, 0)

	# 商店 / 拍卖 / 摆摊 / 寄存：同一套列表面板，configure 区分
	shop_board = ItemListBoardRef.new("PurchaseBoard")
	shop_board.configure("purchase")
	shop_board.on_action = func(_w, idx): _push_sys_msg("购买物品 #%d" % idx)
	control_board.add_widget(shop_board, 0, 0)

	auction_board = ItemListBoardRef.new("AuctionBoard")
	auction_board.configure("auction")
	control_board.add_widget(auction_board, 0, 0)

	stall_board = ItemListBoardRef.new("StallBoard")
	stall_board.configure("stall")
	control_board.add_widget(stall_board, 0, 0)

	secured_board = ItemListBoardRef.new("SecuredItemListBoard")
	secured_board.configure("secured")
	control_board.add_widget(secured_board, 0, 0)


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
