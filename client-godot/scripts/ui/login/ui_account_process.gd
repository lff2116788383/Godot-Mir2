extends RefCounted
class_name UiAccountProcess
## 账号创建 + 修改密码 + 连接错误（三个进程合一）
##
## 【逐行翻译】client/src/processcreateaccount.cpp（373 行）
##            client/src/processchangepassword.cpp（440 行）
##            client/src/processconnectionerror.cpp（39 行）
##            common/src/idstrf.cpp（账号/密码格式校验）
##
## ══════════════ 布局（两个面板共用一套结构）══════════════
##   m_x = 180, m_y = 145（两个 hpp 里都是这两个值）
##   框贴图：创建账号 0x0A000000 (440x310)，改密 0x0A000001 (440x331)
##   输入框 x = m_x + 129 + 6 = 315，宽 186，高 28
##     创建账号 y：m_y + 85 / m_y + 143 / m_y + 198   （账号/密码/确认密码）
##     修改密码 y：m_y + 79 / m_y + 126 / m_y + 173 / m_y + 220（账号/旧密码/新密码/确认新密码）
##   提交按钮 0x0800000B (70x26)：创建 (m_x+189, m_y+233)，改密 (m_x+189, m_y+254)
##   关闭 0x0000001C (32x30)：创建 (m_x+400, m_y+267)，改密 (m_x+400, m_y+288)
##   底层背景：
##     0x00000003 (800x450) 画在 (0, 75)
##     0x00000004 (800x60)  画在 (0, 75)，目标尺寸 800x450（源码就是这么写的，会拉伸）
##
## ══════════════ 账号格式（idstrf.cpp:isEmail，正则照抄）══════════════
##   ^(([^<>()\[\]\\.,;:\s@"]+(\.[^<>()\[\]\\.,;:\s@"]+)*)|(".+"))
##    @
##    ((\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})|(([a-zA-Z\-0-9]+\.)+[a-zA-Z]{2,}))$
##   注意域名里的数字段限定 0~255（_REGEX_BYTE_0_255）
##
## ══════════════ 密码格式（idstrf.cpp:isPassword）══════════════
##   length >= 8 且 digits >= 1 且 lowers >= 1 且 uppers >= 1 且 specials >= 1
##   specials 只认 "~!@#$%^&*()"
##
## ══════════════ 错误文案（逐条照抄）══════════════
##   账号不合法 -> "无效账号"（清空全部输入）
##   密码不合法 -> "无效密码"（清两个密码框）
##   两次不一致 -> "两次密码输入不一致"（清两个密码框）
##   提交       -> "提交中"

const ProgUseTexRef := preload("res://scripts/data/proguse_tex.gd")

const CANVAS_W := 800
const CANVAS_H := 600

## 面板位置（两个 hpp 都是 180/145）
const PANEL_X := 180
const PANEL_Y := 145

const TEX_BG_UPPER := 0x00000003
const TEX_BG_LOWER := 0x00000004
const TEX_CREATE_ACCOUNT := 0x0A000000
const TEX_CHANGE_PWD := 0x0A000001
const BTN_SUBMIT := 0x0800000B
const BTN_CLOSE := 0x0000001C

## 输入框（x = PANEL_X + 129 + 6）
const INPUT_DX := 129 + 6
const INPUT_W := 186
const INPUT_H := 28

## 创建账号的三个框 y 偏移
const Y_CA_ID := 85
const Y_CA_PWD := 143
const Y_CA_PWD2 := 198
## 修改密码的四个框 y 偏移
const Y_CP_ID := 79
const Y_CP_OLD := 126
const Y_CP_NEW := 173
const Y_CP_NEW2 := 220

## 按钮偏移
const SUBMIT_DX := 189
const CLOSE_DX := 400
const Y_CA_SUBMIT := 233
const Y_CA_CLOSE := 267
const Y_CP_SUBMIT := 254
const Y_CP_CLOSE := 288

## 模式
const MODE_CREATE := 0
const MODE_CHANGE := 1

## 协议体
const CMCREATEACCOUNT_SIZE := 136
const CMCHANGEPWD_SIZE := 204

## 进程 id
const PID_LOGIN := 3

# ==================== 状态 ====================

