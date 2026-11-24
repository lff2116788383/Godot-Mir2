extends RefCounted
class_name UiCreateCharProcess
## 建号进程（三职业选择 + 男女预览 + 名字输入）
##
## 【逐行翻译】client/src/processcreatechar.cpp（305 行）+ processcreatechar.hpp
##
## ══════════════ 布局（processcreatechar.cpp:26-42，逐值）══════════════
##   背景 0x0D000000 = 800x600，画在 (0,0)
##   职业按钮（40x38）：战士 (339,539) 道士 (381,539) 法师 (424,539)
##   提交 0x0D000010 (28x28)  (512, 549)
##   退出 0x0D000020 (28x28)  (554, 549)
##   名字框 (355, 520) 85x15，字号 12，IME_SYSTEM
##     底下先填黑底 (355,520,90,15)，再画米色框 RGBA(231,231,189,100)
##   名字框上方装饰 0x0D000001 (164x88) 画在 (320, 500)
##   底部提示 0x0D000002 (256x32) 画在 (268, 555)，modColor alpha 150
##
## ══════════════ 男女预览（drawChar，逐值）══════════════
##   男：画在 (193, 215)
##   女：画在 (495, 220)
##   ★ 已知 hack（源码 TODO 原文）：
##     女性战士的 offset-y 对不齐，要 +40
##     if(gender == false && m_job == JOB_WARRIOR) drawY += 40;
##   非激活方的调制色 = RGBA(128,128,128,255)，激活方 = 白色
##   非激活方只画第 0 帧，激活方按 absFrame % frameCount 播
##
## ══════════════ 点击判定（processEvent，逐值）══════════════
##   点在 (200,290,90,260) 矩形内 -> 选男性
##   点在 (510,310,90,260) 矩形内 -> 选女性
##   任一情况都把 aniTime 归零、lastStartAbsFrame 置 UINT32_MAX
##   （强制重新触发音效）
##
## ══════════════ 音效（playMagicSoundEffect）══════════════
##   seffID = 0x00010000 | (gender << 4) | (job << 8) | 0     // 0 = create
##   在 absFrame % frameCount == 0 且与上次不同的时候播
##
## ══════════════ 提交（onSubmit）══════════════
##   CMCreateChar（70 字节）：
##     StaticBuffer<64> name @0（67）
##     u8 job    @67
##     u8 gender @68
##   name 为空或长度 >= name.capacity()(64) -> "无效的角色名"

const ProtocolDefRef := preload("res://scripts/core/protocol_def.gd")
const SelectGfxRef := preload("res://scripts/data/select_char_gfx.gd")
const ProgUseTexRef := preload("res://scripts/data/proguse_tex.gd")

const CANVAS_W := 800
const CANVAS_H := 600

const TEX_BG := 0x0D000000
const TEX_NAME_DECO := 0x0D000001
const TEX_HINT := 0x0D000002

const BTN_WARRIOR := 0x0D000030
const BTN_WIZARD := 0x0D000040
const BTN_TAOIST := 0x0D000050
const BTN_SUBMIT := 0x0D000010
const BTN_EXIT := 0x0D000020

const BTN_WARRIOR_X := 339
const BTN_WIZARD_X := 381
const BTN_TAOIST_X := 424
const JOB_BTN_Y := 539
const BTN_SUBMIT_X := 512
const BTN_EXIT_X := 554
const ACTION_BTN_Y := 549

const NAME_X := 355
const NAME_Y := 520
const NAME_W := 85
const NAME_H := 15
const NAME_BG_W := 90
const NAME_FONT_SIZE := 12

## 男女预览位置
const MALE_DRAW_X := 193
const MALE_DRAW_Y := 215
const FEMALE_DRAW_X := 495
const FEMALE_DRAW_Y := 220
## 女性战士 y 偏移（源码 TODO hack）
const FEMALE_WARRIOR_Y_FIX := 40

## 点击判定矩形（processEvent）
const MALE_CLICK := [200, 290, 90, 260]
const FEMALE_CLICK := [510, 310, 90, 260]

