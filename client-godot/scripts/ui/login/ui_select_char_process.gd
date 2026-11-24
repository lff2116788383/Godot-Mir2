extends RefCounted
class_name UiSelectCharProcess
## 选人进程（角色预览动画 + 开始/创建/删除/退出）
##
## 【逐行翻译】client/src/processselectchar.cpp（390 行）+ processselectchar.hpp
##
## ══════════════ 布局（processselectchar.cpp:26-30，逐值）══════════════
##   背景 0x0C000000 = 800x600，画在 (0,0)
##   开始  0x0C000030 (96x24)  (335,  75)
##   创建  0x0C000010 (96x26)  (565, 130)
##   删除  0x0C000020 (96x26)  (110, 305)
##   退出  0x0C000040 (48x26)  ( 45, 544)
##   角色绘制锚点 (430, 300)
##   角色信息板 (120, 260 - 板高)，外边距 15，圆角 5
##     内容：角色：%s / 等级：%d / 职业：%s
##       RGB(237,226,200) / RGB(175,196,175) / RGB(231,231,189)
##
## ══════════════ 动画状态机（switchCharGfx，逐条）══════════════
##   absFrame = round(aniTime / 200)      每帧 200ms
##   只在 absFrame % frameCount == 0 时才切，且同一 absFrame 只切一次
##     anim 0 --50%--> 1 --必--> 2 --50%--> 3 --必--> 0
##   ★ 因为只有 1 个角色，原版注释明确「no select/deselect」，
##     但仍完整跑 0→1→2→3→0 的展示循环
##   anim 切到 1 时播音效：0x00010000 | gender<<4 | job<<8 | 1
##
## ══════════════ 按钮显隐（构造函数末尾）══════════════
##   开始 / 创建 / 删除 三个都 .show = hasChar()
##   即：没有角色时只有「退出」可点，「创建」按钮反而藏起来
##   —— 这是原版的实际行为，不是笔误（hpp 里 m_create 的 show 也是 hasChar()）
##
## ══════════════ 删除流程（onDelete，onAccept）══════════════
##   弹 InputStringBoard（security=true），提示「删除的角色将无法还原…」
##   校验：输入长度 <= SYS_PWDSIZE(64)
##   提交 CM_DELETECHAR（68 字节 = StaticBuffer<64> password）
##
## ══════════════ SM_QUERYCHAROK（78 字节）══════════════
##   StaticBuffer<64> name  @0  （67 字节）
##   u8 gender @67
##   u8 job    @68
##   u64 exp   @69
##   注意：mock 服务端把 gender 写在 2+65=67、job 写 68、exp 写 70，
##   与 C++ 结构体 SMCORecord 的 78 字节布局一致。

const ProtocolDefRef := preload("res://scripts/core/protocol_def.gd")
const SelectGfxRef := preload("res://scripts/data/select_char_gfx.gd")
const ProgUseTexRef := preload("res://scripts/data/proguse_tex.gd")
const InputBoardRef := preload("res://scripts/ui/ui_input_string_board.gd")
const SysConstRef := preload("res://scripts/core/sys_const.gd")

## 布局常量
const CANVAS_W := 800
const CANVAS_H := 600

const TEX_BG := 0x0C000000
const BTN_START := 0x0C000030
const BTN_CREATE := 0x0C000010
const BTN_DELETE := 0x0C000020
const BTN_EXIT := 0x0C000040

const BTN_START_X := 335
const BTN_START_Y := 75
const BTN_CREATE_X := 565
const BTN_CREATE_Y := 130
const BTN_DELETE_X := 110
const BTN_DELETE_Y := 305
const BTN_EXIT_X := 45
const BTN_EXIT_Y := 544

const CHAR_DRAW_X := 430
const CHAR_DRAW_Y := 300
const INFO_X := 120
const INFO_Y_BASE := 260
const INFO_MARGIN := 15

## 帧时长（absFrame = round(aniTime / 200)）
const FRAME_MS := 200.0

