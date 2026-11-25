extends Node
class_name MirClient
## 客户端网络连接
##
## 【逐行翻译】client/src/netio.cpp（279 行）
## 对照：
##   - netio.cpp:34  doReadPacketHeadCode   读帧头
##   - netio.cpp:55  doReadPacketResp       读应答 ID
##   - netio.cpp:68  doReadPacketBodySize   读体长度
##   - netio.cpp:89  doReadPacketBody       读体 + XOR 解码
##   - netio.cpp:173 send                   组帧发送
##
## Godot 差异：用 StreamPeerTCP 代替 asio，用信号代替回调。
## 拆包逻辑完全照搬：流式累积，能解几帧解几帧。

signal connected
signal disconnected(reason: String)
signal frame_received(head_code: int, body: PackedByteArray, resp_id: int)
signal decode_error(reason: String)

## 客户端消息码
## 码值以 scripts/net/sm_table.gd 为准（由 C++ 头文件生成）
const CM_PING := 1
const CM_LOGIN := 2
const CM_QUERYCHAR := 3
const CM_CREATECHAR := 4
const CM_DELETECHAR := 5
const CM_CHANGEPASSWORD := 6
const CM_ONLINE := 7
const CM_ACTION := 8
const CM_SETMAGICKEY := 9
const CM_QUERYGOLD := 25
const CM_CHATMESSAGE := 43
const CM_PLAYERSAY := 44

const CM_SETRUNTIMECONFIG := 10
const CM_QUERYCORECORD := 11
const CM_QUERYNPCNAME := 12
const CM_REQUESTADDHEALTH := 13
const CM_REQUESTADDEXP := 14
const CM_REQUESTDIE := 15
const CM_REQUESTREVIVE := 16
const CM_REQUESTLOGOUT := 17
const CM_REQUESTHOSTOFFLINE := 18
const CM_REQUESTKILLPETS := 19
const CM_REQUESTRETRIEVESECUREDITEM := 20
const CM_REQUESTSPACEMOVE := 21
const CM_REQUESTMAGICDAMAGE := 22
const CM_PICKUP := 23
const CM_QUERYBASEMAPUID := 24
const CM_QUERYUIDBUFF := 26
const CM_QUERYPLAYERNAME := 27
const CM_QUERYPLAYERWLDESP := 28
const CM_QUERYCHATPEERLIST := 29
const CM_QUERYCHATMESSAGE := 30
const CM_CREATECHATGROUP := 31
const CM_CREATEACCOUNT := 32
const CM_NPCEVENT := 33
const CM_QUERYSELLITEMLIST := 34
const CM_DROPITEM := 35
const CM_CONSUMEITEM := 36
const CM_MAKEITEM := 37
const CM_BUY := 38
const CM_ADDFRIEND := 39
const CM_ACCEPTADDFRIEND := 40
const CM_REJECTADDFRIEND := 41
const CM_BLOCKPLAYER := 42
const CM_PLAYERBROADCAST := 45
const CM_REQUESTEQUIPWEAR := 46
const CM_REQUESTGRABWEAR := 47
const CM_REQUESTEQUIPBELT := 48
const CM_REQUESTGRABBELT := 49
const CM_REQUESTCONSUMEBELT := 50
const CM_REQUESTJOINTEAM := 51
const CM_REQUESTLEAVETEAM := 52
const CM_CREATETEAM := 53
const CM_SETTEAMREQUIREMENT := 54
const CM_QUERYTEAMINFO := 55
const CM_TRANSFERTEAMLEADER := 56
const CM_SETFOLLOWTEAMLEADER := 57
const CM_REQUESTLATESTCHATMESSAGE := 58
const CM_QUERYRANKING := 59
const CM_CLAIMDELIVERY := 60
const CM_QUERYAUCTIONITEMLIST := 61
const CM_REGISTERAUCTIONITEM := 62
const CM_BUYAUCTIONITEM := 63
const CM_UNREGISTERAUCTIONITEM := 64
const CM_REQUESTDIRECTTRADE := 65
const CM_RESPONDDIRECTTRADE := 66
const CM_UPDATEDIRECTTRADE := 67
const CM_COMMITDIRECTTRADE := 68
const CM_CANCELDIRECTTRADE := 69
const CM_ADDSTALLITEM := 70
const CM_REMOVESTALLITEM := 71
const CM_STARTSTALL := 72
const CM_CLOSESTALL := 73
const CM_QUERYSTALL := 74
const CM_BUYSTALLITEM := 75
const CM_QUERYPLAYERGUILD := 76
const CM_INVITEGUILDMEMBER := 77
const CM_RESPONDGUILDINVITE := 78
const CM_LEAVEGUILD := 79
const CM_KICKGUILDMEMBER := 80
const CM_TRANSFERGUILDLEADER := 81
const CM_DISBANDGUILD := 82
const CM_QUERYGUILDINFO := 83
const CM_SETGUILDNOTICE := 84
const CM_SETGUILDMEMBERTITLE := 85
const CM_GUILDCHAT := 86

