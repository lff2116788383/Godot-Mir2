extends RefCounted
class_name Uid
## UID 位布局与构造规则
##
## 【逐行翻译】common/src/uidf.hpp + uidf.cpp:1-30
##
## UID 是 uint64_t，但只用 63 位（最高位留空，供 Lua 使用）：
##
##      62 - 59 : type field, supports 16 types
##      58 - 35 : id (24 bit)
##      34 - 31 : peerIndex (4 bit)
##      30 -  0 : seq
##
## 说明：原版用 constexpr 算位偏移，这里直接给出等价的常量与函数。

# ---------------- 类型（uidf.hpp:31） ----------------
const UID_NONE := 0
const UID_BEGIN := 1
const UID_COR := 1
const UID_MAP := 2
const UID_NPC := 3
const UID_MON := 4
const UID_PLY := 5
const UID_RCV := 6
const UID_QST := 7
const UID_SLO := 8
const UID_END := 9

# ---------------- 位宽（uidf.cpp:8-16） ----------------
const RESERVED_BIT_WIDTH := 1
const UID_TYPE_BIT_WIDTH := 4
const ID_BIT_WIDTH := 24
const PEER_INDEX_BIT_WIDTH := 4

const UID_TYPE_BIT_OFF := 64 - RESERVED_BIT_WIDTH - UID_TYPE_BIT_WIDTH          ## 59
const ID_BIT_OFF := UID_TYPE_BIT_OFF - ID_BIT_WIDTH                              ## 35
const PEER_INDEX_BIT_OFF := ID_BIT_OFF - PEER_INDEX_BIT_WIDTH                    ## 31
const SEQ_BIT_OFF := 0

const UID_TYPE_BIT_MASK := 0xF
const ID_BIT_MASK := 0xFFFFFF
const PEER_INDEX_BIT_MASK := 0xF


# ---------------- 读取 ----------------

## uidf::getUIDType（uidf.cpp:48）
static func get_uid_type(uid: int) -> int:
	return (uid >> UID_TYPE_BIT_OFF) & UID_TYPE_BIT_MASK


## uidf::peerIndex（uidf.cpp:32）
static func peer_index(uid: int) -> int:
	match get_uid_type(uid):
		UID_COR:
			return get_peer_core_seq(uid)
		UID_MAP, UID_NPC, UID_MON:
			return (uid >> PEER_INDEX_BIT_OFF) & PEER_INDEX_BIT_MASK
	return 0


## uidf::getPlayerDBID（uidf.hpp:75）
static func get_player_dbid(uid: int) -> int:
	return (uid >> ID_BIT_OFF) & ID_BIT_MASK


## uidf::getMapID
static func get_map_id(uid: int) -> int:
	return (uid >> ID_BIT_OFF) & ID_BIT_MASK


## uidf::getNPCID
static func get_npc_id(uid: int) -> int:
	return (uid >> ID_BIT_OFF) & ID_BIT_MASK


## uidf::getQuestID
static func get_quest_id(uid: int) -> int:
	return (uid >> ID_BIT_OFF) & ID_BIT_MASK


## uidf::getMonsterID
static func get_monster_id(uid: int) -> int:
	return (uid >> ID_BIT_OFF) & ID_BIT_MASK


## seq 部分（低 31 位）
static func get_seq(uid: int) -> int:
	return uid & 0x7FFFFFFF


## uidf::getMapSeq
static func get_map_seq(uid: int, first: bool) -> int:
	return get_seq(uid)


## uidf::getNPCSeq
static func get_npc_seq(uid: int, first: bool) -> int:
	return get_seq(uid)


## uidf::getMonsterSeq
static func get_monster_seq(uid: int, first: bool) -> int:
	return get_seq(uid)


## uidf::getPeerCoreSeq
static func get_peer_core_seq(uid: int) -> int:
	return get_seq(uid)


## uidf::getReceiverSeq
static func get_receiver_seq(uid: int) -> int:
	return get_seq(uid)


# ---------------- 构造 ----------------

