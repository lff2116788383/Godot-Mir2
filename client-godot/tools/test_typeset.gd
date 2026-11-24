extends SceneTree
## B8 自检：XML 排版引擎
## 用法：godot --headless --path client-godot --script tools/test_typeset.gd

const TokenScript := preload("res://scripts/ui/xml_token.gd")
const TypesetScript := preload("res://scripts/ui/xml_typeset.gd")

var _pass := 0
var _fail := 0


func _init() -> void:
	print("=== XML 排版自检 ===")
	_test_token()
	_test_line_width()
	_test_wrap()
	_test_align()
	_test_padding()
	_test_height()
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0 if _fail == 0 else 1)


func _ok(c: bool, name: String, detail: String = "") -> void:
	if c:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, "  ", detail)


## 造一个等宽字符 token（模拟 12x12 汉字）
func _mk_char(ch: String, w: int = 12, h: int = 12, h1: int = 10, h2: int = 2):
	var t = TokenScript.new()
	t.kind = TokenScript.KIND_UTF8CHAR
	t.key = ch.unicode_at(0)
	t.set_size(w, h)
	t.set_heights(h1, h2)
	return t


func _mk_line(ts, chars: String) -> Array:
	var arr := []
	for i in range(chars.length()):
		arr.append(_mk_char(chars[i]))
	return arr


func _test_token() -> void:
	print("[1] Token 尺寸模型")
	var t = _mk_char("测", 12, 12, 10, 2)
	t.set_padding(1, 2)
	_ok(t.full_width() == 15, "full_width = W1+W+W2 = 1+12+2", "实际 %d" % t.full_width())
	_ok(t.w == 12 and t.h == 12, "内容尺寸")
	_ok(t.h1 == 10 and t.h2 == 2, "基线上下高度")


func _test_line_width() -> void:
	print("[2] 行宽计算")
	var ts = TypesetScript.new()
	ts.setup(100)
	ts.m_line_list = [{"start_y": 0, "content": []}]
	var line: Array = _mk_line(ts, "三个字")   # 3 个 12px 字
	ts.add_raw_token_line(0, line)
	ts.reset_one_line(0)

	# 3 字 × 12 = 36，中间 2 处留白
	_ok(ts.line_token_count(0) == 3, "行内 3 个 token")
	var fw: int = ts.line_full_width(0)
	_ok(fw >= 36, "行宽至少 36", "实际 %d" % fw)


func _test_wrap() -> void:
	print("[3] 换行判定")
	var ts = TypesetScript.new()
	ts.setup(50)   # 行宽 50，每字 12，最多 4 字（48）
	ts.m_line_list = [{"start_y": 0, "content": []}]

	# 先放 4 个字，应成功
	var ok1: bool = ts.add_raw_token_line(0, _mk_line(ts, "四个字符"))
	_ok(ok1, "4 字（48px）能放入 50px 行")

	# 再放 1 个字，应拒绝（需换行）
	var ok2: bool = ts.add_raw_token_line(0, _mk_line(ts, "第五个"))
	_ok(not ok2, "第 5 字放不下，返回 false 需换行")

	# 换到新行后应能放
	var line2: int = ts.new_line()
	var ok3: bool = ts.add_raw_token_line(line2, _mk_line(ts, "第五个"))
	_ok(ok3, "新行能放下第 5 字")