## 服务端消息码（从 tables 反查确认）
const SM_PING := 3
const SM_UID := 4
const SM_LOGINOK := 5
const SM_LOGINERROR := 6
const SM_QUERYCHAROK := 11
const SM_ONLINEOK := 20
const SM_STARTGAMESCENE := 23
const SM_ACTION := 28
const SM_CORECORD := 29
const SM_HEALTH := 30
const SM_GOLD := 47
const SM_TEXT := 51
const SM_PLAYERSAY := 83
const SM_CODISAPPEAR := 102
const SM_NOTIFYDEAD := 32
const SM_DEADFADEOUT := 33
const SM_CASTMAGIC := 38
const SM_MISS := 37

## ---- 状态类（对应 processrunnet.cpp 各 on_SM_*）----
## ★ 码值一律从 sm_table.gd 读取，不凭记忆填
const SM_EXP := 34
const SM_INVENTORY := 55
const SM_BELT := 56
const SM_EQUIPWEAR := 65
const SM_EQUIPWEARERROR := 66
const SM_GRABWEAR := 67
const SM_GRABWEARERROR := 68
const SM_LEARNEDMAGICLIST := 26
const SM_BUFFIDLIST := 36
const SM_BUFF := 35
const SM_VIEWRANGE := 104
const SM_LOGOUTOK := 21
const SM_REMOVEITEM := 58
const SM_UPDATEITEM := 57
const SM_QUESTDESPLIST := 81
const SM_QUESTDESPUPDATE := 80
const SM_PLAYERCONFIG := 24
const SM_REVIVE := 103
const SM_OFFLINE := 39
const SM_STARTINPUT := 46
const SM_PARTEXT := 52
const SM_PLAYERBROADCAST := 84

## ---- NPC / 聊天 ----
## 注意 type 3 = 「带长度前缀的裸 body」，type 1 = 掩码压缩
const SM_NPCXMLLAYOUT := 42
const SM_NPCNAME := 105

## ---- 地面与交互 ----
const SM_PICKUPERROR := 40
const SM_STRIKEGRID := 49
const SM_SAFEZONELIST := 64
const SM_GROUNDITEMIDLIST := 62
const SM_GROUNDFIREWALLLIST := 63
const SM_STARTINVOP := 45
const SM_INVOPCOST := 48

## ---- 交易 / 商店 ----
const SM_NPCSELL := 43
const SM_SELLITEMLIST := 50
const SM_BUYSUCCEED := 60
const SM_BUYERROR := 61
const SM_AUCTIONITEMLIST := 87
const SM_STALL := 106
const SM_STALLERROR := 107
const SM_PLAYERSTALL := 108
const SM_STARTDIRECTTRADE := 97
const SM_UPDATEDIRECTTRADE := 98
const SM_CLOSEDIRECTTRADE := 100
const SM_COMPLETEDIRECTTRADE := 99
const SM_DIRECTTRADEERROR := 101

## ---- 社交 ----
const SM_TEAMINFO := 79
const SM_TEAMMEMBERLIST := 76
const SM_TEAMCANDIDATE := 77
const SM_TEAMERROR := 78
const SM_GUILDINFO := 112
const SM_GUILDERROR := 111
const SM_GUILDCHAT := 113
const SM_GUILDWARLIST := 114
const SM_FRIENDLIST := 25
const SM_RANKINGLIST := 85

