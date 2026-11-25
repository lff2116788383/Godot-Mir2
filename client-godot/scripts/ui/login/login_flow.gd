extends CanvasLayer
class_name LoginFlow
## 登录链路的进程管理器（对应 client.cpp 的 requestProcess 状态机）
##
## 【逐行翻译】client/src/client.cpp 的进程调度部分
##            client/src/process.hpp（ProcessID 枚举）
##
## ══════════════ 进程表（process.hpp，逐值）══════════════
##   PROCESSID_NONE            = 0
##   PROCESSID_LOGO            = 1
##   PROCESSID_SYRC            = 2   （Logo 之后的过渡，原版无独立实现）
##   PROCESSID_LOGIN           = 3
##   PROCESSID_CREATEACCOUNT   = 4
##   PROCESSID_SELECTCHAR      = 5
##   PROCESSID_CREATECHAR      = 6
##   PROCESSID_CHANGEPASSWORD  = 7
##   PROCESSID_RUN             = 8
##   PROCESSID_CONNECTIONERROR = 9
##
## ══════════════ 流转（从各进程的 onTrigger 归纳）══════════════
##   LOGO --超时/空格--> SYRC --> LOGIN
##   LOGIN --登录成功--> SELECTCHAR（网络 SM_LOGINOK 驱动）
##   LOGIN --创建账号--> CREATEACCOUNT --关闭--> LOGIN
##   LOGIN --修改密码--> CHANGEPASSWORD --关闭--> LOGIN
##   SELECTCHAR --开始--> RUN（网络 SM_ONLINEOK 驱动）
##   SELECTCHAR --创建--> CREATECHAR --成功/返回--> SELECTCHAR
##   RUN --掉线--> CONNECTIONERROR --> LOGIN
##
## ══════════════ 与原版的差异（必须说明）══════════════
##   原版 ProcessRun 是「进游戏后的主进程」，本工程由 main.gd 承担。
##   所以 RUN 这一态在这里只做「交棒」：抛 run_entered 信号让 main 接管，
##   不重复实现已经完成的 B/C 系列逻辑。

## 进程 id
const PID_NONE := 0
const PID_LOGO := 1
const PID_SYRC := 2
const PID_LOGIN := 3
const PID_CREATEACCOUNT := 4
const PID_SELECTCHAR := 5
const PID_CREATECHAR := 6
const PID_CHANGEPASSWORD := 7
const PID_RUN := 8
const PID_CONNECTIONERROR := 9

## 协议码（以 sm_table.gd 为唯一真源，这里只做引用）
## 直接引用 MirClient 的协议常量，避免再次手抄错枚举值
const CM_LOGIN := MirClient.CM_LOGIN
const CM_QUERYCHAR := MirClient.CM_QUERYCHAR
const CM_CREATECHAR := MirClient.CM_CREATECHAR
const CM_DELETECHAR := MirClient.CM_DELETECHAR
const CM_CHANGEPASSWORD := MirClient.CM_CHANGEPASSWORD
const CM_ONLINE := MirClient.CM_ONLINE
const CM_CREATEACCOUNT := MirClient.CM_CREATEACCOUNT
const SM_LOGINOK := MirClient.SM_LOGINOK
const SM_LOGINERROR := MirClient.SM_LOGINERROR
const SM_CREATEACCOUNTOK := MirClient.SM_CREATEACCOUNTOK
const SM_CREATEACCOUNTERROR := MirClient.SM_CREATEACCOUNTERROR
const SM_CHANGEPASSWORDOK := MirClient.SM_CHANGEPASSWORDOK
const SM_CHANGEPASSWORDERROR := MirClient.SM_CHANGEPASSWORDERROR
const SM_QUERYCHAROK := MirClient.SM_QUERYCHAROK
const SM_QUERYCHARERROR := MirClient.SM_QUERYCHARERROR
const SM_CREATECHAROK := MirClient.SM_CREATECHAROK
const SM_CREATECHARERROR := MirClient.SM_CREATECHARERROR
const SM_DELETECHAROK := MirClient.SM_DELETECHAROK
const SM_DELETECHARERROR := MirClient.SM_DELETECHARERROR
const SM_ONLINEOK := MirClient.SM_ONLINEOK
const SM_ONLINEERROR := MirClient.SM_ONLINEERROR

const LoginProcRef := preload("res://scripts/ui/login/ui_login_process.gd")
const SelectProcRef := preload("res://scripts/ui/login/ui_select_char_process.gd")
const CreateProcRef := preload("res://scripts/ui/login/ui_create_char_process.gd")
const AccountProcRef := preload("res://scripts/ui/login/ui_account_process.gd")
const CanvasRef := preload("res://scripts/ui/framework/ui_canvas.gd")
const ProgUseRef := preload("res://scripts/data/proguse_tex.gd")
const SelectGfxRef := preload("res://scripts/data/select_char_gfx.gd")

