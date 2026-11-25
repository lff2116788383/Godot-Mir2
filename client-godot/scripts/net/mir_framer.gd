extends Node
class_name MirFramer
## mir2x 传输层帧编解码器（纯客户端侧，只依赖协议表）
##
## 帧结构（详见 docs/protocol.md）：
##   [headCode(1B)] [respID:LEB128 ?] [bodySize:LEB128 ?] [mask ?] [body ?]
##
## 设计要点：
## 1. 消息形态由静态表决定，不来自线上数据（防伪造）。
## 2. type 1 解码必须用表里的 dataLen，不能用收到的 dataCount。
## 3. 拆包必须能处理「半帧」，所以解码器是流式累积的。

signal frame_decoded(head_code: int, body: PackedByteArray, resp_id: int)
signal decode_error(reason: String)

# 消息形态表：code -> [type, data_len]
# type: 0=空体 1=定长+XOR 2=定长直传 3=变长
const SM_TABLE := {
	0: [0, 0],
	1: [0, 0],
	2: [0, 0],
	3: [2, 0],
	4: [2, 0],
	5: [0, 0],
	6: [1, 0],
	7: [0, 0],
	8: [1, 0],
	9: [0, 0],
	10: [1, 0],
	11: [1, 0],
	12: [1, 0],
	13: [0, 0],
	14: [1, 0],
	15: [3, 0],
	16: [3, 0],
	17: [3, 0],
	18: [0, 0],
	19: [1, 0],
	20: [1, 0],
	21: [0, 0],
	22: [1, 0],
	23: [3, 0],
	24: [3, 0],
	25: [3, 0],
	26: [3, 0],
	27: [3, 0],
	28: [1, 0],
	29: [1, 0],
	30: [3, 0],
	31: [0, 0],
	32: [1, 0],
	33: [1, 0],
	34: [1, 0],
	35: [1, 0],
	36: [3, 0],
	37: [1, 0],
	38: [1, 0],
	39: [1, 0],
	40: [1, 0],
	41: [1, 0],
	42: [3, 0],
	43: [3, 0],
	44: [1, 0],
	45: [3, 0],
	46: [3, 0],
	47: [1, 0],
	48: [1, 0],
	49: [1, 0],
	50: [3, 0],
	51: [3, 0],
	52: [3, 0],
	53: [3, 0],
	54: [1, 0],
	55: [3, 0],
	56: [3, 0],
	57: [3, 0],
	58: [1, 0],
	59: [1, 0],
	60: [1, 0],
	61: [1, 0],
	62: [3, 0],
	63: [3, 0],
	64: [3, 0],
	65: [3, 0],
	66: [1, 0],
	67: [3, 0],
	68: [1, 0],
	69: [3, 0],
	70: [1, 0],
	71: [3, 0],
	72: [1, 0],
	73: [1, 0],
	74: [3, 0],
	75: [1, 0],
	76: [3, 0],
	77: [3, 0],
	78: [1, 0],
	79: [3, 0],
	80: [3, 0],
	81: [3, 0],
	82: [3, 0],
	83: [1, 0],
	84: [1, 0],
	85: [3, 0],
	86: [1, 0],
	87: [3, 0],
	88: [1, 0],
	89: [1, 0],
	90: [1, 0],
	91: [3, 0],
	92: [3, 0],
	93: [3, 0],
	94: [3, 0],
	95: [3, 0],
	96: [1, 0],
	97: [1, 0],
	98: [3, 0],
	99: [1, 0],
	100: [1, 0],
	101: [1, 0],
	102: [1, 0],
	103: [1, 0],
	104: [1, 0],
	105: [3, 0],
	106: [3, 0],
	107: [1, 0],
	108: [1, 0],
	109: [3, 0],
	110: [3, 0],
	111: [1, 0],
	112: [3, 0],
	113: [1, 0],
	114: [3, 0],
}