const SM_CREATEACCOUNTOK := 7
const SM_CREATEACCOUNTERROR := 8
const SM_CHANGEPASSWORDOK := 9
const SM_CHANGEPASSWORDERROR := 10
const SM_QUERYCHARERROR := 12
const SM_CREATECHAROK := 13
const SM_CREATECHARERROR := 14
const SM_CREATECHATGROUP := 15
const SM_ADDFRIENDACCEPTED := 16
const SM_ADDFRIENDREJECTED := 17
const SM_DELETECHAROK := 18
const SM_DELETECHARERROR := 19
const SM_ONLINEERROR := 22
const SM_PLAYERWLDESP := 27
const SM_NEXTSTRIKE := 31
const SM_REMOVEGROUNDITEM := 41
const SM_NPCERROR := 44
const SM_PLAYERNAME := 53
const SM_BUILDVERSION := 54
const SM_REMOVESECUREDITEM := 59
const SM_EQUIPBELT := 69
const SM_EQUIPBELTERROR := 70
const SM_GRABBELT := 71
const SM_GRABBELTERROR := 72
const SM_WEARITEMDURATION := 73
const SM_SHOWSECUREDITEMLIST := 74
const SM_PLAYERTEAMLEADER := 75
const SM_CHATMESSAGELIST := 82
const SM_CLAIMDELIVERYERROR := 86
const SM_AUCTIONREGISTERERROR := 88
const SM_AUCTIONBUYERROR := 89
const SM_AUCTIONUNREGISTERERROR := 90
const SM_QUERYCHATPEERLISTOK := 91
const SM_QUERYCHATMESSAGEOK := 92
const SM_CHATMESSAGEOK := 93
const SM_ADDFRIENDOK := 94
const SM_CREATECHATGROUPOK := 95
const SM_REQUESTDIRECTTRADE := 96
const SM_QUERYSTALLOK := 109
const SM_PLAYERGUILD := 110

const SM_TABLE := preload("res://scripts/net/sm_table.gd")

var _tcp: StreamPeerTCP = null
var _rx: PackedByteArray = PackedByteArray()
var _resp_seq := 0   ## 请求序号，用于应答配对

var is_connected := false


## 注意：含 await，调用方必须用 `var ok = await client.connect_to(...)`
func connect_to(host: String = "127.0.0.1", port: int = 7000) -> bool:
	_tcp = StreamPeerTCP.new()
	var err := _tcp.connect_to_host(host, port)
	if err != OK:
		push_error("连接失败: %s:%d err=%d" % [host, port, err])
		return false

	# 等待连接完成（最多 3 秒）
	var waited := 0
	while _tcp.get_status() == StreamPeerTCP.STATUS_CONNECTING and waited < 300:
		_tcp.poll()
		await get_tree().process_frame
		waited += 1
	if _tcp.get_status() != StreamPeerTCP.STATUS_CONNECTED:
		push_error("连接超时")
		return false
	is_connected = true
	connected.emit()
	return true



func _process(_dt: float) -> void:
	if _tcp == null:
		return
	_tcp.poll()

	var status := _tcp.get_status()
	if status == StreamPeerTCP.STATUS_ERROR or status == StreamPeerTCP.STATUS_NONE:
		if is_connected:
			is_connected = false
			disconnected.emit("连接断开")
		return
	if status != StreamPeerTCP.STATUS_CONNECTED:
		return

	var avail := _tcp.get_available_bytes()
	if avail <= 0:
		return
	var r = _tcp.get_data(avail)
	if r[0] != OK:
		return
	var chunk: PackedByteArray = r[1]
	_rx.append_array(chunk)
	_drain()


## 流式拆包：能解几帧解几帧（netio.cpp 的递归读逻辑）
func _drain() -> void:
	while true:
		var consumed := _try_parse_one()
		if consumed <= 0:
			return
		_rx = _rx.slice(consumed)


## 返回消耗字节数；0 表示数据不足
func _try_parse_one() -> int:
	var n := _rx.size()
	if n < 1:
		return 0

	var head := _rx[0]
	var has_resp := (head & 0x80) != 0
	var code := head & 0x7f

	var spec: Array = SM_TABLE.get_spec(code)
	if spec.is_empty():
		decode_error.emit("未注册的服务端消息码 %d" % code)
		_rx = PackedByteArray()
		return -1

	var mtype: int = spec[0]
	var data_len: int = spec[1]
	var off := 1
	var resp_id := 0

	if has_resp:
		var r := _read_vint(_rx, off)
		if r[0] < 0:
			return 0
		off = r[0]
		resp_id = r[1]

	match mtype:
		0:
			frame_received.emit(code, PackedByteArray(), resp_id)
			return off
		2:
			if n < off + data_len:
				return 0
			frame_received.emit(code, _rx.slice(off, off + data_len), resp_id)
			return off + data_len
		1:
			var r2 := _read_vint(_rx, off)
			if r2[0] < 0:
				return 0
			off = r2[0]
			var data_count: int = r2[1]
			var mask_len := (data_len + 63) / 64 if data_len >= 256 else (data_len + 7) / 8
			if n < off + mask_len + data_count:
				return 0
			var mask := _rx.slice(off, off + mask_len)
			var comp := _rx.slice(off + mask_len, off + mask_len + data_count)
			var body := _xor_decode(mask, comp, data_len)
			if body.is_empty():
				decode_error.emit("XOR 解码失败 code=%d" % code)
				return -1
			frame_received.emit(code, body, resp_id)
			return off + mask_len + data_count
		3:
			var r3 := _read_vint(_rx, off)
			if r3[0] < 0:
				return 0
			off = r3[0]
			var body_size: int = r3[1]
			if n < off + body_size:
				return 0
			frame_received.emit(code, _rx.slice(off, off + body_size), resp_id)
			return off + body_size
	return -1