const FRAME_MS := 200.0

## 协议
const CM_CREATECHAR := 4
const CMCREATECHAR_SIZE := 70

## 进程 id
const PID_SELECTCHAR := 5

# ==================== 状态 ====================

var job := ProtocolDefRef.JOB_WARRIOR
var active_gender_male := true
var name_text: String = ""
var name_focused := false

var ani_time := 0.0
var last_start_abs_frame := 0xFFFFFFFF

var notify: Array = []

## 回调
var on_request: Callable = Callable()
var on_send: Callable = Callable()

var _canvas_ref = null
var hit_areas: Dictionary = {}


func _init() -> void:
	pass


func set_canvas(c) -> void:
	_canvas_ref = c


func enter() -> void:
	ani_time = 0.0
	last_start_abs_frame = 0xFFFFFFFF
	notify.clear()


func add_notify(text: String) -> void:
	notify.append({"text": text, "ms": 5000})
	if notify.size() > 10:
		notify = notify.slice(notify.size() - 10)


func abs_frame() -> int:
	return int(round(ani_time / FRAME_MS))


func frame_count(gender_male: bool) -> int:
	return SelectGfxRef.frame_count(job, gender_male, SelectGfxRef.ANIM_ON_CREATE)


# ==================== 更新 ====================

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

	# processcreatechar.cpp:update —— 整帧边界播音效
	var fc := frame_count(active_gender_male)
	if fc > 0:
		var af := abs_frame()
		if af % fc == 0 and last_start_abs_frame != af:
			play_anim_seff()
			last_start_abs_frame = af


## processcreatechar.cpp:playMagicSoundEffect
## seff = 0x00010000 | gender<<4 | job<<8 | 0  （0 = create）
func play_anim_seff() -> void:
	var sid := SelectGfxRef.anim_seff(job, active_gender_male, false)
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

	# 男女预览（先男后女，原版顺序）
	_draw_char(canvas, true, MALE_DRAW_X, MALE_DRAW_Y)
	_draw_char(canvas, false, FEMALE_DRAW_X, FEMALE_DRAW_Y)

	# 底部提示 0x0D000002，alpha 150
	var hint := ProgUseTex.retrieve(TEX_HINT)
	if hint != null:
		canvas.tex_draw_region(hint, Rect2(0, 0, hint.get_width(), hint.get_height()),
			268, 555, hint.get_width(), hint.get_height(),
			Color(1, 1, 1, 150.0 / 255.0))

	# 名字框：先黑底，再文字，最后米色框
	canvas.draw_rect_c(Color(0, 0, 0, 1), NAME_X, NAME_Y, NAME_BG_W, NAME_H, true)
	canvas.draw_string_c(name_text, NAME_X + 2, NAME_Y,
		Color(1, 0.95, 0.7), null, NAME_FONT_SIZE)
	if name_focused:
		var tw: int = int(canvas.measure_text(name_text, null, NAME_FONT_SIZE).x)
		canvas.draw_rect_c(Color(1, 1, 1, 0.8), NAME_X + 3 + tw, NAME_Y, 1, NAME_H - 2, true)
	canvas.draw_border(Color(231.0 / 255, 231.0 / 255, 189.0 / 255, 100.0 / 255),
		NAME_X, NAME_Y, NAME_BG_W, NAME_H, 1)
	hit_areas["name"] = Rect2(NAME_X, NAME_Y, NAME_BG_W, NAME_H)

	# 名字框上方装饰
	var deco := ProgUseTex.retrieve(TEX_NAME_DECO)
	if deco != null:
		canvas.tex_draw_region(deco, Rect2(0, 0, deco.get_width(), deco.get_height()),
			320, 500, deco.get_width(), deco.get_height())

	# 三个职业按钮（40x38）
	_draw_job_btn(canvas, BTN_WARRIOR, BTN_WARRIOR_X, ProtocolDefRef.JOB_WARRIOR, "warrior")
	_draw_job_btn(canvas, BTN_WIZARD, BTN_WIZARD_X, ProtocolDefRef.JOB_WIZARD, "wizard")
	_draw_job_btn(canvas, BTN_TAOIST, BTN_TAOIST_X, ProtocolDefRef.JOB_TAOIST, "taoist")

	# 提交 / 退出
	_draw_action_btn(canvas, BTN_SUBMIT, BTN_SUBMIT_X, "submit", "确定")
	_draw_action_btn(canvas, BTN_EXIT, BTN_EXIT_X, "exit", "返回")

	_draw_notify(canvas)


