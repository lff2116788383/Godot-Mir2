extends RefCounted
class_name XmlTypesetBuilder
## 把 XmlLayout 解析结果转成排版 Token 并完成排版
##
## 【逐行翻译】client/src/xmltypeset.cpp:624-780
##   buildUTF8Token(leafIndex, font, fontSize, fontStyle, codePoint)
##     - 尺寸来自字体纹理（fontexDB）
##     - w1/w2 是左右自然留白，h1/h2 是基线上下的高度
##   buildEmojiToken(leafIndex, emoji)
##     - 尺寸来自 emoji 图库（emojiDB）
##     - 带 frameCount 与 delay（emoji 是动画）
##   createToken(leafIndex, leafOff)  按 leaf 类型分派
##
## Godot 差异：
##   - 字体度量用 Font.get_string_size / get_ascent
##   - emoji 贴图从 assets/emoji 目录读（文件名 = emoji_id 的 11 位 hex + 帧号）

const LayoutRef := preload("res://scripts/ui/xml_layout.gd")
const TokenRef := preload("res://scripts/ui/xml_token.gd")
const TypesetRef := preload("res://scripts/ui/xml_typeset.gd")

## 排版参数（对应 XMLTypeset::InitArgs）
var line_width := 400
var line_align := 0        ## LALIGN_LEFT
var line_space := 0
var word_space := 0
var line_margin := [0, 0]
var font_size := 15

## 字体
var font: Font = null
## emoji 相关
var emoji_root := ""
## emoji 元数据缓存：emoji_id -> {count, delay, h1, w, h}
var _emoji_meta: Dictionary = {}


func setup(p_line_width: int, p_font: Font = null, p_font_size: int = 15,
		p_emoji_root: String = "") -> void:
	line_width = p_line_width
	font = p_font
	font_size = p_font_size
	emoji_root = p_emoji_root
	if font == null:
		font = ThemeDB.fallback_font


## 主入口：XML 字符串 -> 排版好的 XmlTypeset
func build(xml_string: String, origin_x: int = 0, origin_y: int = 0):
	var lay := LayoutRef.new().parse(xml_string)
	return build_from_layout(lay, origin_x, origin_y)


## 从已解析的 XmlLayout 构建
func build_from_layout(lay, origin_x: int = 0, origin_y: int = 0):
	var ts = TypesetRef.new()
	ts.setup(line_width, line_align, line_space, word_space, line_margin[0], line_margin[1])

	# 初始化第一行
	ts.m_line_list = [{"start_y": 0, "content": []}]
	var cur_line := 0

	for pi in range(lay.paragraphs.size()):
		# 段落之间强制换行（第一段除外）
		if pi > 0:
			cur_line = ts.new_line()

		var par: Dictionary = lay.paragraphs[pi]
		var tokens := _build_tokens(par)
		for tk in tokens:
			# 先尝试放进当前行；放不下就换行
			# 注意 add_raw_token_line 会检查行宽，返回 false 表示超宽
			if not ts.add_raw_token_line(cur_line, [tk]):
				cur_line = ts.new_line()
				ts.add_raw_token_line(cur_line, [tk])

	ts.build_typeset(origin_x, origin_y)
	return ts


## 一个段落 -> Token 数组
func _build_tokens(par: Dictionary) -> Array:
	var out: Array = []
	var leaves: Array = par.get("leaves", [])
	for li in range(leaves.size()):
		var leaf: Dictionary = leaves[li]
		match int(leaf.get("type", 0)):
			LayoutRef.LEAF_UTF8STR:
				out.append_array(_build_utf8_tokens(li, str(leaf.get("text", "")), leaf.get("attrs", {})))
			LayoutRef.LEAF_EMOJI:
				var tk = _build_emoji_token(li, int(leaf.get("id", 0)), int(leaf.get("set", 0)))
				if tk != null:
					out.append(tk)
			LayoutRef.LEAF_IMAGE:
				var tk2 = _build_image_token(li, str(leaf.get("id", "")))
				if tk2 != null:
					out.append(tk2)
	return out


