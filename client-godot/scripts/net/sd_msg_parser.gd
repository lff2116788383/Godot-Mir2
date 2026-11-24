extends RefCounted
class_name SdMsgParser
## 服务端状态消息解析（cerealf PortableBinary 子集）
##
## 【逐行翻译】client/src/processrunnet.cpp 的 on_SM_* 系列
##
## ══════════════ cereal PortableBinary 的实际编码 ══════════════
##   本项目里的消息体统一是「小端 + 每个容器前加 u64 长度」：
##     POD（int/u64/uint8 …）  : 定长小端
##     std::vector<T>          : u64 size + T×size
##     std::array<T,N>         : 无长度前缀，直接 N 个 T
##     std::string / u8string  : u64 size + 字节
##     std::map<K,V>           : u64 size + (K,V)×size
##     std::optional<T>        : u8 has_value + T（has_value 时）
##     std::variant           : 见各自的 deserialize 实现
##     StaticBuffer<N>         : u16 size + (N+1) 字节
##   消息体第 0 字节是端序标记（1 = 小端），**已由 MirClient 剥掉**，
##   进入本类的 buf 直接从字段开始。
##
## ══════════════ 已覆盖的消息（按原版处理函数）══════════════
##   SM_HEALTH          SDHealth        -> 角色血量/蓝量
##   SM_GOLD            SMGold          -> 金币
##   SM_EXP             SMExp           -> 经验
##   SM_INVENTORY       SDInventory     -> 整个背包
##   SM_BELT            SDBelt          -> 腰带 6 格
##   SM_EQUIPWEAR       SDEquipWear     -> 穿戴装备
##   SM_GRABWEAR        SDGrabWear      -> 脱下装备
##   SM_REMOVEITEM      SMRemoveItem    -> 删除背包物品
##   SM_UPDATEITEM      SMUpdateItem    -> 更新背包物品
##   SM_LEARNEDMAGICLIST SMLearnedMagicList -> 已学魔法
##   SM_BUFFIDLIST      SDBuffIDList    -> buff 列表
##   SM_QUESTDESPLIST   SDQuestDespList -> 任务
##   SM_VIEWRANGE       SMViewRange     -> 视野范围
##   SM_PLAYERCONFIG    SDPlayerConfig  -> 玩家配置
##
## 每个 parse_* 返回 Dictionary（字段名对齐 C++ 结构体成员），
## 解析失败返回空字典 —— 调用方按「空 = 忽略」处理，与原版容错一致。

const SDItemRef := preload("res://scripts/item/sd_item.gd")


# ==================== 基础读取 ====================

static func u8(b: PackedByteArray, o: int) -> int:
	if o < 0 or o >= b.size():
		return 0
	return b[o]


static func u16(b: PackedByteArray, o: int) -> int:
	if o + 2 > b.size():
		return 0
	return b.decode_u16(o)


static func u32(b: PackedByteArray, o: int) -> int:
	if o + 4 > b.size():
		return 0
	return b.decode_u32(o)


static func u64(b: PackedByteArray, o: int) -> int:
	if o + 8 > b.size():
		return 0
	return b.decode_u64(o)


## 有符号 int（C++ int 是 32 位）
static func i32(b: PackedByteArray, o: int) -> int:
	if o + 4 > b.size():
		return 0
	return b.decode_s32(o)


## std::string / u8string（u64 长度 + 字节）
## 返回 [字符串, 新偏移]
## ★ 不能叫 str() —— 会和 GDScript 内置的 str() 冲突，且返回值类型对不上
static func rd_str(b: PackedByteArray, o: int) -> Array:
	var n := u64(b, o)
	o += 8
	if n < 0 or n > 65536 or o + n > b.size():
		return ["", b.size()]
	return [b.slice(o, o + n).get_string_from_utf8(), o + n]


## std::optional<T>：u8 标志 + T
## 返回 [has, 新偏移]；has 时调用方再按 T 解析
static func opt_head(b: PackedByteArray, o: int) -> Array:
	var has := u8(b, o)
	return [has != 0, o + 1]


## std::vector 长度
static func vec_len(b: PackedByteArray, o: int) -> Array:
	var n := u64(b, o)
	if n < 0 or n > 1000000:
		return [0, b.size()]
	return [n, o + 8]


# ==================== 各消息解析 ====================