## processcreatechar.cpp:drawChar
func _draw_char(canvas, gender_male: bool, draw_x: int, draw_y: int) -> void:
	var fc := frame_count(gender_male)
	if fc <= 0:
		return
	var active := gender_male == active_gender_male
	var gfx_frame: int = (abs_frame() % fc) if active else 0

	# 源码 TODO hack：女性战士 y 偏移 40
	var dy_fix := 0
	if not gender_male and job == ProtocolDefRef.JOB_WARRIOR:
		dy_fix = FEMALE_WARRIOR_Y_FIX

	SelectGfxRef.draw_char(canvas, job, gender_male, SelectGfxRef.ANIM_ON_CREATE,
		gfx_frame, draw_x, draw_y + dy_fix, active)


func _draw_job_btn(canvas, tex_id: int, x: int, sel_job: int, act: String) -> void:
	var tex := ProgUseTex.retrieve(tex_id)
	if tex != null:
		canvas.tex_draw(tex, x, JOB_BTN_Y)
		hit_areas[act] = Rect2(x, JOB_BTN_Y, tex.get_width(), tex.get_height())
	else:
		canvas.draw_rect_c(Color(0.2, 0.18, 0.14, 0.95), x, JOB_BTN_Y, 40, 38, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), x, JOB_BTN_Y, 40, 38, 1)
		var lbl: String = str({"warrior": "战", "wizard": "法", "taoist": "道"}.get(act, "?"))
		canvas.draw_string_c(str(lbl), x + 12, JOB_BTN_Y + 10, Color(1, 0.95, 0.7), null, 16)
		hit_areas[act] = Rect2(x, JOB_BTN_Y, 40, 38)
	# 当前选中职业高亮
	if job == sel_job:
		canvas.draw_border(Color(1, 0.85, 0.3, 1.0), x - 1, JOB_BTN_Y - 1, 42, 40, 2)


func _draw_action_btn(canvas, tex_id: int, x: int, act: String, label: String) -> void:
	var tex := ProgUseTex.retrieve(tex_id)
	if tex != null:
		canvas.tex_draw(tex, x, ACTION_BTN_Y)
		hit_areas[act] = Rect2(x, ACTION_BTN_Y, tex.get_width(), tex.get_height())
	else:
		canvas.draw_rect_c(Color(0.2, 0.18, 0.14, 0.95), x, ACTION_BTN_Y, 28, 28, true)
		canvas.draw_border(Color(0.5, 0.42, 0.26, 1.0), x, ACTION_BTN_Y, 28, 28, 1)
		canvas.draw_string_c(label.substr(0, 1), x + 8, ACTION_BTN_Y + 6,
			Color(1, 0.95, 0.7), null, 14)
		hit_areas[act] = Rect2(x, ACTION_BTN_Y, 28, 28)


func _draw_notify(canvas) -> void:
	if notify.is_empty():
		return
	var lines: Array = []
	var bw := 0
	for n in notify:
		var t := str(n["text"])
		lines.append(t)
		bw = maxi(bw, int(canvas.measure_text(t, null, 15).x))
	var bh: int = lines.size() * 20
	var bx := (CANVAS_W - bw) / 2
	var by := (CANVAS_H - bh) / 2
	var m := 15
	canvas.draw_rect_c(Color(0, 0, 0, 128.0 / 255.0),
		bx - m, by - m, bw + m * 2, bh + m * 2, true)
	canvas.draw_border(Color(0, 0, 1, 128.0 / 255.0),
		bx - m, by - m, bw + m * 2, bh + m * 2, 1)
	var ly := by
	for ln in lines:
		canvas.draw_string_c(str(ln), bx, ly, Color(1, 1, 0), null, 15)
		ly += 20


