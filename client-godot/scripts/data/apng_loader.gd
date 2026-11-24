extends RefCounted
class_name ApngLoader
## APNG 拆帧加载器
##
## 问题：mir2x 的怪物贴图是 APNG（一文件多帧），而 Godot 的
##       `Image.load_from_file` 只读第一帧。
##
## 方案：手动解析 PNG 块流，把每一帧的 IDAT/fdAT 重新封装成独立 PNG。
##       Godot 再逐帧 `load_png_from_buffer` 得到 Image。
##
## APNG 结构（实测 texture/monster/00000000FF6FD9.PNG）：
##   IHDR, acTL,
##   fcTL, IDAT,          <- 第 0 帧：数据在 IDAT
##   fcTL, fdAT,          <- 第 1 帧：数据在 fdAT（前 4 字节为序号，需跳过）
##   fcTL, fdAT, ...
##   IEND
##
## 【依据】client/src/pngtexoffdb.cpp:26-40
##   `loadPNGAnimation(dataBuf)` 一次读入全部帧
##   `element.frameList[frame]` 按帧号取

## 缓存：路径 -> Array[Texture2D]
static var _cache: Dictionary = {}
## 统计
static var _loaded_files := 0
static var _loaded_frames := 0


## 主入口：返回该 APNG 的全部帧纹理
static func load_frames(abs_path: String) -> Array:
	if _cache.has(abs_path):
		return _cache[abs_path]
	if not FileAccess.file_exists(abs_path):
		_cache[abs_path] = []
		return []

	var data := FileAccess.get_file_as_bytes(abs_path)
	var frames := _split_apng(data)
	var out: Array = []
	for f in frames:
		var img := Image.new()
		if img.load_png_from_buffer(f) != OK:
			continue
		var tex := ImageTexture.create_from_image(img)
		out.append(tex)
	_loaded_files += 1
	_loaded_frames += out.size()
	_cache[abs_path] = out
	return out


## 只取第一帧（非 APNG 或只需要静帧时）
static func load_first(abs_path: String) -> Texture2D:
	var fr := load_frames(abs_path)
	return fr[0] if not fr.is_empty() else null


## 拆帧核心：返回 Array[PackedByteArray]，每项是一个完整 PNG 文件
static func _split_apng(data: PackedByteArray) -> Array:
	if data.size() < 8 or data[0] != 0x89 or data[1] != 0x50:
		return []
	var chunks := _read_chunks(data)
	if chunks.is_empty():
		return []

	# 找 IHDR
	var ihdr := PackedByteArray()
	for c in chunks:
		if c["type"] == "IHDR":
			ihdr = c["data"]
			break
	if ihdr.is_empty():
		return []

	# 是否 APNG
	var is_apng := false
	for c in chunks:
		if c["type"] == "acTL":
			is_apng = true
			break
	if not is_apng:
		# 普通 PNG，直接返回原数据
		return [data]

	# 调色板 PNG（color type 3）必须保留 PLTE，可能还有 tRNS（透明度表）。
	# 这两块是整图共享的，不属于某帧，重建时插在 IHDR 之后、IDAT 之前。
	# 选人图库 74 张里有一部分就是调色板 PNG，丢了 PLTE 会报
	# "IDAT: Missing PLTE before IDAT" 并整帧加载失败。
	var plte := PackedByteArray()
	var trns := PackedByteArray()
	for c in chunks:
		if c["type"] == "PLTE":
			plte = c["data"]
		elif c["type"] == "tRNS":
			trns = c["data"]

	# 按 fcTL 分帧：每帧的压缩数据 = 该 fcTL 之后到下一个 fcTL 之前的所有 IDAT/fdAT
	var frame_datas: Array = []
	var cur := PackedByteArray()
	var started := false
	for c in chunks:
		match c["type"]:
			"fcTL":
				if started:
					frame_datas.append(cur)
				cur = PackedByteArray()
				started = true
			"IDAT":
				cur.append_array(c["data"])
			"fdAT":
				# fdAT = u32 序号 + 压缩数据，跳过前 4 字节
				var d: PackedByteArray = c["data"]
				if d.size() > 4:
					cur.append_array(d.slice(4))
			"IEND":
				if started:
					frame_datas.append(cur)
				started = false
	if started:
		frame_datas.append(cur)

	# 每帧重新封成独立 PNG
	var out: Array = []
	for fd in frame_datas:
		if fd.is_empty():
			continue
		out.append(_build_png(ihdr, fd, plte, trns))
	return out