## 协议
const CM_QUERYCHAR := 3
const CM_ONLINE := 7
const CM_DELETECHAR := 5
const CMDELETECHAR_SIZE := 68

## 进程 id
const PID_SELECTCHAR := 5
const PID_CREATECHAR := 6
const PID_RUN := 8

# ==================== 状态 ====================

## 角色数据（SM_QUERYCHAROK 解析结果）
var has_char := false
var char_name: String = ""
var char_gender_male := true
var char_job := 1
var char_exp := 0

## 动画
var ani_time := 0.0
var char_ani := 0
var ani_switch_frame := -1

## 通知
var notify: Array = []

## 回调
var on_request: Callable = Callable()
var on_send: Callable = Callable()
var on_error: Callable = Callable()
var on_ask_input: Callable = Callable()   ## Callable(title, security, callback)

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init() -> void:
	pass


func set_canvas(c) -> void:
	_canvas_ref = c


## 进入本进程时调用（processselectchar.cpp 构造函数末尾）
func enter() -> void:
	add_notify("正在下载游戏角色")
	if on_send.is_valid():
		on_send.call(CM_QUERYCHAR, PackedByteArray())


func add_notify(text: String) -> void:
	notify.append({"text": text, "ms": 5000})
	if notify.size() > 1:
		notify = notify.slice(notify.size() - 1)


# ==================== 动画 ====================

func abs_frame() -> int:
	return int(round(ani_time / FRAME_MS))


func frame_count() -> int:
	if not has_char:
		return 0
	return SelectGfxRef.frame_count(char_job, char_gender_male, char_ani)


func update(dt: float) -> void:
	ani_time += dt * 1000.0
	var ms := dt * 1000.0
	var kept: Array = []
	for n in notify:
		var left: int = int(n["ms"]) - int(ms)
		if left > 0:
			n["ms"] = left
			kept.append(n)
	notify = kept
	if has_char:
		switch_char_gfx()


## processselectchar.cpp:switchCharGfx
## 0 --50%--> 1 --必--> 2 --50%--> 3 --必--> 0
func switch_char_gfx() -> void:
	if not has_char:
		return
	var n := frame_count()
	if n <= 0:
		return
	var af := abs_frame()
	# 只在整帧边界切
	if af % n != 0:
		return
	# 同一帧只切一次
	if ani_switch_frame == af:
		return
	ani_switch_frame = af

	match char_ani:
		SelectGfxRef.ANIM_STAND_INACTIVE:
			if randi() % 2 == 0:
				char_ani = SelectGfxRef.ANIM_ON_SELECT
		SelectGfxRef.ANIM_ON_SELECT:
			char_ani = SelectGfxRef.ANIM_STAND_ACTIVE
		SelectGfxRef.ANIM_STAND_ACTIVE:
			if randi() % 2 == 0:
				char_ani = SelectGfxRef.ANIM_ON_DESELECT
		SelectGfxRef.ANIM_ON_DESELECT:
			char_ani = SelectGfxRef.ANIM_STAND_INACTIVE

	# 切到「选中动画」时播音效
	if char_ani == SelectGfxRef.ANIM_ON_SELECT:
		var sid := SelectGfxRef.anim_seff(char_job, char_gender_male, true)
		var audio = null
		if Engine.get_main_loop() != null:
			audio = Engine.get_main_loop().root.get_node_or_null("Audio")
		if audio != null:
			audio.play_seff(sid, 0, 0)


# ==================== 绘制 ====================