const CM_TABLE := {
	0: [0, 0],
	1: [2, 0],
	2: [1, 0],
	3: [0, 0],
	4: [1, 0],
	5: [1, 0],
	6: [1, 0],
	7: [0, 0],
	8: [1, 0],
	9: [1, 0],
	10: [1, 0],
	11: [1, 0],
	12: [1, 0],
	13: [1, 0],
	14: [1, 0],
	15: [0, 0],
	16: [0, 0],
	17: [0, 0],
	18: [0, 0],
	19: [0, 0],
	20: [1, 0],
	21: [1, 0],
	22: [1, 0],
	23: [1, 0],
	24: [1, 0],
	25: [0, 0],
	26: [1, 0],
	27: [1, 0],
	28: [1, 0],
	29: [1, 0],
	30: [1, 0],
	31: [1, 0],
	32: [1, 0],
	33: [1, 0],
	34: [1, 0],
	35: [1, 0],
	36: [1, 0],
	37: [1, 0],
	38: [1, 0],
	39: [1, 0],
	40: [1, 0],
	41: [1, 0],
	42: [1, 0],
	43: [3, 0],
	44: [1, 0],
	45: [1, 0],
	46: [1, 0],
	47: [1, 0],
	48: [1, 0],
	49: [1, 0],
	50: [1, 0],
	51: [1, 0],
	52: [1, 0],
	53: [3, 0],
	54: [3, 0],
	55: [1, 0],
	56: [1, 0],
	57: [1, 0],
	58: [1, 0],
	59: [2, 0],
	60: [1, 0],
	61: [2, 0],
	62: [1, 0],
	63: [1, 0],
	64: [1, 0],
	65: [1, 0],
	66: [1, 0],
	67: [3, 0],
	68: [1, 0],
	69: [1, 0],
	70: [1, 0],
	71: [1, 0],
	72: [1, 0],
	73: [0, 0],
	74: [1, 0],
	75: [1, 0],
	76: [1, 0],
	77: [1, 0],
	78: [1, 0],
	79: [0, 0],
	80: [1, 0],
	81: [1, 0],
	82: [0, 0],
	83: [0, 0],
	84: [1, 0],
	85: [1, 0],
	86: [1, 0],
}

# 关键消息码（P0）
const CM_PING := 1
const CM_LOGIN := 2
const CM_QUERYCHAR := 3
const CM_ONLINE := 7
const CM_ACTION := 8
const CM_PLAYERSAY := 44
const CM_CHATMESSAGE := 43

const SM_PING := 3
const SM_LOGINOK := 5
const SM_LOGINERROR := 6
const SM_QUERYCHAROK := 11
const SM_ONLINEOK := 20
const SM_STARTGAMESCENE := 23
const SM_ACTION := 28
const SM_CORECORD := 29
const SM_HEALTH := 30

var _rx: PackedByteArray = PackedByteArray()
var _table: Dictionary = {}


func _init() -> void:
	_table = CM_TABLE


## 只解码服务端消息，故用 SM 表覆盖
func set_sm_table(t: Dictionary) -> void:
	_table = t


func feed(chunk: PackedByteArray) -> void:
	_rx.append_array(chunk)
	_drain()


func _drain() -> void:
	while true:
		var consumed := _try_parse_one(_rx)
		if consumed <= 0:
			return
		_rx = _rx.slice(consumed)


## 返回消耗字节数；0 表示数据不足
func _try_parse_one(buf: PackedByteArray) -> int:
	var n := buf.size()
	if n < 1:
		return 0
	var code := buf[0]
	var has_resp := (code & 0x80) != 0
	var head := code & 0x7f

	if not _table.has(head):
		decode_error.emit("未注册的消息码 %d" % head)
		_rx = PackedByteArray()
		return -1

	var spec: Array = _table[head]
	var mtype: int = spec[0]
	var data_len: int = spec[1]

	var off := 1
	var resp_id := 0
	if has_resp:
		var r := _read_vint(buf, off)
		if r[0] < 0:
			return 0
		off = r[0]
		resp_id = r[1]
		if resp_id == 0:
			decode_error.emit("hasResp 但 respID 为 0")
			return -1

	match mtype:
		0:
			frame_decoded.emit(head, PackedByteArray(), resp_id)
			return off
		2:
			if n < off + data_len:
				return 0
			frame_decoded.emit(head, buf.slice(off, off + data_len), resp_id)
			return off + data_len
		1:
			var r2 := _read_vint(buf, off)
			if r2[0] < 0:
				return 0
			off = r2[0]
			var data_count: int = r2[1]
			var mask_len := (data_len + 63) / 64 if data_len >= 256 else (data_len + 7) / 8
			if n < off + mask_len + data_count:
				return 0
			var mask := buf.slice(off, off + mask_len)
			var comp := buf.slice(off + mask_len, off + mask_len + data_count)
			var body := _xor_decode(mask, comp, data_len)
			if body.is_empty():
				decode_error.emit("XOR 解码失败 code=%d" % head)
				return -1
			frame_decoded.emit(head, body, resp_id)
			return off + mask_len + data_count
		3:
			var r3 := _read_vint(buf, off)
			if r3[0] < 0:
				return 0
			off = r3[0]
			var body_size: int = r3[1]
			if n < off + body_size:
				return 0
			frame_decoded.emit(head, buf.slice(off, off + body_size), resp_id)
			return off + body_size
		_:
			decode_error.emit("非法消息类型 %d" % mtype)
			return -1