## 读 PNG 块流：返回 [{type:String, data:PackedByteArray}]
static func _read_chunks(data: PackedByteArray) -> Array:
	var out: Array = []
	var i := 8   # 跳过 8 字节签名
	var n := data.size()
	while i + 8 <= n:
		var ln := (data[i] << 24) | (data[i + 1] << 16) | (data[i + 2] << 8) | data[i + 3]
		if ln < 0 or i + 12 + ln > n:
			break
		var typ := _chunk_type(data, i + 4)
		var body := data.slice(i + 8, i + 8 + ln)
		out.append({"type": typ, "data": body})
		i += 12 + ln
		if typ == "IEND":
			break
	return out


static func _chunk_type(data: PackedByteArray, off: int) -> String:
	var s := ""
	for k in range(4):
		s += char(data[off + k])
	return s


## 用 IHDR + 压缩数据 组装一个标准 PNG
## PLTE / tRNS 是调色板 PNG 的必要块，必须在 IDAT 之前（PNG 规范 11.2.3）。
## color type 3 缺 PLTE 会让解码器直接判为损坏。
static func _build_png(ihdr: PackedByteArray, idat: PackedByteArray,
		plte: PackedByteArray = PackedByteArray(),
		trns: PackedByteArray = PackedByteArray()) -> PackedByteArray:
	var png := PackedByteArray()
	png.append_array(PackedByteArray([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]))
	png.append_array(_make_chunk("IHDR", ihdr))
	if not plte.is_empty():
		png.append_array(_make_chunk("PLTE", plte))
	if not trns.is_empty():
		png.append_array(_make_chunk("tRNS", trns))
	png.append_array(_make_chunk("IDAT", idat))
	png.append_array(_make_chunk("IEND", PackedByteArray()))
	return png


## 组装一个 PNG 块（长度 + 类型 + 数据 + CRC32）
static func _make_chunk(typ: String, body: PackedByteArray) -> PackedByteArray:
	var out := PackedByteArray()
	var ln := body.size()
	out.append((ln >> 24) & 0xFF)
	out.append((ln >> 16) & 0xFF)
	out.append((ln >> 8) & 0xFF)
	out.append(ln & 0xFF)
	var tb := PackedByteArray()
	for k in range(typ.length()):
		tb.append(typ.unicode_at(k))
	out.append_array(tb)
	out.append_array(body)

	# CRC32（PNG 用标准 CRC-32，多项式 0xEDB88320）
	var crc := _crc32(tb + body)
	out.append((crc >> 24) & 0xFF)
	out.append((crc >> 16) & 0xFF)
	out.append((crc >> 8) & 0xFF)
	out.append(crc & 0xFF)
	return out


static func _crc32(buf: PackedByteArray) -> int:
	var c := 0xFFFFFFFF
	for b in buf:
		c ^= b
		for _k in range(8):
			if c & 1:
				c = (c >> 1) ^ 0xEDB88320
			else:
				c >>= 1
	return c ^ 0xFFFFFFFF


## 文件名解析：KEY(8) + DX(3, 12位补码) + DY(3, 12位补码)
## 【依据】pngtexoffdb.cpp:31-36
##   dx = hexstr::to_2comp(fileName +  8, 3)
##   dy = hexstr::to_2comp(fileName + 11, 3)
static func decode_file_name(base: String) -> Dictionary:
	if base.length() < 14:
		return {}
	return {
		"key": base.substr(0, 8).hex_to_int(),
		"dx": _to_2comp(base.substr(8, 3)),
		"dy": _to_2comp(base.substr(11, 3)),
	}


## hexstr::to_2comp（hexstr.hpp）
## 12 位补码：最高位为 1 表示负数
static func _to_2comp(s: String) -> int:
	var v := s.hex_to_int()
	var bits := s.length() * 4       # 3 位 hex = 12 bit
	var sign_bit := 1 << (bits - 1)
	if v & sign_bit:
		return v - (1 << bits)
	return v


static func stats() -> Dictionary:
	return {"files": _loaded_files, "frames": _loaded_frames, "cached": _cache.size()}