# ==================== 交互 ====================

func handle_click(px: int, py: int) -> bool:
	# 先判男女预览区（processcreatechar.cpp:processEvent 的鼠标分支）
	var male_r := Rect2(MALE_CLICK[0], MALE_CLICK[1], MALE_CLICK[2], MALE_CLICK[3])
	var female_r := Rect2(FEMALE_CLICK[0], FEMALE_CLICK[1], FEMALE_CLICK[2], FEMALE_CLICK[3])
	if male_r.has_point(Vector2(px, py)):
		active_gender_male = true
		ani_time = 0.0
		last_start_abs_frame = 0xFFFFFFFF
		return true
	if female_r.has_point(Vector2(px, py)):
		active_gender_male = false
		ani_time = 0.0
		last_start_abs_frame = 0xFFFFFFFF
		return true

	for key in hit_areas.keys():
		var r: Rect2 = hit_areas[key]
		if px >= r.position.x and px < r.position.x + r.size.x \
				and py >= r.position.y and py < r.position.y + r.size.y:
			var k := str(key)
			match k:
				"warrior":
					job = ProtocolDefRef.JOB_WARRIOR
					ani_time = 0.0
					last_start_abs_frame = 0xFFFFFFFF
					return true
				"wizard":
					job = ProtocolDefRef.JOB_WIZARD
					ani_time = 0.0
					last_start_abs_frame = 0xFFFFFFFF
					return true
				"taoist":
					job = ProtocolDefRef.JOB_TAOIST
					ani_time = 0.0
					last_start_abs_frame = 0xFFFFFFFF
					return true
				"name":
					name_focused = true
					return true
				"submit":
					on_submit()
					return true
				"exit":
					on_exit()
					return true
	return false


func handle_key(keycode: int, unicode: int) -> bool:
	if keycode == KEY_TAB:
		name_focused = true
		return true
	if keycode == KEY_ENTER or keycode == KEY_KP_ENTER:
		on_submit()
		return true
	if not name_focused:
		return false
	if keycode == KEY_BACKSPACE:
		if not name_text.is_empty():
			name_text = name_text.substr(0, name_text.length() - 1)
		return true
	if unicode > 0:
		# 原版 name.capacity() = SYS_NAMESIZE = 64，超长直接不接
		if name_text.length() < 63:
			name_text += char(unicode)
		return true
	return false


## processcreatechar.cpp:onSubmit
func on_submit() -> void:
	if name_text.is_empty() or name_text.length() >= 64:
		add_notify("无效的角色名")
		return
	var body := PackedByteArray()
	body.resize(CMCREATECHAR_SIZE)
	body.fill(0)
	var bytes := name_text.to_utf8_buffer()
	var n: int = mini(bytes.size(), 64)
	body.encode_u16(0, n)
	for i in range(n):
		body[2 + i] = bytes[i]
	# ★ CM_CREATECHAR = StaticBuffer<64> name + u8 job + u8 gender
	#   StaticBuffer 占 68 字节（对齐），所以 job 在 68、gender 在 69。
	body[68] = job
	body[69] = 1 if active_gender_male else 0
	if on_send.is_valid():
		on_send.call(CM_CREATECHAR, body)
	add_notify("提交中")


## processcreatechar.cpp:onExit
func on_exit() -> void:
	if on_request.is_valid():
		on_request.call(PID_SELECTCHAR)


# ==================== 服务端应答 ====================

func on_create_char_ok() -> void:
	add_notify("角色创建成功")
	if on_request.is_valid():
		on_request.call(PID_SELECTCHAR)


func on_create_char_error(code: int) -> void:
	var text := "创建失败"
	match code:
		0: text = "角色名已存在"
		1: text = "角色名不合法"
		2: text = "已达角色上限"
		_: text = "创建失败（错误码 %d）" % code
	add_notify(text)