func _xor_decode(mask: PackedByteArray, comp: PackedByteArray, data_len: int) -> PackedByteArray:
	var out := PackedByteArray()
	out.resize(data_len)
	var ci := 0
	for i in range(data_len):
		var bi := i / 8
		if bi < mask.size() and (mask[bi] & (1 << (i % 8))) != 0:
			if ci >= comp.size():
				return PackedByteArray()
			out[i] = comp[ci]
			ci += 1
	return out


func _read_vint(buf: PackedByteArray, off: int) -> Array:
	var val := 0
	var shift := 0
	var i := off
	while i < buf.size():
		var b := buf[i]
		val |= (b & 0x7f) << shift
		i += 1
		if (b & 0x80) == 0:
			return [i, val]
		shift += 7
		if shift > 63:
			return [-1, 0]
	return [-1, 0]


## 发送一帧（netio.cpp:173）
func send(code: int, body: PackedByteArray = PackedByteArray(), use_resp: bool = false) -> int:
	if _tcp == null or not is_connected:
		return 0

	var resp_id := 0
	if use_resp:
		_resp_seq += 1
		resp_id = _resp_seq

	var spec: Array = SM_TABLE.get_cm_spec(code)
	if spec.is_empty():
		push_error("未注册的客户端消息码 %d" % code)
		return 0

	var mtype: int = spec[0]
	var data_len: int = spec[1]

	var out := PackedByteArray()
	if resp_id != 0:
		out.append(code | 0x80)
		out.append_array(_write_vint(resp_id))
	else:
		out.append(code)

	match mtype:
		0:
			pass
		2:
			out.append_array(body)
		3:
			out.append_array(_write_vint(body.size()))
			out.append_array(body)
		1:
			if body.size() != data_len:
				push_error("CM %d 需要 %d 字节，实际 %d" % [code, data_len, body.size()])
				return 0
			var mask_len := (data_len + 63) / 64 if data_len >= 256 else (data_len + 7) / 8
			var mask := PackedByteArray()
			mask.resize(mask_len)
			var comp := PackedByteArray()
			for i in range(data_len):
				if body[i] != 0:
					mask[i / 8] |= 1 << (i % 8)
					comp.append(body[i])
			out.append_array(_write_vint(comp.size()))
			out.append_array(mask)
			out.append_array(comp)

	_tcp.put_data(out)
	return resp_id


func _write_vint(v: int) -> PackedByteArray:
	var out := PackedByteArray()
	var x := v
	while true:
		var b := x & 0x7f
		x >>= 7
		if x > 0:
			out.append(b | 0x80)
		else:
			out.append(b)
			break
	return out


# ---------------- 业务封装 ----------------

## 登录（CMLogin = 136 字节：2 个 StaticBuffer<64>）
func login(account: String, password: String) -> int:
	var body := PackedByteArray()
	body.resize(136)
	body.fill(0)
	var a := account.to_utf8_buffer()
	body.encode_u16(0, a.size())
	for i in range(mini(a.size(), 64)):
		body[2 + i] = a[i]
	# ★ StaticBuffer<64> 线上占 68 字节，不是 67：
	#   C++ 是 `uint16_t size + uint8_t data[65]` = 67，
	#   但 uint16_t 要求 2 字节对齐，结构体被 padding 到 68。
	#   cerealf 按 POD 原始内存布局拷贝，所以第二个 buffer 从 68 开始。
	#   写成 67/69 会让服务端读到的长度字段错位（读到 0x3100 = 12544），
	#   表现就是「连上了但登录永远没回应」。
	var pw_off := 68
	var pw_bytes := password.to_utf8_buffer()
	body.encode_u16(pw_off, pw_bytes.size())
	for i in range(mini(pw_bytes.size(), 64)):
		body[pw_off + 2 + i] = pw_bytes[i]
	return send(CM_LOGIN, body, true)


func create_account(account: String, password: String) -> int:
	var b := _fixed_body(136)
	_write_static(b, 0, account, 64)
	_write_static(b, 68, password, 64)
	return send(CM_CREATEACCOUNT, b, true)


func change_password(account: String, old_pwd: String, new_pwd: String) -> int:
	var b := _fixed_body(204)
	_write_static(b, 0, account, 64)
	_write_static(b, 68, old_pwd, 64)
	_write_static(b, 136, new_pwd, 64)
	return send(CM_CHANGEPASSWORD, b, true)


