extends RefCounted
class_name ItemRecord
## 物品记录查询与判定
##
## 【逐行翻译】common/src/itemrecord.hpp（末尾 300-259 行的判定方法）
## 数据来自 data/itemrecord.json（1258 条，由 tools/inc2json.py 生成）
##
## 说明：原版 ItemRecord 是编译期常量表；这里改为运行时查 JSON，
## 判定函数逐条对应原版 constexpr 方法。

const SysConstRef6 := preload("res://scripts/core/sys_const.gd")
const ProtocolDefRef := preload("res://scripts/core/protocol_def.gd")

static var _records: Dictionary = {}
static var _loaded := false


## 加载数据表
static func load_data(path: String = "res://data/itemrecord.json") -> bool:
	if _loaded:
		return true
	if not FileAccess.file_exists(path):
		push_error("物品表不存在: " + path)
		return false
	var f := FileAccess.open(path, FileAccess.READ)
	if f == null:
		return false
	var txt := f.get_as_text()
	f.close()
	var parsed = JSON.parse_string(txt)
	if parsed == null or not parsed is Dictionary:
		push_error("物品表解析失败")
		return false
	_records = parsed.get("records", {})
	_loaded = true
	return true


static func count() -> int:
	return _records.size()


## 查记录，返回 Dictionary（空 dict 表示不存在）
static func get_record(item_id: int) -> Dictionary:
	if not _loaded:
		load_data()
	return _records.get(str(item_id), {})


static func exists(item_id: int) -> bool:
	return not get_record(item_id).is_empty()


# ---------------- 字段读取 ----------------

static func name(item_id: int) -> String:
	return str(get_record(item_id).get("name", ""))


static func type(item_id: int) -> String:
	return str(get_record(item_id).get("type", ""))


static func rarity(item_id: int) -> String:
	return str(get_record(item_id).get("rarity", ""))


static func weight(item_id: int) -> int:
	return int(get_record(item_id).get("weight", 0))


static func pkg_gfx_id(item_id: int) -> int:
	return int(get_record(item_id).get("pkgGfxID", 0))


static func shape(item_id: int) -> int:
	return int(get_record(item_id).get("shape", 0))


static func price(item_id: int) -> int:
	return int(get_record(item_id).get("price", 0))


static func description(item_id: int) -> String:
	return str(get_record(item_id).get("description", ""))


## 装备属性子块
static func equip(item_id: int) -> Dictionary:
	return get_record(item_id).get("equip", {})


static func potion(item_id: int) -> Dictionary:
	return get_record(item_id).get("potion", {})


static func dope(item_id: int) -> Dictionary:
	return get_record(item_id).get("dope", {})


static func poison(item_id: int) -> Dictionary:
	return get_record(item_id).get("poison", {})


static func book(item_id: int) -> Dictionary:
	return get_record(item_id).get("book", {})


# ---------------- operator bool（itemrecord.hpp:145） ----------------
## 有效 = name 与 type 均非空
static func is_valid(item_id: int) -> bool:
	return not name(item_id).is_empty() and not type(item_id).is_empty()


# ---------------- 类型判定（itemrecord.hpp:220+） ----------------

static func is_gold(item_id: int) -> bool:
	return type(item_id) == "金币"


static func is_dress(item_id: int) -> bool:
	return type(item_id) == "衣服"


static func is_weapon(item_id: int) -> bool:
	return type(item_id) == "武器"


static func is_ring(item_id: int) -> bool:
	return type(item_id) == "戒指"


static func is_helmet(item_id: int) -> bool:
	return type(item_id) == "头盔"


static func is_book(item_id: int) -> bool:
	return type(item_id) == "技能书"


## 秘籍：技能书且有 magic 字段（itemrecord.hpp:229）
static func is_magic_book(item_id: int) -> bool:
	if not is_book(item_id):
		return false
	return not str(book(item_id).get("magic", "")).is_empty()


static func is_potion(item_id: int) -> bool:
	return type(item_id) == "恢复药水"


static func is_dope(item_id: int) -> bool:
	return type(item_id) == "强效药水"


