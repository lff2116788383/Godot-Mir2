extends Node
## B10 联调测试：客户端 <-> mock 服务端
## 触发：--run-test net
##
## 验证链路：连接 -> PING -> 登录 -> 查人 -> 进入游戏 -> 说话

const MirClientScript := preload("res://scripts/net/mir_client.gd")
const UidScript := preload("res://scripts/core/uid.gd")

var _pass := 0
var _fail := 0
var _client
var _got := {}
var _self_uid := 0
var _self_name := ""


func run(main: Node) -> void:
	print("=== 联调测试：客户端 <-> mock 服务端 ===")

	_client = MirClientScript.new()
	add_child(_client)
	_client.frame_received.connect(_on_frame)
	_client.decode_error.connect(func(r): print("  [decode error] ", r))

	var ok = await _client.connect_to("127.0.0.1", 7000)
	_ok(ok, "连接 127.0.0.1:7000")
	if not ok:
		print("  请先启动: cd tools/mock-server && ./mock-server.exe")
		_finish()
		return

	# 1. PING
	var tick := 12345
	_client.ping(tick)
	var got_ping := await _wait_for(MirClientScript.SM_PING, 2000)
	_ok(got_ping, "收到 SM_PING")

	# 2. 登录
	_client.login("testuser", "testpass")
	var got_login := await _wait_for(MirClientScript.SM_LOGINOK, 2000)
	_ok(got_login, "收到 SM_LOGINOK")

	# 3. 查人物
	_client.query_char()
	var got_char := await _wait_for(MirClientScript.SM_QUERYCHAROK, 2000)
	_ok(got_char, "收到 SM_QUERYCHAROK")
	if got_char:
		var body: PackedByteArray = _got[MirClientScript.SM_QUERYCHAROK]
		_ok(body.size() == 78, "QUERYCHAROK 78 字节", "实际 %d" % body.size())
		var name_size := body.decode_u16(0)
		var nm := body.slice(2, 2 + name_size).get_string_from_utf8()
		_ok(not nm.is_empty(), "角色名非空: " + nm)
		_self_name = nm

	# 4. 进入游戏
	_client.online()
	var got_online := await _wait_for(MirClientScript.SM_ONLINEOK, 3000)
	_ok(got_online, "收到 SM_ONLINEOK")
	if got_online:
		var body2: PackedByteArray = _got[MirClientScript.SM_ONLINEOK]
		_ok(body2.size() == 177, "ONLINEOK 177 字节", "实际 %d" % body2.size())
		var uid := body2.decode_u64(0)
		_self_uid = uid
		var uid_obj = UidScript.new()
		_ok(uid_obj.is_player(uid), "UID 是玩家类型", "uid=%d" % uid)
		var nsz := body2.decode_u16(8)
		var nm2 := body2.slice(10, 10 + nsz).get_string_from_utf8()
		_ok(nm2 == _self_name, "名字一致: " + nm2)
		# mapUID 是 uidf::buildMapUID 编码值，不是裸 mapID：
		#   type(MAP=2)<<59 | id(1)<<35 | peerIndex(1)<<31 = 0x1000000880000000
		var map_uid := body2.decode_u64(141)
		_ok(map_uid == 0x1000000880000000,
			"地图 UID 是 MAP 类型编码值", "实际 0x%X" % map_uid)
		_ok(UidScript.get_uid_type(map_uid) == UidScript.UID_MAP, "UID 类型 = MAP")
		_ok(UidScript.get_map_id(map_uid) == 1, "mapID = 1")

	# 5. 进场场景
	var got_scene := await _wait_for(MirClientScript.SM_STARTGAMESCENE, 3000)
	_ok(got_scene, "收到 SM_STARTGAMESCENE")
	if got_scene:
		var scene: PackedByteArray = _got[MirClientScript.SM_STARTGAMESCENE]
		_ok(scene.size() > 20, "场景数据非空", "%d 字节" % scene.size())
		# cereal 首字节应为端序标记 1
		_ok(scene[0] == 1, "cerealf 端序标记 = 1", "实际 %d" % scene[0])

	# 6. 说话
	_client.say("你好，传奇")
	var got_say := await _wait_for(MirClientScript.SM_PLAYERSAY, 3000)
	_ok(got_say, "收到 SM_PLAYERSAY（聊天广播）")
	if got_say:
		var sb: PackedByteArray = _got[MirClientScript.SM_PLAYERSAY]
		_ok(sb.size() == 136, "PLAYERSAY 136 字节", "实际 %d" % sb.size())
		var content := sb.slice(8, 136).get_string_from_utf8()
		_ok(content.begins_with("你好"), "回显内容正确: " + content.strip_edges())

	_client.close_connection()
	_finish()


func _on_frame(code: int, body: PackedByteArray, _resp: int) -> void:
	_got[code] = body


func _wait_for(code: int, timeout_ms: int) -> bool:
	var waited := 0
	while waited < timeout_ms:
		if _got.has(code):
			return true
		await get_tree().process_frame
		waited += 16
	return false


func _finish() -> void:
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	get_tree().quit(0 if _fail == 0 else 1)


func _ok(c: bool, name: String, detail: String = "") -> void:
	if c:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, "  ", detail)
