extends SceneTree
## 数据层自检：物品表 / UID / ActionNode / 魔法文件名 / 物品布局
## 用法：godot --headless --path client-godot --script tools/test_data.gd
##
## 注意：--script 模式下 Autoload 不可用，且 GDScript 对带 static var 的脚本
## 静态调用不可靠，故全部用 preload + new() 实例调用。

const ItemRecordScript := preload("res://scripts/item/item_record.gd")
const UidScript := preload("res://scripts/core/uid.gd")
const ActionNodeScript := preload("res://scripts/action/action_node.gd")
const ActionTypeScript := preload("res://scripts/action/action_type.gd")
const MagicDBScript := preload("res://scripts/magic/magic_db.gd")
const ItemLayoutScript := preload("res://scripts/item/item_layout.gd")
const ProtocolDefScript := preload("res://scripts/core/protocol_def.gd")

var _ir
var _uid
var _an
var _at
var _mdb
var _lay
var _pd

var _pass := 0
var _fail := 0


func _init() -> void:
	_ir = ItemRecordScript.new()
	_uid = UidScript.new()
	_an = ActionNodeScript.new()
	_at = ActionTypeScript.new()
	_mdb = MagicDBScript.new()
	_lay = ItemLayoutScript.new()
	_pd = ProtocolDefScript.new()

	print("=== 数据层自检 ===")
	_test_item_record()
	_test_uid()
	_test_action_node()
	_test_magic_db()
	_test_item_layout()
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0 if _fail == 0 else 1)


func _ok(c: bool, name: String, detail: String = "") -> void:
	if c:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, "  ", detail)


func _test_item_record() -> void:
	print("[1] 物品表")
	_ok(_ir.load_data(), "加载 itemrecord.json")
	_ok(_ir.count() == 1258, "条目数 1258", "实际 %d" % _ir.count())

	_ok(_ir.name(1) == "金币（小）", "ID1 名称")
	_ok(_ir.pkg_gfx_id(1) == 120, "ID1 pkgGfxID=0x78", "实际 %d" % _ir.pkg_gfx_id(1))
	_ok(_ir.is_gold(1), "ID1 是金币")
	_ok(not _ir.packable(1), "金币不可堆叠")

	_ok(_ir.is_potion(7), "ID7 是恢复药水")
	_ok(int(_ir.potion(7).get("hp", 0)) == 70, "ID7 hp=70")
	_ok(_ir.packable(7), "ID7 可堆叠")
	_ok(_ir.beltable(7), "ID7 可放腰带")

	var dress_id := _find_first_type("衣服")
	_ok(dress_id > 0, "找到衣服类物品", "id=%d" % dress_id)
	if dress_id > 0:
		_ok(_ir.wearable(dress_id, _pd.WLG_DRESS), "衣服可穿到 DRESS 槽")
		_ok(not _ir.wearable(dress_id, _pd.WLG_WEAPON), "衣服不能穿到 WEAPON 槽")

	var weapon_id := _find_first_type("武器")
	_ok(weapon_id > 0, "找到武器类物品", "id=%d" % weapon_id)
	if weapon_id > 0:
		_ok(_ir.wearable(weapon_id, _pd.WLG_WEAPON), "武器可穿到 WEAPON 槽")


func _find_first_type(t: String) -> int:
	for i in range(1, 1300):
		if _ir.exists(i) and _ir.type(i) == t:
			return i
	return 0


func _test_uid() -> void:
	print("[2] UID 位布局")
	var puid: int = _uid.get_player_uid(12345)
	_ok(_uid.get_uid_type(puid) == _uid.UID_PLY, "玩家 UID 类型=5")
	_ok(_uid.get_player_dbid(puid) == 12345, "玩家 DBID 往返一致")
	_ok(_uid.is_player(puid), "is_player")

	var muid: int = _uid.build_map_uid(7, 2)
	_ok(_uid.get_uid_type(muid) == _uid.UID_MAP, "地图 UID 类型=2")
	_ok(_uid.get_map_id(muid) == 7, "地图 ID=7")
	_ok(_uid.peer_index(muid) == 2, "peerIndex=2")

	var monuid: int = _uid.build_monster_uid(100, 0)
	_ok(_uid.get_uid_type(monuid) == _uid.UID_MON, "怪物 UID 类型=4")
	_ok(_uid.get_monster_id(monuid) == 100, "怪物 ID=100")
	_ok(_uid.is_monster(monuid), "is_monster")
	_ok(puid > 0 and (puid >> 63) == 0, "高位保留为 0")