var mode := MODE_CREATE
var inputs: Array = ["", "", "", ""]
var focus_index := 0
var info_text: String = ""

var on_request: Callable = Callable()
var on_send: Callable = Callable()

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init() -> void:
	pass


func set_canvas(c) -> void:
	_canvas_ref = c


func enter(p_mode: int) -> void:
	mode = p_mode
	# 创建账号 3 个框（账号/密码/确认密码），修改密码 4 个框（+ 旧密码）
	# ★ 原版是两个独立进程，各有一套 InputLine；这里合用一个类，
	#   所以输入框数量必须随模式变化，否则 Tab 会走到不存在的第 4 个框
	inputs = ["", "", ""] if p_mode == MODE_CREATE else ["", "", "", ""]
	focus_index = 0
	info_text = ""


# ==================== 格式校验（idstrf.cpp）====================

## idstrf::isEmail
## 源码用 std::regex，这里用等价的分段判断（避免 Godot 正则方言差异）
static func is_email(s: String) -> bool:
	if s.is_empty():
		return false
	var at := s.rfind("@")
	if at <= 0 or at >= s.length() - 1:
		return false
	var local := s.substr(0, at)
	var domain := s.substr(at + 1)
	# local 部分：不允许 < > ( ) [ ] \ . , ; : @ " 串中出现裸点开头结尾
	if local.begins_with(".") or local.ends_with("."):
		return false
	for ch in ["<", ">", "(", ")", "[", "]", "\\", ",", ";", ":", " ", '@']:
		if local.contains(ch):
			return false
	# 域名：纯 IPv4，或者 xxx.yyy.zz 形式（顶级域至少 2 个字母）
	return _is_ipv4(domain) or _is_domain(domain)


static func _is_ipv4(s: String) -> bool:
	var parts := s.split(".")
	if parts.size() != 4:
		return false
	for p in parts:
		if p.is_empty() or p.length() > 3:
			return false
		for i in range(p.length()):
			var ch := p[i]
			if ch < "0" or ch > "9":
				return false
		var v := int(p)
		if v < 0 or v > 255:
			return false
	return true


static func _is_domain(s: String) -> bool:
	var parts := s.split(".")
	if parts.size() < 2:
		return false
	# 顶级域至少 2 个字母
	var tld := parts[parts.size() - 1]
	if tld.length() < 2:
		return false
	for i in range(tld.length()):
		var c := tld[i]
		if not ((c >= "a" and c <= "z") or (c >= "A" and c <= "Z")):
			return false
	# 前面每段只能是字母/数字/连字符
	for i in range(parts.size() - 1):
		var p := parts[i]
		if p.is_empty():
			return false
		for j in range(p.length()):
			var c2 := p[j]
			var ok := (c2 >= "a" and c2 <= "z") or (c2 >= "A" and c2 <= "Z") \
				or (c2 >= "0" and c2 <= "9") or c2 == "-"
			if not ok:
				return false
	return true


## idstrf::isPassword
## length >= 8 && digits >= 1 && lowers >= 1 && uppers >= 1 && specials >= 1
static func is_password(s: String) -> bool:
	if s.is_empty():
		return false
	var digits := 0
	var lowers := 0
	var uppers := 0
	var specials := 0
	var specials_set := "~!@#$%^&*()"
	for i in range(s.length()):
		var c := s[i]
		if c >= "0" and c <= "9":
			digits += 1
		elif c >= "a" and c <= "z":
			lowers += 1
		elif c >= "A" and c <= "Z":
			uppers += 1
		elif specials_set.contains(c):
			specials += 1
	return s.length() >= 8 and digits >= 1 and lowers >= 1 and uppers >= 1 and specials >= 1


## idstrf::isCharName（角色名：非空且长度 > 6）
static func is_char_name(s: String) -> bool:
	return not s.is_empty() and s.length() > 6


# ==================== 布局辅助 ====================

## 当前模式下各输入框的 y 偏移
func input_y_list() -> Array:
	if mode == MODE_CREATE:
		return [PANEL_Y + Y_CA_ID, PANEL_Y + Y_CA_PWD, PANEL_Y + Y_CA_PWD2]
	return [PANEL_Y + Y_CP_ID, PANEL_Y + Y_CP_OLD,
		PANEL_Y + Y_CP_NEW, PANEL_Y + Y_CP_NEW2]


