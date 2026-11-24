extends SceneTree
## C7+C8 自检：新面板布局 + 音频系统
##
## 静态测试（--script 模式）：
##   Autoload 与 class_name 不可用，全部走 preload
##   验证几何常量、状态机、ID 合成规则、素材可达性

const CtrlRef := preload("res://scripts/ui/ui_control_board.gd")
const HeroStateRef := preload("res://scripts/ui/ui_hero_state_board.gd")
const QuickAccessRef := preload("res://scripts/ui/ui_quick_access_board.gd")
const DirectTradeRef := preload("res://scripts/ui/ui_direct_trade_board.gd")
const StallRef := preload("res://scripts/ui/ui_stall_board.gd")
const PurchaseRef := preload("res://scripts/ui/ui_purchase_board.gd")
const AuctionRef := preload("res://scripts/ui/ui_auction_board.gd")
const FriendChatRef := preload("res://scripts/ui/ui_friend_chat_board.gd")
const QuestRef := preload("res://scripts/ui/ui_quest_board.gd")
const HorseRef := preload("res://scripts/ui/ui_horse_board.gd")
const MiniMapRef := preload("res://scripts/ui/ui_mini_map_board.gd")
const InputRef := preload("res://scripts/ui/ui_input_string_board.gd")
const MenuRef := preload("res://scripts/ui/ui_menu_board.gd")
const OverHeadRef := preload("res://scripts/ui/ui_over_head.gd")
const StallBarRef := preload("res://scripts/ui/ui_stall_bar.gd")
const UnderBodyRef := preload("res://scripts/ui/ui_under_body.gd")
const AudioRef := preload("res://scripts/audio/audio_manager.gd")
const ProgUseRef := preload("res://scripts/data/proguse_tex.gd")

var _pass := 0
var _fail := 0


func _ok(cond: bool, name: String, extra: String = "") -> void:
	if cond:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, ("  " + extra) if extra else "")


