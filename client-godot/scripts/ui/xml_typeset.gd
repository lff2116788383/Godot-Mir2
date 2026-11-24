extends RefCounted
class_name XmlTypeset
## XML 排版引擎（NPC 对话框、聊天、物品描述全靠它）
##
## 【逐行翻译】client/src/xmltypeset.cpp（1730 行）+ xmltypeset.hpp（545 行）
## 对照关键函数：
##   - xmltypeset.cpp:37   LineFullWidth        行内 token 总宽
##   - xmltypeset.cpp:50   LineTargetWidth      目标行宽 = 最大行宽 + 左右边距
##   - xmltypeset.cpp:60   getTokenNaturalPadding  自然留白（含词间距）
##   - xmltypeset.cpp:76   addRawTokenLine      加入 token 并判是否换行
##   - xmltypeset.cpp:129  LinePadding          两端对齐填充
##   - xmltypeset.cpp:350  setLineTokenStartX   按对齐方式算起始 X
##   - xmltypeset.cpp:511  LineNewStartY        换行后的新 Y
##   - xmltypeset.cpp:603  setLineTokenStartY   设置 token 的 Y
##
## 本文件翻译「排版核心」：换行判定、对齐、坐标计算。
## 不包含：XML 解析（xmlf）、字体纹理查询（g_fontexDB）、绘制。

const TokenRef := preload("res://scripts/ui/xml_token.gd")

# ---------------- 对齐（lalign.hpp） ----------------
const LALIGN_NONE := 0
const LALIGN_BEGIN := 1
const LALIGN_LEFT := 1
const LALIGN_RIGHT := 2
const LALIGN_CENTER := 3
const LALIGN_JUSTIFY := 4
const LALIGN_DISTRIBUTED := 5
const LALIGN_END := 6

# ---------------- leaf 类型（xmlparagraphleaf.hpp:14） ----------------
const LEAF_UTF8STR := 0
const LEAF_IMAGE := 1
const LEAF_EMOJI := 2

# ---------------- 配置（InitArgs） ----------------
var line_width: int = 0          ## 不含 lineMargin
var line_align: int = LALIGN_LEFT
var can_through: bool = true
var compact_line: bool = false
var allow_extend: bool = true
var line_space: int = 0
var word_space: int = 0
## 左右边距 [左, 右]，替换首 token 的 W1 与末 token 的 W2
var line_margin: Array[int] = [0, 0]

# ---------------- 数据 ----------------
## 每行：{start_y, content: Array[Token]}
var m_line_list: Array = []
## 全文最大行宽
var m_max_line_width: int = 0


func setup(lw: int, align: int = LALIGN_LEFT, space: int = 0, wspace: int = 0,
		margin_left: int = 0, margin_right: int = 0) -> void:
	line_width = lw
	line_align = align
	line_space = space
	word_space = wspace
	line_margin = [margin_left, margin_right]


# ---------------- 行工具 ----------------

func line_valid(i: int) -> bool:
	return i >= 0 and i < m_line_list.size()


func line_count() -> int:
	return m_line_list.size()


func line_token_count(i: int) -> int:
	if not line_valid(i):
		return 0
	return m_line_list[i]["content"].size()


func get_token(idx: int, line: int):
	if not line_valid(line):
		return null
	var c: Array = m_line_list[line]["content"]
	if idx < 0 or idx >= c.size():
		return null
	return c[idx]


## LineFullWidth（xmltypeset.cpp:37）
func line_full_width(line: int) -> int:
	if not line_valid(line):
		return 0
	var total := 0
	for t in m_line_list[line]["content"]:
		total += t.full_width()
	return total


## LineTargetWidth（xmltypeset.cpp:50）
func line_target_width() -> int:
	return m_max_line_width + line_margin[0] + line_margin[1]


## getTokenNaturalPadding（xmltypeset.cpp:60）
## 返回 [左留白, 右留白]
## 原版从字体库查字符的实际左右空边；这里按 word_space 均分
## （字体纹理查询属于 L1 资源层，B9 接入真实的左右空边）
func get_token_natural_padding(token) -> Array:
	var left := 0
	var right := 0
	if token.kind == TokenRef.KIND_UTF8CHAR:
		# 原版：g_fontexDB->retrieve(keyXfer, &left, &right)
		# 当前按 0 处理，等字体模块接入后填真实值
		left = 0
		right = 0
	return [
		left + int((word_space + 0) / 2),
		right + int((word_space + 1) / 2),
	]


