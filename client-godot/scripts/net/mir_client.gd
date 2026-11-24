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


func close_connection() -> void:
	if _tcp != null:
		_tcp.disconnect_from_host()
	is_connected = false