func draw(canvas) -> void:
	if _canvas_ref != null:
		canvas = _canvas_ref
	hit_areas.clear()

	var bg := ProgUseTex.retrieve(TEX_BG)
	if bg != null:
		canvas.tex_draw_region(bg, Rect2(0, 0, bg.get_width(), bg.get_height()),
			0, 0, bg.get_width(), bg.get_height())

	if has_char:
		# 角色四层（SelectGfxRef.draw_char 内部按原版顺序画）
		SelectGfxRef.draw_char(canvas, char_job, char_gender_male, char_ani,
			abs_frame(), CHAR_DRAW_X, CHAR_DRAW_Y, true)
		_draw_char_name(canvas)

	# 四个按钮
	_draw_btn(canvas, BTN_START, BTN_START_X, BTN_START_Y, "start")
	_draw_btn(canvas, BTN_CREATE, BTN_CREATE_X, BTN_CREATE_Y, "create")
	_draw_btn(canvas, BTN_DELETE, BTN_DELETE_X, BTN_DELETE_Y, "delete")
	_draw_btn(canvas, BTN_EXIT, BTN_EXIT_X, BTN_EXIT_Y, "exit")

	_draw_notify(canvas)


## 角色信息板（processselectchar.cpp:drawCharName）
func _draw_char_name(canvas) -> void:
	var lines: Array = [
		{"text": "角色：%s" % char_name, "color": Color(237.0 / 255, 226.0 / 255, 200.0 / 255)},
		{"text": "等级：%d" % level_from_exp(char_exp),
			"color": Color(175.0 / 255, 196.0 / 255, 175.0 / 255)},
		{"text": "职业：%s" % SelectGfxRef.job_name(char_job),
			"color": Color(231.0 / 255, 231.0 / 255, 189.0 / 255)},
	]
	var bw := 0
	for ln in lines:
		bw = maxi(bw, int(canvas.measure_text(str(ln["text"]), null, 15).x))
	var bh: int = lines.size() * 20

	var bx := INFO_X
	var by := INFO_Y_BASE - bh
	var m := INFO_MARGIN
	# 黑底 + 米色边（RGBA(231,231,189,128)）
	canvas.draw_rect_c(Color(0, 0, 0, 128.0 / 255.0),
		bx - m, by - m, bw + m * 2, bh + m * 2, true)
	canvas.draw_border(Color(231.0 / 255, 231.0 / 255, 189.0 / 255, 128.0 / 255),
		bx - m, by - m, bw + m * 2, bh + m * 2, 1)

	var ly := by
	for ln in lines:
		canvas.draw_string_c(str(ln["text"]), bx, ly, ln["color"], null, 15)
		ly += 20


## SYS_LEVEL(exp) -> level（sysconst.hpp:228，本工程已有实现）
static func level_from_exp(exp: int) -> int:
	return int(SysConstRef.SYS_LEVEL(exp)["level"])


func _draw_btn(canvas, tex_id: int, x: int, y: int, act: String) -> void:
	# 三个按钮只在有角色时显示（构造函数末尾的 setShow）
	if act != "exit" and not has_char:
		return
	var tex := ProgUseTex.retrieve(tex_id)
	if tex != null:
		canvas.tex_draw(tex, x, y)
		hit_areas[act] = Rect2(x, y, tex.get_width(), tex.get_height())
	else:
		var w: int = 48 if act == "exit" else 96
		var h: int = 26
		canvas.draw_rect_c(Color(0.2, 0.18, 0.14, 0.95), x, y, w, h, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), x, y, w, h, 1)
		var label: String = str({"start": "开始", "create": "创建", "delete": "删除", "exit": "退出"}.get(act, act))
		canvas.draw_string_c(str(label), x + 8, y + 5, Color(1, 0.95, 0.72), null, 14)
		hit_areas[act] = Rect2(x, y, w, h)


func _draw_notify(canvas) -> void:
	if notify.is_empty():
		return
	var t := str(notify[notify.size() - 1]["text"])
	var bw: int = int(canvas.measure_text(t, null, 15).x)
	var bx := CANVAS_W / 2 - bw / 2
	var by := CANVAS_H / 2 - 10
	var m := 8
	canvas.draw_rect_c(Color(0, 0, 0, 128.0 / 255.0),
		bx - m, by - m, bw + m * 2, 20 + m * 2, true)
	canvas.draw_border(Color(0, 0, 1, 128.0 / 255.0),
		bx - m, by - m, bw + m * 2, 20 + m * 2, 1)
	canvas.draw_string_c(t, bx, by, Color(1, 1, 0), null, 15)