func _init() -> void:
	print("=== C7+C8 自检 ===")
	ProgUseRef.setup()
	print("[info] UI 图库根: %s" % ProgUseRef.assets_root())
	_test_proguse()
	_test_control_board()
	_test_hero_state()
	_test_quick_access()
	_test_overhead_underbody()
	_test_trade_boards()
	_test_social_boards()
	_test_audio()
	print("\n=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0 if _fail == 0 else 1)


# ==================== UI 图库 ====================

func _test_proguse() -> void:
	print("\n[C7-0] UI 图库（proguse）")
	var root := ProgUseRef.assets_root()
	_ok(not root.is_empty(), "素材根可达", root)

	# 关键贴图存在且尺寸吻合 C++ 常量
	var checks := [
		[0x00000012, 800, 133, "控制栏左右背景(800x133)"],
		[0x00000013, 456, 131, "控制栏中部(456x131)"],
		[0x00000022, 127, 41, "控制栏标题(127x41)"],
		[0x00000018, 36, 88, "HP条(36x88)"],
		[0x00000019, 36, 88, "MP条(36x88)"],
		[0x000000A0, 3, 89, "等级/负重条(3x89)"],
		[0x06000000, 328, 466, "自身状态框(328x466)"],
		[0x00002020, 284, 348, "他人状态框(284x348)"],
		[0x00002010, 494, 330, "直接交易背景(494x330)"],
		[0x00000230, 282, 390, "摆摊背景(282x390)"],
		[0x00001400, 720, 440, "拍卖行背景(720x440)"],
		[0x00000800, 491, 437, "好友聊天框(491x437)"],
		[0x00000350, 363, 446, "任务面板(363x446)"],
		[0x00000700, 257, 322, "马匹面板(257x322)"],
		[0x07000000, 358, 268, "输入串面板(358x268)"],
		[0x00000060, 280, 48, "快捷栏背景(280x48)"],
		[0x00000014, 32, 4, "血条-不显数字(32x4)"],
		[0x00000016, 101, 7, "血条-显数字(101x7)"],
	]
	for c in checks:
		var sz := ProgUseRef.tex_size(int(c[0]))
		_ok(sz.x == int(c[1]) and sz.y == int(c[2]),
			"贴图 %s 尺寸 %dx%d" % [c[3], sz.x, sz.y],
			"期望 %dx%d" % [int(c[1]), int(c[2])])

	# 各按钮三态贴图
	var btns := [0x0000001C, 0x0000001E, 0x00000020, 0x00000028,
		0x00000030, 0x00000032, 0x00000034, 0x00000036, 0x00000038,
		0x0000003A, 0x0000003C, 0x0000003E, 0x00000040,
		0x00000042, 0x00000043, 0x00000044]
	var found := 0
	for b in btns:
		if ProgUseRef.retrieve(int(b)) != null:
			found += 1
	_ok(found == btns.size(), "16 个控制栏按钮贴图全部存在", "%d/%d" % [found, btns.size()])

	var st := ProgUseRef.stats()
	print("    [info] 图库统计: %s" % str(st))


# ==================== C7-1 控制栏 ====================

func _test_control_board() -> void:
	print("\n[C7-1] 底部控制栏")
	var cb := CtrlRef.new()
	_ok(cb.w == 800, "默认宽 800（原版 178+296+166+…=800 的基准）")

	# 三段几何
	_ok(CtrlRef.LEFT_W == 178, "左段 178")
	_ok(CtrlRef.RIGHT_W == 166, "右段 166")
	_ok(CtrlRef.MIDDLE_H == 131, "中段高 131（=0x13 贴图高）")
	_ok(CtrlRef.TITLE_H == 21, "标题高 21（CBTitle::UP_HEIGHT）")
	_ok(CtrlRef.TOTAL_H == 152, "整栏高 152")

	# 中段宽度随视口
	cb.vw = 1280
	_ok(cb.middle_width() == 1280 - 178 - 166, "中段宽 = 视口 - 左 - 右",
		str(cb.middle_width()))
	_ok(cb.log_window_width() == cb.middle_width() - 456 + 344,
		"日志窗宽 = 中段 - 456 + 344", str(cb.log_window_width()))

	# 标题条居中
	var tx := cb.title_x()
	_ok(tx == 178 + (1280 - 178 - 166) / 2, "标题条 x = 左 + 中段/2", str(tx))

	# 模式切换影响高度
	_ok(cb.bar_height() == 152, "普通模式高 152")
	cb.mode = CtrlRef.MODE_MINIMIZE
	_ok(cb.bar_height() == 24, "最小化高 21+3=24", str(cb.bar_height()))
	cb.mode = CtrlRef.MODE_NORMAL

	# 日志
	cb.add_log("系统消息", CtrlRef.CBLOG_SYS)
	cb.add_log("普通消息")
	_ok(cb.log_lines.size() == 2, "日志 2 条")
	_ok(int(cb.log_lines[0]["type"]) == CtrlRef.CBLOG_SYS, "第一条是系统类型")
	_ok(is_equal_approx(cb.log_scroll, 1.0), "加日志后自动滚到底")
	cb.scroll_log(-0.5)
	_ok(is_equal_approx(cb.log_scroll, 0.5), "滚动生效")

	# 日志上限 1000
	for i in range(1100):
		cb.add_log("x%d" % i)
	_ok(cb.log_lines.size() == CtrlRef.MAX_LOG_PAR, "日志上限 1000",
		str(cb.log_lines.size()))

	# 命令分发
	_ok(CtrlRef.command_kind("!~行会") == "guild", "!~ 是行会")
	_ok(CtrlRef.command_kind("!广播") == "broadcast", "! 是广播")
	_ok(CtrlRef.command_kind("@cmd") == "user", "@ 是命令")
	_ok(CtrlRef.command_kind("$lua") == "lua", "$ 是 lua")
	_ok(CtrlRef.command_kind("你好") == "talk", "普通是 talk")
	_ok(CtrlRef.command_kind("") == "empty", "空是 empty")

	# 命令提交清空输入
	cb.cmd_text = "  你好  "
	var submitted := cb.submit_command()
	_ok(submitted == "你好", "提交时 trim", submitted)
	_ok(cb.cmd_text.is_empty(), "提交后清空")


# ==================== C7-2 人物状态 / 快捷栏 / 头顶脚下 ====================

func _test_hero_state() -> void:
	print("\n[C7-2] 人物状态板")
	var me := HeroStateRef.new("MyHeroStateBoard", true)
	_ok(me.w == 328 and me.h == 466, "自身版 328x466")
	_ok(not me.show, "默认关闭")

	var peer := HeroStateRef.new("HeroStateBoard", false)
	_ok(peer.w == 284 and peer.h == 348, "他人版 284x348")

	# 装备格布局
	var gs := HeroStateRef.grid_layout_self()
	_ok(gs.size() == 11, "自身 11 个装备格", str(gs.size()))
	var gp := HeroStateRef.grid_layout_peer()
	_ok(gp.size() == 11, "他人 11 个装备格", str(gp.size()))

	# 衣服格相对锚点：x = char_x, y = char_y - 100
	var dress: Array = gs[1]   # WLG_DRESS
	_ok(int(dress[0]) == 90 and int(dress[1]) == 100,
		"自身衣服格 (90, 200-100=100)", str(dress))
	var dress_peer: Array = gp[1]
	_ok(int(dress_peer[0]) == 103 and int(dress_peer[1]) == 129,
		"他人衣服格 (103, 229-100=129)", str(dress_peer))

	# 头盔格：char_x+10, char_y-135
	var helm: Array = gs[2]
	_ok(int(helm[0]) == 100 and int(helm[1]) == 65, "自身头盔格 (100,65)", str(helm))
	# 武器格：char_x-50, char_y-120
	var wp: Array = gs[3]
	_ok(int(wp[0]) == 40 and int(wp[1]) == 80, "自身武器格 (40,80)", str(wp))

	# 他人的鞋格 43x61（hpp 里的特殊值）
	var shoes_peer: Array = gp[4]
	_ok(int(shoes_peer[2]) == 43 and int(shoes_peer[3]) == 61,
		"他人鞋格 43x61", str(shoes_peer))

	# inspect / close
	peer.inspect("测试", 30, 0, {3: 100})
	_ok(peer.show, "inspect 后显示")
	_ok(peer.get_wear(3) == 100, "装备读取正确")
	peer.close()
	_ok(not peer.show, "close 后隐藏")


func _test_quick_access() -> void:
	print("\n[C7-2] 快捷栏")
	var q := QuickAccessRef.new()
	_ok(q.w == 280 and q.h == 48, "280x48（0x60 贴图尺寸）")
	_ok(not q.show, "默认关闭")

	# getGridLoc：17 + 42*slot, 6, 36x36
	var g0 := QuickAccessRef.grid_loc(0)
	_ok(int(g0.position.x) == 17 and int(g0.position.y) == 6, "第0格 (17,6)", str(g0))
	var g5 := QuickAccessRef.grid_loc(5)
	_ok(int(g5.position.x) == 17 + 42 * 5, "第5格 x = 17+42*5 = 227", str(g5))
	_ok(int(g5.size.x) == 36 and int(g5.size.y) == 36, "格 36x36")

	# 越界返回空矩形
	var gx := QuickAccessRef.grid_loc(6)
	_ok(int(gx.size.x) == 0, "第6格越界返回空")

	# 腰带数据
	q.set_belt([{"item_id": 1, "count": 5}])
	_ok(q.get_belt(0) != null, "第0格有物品")
	_ok(q.get_belt(1) == null, "第1格已补齐为 null")
	_ok(q.belt.size() == QuickAccessRef.SLOT_COUNT, "腰带补齐到 6 格")
	# 消耗
	q.consume_slot(0)
	_ok(int(q.get_belt(0)["count"]) == 4, "消耗后数量 4")
	for i in range(4):
		q.consume_slot(0)
	_ok(q.get_belt(0) == null, "用完清空")


func _test_overhead_underbody() -> void:
	print("\n[C7-2] 头顶板 / 脚下板 / 摊位列")
	# 头顶血条尺寸规则
	var oh := OverHeadRef.new()
	oh.cfg_show_number = true
	oh.set_health(50, 100)
	_ok(oh.buff_icons_per_row() > 0, "显数字时每行图标数 > 0",
		str(oh.buff_icons_per_row()))
	_ok(oh.bar_width() == oh.buff_icons_per_row() * 10 + 2,
		"条宽 = 图标数*10 + 2", str(oh.bar_width()))
	_ok(oh.bar_width_clip() == oh.bar_width() / 2,
		"半血时裁剪宽 = 一半", "%d/%d" % [oh.bar_width_clip(), oh.bar_width()])
	_ok(oh.bar_tex_empty() == 0x00000016, "显数字用 0x16 空条")

	oh.cfg_show_number = false
	_ok(oh.buff_icons_per_row() == 3, "不显数字时每行固定 3")
	_ok(oh.bar_tex_empty() == 0x00000014, "不显数字用 0x14 空条")

	# 摊位时不显血条
	oh.cfg_show_number = true
	oh.cfg_show_bar = true
	oh.stall_name = "某某的摊位"
	_ok(not oh.show_bar(), "摆摊中不显血条")
	_ok(oh.show_stall(), "show_stall 为真")
	oh.stall_name = ""
	_ok(oh.show_bar(), "取消摆摊后显血条")

	# 满血隐藏
	oh.cfg_show_bar = false
	oh.cfg_hide_full_bar = true
	oh.set_health(100, 100)
	_ok(not oh.show_bar(), "满血且开启隐藏时不显")
	oh.set_health(99, 100)
	_ok(oh.show_bar(), "掉血后显示")

	# 喊话与超时
	oh.add_say("你好")
	_ok(oh.says.size() == 1, "喊话入队")
	oh.update(6.0)
	_ok(oh.says.size() == 0, "6 秒后过期清除")

	# 摊位列宽：max(name_w, 60) + 10
	_ok(StallBarRef.name_space_width(30) == 60, "短名字取最小 60")
	_ok(StallBarRef.name_space_width(80) == 80, "长名字取实际宽")
	_ok(StallBarRef.bar_width(30) == 70, "招牌总宽 = 60+10")

	# 脚下板：空行要跳过
	var lines := UnderBodyRef.build_lines("张三", "行会名", "", "", [])
	_ok(lines.size() == 2, "名字+行会共 2 行", str(lines.size()))
	var lines2 := UnderBodyRef.build_lines("张三", "行会", "长老", "", ["称号A"])
	_ok(lines2.size() == 3, "名字+行会[头衔]+称号 = 3 行", str(lines2.size()))
	_ok(str(lines2[1]["text"]) == "行会[长老]", "行会带头衔格式", str(lines2[1]["text"]))
	var lines3 := UnderBodyRef.build_lines("", "", "", "", [])
	_ok(lines3.size() == 0, "全空时 0 行")


# ==================== C7-3 交易类面板 ====================

func _test_trade_boards() -> void:
	print("\n[C7-3] 交易类面板")
	# 直接交易
	var dt := DirectTradeRef.new()
	_ok(dt.w == 494 and dt.h == 330, "494x330（0x2010 尺寸）")
	_ok(DirectTradeRef.ksep(1234567) == "1,234,567", "千分位 1234567",
		DirectTradeRef.ksep(1234567))
	_ok(DirectTradeRef.ksep(0) == "0", "千分位 0")
	_ok(DirectTradeRef.ksep(999) == "999", "千分位 999")
	_ok(DirectTradeRef.ksep(1000) == "1,000", "千分位 1000")
	# 金钱解析
	_ok(DirectTradeRef.parse_gold("123") == 123, "解析 123")
	_ok(DirectTradeRef.parse_gold("12a") == null, "含字母返回 null")
	_ok(DirectTradeRef.parse_gold("") == null, "空返回 null")
	_ok(DirectTradeRef.parse_gold("  45 ") == 45, "trim 后解析")
	# 状态机
	dt.begin_trade(1001, "对方")
	_ok(dt.show, "开店后显示")
	_ok(dt.peer_state_text() == DirectTradeRef.STATE_EDIT, "对方初始编辑中")
	_ok(dt.my_state_text() == DirectTradeRef.STATE_EDIT, "自己初始编辑中")
	dt._on_trade_button()
	_ok(dt.lock_pending, "点锁定后 pending")
	_ok(dt.my_state_text() == DirectTradeRef.STATE_LOCKING, "显示锁定中")
	dt.apply_lock_ack()
	_ok(dt.locked and not dt.lock_pending, "回执后已锁定")
	_ok(dt.my_state_text() == DirectTradeRef.STATE_LOCKED, "显示已锁定")
	dt._on_trade_button()
	_ok(dt.confirmed, "再点一次确认")
	_ok(dt.my_state_text() == DirectTradeRef.STATE_CONFIRM, "显示已确认")

	# 摆摊
	var sb := StallRef.new()
	_ok(sb.w == 282 and sb.h == 390, "282x390（0x230 尺寸）")
	sb.my_db_id = 7
	sb.load_stall({"dbid": 1, "name": "摊", "seller_uid": 7, "selling": false,
		"item_list": []}, "我", 7)
	_ok(sb.is_seller(), "自己是摊主")
	_ok(not sb.show_item_op(), "无选中时不显示操作")
	_ok(not sb.show_start_stall(), "无物品时不能开摊")
	sb.stall["item_list"] = [{"item_id": 1, "count": 2, "price": 100},
		{"item_id": 2, "count": 1, "price": 50}]
	_ok(sb.total_value() == 250, "总值 = 2*100 + 1*50 = 250", str(sb.total_value()))
	_ok(sb.show_start_stall(), "有物品可开摊")
	sb.selected = 0
	_ok(sb.show_item_op(), "选中后显示取出")
	_ok(not sb.show_start_stall(), "选中时不显示开摊")
	_ok(sb.selected_price() == 100, "选中单价 100")
	sb.stall["selling"] = true
	sb.selected = -1
	_ok(sb.show_close_stall(), "在售无选中显示收摊")

	# 购买
	var pb := PurchaseRef.new()
	_ok(pb.w == 290, "主区宽 290（0x8000000 尺寸）")
	pb.load_sell(9, [])
	for i in range(30):
		pb.items.append({"item_id": i + 1, "count": 1, "price": 10 * (i + 1)})
	_ok(pb.page_count() == 2, "30 项 / 每页 24 = 2 页", str(pb.page_count()))
	_ok(pb.page_items().size() == 24, "第 1 页 24 项")
	pb.next_page()
	_ok(pb.page_items().size() == 6, "第 2 页 6 项", str(pb.page_items().size()))
	_ok(pb.board_tex_id() == 0x08000000, "扩展 0 用 0x8000000")

	# 拍卖
	var ab := AuctionRef.new()
	_ok(ab.w == 720 and ab.h == 440, "720x440（0x1400 尺寸）")
	_ok(AuctionRef.ROW_W == 460, "行宽 = 108+140+70+142 = 460")
	_ok(AuctionRef.COL_C_ITEM == 54, "物品列中心 54")
	_ok(AuctionRef.COL_C_SELLER == 178, "卖家列中心 108+70=178")
	_ok(AuctionRef.COL_C_TIME == 283, "时间列中心 108+140+35=283")
	_ok(AuctionRef.COL_C_PRICE == 399, "价格列中心 108+140+70+71=399")
	_ok(AuctionRef.PAGE_SIZE == 13, "每页 13 行（m_rowCount）")
	_ok(AuctionRef.fmt_remain(86400) == "1d 0h", "剩余 1 天", AuctionRef.fmt_remain(86400))
	_ok(AuctionRef.fmt_remain(3600) == "1h 0m", "剩余 1 小时", AuctionRef.fmt_remain(3600))
	_ok(AuctionRef.fmt_remain(0) == "已过期", "0 秒已过期")
	# 分页与撤拍权限
	ab.my_db_id = 5
	ab.set_entries([
		{"item_id": 1, "seller_id": 5, "price": 10, "remain_sec": 100},
		{"item_id": 2, "seller_id": 9, "price": 20, "remain_sec": 200},
	], 5)
	_ok(ab.page_count() == 1, "2 条 1 页")
	ab.selected = 0
	_ok(ab.can_cancel(), "自己的可以撤拍")
	ab.selected = 1
	_ok(not ab.can_cancel(), "别人的不能撤拍")


# ==================== C7-4 社交与杂项 ====================

func _test_social_boards() -> void:
	print("\n[C7-4] 社交与杂项")
	# 好友聊天
	var fc := FriendChatRef.new()
	_ok(fc.w == 420 and fc.h == 422, "初始 400+10+10 x 400+10+12",
		"%dx%d" % [fc.w, fc.h])
	_ok(FriendChatRef.PAGE_NAMES.size() == 5, "5 个页面")
	_ok(FriendChatRef.AVATAR_H == 39, "头像高 35*94/84 = 39")
	_ok(FriendChatRef.PAGE_CHATPREVIEW == 1, "默认页是会话预览")
	fc.set_ui_page(FriendChatRef.PAGE_FRIENDLIST)
	_ok(fc.cur_page == 2, "切到好友列表")
	fc.open_chat("p1", "张三")
	_ok(fc.cur_page == FriendChatRef.PAGE_CHAT, "开聊切到聊天页")
	_ok(fc.cur_peer == "p1", "当前会话已设")
	fc.add_message(true, "我", "你好")
	fc.add_message(false, "张三", "在")
	_ok(fc.messages.size() == 2, "两条消息")
	_ok(fc.unread_total() == 0, "未读总数 0（空列表）")
	# 标题中心
	_ok(fc.title_cx() == 45 + (420 - 45 - 190) / 2, "标题中心 x 公式",
		str(fc.title_cx()))

	# 任务
	var qb := QuestRef.new()
	_ok(qb.w == 363 and qb.h == 446, "363x446（0x350 尺寸）")
	_ok(QuestRef.DESP_X == 40 and QuestRef.DESP_Y == 100, "描述区 (40,100)")
	_ok(QuestRef.DESP_W == 270, "描述区宽 270")
	qb.set_quests([
		{"id": "q1", "title": "任务一", "xml": "<layout><par>去打怪</par></layout>"},
	])
	_ok(qb.quests.size() == 1, "1 条任务")
	var q0: Dictionary = qb.quests[0]
	_ok(not bool(q0["folded"]), "默认展开")
	_ok((q0["lines"] as Array).size() > 1, "展开时有多行", str((q0["lines"] as Array).size()))
	var n_before: int = (q0["lines"] as Array).size()
	q0["folded"] = true
	qb._rebuild_lines(q0)
	_ok((q0["lines"] as Array).size() == 1, "折叠后只剩标题",
		str((q0["lines"] as Array).size()))
	_ok(n_before > 1, "折叠前确实多于 1 行")
	# xml 标签剥离
	_ok(QuestRef._strip_tags("<a>你好</a>") == "你好", "剥离 xml 标签")

	# 马匹
	var hb := HorseRef.new()
	_ok(hb.w == 257 and hb.h == 322, "257x322（0x700 尺寸）")
	_ok(hb._is_enabled("mount"), "未骑马时上马可用")
	_ok(not hb._is_enabled("dismount"), "未骑马时下马禁用")
	hb.riding = true
	_ok(hb._is_enabled("dismount"), "骑马后下马可用")
	_ok(not hb._is_enabled("mount"), "骑马后上马禁用")

	# 迷你地图
	var mm := MiniMapRef.new()
	_ok(mm.w == 200 and mm.h == 200, "普通模式 200x200")
	mm.set_map(0x09000000, 100, 100)
	_ok(mm.calc != null, "坐标换算器已实例化")
	var p0 := mm.place(1280, 720, 0, 0)
	_ok(int(p0.x) == 1280 - 1 - 200 and int(p0.y) == 0,
		"普通模式贴右上 (W-1-200, 0)", str(p0))
	mm.toggle_extended(1280, 720)
	_ok(mm.w == int(1280 * 0.8) and mm.h == int(720 * 0.5), "扩展 1024x360",
		"%dx%d" % [mm.w, mm.h])
	var p1 := mm.place(1280, 720, 0, 0)
	_ok(int(p1.x) == 1280 / 2 - mm.w / 2, "扩展模式居中", str(p1))

	# 输入串
	var ib := InputRef.new()
	_ok(ib.w == 358 and ib.h == 268, "358x268（0x7000000 尺寸）")
	ib.wait_input("改名", "默认", true)
	_ok(ib.mode == InputRef.MODE_INPUT, "输入模式")
	ib.type_text("abc")
	_ok(ib.input_text == "默认abc", "追加文本", ib.input_text)
	ib.backspace()
	_ok(ib.input_text == "默认ab", "退格", ib.input_text)
	ib.wait_confirm("确定要走？")
	_ok(ib.mode == InputRef.MODE_CONFIRM, "确认模式")
	_ok(ib.choices.size() == 2, "确认有两个选项")
	ib.wait_choice("选一个", ["A", "B", "C"])
	_ok(ib.choices.size() == 3, "三选一")

	# 菜单
	var menu := MenuRef.new()
	menu.add_item("攻击")
	menu.add_item("交易")
	menu.add_item("加好友", true, false, true)   # 带分隔
	_ok(menu.count() == 3, "3 个菜单项")
	var sz := menu.board_size()
	_ok(sz.x >= 90, "菜单宽度至少 90", str(sz.x))
	_ok(sz.y > 3 * 20, "菜单高度包含 3 行+分隔", str(sz.y))
	# 箭头帧
	_ok(MenuRef.arrow_frame(false, false) == 0, "箭头关=帧0")
	_ok(MenuRef.arrow_frame(true, false) == 1, "箭头开=帧1")
	_ok(MenuRef.arrow_frame(true, true) == 2, "箭头按下=帧2")


# ==================== C8 音频 ====================

func _test_audio() -> void:
	print("\n[C8] 音频系统")

	# 怪物音效基址（SYS_MONSEFFBASE）
	_ok(AudioRef.mon_seff_base(0) == 0x02000000, "lookID 0 基址 0x02000000",
		_hex(AudioRef.mon_seff_base(0)))
	_ok(AudioRef.mon_seff_base(1) == 0x02000010, "lookID 1 基址 0x02000010",
		_hex(AudioRef.mon_seff_base(1)))
	_ok(AudioRef.mon_seff(5, AudioRef.MONSEFF_ATTACK) == 0x02000000 + 5 * 16 + 2,
		"lookID 5 攻击音", _hex(AudioRef.mon_seff(5, 2)))
	_ok(AudioRef.SYS_SEFFSIZE == 16, "每块 16 个（SYS_SEFFSIZE）")
	_ok(AudioRef.MONSEFF_SPAWN == 0, "出现 = 0")
	_ok(AudioRef.MONSEFF_STAND == 1, "站立 = 1")
	_ok(AudioRef.MONSEFF_ATTACK == 2, "攻击 = 2")
	_ok(AudioRef.MONSEFF_HITTED == 4, "受击 = 4")
	_ok(AudioRef.MONSEFF_DIE == 5, "死亡 = 5")

	# 魔法音效（DBCOM_MAGICGFXSEFFID）
	_ok(AudioRef.magic_seff(13, 0) == 0x04000000 + 13 * 16,
		"seffBase 13 启动帧", _hex(AudioRef.magic_seff(13, 0)))
	_ok(AudioRef.magic_seff(13, 1) == 0x04000000 + 13 * 16 + 1,
		"seffBase 13 运行帧", _hex(AudioRef.magic_seff(13, 1)))

	# 脚步音效（hero.cpp:323-326）
	_ok(AudioRef.step_seff(22, 1) == 0x01000001, "走路 frame1 = 0x01000001",
		_hex(AudioRef.step_seff(22, 1)))
	_ok(AudioRef.step_seff(22, 4) == 0x01000002, "走路 frame4 = 0x01000002",
		_hex(AudioRef.step_seff(22, 4)))
	_ok(AudioRef.step_seff(22, 2) == -1, "走路 frame2 无声")
	_ok(AudioRef.step_seff(23, 1) == 0x01000003, "跑步 frame1 = 0x01000003")
	_ok(AudioRef.step_seff(31, 1) == 0x01000021, "马走 frame1 = 0x01000021",
		_hex(AudioRef.step_seff(31, 1)))
	_ok(AudioRef.step_seff(32, 1) == 0x01000023, "马跑 frame1 = 0x01000023",
		_hex(AudioRef.step_seff(32, 1)))
	_ok(AudioRef.step_seff(99, 1) == -1, "非移动动作无声")

	# 挥击音效（按武器 category）
	_ok(AudioRef.attack_seff("匕首") == 0x01010000 + 50, "匕首 50")
	_ok(AudioRef.attack_seff("木剑") == 0x01010000 + 51, "木剑 51")
	_ok(AudioRef.attack_seff("剑") == 0x01010000 + 52, "剑 52")
	_ok(AudioRef.attack_seff("刀") == 0x01010000 + 53, "刀 53")
	_ok(AudioRef.attack_seff("斧") == 0x01010000 + 54, "斧 54")
	_ok(AudioRef.attack_seff("锏") == 0x01010000 + 55, "锏 55")
	_ok(AudioRef.attack_seff("棍") == 0x01010000 + 56, "棍 56")
	_ok(AudioRef.attack_seff("") == 0x01010000 + 57, "空手 57")
	_ok(AudioRef.attack_seff("弓") == 0x01010000 + 57, "未列出的也用空手 57")

	# 受击音效
	_ok(AudioRef.struck_voice_seff(0) == 0x01030000 + 139, "男声 139",
		_hex(AudioRef.struck_voice_seff(0)))
	_ok(AudioRef.struck_voice_seff(1) == 0x01030000 + 138, "女声 138",
		_hex(AudioRef.struck_voice_seff(1)))
	_ok(AudioRef.struck_body_seff(false) == 0x01010000 + 73, "无甲 73")
	_ok(AudioRef.struck_body_seff(true) == 0x01010000 + 83, "有甲 83")
	_ok(AudioRef.MON_HITTED_BY == 0x01010000 + 61, "怪被击用木剑 61")

	# 装备点击音（hero.cpp:1575-1583）
	_ok(AudioRef.equip_click_seff(3) == 0x01020000 + 111, "武器 111")
	_ok(AudioRef.equip_click_seff(1) == 0x01020000 + 112, "衣服 112")
	_ok(AudioRef.equip_click_seff(8) == 0x01020000 + 113, "戒指 113")
	_ok(AudioRef.equip_click_seff(6) == 0x01020000 + 114, "手镯 114")
	_ok(AudioRef.equip_click_seff(5) == 0x01020000 + 115, "项链 115")
	_ok(AudioRef.equip_click_seff(2) == 0x01020000 + 116, "头盔 116")
	_ok(AudioRef.equip_click_seff(4) == 0x01020000 + 118, "其它 118")

	# 3D 定位（processrun.cpp:3063-3074）
	var sp0 := AudioRef.spatial_param(0, 0)
	_ok(int(sp0["distance"]) == 0 and int(sp0["angle"]) == 0, "原地 distance=0,angle=0")
	var sp_e := AudioRef.spatial_param(10, 0)
	_ok(int(sp_e["distance"]) == 10, "正东 distance=10", str(sp_e["distance"]))
	_ok(int(sp_e["angle"]) == 90, "正东 angle=90", str(sp_e["angle"]))
	var sp_w := AudioRef.spatial_param(-10, 0)
	_ok(int(sp_w["angle"]) == 270, "正西 angle=270", str(sp_w["angle"]))
	var sp_s := AudioRef.spatial_param(0, 10)
	_ok(int(sp_s["angle"]) == 180, "正南 angle=180", str(sp_s["angle"]))
	var sp_n := AudioRef.spatial_param(0, -10)
	_ok(int(sp_n["angle"]) == 0, "正北 angle=0", str(sp_n["angle"]))
	# 斜向：3-4-5 直角三角形
	var sp_d := AudioRef.spatial_param(3, 4)
	_ok(int(sp_d["distance"]) == 5, "3,4 -> distance 5（欧氏，非 max）",
		str(sp_d["distance"]))
	# 远距离 clamp 到 255
	var sp_far := AudioRef.spatial_param(1000, 0)
	_ok(int(sp_far["distance"]) == 255, "超远 clamp 255", str(sp_far["distance"]))
	# 角度归一化：负数角度要落到 [0,360)
	var sp_neg := AudioRef.spatial_param(-1, -1)
	var ang_neg := int(sp_neg["angle"])
	_ok(ang_neg >= 0 and ang_neg < 360, "角度归一到 [0,360)", str(ang_neg))

	# 3D 位置向量（apply3DPosition）
	var v_n := AudioRef.spatial_pos(255, 0)
	_ok(absf(v_n.x) < 0.01 and absf(v_n.z + 1.0) < 0.01, "北 = (0,0,-1)", str(v_n))
	var v_e := AudioRef.spatial_pos(255, 90)
	_ok(absf(v_e.x - 1.0) < 0.01 and absf(v_e.z) < 0.01, "东 = (1,0,0)", str(v_e))
	var v_s := AudioRef.spatial_pos(255, 180)
	_ok(absf(v_s.z - 1.0) < 0.01, "南 = (0,0,1)", str(v_s))
	var v_w := AudioRef.spatial_pos(255, 270)
	_ok(absf(v_w.x + 1.0) < 0.01, "西 = (-1,0,0)", str(v_w))

	# 素材可达性：抽查几个关键音效文件
	var root := OS.get_environment("MIR2X_ASSETS")
	if root.is_empty():
		root = "E:/Desktop/game/mir2x_assets-main"
	_ok(DirAccess.dir_exists_absolute(root + "/sound/seff"), "seff 目录存在")
	_ok(DirAccess.dir_exists_absolute(root + "/sound/bgm"), "bgm 目录存在")
	# 人形脚步 0x01000001
	var p_step := "%s/sound/seff/01000001.WAV" % root
	_ok(FileAccess.file_exists(p_step), "脚步音效文件 01000001.WAV 存在", p_step)
	# 怪物音效 0x02000000
	# 怪物段 647 个文件，最小 02000001（lookID 0 + offset 1 才是站立音）
	var p_mon := "%s/sound/seff/02000001.WAV" % root
	_ok(FileAccess.file_exists(p_mon), "怪物音效 02000001.WAV 存在", p_mon)
	# BGM 00040007（登录）
	var d := DirAccess.open(root + "/sound/bgm")
	var found_bgm := false
	if d != null:
		d.list_dir_begin()
		var fn := d.get_next()
		while not fn.is_empty():
			if fn.begins_with("00040007"):
				found_bgm = true
			fn = d.get_next()
		d.list_dir_end()
	_ok(found_bgm, "登录 BGM 00040007 存在")

	# 统计数量
	var seff_count := _count_files(root + "/sound/seff")
	_ok(seff_count == 848, "音效文件 848 个", str(seff_count))
	var bgm_count := _count_files(root + "/sound/bgm")
	_ok(bgm_count == 25, "BGM 文件 25 首", str(bgm_count))


func _hex(v) -> String:
	return "0x%08X" % int(v)


func _count_files(dir_path: String) -> int:
	var d := DirAccess.open(dir_path)
	if d == null:
		return 0
	var n := 0
	d.list_dir_begin()
	var fn := d.get_next()
	while not fn.is_empty():
		if not d.current_is_dir():
			n += 1
		fn = d.get_next()
	d.list_dir_end()
	return n