func create_char(name_: String, job: int, gender: int) -> int:
	var b := _fixed_body(70)
	_write_static(b, 0, name_, 64)
	b[68] = job & 0xFF
	b[69] = gender & 0xFF
	return send(CM_CREATECHAR, b, true)


func delete_char(password: String) -> int:
	var b := _fixed_body(68)
	_write_static(b, 0, password, 64)
	return send(CM_DELETECHAR, b, true)


func query_char() -> int:
	return send(CM_QUERYCHAR, PackedByteArray(), true)


func online() -> int:
	return send(CM_ONLINE, PackedByteArray(), true)


func ping(tick: int) -> int:
	var body := PackedByteArray()
	body.resize(4)
	body.encode_u32(0, tick)
	return send(CM_PING, body, true)


func say(content: String) -> int:
	var body := PackedByteArray()
	body.resize(128)
	body.fill(0)
	var c := content.to_utf8_buffer()
	for i in range(mini(c.size(), 127)):
		body[i] = c[i]
	return send(CM_PLAYERSAY, body)


## 发送移动动作（CMAction = 44 字节）
func send_action(uid: int, map_uid: int, action_bytes: PackedByteArray) -> int:
	var body := PackedByteArray()
	body.resize(44)
	body.fill(0)
	body.encode_u64(0, uid)
	body.encode_u64(8, map_uid)
	for i in range(mini(action_bytes.size(), 28)):
		body[16 + i] = action_bytes[i]
	return send(CM_ACTION, body)



# ==================== 通用编码辅助 ====================

func _fixed_body(n: int) -> PackedByteArray:
	var b := PackedByteArray()
	b.resize(n)
	b.fill(0)
	return b


func _write_static(body: PackedByteArray, off: int, text: String, cap: int) -> void:
	var raw := text.to_utf8_buffer()
	var n: int = mini(raw.size(), cap)
	body.encode_u16(off, n)
	for i in range(n):
		body[off + 2 + i] = raw[i]


func _write_u64(body: PackedByteArray, off: int, v: int) -> void:
	body.encode_u64(off, v)


# ==================== 查询类上行 ====================

func query_gold() -> int:
	return send(CM_QUERYGOLD, PackedByteArray(), true)

func query_corecord(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_QUERYCORECORD, b, true)

func query_npc_name(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_QUERYNPCNAME, b, true)

func query_player_name(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_QUERYPLAYERNAME, b, true)

func query_player_wl_desp(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_QUERYPLAYERWLDESP, b, true)

func query_uid_buff(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_QUERYUIDBUFF, b, true)

func query_base_map_uid(map_id: int) -> int:
	var b := _fixed_body(4); b.encode_u32(0, map_id)
	return send(CM_QUERYBASEMAPUID, b, true)

func query_sell_item_list(npc_uid: int, item_id: int = 0) -> int:
	var b := _fixed_body(12)
	b.encode_u64(0, npc_uid); b.encode_u32(8, item_id)
	return send(CM_QUERYSELLITEMLIST, b, true)


# ==================== 动作 / 魔法 / 拾取 ====================

func request_die() -> int:
	return send(CM_REQUESTDIE, PackedByteArray())

func request_revive() -> int:
	return send(CM_REQUESTREVIVE, PackedByteArray(), true)

func request_logout() -> int:
	return send(CM_REQUESTLOGOUT, PackedByteArray(), true)

func request_host_offline() -> int:
	return send(CM_REQUESTHOSTOFFLINE, PackedByteArray())

func request_kill_pets() -> int:
	return send(CM_REQUESTKILLPETS, PackedByteArray())

func request_add_health(hp: int, mp: int) -> int:
	var b := _fixed_body(8)
	b.encode_s32(0, hp); b.encode_s32(4, mp)
	return send(CM_REQUESTADDHEALTH, b)

func request_add_exp(exp: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, exp)
	return send(CM_REQUESTADDEXP, b)

func request_space_move(map_uid: int, x: int, y: int) -> int:
	var b := _fixed_body(12)
	b.encode_u64(0, map_uid); b.encode_u16(8, x); b.encode_u16(10, y)
	return send(CM_REQUESTSPACEMOVE, b, true)

func request_magic_damage(magic_id: int, aim_uid: int) -> int:
	var b := _fixed_body(12)
	b.encode_u32(0, magic_id); b.encode_u64(4, aim_uid)
	return send(CM_REQUESTMAGICDAMAGE, b)

func pickup(x: int, y: int, map_uid: int) -> int:
	var b := _fixed_body(12)
	b.encode_u16(0, x); b.encode_u16(2, y); b.encode_u64(4, map_uid)
	return send(CM_PICKUP, b, true)