# ==================== 交互 ====================

func handle_click(px: int, py: int) -> bool:
	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			match str(key):
				"start":
					on_start()
					return true
				"create":
					on_create()
					return true
				"delete":
					on_delete()
					return true
				"exit":
					on_exit()
					return true
	return false


## processselectchar.cpp:onStart
func on_start() -> void:
	if not has_char:
		add_notify("正在下载游戏角色")
		return
	if char_name.is_empty():
		add_notify("请先创建游戏角色")
		return
	if on_send.is_valid():
		on_send.call(CM_ONLINE, PackedByteArray())


## processselectchar.cpp:onCreate
func on_create() -> void:
	if not has_char:
		add_notify("正在下载游戏角色")
		return
	if char_name.is_empty():
		if on_request.is_valid():
			on_request.call(PID_CREATECHAR)
		return
	add_notify("一个账号只能创建一个游戏角色")


## processselectchar.cpp:onDelete
func on_delete() -> void:
	if not has_char:
		add_notify("此账号没有角色")
		return
	if char_name.is_empty():
		add_notify("此账号没有角色")
		return
	# 弹输入框要密码
	if on_ask_input.is_valid():
		on_ask_input.call("删除的角色将无法还原，请谨慎操作。如果确定删除，请输入游戏密码，并点击确定。",
			true, _on_delete_password)


func _on_delete_password(pwd: String) -> void:
	if pwd.is_empty() or pwd.length() > 64:
		add_notify("无效的密码")
		return
	# CM_DELETECHAR（68 字节 = StaticBuffer<64>）
	var body := PackedByteArray()
	body.resize(CMDELETECHAR_SIZE)
	body.fill(0)
	var bytes := pwd.to_utf8_buffer()
	var n: int = mini(bytes.size(), 64)
	body.encode_u16(0, n)
	for i in range(n):
		body[2 + i] = bytes[i]
	if on_send.is_valid():
		on_send.call(CM_DELETECHAR, body)


## processselectchar.cpp:onExit
func on_exit() -> void:
	if on_request.is_valid():
		on_request.call(PID_SELECTCHAR)


# ==================== 服务端应答 ====================

## SM_QUERYCHAROK（78 字节）
func on_query_char_ok(body: PackedByteArray) -> void:
	if body.size() < 78:
		add_notify("角色数据异常")
		return
	has_char = true
	char_name = _get_static_buf(body, 0)
	# ★ StaticBuffer<64> 占 68 字节（u16 + u8[65]，对齐到 68），
	#   不是 67。所以 gender 在 68、job 在 69、exp 在 70。
	#   按 67/68/69 读会把性别读成 job 的字节 —— 角色显示成错的性别。
	char_gender_male = body[68] != 0
	char_job = body[69]
	char_exp = body.decode_u64(70)
	# 原版：收到角色数据后按 hasChar 决定按钮显隐，无需额外处理
	notify.clear()
	if char_name.is_empty():
		add_notify("请先创建游戏角色")


func on_query_char_error(code: int) -> void:
	add_notify("查询角色失败（错误码 %d）" % code)


func on_delete_char_ok() -> void:
	add_notify("角色已删除")
	has_char = false
	char_name = ""


func on_delete_char_error(code: int) -> void:
	add_notify("删除失败（错误码 %d）" % code)


func on_online_ok(_body: PackedByteArray) -> void:
	# 进入游戏由上层接管
	if on_request.is_valid():
		on_request.call(PID_RUN)


func on_online_error(code: int) -> void:
	add_notify("进入游戏失败（错误码 %d）" % code)


static func _get_static_buf(buf: PackedByteArray, off: int) -> String:
	if off + 2 > buf.size():
		return ""
	var n: int = buf.decode_u16(off)
	if n <= 0 or n > 64:
		return ""
	if off + 2 + n > buf.size():
		return ""
	return buf.slice(off + 2, off + 2 + n).get_string_from_utf8()
