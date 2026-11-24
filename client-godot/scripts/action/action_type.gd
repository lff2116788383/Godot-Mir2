extends RefCounted
class_name ActionType
## 动作类型枚举
##
## 【逐行翻译】common/src/actionnode.hpp:8-27
## 说明：取值必须与 C++ 完全一致。

const ACTION_NONE := 0
const ACTION_BEGIN := 1
const ACTION_SPAWN := 1
const ACTION_STAND := 2
const ACTION_MOVE := 3
const ACTION_JUMP := 4
const ACTION_PUSHMOVE := 5
const ACTION_SPACEMOVE := 6
const ACTION_ATTACK := 7
const ACTION_PICKUP := 8
const ACTION_SPELL := 9
const ACTION_TRANSF := 10
const ACTION_HITTED := 11
const ACTION_SPINKICK := 12
const ACTION_DIE := 13
const ACTION_MINE := 14
const ACTION_END := 15


static func action_valid(t: int) -> bool:
	return t >= ACTION_BEGIN and t < ACTION_END


## actionName（actionnode.hpp:694）
static func action_name(t: int) -> String:
	match t:
		ACTION_NONE: return "ACTION_NONE"
		ACTION_SPAWN: return "ACTION_SPAWN"
		ACTION_STAND: return "ACTION_STAND"
		ACTION_MOVE: return "ACTION_MOVE"
		ACTION_JUMP: return "ACTION_JUMP"
		ACTION_PUSHMOVE: return "ACTION_PUSHMOVE"
		ACTION_SPACEMOVE: return "ACTION_SPACEMOVE"
		ACTION_ATTACK: return "ACTION_ATTACK"
		ACTION_PICKUP: return "ACTION_PICKUP"
		ACTION_SPELL: return "ACTION_SPELL"
		ACTION_TRANSF: return "ACTION_TRANSF"
		ACTION_HITTED: return "ACTION_HITTED"
		ACTION_SPINKICK: return "ACTION_SPINKICK"
		ACTION_DIE: return "ACTION_DIE"
		ACTION_MINE: return "ACTION_MINE"
	return "ACTION_UNKNOWN"