## addRawTokenLine（xmltypeset.cpp:76）
## 把 tokenLine 加入指定行；返回 false 表示放不下需换行
func add_raw_token_line(line: int, token_line: Array) -> bool:
	if not line_valid(line):
		return false
	if token_line.is_empty():
		return false

	# lineWidth <= 0 表示不限宽（单行模式）
	if line_width <= 0:
		m_line_list[line]["content"].append_array(token_line)
		return true

	# 按 resetOneLine 的口径测量：首 token 的 W1 与末 token 的 W2 被 lineMargin 替代
	var exist: Array = m_line_list[line]["content"]
	var total_count := exist.size() + token_line.size()
	var raw_width := 0
	var token_index := 0
	var merged := exist.duplicate()
	merged.append_array(token_line)

	for t in merged:
		var pad: Array = get_token_natural_padding(t)
		raw_width += t.w
		raw_width += 0 if token_index == 0 else int(pad[0])
		raw_width += 0 if token_index + 1 == total_count else int(pad[1])
		token_index += 1

	if raw_width > line_width:
		if exist.size() > 0:
			return false    # 当前行已有内容，需换行
		# 单个 token 就超宽
		if allow_extend:
			push_warning("XMLTypeset 宽度不足: lineWidth=%d rawWidth=%d" % [line_width, raw_width])
		else:
			push_error("XMLTypeset 宽度不足: lineWidth=%d rawWidth=%d" % [line_width, raw_width])

	m_line_list[line]["content"].append_array(token_line)
	return true


## LinePadding（xmltypeset.cpp:129）
## 两端对齐：把剩余宽度平均分配到各 token 的 W1/W2，但首 W1 与末 W2 不动
## 返回是否做了填充
func line_padding(line: int) -> bool:
	if not line_valid(line):
		return false
	if line_align != LALIGN_JUSTIFY and line_align != LALIGN_DISTRIBUTED:
		return false

	var n := line_token_count(line)
	if n == 0:
		return false
	if n == 1:
		return false

	var target := line_target_width()
	if target <= 0:
		return false
	if line_full_width(line) >= target:
		return false

	# 逐轮加宽，直到达到目标宽度
	var guard := 0
	while line_full_width(line) < target and guard < 10000:
		guard += 1
		var curr_diff := target - line_full_width(line)
		var done_diff := curr_diff

		for i in range(n):
			var t = get_token(i, line)
			if t == null:
				continue
			# 选较小的一侧加宽，保持 w1/w2 平衡
			# 若该侧是冻结边（首 token 的 w1、末 token 的 w2），则换另一侧
			var is_first := (i == 0)
			var is_last := (i == n - 1)
			var prefer_w1: bool = t.w1 <= t.w2

			var step := 0
			if prefer_w1 and not is_first:
				t.w1 += 1
				step = 1
			elif not is_last:
				t.w2 += 1
				step = 1
			elif is_first and not is_last:
				t.w2 += 1
				step = 1
			elif not is_first:
				t.w1 += 1
				step = 1

			done_diff -= step
			if done_diff == 0:
				break
		if done_diff == done_diff and done_diff >= curr_diff:
			break    # 无法再填充（都是冻结边）
	return true


## setLineTokenStartX（xmltypeset.cpp:350）
## 按对齐方式计算每个 token 的 x
func set_line_token_start_x(line: int, full_width: int) -> void:
	if not line_valid(line):
		return

	var start_x := 0
	var lw := line_full_width(line)

	match line_align:
		LALIGN_RIGHT:
			start_x = maxi(0, full_width - lw)
		LALIGN_CENTER:
			start_x = maxi(0, (full_width - lw) / 2)
		LALIGN_DISTRIBUTED:
			if line_token_count(line) == 1:
				start_x = maxi(0, (full_width - lw) / 2)
		_:
			start_x = 0

	var cur_x := start_x
	for t in m_line_list[line]["content"]:
		cur_x += t.w1
		t.x = cur_x
		cur_x += t.w
		cur_x += t.w2


## LineIntervalMaxH2（xmltypeset.cpp:392）
## 给定水平区间，取该区间内 token 的最大 H2；无 token 返回 -1
func line_interval_max_h2(line: int, itv_start_x: int, itv_width: int) -> int:
	if not line_valid(line):
		return -1
	var result := -1
	for t in m_line_list[line]["content"]:
		var t_start: int = t.x - t.w1
		var t_end: int = t.x + t.w + t.w2
		# 区间重叠判定
		if t_end > itv_start_x and t_start < itv_start_x + itv_width:
			result = maxi(result, t.h2)
	return result


## LineTokenBestY（xmltypeset.cpp:470）
## 为 token 找最佳 Y：在水平区间内取最大 H2 作为基线偏移
func line_token_best_y(line: int, token_x: int, token_w: int, token_h: int) -> int:
	var max_h2 := line_interval_max_h2(line, token_x, token_w)
	if max_h2 < 0:
		max_h2 = 0
	# 基线 = 该区间最大 H2，token 顶部 = 基线 - token.h1
	return max_h2


