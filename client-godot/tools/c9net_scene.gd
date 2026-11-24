extends Node
## C9 联调自检（场景内，需先启动 mock 服务端）
##
## 覆盖：连接 → 跳过 Logo → CM_LOGIN → SM_LOGINOK → CM_QUERYCHAR
##       → SM_QUERYCHAROK（角色数据）→ CM_ONLINE → SM_ONLINEOK → 进程切到 RUN
##
## 运行：Godot --path . -- --login --run-test c9c8
## 前置：tools/mock-server/mock-server.exe -port 7000

var _pass := 0
var _fail := 0


func _ok(cond: bool, name: String, extra: String = "") -> void:
	if cond:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, ("  " + extra) if extra else "")


func run(main: Node) -> void:
	print("=== C9 联调自检 ===")
	var lf = main.get("login_flow")
	_ok(lf != null, "LoginFlow 已创建")
	if lf == null:
		_report()
		return

	# 1. 网络连接
	var waited := 0
	while (lf.net == null or not lf.net.is_connected) and waited < 300:
		await get_tree().process_frame
		waited += 1
	_ok(lf.net != null and lf.net.is_connected, "已连上 mock 服务端")
	if lf.net == null or not lf.net.is_connected:
		_report()
		return

	# 2. 初始在 Logo
	_ok(lf.cur_pid == lf.PID_LOGO, "初始进程 = LOGO", str(lf.cur_pid))

	# 3. 跳过 Logo 进登录
	lf.login.skip_logo()
	await get_tree().process_frame
	_ok(lf.cur_pid == lf.PID_LOGIN, "跳过 Logo 后 = LOGIN", str(lf.cur_pid))

	# 4. 登录（走 net.login，它按 sm_table 的掩码编码发 CM_LOGIN）
	var seq = lf.net.login("test@test.com", "123456")
	_ok(int(seq) > 0, "CM_LOGIN 已发出", "resp=%d" % int(seq))

	waited = 0
	while lf.cur_pid != lf.PID_SELECTCHAR and waited < 400:
		await get_tree().process_frame
		waited += 1
	_ok(lf.cur_pid == lf.PID_SELECTCHAR, "SM_LOGINOK 后 = SELECTCHAR",
		"进程=%d 等待=%d帧" % [lf.cur_pid, waited])

	# 5. 角色数据
	waited = 0
	while not lf.select_char.has_char and waited < 400:
		await get_tree().process_frame
		waited += 1
	_ok(lf.select_char.has_char, "SM_QUERYCHAROK 收到角色", "等待=%d帧" % waited)
	_ok(not lf.select_char.char_name.is_empty(), "角色名非空", lf.select_char.char_name)
	_ok(lf.select_char.char_job > 0, "职业有效", str(lf.select_char.char_job))
	_ok(lf.select_char.frame_count() > 0, "动画帧数 > 0",
		str(lf.select_char.frame_count()))

	# 6. 进游戏
	lf.select_char.on_start()
	waited = 0
	while lf.cur_pid != lf.PID_RUN and waited < 400:
		await get_tree().process_frame
		waited += 1
	_ok(lf.cur_pid == lf.PID_RUN, "SM_ONLINEOK 后 = RUN",
		"进程=%d 等待=%d帧" % [lf.cur_pid, waited])
	_ok(not lf.in_login_flow(), "已交棒给游戏主进程")

	# 7. 角色数据与 mock 配置一致（mock 默认「测试角色」/战士/男）
	_ok(lf.select_char.char_name == "测试角色", "角色名 = 测试角色",
		lf.select_char.char_name)
	_ok(lf.select_char.char_job == 1, "职业 = 战士(1)", str(lf.select_char.char_job))
	_ok(lf.select_char.char_gender_male, "性别 = 男")

	_report()
	get_tree().quit(0)


func _report() -> void:
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