func set_magic_key(magic_id: int, key: String) -> int:
	var b := _fixed_body(5)
	b.encode_u32(0, magic_id)
	b[4] = key.to_utf8_buffer()[0] if not key.is_empty() else 0
	return send(CM_SETMAGICKEY, b)

func request_retrieve_secured_item(item_id: int, seq_id: int) -> int:
	var b := _fixed_body(8)
	b.encode_u32(0, item_id); b.encode_u32(4, seq_id)
	return send(CM_REQUESTRETRIEVESECUREDITEM, b, true)


# ==================== 背包 / 装备 / 商店 ====================

func drop_item(item_id: int, seq_id: int, count: int) -> int:
	var b := _fixed_body(10)
	b.encode_u32(0, item_id); b.encode_u32(4, seq_id); b.encode_u16(8, count)
	return send(CM_DROPITEM, b)

func consume_item(item_id: int, seq_id: int, count: int) -> int:
	var b := _fixed_body(10)
	b.encode_u32(0, item_id); b.encode_u32(4, seq_id); b.encode_u16(8, count)
	return send(CM_CONSUMEITEM, b)

func make_item(item_id: int, count: int) -> int:
	var b := _fixed_body(6)
	b.encode_u32(0, item_id); b.encode_u16(4, count)
	return send(CM_MAKEITEM, b)

func buy(npc_uid: int, item_id: int, seq_id: int, count: int) -> int:
	var b := _fixed_body(24)
	b.encode_u64(0, npc_uid); b.encode_u32(8, item_id)
	b.encode_u32(12, seq_id); b.encode_u64(16, count)
	return send(CM_BUY, b, true)

func request_equip_wear(item_id: int, seq_id: int, wltype: int) -> int:
	var b := _fixed_body(10)
	b.encode_u32(0, item_id); b.encode_u32(4, seq_id); b.encode_u16(8, wltype)
	return send(CM_REQUESTEQUIPWEAR, b, true)

func request_grab_wear(wltype: int) -> int:
	var b := _fixed_body(2); b.encode_u16(0, wltype)
	return send(CM_REQUESTGRABWEAR, b, true)

func request_equip_belt(item_id: int, seq_id: int, slot: int) -> int:
	var b := _fixed_body(10)
	b.encode_u32(0, item_id); b.encode_u32(4, seq_id); b.encode_u16(8, slot)
	return send(CM_REQUESTEQUIPBELT, b, true)

func request_grab_belt(slot: int) -> int:
	var b := _fixed_body(2); b.encode_u16(0, slot)
	return send(CM_REQUESTGRABBELT, b, true)

func request_consume_belt(slot: int) -> int:
	var b := _fixed_body(2); b.encode_u16(0, slot)
	return send(CM_REQUESTCONSUMEBELT, b)


# ==================== 聊天 / 好友 ====================

func player_broadcast(content: String) -> int:
	var b := _fixed_body(128)
	var raw := content.to_utf8_buffer()
	for i in range(mini(raw.size(), 127)):
		b[i] = raw[i]
	return send(CM_PLAYERBROADCAST, b)

func chat_message(to_cpid: int, content: String, ref_id: int = 0) -> int:
	var raw := content.to_utf8_buffer()
	var b := PackedByteArray()
	b.resize(16 + raw.size())
	b.encode_u64(0, to_cpid)
	var refbits := 0
	if ref_id != 0:
		refbits = 1 | ((ref_id & 0x7FFFFFFFFFFFFFFF) << 1)
	b.encode_u64(8, refbits)
	for i in range(raw.size()):
		b[16 + i] = raw[i]
	return send(CM_CHATMESSAGE, b, true)

func add_friend(cpid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, cpid)
	return send(CM_ADDFRIEND, b, true)

func accept_add_friend(cpid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, cpid)
	return send(CM_ACCEPTADDFRIEND, b, true)

func reject_add_friend(cpid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, cpid)
	return send(CM_REJECTADDFRIEND, b, true)

func block_player(cpid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, cpid)
	return send(CM_BLOCKPLAYER, b, true)

func query_chat_peer_list(input_text: String = "") -> int:
	var b := _fixed_body(132)
	_write_static(b, 0, input_text, 128)
	return send(CM_QUERYCHATPEERLIST, b, true)

func query_chat_message(msg_id: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, msg_id)
	return send(CM_QUERYCHATMESSAGE, b, true)


# ==================== 队伍 ====================

