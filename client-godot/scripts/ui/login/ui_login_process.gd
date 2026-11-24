extends RefCounted
class_name UiLoginProcess
## 登录进程（Logo 淡入淡出 → 登录界面）
##
## 【逐行翻译】client/src/processlogo.cpp（69 行）+ processlogo.hpp
##            client/src/processlogin.cpp（253 行）+ processlogin.hpp
##            client/src/processconnectionerror.cpp（39 行）
##
## ══════════════ Logo（processlogo.hpp 常量，逐值）══════════════
##   m_timeR1 = 0.3    淡入占 30%
##   m_timeR2 = 0.3    全亮占 30%
##   m_fullTime = 5000.0 ms
##   colorRatio()：
##     r < r1        -> r / r1              （淡入）
##     r < r1 + r2   -> 1.0                 （全亮）
##     否则          -> 1 - (r-r1-r2)/(1-r1-r2)（淡出）
##   背景 0x00000000 = 800x600，拉伸铺满，按 colorRatio 调制灰度
##   空格 / ESC 跳过；超过 fullTime 自动进下一个进程
##
## ══════════════ 登录界面（processlogin.cpp，逐值）══════════════
##   背景层：
##     0x00000003 (800x450) 画在 (0, 75)
##     0x00000004 (800x60)  画在 (0, 465)
##     0x00000011 (456x26)  画在 (103, 536)   账号/密码分隔条
##   按钮（都是 96 宽 24~26 高）：
##     0x00000005 创建账号   (150, 482)
##     0x00000008 修改密码   (352, 482)
##     0x0000000B 退出       (554, 482)
##     0x0000000E 登录       (600, 536)   ← 96x24
##   输入框：
##     账号  (159, 540) 146x18  字号 18
##     密码  (409, 540) 146x18  字号 18  ★ 显示为 * （codeXfer）
##   Tab 在两框间切换；任一框回车 = 登录
##   通知栈：居中，蓝边黑底，(w/2, h/2)，留 5000ms，最多 10 条
##
## ══════════════ 登录逻辑（processlogin.cpp:120-180）══════════════
##   doLogin：id 或 pwd 为空 -> "无效的账号或密码"；否则发 CM_LOGIN
##   ★ 注释原文：don't check id/password by idstf functions
##     （允许 test/123456 这类测试账号；建号和改密才严格校验）
##   SM_LOGINOK    -> 进选人进程
##   SM_LOGINERROR -> 按错误码给提示
##
## ══════════════ 协议体（CMLogin，136 字节）══════════════
##   StaticBuffer<64> id       = u16 size + 65 字节 data，共 67
##   StaticBuffer<64> password = u16 size + 65 字节 data，共 67
##   合计 134…但 mock 与 C++ 都按 136 走（含 2 字节对齐），
##   所以这里也按 136 构造：id@0（67）+ pad@67（2）+ pwd@69（67）

const InputRef := preload("res://scripts/ui/framework/ui_input_line.gd")
const ProgUseTexRef := preload("res://scripts/data/proguse_tex.gd")
const ProgUseTex = ProgUseTexRef

## Logo
const LOGO_TEX := 0x00000000
const LOGO_TIME_R1 := 0.3
const LOGO_TIME_R2 := 0.3
const LOGO_FULL_MS := 5000.0

## 登录界面贴图与坐标
const TEX_BG_UPPER := 0x00000003
const TEX_BG_LOWER := 0x00000004
const TEX_SEP := 0x00000011

const BTN_CREATE_ACCOUNT := 0x00000005
const BTN_CHANGE_PWD := 0x00000008
const BTN_EXIT := 0x0000000B
const BTN_LOGIN := 0x0000000E

const BTN_Y := 482
const BTN_CREATE_X := 150
const BTN_CHPWD_X := 352
const BTN_EXIT_X := 554
const BTN_LOGIN_X := 600
const BTN_LOGIN_Y := 536

const ID_X := 159
const ID_Y := 540
const ID_W := 146
const ID_H := 18
const PWD_X := 409
const PWD_Y := 540
const PWD_W := 146
const PWD_H := 18
const INPUT_FONT_SIZE := 18

## 逻辑画布（原版 m_canvas 固定 800x600）
const CANVAS_W := 800
const CANVAS_H := 600

