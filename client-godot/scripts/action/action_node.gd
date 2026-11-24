extends RefCounted
class_name ActionNode
## 动作节点：28 字节定长结构 + 二进制编解码
##
## 【逐行翻译】common/src/actionnode.hpp:44-238
## 结构定义见 actionnode.hpp:45-235，实测 sizeof(ActionNode) = 28（tools/struct_sizeof.txt）
##
## 位域布局（actionnode.hpp:47-49，pack(1)）：
##   uint16 type:5      bits 0-4
##   uint16 speed:9     bits 5-13
##   uint16 direction:5 bits 14-15 -> 注意：5 bit 跨出 16 位，实际是拆到第二个 u16 的低 3 位
##
## 精确字节布局（小端）：
##   偏移 0-1 : [type:5][speed:9]                  => u16
##   偏移 2-3 : [direction:5]  => 实际占 1 字节，但 pack(1) 下位域延续
##   偏移 4-5 : x     (int16)
##   偏移 6-7 : y     (int16)
##   偏移 8-9 : aimX  (int16)
##   偏移 10-11: aimY (int16)
##   偏移 12-19: aimUID / fromUID (uint64)
##   偏移 20-23: extParam (4 字节 union)
##   偏移 24-27: 对齐填充（因 union 内 spell/attack 为 8 字节，
##              而 extParam 在 28 字节结构内的实际大小由最大成员决定）
##
## 说明：为避免位域跨字节歧义，这里按「先按 u16 解 type/speed，再按剩余位解 direction」
## 的方式实现，与 GCC/Clang 在 x86-64 + pack(1) 下的实际布局一致。

const SIZE := 28

# extParam 的联合体在哪个动作下用哪块
# 对应 actionnode.hpp:210-219
var type: int = 0
var speed: int = 0
var direction: int = 0
var x: int = 0
var y: int = 0
var aim_x: int = 0
var aim_y: int = 0
var aim_uid: int = 0       ## 与 from_uid 共用同一块内存

# extParam 各视图
var ext_magic_id: int = 0
var ext_modifier_id: int = 0
var ext_on_horse: int = 0
var ext_stand_mode: int = 0
var ext_stand_mode_req: int = 0

## fromUID 视图（仅 ACTION_HITTED）
var from_uid: int:
	get: return aim_uid
	set(v): aim_uid = v


static func from_bytes(buf: PackedByteArray, off: int = 0) -> ActionNode:
	var n := ActionNode.new()
	if buf.size() < off + SIZE:
		return n

	# 位域：低字节在前
	var w0 := buf.decode_u16(off)          # type:5 | speed:9
	n.type = w0 & 0x1F
	n.speed = (w0 >> 5) & 0x1FF

	# direction 占 5 位，跨 u16 边界。
	# 在 pack(1) 下 GCC 的实际布局：位域从 bit15 继续，
	# 即 direction 的低 1 位在 w0 的 bit15，高 4 位在下一字节的低 4 位。
	var bit15 := (w0 >> 15) & 0x1
	var next_byte := buf[off + 2]
	n.direction = bit15 | ((next_byte & 0x0F) << 1)

	n.x = buf.decode_s16(off + 4)
	n.y = buf.decode_s16(off + 6)
	n.aim_x = buf.decode_s16(off + 8)
	n.aim_y = buf.decode_s16(off + 10)
	n.aim_uid = buf.decode_u64(off + 12)

	# extParam 位于偏移 20，4 字节
	match n.type:
		ActionType.ACTION_SPELL, ActionType.ACTION_ATTACK:
			n.ext_magic_id = buf.decode_u32(off + 20)
			n.ext_modifier_id = buf.decode_u32(off + 24)
		ActionType.ACTION_MOVE:
			n.ext_on_horse = buf[off + 20]
		ActionType.ACTION_STAND:
			n.ext_stand_mode = buf[off + 20]
		ActionType.ACTION_TRANSF:
			n.ext_stand_mode_req = buf[off + 20]
		_:
			pass
	return n