static func _build(uid_type: int, id: int, peer_index_v: int, seq: int) -> int:
	return ((uid_type & UID_TYPE_BIT_MASK) << UID_TYPE_BIT_OFF) \
		| ((id & ID_BIT_MASK) << ID_BIT_OFF) \
		| ((peer_index_v & PEER_INDEX_BIT_MASK) << PEER_INDEX_BIT_OFF) \
		| (seq & 0x7FFFFFFF)


## uidf::buildMapUID（uidf.hpp:58）
static func build_map_uid(map_id: int, peer_index_v: int) -> int:
	return _build(UID_MAP, map_id, peer_index_v, 0)


## uidf::buildNPCUID
static func build_npc_uid(npc_id: int, peer_index_v: int) -> int:
	return _build(UID_NPC, npc_id, peer_index_v, 0)


## uidf::buildMonsterUID
## 注意：原版还有 seq 参数用于区分同种怪物的不同实例
static func build_monster_uid(monster_id: int, peer_index_v: int, seq: int = 0) -> int:
	return _build(UID_MON, monster_id, peer_index_v, seq)


## uidf::getPlayerUID（uidf.hpp:69）
static func get_player_uid(dbid: int) -> int:
	return _build(UID_PLY, dbid, 0, 0)


## uidf::getQuestUID
static func get_quest_uid(quest_id: int) -> int:
	return _build(UID_QST, quest_id, 0, 0)


## uidf::getReceiverUID
static func build_receiver_uid() -> int:
	return _build(UID_RCV, 0, 0, 0)


## uidf::getServiceCoreUID
static func get_service_core_uid() -> int:
	return _build(UID_COR, 0, 0, 0)


## uidf::getServerLuaObjectUID
static func get_server_lua_object_uid(v: int) -> int:
	return _build(UID_SLO, v, 0, 0)


# ---------------- 判定（uidf.hpp:92-113） ----------------

static func valid_uid(uid: int) -> bool:
	return (uid >> 63) == 0 and uid != 0


static func is_player(uid: int) -> bool:
	return get_uid_type(uid) == UID_PLY


static func is_quest(uid: int) -> bool:
	return get_uid_type(uid) == UID_QST


static func is_np_char(uid: int) -> bool:
	return get_uid_type(uid) == UID_NPC


static func is_receiver(uid: int) -> bool:
	return get_uid_type(uid) == UID_RCV


static func is_service_core(uid: int) -> bool:
	return get_uid_type(uid) == UID_COR and peer_index(uid) == 0


static func is_peer_core(uid: int) -> bool:
	return get_uid_type(uid) == UID_COR


static func is_map(uid: int) -> bool:
	return get_uid_type(uid) == UID_MAP


static func is_monster(uid: int) -> bool:
	return get_uid_type(uid) == UID_MON


## uidf::getUIDString（uidf.cpp:52）
static func get_uid_string(uid: int) -> String:
	if uid == 0:
		return "ZERO"
	match get_uid_type(uid):
		UID_PLY: return "PLY_%d" % get_player_dbid(uid)
		UID_MON: return "MON_%d_%d_%d" % [get_monster_id(uid), peer_index(uid), get_monster_seq(uid, false)]
		UID_NPC: return "NPC_%d_%d_%d" % [get_npc_id(uid), peer_index(uid), get_npc_seq(uid, false)]
		UID_MAP: return "MAP_%d_%d_%d" % [get_map_id(uid), peer_index(uid), get_map_seq(uid, false)]
		UID_COR: return "CORE_%d" % peer_index(uid)
		UID_RCV: return "RCV_%d" % get_receiver_seq(uid)
		UID_QST: return "QST_%d" % get_quest_id(uid)
	return "UNKNOWN_%d" % uid


## uidf::getUIDTypeCStr
static func get_uid_type_string(uid: int) -> String:
	match get_uid_type(uid):
		UID_COR: return "COR"
		UID_MAP: return "MAP"
		UID_NPC: return "NPC"
		UID_MON: return "MON"
		UID_PLY: return "PLY"
		UID_RCV: return "RCV"
		UID_QST: return "QST"
		UID_SLO: return "SLO"
	return "NONE"