## 字符串 -> 每个字符一个 Token（对应 createTokenLine 的逐字符处理）
func _build_utf8_tokens(leaf_idx: int, text: String, attrs: Dictionary) -> Array:
	var out: Array = []
	var f: Font = font
	var fs := font_size
	# 叶子属性可覆盖字号
	if attrs.has("size"):
		var s := str(attrs["size"])
		if s.is_valid_int():
			fs = s.to_int()

	for i in range(text.length()):
		var ch := text[i]
		var tk = TokenRef.new()
		tk.kind = TokenRef.KIND_UTF8CHAR
		tk.leaf = leaf_idx
		tk.key = ch.unicode_at(0)
		# 字体度量（对应 fontexDB->retrieve 的 boxW/boxH/left/right/ascent）
		var sz: Vector2 = f.get_string_size(ch, HORIZONTAL_ALIGNMENT_LEFT, -1, fs)
		var ascent: float = f.get_ascent(fs)
		var descent: float = f.get_descent(fs)
		tk.set_size(int(ceil(sz.x)), int(ceil(ascent + descent)))
		tk.set_heights(int(ceil(ascent)), int(ceil(descent)))
		out.append(tk)
	return out


## emoji token（对应 buildEmojiToken）
func _build_emoji_token(leaf_idx: int, emoji_id: int, emoji_set: int) -> XmlToken:
	if emoji_id < 0:
		return null
	var meta := _get_emoji_meta(emoji_id, emoji_set)
	if meta.is_empty():
		# 没有贴图时退化成占位方块，避免整段排版失败
		var tk = TokenRef.new()
		tk.kind = TokenRef.KIND_EMOJI
		tk.leaf = leaf_idx
		tk.key = emoji_id
		tk.set_meta("emoji_set", emoji_set)
		tk.set_meta("fallback", true)
		var sz := font_size
		tk.set_size(sz, sz)
		tk.set_heights(sz, 0)
		return tk

	var tk2 = TokenRef.new()
	tk2.kind = TokenRef.KIND_EMOJI
	tk2.leaf = leaf_idx
	tk2.key = emoji_id
	tk2.set_meta("emoji_set", emoji_set)
	tk2.emoji_frame_count = int(meta.get("count", 1))
	tk2.emoji_delay = int(meta.get("delay", 100))
	tk2.set_size(int(meta.get("w", 0)), int(meta.get("h", 0)))
	tk2.set_heights(int(meta.get("h1", 0)), int(meta.get("h", 0)) - int(meta.get("h1", 0)))
	return tk2


## image token（对应 LEAF_IMAGE）
func _build_image_token(leaf_idx: int, img_id: String) -> XmlToken:
	if img_id.is_empty():
		return null
	var tk = TokenRef.new()
	tk.kind = TokenRef.KIND_IMAGE
	tk.leaf = leaf_idx
	# image id 如果是数字就当贴图 ID
	if img_id.is_valid_int():
		tk.key = img_id.to_int()
	tk.set_size(font_size, font_size)
	tk.set_heights(font_size, 0)
	return tk


## 读 emoji 元数据（首次读目录，之后缓存）
## 素材命名：emoji/{11位hex}.PNG，帧号在目录里按序
func _get_emoji_meta(emoji_id: int, emoji_set: int) -> Dictionary:
	var key := "%d_%d" % [emoji_set, emoji_id]
	if _emoji_meta.has(key):
		return _emoji_meta[key]

	if emoji_root.is_empty():
		_emoji_meta[key] = {}
		return {}

	var dir := emoji_root
	if not DirAccess.dir_exists_absolute(dir):
		_emoji_meta[key] = {}
		return {}

	# 文件名前 11 位是 emoji_id 的十六进制（实测 00000000016.PNG）
	var prefix := "%011X" % emoji_id
	var files: Array[String] = []
	var da := DirAccess.open(dir)
	if da == null:
		_emoji_meta[key] = {}
		return {}
	da.list_dir_begin()
	var fn := da.get_next()
	while fn != "":
		if fn.to_upper().ends_with(".PNG") and fn.begins_with(prefix):
			files.append(fn)
		fn = da.get_next()
	da.list_dir_end()

	if files.is_empty():
		_emoji_meta[key] = {}
		return {}

	files.sort()
	var first := "%s/%s" % [dir, files[0]]
	var img := Image.load_from_file(first)
	if img == null:
		_emoji_meta[key] = {}
		return {}

	var meta := {
		"count": files.size(),
		"delay": 100,
		"w": img.get_width(),
		"h": img.get_height(),
		"h1": img.get_height(),
		"files": files,
	}
	_emoji_meta[key] = meta
	return meta


## emoji 某帧的贴图路径
func emoji_frame_path(emoji_id: int, emoji_set: int, frame: int) -> String:
	var meta := _get_emoji_meta(emoji_id, emoji_set)
	if meta.is_empty():
		return ""
	var files: Array = meta.get("files", [])
	if files.is_empty():
		return ""
	var idx: int = clampi(frame, 0, files.size() - 1)
	return "%s/%s" % [emoji_root, str(files[idx])]