func create_team(max_member: int = 0, auto_accept: bool = false) -> int:
	var b := PackedByteArray()
	b.resize(9)
	b.fill(0)
	b.encode_s32(0, max_member)
	b[4] = 1 if auto_accept else 0
	# 3 x optional<SDTeamJobReq>: all absent
	b[5] = 0; b[6] = 0; b[7] = 0
	b[8] = 0 # cerealf CF_NONE
	return send(CM_CREATETEAM, b, true)

func request_join_team(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_REQUESTJOINTEAM, b, true)

func request_leave_team(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_REQUESTLEAVETEAM, b, true)

func query_team_info(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_QUERYTEAMINFO, b, true)

func transfer_team_leader(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_TRANSFERTEAMLEADER, b, true)

func set_follow_team_leader(follow: bool) -> int:
	var b := _fixed_body(1); b[0] = 1 if follow else 0
	return send(CM_SETFOLLOWTEAMLEADER, b)


# ==================== 直接交易 ====================

func request_direct_trade(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_REQUESTDIRECTTRADE, b, true)

func respond_direct_trade(uid: int, accept: bool) -> int:
	var b := _fixed_body(9)
	b.encode_u64(0, uid); b[8] = 1 if accept else 0
	return send(CM_RESPONDDIRECTTRADE, b, true)

func update_direct_trade_offer(uid: int, gold: int, locked: bool, items: Array) -> int:
	var n := items.size()
	var b := _fixed_body(8 + 8 + 1 + 8 + n * 16)
	b.encode_u64(0, uid)
	b.encode_u64(8, gold)
	b[16] = 1 if locked else 0
	b.encode_u64(17, n)
	var off := 25
	for it in items:
		b.encode_u32(off, int(it.item_id))
		b.encode_u32(off + 4, int(it.seq_id))
		b.encode_u64(off + 8, int(it.count))
		off += 16
	return send(CM_UPDATEDIRECTTRADE, b, true)

func commit_direct_trade(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_COMMITDIRECTTRADE, b, true)

func cancel_direct_trade(uid: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, uid)
	return send(CM_CANCELDIRECTTRADE, b)


# ==================== 摆摊 / 拍卖 / 仓库 ====================

func add_stall_item(item_id: int, seq_id: int, price: int) -> int:
	var b := _fixed_body(16)
	b.encode_u32(0, item_id); b.encode_u32(4, seq_id); b.encode_u64(8, price)
	return send(CM_ADDSTALLITEM, b, true)

func remove_stall_item(item_id: int, seq_id: int) -> int:
	var b := _fixed_body(8)
	b.encode_u32(0, item_id); b.encode_u32(4, seq_id)
	return send(CM_REMOVESTALLITEM, b, true)

func start_stall(name: String) -> int:
	var b := _fixed_body(52)
	_write_static(b, 0, name, 48)
	return send(CM_STARTSTALL, b, true)

func close_stall() -> int:
	return send(CM_CLOSESTALL, PackedByteArray(), true)

func query_stall(dbid: int) -> int:
	var b := _fixed_body(4); b.encode_u32(0, dbid)
	return send(CM_QUERYSTALL, b, true)

func buy_stall_item(seller_dbid: int, item_id: int, seq_id: int,
		count: int, price: int) -> int:
	var b := _fixed_body(28)
	b.encode_u32(0, seller_dbid); b.encode_u32(4, item_id)
	b.encode_u32(8, seq_id); b.encode_u64(12, count); b.encode_u64(20, price)
	return send(CM_BUYSTALLITEM, b, true)

func query_auction_item_list(category: int) -> int:
	var b := _fixed_body(1); b[0] = category & 0xff
	return send(CM_QUERYAUCTIONITEMLIST, b, true)

func register_auction_item(item_id: int, seq_id: int, price: int, note: String) -> int:
	var b := _fixed_body(148)
	b.encode_u32(0, item_id); b.encode_u32(4, seq_id)
	b.encode_u64(8, price); _write_static(b, 16, note, 128)
	return send(CM_REGISTERAUCTIONITEM, b, true)

func buy_auction_item(auction_id: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, auction_id)
	return send(CM_BUYAUCTIONITEM, b, true)

func unregister_auction_item(auction_id: int) -> int:
	var b := _fixed_body(8); b.encode_u64(0, auction_id)
	return send(CM_UNREGISTERAUCTIONITEM, b, true)


# ==================== 行会 ====================

func query_player_guild(dbid: int) -> int:
	var b := _fixed_body(4); b.encode_u32(0, dbid)
	return send(CM_QUERYPLAYERGUILD, b, true)

func invite_guild_member(name_: String) -> int:
	var b := _fixed_body(68)
	_write_static(b, 0, name_, 64)
	return send(CM_INVITEGUILDMEMBER, b, true)