## 协议体尺寸
const CMLOGIN_SIZE := 136
## StaticBuffer<64> 的线上大小：u16 size + u8[65]，对齐到偶数 = 68
const STATIC_BUF_64_SIZE := 68
const CMNEWACCOUNT_SIZE := 203      ## id 67 + pwd 67 + pwdConfirm 67 = 201，mock 按 203
const CMCHANGEPWD_SIZE := 203       ## id 67 + pwd 67 + pwdNew 67

# ==================== 状态 ====================

## 阶段：0=logo 1=登录
var phase := 0
var logo_time := 0.0

## 登录输入
var account: String = ""
var password: String = ""

## 焦点：0=账号 1=密码
var focus_field := 0

## 通知（居中蓝边提示）
var notify: Array = []

## 画面尺寸（由外部同步，原版固定 800x600 但绘制按逻辑坐标）
var canvas_w := CANVAS_W
var canvas_h := CANVAS_H

## 回调
var on_request: Callable = Callable()   ## Callable(process_id)
var on_send: Callable = Callable()      ## Callable(msg_id, body: PackedByteArray)
var on_error: Callable = Callable()     ## Callable(text)

## 进程 id（对应 ProcessID 枚举）
const PID_LOGO := 1
const PID_LOGIN := 3
const PID_CREATEACCOUNT := 4
const PID_SELECTCHAR := 5
const PID_CREATECHAR := 6
const PID_CHANGEPASSWORD := 7
const PID_RUN := 8
const PID_CONNECTIONERROR := 9

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init() -> void:
	pass


func set_canvas(c) -> void:
	_canvas_ref = c


func reset() -> void:
	phase = 0
	logo_time = 0.0
	notify.clear()


# ==================== Logo ====================

## processlogo.cpp:colorRatio
func logo_color_ratio() -> float:
	var r: float = logo_time / LOGO_FULL_MS
	if r < LOGO_TIME_R1:
		return r / LOGO_TIME_R1
	if r < LOGO_TIME_R1 + LOGO_TIME_R2:
		return 1.0
	var rest := 1.0 - LOGO_TIME_R1 - LOGO_TIME_R2
	if rest <= 0.0001:
		return 0.0
	return 1.0 - (r - LOGO_TIME_R1 - LOGO_TIME_R2) / rest


func update(dt: float) -> void:
	var ms := dt * 1000.0
	if phase == 0:
		logo_time += ms
		notify_tick(ms)
		if logo_time >= LOGO_FULL_MS:
			skip_logo()
	else:
		notify_tick(ms)


## 跳过 logo（空格 / ESC / 超时）
func skip_logo() -> void:
	if phase != 0:
		return
	phase = 1
	_request(PID_LOGIN)


func notify_tick(ms: float) -> void:
	var kept: Array = []
	for n in notify:
		var left: int = int(n["ms"]) - int(ms)
		if left > 0:
			n["ms"] = left
			kept.append(n)
	notify = kept


func add_notify(text: String) -> void:
	notify.append({"text": text, "ms": 5000})
	if notify.size() > 10:
		notify = notify.slice(notify.size() - 10)


func _request(pid: int) -> void:
	if on_request.is_valid():
		on_request.call(pid)


# ==================== 绘制 ====================

func draw(canvas) -> void:
	if _canvas_ref != null:
		canvas = _canvas_ref
	if phase == 0:
		_draw_logo(canvas)
	else:
		_draw_login(canvas)
	_draw_notify(canvas)


func _draw_logo(canvas) -> void:
	var tex := ProgUseTex.retrieve(LOGO_TEX)
	var c: float = logo_color_ratio()
	var col := Color(c, c, c, 1.0)
	if tex != null:
		# 原版拉伸铺满窗口
		canvas.tex_draw_region(tex, Rect2(0, 0, tex.get_width(), tex.get_height()),
			0, 0, canvas_w, canvas_h, col)
	else:
		canvas.draw_rect_c(Color(c * 0.1, c * 0.1, c * 0.15, 1), 0, 0, canvas_w, canvas_h, true)
		canvas.draw_string_c("mir2x", canvas_w / 2 - 40, canvas_h / 2 - 20,
			Color(c, c, c * 0.8), null, 40)