func _test_action_node() -> void:
	print("[3] ActionNode 编解码")
	var n = _an.make_move(100, 10, 20, 11, 21, 0)
	var buf: PackedByteArray = n.to_bytes()
	_ok(buf.size() == 28, "编码 28 字节", "实际 %d" % buf.size())

	var n2 = _an.from_bytes(buf)
	_ok(n2.type == _at.ACTION_MOVE, "type 往返", "实际 %d" % n2.type)
	_ok(n2.speed == 100, "speed 往返", "实际 %d" % n2.speed)
	_ok(n2.x == 10 and n2.y == 20, "起点往返")
	_ok(n2.aim_x == 11 and n2.aim_y == 21, "终点往返")

	var ok_dir := true
	for d in range(1, 9):
		var nn = _an.make_stand(d, 0, 0)
		var back = _an.from_bytes(nn.to_bytes())
		if back.direction != d:
			ok_dir = false
	_ok(ok_dir, "8 个方向全部往返正确")

	var atk = _an.make_attack(100, 5, 5, 999, 12, 34)
	var atk2 = _an.from_bytes(atk.to_bytes())
	_ok(atk2.type == _at.ACTION_ATTACK, "攻击 type")
	_ok(atk2.aim_uid == 999, "aimUID 往返")


func _test_magic_db() -> void:
	print("[4] 魔法文件名解析")
	var info = _mdb.parse_file_name("00000000F37F67000000000000.PNG")
	_ok(info.dx == -201, "dx = -201（F37 补码）", "实际 %d" % info.dx)
	_ok(info.dy == -153, "dy = -153（F67 补码）", "实际 %d" % info.dy)
	_ok(info.head_x == 0 and info.head_y == 0, "head 偏移 0")
	_ok(_mdb.seq_id(0x100, 3) == ((0x100 << 4) | 3), "seqID = gfxID<<4|dir")
	_ok(_mdb.to_2comp("FFF", 3) == -1, "FFF -> -1")
	_ok(_mdb.to_2comp("800", 3) == -2048, "800 -> -2048")
	_ok(_mdb.to_2comp("7FF", 3) == 2047, "7FF -> 2047")


func _test_item_layout() -> void:
	print("[5] 物品布局")
	var items := [
		{"w": 38, "h": 38, "visible": true},
		{"w": 38, "h": 38, "visible": true},
		{"w": 38, "h": 38, "visible": true},
	]
	var lay: Array = _lay.compute_layout(true, _lay.ALIGN_UPLEFT, items, 0, 2, 38, 200)
	_ok(lay.size() == 3, "布局输出 3 项")
	_ok(lay[0].y == 0, "第1项 y=0")
	_ok(lay[1].y == 40, "第2项 y=40（38+2）", "实际 %d" % lay[1].y)
	_ok(lay[2].y == 80, "第3项 y=80", "实际 %d" % lay[2].y)

	var cs: Vector2i = _lay.canvas_size(true, items, 0, 2)
	_ok(cs.x == 38, "容器宽 38")
	_ok(cs.y == 38 * 3 + 2 * 2, "容器高 118", "实际 %d" % cs.y)

	_ok(_lay.inv_grid_pos(0) == Vector2i(0, 0), "格 0 -> (0,0)")
	_ok(_lay.inv_grid_pos(15) == Vector2i(5, 1), "格 15 -> (5,1)")
	_ok(_lay.inv_capacity() == 100, "背包 100 格")
	_ok(_lay.belt_slot_valid(5), "腰带 slot5 有效")
	_ok(not _lay.belt_slot_valid(6), "腰带 slot6 无效")
