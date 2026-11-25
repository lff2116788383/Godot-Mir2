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
## SM_UPDATEITEM -> SMUpdateItem（uid, itemID, seqID, count）
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
## SM_QUESTDESPLIST -> SDQuestDespList（vector<SDQuestDesp>）
## SDQuestDesp { u64 questID; string xml; bool folded }
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
static func parse_view_range(b: PackedByteArray) -> Dictionary:
	return {"range": u8(b, 0)}


static func parse_npc_xml(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var pr: Array = rd_str(b, 8)
	var xr: Array = rd_str(b, int(pr[1]))
	return {"uid": uid, "event_path": str(pr[0]), "xml": str(xr[0])}


static func parse_ground_firewall(b: PackedByteArray) -> Dictionary:
	var map_uid := u64(b, 0)
	var r := vec_len(b, 8)
	var n: int = int(r[0]); var off: int = int(r[1])
	var cells: Array = []
	for i in range(n):
		if off + 12 > b.size(): break
		cells.append({"x": i32(b, off), "y": i32(b, off + 4),
			"count": i32(b, off + 8)})
		off += 12
	return {"map_uid": map_uid, "cells": cells}


static func parse_update_item(b: PackedByteArray) -> Dictionary:
	var got: Array = SDItemRef.from_bytes(b, 0)
	var it = got[0]
	return {"item": it, "item_id": it.item_id, "seq_id": it.seq_id,
		"count": it.count, "duration": it.duration}


static func parse_remove_item(b: PackedByteArray) -> Dictionary:
	return {"item_id": u32(b, 0), "seq_id": u32(b, 4), "count": u16(b, 8)}


static func parse_remove_secured_item(b: PackedByteArray) -> Dictionary:
	return {"item_id": u32(b, 0), "seq_id": u32(b, 4)}


static func parse_equip_belt(b: PackedByteArray) -> Dictionary:
	var slot := i32(b, 0)
	var got: Array = SDItemRef.from_bytes(b, 4)
	return {"slot": slot, "item": got[0]}


static func parse_grab_belt(b: PackedByteArray) -> Dictionary:
	var slot := i32(b, 0)
	var got: Array = SDItemRef.from_bytes(b, 4)
	return {"slot": slot, "item": got[0]}


# ==================== P0/P1 扩展解析（按 C++ cereal 结构直译） ====================

static func parse_online_ok(b: PackedByteArray) -> Dictionary:
	if b.size() < 149:
		return {}
	var name_r := rd_str(b, 8)
	var meta_off := 8 + 132
	var bits := u8(b, meta_off)
	var a := SDItemRef.new() # keep preload alive; no-op
	var act = null
	var apath := "res://scripts/action/action_node.gd"
	var script = load(apath)
	if script != null and b.size() >= meta_off + 1 + 8 + 28:
		act = script.from_bytes(b, meta_off + 1 + 8)
	return {
		"uid": u64(b, 0),
		"name": str(name_r[0]),
		"gender": bits & 0x1,
		"job": (bits >> 1) & 0x7,
		"map_uid": u64(b, meta_off + 1),
		"action": act,
	}


static func parse_start_game_scene(b: PackedByteArray) -> Dictionary:
	if b.size() < 28:
		return {}
	var out := {
		"uid": u64(b, 0), "map_uid": u64(b, 8),
		"x": i32(b, 16), "y": i32(b, 20), "direction": i32(b, 24),
	}
	var r := _read_wear(b, 28)
	out["wear"] = r[0]
	var off: int = int(r[1])
	out["hair"] = u32(b, off)
	out["hair_color"] = u32(b, off + 4)
	off += 8
	var nr := rd_str(b, off); off = int(nr[1])
	out["name"] = str(nr[0])
	out["name_color"] = u32(b, off); off += 4
	out["view_range"] = i32(b, off)
	return out


static func parse_buff(b: PackedByteArray) -> Dictionary:
	if b.size() < 16: return {}
	return {"uid": u64(b, 0), "type": u32(b, 8), "state": u32(b, 12)}


static func parse_cast_magic(b: PackedByteArray) -> Dictionary:
	if b.size() < 36: return {}
	return {
		"uid": u64(b, 0), "map_uid": u64(b, 8),
		"magic": u8(b, 16), "magic_param": u8(b, 17),
		"speed": u8(b, 18), "direction": u8(b, 19),
		"x": u16(b, 20), "y": u16(b, 22),
		"aim_x": u16(b, 24), "aim_y": u16(b, 26),
		"aim_uid": u64(b, 28),
	}


static func parse_safe_zone_list(b: PackedByteArray) -> Dictionary:
	var map_uid := u64(b, 0)
	var off := 8
	var nrect := u64(b, off); off += 8
	var rects: Array = []
	for i in range(nrect):
		if off + 16 > b.size(): break
		rects.append({"x": i32(b, off), "y": i32(b, off + 4),
			"w": i32(b, off + 8), "h": i32(b, off + 12)})
		off += 16
	var ncircle := u64(b, off); off += 8
	var circles: Array = []
	for i in range(ncircle):
		if off + 12 > b.size(): break
		circles.append({"x": i32(b, off), "y": i32(b, off + 4),
			"r": i32(b, off + 8)})
		off += 12
	return {"map_uid": map_uid, "rects": rects, "circles": circles}


static func parse_npc_sell(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var r := vec_len(b, 8)
	var n: int = int(r[0]); var off: int = int(r[1])
	var ids: Array = []
	for i in range(n):
		if off + 4 > b.size(): break
		ids.append(u32(b, off)); off += 4
	return {"npc_uid": uid, "items": ids}


static func parse_sell_item_list(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var r := vec_len(b, 8)
	var n: int = int(r[0]); var off: int = int(r[1])
	var items: Array = []
	for i in range(n):
		var got: Array = SDItemRef.from_bytes(b, off)
		var it = got[0]; off = int(got[1])
		var cr := vec_len(b, off)
		var cn: int = int(cr[0]); off = int(cr[1])
		var costs: Array = []
		for k in range(cn):
			if off + 12 > b.size(): break
			costs.append({"item_id": u32(b, off), "count": u64(b, off + 8)})
			off += 16
		items.append({"item": it, "item_id": it.item_id,
			"count": it.count, "seq_id": it.seq_id, "costs": costs})
	return {"npc_uid": uid, "items": items}


static func parse_show_secured_item_list(b: PackedByteArray) -> Dictionary:
	var r := vec_len(b, 0)
	var n: int = int(r[0]); var off: int = int(r[1])
	var items: Array = []
	for i in range(n):
		var got: Array = SDItemRef.from_bytes(b, off)
		items.append(got[0]); off = int(got[1])
	return {"items": items}


static func parse_quest_desp_list(b: PackedByteArray) -> Dictionary:
	# SDQuestDespList = unordered_map<string, unordered_map<string,string>>
	var n := u64(b, 0)
	var off := 8
	var quests: Array = []
	for i in range(n):
		var qr := rd_str(b, off); off = int(qr[1])
		var qid := str(qr[0])
		var subn := u64(b, off); off += 8
		var parts: Array = []
		for k in range(subn):
			var fr := rd_str(b, off); off = int(fr[1])
			var vr := rd_str(b, off); off = int(vr[1])
			parts.append({"fsm": str(fr[0]), "xml": str(vr[0])})
		var xml := ""
		for p in parts:
			if xml != "": xml += "\n"
			xml += str(p["xml"])
		quests.append({"id": qid, "title": qid, "xml": xml,
			"folded": false, "parts": parts})
	return {"quests": quests}


static func parse_quest_desp_update(b: PackedByteArray) -> Dictionary:
	var nr := rd_str(b, 0)
	var name := str(nr[0]); var off := int(nr[1])
	var fr := rd_str(b, off)
	var fsm := str(fr[0]); off = int(fr[1])
	var has := u8(b, off) != 0; off += 1
	var text := ""
	if has:
		var vr := rd_str(b, off)
		text = str(vr[0]); off = int(vr[1])
	return {"name": name, "fsm": fsm, "desp": text if has else ""}


static func parse_team_member_list(b: PackedByteArray) -> Dictionary:
	var leader := u64(b, 0)
	var r := vec_len(b, 8)
	var n: int = int(r[0]); var off: int = int(r[1])
	var members: Array = []
	for i in range(n):
		var tr := _read_team_player(b, off)
		members.append(tr[0]); off = int(tr[1])
	var req := _read_team_req(b, off)
	return {"leader": leader, "members": members, "req": req[0]}


static func parse_team_info(b: PackedByteArray) -> Dictionary:
	var leader := u64(b, 0)
	var nr := rd_str(b, 8); var off := int(nr[1])
	var out := {"leader": leader, "name": str(nr[0]),
		"job": i32(b, off), "level": i32(b, off + 4),
		"member_count": i32(b, off + 8)}
	var req := _read_team_req(b, off + 12)
	out["req"] = req[0]
	return out


static func parse_team_candidate(b: PackedByteArray) -> Dictionary:
	var r := _read_team_player(b, 0)
	var p: Dictionary = r[0]
	var off: int = int(r[1])
	p["job"] = i32(b, off)
	p["stat"] = i32(b, off + 4)
	var mr := vec_len(b, off + 8)
	p["magic_count"] = int(mr[0])
	return p


static func parse_friend_list(b: PackedByteArray) -> Dictionary:
	var r := vec_len(b, 0)
	var n: int = int(r[0]); var off: int = int(r[1])
	var friends: Array = []
	for i in range(n):
		var pr := _read_chat_peer(b, off)
		friends.append(pr[0]); off = int(pr[1])
	return {"friends": friends}


static func parse_chat_peer(b: PackedByteArray) -> Dictionary:
	var r := _read_chat_peer(b, 0)
	return r[0] if r[0] is Dictionary else {}


static func parse_guild_info(b: PackedByteArray) -> Dictionary:
	var gid := u32(b, 0)
	var nr := rd_str(b, 4); var name_ := str(nr[0]); var off := int(nr[1])
	var rr := rd_str(b, off); var notice := str(rr[0]); off = int(rr[1])
	var vr := vec_len(b, off)
	var n: int = int(vr[0]); off = int(vr[1])
	var members: Array = []
	for i in range(n):
		var dbid := u32(b, off)
		var mr := rd_str(b, off + 4); off = int(mr[1])
		var rank := i32(b, off); off += 4
		var tr := rd_str(b, off); off = int(tr[1])
		var online := u8(b, off) != 0; off += 1
		members.append({"dbid": dbid, "name": str(mr[0]),
			"rank": rank, "title": str(tr[0]), "online": online})
	return {"guild_id": gid, "name": name_, "notice": notice, "members": members}


static func parse_player_guild(b: PackedByteArray) -> Dictionary:
	var out := {"dbid": u32(b, 0), "guild_id": u32(b, 4)}
	var nr := rd_str(b, 8); var off := int(nr[1])
	var tr := rd_str(b, off)
	out["guild_name"] = str(nr[0]); out["title"] = str(tr[0])
	return out


static func parse_guild_war_list(b: PackedByteArray) -> Dictionary:
	var gid := u32(b, 0)
	var r := vec_len(b, 4)
	var n: int = int(r[0]); var off: int = int(r[1])
	var wars: Array = []
	for i in range(n):
		var id := u32(b, off); off += 4
		var nr := rd_str(b, off); off = int(nr[1])
		wars.append({"guild_id": id, "guild_name": str(nr[0]),
			"end_time": u64(b, off)})
		off += 8
	return {"guild_id": gid, "wars": wars}


static func parse_player_name(b: PackedByteArray) -> Dictionary:
	var out := {"uid": u64(b, 0)}
	var nr := rd_str(b, 8); out["name"] = str(nr[0])
	out["name_color"] = u32(b, int(nr[1]))
	return out


static func parse_player_wl_desp(b: PackedByteArray) -> Dictionary:
	var out := {"uid": u64(b, 0)}
	var r := _read_wear(b, 8)
	out["wear"] = r[0]
	var off: int = int(r[1])
	out["hair"] = u32(b, off); out["hair_color"] = u32(b, off + 4)
	return out


static func parse_stall(b: PackedByteArray) -> Dictionary:
	var out := {"dbid": u32(b, 0), "selling": u8(b, 4) != 0}
	var nr := rd_str(b, 5); out["name"] = str(nr[0]); var off := int(nr[1])
	var r := vec_len(b, off)
	var n: int = int(r[0]); off = int(r[1])
	var items: Array = []
	for i in range(n):
		var got: Array = SDItemRef.from_bytes(b, off)
		var it = got[0]; off = int(got[1])
		var price := u64(b, off); off += 8
		items.append({"item": it, "item_id": it.item_id, "seq_id": it.seq_id,
			"count": it.count, "price": price})
	out["items"] = items
	return out


static func parse_auction_item_list(b: PackedByteArray) -> Dictionary:
	var category := i32(b, 0)
	var r := vec_len(b, 4)
	var n: int = int(r[0]); var off: int = int(r[1])
	var entries: Array = []
	for i in range(n):
		var auction_id := u64(b, off); off += 8
		var pr := _read_chat_peer(b, off)
		var seller = pr[0]; off = int(pr[1])
		var nr := rd_str(b, off); off = int(nr[1])
		var time_left := u64(b, off); off += 8
		var price := u64(b, off); off += 8
		var got: Array = SDItemRef.from_bytes(b, off)
		var it = got[0]; off = int(got[1])
		entries.append({"auction_id": auction_id, "seller": seller,
			"seller_name": str(seller.get("name", "")),
			"note": str(nr[0]), "time_left": time_left,
			"price": price, "item": it, "item_id": it.item_id,
			"count": it.count, "seq_id": it.seq_id})
	return {"category": category, "entries": entries}


static func parse_ranking_list(b: PackedByteArray) -> Dictionary:
	var typ := u8(b, 0)
	var r := vec_len(b, 1)
	var n: int = int(r[0]); var off: int = int(r[1])
	var entries: Array = []
	for i in range(n):
		var dbid := u32(b, off)
		var level := u32(b, off + 4)
		var gold := u64(b, off + 8)
		var nr := rd_str(b, off + 16); off = int(nr[1])
		entries.append({"dbid": dbid, "level": level, "gold": gold, "name": str(nr[0])})
	return {"type": typ, "entries": entries}


static func parse_direct_trade_offer(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var gold := u64(b, 8)
	var locked := u8(b, 16) != 0
	var confirmed := u8(b, 17) != 0
	var r := vec_len(b, 18)
	var n: int = int(r[0]); var off: int = int(r[1])
	var items: Array = []
	for i in range(n):
		var got: Array = SDItemRef.from_bytes(b, off)
		items.append(got[0]); off = int(got[1])
	return {"uid": uid, "gold": gold, "locked": locked,
		"confirmed": confirmed, "items": items}


static func parse_direct_trade_request(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var nr := rd_str(b, 8)
	return {"uid": uid, "name": str(nr[0])}


static func parse_player_config(b: PackedByteArray) -> Dictionary:
	var off := 0
	var mn := u64(b, off); off += 8
	var magic_keys := {}
	for i in range(mn):
		if off + 5 > b.size(): break
		var mid := u32(b, off); off += 4
		magic_keys[mid] = b[off]; off += 1
	var rn := u64(b, off); off += 8
	var runtime := {}
	for i in range(rn):
		var kr := rd_str(b, off); off = int(kr[1])
		var vr := rd_str(b, off); off = int(vr[1])
		runtime[str(kr[0])] = str(vr[0])
	return {"magic_keys": magic_keys, "runtime": runtime}


static func parse_start_input(b: PackedByteArray) -> Dictionary:
	var uid := u64(b, 0)
	var tr := rd_str(b, 8)
	var cr := rd_str(b, int(tr[1]))
	var security := u8(b, int(cr[1])) != 0
	return {"uid": uid, "title": str(tr[0]), "commit_tag": str(cr[0]), "security": security}


static func parse_start_invop(b: PackedByteArray) -> Dictionary:
	var invop := i32(b, 0)
	var uid := u64(b, 4)
	var qr := rd_str(b, 12)
	var cr := rd_str(b, int(qr[1]))
	var vr := vec_len(b, int(cr[1]))
	return {"inv_op": invop, "uid": uid, "query_tag": str(qr[0]),
		"commit_tag": str(cr[0]), "type_count": int(vr[0])}


static func parse_chat_peer_list(b: PackedByteArray) -> Dictionary:
	var r := vec_len(b, 0)
	var n: int = int(r[0]); var off: int = int(r[1])
	var peers: Array = []
	for i in range(n):
		var pr := _read_chat_peer(b, off)
		peers.append(pr[0]); off = int(pr[1])
	return {"peers": peers}


static func parse_chat_message(b: PackedByteArray) -> Dictionary:
	var r := _read_chat_message(b, 0)
	return r[0]


static func parse_chat_message_list(b: PackedByteArray) -> Dictionary:
	var r := vec_len(b, 0)
	var n: int = int(r[0]); var off: int = int(r[1])
	var messages: Array = []
	for i in range(n):
		var mr := _read_chat_message(b, off)
		messages.append(mr[0]); off = int(mr[1])
	return {"messages": messages}


static func parse_chat_message_seq(b: PackedByteArray) -> Dictionary:
	return {"id": u64(b,0), "timestamp": u64(b,8)}


# ---------------- 内部结构读取 ----------------

static func _read_chat_message(b: PackedByteArray, off: int) -> Array:
	var out := {"seq_id": 0, "timestamp": 0, "refer_id": 0,
		"from": 0, "to": 0, "message": ""}
	var has_seq := u8(b, off) != 0; off += 1
	if has_seq:
		out["seq_id"] = u64(b, off)
		out["timestamp"] = u64(b, off + 8)
		off += 16
	var has_ref := u8(b, off) != 0; off += 1
	if has_ref:
		out["refer_id"] = u64(b, off); off += 8
	out["from"] = u64(b, off); off += 8
	out["to"] = u64(b, off); off += 8
	var sr := rd_str(b, off); off = int(sr[1])
	out["message"] = str(sr[0])
	return [out, off]


static func _read_item_vec(b: PackedByteArray, off: int) -> Array:
	var r := vec_len(b, off)
	var n: int = int(r[0]); var p: int = int(r[1])
	var out: Array = []
	for i in range(n):
		var got: Array = SDItemRef.from_bytes(b, p)
		out.append(got[0]); p = int(got[1])
	return [out, p]


static func _read_wear(b: PackedByteArray, off: int) -> Array:
	var n := u64(b, off); off += 8
	var wear := {}
	for i in range(n):
		if off + 4 > b.size(): break
		var slot := i32(b, off); off += 4
		var got: Array = SDItemRef.from_bytes(b, off)
		wear[slot] = got[0]; off = int(got[1])
	return [wear, off]


static func _read_chat_peer(b: PackedByteArray, off: int) -> Array:
	var out := {"id": u32(b, off), "name": "", "avatar": 0, "kind": 0}
	off += 4
	var nr := rd_str(b, off); out["name"] = str(nr[0]); off = int(nr[1])
	if u8(b, off) != 0:
		out["avatar"] = u64(b, off + 1)
		off += 9
	else:
		off += 1
	if off + 8 <= b.size():
		var variant := u64(b, off); off += 8
		out["kind"] = variant
		if variant == 1 and off + 5 <= b.size():
			out["gender"] = u8(b, off) != 0
			out["job"] = i32(b, off + 1)
			off += 5
		elif variant == 2 and off + 12 <= b.size():
			out["creator"] = u32(b, off)
			out["create_time"] = u64(b, off + 4)
			off += 12
			var mr := vec_len(b, off)
			var mn: int = int(mr[0]); off = int(mr[1])
			var members: Array = []
			for i in range(mn):
				if off + 6 > b.size(): break
				members.append({"dbid": u32(b, off), "priority": u16(b, off + 4)})
				off += 6
			out["members"] = members
	return [out, off]


static func _read_team_player(b: PackedByteArray, off: int) -> Array:
	var uid := u64(b, off)
	var level := u32(b, off + 8)
	var job := i32(b, off + 12)
	var nr := rd_str(b, off + 16)
	return [{"uid": uid, "level": level, "job": job, "name": str(nr[0])}, int(nr[1])]


static func _read_team_req(b: PackedByteArray, off: int) -> Array:
	if off + 5 > b.size(): return [{}, off]
	var req := {"max_member": i32(b, off), "auto_accept": u8(b, off + 4) != 0}
	off += 5
	for i in range(3):
		if off >= b.size(): break
		var has := u8(b, off) != 0; off += 1
		if not has: continue
		if off + 12 > b.size(): break
		var minl := i32(b, off); var maxl := i32(b, off + 4)
		var cap := i32(b, off + 8); off += 12
		var sr := vec_len(b, off)
		var sn: int = int(sr[0]); off = int(sr[1])
		var magics: Array = []
		for k in range(sn):
			if off + 4 > b.size(): break
			magics.append(u32(b, off)); off += 4
		req["job_%d" % i] = {"min_level": minl, "max_level": maxl,
			"min_capacity": cap, "magics": magics}
	return [req, off]

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