## 当前进程
var cur_pid := PID_LOGO
## 各进程实例
var login: LoginProcRef = null
var select_char: SelectProcRef = null
var create_char: CreateProcRef = null
var account: AccountProcRef = null

## 网络
var net = null
## 绘制器
var _canvas: CanvasRef = null

## 信号
signal run_entered(online_body: PackedByteArray)
signal conn_error(reason: String)

## 登录态
var account_id: String = ""
var account_pwd: String = ""


func _ready() -> void:
	layer = 20
	_canvas = CanvasRef.new()
	_canvas.name = "LoginCanvas"
	add_child(_canvas)
	_canvas.setup()
	# 每个进程的 draw 直接画到 canvas 上：让 canvas 每帧回调
	_canvas.set_process(true)
	_canvas.draw.connect(_on_canvas_draw)
	# 登录链路的画布是固定 800x600（原版逻辑分辨率），
	# 缩放到视口并居中，否则 1280x720 下会只占左上角一块。
	_fit_canvas()

	ProgUseRef.setup()
	SelectGfxRef.setup()
	var audio = get_node_or_null("/root/Audio")
	if audio != null:
		audio.play_bgm(0x00040007)

	login = LoginProcRef.new()
	login.set_canvas(_canvas)
	login.on_request = _on_request
	login.on_send = _on_send
	login.on_error = func(t):
		if t == "exit":
			get_tree().quit(0)

	select_char = SelectProcRef.new()
	select_char.set_canvas(_canvas)
	select_char.on_request = _on_request
	select_char.on_send = _on_send
	select_char.on_ask_input = _on_ask_input

	create_char = CreateProcRef.new()
	create_char.set_canvas(_canvas)
	create_char.on_request = _on_request
	create_char.on_send = _on_send

	account = AccountProcRef.new()
	account.set_canvas(_canvas)
	account.on_request = _on_request
	account.on_send = _on_send

	_switch_to(PID_LOGO)


## 接网络对象
func bind_net(p_net) -> void:
	net = p_net
	if net != null:
		if net.has_signal("frame_received"):
			if not net.frame_received.is_connected(_on_frame):
				net.frame_received.connect(_on_frame)
			if not net.disconnected.is_connected(_on_disconnected):
				net.disconnected.connect(_on_disconnected)


# ==================== 进程切换 ====================

## client.cpp:requestProcess
func _on_request(pid: int) -> void:
	_switch_to(pid)


func _switch_to(pid: int) -> void:
	match pid:
		PID_SYRC:
			# 原版是过渡态，直接进登录
			_switch_to(PID_LOGIN)
			return
		PID_LOGIN:
			cur_pid = PID_LOGIN
			login.phase = 1
		PID_CREATEACCOUNT:
			cur_pid = PID_CREATEACCOUNT
			account.enter(AccountProcRef.MODE_CREATE)
		PID_CHANGEPASSWORD:
			cur_pid = PID_CHANGEPASSWORD
			account.enter(AccountProcRef.MODE_CHANGE)
		PID_SELECTCHAR:
			cur_pid = PID_SELECTCHAR
			select_char.enter()
		PID_CREATECHAR:
			cur_pid = PID_CREATECHAR
			create_char.enter()
		PID_RUN:
			cur_pid = PID_RUN
			# 交棒给 main
			run_entered.emit(_last_online_body)
		PID_CONNECTIONERROR:
			cur_pid = PID_CONNECTIONERROR
			conn_error.emit("连接中断")
		PID_LOGO:
			cur_pid = PID_LOGO
			login.reset()
		_:
			cur_pid = pid


func current_process():
	match cur_pid:
		PID_LOGO, PID_LOGIN:
			return login
		PID_SELECTCHAR:
			return select_char
		PID_CREATECHAR:
			return create_char
		PID_CREATEACCOUNT, PID_CHANGEPASSWORD:
			return account
	return null


## 是否还在登录链路（false 表示已交棒给游戏主进程）
func in_login_flow() -> bool:
	return cur_pid != PID_RUN


# ==================== 每帧 ====================

func _process_update(dt: float) -> void:
	var p = current_process()
	if p != null and p.has_method("update"):
		p.update(dt)
	if _canvas != null:
		_canvas.queue_redraw()


## 把 800x600 逻辑画布等比缩放并居中到当前视口
## 原版用 SDL 的 logical size（800x600 + letterbox），Godot 这里等价实现：
##   scale = min(vw/800, vh/600)
##   offset = (视口 - 画布*scale) / 2
func _fit_canvas() -> void:
	if _canvas == null:
		return
	var vp := get_viewport()
	if vp == null:
		return
	var vs := vp.get_visible_rect().size
	var s: float = minf(vs.x / 800.0, vs.y / 600.0)
	_canvas.scale = Vector2(s, s)
	_canvas.position = Vector2(
		(vs.x - 800.0 * s) * 0.5,
		(vs.y - 600.0 * s) * 0.5)