## SM_HEALTH -> SDHealth（serdesmsg.hpp:1298）
##   uint64 uid; int hp; int mp; int maxHP; int maxMP; int hpRecover; int mpRecover;
##   （后面还有缓冲/元素抗性等，本工程只取前 7 个常用字段）
static func parse_health(b: PackedByteArray) -> Dictionary:
	if b.size() < 8 + 4 * 6:
		return {}
	return {
		"uid": u64(b, 0),
		"hp": i32(b, 8),
		"mp": i32(b, 12),
		"max_hp": i32(b, 16),
		"max_mp": i32(b, 20),
		"hp_recover": i32(b, 24),
		"mp_recover": i32(b, 28),
	}


## SM_GOLD -> SMGold（uint64 gold）
static func parse_gold(b: PackedByteArray) -> Dictionary:
	if b.size() < 8:
		return {}
	return {"gold": u64(b, 0)}


## SM_EXP -> SMExp（uint64 exp）
static func parse_exp(b: PackedByteArray) -> Dictionary:
	if b.size() < 8:
		return {}
	return {"exp": u64(b, 0)}


## SM_INVENTORY -> SDInventory（std::vector<SDItem>）
static func parse_inventory(b: PackedByteArray) -> Dictionary:
	var r := vec_len(b, 0)
	var n: int = int(r[0])
	var off: int = int(r[1])
	var items: Array = []
	for i in range(n):
		# SDItem.from_bytes 恒返回 [item, 新偏移]（不会返回空）
		var got: Array = SDItemRef.from_bytes(b, off)
		var new_off: int = int(got[1])
		if new_off <= off:
			break            # 偏移没推进，说明数据不够，停止
		items.append(got[0])
		off = new_off
	return {"items": items}


## SM_BELT -> SDBelt（std::array<SDItem,6>，无长度前缀）
static func parse_belt(b: PackedByteArray) -> Dictionary:
	var items: Array = []
	var off := 0
	for i in range(6):
		if off >= b.size():
			items.append(null)
			continue
		var got: Array = SDItemRef.from_bytes(b, off)
		var new_off: int = int(got[1])
		if new_off <= off:
			items.append(null)
			break
		items.append(got[0])
		off = new_off
	while items.size() < 6:
		items.append(null)
	return {"items": items}