func respond_guild_invite(guild_id: int, accept: bool) -> int:
	var b := _fixed_body(5)
	b.encode_u32(0, guild_id); b[4] = 1 if accept else 0
	return send(CM_RESPONDGUILDINVITE, b, true)

func leave_guild() -> int:
	return send(CM_LEAVEGUILD, PackedByteArray(), true)

func kick_guild_member(dbid: int) -> int:
	var b := _fixed_body(4); b.encode_u32(0, dbid)
	return send(CM_KICKGUILDMEMBER, b, true)

func transfer_guild_leader(dbid: int) -> int:
	var b := _fixed_body(4); b.encode_u32(0, dbid)
	return send(CM_TRANSFERGUILDLEADER, b, true)

func disband_guild() -> int:
	return send(CM_DISBANDGUILD, PackedByteArray(), true)

func query_guild_info() -> int:
	return send(CM_QUERYGUILDINFO, PackedByteArray(), true)

func set_guild_notice(notice: String) -> int:
	var b := _fixed_body(1028)
	_write_static(b, 0, notice, 1024)
	return send(CM_SETGUILDNOTICE, b, true)

func set_guild_member_title(dbid: int, title: String) -> int:
	var b := _fixed_body(40)
	b.encode_u32(0, dbid); _write_static(b, 4, title, 32)
	return send(CM_SETGUILDMEMBERTITLE, b, true)

func guild_chat(content: String) -> int:
	var b := _fixed_body(128)
	var raw := content.to_utf8_buffer()
	for i in range(mini(raw.size(), 127)):
		b[i] = raw[i]
	return send(CM_GUILDCHAT, b)


# ==================== NPC / 设置 / 排行 ====================

func npc_event(uid: int, path: String, event: String, value: String = "") -> int:
	var b := _fixed_body(410)
	b.encode_u64(0, uid)
	var pb := path.to_utf8_buffer()
	var eb := event.to_utf8_buffer()
	var vb := value.to_utf8_buffer()
	for i in range(mini(pb.size(), 99)): b[8 + i] = pb[i]
	for i in range(mini(eb.size(), 99)): b[108 + i] = eb[i]
	for i in range(mini(vb.size(), 199)): b[208 + i] = vb[i]
	b.encode_s16(408, vb.size())
	return send(CM_NPCEVENT, b, true)

func set_runtime_config(cfg_type: int, key_bytes: PackedByteArray,
		value_bytes: PackedByteArray) -> int:
	var b := _fixed_body(330)
	b.encode_u16(0, cfg_type)
	for i in range(mini(key_bytes.size(), 64)):
		b[2 + i] = key_bytes[i]
	for i in range(mini(value_bytes.size(), 256)):
		b[70 + i] = value_bytes[i]
	return send(CM_SETRUNTIMECONFIG, b, true)

func query_ranking(rank_type: int) -> int:
	var b := _fixed_body(1); b[0] = rank_type & 0xff
	return send(CM_QUERYRANKING, b, true)

func set_team_requirement(max_member: int = 0, auto_accept: bool = false) -> int:
	var b := _fixed_body(9)
	b.encode_s32(0, max_member)
	b[4] = 1 if auto_accept else 0
	b[5] = 0; b[6] = 0; b[7] = 0
	b[8] = 0
	return send(CM_SETTEAMREQUIREMENT, b, true)


func create_chat_group(group_name: String, member_ids: Array) -> int:
	var b := _fixed_body(2184)
	_write_static(b, 0, group_name, 128)
	var n: int = mini(member_ids.size(), 512)
	b.encode_u16(132, n)
	for i in range(n):
		b.encode_u32(134 + i * 4, int(member_ids[i]))
	return send(CM_CREATECHATGROUP, b, true)


func request_latest_chat_message(cpid_list: Array, limit_count: int,
		include_send: bool, include_recv: bool) -> int:
	var b := _fixed_body(1036)
	var n: int = mini(cpid_list.size(), 128)
	b.encode_u16(0, n)
	for i in range(n):
		b.encode_u64(2 + i * 8, int(cpid_list[i]))
	var bits := (limit_count & 0x3FFFFFFF)
	if include_send: bits |= (1 << 30)
	if include_recv: bits |= (1 << 31)
	b.encode_u32(1032, bits)
	return send(CM_REQUESTLATESTCHATMESSAGE, b, true)


func claim_delivery(record: String) -> int:
	var b := _fixed_body(36)
	_write_static(b, 0, record, 32)
	return send(CM_CLAIMDELIVERY, b, true)


func close_connection() -> void:
	if _tcp != null:
		_tcp.disconnect_from_host()
	is_connected = false