func _read_vint(buf: PackedByteArray, off: int) -> Array:
	# 返回 [新偏移, 值]；数据不足返回 [-1, 0]
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


func _xor_decode(mask: PackedByteArray, comp: PackedByteArray, data_len: int) -> PackedByteArray:
	var out := PackedByteArray()
	out.resize(data_len)
	var ci := 0
	if data_len >= 256:
		# 64 位宽掩码：每 8 字节一整块
		var i := 0
		while i < data_len:
			var blk := mini(8, data_len - i)
			var bit := (i / 64) * 8 + ((i % 64) / 8)
			var bi := bit / 8
			var bmask := (1 << (bit % 8)) if bi < mask.size() else 0
			if mask[bi] & bmask:
				if ci + blk > comp.size():
					return PackedByteArray()
				for k in range(blk):
					out[i + k] = comp[ci + k]
				ci += blk
			i += 8
	else:
		for i in range(data_len):
			var bi := i / 8
			if bi < mask.size() and (mask[bi] & (1 << (i % 8))) != 0:
				if ci >= comp.size():
					return PackedByteArray()
				out[i] = comp[ci]
				ci += 1
	return out


static func write_vint(v: int) -> PackedByteArray:
	var out := PackedByteArray()
	var x := v
	while true:
		var b := x & 0x7f
		x = x >> 7
		if x > 0:
			out.append(b | 0x80)
		else:
			out.append(b)
			break
	return out


## 组一个客户端请求帧
## respond_id 非 0 时 headCode 打上 0x80 并写入 respID
func encode_client(head: int, body: PackedByteArray, respond_id: int = 0) -> PackedByteArray:
	var out := PackedByteArray()
	if respond_id != 0:
		out.append(head | 0x80)
		out.append_array(write_vint(respond_id))
	else:
		out.append(head)

	var spec: Array = CM_TABLE.get(head, [3, 0])
	var mtype: int = spec[0]
	var data_len: int = spec[1]

	match mtype:
		0:
			pass
		2:
			out.append_array(body)
		3:
			out.append_array(write_vint(body.size()))
			out.append_array(body)
		1:
			var mask_len := (data_len + 63) / 64 if data_len >= 256 else (data_len + 7) / 8
			var mask := PackedByteArray()
			mask.resize(mask_len)
			var comp := PackedByteArray()
			if data_len >= 256:
				var i := 0
				while i < data_len:
					var blk := mini(8, data_len - i)
					var nonzero := false
					for k in range(blk):
						if i + k < body.size() and body[i + k] != 0:
							nonzero = true
							break
					if nonzero:
						var bit := (i / 64) * 8 + ((i % 64) / 8)
						mask[bit / 8] |= 1 << (bit % 8)
						for k in range(blk):
							comp.append(body[i + k] if i + k < body.size() else 0)
					i += 8
			else:
				for i in range(data_len):
					if i < body.size() and body[i] != 0:
						mask[i / 8] |= 1 << (i % 8)
						comp.append(body[i])
			out.append_array(write_vint(comp.size()))
			out.append_array(mask)
			out.append_array(comp)
	return out
