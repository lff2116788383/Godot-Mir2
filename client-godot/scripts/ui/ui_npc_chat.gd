extends UiWidget
class_name UiNpcChat
## NPC 对话面板
##
## 【逐行翻译】client/src/gui/npcchatboard/npcchatboard.hpp + npcchatboard.cpp
## 对照：
##   - npcchatboard.hpp:12   m_margin = 35
##   - npcchatboard.hpp:24   m_bg / m_face / m_chatBoard / m_buttonClose 四件套
##   - LayoutBoard 负责把 XML 排成文字（我的 XmlTypesetBuilder）
##
## 消息来源：SM_NPCXMLLAYOUT（type 3，cerealf 序列化）
##   SDNPCChat { u64 npcUID; string xmlLayout; string eventPath; }
##
## XML 格式实测（server/script/npc/*.lua）：
##   <layout><par>请输入行会的名字</par></layout>
##   <layout><par>你好<emoji id="0"/>欢迎</par></layout>

const RoiRef := preload("res://scripts/ui/framework/ui_roi.gd")
const WidgetRef := preload("res://scripts/ui/framework/ui_widget.gd")
const ButtonRef := preload("res://scripts/ui/framework/ui_button.gd")
const BuilderRef := preload("res://scripts/ui/xml_typeset_builder.gd")
const LayoutRef := preload("res://scripts/ui/xml_layout.gd")
const TokenRef := preload("res://scripts/ui/xml_token.gd")

## 边距（npcchatboard.hpp:12）
const MARGIN := 35

var npc_uid: int = 0
var event_path: String = ""
## 原始 XML
var xml_layout: String = ""

## 排版结果
var _typeset = null
## 排版构建器
var _builder: BuilderRef = null
## 画布引用（用于文字绘制）
var _canvas = null

## 点击正文回调：Callable(widget, event_path)
## 注意不能叫 on_event —— 父类 UiWidget 的 on_event 是输入事件处理器
var on_npc_event: Callable = Callable()
var on_close: Callable = Callable()

## 背景色与边框（原版用 m_bg 贴图，素材缺失时用纯色）
var bg_color := Color(0.08, 0.07, 0.06, 0.96)
var border_color := Color(0.62, 0.52, 0.3, 1.0)
var text_color := Color(0.95, 0.92, 0.82)


func _init(p_name: String = "NpcChat") -> void:
	widget_name = p_name
	w = 460
	h = 180
	show = false
	on_event = _handle_event


## 设置排版器（由外部注入 canvas，用于字体度量）
func set_canvas(canvas) -> void:
	_canvas = canvas
	_builder = BuilderRef.new()
	_builder.setup(w - MARGIN * 2,
		canvas.default_font if canvas != null else null,
		canvas.default_font_size if canvas != null else 15,
		"")


## 显示一段 XML（对应 on_SM_NPCXMLLAYOUT）
func show_layout(uid: int, xml_str: String, evt_path: String = "") -> void:
	npc_uid = uid
	xml_layout = xml_str
	event_path = evt_path
	show = true
	_rebuild()
	# 高度按实际行数自适应
	if _typeset != null:
		h = int(_typeset.fh()) + MARGIN * 2 + 40


## 兼容旧名
func set_xml(uid: int, xml_str: String, evt_path: String = "") -> void:
	show_layout(uid, xml_str, evt_path)


func _rebuild() -> void:
	if _builder == null:
		_builder = BuilderRef.new()
		_builder.setup(w - MARGIN * 2,
			_canvas.default_font if _canvas != null else null,
			_canvas.default_font_size if _canvas != null else 15,
			"")
	_typeset = _builder.build(xml_layout if not xml_layout.is_empty() else "<layout><par></par></layout>", 0, 0)


## 绘制
func draw_self(canvas, my_roi) -> void:
	if _typeset == null:
		return
	var t = canvas
	# 背景
	t.draw_rect_c(bg_color, my_roi.x, my_roi.y, my_roi.w, my_roi.h, true)
	t.draw_border(border_color, my_roi.x, my_roi.y, my_roi.w, my_roi.h, 2)
	# 内层装饰线
	t.draw_border(Color(0.35, 0.3, 0.18, 0.9), my_roi.x + 4, my_roi.y + 4,
		my_roi.w - 8, my_roi.h - 8, 1)
	# 关闭按钮（右上角）
	t.draw_string_c("×", my_roi.x + my_roi.w - 20, my_roi.y + 6,
		Color(0.9, 0.75, 0.5), null, 16)

	# 文字（按排版结果的坐标画）
	var ox: int = my_roi.x + MARGIN
	var oy: int = my_roi.y + MARGIN
	var f: Font = canvas.default_font
	var fs: int = canvas.default_font_size

	for li in range(_typeset.line_count()):
		var cnt: int = _typeset.line_token_count(li)
		for ti in range(cnt):
			var tk = _typeset.get_token(ti, li)
			if tk == null:
				continue
			var tx: int = ox + int(tk.x) + int(tk.w1)
			var ty: int = oy + int(tk.y)
			match int(tk.kind):
				TokenRef.KIND_UTF8CHAR:
					var ch := char(int(tk.key))
					t.draw_string_shadow(ch, tx, ty, text_color, Color(0, 0, 0, 0.9), 1, f, fs)
				TokenRef.KIND_EMOJI:
					# emoji 贴图：素材在 assets/emoji，这里画占位框（后续接 emoji_loader）
					t.draw_rect_c(Color(0.5, 0.45, 0.25, 0.8), tx, ty, int(tk.w), int(tk.h), true)
				TokenRef.KIND_IMAGE:
					t.draw_rect_c(Color(0.3, 0.35, 0.5, 0.8), tx, ty, int(tk.w), int(tk.h), true)


## 事件：点关闭按钮关闭，点正文触发 event_path
func _handle_event(self_widget, event: InputEvent, valid: bool, my_roi) -> bool:
	if not show:
		return false
	if event is InputEventMouseButton and event.button_index == MOUSE_BUTTON_LEFT and event.pressed:
		var mpos: Vector2 = event.position
		if not my_roi.contains(int(mpos.x), int(mpos.y)):
			return false
		# 右上角关闭区
		if mpos.x > my_roi.x + my_roi.w - 28 and mpos.y < my_roi.y + 28:
			show = false
			if on_close.is_valid():
				on_close.call(self)
			return true
		# 正文点击 -> 下发 NPC 事件
		if not event_path.is_empty() and on_npc_event.is_valid():
			on_npc_event.call(self, event_path)
			return true
		return true
	return false


func is_open() -> bool:
	return WidgetRef.eval_bool(show, self, false)