func _draw_login(canvas) -> void:
	hit_areas.clear()

	# 三层背景（坐标照抄 processlogin.cpp:draw）
	var up := ProgUseTex.retrieve(TEX_BG_UPPER)
	if up != null:
		canvas.tex_draw_region(up, Rect2(0, 0, up.get_width(), up.get_height()),
			0, 75, up.get_width(), up.get_height())
	else:
		canvas.draw_rect_c(Color(0.1, 0.09, 0.12, 1), 0, 75, 800, 450, true)

	var lo := ProgUseTex.retrieve(TEX_BG_LOWER)
	if lo != null:
		canvas.tex_draw_region(lo, Rect2(0, 0, lo.get_width(), lo.get_height()),
			0, 465, lo.get_width(), lo.get_height())
	else:
		canvas.draw_rect_c(Color(0.08, 0.07, 0.09, 1), 0, 465, 800, 60, true)

	var sep := ProgUseTex.retrieve(TEX_SEP)
	if sep != null:
		canvas.tex_draw_region(sep, Rect2(0, 0, sep.get_width(), sep.get_height()),
			103, 536, sep.get_width(), sep.get_height())

	# 三个功能按钮
	_draw_btn(canvas, BTN_CREATE_ACCOUNT, BTN_CREATE_X, BTN_Y, "create_account")
	_draw_btn(canvas, BTN_CHANGE_PWD, BTN_CHPWD_X, BTN_Y, "change_pwd")
	_draw_btn(canvas, BTN_EXIT, BTN_EXIT_X, BTN_Y, "exit")
	_draw_btn(canvas, BTN_LOGIN, BTN_LOGIN_X, BTN_LOGIN_Y, "login")

	# 两个输入框
	_draw_input(canvas, ID_X, ID_Y, ID_W, ID_H, account, focus_field == 0, "account")
	# 密码显示为 *
	var masked := "*".repeat(password.length())
	_draw_input(canvas, PWD_X, PWD_Y, PWD_W, PWD_H, masked, focus_field == 1, "password")


## 画一个贴图按钮，并登记命中区
func _draw_btn(canvas, tex_id: int, x: int, y: int, act: String) -> void:
	var tex := ProgUseTex.retrieve(tex_id)
	if tex != null:
		canvas.tex_draw(tex, x, y)
		hit_areas[act] = Rect2(x, y, tex.get_width(), tex.get_height())
	else:
		var w := 96
		var h := 24
		canvas.draw_rect_c(Color(0.2, 0.18, 0.14, 0.95), x, y, w, h, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), x, y, w, h, 1)
		var label: String = str({"create_account": "创建账号", "change_pwd": "修改密码",
			"exit": "退出", "login": "登录"}.get(act, act))
		canvas.draw_string_c(str(label), x + 6, y + 4, Color(1, 0.95, 0.72), null, 14)
		hit_areas[act] = Rect2(x, y, w, h)


func _draw_input(canvas, x: int, y: int, w: int, h: int, text: String,
		focused: bool, act: String) -> void:
	# 原版 InputLine 无底色（直接画在背景图上），聚焦时画框
	canvas.draw_string_c(text, x, y + 1, Color(1, 0.95, 0.6), null, INPUT_FONT_SIZE)
	if focused:
		canvas.draw_border(Color(1, 0.85, 0.3, 0.9), x - 2, y - 2, w + 4, h + 4, 1)
		# 光标
		var tw: int = int(canvas.measure_text(text, null, INPUT_FONT_SIZE).x)
		canvas.draw_rect_c(Color(1, 1, 1, 0.8), x + tw + 1, y + 1, 1, h - 2, true)
	hit_areas[act] = Rect2(x - 2, y - 2, w + 4, h + 4)


func _draw_notify(canvas) -> void:
	if notify.is_empty():
		return
	# 居中，蓝边黑底（processlogin.cpp:m_notifyBoardBg）
	var bw := 0
	var lines: Array = []
	for n in notify:
		var t := str(n["text"])
		lines.append(t)
		bw = maxi(bw, int(canvas.measure_text(t, null, 15).x))
	var bh: int = lines.size() * 20
	var bx: int = canvas_w / 2 - bw / 2
	var by: int = canvas_h / 2 - bh / 2
	var margin := 10
	canvas.draw_rect_c(Color(0, 0, 0, 128.0 / 255.0),
		bx - margin, by - margin, bw + margin * 2, bh + margin * 2, true)
	canvas.draw_border(Color(0, 0, 1, 128.0 / 255.0),
		bx - margin, by - margin, bw + margin * 2, bh + margin * 2, 1)
	var ly := by
	for ln in lines:
		canvas.draw_string_c(str(ln), bx, ly, Color(1, 1, 0), null, 15)
		ly += 20


