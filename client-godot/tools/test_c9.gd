extends SceneTree
## C9 自检：登录链路（Logo / 登录 / 选人 / 建号 / 账号 / 改密）
##
## 静态测试（--script 模式，Autoload 与 class_name 不可用，全走 preload）

const LoginRef := preload("res://scripts/ui/login/ui_login_process.gd")
const SelectRef := preload("res://scripts/ui/login/ui_select_char_process.gd")
const CreateRef := preload("res://scripts/ui/login/ui_create_char_process.gd")
const AccountRef := preload("res://scripts/ui/login/ui_account_process.gd")
const FlowRef := preload("res://scripts/ui/login/login_flow.gd")
const SelectGfxRef := preload("res://scripts/data/select_char_gfx.gd")
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
	print("=== C9 自检 ===")
	ProgUseRef.setup()
	SelectGfxRef.setup()
	_test_logo()
	_test_login()
	_test_select_gfx()
	_test_select_process()
	_test_create_char()
	_test_account()
	_test_flow()
	_test_apng_plte()
	print("\n=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0 if _fail == 0 else 1)


# ==================== Logo（processlogo）====================

func _test_logo() -> void:
	print("\n[C9-1] Logo 淡入淡出")
	var lg := LoginRef.new()
	_ok(lg.phase == 0, "初始在 logo 阶段")
	_ok(is_equal_approx(LoginRef.LOGO_TIME_R1, 0.3), "淡入占 0.3")
	_ok(is_equal_approx(LoginRef.LOGO_TIME_R2, 0.3), "全亮占 0.3")
	_ok(is_equal_approx(LoginRef.LOGO_FULL_MS, 5000.0), "总时长 5000ms")

	# colorRatio 三段
	lg.logo_time = 0.0
	_ok(is_equal_approx(lg.logo_color_ratio(), 0.0), "t=0 全黑")
	lg.logo_time = 5000.0 * 0.15          # r = 0.15, r1 = 0.3
	_ok(absf(lg.logo_color_ratio() - 0.5) < 0.01, "淡入中点 = 0.5",
		"%.3f" % lg.logo_color_ratio())
	lg.logo_time = 5000.0 * 0.45          # r = 0.45，在第一段+第二段之间
	_ok(is_equal_approx(lg.logo_color_ratio(), 1.0), "全亮区间 = 1.0")
	lg.logo_time = 5000.0 * 0.95
	var last: float = lg.logo_color_ratio()
	_ok(last > 0.0 and last < 0.3, "淡出末段接近 0", "%.3f" % last)

	# 超时自动跳过
	lg.reset()
	lg.update(5.1)
	_ok(lg.phase == 1, "超过 5s 自动进入登录")
	# 手动跳过
	lg.reset()
	lg.skip_logo()
	_ok(lg.phase == 1, "skip_logo 立即进入登录")

	# logo 贴图存在
	var t := ProgUseRef.retrieve(LoginRef.LOGO_TEX)
	_ok(t != null, "logo 贴图 0x0 存在")
	if t != null:
		_ok(t.get_width() == 800 and t.get_height() == 600, "logo 800x600",
			"%dx%d" % [t.get_width(), t.get_height()])


# ==================== 登录（processlogin）====================

func _test_login() -> void:
	print("\n[C9-2] 登录界面")
	var lg := LoginRef.new()
	lg.phase = 1

	# 布局常量
	_ok(LoginRef.CANVAS_W == 800 and LoginRef.CANVAS_H == 600, "逻辑画布 800x600")
	_ok(LoginRef.BTN_CREATE_X == 150, "创建账号 x=150")
	_ok(LoginRef.BTN_CHPWD_X == 352, "修改密码 x=352")
	_ok(LoginRef.BTN_EXIT_X == 554, "退出 x=554")
	_ok(LoginRef.BTN_LOGIN_X == 600 and LoginRef.BTN_LOGIN_Y == 536, "登录 (600,536)")
	_ok(LoginRef.ID_X == 159 and LoginRef.ID_Y == 540, "账号框 (159,540)")
	_ok(LoginRef.PWD_X == 409 and LoginRef.PWD_Y == 540, "密码框 (409,540)")
	_ok(LoginRef.ID_W == 146 and LoginRef.ID_H == 18, "输入框 146x18")
	_ok(LoginRef.INPUT_FONT_SIZE == 18, "输入字号 18")

	# 贴图
	var checks := [
		[LoginRef.TEX_BG_UPPER, 800, 450, "上部背景"],
		[LoginRef.TEX_BG_LOWER, 800, 60, "下部背景"],
		[LoginRef.TEX_SEP, 456, 26, "分隔条"],
		[LoginRef.BTN_LOGIN, 96, 24, "登录按钮"],
		[LoginRef.BTN_CREATE_ACCOUNT, 96, 26, "创建账号按钮"],
	]
	for c in checks:
		var t := ProgUseRef.retrieve(int(c[0]))
		_ok(t != null and t.get_width() == int(c[1]) and t.get_height() == int(c[2]),
			"%s %dx%d" % [c[3], int(c[1]), int(c[2])])

	# 空账号/密码 -> 提示不发送
	var sent := []
	lg.on_send = func(c, b): sent.append([c, b.size()])
	lg.account = ""
	lg.password = ""
	lg.do_login()
	_ok(sent.is_empty(), "空输入不发包")
	_ok(lg.notify.size() == 1, "空输入给提示")

	# 正常登录 -> CMLogin 136 字节
	lg.account = "test@test.com"
	lg.password = "123456"
	lg.do_login()
	_ok(sent.size() == 1, "有效输入发一包")
	if sent.size() == 1:
		_ok(int(sent[0][0]) == 2, "消息码 CM_LOGIN=2")
		_ok(int(sent[0][1]) == LoginRef.CMLOGIN_SIZE, "包体 136 字节",
			str(sent[0][1]))

	# 包体内容校验
	var body := PackedByteArray()
	body.resize(136)
	body.fill(0)
	LoginRef._put_static_buf(body, 0, "test@test.com")
	LoginRef._put_static_buf(body, 69, "123456")
	_ok(body.decode_u16(0) == 13, "账号长度 13", str(body.decode_u16(0)))
	_ok(body.decode_u16(69) == 6, "密码长度 6", str(body.decode_u16(69)))
	_ok(LoginRef._get_static_buf(body, 0) == "test@test.com", "账号回读一致")
	_ok(LoginRef._get_static_buf(body, 69) == "123456", "密码回读一致")

	# Tab 切焦点
	lg.focus_field = 0
	lg.handle_key(KEY_TAB, 0)
	_ok(lg.focus_field == 1, "Tab 从账号切到密码")
	lg.handle_key(KEY_TAB, 0)
	_ok(lg.focus_field == 0, "Tab 再切回账号")

	# 输入与退格
	lg.focus_field = 0
	lg.account = ""
	lg.handle_key(0, "a".unicode_at(0))
	lg.handle_key(0, "b".unicode_at(0))
	_ok(lg.account == "ab", "输入 ab", lg.account)
	lg.handle_key(KEY_BACKSPACE, 0)
	_ok(lg.account == "a", "退格后剩 a", lg.account)

	# 错误码文案
	_ok(LoginRef.login_error_text(0) == "账号不存在", "错误码 0")
	_ok(LoginRef.login_error_text(1) == "密码错误", "错误码 1")
	_ok(LoginRef.login_error_text(2) == "账号已在线", "错误码 2")
	_ok(LoginRef.login_error_text(99).contains("99"), "未知错误码带数字")

	# 通知上限
	var lg2 := LoginRef.new()
	for i in range(15):
		lg2.add_notify("x%d" % i)
	_ok(lg2.notify.size() == 10, "通知最多 10 条", str(lg2.notify.size()))


# ==================== 选人图库 ====================

func _test_select_gfx() -> void:
	print("\n[C9-3] 选人图库与帧表")

	# 键编码（gender 位实测校准：1=男 0=女）
	# gender 位：1=男 0=女（实测校准，见 select_char_gfx.gd 注释）
	_ok(SelectGfxRef.make_key(0, true, 0) == 0x00000800, "战士男 anim0 键=0x800",
		"%08X" % SelectGfxRef.make_key(0, true, 0))
	_ok(SelectGfxRef.make_key(0, false, 0) == 0, "战士女 anim0 键=0",
		"%08X" % SelectGfxRef.make_key(0, false, 0))
	_ok(SelectGfxRef.make_key(0, true, 1) == 0x00000808, "战士男 anim1 键=0x808",
		"%08X" % SelectGfxRef.make_key(0, true, 1))
	_ok(SelectGfxRef.make_key(1, true, 4) == 0x00001820, "道士男 anim4 键=0x1820",
		"%08X" % SelectGfxRef.make_key(1, true, 4))
	_ok(SelectGfxRef.make_key(2, false, 2) == 0x00002010, "法师女 anim2 键=0x2010",
		"%08X" % SelectGfxRef.make_key(2, false, 2))

	# 键解码往返
	var k := SelectGfxRef.make_key(2, false, 3, true, 1)
	var d := SelectGfxRef.decode_key(k)
	_ok(int(d["anim"]) == 3, "解码 anim=3")
	_ok(int(d["layer"]) == 1, "解码 layer=1")
	_ok(int(d["shadow"]) == 1, "解码 shadow=1")
	_ok(int(d["gender"]) == 0, "解码 gender=0（女）")
	_ok(int(d["job"]) == 2, "解码 job=2（法师）")

	# 帧表 30 条
	_ok(SelectGfxRef.FRAME_TABLE.size() == 30, "帧表 30 条",
		str(SelectGfxRef.FRAME_TABLE.size()))
	# 抽查（战士男/女差异最大的一组）
	_ok(SelectGfxRef.frame_count(1, true, 1) == 20, "战士男 anim1 = 20 帧")
	_ok(SelectGfxRef.frame_count(1, false, 1) == 11, "战士女 anim1 = 11 帧")
	_ok(SelectGfxRef.frame_count(1, true, 3) == 8, "战士男 anim3 = 8 帧")
	_ok(SelectGfxRef.frame_count(1, false, 3) == 12, "战士女 anim3 = 12 帧")
	_ok(SelectGfxRef.frame_count(2, true, 2) == 20, "道士男 anim2 = 20 帧")
	_ok(SelectGfxRef.frame_count(4, true, 4) == 15, "法师男 anim4 = 15 帧")
	_ok(SelectGfxRef.frame_count(4, false, 4) == 17, "法师女 anim4 = 17 帧")
	_ok(SelectGfxRef.frame_count(0, true, 0) == 0, "非法职业返回 0")

	# jobf
	_ok(SelectGfxRef.job_valid(1), "战士有效")
	_ok(SelectGfxRef.job_valid(0) == false, "0 无效")
	_ok(SelectGfxRef.first_job(4) == 4, "firstJob(法师)=4")
	_ok(SelectGfxRef.job_gfx_index(1) == 0, "战士 gfxIndex=0")
	_ok(SelectGfxRef.job_gfx_index(2) == 1, "道士 gfxIndex=1")
	_ok(SelectGfxRef.job_gfx_index(4) == 2, "法师 gfxIndex=2")
	_ok(SelectGfxRef.job_name(1) == "战士", "jobName(1)=战士")
	_ok(SelectGfxRef.job_name(2) == "道士", "jobName(2)=道士")
	_ok(SelectGfxRef.job_name(4) == "法师", "jobName(4)=法师")

	# 音效（0x00010000 | gender<<4 | job<<8 | flag）
	_ok(SelectGfxRef.anim_seff(1, true, true) == 0x00010001,
		"战士男选人音 = 0x10001", "%08X" % SelectGfxRef.anim_seff(1, true, true))
	_ok(SelectGfxRef.anim_seff(1, false, true) == 0x00010011,
		"战士女选人音 = 0x10011", "%08X" % SelectGfxRef.anim_seff(1, false, true))
	_ok(SelectGfxRef.anim_seff(4, true, false) == 0x00010200,
		"法师男建号音 = 0x10200", "%08X" % SelectGfxRef.anim_seff(4, true, false))
	_ok(SelectGfxRef.anim_seff(2, true, false) == 0x00010100,
		"道士男建号音 = 0x10100", "%08X" % SelectGfxRef.anim_seff(2, true, false))

	# 图库实际加载：帧表与实载必须一一对应（这是发现 gender 位反了的那个检查）
	var mismatch := 0
	var checked := 0
	for job in range(3):
		for male in [true, false]:
			for anim in range(5):
				var n: int = SelectGfxRef.frame_count(
					[1, 2, 4][job], male, anim)
				var rec = SelectGfxRef.retrieve(job, male, anim, false, 0)
				if rec == null:
					mismatch += 1
					continue
				checked += 1
				var fc: int = (rec["frames"] as Array).size()
				if fc != n:
					mismatch += 1
					print("    [mismatch] job=%d male=%s anim=%d 帧表=%d 实载=%d"
						% [job, str(male), anim, n, fc])
	_ok(checked == 30, "30 组主图全部可加载", str(checked))
	_ok(mismatch == 0, "帧表与实载帧数一致", "%d 处不符" % mismatch)


# ==================== 选人进程 ====================

func _test_select_process() -> void:
	print("\n[C9-3] 选人进程")
	var sp := SelectRef.new()

	# 布局
	_ok(SelectRef.BTN_START_X == 335 and SelectRef.BTN_START_Y == 75, "开始 (335,75)")
	_ok(SelectRef.BTN_CREATE_X == 565 and SelectRef.BTN_CREATE_Y == 130, "创建 (565,130)")
	_ok(SelectRef.BTN_DELETE_X == 110 and SelectRef.BTN_DELETE_Y == 305, "删除 (110,305)")
	_ok(SelectRef.BTN_EXIT_X == 45 and SelectRef.BTN_EXIT_Y == 544, "退出 (45,544)")
	_ok(SelectRef.CHAR_DRAW_X == 430 and SelectRef.CHAR_DRAW_Y == 300, "角色锚点 (430,300)")
	_ok(SelectRef.INFO_X == 120 and SelectRef.INFO_Y_BASE == 260, "信息板 (120, 260-h)")
	_ok(is_equal_approx(SelectRef.FRAME_MS, 200.0), "每帧 200ms")

	# 背景贴图
	var bg := ProgUseRef.retrieve(SelectRef.TEX_BG)
	_ok(bg != null and bg.get_width() == 800 and bg.get_height() == 600,
		"背景 800x600")

	# 无角色时：开始/创建/删除都不显示（原版行为）
	_ok(not sp.has_char, "初始无角色")
	_ok(sp.frame_count() == 0, "无角色时帧数 0")
	sp.on_start()
	_ok(sp.notify.size() == 1, "无角色点开始给提示")

	# SM_QUERYCHAROK 解析（78 字节）
	var body := PackedByteArray()
	body.resize(78)
	body.fill(0)
	var nm := "测试角色".to_utf8_buffer()
	body.encode_u16(0, nm.size())
	for i in range(nm.size()):
		body[2 + i] = nm[i]
	# StaticBuffer<64> 占 68 字节（对齐），所以 gender 在 68、job 在 69
	body[68] = 1        # gender（1=男）
	body[69] = 1        # job（战士）
	body.encode_u64(70, 0)  # exp 在 70（job/gender 之后，别覆盖它们）
	sp.on_query_char_ok(body)
	_ok(sp.has_char, "收到角色后 has_char")
	_ok(sp.char_name == "测试角色", "角色名解析", sp.char_name)
	_ok(sp.char_gender_male, "性别男")
	_ok(sp.char_job == 1, "职业战士")
	_ok(sp.char_exp == 0, "经验 0")
	_ok(sp.frame_count() == 11, "战士男 anim0 = 11 帧", str(sp.frame_count()))

	# 动画状态机（0→1→2→3→0）
	sp.char_ani = 0
	sp.ani_time = 0.0
	sp.ani_switch_frame = -1
	# 强制走一遍（50% 概率那步可能不切，所以循环几次确定能推进）
	var advanced := false
	for i in range(20):
		sp.ani_time = 0.0
		sp.ani_switch_frame = -1
		var before := sp.char_ani
		sp.switch_char_gfx()
		if sp.char_ani != before:
			advanced = true
	_ok(advanced, "anim 状态能推进")
	# anim 2 -> 3 是 50%，anim 1 -> 2 必定
	sp.char_ani = 1
	sp.ani_time = 0.0
	sp.ani_switch_frame = -1
	sp.switch_char_gfx()
	_ok(sp.char_ani == 2, "anim1 必进 anim2", str(sp.char_ani))
	# anim 3 -> 0 必定
	sp.char_ani = 3
	sp.ani_time = 0.0
	sp.ani_switch_frame = -1
	sp.switch_char_gfx()
	_ok(sp.char_ani == 0, "anim3 必回 anim0", str(sp.char_ani))
	# 同一帧不重复切
	sp.char_ani = 0
	sp.ani_time = 0.0
	sp.ani_switch_frame = 0
	var keep: int = sp.char_ani
	sp.switch_char_gfx()
	_ok(sp.char_ani == keep, "同一 absFrame 不重复切")

	# 有角色时点创建 -> 提示只能一个角色
	sp.notify.clear()
	sp.on_create()
	_ok(sp.notify.size() == 1, "已有角色点创建给提示")
	_ok(str(sp.notify[0]["text"]).contains("只能创建一个"), "提示文案正确",
		str(sp.notify[0]["text"]))


# ==================== 建号 ====================

func _test_create_char() -> void:
	print("\n[C9-4] 建号进程")
	var cp := CreateRef.new()

	# 布局
	_ok(CreateRef.BTN_WARRIOR_X == 339, "战士钮 x=339")
	_ok(CreateRef.BTN_WIZARD_X == 381, "法师钮 x=381")
	_ok(CreateRef.BTN_TAOIST_X == 424, "道士钮 x=424")
	_ok(CreateRef.JOB_BTN_Y == 539, "职业钮 y=539")
	_ok(CreateRef.BTN_SUBMIT_X == 512 and CreateRef.ACTION_BTN_Y == 549,
		"提交 (512,549)")
	_ok(CreateRef.NAME_X == 355 and CreateRef.NAME_Y == 520, "名字框 (355,520)")
	_ok(CreateRef.NAME_W == 85 and CreateRef.NAME_H == 15, "名字框 85x15")
	_ok(CreateRef.MALE_DRAW_X == 193 and CreateRef.MALE_DRAW_Y == 215, "男预览 (193,215)")
	_ok(CreateRef.FEMALE_DRAW_X == 495 and CreateRef.FEMALE_DRAW_Y == 220,
		"女预览 (495,220)")
	_ok(CreateRef.FEMALE_WARRIOR_Y_FIX == 40, "女性战士 y 修正 +40")

	# 默认状态
	_ok(cp.job == 1, "默认战士")
	_ok(cp.active_gender_male, "默认男性")
	_ok(cp.frame_count(true) == 18, "战士男建号 = 18 帧", str(cp.frame_count(true)))
	_ok(cp.frame_count(false) == 16, "战士女建号 = 16 帧", str(cp.frame_count(false)))

	# 切职业
	cp.job = 4
	_ok(cp.frame_count(true) == 15, "法师男建号 = 15 帧", str(cp.frame_count(true)))
	cp.job = 2
	_ok(cp.frame_count(true) == 17, "道士男建号 = 17 帧", str(cp.frame_count(true)))
	cp.job = 1

	# 点男女预览区（矩形判定）
	cp.active_gender_male = true
	var male_r := Rect2(CreateRef.MALE_CLICK[0], CreateRef.MALE_CLICK[1],
		CreateRef.MALE_CLICK[2], CreateRef.MALE_CLICK[3])
	cp.handle_click(int(male_r.position.x) + 10, int(male_r.position.y) + 10)
	_ok(cp.active_gender_male, "点男区选男")
	var fe_r := Rect2(CreateRef.FEMALE_CLICK[0], CreateRef.FEMALE_CLICK[1],
		CreateRef.FEMALE_CLICK[2], CreateRef.FEMALE_CLICK[3])
	cp.handle_click(int(fe_r.position.x) + 10, int(fe_r.position.y) + 10)
	_ok(not cp.active_gender_male, "点女区选女")
	_ok(is_equal_approx(cp.ani_time, 0.0), "切换后 aniTime 归零")
	_ok(cp.last_start_abs_frame == 0xFFFFFFFF, "切换后强制重播音效")

	# 名字校验
	var sent := []
	cp.on_send = func(c, b): sent.append([c, b.size()])
	cp.name_text = ""
	cp.on_submit()
	_ok(sent.is_empty(), "空名字不发包")
	_ok(cp.notify.size() >= 1, "空名字给提示")

	# 正常提交 -> CMCreateChar 70 字节
	cp.name_text = "张三"
	cp.job = 4
	cp.active_gender_male = false
	cp.on_submit()
	_ok(sent.size() == 1, "有效名字发一包")
	if sent.size() == 1:
		_ok(int(sent[0][0]) == 4, "消息码 CM_CREATECHAR=4")
		_ok(int(sent[0][1]) == CreateRef.CMCREATECHAR_SIZE, "包体 70 字节",
			str(sent[0][1]))
	var b2 := PackedByteArray()
	b2.resize(70)
	b2.fill(0)
	var nb := "张三".to_utf8_buffer()
	b2.encode_u16(0, nb.size())
	for i in range(nb.size()):
		b2[2 + i] = nb[i]
	b2[68] = 4   # job（法师）
	b2[69] = 0   # gender（0=女）
	_ok(b2.decode_u16(0) == 6, "角色名 UTF-8 长度 6", str(b2.decode_u16(0)))
	_ok(b2[68] == 4, "职业字段 = 4（法师）")
	_ok(b2[69] == 0, "性别字段 = 0（女）")


# ==================== 账号 / 改密 ====================

func _test_account() -> void:
	print("\n[C9-5] 账号与改密")

	# 面板常量
	_ok(AccountRef.PANEL_X == 180 and AccountRef.PANEL_Y == 145, "面板 (180,145)")
	_ok(AccountRef.INPUT_W == 186 and AccountRef.INPUT_H == 28, "输入框 186x28")
	_ok(AccountRef.Y_CA_ID == 85, "创建账号 账号框 y+85")
	_ok(AccountRef.Y_CA_PWD == 143, "创建账号 密码框 y+143")
	_ok(AccountRef.Y_CA_PWD2 == 198, "创建账号 确认框 y+198")
	_ok(AccountRef.Y_CP_ID == 79, "改密 账号框 y+79")
	_ok(AccountRef.Y_CP_OLD == 126, "改密 旧密码 y+126")
	_ok(AccountRef.Y_CP_NEW == 173, "改密 新密码 y+173")
	_ok(AccountRef.Y_CP_NEW2 == 220, "改密 确认新密码 y+220")

	# 贴图
	var t1 := ProgUseRef.retrieve(AccountRef.TEX_CREATE_ACCOUNT)
	_ok(t1 != null and t1.get_width() == 440 and t1.get_height() == 310,
		"创建账号框 440x310")
	var t2 := ProgUseRef.retrieve(AccountRef.TEX_CHANGE_PWD)
	_ok(t2 != null and t2.get_width() == 440 and t2.get_height() == 331,
		"改密框 440x331")

	# idstrf::isEmail
	_ok(AccountRef.is_email("a@b.com"), "a@b.com 合法")
	_ok(AccountRef.is_email("test.user@mail.example.com"), "多点域名合法")
	_ok(AccountRef.is_email("hello@123.45.67.99"), "IP 域名合法")
	_ok(not AccountRef.is_email("abc"), "无 @ 不合法")
	_ok(not AccountRef.is_email("@b.com"), "空本地部分不合法")
	_ok(not AccountRef.is_email("a@"), "空域名不合法")
	_ok(not AccountRef.is_email("a@b"), "顶级域不足 2 位不合法")
	_ok(not AccountRef.is_email("a b@c.com"), "含空格不合法")
	_ok(not AccountRef.is_email("a@256.1.1.1"), "IP 段超 255 不合法")
	_ok(AccountRef.is_email("a@255.255.255.255"), "255 段合法")

	# idstrf::isPassword（>=8 且 4 类各 1 个）
	_ok(AccountRef.is_password("Abc123!@"), "Abc123!@ 合法")
	_ok(not AccountRef.is_password("abc123!@"), "缺大写不合法")
	_ok(not AccountRef.is_password("ABC123!@"), "缺小写不合法")
	_ok(not AccountRef.is_password("Abcdef!@"), "缺数字不合法")
	_ok(not AccountRef.is_password("Abc12345"), "缺特殊符不合法")
	_ok(not AccountRef.is_password("Ab1!"), "长度不足不合法")

	# idstrf::isCharName（> 6 字节）
	_ok(not AccountRef.is_char_name("abc"), "abc 太短")
	_ok(AccountRef.is_char_name("abcdefg"), "7 字符合法")

	# 提交校验流程
	var ap := AccountRef.new()
	ap.enter(AccountRef.MODE_CREATE)
	var sent := []
	ap.on_send = func(c, b): sent.append([c, b.size()])

	# 账号不合法
	ap.inputs = ["bad", "Abc123!@", "Abc123!@"]
	ap.do_submit()
	_ok(sent.is_empty(), "非法账号不发包")
	_ok(ap.info_text == "无效账号", "提示无效账号", ap.info_text)
	_ok(ap.inputs[0] == "", "全清输入")

	# 密码不合法
	ap.inputs = ["a@b.com", "weak", "weak"]
	ap.do_submit()
	_ok(ap.info_text == "无效密码", "提示无效密码", ap.info_text)
	_ok(ap.inputs[1] == "" and ap.inputs[2] == "", "清两个密码框")

	# 两次不一致
	ap.inputs = ["a@b.com", "Abc123!@", "Abc123!@x"]
	ap.do_submit()
	_ok(ap.info_text == "两次密码输入不一致", "提示不一致", ap.info_text)

	# 正常提交
	ap.inputs = ["a@b.com", "Abc123!@", "Abc123!@"]
	ap.do_submit()
	_ok(sent.size() == 1, "合法输入发一包")
	if sent.size() == 1:
		_ok(int(sent[0][0]) == 8, "消息码 CM_CREATEACCOUNT=8")
		_ok(ap.info_text == "提交中", "提示提交中")

	# 改密模式
	ap.enter(AccountRef.MODE_CHANGE)
	_ok(ap.inputs.size() == 4, "改密 4 个输入框", str(ap.inputs.size()))
	sent.clear()
	ap.inputs = ["a@b.com", "Old123!@", "New456!@", "New456!@"]
	ap.do_submit()
	_ok(sent.size() == 1, "改密发一包")
	if sent.size() == 1:
		_ok(int(sent[0][0]) == 10, "消息码 CM_CHANGEPASSWORD=10")
		_ok(int(sent[0][1]) == AccountRef.CMCHANGEPWD_SIZE, "包体 201 字节",
			str(sent[0][1]))

	# Tab 循环
	ap.enter(AccountRef.MODE_CREATE)
	_ok(ap.focus_index == 0, "初始焦点在账号")
	_ok(ap.inputs.size() == 3, "创建模式 3 个框", str(ap.inputs.size()))
	ap.handle_key(KEY_TAB, 0)
	_ok(ap.focus_index == 1, "Tab -> 1")
	ap.handle_key(KEY_TAB, 0)
	_ok(ap.focus_index == 2, "Tab -> 2")
	ap.handle_key(KEY_TAB, 0)
	_ok(ap.focus_index == 0, "第 3 次 Tab 回到 0（3 个框循环）")


# ==================== 进程管理器 ====================

func _test_flow() -> void:
	print("\n[C9-6] 进程管理器")
	# ProcessID 枚举（process.hpp，逐值）
	_ok(FlowRef.PID_NONE == 0, "PROCESSID_NONE=0")
	_ok(FlowRef.PID_LOGO == 1, "PROCESSID_LOGO=1")
	_ok(FlowRef.PID_SYRC == 2, "PROCESSID_SYRC=2")
	_ok(FlowRef.PID_LOGIN == 3, "PROCESSID_LOGIN=3")
	_ok(FlowRef.PID_CREATEACCOUNT == 4, "PROCESSID_CREATEACCOUNT=4")
	_ok(FlowRef.PID_SELECTCHAR == 5, "PROCESSID_SELECTCHAR=5")
	_ok(FlowRef.PID_CREATECHAR == 6, "PROCESSID_CREATECHAR=6")
	_ok(FlowRef.PID_CHANGEPASSWORD == 7, "PROCESSID_CHANGEPASSWORD=7")
	_ok(FlowRef.PID_RUN == 8, "PROCESSID_RUN=8")
	_ok(FlowRef.PID_CONNECTIONERROR == 9, "PROCESSID_CONNECTIONERROR=9")

	# 协议码（必须与 sm_table.gd 一致）
	_ok(FlowRef.CM_LOGIN == 2, "CM_LOGIN=2")
	_ok(FlowRef.CM_QUERYCHAR == 3, "CM_QUERYCHAR=3")
	_ok(FlowRef.CM_CREATECHAR == 4, "CM_CREATECHAR=4")
	_ok(FlowRef.CM_DELETECHAR == 5, "CM_DELETECHAR=5")
	_ok(FlowRef.CM_ONLINE == 7, "CM_ONLINE=7")
	_ok(FlowRef.CM_CREATEACCOUNT == 8, "CM_CREATEACCOUNT=8")
	_ok(FlowRef.CM_CHANGEPASSWORD == 10, "CM_CHANGEPASSWORD=10")
	_ok(FlowRef.SM_LOGINOK == 5, "SM_LOGINOK=5")
	_ok(FlowRef.SM_QUERYCHAROK == 11, "SM_QUERYCHAROK=11")
	_ok(FlowRef.SM_CREATECHAROK == 13, "SM_CREATECHAROK=13")
	_ok(FlowRef.SM_ONLINEOK == 20, "SM_ONLINEOK=20")

	# 选人/建号的协议尺寸
	_ok(SelectRef.CMDELETECHAR_SIZE == 68, "CM_DELETECHAR 68 字节")
	_ok(CreateRef.CMCREATECHAR_SIZE == 70, "CM_CREATECHAR 70 字节")
	_ok(LoginRef.CMLOGIN_SIZE == 136, "CM_LOGIN 136 字节")


# ==================== 附加：APNG 调色板修复回归 ====================

func _test_apng_plte() -> void:
	print("\n[C9-7] APNG 调色板修复")
	var A := load("res://scripts/data/apng_loader.gd")
	var root: String = SelectGfxRef.assets_root()
	# 取一张已知是调色板的图，验证 PLTE 被保留
	# 调色板图（colorType 3）全在 shadow 层：实测 31 张，键的 bit10=1
	# （这是修复前会报 "IDAT: Missing PLTE before IDAT" 的那批）
	var p: String = root + "/texture/selectchar/0000040001504C.PNG"
	_ok(FileAccess.file_exists(p), "调色板样本存在")
	var raw := FileAccess.get_file_as_bytes(p)
	_ok(raw[25] == 3, "样本确为 colorType 3（调色板）", str(raw[25]))
	var frames: Array = A._split_apng(raw)
	_ok(frames.size() > 0, "拆出帧", str(frames.size()))
	if not frames.is_empty():
		var f0: PackedByteArray = frames[0]
		# 重建后的 PNG 里必须含 PLTE 块
		var has_plte := false
		var i := 8
		while i + 8 <= f0.size():
			var ln: int = (f0[i] << 24) | (f0[i+1] << 16) | (f0[i+2] << 8) | f0[i+3]
			var typ := ""
			for k in range(4):
				typ += char(f0[i + 4 + k])
			if typ == "PLTE":
				has_plte = true
			i += 12 + ln
		_ok(has_plte, "重建 PNG 含 PLTE 块")
		var img := Image.new()
		_ok(img.load_png_from_buffer(f0) == OK, "重建帧可被解码")