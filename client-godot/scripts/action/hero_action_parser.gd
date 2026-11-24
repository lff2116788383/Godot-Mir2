extends RefCounted
class_name HeroActionParser
## 动作解析：把服务端的 ActionNode 翻译成动作队列
##
## 【逐行翻译】client/src/hero.cpp:509-760 的 Hero::parseAction
##
## 这段代码的注释非常重要，原版解释了「为什么 rush 当前动作要排除 STAND」：
##   rush 是为了追位移，STAND 没有位移
##   rush STAND 或末帧会提前结束动作，导致下一动作的第 0 帧被截断
##   远程动作总是稍晚于 myhero 到达，同相位会造成每走一步都可见的跳动
##
## 本文件只翻译「动作 -> 动作队列」这一层，
## 魔法特效（HeroSpellMagicEffect）、延迟回调（addDelay）依赖 B4/B9，留空标记。

const MotionNodeRef := preload("res://scripts/action/motion_node.gd")
const MotionTypeRef := preload("res://scripts/action/motion_type.gd")
const ActionTypeRef := preload("res://scripts/action/action_type.gd")
const PathFinderRef := preload("res://scripts/action/path_finder.gd")
const MovableRef := preload("res://scripts/action/creature_movable.gd")
const SysConstRef := preload("res://scripts/core/sys_const.gd")


## 解析结果，对应原版填充 m_motionQueue / m_forcedMotionQueue
class ParseResult:
	var ok: bool = false
	## 普通动作队列
	var motion_queue: Array = []
	## 强制动作队列（优先播放）
	var forced_queue: Array = []
	## 需要外部处理的动作（如 SPACEMOVE 的传送特效）
	var pending: Dictionary = {}