# ==================== 交互 ====================

func handle_click(px: int, py: int) -> bool:
	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			var k := str(key)
			match k:
				"account":
					focus_field = 0
					return true
				"password":
					focus_field = 1
					return true
				"login":
					do_login()
					return true
				"create_account":
					_request(PID_CREATEACCOUNT)
					return true
				"change_pwd":
					_request(PID_CHANGEPASSWORD)
					return true
				"exit":
					if on_error.is_valid():
						on_error.call("exit")
					return true
	return false


func handle_key(keycode: int, unicode: int) -> bool:
	if phase == 0:
		# 空格 / ESC 跳过 logo（processlogo.cpp:19-30）
		if keycode == KEY_SPACE or keycode == KEY_ESCAPE:
			skip_logo()
			return true
		return false

	if keycode == KEY_TAB:
		focus_field = 1 - focus_field
		return true
	if keycode == KEY_ENTER or keycode == KEY_KP_ENTER:
		do_login()
		return true
	if keycode == KEY_BACKSPACE:
		if focus_field == 0:
			if not account.is_empty():
				account = account.substr(0, account.length() - 1)
		else:
			if not password.is_empty():
				password = password.substr(0, password.length() - 1)
		return true
	if unicode > 0:
		var ch := char(unicode)
		if focus_field == 0:
			account += ch
		else:
			password += ch
		return true
	return false


## processlogin.cpp:doLogin
func do_login() -> void:
	if account.is_empty() or password.is_empty():
		add_notify("无效的账号或密码")
		return
	send_login(account, password)


## processlogin.cpp:sendLogin（CMLogin 136 字节）
##
## ★ 每个 StaticBuffer<64> 占 **68 字节**，不是 67：
##   C++ 里是 `uint16_t size + uint8_t data[65]` = 67，
##   但 uint16_t 要求 2 字节对齐，整个结构体被 padding 到 68。
##   cerealf 按 POD 原始内存布局拷贝，所以落盘/上线就是 68。
##   所以第二个 buffer 的起点是 68，不是 69 ——
##   之前写 69 会让服务端读到的长度字段错位（读到 0x3100 = 12544）。
func send_login(id: String, pwd: String) -> void:
	var body := PackedByteArray()
	body.resize(CMLOGIN_SIZE)
	body.fill(0)
	_put_static_buf(body, 0, id)
	_put_static_buf(body, STATIC_BUF_64_SIZE, pwd)
	if on_send.is_valid():
		on_send.call(2, body)   # CM_LOGIN = 2


## StaticBuffer<64> 写入：u16 size + 65 字节 data
static func _put_static_buf(buf: PackedByteArray, off: int, s: String) -> void:
	var bytes := s.to_utf8_buffer()
	var n: int = mini(bytes.size(), 64)
	buf.encode_u16(off, n)
	for i in range(n):
		buf[off + 2 + i] = bytes[i]


## StaticBuffer<64> 读取
static func _get_static_buf(buf: PackedByteArray, off: int) -> String:
	var n: int = buf.decode_u16(off)
	if n <= 0 or n > 64:
		return ""
	var b := buf.slice(off + 2, off + 2 + n)
	return b.get_string_from_utf8()


# ==================== 服务端应答 ====================

## SM_LOGINOK（0 长度体）
func on_login_ok() -> void:
	add_notify("登录成功")
	_request(PID_SELECTCHAR)


## SM_LOGINERROR（1 字节错误码）
func on_login_error(code: int) -> void:
	add_notify(login_error_text(code))


## 登录错误码文案（servermsg 的 SM_LOGINERROR 枚举顺序）
static func login_error_text(code: int) -> String:
	match code:
		0: return "账号不存在"
		1: return "密码错误"
		2: return "账号已在线"
		3: return "服务器满员"
		4: return "版本不匹配"
		_: return "登录失败（错误码 %d）" % code
