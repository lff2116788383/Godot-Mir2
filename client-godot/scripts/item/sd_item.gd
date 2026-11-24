extends RefCounted
class_name SDItem
## 物品数据结构
##
## 【逐行翻译】common/src/sditem.hpp（226 行）
##
## 序列化（sditem.hpp:158）：
##   ar(itemID, seqID, count, duration[0], duration[1], extAttrList)
## 全部走 cereal PortableBinary，即小端 + u64 长度前缀。

# ---------------- XML 布局类型（sditem.hpp:22） ----------------
const XMLLAYOUT_BEGIN := 0
const XMLLAYOUT_FULL := 0
const XMLLAYOUT_ATTRIBUTE := 1
const XMLLAYOUT_END := 2

# ---------------- XML 参数类型（sditem.hpp:30） ----------------
const XML_NONE := 0
const XML_BEGIN := 1
const XML_DC := 1
const XML_MC := 2
const XML_SC := 3
const XML_LEVEL := 4
const XML_JOB := 5
const XML_PRICE := 6
const XML_PRICECOLOR := 7
const XML_END := 8

# ---------------- 额外属性键（sditem.hpp:47-139） ----------------
## 原版用 __COUNTER__ 自动编号，这里按出现顺序显式写出，值必须一致
const EA_NONE := 0
const EA_BEGIN := 1

const EA_DC := 1
const EA_MC := 2
const EA_SC := 3
const EA_AC := 4
const EA_MAC := 5

const EA_DCHIT := 6
const EA_MCHIT := 7

const EA_DCDODGE := 8
const EA_MCDODGE := 9

const EA_SPEED := 10
const EA_COMFORT := 11
const EA_LUCKCURSE := 12

const EA_HPADD := 13
const EA_HPSTEAL := 14
const EA_HPRECOVER := 15

const EA_MPADD := 16
const EA_MPSTEAL := 17
const EA_MPRECOVER := 18

const EA_DCFIRE := 19
const EA_DCICE := 20
const EA_DCLIGHT := 21
const EA_DCWIND := 22
const EA_DCHOLY := 23
const EA_DCDARK := 24
const EA_DCPHANTOM := 25

const EA_ACFIRE := 26
const EA_ACICE := 27
const EA_ACLIGHT := 28
const EA_ACWIND := 29
const EA_ACHOLY := 30
const EA_ACDARK := 31
const EA_ACPHANTOM := 32

const EA_LOADBODY := 33
const EA_LOADWEAPON := 34
const EA_LOADINVENTORY := 35

const EA_EXTEXP := 36
const EA_EXTGOLD := 37
const EA_EXTDROP := 38

const EA_BUFFID := 39

const EA_TELEPORT := 40
const EA_COLOR := 41
## once worn the player can not take it off, see GWERR_BIND
const EA_BIND := 42
## ore purity / meat quality
const EA_QUALITY := 43

const EA_END := 44

# ---------------- 字段（sditem.hpp:141-156） ----------------

var item_id: int = 0
var seq_id: int = 0
var count: int = 1

## duration[0]: current durability
## duration[1]: max durability of this item
##
## duration[1] == 0 means the item has no durability at all
## duration[1] >  0 and duration[0] == 0 means the item is broken, it contributes nothing
var duration: Array[int] = [0, 0]

## int -> 序列化后的值(bytes)
var ext_attr_list: Dictionary = {}


## itemIDSeq（sditem.hpp:186）
func item_id_seq() -> int:
	return (item_id << 32) | seq_id


## operator bool（sditem.hpp:198）
func is_valid() -> bool:
	return item_id != 0 and seq_id != 0 and count != 0


## 是否金币（sditem.hpp:193）
## 原版比对 ItemRecord.type == "金币"，这里用常量名判断
func is_gold(item_type: String) -> bool:
	return item_type == "金币"


## getExtAttr（sditem.hpp:202）
## 返回序列化后的原始字节，调用方自行反序列化
func get_ext_attr(ea_key: int) -> PackedByteArray:
	if ext_attr_list.has(ea_key):
		return ext_attr_list[ea_key]
	return PackedByteArray()


func has_ext_attr(ea_key: int) -> bool:
	return ext_attr_list.has(ea_key)


## buildItemList（sditem.hpp:199）
## 生成 count 个 seqID 递增的物品
static func build_item_list(item_id_v: int, item_count: int) -> Array:
	var result := []
	for i in range(item_count):
		var it := SDItem.new()
		it.item_id = item_id_v
		it.seq_id = i + 1
		it.count = 1
		result.append(it)
	return result


## buildGoldItem（sditem.hpp:200）
static func build_gold_item(gold_count: int) -> Array:
	return build_item_list(0, gold_count)   ## 具体 itemID 由调用方按 DBCOM 填入


# ---------------- 序列化（sditem.hpp:158） ----------------
## 对应 ar(itemID, seqID, count, duration[0], duration[1], extAttrList)
## extAttrList 是 std::unordered_map<int, std::string>，
## cereal 序列化为 u64 元素数 + (key:u64, value:u64 len + bytes) 重复

func to_bytes() -> PackedByteArray:
	var out := PackedByteArray()
	out.append_array(_u64(item_id))
	out.append_array(_u64(seq_id))
	out.append_array(_u64(count))
	out.append_array(_u64(duration[0]))
	out.append_array(_u64(duration[1]))
	out.append_array(_u64(ext_attr_list.size()))
	for k in ext_attr_list.keys():
		out.append_array(_u64(int(k)))
		var v: PackedByteArray = ext_attr_list[k]
		out.append_array(_u64(v.size()))
		out.append_array(v)
	return out


static func from_bytes(buf: PackedByteArray, off: int = 0) -> Array:
	## 返回 [SDItem, 新偏移]
	var it := SDItem.new()
	if buf.size() < off + 40:
		return [it, off]
	it.item_id = buf.decode_u64(off); off += 8
	it.seq_id = buf.decode_u64(off); off += 8
	it.count = buf.decode_u64(off); off += 8
	it.duration[0] = buf.decode_u64(off); off += 8
	it.duration[1] = buf.decode_u64(off); off += 8
	var map_size := buf.decode_u64(off); off += 8
	for i in range(map_size):
		if buf.size() < off + 16:
			break
		var k := buf.decode_u64(off); off += 8
		var vlen := buf.decode_u64(off); off += 8
		var v := buf.slice(off, off + vlen)
		off += vlen
		it.ext_attr_list[int(k)] = v
	return [it, off]


static func _u64(v: int) -> PackedByteArray:
	var b := PackedByteArray()
	b.resize(8)
	b.encode_u64(0, v)
	return b


func to_str() -> String:
	return "SDItem{id=%d seq=%d cnt=%d dur=%d/%d attrs=%d}" % [
		item_id, seq_id, count, duration[0], duration[1], ext_attr_list.size()
	]