## parseAction 的主体
## 参数对应 Hero 的运行时状态，用字典传入避免过长参数列表
##   curr_motion       : MotionNode        当前动作
##   forced_queue      : Array             当前强制队列
##   on_horse          : bool
##   can_move          : Callable(x,y)->bool
##   make_finder       : Callable(maxStep)->PathFinder
##   max_step          : int
##   is_my_hero        : bool
static func parse_action(action, ctx: Dictionary) -> ParseResult:
	var res := ParseResult.new()

	var curr: MotionNode = ctx.get("curr_motion")
	var forced: Array = ctx.get("forced_queue", [])
	var on_horse: bool = ctx.get("on_horse", false)
	var can_move: Callable = ctx.get("can_move", Callable())
	var make_finder: Callable = ctx.get("make_finder", Callable())
	var max_step: int = int(ctx.get("max_step", 2))

	if curr == null:
		return res

	# ---- hero.cpp:533-538 ----
	# rush 当前动作，但不 rush STAND / ONHORSESTAND / 末帧
	# 原因见上方注释：提前结束会截断下一动作的第 0 帧，造成可见跳动
	if curr.type != MotionTypeRef.MOTION_STAND \
			and curr.type != MotionTypeRef.MOTION_ONHORSESTAND \
			and curr.frame + 1 < curr.frame_count_ex():
		curr.speed = SysConstRef.SYS_MAXSPEED

	# ---- hero.cpp:541-543 ----
	# 队列的终点：优先取强制队列末尾，否则取当前动作终点
	var end_x: int = curr.end_x
	var end_y: int = curr.end_y
	var end_dir: int = curr.direction
	if not forced.is_empty():
		var last: MotionNode = forced[forced.size() - 1]
		end_x = last.end_x
		end_y = last.end_y
		end_dir = last.direction

	# ---- hero.cpp:547-575 ----
	# 1. 解析前先补齐位移
	res.motion_queue = []
	match action.type:
		ActionTypeRef.ACTION_DIE:
			# 视为紧急：服务端用 ReportStand() 做位置同步
			var walk := MovableRef.make_walk_motion_queue(
				end_x, end_y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
				on_horse, can_move, make_finder, max_step)
			res.forced_queue = forced.duplicate()
			res.forced_queue.append_array(walk)
			res.ok = true
			return res

		ActionTypeRef.ACTION_MOVE, \
		ActionTypeRef.ACTION_STAND, \
		ActionTypeRef.ACTION_SPELL, \
		ActionTypeRef.ACTION_ATTACK, \
		ActionTypeRef.ACTION_PICKUP, \
		ActionTypeRef.ACTION_SPINKICK:
			res.motion_queue = MovableRef.make_walk_motion_queue(
				end_x, end_y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
				on_horse, can_move, make_finder, max_step)

		ActionTypeRef.ACTION_SPACEMOVE:
			# 不做位移补齐，直接传送
			pass

		_:
			pass

	# ---- hero.cpp:579 起 ----
	# 2. 解析动作本体
	match action.type:
		ActionTypeRef.ACTION_STAND:
			var n := MotionNode.new()
			n.type = MotionTypeRef.MOTION_ONHORSESTAND if on_horse else MotionTypeRef.MOTION_STAND
			n.direction = action.direction
			n.x = action.x
			n.y = action.y
			res.motion_queue.append(n)
			res.ok = true

		ActionTypeRef.ACTION_SPINKICK:
			var n := MotionNode.new()
			n.type = MotionTypeRef.MOTION_SPINKICK
			# hero.cpp:603：若瞄准目标在 1 格内，朝向目标
			var dir := curr.direction
			if action.aim_uid != 0:
				var aim = ctx.get("find_uid", Callable()).call(action.aim_uid) \
					if ctx.get("find_uid", Callable()).is_valid() else null
				if aim != null and _cdistance(aim.grid, Vector2i(action.x, action.y)) == 1:
					dir = PathFinderRef.get_off_dir(aim.grid.x, aim.grid.y, action.x, action.y)
			n.direction = dir
			n.x = action.x
			n.y = action.y
			res.motion_queue.append(n)
			res.ok = true

		ActionTypeRef.ACTION_MOVE:
			var m := MovableRef.make_walk_motion(
				action.x, action.y, action.aim_x, action.aim_y,
				action.speed, on_horse, can_move)
			if m == null:
				res.ok = false
				return res
			res.motion_queue.append(m)
			res.ok = true

		ActionTypeRef.ACTION_MINE:
			# hero.cpp:632
			var stand_dir := _dir8(mine_x(action) - end_x, mine_y(action) - end_y) + PathFinderRef.DIR_BEGIN
			var n1 := MotionNode.new()
			n1.type = MotionTypeRef.MOTION_TWOVSWING
			n1.direction = stand_dir
			n1.x = end_x
			n1.y = end_y
			res.motion_queue.append(n1)

			var n2 := MotionNode.new()
			n2.type = MotionTypeRef.MOTION_ATTACKMODE
			n2.direction = stand_dir
			n2.x = end_x
			n2.y = end_y
			res.motion_queue.append(n2)

			# 原版在这里给最后一个动作挂 trigger：播放完后重新入队 MINE 动作
			# 客户端行为由调用方接续，这里记录意图
			res.pending["mine_requeue"] = true
			res.ok = true

		ActionTypeRef.ACTION_SPACEMOVE:
			# hero.cpp:661：传送
			# 原版：flushForcedMotion + jumpLoc + 固定位置魔法 + 附着魔法
			res.pending["space_move"] = Vector2i(action.aim_x, action.aim_y)
			res.ok = true

		ActionTypeRef.ACTION_SPELL:
			# hero.cpp:670 起：需要 magicrecord 数据（B4 完成）
			# 这里先产出占位结构，动作类型与朝向按原版规则计算
			var magic_id: int = action.ext_magic_id
			res.pending["spell"] = {
				"magic_id": magic_id,
				"action": action,
			}
			res.ok = true

		ActionTypeRef.ACTION_ATTACK:
			var n := MotionNode.new()
			n.type = _attack_motion(action)
			n.direction = action.direction
			n.x = action.x
			n.y = action.y
			n.ext_magic_id = action.ext_magic_id
			res.motion_queue.append(n)
			res.ok = true

		ActionTypeRef.ACTION_HITTED:
			var n := MotionNode.new()
			n.type = MotionTypeRef.MOTION_HITTED
			n.direction = action.direction
			n.x = action.x
			n.y = action.y
			n.ext_from_uid = action.aim_uid
			res.motion_queue.append(n)
			res.ok = true

		ActionTypeRef.ACTION_DIE:
			var n := MotionNode.new()
			n.type = MotionTypeRef.MOTION_ONHORSEDIE if on_horse else MotionTypeRef.MOTION_DIE
			n.direction = action.direction
			n.x = action.x
			n.y = action.y
			res.motion_queue.append(n)
			res.ok = true

		ActionTypeRef.ACTION_PICKUP:
			# 原版先补位移，再播 ATTACKMODE
			var n := MotionNode.new()
			n.type = MotionTypeRef.MOTION_ATTACKMODE
			n.direction = end_dir
			n.x = action.x
			n.y = action.y
			res.motion_queue.append(n)
			res.ok = true

		ActionTypeRef.ACTION_SPAWN:
			var n := MotionNode.new()
			n.type = MotionTypeRef.MOTION_STAND
			n.direction = action.direction
			n.x = action.x
			n.y = action.y
			res.motion_queue.append(n)
			res.ok = true

		_:
			res.ok = false

	return res


## 攻击动作类型：由武器类型决定（hero.cpp 的 attack motion 选择）
## 完整规则依赖武器 shape 与 weaponorder.inc，B9 补全
static func _attack_motion(action) -> int:
	if action.ext_magic_id != 0:
		return MotionTypeRef.MOTION_SPELL0
	return MotionTypeRef.MOTION_ONEVSWING


## getDir8（pathf.cpp）：由位移求 0-7 索引（不含 DIR_BEGIN）
static func _dir8(dx: int, dy: int) -> int:
	# 与 getFrontGLoc 的表对应：上=0 右上=1 ... 左上=7
	if dx == 0 and dy < 0: return 0
	if dx > 0 and dy < 0: return 1
	if dx > 0 and dy == 0: return 2
	if dx > 0 and dy > 0: return 3
	if dx == 0 and dy > 0: return 4
	if dx < 0 and dy > 0: return 5
	if dx < 0 and dy == 0: return 6
	if dx < 0 and dy < 0: return 7
	return 0


## CDistance（mathf.hpp）：切比雪夫距离
static func _cdistance(a: Vector2i, b: Vector2i) -> int:
	return maxi(absi(a.x - b.x), absi(a.y - b.y))


static func mine_x(action) -> int:
	return action.aim_x if action.aim_x != 0 else action.x


static func mine_y(action) -> int:
	return action.aim_y if action.aim_y != 0 else action.y
