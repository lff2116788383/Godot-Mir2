extends SceneTree
## B7 自检：背包容器 / 消息栈 / 小地图
## 用法：godot --headless --path client-godot --script tools/test_ui.gd

const SDItemScript := preload("res://scripts/item/sd_item.gd")
const SDInvScript := preload("res://scripts/item/sd_inventory.gd")
const MsgStackScript := preload("res://scripts/ui/message_stack.gd")
const MiniMapScript := preload("res://scripts/ui/mini_map.gd")

var _pass := 0
var _fail := 0


func _init() -> void:
	print("=== UI 逻辑自检 ===")
	_test_inventory()
	_test_message_stack()
	_test_minimap()
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0 if _fail == 0 else 1)


func _ok(c: bool, name: String, detail: String = "") -> void:
	if c:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, "  ", detail)


func _mk_item(item_id: int, count: int, seq: int = 0):
	var it = SDItemScript.new()
	it.item_id = item_id
	it.seq_id = seq if seq > 0 else 1
	it.count = count
	it.set_duration(0, 0)
	return it


func _test_inventory() -> void:
	print("[1] 背包容器")
	var inv = SDInvScript.new()

	# 添加可堆叠物品（金创药 ID=7，packable）
	var a = _mk_item(7, 10)
	var r1 = inv.add(a, false)
	_ok(r1 != null, "添加金创药成功")
	_ok(inv.size() == 1, "背包 1 格")

	# 再添加同物品，应合并（10+20=30 <= 99）
	var b = _mk_item(7, 20)
	inv.add(b, false)
	_ok(inv.size() == 1, "同物品合并未新增格", "实际 %d 格" % inv.size())
	_ok(inv.has(7) == 30, "合并后总数 30", "实际 %d" % inv.has(7))

	# 合并到上限：再加 80，30+80=110 > 99，应新开一格
	var c = _mk_item(7, 80)
	inv.add(c, false)
	_ok(inv.size() == 2, "超上限后新开一格", "实际 %d 格" % inv.size())
	_ok(inv.has(7) == 110, "总数 110")

	# 不可堆叠物品（金币 ID=1）每次新开
	var inv2 = SDInvScript.new()
	inv2.add(_mk_item(1, 1), false)
	inv2.add(_mk_item(1, 1), false)
	_ok(inv2.size() == 2, "金币不堆叠，各占一格", "实际 %d" % inv2.size())

	# seqID 唯一性
	var g1 = inv2.find(1, 1)
	var g2 = inv2.find(1, 2)
	_ok(g1 != null and g2 != null, "两个金币 seqID 分别为 1、2")

	# 删除：部分删除
	var inv3 = SDInvScript.new()
	inv3.add(_mk_item(7, 50), false)
	var rm = inv3.remove(7, 0, 20, false)
	_ok(int(rm["count"]) == 20, "部分删除 20")
	_ok(inv3.has(7) == 30, "剩余 30", "实际 %d" % inv3.has(7))

	# 删除：全部删除
	var rm2 = inv3.remove(7, 0, 30, false)
	_ok(int(rm2["count"]) == 30, "删除剩余 30")
	_ok(inv3.size() == 0, "背包已空")

	# 删除：数量不足（strict=false 全删）
	var inv4 = SDInvScript.new()
	inv4.add(_mk_item(7, 10), false)
	var rm3 = inv4.remove(7, 0, 99, false)
	_ok(int(rm3["count"]) == 10, "数量不足时删除全部 10")
	_ok(inv4.size() == 0, "已删空")

	# 删除：strict=true 数量不匹配则不删
	var inv5 = SDInvScript.new()
	inv5.add(_mk_item(7, 10), false)
	var rm4 = inv5.remove(7, 0, 99, true)
	_ok(int(rm4["count"]) == 0, "strict 模式下数量不匹配返回 0")
	_ok(inv5.size() == 1, "strict 模式下未删除")

	# 合并两格
	var inv6 = SDInvScript.new()
	inv6.add(_mk_item(7, 10), false)
	inv6.add(_mk_item(7, 20, 5), true)
	_ok(inv6.size() == 2, "两格金创药")
	var ok_merge: bool = inv6.merge(7, 5, 1)
	_ok(ok_merge, "合并成功")
	_ok(inv6.size() == 1, "合并后 1 格")
	_ok(inv6.has(7) == 30, "合并后 30")

	# 容量
	_ok(inv.capacity() == 100, "背包容量 100")


func _test_message_stack() -> void:
	print("[2] 消息栈")
	var ms = MsgStackScript.new()
	ms.setup(3, 2, 0)   # 上限 3 条

	ms.add_message("第一条")
	ms.add_message("第二条")
	_ok(ms.size() == 2, "2 条消息")

	ms.add_message("第三条")
	_ok(ms.size() == 3, "3 条消息")

	# 超上限，最旧的被移除
	ms.add_message("第四条")
	_ok(ms.size() == 3, "仍为 3 条（超限移除最旧）")
	var msgs: Array = ms.messages()
	_ok(msgs[0].xml.contains("第二条"), "最旧的「第一条」已被移除", "首条=%s" % msgs[0].xml)
	_ok(msgs[2].xml.contains("第四条"), "最新的是「第四条」")

	# 最近 N 条
	var latest: Array = ms.latest(2)
	_ok(latest.size() == 2, "最近 2 条")
	_ok(latest[1].xml.contains("第四条"), "最近一条是第四条")

	ms.clear()
	_ok(ms.size() == 0, "清空后 0 条")


func _test_minimap() -> void:
	print("[3] 小地图")
	var mm = MiniMapScript.new()
	# 画布 200x200，图像 400x400（比画布大）
	mm.setup(200, 200, 400, 400)

	# 图像大于画布时，初始应夹在 [canvas-map, 0]
	_ok(mm.map_dx <= 0, "初始 dx <= 0", "实际 %d" % mm.map_dx)
	_ok(mm.map_dx >= 200 - 400, "初始 dx >= -200", "实际 %d" % mm.map_dx)

	# 坐标换算往返
	mm.map_dx = -50
	mm.map_dy = -60
	var img := mm.canvas_to_image(80, 90)
	_ok(img == Vector2i(130, 150), "画布(80,90) -> 图像(130,150)", "实际 %s" % str(img))
	var back := mm.image_to_canvas(img.x, img.y)
	_ok(back == Vector2i(80, 90), "往返一致")

	# 格坐标映射（400x400 图对应 800x800 格）
	var g := mm.image_to_grid(200, 300, 800, 800)
	_ok(g == Vector2i(400, 600), "图像(200,300) -> 格(400,600)", "实际 %s" % str(g))

	var gp := mm.grid_to_image(400, 600, 800, 800)
	_ok(gp == Vector2i(200, 300), "格(400,600) -> 图像(200,300)")

	var cp := mm.grid_to_canvas(400, 600, 800, 800)
	_ok(cp == Vector2i(150, 240), "格->画布 = 图像+偏移", "实际 %s" % str(cp))

	# 图像小于画布时居中
	var mm2 = MiniMapScript.new()
	mm2.setup(300, 300, 100, 100)
	_ok(mm2.map_dx == 100, "小图居中 dx=100", "实际 %d" % mm2.map_dx)
	_ok(mm2.map_dy == 100, "小图居中 dy=100")

	# 平移边界夹取
	mm2.pan(500, 500)
	_ok(mm2.map_dx == 100, "小图不会被拖走（保持居中）", "实际 %d" % mm2.map_dx)