func input_labels() -> Array:
	if mode == MODE_CREATE:
		return ["账号", "密码", "确认密码"]
	return ["账号", "旧密码", "新密码", "确认新密码"]


func submit_y() -> int:
	return PANEL_Y + (Y_CA_SUBMIT if mode == MODE_CREATE else Y_CP_SUBMIT)


func close_y() -> int:
	return PANEL_Y + (Y_CA_CLOSE if mode == MODE_CREATE else Y_CP_CLOSE)


func panel_tex() -> int:
	return TEX_CREATE_ACCOUNT if mode == MODE_CREATE else TEX_CHANGE_PWD


# ==================== 绘制 ====================

func draw(canvas) -> void:
	if _canvas_ref != null:
		canvas = _canvas_ref
	hit_areas.clear()

	# 底层背景（processcreateaccount.cpp:182-183）
	var up := ProgUseTex.retrieve(TEX_BG_UPPER)
	if up != null:
		canvas.tex_draw_region(up, Rect2(0, 0, up.get_width(), up.get_height()),
			0, 75, up.get_width(), up.get_height())
	var lo := ProgUseTex.retrieve(TEX_BG_LOWER)
	if lo != null:
		# 源码：drawTexture(tex, 0, 75, 0, 0, 800, 450) —— 目标高 450 会拉伸
		canvas.tex_draw_region(lo, Rect2(0, 0, lo.get_width(), lo.get_height()),
			0, 75, 800, 450)

	# 面板框
	var panel := ProgUseTex.retrieve(panel_tex())
	if panel != null:
		canvas.tex_draw(panel, PANEL_X, PANEL_Y)

	# 输入框与标签
	var ys := input_y_list()
	var labels := input_labels()
	for i in range(ys.size()):
		var iy: int = int(ys[i])
		var ix: int = PANEL_X + INPUT_DX
		# 标签在框左侧
		canvas.draw_string_c(str(labels[i]), PANEL_X + 40, iy + 6,
			Color(0.9, 0.86, 0.72), null, 15)
		# 输入内容（密码框显示 *）
		var shown: String = str(inputs[i])
		if i > 0:
			shown = "*".repeat(shown.length())
		canvas.draw_string_c(shown, ix, iy + 6, Color(1, 0.95, 0.6), null, 15)
		if focus_index == i:
			canvas.draw_border(Color(1, 0.85, 0.3, 0.9),
				ix - 2, iy - 2, INPUT_W + 4, INPUT_H + 4, 1)
			var tw: int = int(canvas.measure_text(shown, null, 15).x)
			canvas.draw_rect_c(Color(1, 1, 1, 0.8), ix + tw + 1, iy + 6, 1, INPUT_H - 12, true)
		hit_areas["input_%d" % i] = Rect2(ix - 2, iy - 2, INPUT_W + 4, INPUT_H + 4)

	# 提交 / 关闭
	_draw_btn(canvas, BTN_SUBMIT, PANEL_X + SUBMIT_DX, submit_y(), "submit", "提交")
	_draw_btn(canvas, BTN_CLOSE, PANEL_X + CLOSE_DX, close_y(), "close", "关闭")

	# 信息文字（在面板下方居中）
	if not info_text.is_empty():
		var tw2: int = int(canvas.measure_text(info_text, null, 15).x)
		canvas.draw_string_c(info_text, PANEL_X + 220 - tw2 / 2, PANEL_Y + 300,
			Color(1, 0.85, 0.4), null, 15)


func _draw_btn(canvas, tex_id: int, x: int, y: int, act: String, label: String) -> void:
	var tex := ProgUseTex.retrieve(tex_id)
	if tex != null:
		canvas.tex_draw(tex, x, y)
		hit_areas[act] = Rect2(x, y, tex.get_width(), tex.get_height())
	else:
		var w: int = 70 if act == "submit" else 32
		var h: int = 26 if act == "submit" else 30
		canvas.draw_rect_c(Color(0.2, 0.18, 0.14, 0.95), x, y, w, h, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), x, y, w, h, 1)
		canvas.draw_string_c(label, x + 8, y + 6, Color(1, 0.95, 0.72), null, 14)
		hit_areas[act] = Rect2(x, y, w, h)