func _test_align() -> void:
	print("[4] 水平对齐")
	# 左对齐
	var ts = TypesetScript.new()
	ts.setup(100, TypesetScript.LALIGN_LEFT)
	ts.m_line_list = [{"start_y": 0, "content": []}]
	ts.add_raw_token_line(0, _mk_line(ts, "三字"))
	ts.build_typeset()
	var t0 = ts.get_token(0, 0)
	_ok(t0.x == 0, "左对齐首 token x=0", "实际 %d" % t0.x)

	# 右对齐：需要「本行窄于最宽行」才能看出效果
	# 第 1 行 6 字（最宽），第 2 行 2 字（应靠右）
	var ts2 = TypesetScript.new()
	ts2.setup(100, TypesetScript.LALIGN_RIGHT)
	ts2.m_line_list = [{"start_y": 0, "content": []}]
	ts2.add_raw_token_line(0, _mk_line(ts2, "六个字符"))
	ts2.new_line()
	ts2.add_raw_token_line(1, _mk_line(ts2, "两字"))
	ts2.build_typeset()
	var wide = ts2.get_token(0, 0)
	var narrow = ts2.get_token(0, 1)
	# 原版对齐基准是「最宽行」而非 lineWidth：
	#   fullWidth = MaxLineWidth + margins，右对齐 startX = fullWidth - 本行宽
	# 最宽行 = 4 字 = 48px，窄行 = 2 字 = 24px，故窄行 x = 48-24 = 24
	_ok(wide.x == 0, "最宽行起点 0", "实际 %d" % wide.x)
	_ok(narrow.x == 24, "窄行右对齐到最宽行右侧（x=24）", "实际 %d" % narrow.x)

	# 居中：窄行应居中
	var ts3 = TypesetScript.new()
	ts3.setup(100, TypesetScript.LALIGN_CENTER)
	ts3.m_line_list = [{"start_y": 0, "content": []}]
	ts3.add_raw_token_line(0, _mk_line(ts3, "六个字符"))
	ts3.new_line()
	ts3.add_raw_token_line(1, _mk_line(ts3, "两字"))
	ts3.build_typeset()
	var narrow3 = ts3.get_token(0, 1)
	# 居中：(48 - 24) / 2 = 12
	_ok(narrow3.x == 12, "窄行居中（x=12）", "实际 %d" % narrow3.x)


func _test_padding() -> void:
	print("[5] 两端对齐填充")
	# 两端对齐：第 1 行最宽，第 2 行会被拉伸到同样的宽度
	var ts = TypesetScript.new()
	ts.setup(100, TypesetScript.LALIGN_JUSTIFY, 0, 0, 0, 0)
	ts.m_line_list = [{"start_y": 0, "content": []}]
	ts.add_raw_token_line(0, _mk_line(ts, "六个字符"))
	ts.new_line()
	ts.add_raw_token_line(1, _mk_line(ts, "四个字符"))
	ts.build_typeset()

	var fw0: int = ts.line_full_width(0)
	var fw1: int = ts.line_full_width(1)
	_ok(fw1 >= fw0 - 2, "第2行被拉伸到接近第1行宽度", "fw0=%d fw1=%d" % [fw0, fw1])

	# 首 token 的 w1 应为 0（冻结），末 token 的 w2 应为 0（冻结）
	var first = ts.get_token(0, 0)
	var last = ts.get_token(3, 0)
	_ok(first.w1 == 0, "首 token 左留白 = lineMargin[0]", "实际 %d" % first.w1)
	_ok(last.w2 == 0, "末 token 右留白 = lineMargin[1]", "实际 %d" % last.w2)


func _test_height() -> void:
	print("[6] 垂直排版")
	var ts = TypesetScript.new()
	ts.setup(100)
	ts.m_line_list = [{"start_y": 0, "content": []}]
	ts.add_raw_token_line(0, _mk_line(ts, "三字"))
	ts.new_line()
	ts.add_raw_token_line(1, _mk_line(ts, "两字"))
	ts.build_typeset()

	_ok(ts.line_count() == 2, "共 2 行")
	var y0: int = int(ts.m_line_list[0]["start_y"])
	var y1: int = int(ts.m_line_list[1]["start_y"])
	_ok(y1 > y0, "第 2 行在下方", "y0=%d y1=%d" % [y0, y1])
	_ok(ts.fh() > 0, "总高 > 0", "实际 %d" % ts.fh())
	_ok(ts.fw() > 0, "总宽 > 0", "实际 %d" % ts.fw())

	# token 的 y 应为 行基线 - h1
	var t = ts.get_token(0, 0)
	_ok(t.y == y0 - t.h1, "token y = startY - h1", "y=%d startY=%d h1=%d" % [t.y, y0, t.h1])
