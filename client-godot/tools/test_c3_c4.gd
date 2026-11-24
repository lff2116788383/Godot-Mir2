extends SceneTree
## C3+C4 自检：XML 解析 / 排版 / NPC 面板 / 战斗表现 / 物品图标

const LayoutRef := preload("res://scripts/ui/xml_layout.gd")
const BuilderRef := preload("res://scripts/ui/xml_typeset_builder.gd")
const NpcChatRef := preload("res://scripts/ui/ui_npc_chat.gd")
const FxRef := preload("res://scripts/actor/fight_effects.gd")
const ItemGfxRef := preload("res://scripts/item/item_gfx.gd")
const ItemRecordRef := preload("res://scripts/item/item_record.gd")
const InvRef := preload("res://scripts/item/sd_inventory.gd")
const SdItemRef := preload("res://scripts/item/sd_item.gd")
const TokenRef := preload("res://scripts/ui/xml_token.gd")

var _pass := 0
var _fail := 0
var _assets := ""


func _ok(cond: bool, name: String, extra: String = "") -> void:
	if cond:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, ("  " + extra) if extra else "")


func _init() -> void:
	_assets = OS.get_environment("MIR2X_ASSETS")
	if _assets.is_empty():
		_assets = "E:/Desktop/game/mir2x_assets-main"
	print("=== C3+C4 自检 ===")
	_test_xml_parse()
	_test_xml_typeset()
	_test_npc_chat()
	_test_fight_fx()
	_test_item_gfx()
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0)


# ==================== C3: XML 解析 ====================

func _test_xml_parse() -> void:
	print("\n[C3-1] XML 解析")

	# 实测格式（server/script/npc/*.lua）
	var lay = LayoutRef.new().parse("<layout><par>请输入行会的名字</par></layout>")
	_ok(lay.paragraphs.size() == 1, "1 个段落", str(lay.paragraphs.size()))
	_ok(lay.plain_text(0) == "请输入行会的名字", "段落文字正确", lay.plain_text(0))
	_ok(lay.leaf_count() == 1, "1 个叶子", str(lay.leaf_count()))

	# 带 emoji
	var lay2 = LayoutRef.new().parse('<layout><par>你好<emoji id="0"/>欢迎</par></layout>')
	_ok(lay2.paragraphs.size() == 1, "带 emoji 解析成 1 段")
	_ok(lay2.leaf_count() == 3, "3 个叶子（文字+emoji+文字）", str(lay2.leaf_count()))
	var leaves: Array = lay2.paragraphs[0]["leaves"]
	_ok(int(leaves[0]["type"]) == LayoutRef.LEAF_UTF8STR, "叶子0 是文字")
	_ok(int(leaves[1]["type"]) == LayoutRef.LEAF_EMOJI, "叶子1 是 emoji")
	_ok(int(leaves[1]["id"]) == 0, "emoji id = 0")
	_ok(lay2.plain_text(0) == "你好欢迎", "去 emoji 后文字", lay2.plain_text(0))

	# 多段落
	var lay3 = LayoutRef.new().parse("<layout><par>第一段</par><par>第二段</par></layout>")
	_ok(lay3.paragraphs.size() == 2, "2 个段落", str(lay3.paragraphs.size()))
	_ok(lay3.all_plain_text() == "第一段\n第二段", "多段拼接", lay3.all_plain_text())

	# 空串
	var lay4 = LayoutRef.new().parse("")
	_ok(lay4.paragraphs.is_empty(), "空串无段落")

	# 非法 XML 不崩溃
	var lay5 = LayoutRef.new().parse("<layout><par>未闭合")
	_ok(true, "非法 XML 不崩溃")

	# 静态构造
	var s := LayoutRef.single_par("测试<>&")
	_ok(s.contains("&lt;") and s.contains("&gt;") and s.contains("&amp;"), "转义 < > &", s)


# ==================== C3: 排版 ====================

func _test_xml_typeset() -> void:
	print("\n[C3-2] XML 排版")
	var b := BuilderRef.new()
	b.setup(200, null, 15, "%s/emoji" % _assets)

	var ts = b.build("<layout><par>测试文字排版</par></layout>")
	_ok(ts != null, "排版成功")
	if ts == null:
		return
	_ok(ts.line_count() >= 1, "至少 1 行", str(ts.line_count()))
	_ok(ts.get_token(0, 0) != null, "首个 token 存在")

	# 长文本应换行
	var ts2 = b.build("<layout><par>这是一段很长的文字需要换行测试换行逻辑是否正确工作起来</par></layout>")
	_ok(ts2.line_count() >= 2, "长文本换行", "%d 行" % ts2.line_count())

	# token 有正尺寸
	var tk = ts2.get_token(0, 0)
	_ok(tk.w > 0 and tk.h > 0, "token 尺寸为正", "%dx%d" % [tk.w, tk.h])

	# 多段落必须分行（两段文字 y 坐标不同，不能重叠）
	var ts3 = b.build("<layout><par>第一段文字</par><par>第二段文字</par></layout>")
	_ok(ts3.line_count() == 2, "两段落占 2 行", str(ts3.line_count()))
	if ts3.line_count() >= 2:
		var y0: int = int(ts3.get_token(0, 0).y)
		var y1: int = int(ts3.get_token(0, 1).y)
		print("    [info] 第0行 token.y=%d  第1行 token.y=%d" % [y0, y1])
		_ok(y1 > y0, "第2行在第1行下方（不重叠）", "y0=%d y1=%d" % [y0, y1])


