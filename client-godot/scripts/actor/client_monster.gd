extends RefCounted
class_name ClientMonster
## 怪物基类逻辑
##
## 【逐行翻译】client/src/clientmonster.cpp + clientstandmonster.hpp
## 对照：
##   - clientmonster.cpp:618-634  getFrameGfxSeq()    默认帧表
##   - clientmonster.cpp:57-78    MonsterFrameGfxSeq::gfxID() 贴图键合成
##   - clientstandmonster.hpp:23-40  addActionTransf()  站立/趴下切换
##   - clientstandmonster.hpp:42-58  finalStandMode()   最终状态判定

const MonsterFramesRef := preload("res://scripts/actor/monster_frames.gd")
const MotionTypeRef := preload("res://scripts/action/motion_type.gd")
const ProtocolDefRef := preload("res://scripts/core/protocol_def.gd")
const SysConstRef8 := preload("res://scripts/core/sys_const.gd")

var uid: int = 0
## 怪物记录（来自 monsterrecord.json）
var mr: Dictionary = {}
## 对应 class_name，用于查自定义帧表（空串走默认表）
var gfx_class: String = ""
## 站/趴状态（对应 m_standMode）
var stand_mode: bool = false


func setup(uid_v: int, monster_record: Dictionary, gfx_class_v: String = "") -> void:
	uid = uid_v
	mr = monster_record
	gfx_class = gfx_class_v


func monster_id() -> int:
	return int(mr.get("_id", 0))


func monster_name() -> String:
	var n := str(mr.get("name", ""))
	# clientmonster.cpp:83 去掉尾部数字
	while not n.is_empty() and n[n.length() - 1] >= "0" and n[n.length() - 1] <= "9":
		n = n.substr(0, n.length() - 1)
	return n


func look_id() -> int:
	return int(mr.get("lookID", 0))


# ==================== 帧表 ====================

## getFrameGfxSeq（clientmonster.cpp:618）
## 返回 {count, gfxLookID, gfxMotionID, begin, reverse}
func get_frame_gfx_seq(motion: int) -> Dictionary:
	return MonsterFramesRef.frame_info(gfx_class, motion, stand_mode)


## getFrameCount（clientmonster.hpp:107）
func frame_count(motion: int) -> int:
	return int(get_frame_gfx_seq(motion).get("count", 0))


# ==================== 贴图键合成 ====================

## MonsterFrameGfxSeq::gfxID（clientmonster.cpp:57）
## 返回 {key, frame}，key 用于查纹理
##   公式：
##     gfxFrame        = begin + frame * (reverse ? -1 : 1)
##     lookGfxIndex    = (gfxLookID ?? mr.lookID) - LID_BEGIN
##     motionGfxIndex  = (gfxMotionID ?? currMotion.type) - MOTION_MON_BEGIN
##     dirGfxIndex     = (gfxDirectionID ?? currMotion.direction) - DIR_BEGIN
##   最后走 creatureKey 位编码
func gfx_id(motion: int, direction: int, frame: int, shadow: bool = false) -> Dictionary:
	var seq := get_frame_gfx_seq(motion)
	var count := int(seq.get("count", 0))
	if count <= 0:
		return {}
	if frame < 0 or frame >= count:
		return {}

	var begin := int(seq.get("begin", 0))
	var reverse := bool(seq.get("reverse", false))
	var gfx_frame := begin + frame * (-1 if reverse else 1)

	var look := int(seq.get("gfxLookID", look_id())) - ProtocolDefRef.LID_BEGIN
	var gfx_motion := int(seq.get("gfxMotionID", motion)) - MotionTypeRef.MOTION_MON_BEGIN
	var gfx_dir := int(seq.get("gfxDirectionID", direction)) - ProtocolDefRef.DIR_BEGIN

	return {
		"key": creature_key(look, gfx_motion, gfx_dir, shadow),
		"frame": gfx_frame,
	}


## PNGTexOffDB::creatureKey（pngtexoffdb.hpp:69）
##   key: direction 0-2, motion 3-6, layer 7, shadow 8, look 9-19
static func creature_key(look: int, motion: int, direction: int, shadow: bool = false) -> int:
	return ((direction & 0x7) << 0) \
		| ((motion & 0xF) << 3) \
		| ((1 if shadow else 0) << 8) \
		| ((look & 0x7FF) << 9)


# ==================== standMode（clientstandmonster.hpp） ====================

## addActionTransf（clientstandmonster.hpp:23）
## 生成一个 SPAWN 动作并挂触发器，播完翻转 standMode
## 返回 MotionNode
func make_action_transf(end_x: int, end_y: int, end_dir: int) -> MotionNode:
	var n := MotionNode.new()
	n.type = MotionTypeRef.MOTION_MON_SPAWN
	n.direction = end_dir
	n.x = end_x
	n.y = end_y
	# 触发器：动作播完时翻转状态（原版 addTrigger(true, ...)）
	n.add_trigger(true, func(_node) -> bool:
		stand_mode = not stand_mode
		return true
	)
	return n


## finalStandMode（clientstandmonster.hpp:42）
## 通用规则：用「末帧状态」作为当前状态。
## 若队列里有奇数个 SPAWN，说明还会翻一次，返回相反值。
func final_stand_mode(forced_queue: Array) -> bool:
	var count_transf := 0
	for m in forced_queue:
		if m != null and m.type == MotionTypeRef.MOTION_MON_SPAWN:
			count_transf += 1
	if count_transf % 2 == 1:
		return not stand_mode
	return stand_mode


## 是否站立状态（供外部查询）
func is_stand_mode() -> bool:
	return stand_mode
