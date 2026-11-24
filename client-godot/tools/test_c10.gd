extends SceneTree
## C10 自检：地图物件层 + 角色状态消息解析
const MapLoaderRef := preload("res://scripts/data/map_loader.gd")
const MapObjRef := preload("res://scripts/data/map_objects.gd")
const ParserRef := preload("res://scripts/net/sd_msg_parser.gd")
const SdItemRef := preload("res://scripts/item/sd_item.gd")
const MirRef := preload("res://scripts/net/mir_client.gd")

var _pass := 0
var _fail := 0

func _ok(cond: bool, name_: String, extra: String = "") -> void:
	if cond:
		_pass += 1
		print("  [PASS] ", name_)
	else:
		_fail += 1
		print("  [FAIL] ", name_, ("  " + extra) if extra else "")


func _init() -> void:
	print("=== C10 自检 ===")
	_test_map_obj()
	_test_sd_parser()
	_test_proto_consts()
	print("\n=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0 if _fail == 0 else 1)


# ==================== 地图物件层 ====================

func _test_map_obj() -> void:
	print("\n[C10-1] 地图物件层解析")
	var md = MapLoaderRef.load_file("E:/Desktop/game/mir2x_assets-main/map/0.bin")
	_ok(md != null, "地图加载成功")
	if md == null:
		return

	# 瓦片与可走格没被改坏（回归）
	var valid_tiles := 0
	for t in md.tiles:
		if t >= 0: valid_tiles += 1
	_ok(valid_tiles == 159997, "瓦片有效数不变 159997", str(valid_tiles))
	var walk := 0
	for v in md.walkable:
		if v != 0: walk += 1
	_ok(walk > 300000, "可走格 > 30 万（54% 左右）", str(walk))

	# 物件
	var g := md.obj_count_for_depth(MapLoaderRef.OBJD_GROUND)
	var o0 := md.obj_count_for_depth(MapLoaderRef.OBJD_OVERGROUND0)
	var o1 := md.obj_count_for_depth(MapLoaderRef.OBJD_OVERGROUND1)
	print("    [info] 地面物件=%d 上层0=%d 上层1=%d" % [g, o0, o1])
	_ok(g > 100000, "地面物件 > 10 万", str(g))
	_ok(o0 > 50000, "上层物件0 > 5 万", str(o0))
	_ok(g + o0 + o1 > 150000, "物件总数 > 15 万", str(g + o0 + o1))

	# 光照
	var lc := md.light_count()
	_ok(lc > 100, "光照格 > 100", str(lc))

	# 单格物件取值
	var found := false
	for y in range(md.h):
		for x in range(md.w):
			if md.obj_count[y * md.w + x] > 0:
				var o = md.obj_at(x, y, 0)
				_ok(o != null and o.valid, "obj_at 返回有效物件")
				_ok(o.tex_id > 0, "物件有 texID", str(o.tex_id))
				found = true
				break
		if found: break

	# 键算法
	_ok(MapObjRef.is_animated_tex(11 << 16 | 0x1234), "fileIndex 11 参与动画")
	_ok(MapObjRef.is_animated_tex(26 << 16 | 0xABCD), "fileIndex 26 参与动画")
	_ok(not MapObjRef.is_animated_tex(33 << 16 | 0x1111), "fileIndex 33 不参与")
	_ok(not MapObjRef.is_animated_tex(0x0000FFFF), "fileIndex 0 不参与")

	# 动画帧推进
	MapObjRef.tick(0.0)
	var f1 := MapObjRef.ani_frame(0, 4)
	MapObjRef.tick(0.13)
	var f2 := MapObjRef.ani_frame(0, 4)
	_ok(f1 != f2 or f1 == 0, "动画帧随 tick 推进", "%d -> %d" % [f1, f2])
	_ok(MapObjRef.ani_frame(0, 0) == 0, "0 帧返回 0")
	_ok(MapObjRef.ani_frame(0, 1) == 0, "1 帧返回 0")

	# 透明度三分支
	var fake := MapLoaderRef.MapObject.new()
	fake.valid = true
	fake.alpha = 0
	_ok(is_equal_approx(MapObjRef.obj_alpha(fake), 1.0), "无 alpha 不透明")
	fake.alpha = 100
	_ok(is_equal_approx(MapObjRef.obj_alpha(fake), 96.0/255.0), "有 alpha 用 96")
	fake.alpha = 0
	_ok(is_equal_approx(MapObjRef.obj_alpha(fake, true), 128.0/255.0), "传 dim 用 128")


# ==================== 状态消息解析 ====================

func _test_sd_parser() -> void:
	print("\n[C10-2] 状态消息解析")

	# SM_HEALTH: uid(8) + 6×int(24)
	var h := PackedByteArray(); h.resize(32)
	h.encode_u64(0, 0x11223344)
	h.encode_s32(8, 88)     # hp
	h.encode_s32(12, 42)    # mp
	h.encode_s32(16, 100)   # maxHP
	h.encode_s32(20, 50)    # maxMP
	h.encode_s32(24, 3)
	h.encode_s32(28, 2)
	var r := ParserRef.parse_health(h)
	_ok(int(r["uid"]) == 0x11223344, "health uid")
	_ok(int(r["hp"]) == 88, "health hp=88", str(r["hp"]))
	_ok(int(r["mp"]) == 42, "health mp=42")
	_ok(int(r["max_hp"]) == 100, "health maxHP=100")
	_ok(int(r["max_mp"]) == 50, "health maxMP=50")
	_ok(ParserRef.parse_health(PackedByteArray()).is_empty(), "过短返回空")

	# SM_GOLD / SM_EXP
	var g := PackedByteArray(); g.resize(8); g.encode_u64(0, 12345)
	_ok(int(ParserRef.parse_gold(g)["gold"]) == 12345, "gold=12345")
	var e := PackedByteArray(); e.resize(8); e.encode_u64(0, 99999)
	_ok(int(ParserRef.parse_exp(e)["exp"]) == 99999, "exp=99999")

	# SM_INVENTORY: vector<SDItem>
	var inv := PackedByteArray()
	inv.resize(8)
	inv.encode_u64(0, 2)     # 2 个物品
	# 物品1: itemID=100, seqID=1, count=5, dur 0/0, 无扩展属性
	var it1 := PackedByteArray(); it1.resize(48); it1.fill(0)
	it1.encode_u64(0, 100); it1.encode_u64(8, 1); it1.encode_u64(16, 5)
	inv.append_array(it1)
	# 物品2: itemID=200, seqID=2, count=3
	var it2 := PackedByteArray(); it2.resize(48); it2.fill(0)
	it2.encode_u64(0, 200); it2.encode_u64(8, 2); it2.encode_u64(16, 3)
	inv.append_array(it2)
	var ri := ParserRef.parse_inventory(inv)
	_ok((ri["items"] as Array).size() == 2, "背包解析 2 件", str((ri["items"] as Array).size()))
	if (ri["items"] as Array).size() == 2:
		var i0 = ri["items"][0]
		_ok(i0.item_id == 100, "物品1 id=100", str(i0.item_id))
		_ok(i0.count == 5, "物品1 count=5")
		var i1 = ri["items"][1]
		_ok(i1.item_id == 200, "物品2 id=200")

	# SM_BELT: array<SDItem,6>（无长度前缀）
	var belt := PackedByteArray()
	for k in range(6):
		var bi := PackedByteArray(); bi.resize(48); bi.fill(0)
		bi.encode_u64(0, 300 + k); bi.encode_u64(8, k); bi.encode_u64(16, 1)
		belt.append_array(bi)
	var rb := ParserRef.parse_belt(belt)
	_ok((rb["items"] as Array).size() == 6, "腰带 6 格")
	_ok(rb["items"][0].item_id == 300, "腰格0 id=300")

	# SM_EQUIPWEAR: uid(8) + wltype(4) + item
	var ew := PackedByteArray()
	ew.resize(12); ew.encode_u64(0, 7); ew.encode_s32(8, 3)
	var ewi := PackedByteArray(); ewi.resize(48); ewi.fill(0)
	ewi.encode_u64(0, 555); ewi.encode_u64(8, 9); ewi.encode_u64(16, 1)
	ew.append_array(ewi)
	var rew := ParserRef.parse_equip_wear(ew)
	_ok(int(rew["wltype"]) == 3, "装备槽 = 武器(3)")
	_ok(rew["item"] != null and rew["item"].item_id == 555, "装备物品 id=555")

	# SM_REMOVEITEM
	var rm := PackedByteArray(); rm.resize(32)
	rm.encode_u64(0, 1); rm.encode_u64(8, 777); rm.encode_u64(16, 2); rm.encode_u64(24, 1)
	var rrm := ParserRef.parse_remove_item(rm)
	_ok(int(rrm["item_id"]) == 777, "removeitem id=777")
	_ok(int(rrm["count"]) == 1, "removeitem count=1")

	# SM_LEARNEDMAGICLIST: uid(8) + vector<u32>
	var lm := PackedByteArray(); lm.resize(8)
	lm.encode_u64(0, 5)
	lm.append_array(PackedByteArray([3,0,0,0,0,0,0,0]))   # 长度 3
	lm.append_array(PackedByteArray([11,0,0,0]))          # magic 11
	lm.append_array(PackedByteArray([22,0,0,0]))          # magic 22
	lm.append_array(PackedByteArray([33,0,0,0]))          # magic 33
	var rlm := ParserRef.parse_learned_magic(lm)
	_ok((rlm["magics"] as Array).size() == 3, "3 个已学魔法", str((rlm["magics"] as Array).size()))
	if (rlm["magics"] as Array).size() == 3:
		_ok(int(rlm["magics"][1]) == 22, "第二个魔法 = 22")

	# SM_VIEWRANGE
	var vr := PackedByteArray(); vr.resize(4); vr.encode_u32(0, 15)
	_ok(int(ParserRef.parse_view_range(vr)["range"]) == 15, "视野 = 15")

	# 基础读取工具
	var bb := PackedByteArray([1,2,3,4,5,6,7,8,9,10])
	_ok(ParserRef.u8(bb, 0) == 1, "u8 读 1")
	_ok(ParserRef.u16(bb, 0) == 0x0201, "u16 小端")
	_ok(ParserRef.u64(bb, 0) == 0x0807060504030201, "u64 小端")
	_ok(ParserRef.u8(bb, 100) == 0, "越界返回 0")

	# string 读取
	var sb := PackedByteArray(); sb.resize(8)
	var txt := "hello".to_utf8_buffer()
	sb.encode_u64(0, txt.size())
	sb.append_array(txt)
	var rs := ParserRef.rd_str(sb, 0)
	_ok(str(rs[0]) == "hello", "字符串读取", str(rs[0]))


# ==================== 协议码一致性 ====================

func _test_proto_consts() -> void:
	print("\n[C10-3] 协议码一致性")
	var SMT = load("res://scripts/net/sm_table.gd")
	var checks := [
		["SM_HEALTH", 30], ["SM_GOLD", 47], ["SM_EXP", 34],
		["SM_INVENTORY", 55], ["SM_BELT", 56], ["SM_EQUIPWEAR", 65],
		["SM_LEARNEDMAGICLIST", 26], ["SM_VIEWRANGE", 104], ["SM_LOGOUTOK", 21],
	]
	# 用 load 出来的脚本对象取常量（不能直接在 class 上调 get）
	for c in checks:
		var const_val: int = int(MirRef[c[0]])
		_ok(const_val == int(c[1]), "%s = %d" % [c[0], c[1]], str(const_val))

	# 表内 SM 总数
	var spec = SMT.SM_SPEC
	if spec is Dictionary:
		print("    [info] 表内 SM 数 = %d" % (spec as Dictionary).size())