## SM_EQUIPWEAR -> SDEquipWear（uid, wltype, item）
static func parse_equip_wear(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var wltype := i32(b, 8)
	var got: Array = SDItemRef.from_bytes(b, 12)
	return {"uid": uid, "wltype": wltype, "item": got[0]}


## SM_GRABWEAR -> SDGrabWear（uid, wltype, itemID, seqID）
static func parse_grab_wear(b: PackedByteArray) -> Dictionary:
	return {
		"uid": u64(b, 0),
		"wltype": i32(b, 8),
		"item_id": u64(b, 12),
		"seq_id": u64(b, 20),
	}


## SM_REMOVEITEM -> SMRemoveItem（uid, itemID, seqID, count）
static func parse_remove_item(b: PackedByteArray) -> Dictionary:
	return {
		"uid": u64(b, 0),
		"item_id": u64(b, 8),
		"seq_id": u64(b, 16),
		"count": u64(b, 24),
	}


## SM_UPDATEITEM -> SMUpdateItem（uid, itemID, seqID, count）
static func parse_update_item(b: PackedByteArray) -> Dictionary:
	return {
		"uid": u64(b, 0),
		"item_id": u64(b, 8),
		"seq_id": u64(b, 16),
		"count": u64(b, 24),
	}


## SM_LEARNEDMAGICLIST -> SMLearnedMagicList（u64 uid + vector<u32> magicList）
static func parse_learned_magic(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var r := vec_len(b, 8)
	var n: int = int(r[0])
	var off: int = int(r[1])
	var list: Array = []
	for i in range(n):
		if off + 4 > b.size():
			break
		list.append(u32(b, off))
		off += 4
	return {"uid": uid, "magics": list}


## SM_BUFFIDLIST -> SDBuffIDList（u64 uid + vector<u64> idList）
static func parse_buff_id_list(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var r := vec_len(b, 8)
	var n: int = int(r[0])
	var off: int = int(r[1])
	var ids: Array = []
	for i in range(n):
		if off + 8 > b.size():
			break
		ids.append(u64(b, off))
		off += 8
	return {"uid": uid, "ids": ids}


## SM_VIEWRANGE -> SMViewRange（uint32 range）
static func parse_view_range(b: PackedByteArray) -> Dictionary:
	return {"range": u32(b, 0)}


## SM_QUESTDESPLIST -> SDQuestDespList（vector<SDQuestDesp>）
## SDQuestDesp { u64 questID; string xml; bool folded }
static func parse_quest_desp_list(b: PackedByteArray) -> Dictionary:
	var r := vec_len(b, 0)
	var n: int = int(r[0])
	var off: int = int(r[1])
	var quests: Array = []
	for i in range(n):
		if off + 8 > b.size():
			break
		var qid := u64(b, off)
		off += 8
		var sr: Array = rd_str(b, off)
		var xml: String = str(sr[0])
		off = int(sr[1])
		var folded := u8(b, off) != 0
		off += 1
		quests.append({"id": qid, "xml": xml, "folded": folded})
	return {"quests": quests}


## SM_GROUNDITEMIDLIST -> SDGroundItemIDList
##   uint64 mapUID; vector<GridItemIDList{ int x; int y; vector<u32> itemIDList }>
static func parse_ground_item_list(b: PackedByteArray) -> Dictionary:
	var map_uid := u64(b, 0)
	var r := vec_len(b, 8)
	var n: int = int(r[0])
	var off: int = int(r[1])
	var cells: Array = []
	for i in range(n):
		if off + 8 > b.size():
			break
		var gx := i32(b, off)
		var gy := i32(b, off + 4)
		off += 8
		var vr := vec_len(b, off)
		var cnt: int = int(vr[0])
		off = int(vr[1])
		var ids: Array = []
		for k in range(cnt):
			if off + 4 > b.size():
				break
			ids.append(u32(b, off))
			off += 4
		cells.append({"x": gx, "y": gy, "items": ids})
	return {"map_uid": map_uid, "cells": cells}


## SM_GROUNDFIREWALLLIST -> SDGroundFireWallList（mapUID + vector<{x,y,ownerUID}>）
static func parse_ground_firewall(b: PackedByteArray) -> Dictionary:
	var map_uid := u64(b, 0)
	var r := vec_len(b, 8)
	var n: int = int(r[0])
	var off: int = int(r[1])
	var cells: Array = []
	for i in range(n):
		if off + 24 > b.size():
			break
		cells.append({
			"x": i32(b, off), "y": i32(b, off + 4),
			"owner_uid": u64(b, off + 8),
		})
		off += 16
	return {"map_uid": map_uid, "cells": cells}


## SM_PLAYERSAY / SM_PLAYERBROADCAST -> { uid, content }
##   uint64 uid; char content[128]（定长，遇 0 截断）
static func parse_player_say(b: PackedByteArray) -> Dictionary:
	if b.size() < 8:
		return {}
	var uid := u64(b, 0)
	var n: int = mini(128, maxi(0, b.size() - 8))
	var raw := b.slice(8, 8 + n)
	var zero := raw.find(0)
	if zero >= 0:
		raw = raw.slice(0, zero)
	return {"uid": uid, "content": raw.get_string_from_utf8()}


## SM_STRIKEGRID -> { x, y }（u32 ×2）
static func parse_strike_grid(b: PackedByteArray) -> Dictionary:
	if b.size() < 8:
		return {}
	return {"x": u32(b, 0), "y": u32(b, 4)}


## SM_TEXT / SM_PARTEXT：body 就是裸字符串（无长度前缀）
static func parse_raw_text(b: PackedByteArray) -> String:
	return b.get_string_from_utf8()


## SM_NPCNAME -> { npc_uid, name }
static func parse_npc_name(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var sr: Array = rd_str(b, 8)
	return {"uid": uid, "name": str(sr[0])}


## SM_NPCXMLLAYOUT -> { npc_uid, xml }
static func parse_npc_xml(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var sr: Array = rd_str(b, 8)
	return {"uid": uid, "xml": str(sr[0])}


## 背包里的金币（SYS_GOLDNAME 对应的物品）总额
## 原版 getGold 就是从背包里累计金币物品
static func gold_from_inventory(items: Array) -> int:
	var total := 0
	for it in items:
		if it == null:
			continue
		if it.is_gold("金币"):
			total += it.count
	return total