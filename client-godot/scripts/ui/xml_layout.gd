extends RefCounted
class_name XmlLayout
## NPC 对话 XML 解析器
##
## 【逐行翻译】client/src/xmlparagraph.cpp:320-410  loadXML / loadXMLNode
##            common/src/xmlf.cpp:218-250          validTagName / validAttributeName
##
## ── 协议里的实际格式（实测 server/script/npc/*.lua）──
##   <layout><par>请输入行会的名字</par></layout>
##   <layout><par>你好<emoji id="0"/>欢迎</par></layout>
##
## ── 叶子节点三种类型（xmlparagraphleaf.hpp:14-16）──
##   LEAF_UTF8STR = 0  纯文字
##   LEAF_IMAGE   = 1  <image id="..."/>
##   LEAF_EMOJI   = 2  <emoji id="..." set="..."/>
##
## ── 叶子可带属性（xmlparagraphleaf 的 color/bgColor/font/fontSize）──
##   color / bgcolor / font / size / wrap / single（点击事件）
##
## Godot 差异：用内置 XMLParser 代替 tinyxml2，输出统一成
##   { type, text/id/set, attrs:{} } 的 Dictionary 列表，交给 XmlTypeset 排版。

const LEAF_UTF8STR := 0
const LEAF_IMAGE := 1
const LEAF_EMOJI := 2

# ==================== 解析结果 ====================

## 段落列表：每个段落是 { attrs: {}, leaves: [leaf...] }
var paragraphs: Array = []
## 外层 layout 属性（对齐、行宽等）
var layout_attrs: Dictionary = {}


## 主入口：解析 XML 字符串
## 返回 self，便于链式调用
func parse(xml_string: String) -> XmlLayout:
	paragraphs.clear()
	layout_attrs.clear()

	if xml_string.strip_edges().is_empty():
		return self

	var parser := XMLParser.new()
	var err := parser.open_buffer(xml_string.to_utf8_buffer())
	if err != OK:
		push_warning("XML 解析器打开失败: err=%d" % err)
		return self

	var cur_par: Dictionary = {}
	var in_par := false
	var in_layout := false
	var pending_text := ""

	while true:
		var e := parser.read()
		if e == ERR_FILE_EOF or e == ERR_PARSE_ERROR:
			break

		match parser.get_node_type():
			XMLParser.NODE_ELEMENT:
				var tag := parser.get_node_name().to_lower()
				var attrs := _read_attrs(parser)

				if tag == "layout":
					in_layout = true
					layout_attrs = attrs
				elif tag == "par":
					_flush_text(cur_par, pending_text)
					pending_text = ""
					cur_par = {"attrs": attrs, "leaves": []}
					in_par = true
				elif tag == "emoji":
					_flush_text(cur_par, pending_text)
					pending_text = ""
					if in_par:
						cur_par["leaves"].append({
							"type": LEAF_EMOJI,
							"id": _attr_int(attrs, "id", 0),
							"set": _attr_int(attrs, "set", 0),
							"attrs": attrs,
						})
				elif tag == "image":
					_flush_text(cur_par, pending_text)
					pending_text = ""
					if in_par:
						cur_par["leaves"].append({
							"type": LEAF_IMAGE,
							"id": _attr_str(attrs, "id", ""),
							"attrs": attrs,
						})
				elif tag == "text":
					# <text 属性>内容</text>：属性作用于内容
					_flush_text(cur_par, pending_text)
					pending_text = ""
					var inner := _read_element_text(parser)
					if in_par:
						cur_par["leaves"].append({
							"type": LEAF_UTF8STR,
							"text": inner,
							"attrs": attrs,
						})
				# 自闭合元素（<emoji id="0"/>）不需要额外处理

			XMLParser.NODE_TEXT:
				var t := parser.get_node_data()
				if not t.is_empty():
					pending_text += t

			XMLParser.NODE_ELEMENT_END:
				var tag_end := parser.get_node_name().to_lower()
				if tag_end == "par" and in_par:
					_flush_text(cur_par, pending_text)
					pending_text = ""
					paragraphs.append(cur_par)
					cur_par = {}
					in_par = false
				elif tag_end == "layout":
					in_layout = false

	# 兜底：没有 <par> 包裹的纯文字也当一段
	if pending_text.strip_edges() != "":
		paragraphs.append({
			"attrs": {},
			"leaves": [{"type": LEAF_UTF8STR, "text": pending_text, "attrs": {}}],
		})

	if paragraphs.is_empty() and not pending_text.is_empty():
		paragraphs.append({
			"attrs": {},
			"leaves": [{"type": LEAF_UTF8STR, "text": pending_text, "attrs": {}}],
		})

	return self


func _flush_text(par: Dictionary, text: String) -> void:
	if text.is_empty() or par.is_empty():
		return
	if not par.has("leaves"):
		return
	par["leaves"].append({"type": LEAF_UTF8STR, "text": text, "attrs": {}})


## 读当前元素的全部属性
func _read_attrs(parser: XMLParser) -> Dictionary:
	var d := {}
	var n := parser.get_attribute_count()
	for i in range(n):
		d[parser.get_attribute_name(i).to_lower()] = parser.get_attribute_value(i)
	return d


## 读取 <tag>内容</tag> 的文本（消费到对应结束标签）
func _read_element_text(parser: XMLParser) -> String:
	var out := ""
	var depth := 1
	while depth > 0:
		var e := parser.read()
		if e == ERR_FILE_EOF or e == ERR_PARSE_ERROR:
			break
		match parser.get_node_type():
			XMLParser.NODE_TEXT:
				out += parser.get_node_data()
			XMLParser.NODE_ELEMENT:
				# 自闭合元素当作无内容
				if not parser.is_empty():
					depth += 1
			XMLParser.NODE_ELEMENT_END:
				depth -= 1
	return out


func _attr_int(d: Dictionary, key: String, fallback: int) -> int:
	if not d.has(key):
		return fallback
	var v = d[key]
	if v is int:
		return v
	var s := str(v)
	if s.is_valid_int():
		return s.to_int()
	return fallback


func _attr_str(d: Dictionary, key: String, fallback: String) -> String:
	return str(d.get(key, fallback))


# ==================== 辅助 ====================

## 段落纯文字（去掉 emoji/image），用于日志或按钮文字
func plain_text(par_index: int = 0) -> String:
	if par_index < 0 or par_index >= paragraphs.size():
		return ""
	var s := ""
	for leaf in paragraphs[par_index].get("leaves", []):
		if int(leaf.get("type", -1)) == LEAF_UTF8STR:
			s += str(leaf.get("text", ""))
	return s


## 全文纯文字
func all_plain_text() -> String:
	var out: Array[String] = []
	for i in range(paragraphs.size()):
		out.append(plain_text(i))
	return "\n".join(out)


## 统计叶子数量
func leaf_count() -> int:
	var n := 0
	for p in paragraphs:
		n += int(p.get("leaves", []).size())
	return n


## 便捷入口：一句话转成单段落 XML（对应 xmlf::toParString）
static func single_par(text: String) -> String:
	return "<layout><par>%s</par></layout>" % _escape(text)


static func _escape(s: String) -> String:
	s = s.replace("&", "&amp;")
	s = s.replace("<", "&lt;")
	s = s.replace(">", "&gt;")
	return s


func to_str() -> String:
	return "XmlLayout(段落=%d, 叶子=%d)" % [paragraphs.size(), leaf_count()]