static func is_powder(item_id: int) -> bool:
	return type(item_id) == "药粉"


## 可堆叠（itemrecord.hpp:161）
static func packable(item_id: int) -> bool:
	var t := type(item_id)
	return t == "恢复药水" or t == "传送卷轴" or t == "护身符" or t == "药粉"


## 可放腰带（itemrecord.hpp:170）
static func beltable(item_id: int) -> bool:
	var t := type(item_id)
	return t == "恢复药水" or t == "传送卷轴"


## 衣服性别（itemrecord.hpp:252）
## 返回 true=男 / false=女 / null=不适用
static func cloth_gender(item_id: int) -> Variant:
	if type(item_id) != "衣服":
		return null
	var n := name(item_id)
	if n.contains("（男）"):
		return true
	if n.contains("（女）"):
		return false
	return null


# ---------------- 穿戴判定（itemrecord.hpp:176） ----------------
## 逐条对应原版 switch
static func wearable(item_id: int, wltype: int) -> bool:
	var t := type(item_id)
	match wltype:
		ProtocolDefRef.WLG_DRESS: return t == "衣服"
		ProtocolDefRef.WLG_HELMET: return t == "头盔"
		ProtocolDefRef.WLG_WEAPON: return t == "武器"
		ProtocolDefRef.WLG_SHOES: return t == "鞋"
		ProtocolDefRef.WLG_NECKLACE: return t == "项链"
		ProtocolDefRef.WLG_ARMRING0: return t == "手镯"
		ProtocolDefRef.WLG_ARMRING1: return t == "手镯"
		ProtocolDefRef.WLG_RING0: return t == "戒指"
		ProtocolDefRef.WLG_RING1: return t == "戒指"
		ProtocolDefRef.WLG_TORCH: return t == "火把"
		ProtocolDefRef.WLG_CHARM:
			# 原版：u8"魅力|护身符|药粉".find(type) != npos
			return t == "魅力" or t == "护身符" or t == "药粉"
	return false


# ---------------- 装备需求（itemrecord.hpp 的 EquipReq） ----------------

static func equip_req(item_id: int) -> Dictionary:
	return equip(item_id).get("req", {})


static func req_level(item_id: int) -> int:
	return int(equip_req(item_id).get("level", 0))


## 职业要求，返回职业位掩码；0 表示无要求
static func req_job_mask(item_id: int) -> int:
	var job := str(equip_req(item_id).get("job", ""))
	if job.is_empty():
		return 0
	# 原版按中文职业名匹配
	var mask := 0
	if job.contains("战士"):
		mask |= ProtocolDefRef.JOB_WARRIOR
	if job.contains("道士"):
		mask |= ProtocolDefRef.JOB_TAOIST
	if job.contains("法师"):
		mask |= ProtocolDefRef.JOB_WIZARD
	return mask


## 能力值要求：返回 {dc, mc, sc, level}
static func req_all(item_id: int) -> Dictionary:
	var r := equip_req(item_id)
	return {
		"dc": int(r.get("dc", 0)),
		"mc": int(r.get("mc", 0)),
		"sc": int(r.get("sc", 0)),
		"level": int(r.get("level", 0)),
	}


## 耐久上限（itemrecord.hpp 的 equip.duration）
static func max_duration(item_id: int) -> int:
	return int(equip(item_id).get("duration", 0))


## 武器分类（用于音效选择）
static func weapon_category(item_id: int) -> String:
	var w: Dictionary = equip(item_id).get("weapon", {})
	return str(w.get("category", ""))


## 是否双手武器
static func is_double_hand(item_id: int) -> bool:
	var w: Dictionary = equip(item_id).get("weapon", {})
	return int(w.get("doubleHand", 0)) != 0


# ---------------- 便捷：按名字查 ID ----------------
## 反查接口（原版 DBCOM_ITEMID）
static func find_id_by_name(item_name: String) -> int:
	if not _loaded:
		load_data()
	for k in _records.keys():
		if str(_records[k].get("name", "")) == item_name:
			return int(k)
	return 0