## LineNewStartY（xmltypeset.cpp:511）
## 换行后新行的起始 Y
func line_new_start_y(line: int) -> int:
	if not line_valid(line) or m_line_list[line]["content"].is_empty():
		if line == 0:
			return 0
		# 沿用上一行的结束 Y
		if line - 1 >= 0 and line_valid(line - 1):
			return _line_bottom(line - 1) + line_space
		return 0

	var max_h1 := 0
	var max_h2 := 0
	for t in m_line_list[line]["content"]:
		max_h1 = maxi(max_h1, t.h1)
		max_h2 = maxi(max_h2, t.h2)
	return max_h1 + max_h2 + line_space


## setLineTokenStartY（xmltypeset.cpp:603）
## 按行的 startY 与 token 的 h1/h2 计算每个 token 的 y
func set_line_token_start_y(line: int) -> void:
	if not line_valid(line):
		return
	var start_y: int = int(m_line_list[line]["start_y"])
	for t in m_line_list[line]["content"]:
		# token 的 y 是锚点，等于行基线 - h1
		t.y = start_y - t.h1


## resetOneLine（xmltypeset.cpp:264）
## 重置一行的 token 状态（W1/W2/X/Y 归零，重新按 word_space 分配）
func reset_one_line(line: int, cr_end: bool = true) -> void:
	if not line_valid(line):
		return
	var content: Array = m_line_list[line]["content"]
	var n := content.size()
	for i in range(n):
		var t = content[i]
		var pad: Array = get_token_natural_padding(t)
		# xmltypeset.cpp:276-277：首 token 的左留白用 lineMargin[0]，末 token 的右留白用 lineMargin[1]
		t.w1 = line_margin[0] if i == 0 else int(pad[0])
		t.w2 = line_margin[1] if i + 1 == n else int(pad[1])
		t.x = 0
		t.y = 0


# ---------------- 整体排版 ----------------

## 追加一行（自动推进 Y）
func new_line(start_y: int = 0) -> int:
	var y := start_y
	if not m_line_list.is_empty():
		y = _line_bottom(m_line_list.size() - 1) + line_space
	m_line_list.append({"start_y": y, "content": []})
	return m_line_list.size() - 1


## 一行底部的 Y
func _line_bottom(line: int) -> int:
	if not line_valid(line):
		return 0
	var start_y: int = int(m_line_list[line]["start_y"])
	var max_h2 := 0
	for t in m_line_list[line]["content"]:
		max_h2 = maxi(max_h2, t.h2)
	return start_y + max_h2


## buildTypeset（xmltypeset.cpp:773）
## 对已加入的所有 token 做最终排版：算 X/Y
func build_typeset(origin_x: int = 0, origin_y: int = 0) -> void:
	if m_line_list.is_empty():
		return

	# 先算最大行宽
	m_max_line_width = 0
	for i in range(m_line_list.size()):
		reset_one_line(i)
		m_max_line_width = maxi(m_max_line_width, _calc_raw_width(i))

	# xmltypeset.cpp:313-340：fullWidth 初始 = LineTargetWidth()，
	# 右对齐/居中时再取 max(fullWidth, 本行实际宽)，避免超宽行错位
	var base_width := line_target_width()

	# 逐行设置 X 与 Y
	for i in range(m_line_list.size()):
		if line_align == LALIGN_JUSTIFY or line_align == LALIGN_DISTRIBUTED:
			line_padding(i)

		var full_width := base_width
		if allow_extend and (line_align == LALIGN_RIGHT or line_align == LALIGN_CENTER):
			full_width = maxi(full_width, line_full_width(i))

		set_line_token_start_x(i, full_width)
		set_line_token_start_y(i)

		# 加上原点偏移
		for t in m_line_list[i]["content"]:
			t.x += origin_x
			t.y += origin_y


## 计算行的原始宽度（不含首 W1 与末 W2）
func _calc_raw_width(line: int) -> int:
	if not line_valid(line):
		return 0
	var content: Array = m_line_list[line]["content"]
	var n := content.size()
	var width := 0
	for i in range(n):
		var t = content[i]
		width += t.w
		if i > 0:
			width += t.w1
		if i + 1 < n:
			width += t.w2
	return width


# ---------------- 尺寸查询 ----------------

## fw（全文宽）
func fw() -> int:
	var m := 0
	for i in range(m_line_list.size()):
		m = maxi(m, line_full_width(i))
	return m + line_margin[0] + line_margin[1]


## fh（全文高）
func fh() -> int:
	if m_line_list.is_empty():
		return 0
	return _line_bottom(m_line_list.size() - 1)


## 纯文本内容（便于测试）
func to_plain_text() -> String:
	var sb := ""
	for line in m_line_list:
		for t in line["content"]:
			if t.kind == TokenRef.KIND_UTF8CHAR and t.key != 0:
				sb += String.chr(t.key)
		sb += "\n"
	return sb
