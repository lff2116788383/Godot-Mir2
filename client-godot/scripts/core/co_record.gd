extends RefCounted
class_name CoRecord
## 进场记录解析
##
## 【逐行翻译】common/src/servermsg.hpp:204 SMCORecord
## 布局（实测 49 字节，见 tools/struct_sizeof.txt）：
##   [0]  UID       u64
##   [8]  mapUID    u64
##   [16] action    ActionNode (28B)
##   [44] union     5B（C++ 实测：Player 大小 = 5，Level 偏移 +1）
##          Mon: MonsterID u32 @44
##          Ply: 位域 1B @44 + Level u32 @45   <- 注意不是 48！
##          Npc: NPCID u32 @44

const ActionNodeRef := preload("res://scripts/action/action_node.gd")
const UidRef := preload("res://scripts/core/uid.gd")

var uid: int = 0
var map_uid: int = 0
var action = null            ## ActionNode
var monster_id: int = 0
var npc_id: int = 0
var gender: int = 0
var job: int = 0
var team_leader: bool = false
var stall: bool = false
var level: int = 0


## 解析 49 字节
static func parse(body: PackedByteArray) -> CoRecord:
	if body.size() < 49:
		push_error("SMCORecord 长度不足: %d" % body.size())
		return null
	var r := CoRecord.new()
	r.uid = body.decode_s64(0)
	r.map_uid = body.decode_s64(8)
	r.action = ActionNodeRef.from_bytes(body, 16)

	# 按 UID 类型解释 union 区（servermsg.hpp:211-231）
	match UidRef.get_uid_type(r.uid):
		UidRef.UID_MON:
			r.monster_id = body.decode_u32(44)
		UidRef.UID_PLY:
			# 位域 1 字节：gender:1 / job:3 / teamLeader:1 / stall:1
			var bits := body.decode_u8(44)
			r.gender = bits & 0x1
			r.job = (bits >> 1) & 0x7
			r.team_leader = ((bits >> 4) & 0x1) != 0
			r.stall = ((bits >> 5) & 0x1) != 0
			r.level = body.decode_u32(45)   # 实测偏移 45，非 48
		UidRef.UID_NPC:
			r.npc_id = body.decode_u32(44)
	return r


func to_str() -> String:
	return "CoRecord(uid=0x%X map=0x%X mon=%d npc=%d lv=%d)" % [
		uid, map_uid, monster_id, npc_id, level]