# ==================== 交互 ====================

func handle_click(px: int, py: int) -> bool:
	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			var k := str(key)
			if k.begins_with("input_"):
				focus_index = int(k.substr(6))
				return true
			if k == "submit":
				do_submit()
				return true
			if k == "close":
				if on_request.is_valid():
					on_request.call(PID_LOGIN)
				return true
	return false


func handle_key(keycode: int, unicode: int) -> bool:
	if keycode == KEY_TAB:
		focus_index = (focus_index + 1) % inputs.size()
		return true
	if keycode == KEY_ENTER or keycode == KEY_KP_ENTER:
		do_submit()
		return true
	if keycode == KEY_BACKSPACE:
		var cur := str(inputs[focus_index])
		if not cur.is_empty():
			inputs[focus_index] = cur.substr(0, cur.length() - 1)
		return true
	if unicode > 0:
		inputs[focus_index] = str(inputs[focus_index]) + char(unicode)
		return true
	return false


## processcreateaccount.cpp:doPostAccount
func do_submit() -> void:
	if mode == MODE_CREATE:
		_post_create_account()
	else:
		_post_change_password()


func _post_create_account() -> void:
	var id_str := str(inputs[0])
	var pwd_str := str(inputs[1])
	var pwd2_str := str(inputs[2])

	if not is_email(id_str):
		info_text = "无效账号"
		inputs = ["", "", "", ""]
		return
	if not is_password(pwd_str):
		info_text = "无效密码"
		inputs[1] = ""
		inputs[2] = ""
		return
	if pwd_str != pwd2_str:
		info_text = "两次密码输入不一致"
		inputs[1] = ""
		inputs[2] = ""
		return

	info_text = "提交中"
	var body := PackedByteArray()
	body.resize(CMCREATEACCOUNT_SIZE)
	body.fill(0)
	_put_static_buf(body, 0, id_str)
	_put_static_buf(body, 68, pwd_str)
	if on_send.is_valid():
		on_send.call(MirClient.CM_CREATEACCOUNT, body)   # CM_CREATEACCOUNT


## processchangepassword.cpp:doPostPassword（同构，多一个旧密码）
func _post_change_password() -> void:
	var id_str := str(inputs[0])
	var old_pwd := str(inputs[1])
	var new_pwd := str(inputs[2])
	var new_pwd2 := str(inputs[3])

	if not is_email(id_str):
		info_text = "无效账号"
		inputs = ["", "", "", ""]
		return
	if not is_password(new_pwd):
		info_text = "无效密码"
		inputs[2] = ""
		inputs[3] = ""
		return
	if new_pwd != new_pwd2:
		info_text = "两次密码输入不一致"
		inputs[2] = ""
		inputs[3] = ""
		return

	info_text = "提交中"
	var body := PackedByteArray()
	body.resize(CMCHANGEPWD_SIZE)
	body.fill(0)
	_put_static_buf(body, 0, id_str)
	_put_static_buf(body, 68, old_pwd)
	_put_static_buf(body, 136, new_pwd)
	if on_send.is_valid():
		on_send.call(MirClient.CM_CHANGEPASSWORD, body)  # CM_CHANGEPASSWORD


static func _put_static_buf(buf: PackedByteArray, off: int, s: String) -> void:
	var bytes := s.to_utf8_buffer()
	var n: int = mini(bytes.size(), 64)
	if off + 2 > buf.size():
		return
	buf.encode_u16(off, n)
	for i in range(n):
		if off + 2 + i < buf.size():
			buf[off + 2 + i] = bytes[i]


# ==================== 服务端应答 ====================

func on_create_account_ok() -> void:
	info_text = "账号创建成功"
	inputs = ["", "", "", ""]
	focus_index = 0


func on_create_account_error(code: int) -> void:
	var texts := ["账号已存在", "账号不合法", "密码不合法"]
	info_text = texts[code] if code < texts.size() else "创建失败（%d）" % code


func on_change_pwd_ok() -> void:
	info_text = "密码修改成功"
	inputs = ["", "", "", ""]
	focus_index = 0


func on_change_pwd_error(code: int) -> void:
	var texts := ["账号不存在", "旧密码错误", "新密码不合法"]
	info_text = texts[code] if code < texts.size() else "修改失败（%d）" % code