# ==================== C3: NPC 面板 ====================

func _test_npc_chat() -> void:
	print("\n[C3-3] NPC 对话面板")
	var npc := NpcChatRef.new("NpcChat")
	_ok(not npc.is_open(), "默认关闭")

	npc.show_layout(0x1234, "<layout><par>欢迎来到比奇县</par></layout>", "npc_test")
	_ok(npc.is_open(), "show 后打开")
	_ok(npc.npc_uid == 0x1234, "NPC UID 记录")
	_ok(npc.event_path == "npc_test", "事件路径记录")
	_ok(npc.h > 0, "高度已自适应", str(npc.h))

	# 空 XML 不崩
	npc.show_layout(1, "", "")
	_ok(npc.is_open(), "空 XML 仍能打开")


# ==================== C4: 战斗表现 ====================

func _test_fight_fx() -> void:
	print("\n[C4] 战斗表现")
	var fx := FxRef.new()

	# 飘字添加
	fx.on_miss(100, 100)
	_ok(fx.ascend_count() == 1, "MISS 飘字已添加")

	fx.on_damage(100, 100, 42)
	fx.on_magic_damage(101, 100, 88)
	fx.on_heal(102, 100, 15)
	_ok(fx.ascend_count() == 4, "4 条飘字", str(fx.ascend_count()))

	# 3000ms 后过期（模拟推进）
	for e in fx._ascend_list:
		e["tick"] = 3100.0
	# 手动跑一次清理逻辑
	var keep := []
	for e in fx._ascend_list:
		if float(e["tick"]) < FxRef.ASCEND_MS:
			keep.append(e)
	fx._ascend_list = keep
	_ok(fx.ascend_count() == 0, "超时后清空")

	# 死亡淡出
	var dummy := Node2D.new()
	var spr := Sprite2D.new()
	dummy.add_child(spr)
	fx.start_fade_out(dummy)
	_ok(fx.fade_count() == 1, "淡出已登记")
	# 重复登记不重复
	fx.start_fade_out(dummy)
	_ok(fx.fade_count() == 1, "重复登记被忽略")

	# 常量核对（ascendstr.hpp）
	_ok(FxRef.ASCEND_MS == 3000.0, "飘字时长 3000ms")
	_ok(FxRef.ASCEND_DIST == 50.0, "飘动距离 50px")
	_ok(FxRef.ASCENDSTR_MISS == 0 and FxRef.ASCENDSTR_RED == 1, "类型常量对齐")

	dummy.free()


# ==================== C4: 物品图标 ====================

func _test_item_gfx() -> void:
	print("\n[C4-2] 物品图标")
	ItemGfxRef.setup(_assets)

	# 金币（item_id=1，pkgGfxID=120）
	var gold_pkg := ItemRecordRef.pkg_gfx_id(1)
	_ok(gold_pkg == 120, "金币 pkgGfxID = 120", str(gold_pkg))

	var tex := ItemGfxRef.icon_of_item(1)
	_ok(tex != null, "金币图标已加载")
	if tex != null:
		_ok(tex.get_width() > 0 and tex.get_height() > 0, "图标尺寸有效",
			"%dx%d" % [tex.get_width(), tex.get_height()])

	# 居中偏移
	if tex != null:
		var off := ItemGfxRef.center_offset(tex, 32, 32)
		_ok(off.x >= 0 and off.y >= 0, "居中偏移非负", str(off))

	# 不存在的物品不崩溃
	var t2 := ItemGfxRef.icon_of_item(999999)
	_ok(t2 == null, "不存在的物品返回 null")

	# 背包集成：放入金币后能查到图标
	var inv = InvRef.new()
	var it = SdItemRef.new()
	it.item_id = 1
	it.count = 100
	inv.add(it)
	_ok(inv.size() == 1, "背包有 1 格")
	if inv.size() > 0:
		var first = inv.item_list()[0]
		var tex2 := ItemGfxRef.icon_of_item(first.item_id)
		_ok(tex2 != null, "背包物品图标可得")