func _process(dt: float) -> void:
	_fit_canvas()
	_process_update(dt)


## canvas 的 _draw 里回调：交给当前进程绘制
func _on_canvas_draw() -> void:
	var p = current_process()
	if p != null and p.has_method("draw"):
		p.draw(_canvas)


# ==================== 输入转发 ====================

func handle_input(event: InputEvent) -> bool:
	var p = current_process()
	if p == null:
		return false

	if event is InputEventMouseButton:
		var mb := event as InputEventMouseButton
		if mb.pressed and mb.button_index == MOUSE_BUTTON_LEFT:
			var pos := mb.position
			# 屏幕坐标 -> 800x600 逻辑坐标
			# 必须先减去居中的 offset，再除以缩放比，否则点偏（画布不在左上角了）
			var vp := get_viewport().get_visible_rect().size
			var s: float = minf(vp.x / 800.0, vp.y / 600.0)
			var off := Vector2((vp.x - 800.0 * s) * 0.5, (vp.y - 600.0 * s) * 0.5)
			var lx: int = int((pos.x - off.x) / maxf(0.0001, s))
			var ly: int = int((pos.y - off.y) / maxf(0.0001, s))
			if p.has_method("handle_click"):
				return bool(p.handle_click(lx, ly))
		return false

	if event is InputEventKey:
		var ke := event as InputEventKey
		if ke.pressed and not ke.echo:
			if p.has_method("handle_key"):
				return bool(p.handle_key(ke.keycode, ke.unicode))
		return false

	return false


# ==================== 网络 ====================

var _last_online_body := PackedByteArray()


func _on_send(msg_id: int, body: PackedByteArray) -> void:
	if net == null:
		return
	# CM 消息的编码方式由 sm_table 的 type 决定（1=掩码压缩，2=裸 body），
	# net.send 会自动按表选择，这里只需把 body 原样给它。
	var seq: int = net.send(msg_id, body, true)
	print("[C9] 发送 CM %d, %d 字节, resp=%d" % [msg_id, body.size(), seq])


func _on_frame(head_code: int, body: PackedByteArray, _resp_id: int) -> void:
	match head_code:
		SM_LOGINOK:
			if login != null:
				login.on_login_ok()
		SM_LOGINERROR:
			if login != null:
				login.on_login_error(body[0] if body.size() > 0 else 0)
		SM_QUERYCHAROK:
			if select_char != null:
				select_char.on_query_char_ok(body)
		SM_QUERYCHARERROR:
			if select_char != null:
				select_char.on_query_char_error(body[0] if body.size() > 0 else 0)
		SM_CREATECHAROK:
			if create_char != null:
				create_char.on_create_char_ok()
		SM_CREATECHARERROR:
			if create_char != null:
				create_char.on_create_char_error(body[0] if body.size() > 0 else 0)
		SM_DELETECHAROK:
			if select_char != null:
				select_char.on_delete_char_ok()
		SM_DELETECHARERROR:
			if select_char != null:
				select_char.on_delete_char_error(body[0] if body.size() > 0 else 0)
		SM_ONLINEOK:
			_last_online_body = body
			if select_char != null:
				select_char.on_online_ok(body)
		SM_ONLINEERROR:
			if select_char != null:
				select_char.on_online_error(body[0] if body.size() > 0 else 0)
		SM_CREATEACCOUNTOK:
			if account != null:
				account.on_create_account_ok()
		SM_CREATEACCOUNTERROR:
			if account != null:
				account.on_create_account_error(body[0] if body.size() > 0 else 0)
		SM_CHANGEPASSWORDOK:
			if account != null:
				account.on_change_pwd_ok()
		SM_CHANGEPASSWORDERROR:
			if account != null:
				account.on_change_pwd_error(body[0] if body.size() > 0 else 0)


func _on_disconnected(reason: String) -> void:
	if cur_pid != PID_RUN:
		_switch_to(PID_CONNECTIONERROR)


## 弹输入框（删除角色要密码）
func _on_ask_input(title: String, security: bool, cb: Callable) -> void:
	var board = get_node_or_null("/root/Game")
	if board != null:
		pass
	# 直接用 Godot 的输入框兜底：原版走 InputStringBoard，
	# 这里因为登录态下没有 GameUi，用 Java 式回调 + 简单弹窗
	var dlg := AcceptDialog.new()
	dlg.dialog_text = title
	dlg.title = "删除角色"
	var le := LineEdit.new()
	le.secret = security
	le.custom_minimum_size = Vector2(240, 0)
	dlg.add_child(le)
	add_child(dlg)
	dlg.confirmed.connect(func():
		cb.call(le.text)
		dlg.queue_free()
	)
	dlg.canceled.connect(func(): dlg.queue_free())
	dlg.popup_centered()