func to_bytes() -> PackedByteArray:
	var buf := PackedByteArray()
	buf.resize(SIZE)
	buf.fill(0)

	var w0: int = (type & 0x1F) | ((speed & 0x1FF) << 5)
	# direction: 低 1 位进 w0 的 bit15，高 4 位进下一字节低 4 位
	w0 |= (direction & 0x01) << 15
	buf.encode_u16(0, w0)
	buf[2] = (direction >> 1) & 0x0F

	buf.encode_s16(4, x)
	buf.encode_s16(6, y)
	buf.encode_s16(8, aim_x)
	buf.encode_s16(10, aim_y)
	buf.encode_u64(12, aim_uid)

	match type:
		ActionType.ACTION_SPELL, ActionType.ACTION_ATTACK:
			buf.encode_u32(20, ext_magic_id)
			buf.encode_u32(24, ext_modifier_id)
		ActionType.ACTION_MOVE:
			buf[20] = ext_on_horse & 0xFF
		ActionType.ACTION_STAND:
			buf[20] = ext_stand_mode & 0xFF
		ActionType.ACTION_TRANSF:
			buf[20] = ext_stand_mode_req & 0xFF
		_:
			pass
	return buf


# ---------------- 构造器（actionnode.hpp:240-682） ----------------

## ActionSpawn（actionnode.hpp:240）
static func make_spawn(dir: int, px: int, py: int) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_SPAWN
	n.direction = dir
	n.x = px
	n.y = py
	return n


## ActionStand（actionnode.hpp:270）
static func make_stand(dir: int, px: int, py: int, stand_mode: int = 0) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_STAND
	n.direction = dir
	n.x = px
	n.y = py
	n.ext_stand_mode = stand_mode
	return n


## ActionMove（actionnode.hpp:303）
static func make_move(spd: int, px: int, py: int, ax: int, ay: int, on_horse: int = 0) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_MOVE
	n.speed = spd
	n.x = px
	n.y = py
	n.aim_x = ax
	n.aim_y = ay
	n.ext_on_horse = on_horse
	return n


## ActionJump（actionnode.hpp:342）
static func make_jump(dir: int, px: int, py: int) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_JUMP
	n.direction = dir
	n.x = px
	n.y = py
	return n


## ActionSpaceMove（actionnode.hpp:372）
static func make_space_move(dir: int, px: int, py: int, ax: int, ay: int) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_SPACEMOVE
	n.direction = dir
	n.x = px
	n.y = py
	n.aim_x = ax
	n.aim_y = ay
	return n


## ActionAttack（actionnode.hpp:408）
static func make_attack(spd: int, px: int, py: int, uid: int, magic_id: int = 0, modifier_id: int = 0) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_ATTACK
	n.speed = spd
	n.x = px
	n.y = py
	n.aim_uid = uid
	n.ext_magic_id = magic_id
	n.ext_modifier_id = modifier_id
	return n


## ActionPickUp（actionnode.hpp:444）
static func make_pickup(spd: int, px: int, py: int) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_PICKUP
	n.speed = spd
	n.x = px
	n.y = py
	return n


## ActionSpell（actionnode.hpp:474）
static func make_spell(spd: int, px: int, py: int, ax: int, ay: int, uid: int, magic_id: int, modifier_id: int) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_SPELL
	n.speed = spd
	n.x = px
	n.y = py
	n.aim_x = ax
	n.aim_y = ay
	n.aim_uid = uid
	n.ext_magic_id = magic_id
	n.ext_modifier_id = modifier_id
	return n


## ActionTransf（actionnode.hpp:516）
static func make_transf(dir: int, px: int, py: int, mode_req: int = 0) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_TRANSF
	n.direction = dir
	n.x = px
	n.y = py
	n.ext_stand_mode_req = mode_req
	return n


## ActionHitted（actionnode.hpp:549）
static func make_hitted(dir: int, px: int, py: int, from: int, stand_mode: int = 0) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_HITTED
	n.direction = dir
	n.x = px
	n.y = py
	n.aim_uid = from
	n.ext_stand_mode = stand_mode
	return n


## ActionSpinKick（actionnode.hpp:585）
static func make_spin_kick(spd: int, px: int, py: int, ax: int, ay: int, uid: int) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_SPINKICK
	n.speed = spd
	n.x = px
	n.y = py
	n.aim_x = ax
	n.aim_y = ay
	n.aim_uid = uid
	return n


## ActionDie（actionnode.hpp:624）
static func make_die(px: int, py: int, stand_mode: int = 0) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_DIE
	n.x = px
	n.y = py
	n.ext_stand_mode = stand_mode
	return n


## ActionMine（actionnode.hpp:654）
static func make_mine(spd: int, px: int, py: int) -> ActionNode:
	var n := ActionNode.new()
	n.type = ActionType.ACTION_MINE
	n.speed = spd
	n.x = px
	n.y = py
	return n


func to_string() -> String:
	return "ActionNode{t=%s spd=%d dir=%d (%d,%d)->(%d,%d) uid=%d}" % [
		ActionType.action_name(type), speed, direction, x, y, aim_x, aim_y, aim_uid
	]
