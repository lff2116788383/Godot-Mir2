extends RefCounted
class_name ActorFactory
## 实体表现节点工厂
##
## 【依据】client/src/processrunnet.cpp:309-343（按 UID 类型分派创建）
##   UID_MON -> ClientMonster::create(uid, this, action)
##   UID_PLY -> new Hero(uid, gender, job, this, action)
##   UID_NPC -> new ClientNPC(...)

const MonsterActorRef := preload("res://scripts/actor/monster_actor.gd")
const HumanViewRef := preload("res://scripts/actor/human_view.gd")


## 创建怪物表现节点
static func make_monster(assets_root: String, uid: int, monster_id: int) -> Node2D:
	var a := MonsterActorRef.new()
	if not a.setup(assets_root, uid, monster_id):
		a.free()
		return null
	a.name = "Mon_%X" % uid
	return a


## 创建其他玩家表现节点
static func make_player_view(assets_root: String, uid: int,
		gender: bool = false, job: int = 0) -> Node2D:
	var v := HumanViewRef.new()
	v.name = "Ply_%X" % uid
	# 外观待 SM_QUERYPLAYERWLDESP 更新，先用默认
	v.setup(assets_root, 0, 1, 0, 0, gender)
	return v


## 创建 NPC 表现节点
static func make_npc(assets_root: String, uid: int, npc_id: int) -> Node2D:
	var v := HumanViewRef.new()
	v.name = "Npc_%X" % uid
	# NPC 外观来自 NPCID，待 NPC 记录表接入
	v.setup(assets_root, 0, 1, 0, 0, false)
	return v
