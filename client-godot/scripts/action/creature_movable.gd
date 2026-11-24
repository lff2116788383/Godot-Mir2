extends RefCounted
class_name CreatureMovable
## 可移动生物：动作队列生成与推进
##
## 【逐行翻译】client/src/creaturemovable.cpp + hero.cpp 的移动部分
## 对照：
##   - creaturemovable.cpp:11-90   parseMovePath()      路径解析
##   - creaturemovable.cpp:120-156 makeWalkMotionQueue() 移动队列
##   - hero.cpp:1352-1412          makeWalkMotion()     单个移动动作
##   - clientcreature.cpp:149      updateMotion()       帧推进
##
## 关键公式 mathf::LDistance2（mathf.hpp）：
##   LDistance2 = max(|dx|, |dy|)^2 + min(|dx|, |dy|)^2
##   用于区分走/跑/骑马：1,2=走  4,8=跑  9,18=骑跑

const PathFinderRef := preload("res://scripts/action/path_finder.gd")
const MotionNodeRef := preload("res://scripts/action/motion_node.gd")
const SysConstRef := preload("res://scripts/core/sys_const.gd")

## 方向查表（hero.cpp:1362）
const DIR_V := [
	[PathFinderRef.DIR_UPLEFT, PathFinderRef.DIR_UP, PathFinderRef.DIR_UPRIGHT],
	[PathFinderRef.DIR_LEFT, PathFinderRef.DIR_NONE, PathFinderRef.DIR_RIGHT],
	[PathFinderRef.DIR_DOWNLEFT, PathFinderRef.DIR_DOWN, PathFinderRef.DIR_DOWNRIGHT],
]


## LDistance2（mathf.hpp）
## 返回 max^2 + min^2，用于判定移动类型
static func l_distance2(x0: int, y0: int, x1: int, y1: int) -> int:
	var dx := absi(x1 - x0)
	var dy := absi(y1 - y0)
	var hi := maxi(dx, dy)
	var lo := mini(dx, dy)
	return hi * hi + lo * lo


## parseMovePath（creaturemovable.cpp:11）
## 返回路径点数组；空数组表示出错，单点表示原地
## 调用方需提供 can_move 与 path_finder 回调
static func parse_move_path(x0: int, y0: int, x1: int, y1: int,
		can_move: Callable, make_finder: Callable, max_step: int,
		check_ground: bool = true) -> Array:
	var result: Array = []

	if not bool(can_move.call(x0, y0)):
		return result

	var ld2 := l_distance2(x0, y0, x1, y1)
	match ld2:
		0:
			return [Vector2i(x0, y0)]
		1, 2:
			# dst 在一跳距离内，中间不可能有阻挡
			if check_ground:
				if bool(can_move.call(x1, y1)):
					return [Vector2i(x0, y0), Vector2i(x1, y1)]
				return [Vector2i(x0, y0)]
			return [Vector2i(x0, y0), Vector2i(x1, y1)]

	# 其他距离：走 A*
	if make_finder.is_null():
		return result
	var finder = make_finder.call(max_step)
	if finder == null:
		return result
	var path: Array = finder.find_path(Vector2i(x0, y0), Vector2i(x1, y1))
	if path.is_empty():
		return [Vector2i(x0, y0)]

	# 原版 PathNode 含起点，这里补上
	var nodes: Array = [Vector2i(x0, y0)]
	nodes.append_array(path)
	return nodes


## makeWalkMotion（hero.cpp:1352）
## 返回 MotionNode 或 null
static func make_walk_motion(x0: int, y0: int, x1: int, y1: int,
		speed: int, on_horse: bool, can_move: Callable) -> MotionNode:
	if can_move.is_null():
		return null
	if not bool(can_move.call(x0, y0)):
		return null
	if not bool(can_move.call(x1, y1)):
		return null
	if speed < SysConstRef.SYS_MINSPEED or speed > SysConstRef.SYS_MAXSPEED:
		return null

	# hero.cpp:1367
	var sdx: int = 1 + (1 if x1 > x0 else 0) - (1 if x1 < x0 else 0)
	var sdy: int = 1 + (1 if y1 > y0 else 0) - (1 if y1 < y0 else 0)

	var motion_type := 0
	match l_distance2(x0, y0, x1, y1):
		1, 2:
			motion_type = MotionType.MOTION_ONHORSEWALK if on_horse else MotionType.MOTION_WALK
		4, 8:
			motion_type = MotionType.MOTION_RUN
		9, 18:
			motion_type = MotionType.MOTION_ONHORSERUN
		_:
			return null

	var n := MotionNode.new()
	n.type = motion_type
	n.direction = int(DIR_V[sdy][sdx])
	n.speed = speed
	n.x = x0
	n.y = y0
	n.end_x = x1
	n.end_y = y1
	return n


## makeWalkMotionQueue（creaturemovable.cpp:120）
## 返回 MotionNode 数组（每个元素走一格）
static func make_walk_motion_queue(sx: int, sy: int, ex: int, ey: int,
		speed: int, on_horse: bool, can_move: Callable,
		make_finder: Callable, max_step: int) -> Array:
	if l_distance2(sx, sy, ex, ey) == 0:
		return []

	var path := parse_move_path(sx, sy, ex, ey, can_move, make_finder, max_step, true)
	if path.size() <= 1:
		# 0 = 出错；1 = 找不到路径（因为 LDistance2 != 0）
		return []

	var queue: Array = []
	for i in range(1, path.size()):
		var p0: Vector2i = path[i - 1]
		var p1: Vector2i = path[i]
		var m := make_walk_motion(p0.x, p0.y, p1.x, p1.y, speed, on_horse, can_move)
		if m == null:
			return []    # 原版 throw，这里返回空表示失败
		queue.append(m)
	return queue


## motionQueueValid（creaturemovable.cpp:158）
## 校验队列首尾是否衔接
static func motion_queue_valid(curr: MotionNode, queue: Array) -> bool:
	if queue.is_empty():
		return true
	var last: MotionNode = curr
	for m in queue:
		if m != null and last.end_x == m.x and last.end_y == m.y:
			last = m
		else:
			return false
	return true


## 动作完成度（creaturemovable.cpp:348）
## doneRatio: (frame + 1) / frameCount，最大 1.0
static func done_ratio(n: MotionNode) -> float:
	var fc := n.frame_count_ex()
	if fc <= 0:
		return 1.0
	return minf(float(n.frame + 1) / float(fc), 1.0)


## 怪物专用 makeWalkMotion（clientmonster.cpp:640-672）
## 与玩家版的差别：
##   1. 动作类型固定 MOTION_MON_WALK（怪物没有 RUN）
##   2. 判定 LDistance2 ∈ {1, 2, maxStep², 2×maxStep²}
static func make_monster_walk_motion(x0: int, y0: int, x1: int, y1: int,
		speed: int, can_move: Callable, max_step: int = 1) -> MotionNode:
	if can_move.is_null():
		return null
	if not bool(can_move.call(x0, y0)):
		return null
	if not bool(can_move.call(x1, y1)):
		return null
	if speed < SysConstRef.SYS_MINSPEED or speed > SysConstRef.SYS_MAXSPEED:
		return null

	var sdx: int = 1 + (1 if x1 > x0 else 0) - (1 if x1 < x0 else 0)
	var sdy: int = 1 + (1 if y1 > y0 else 0) - (1 if y1 < y0 else 0)
	var ld2 := l_distance2(x0, y0, x1, y1)
	if ld2 == 1 or ld2 == 2 or ld2 == max_step * max_step or ld2 == 2 * max_step * max_step:
		var n := MotionNode.new()
		n.type = MotionType.MOTION_MON_WALK
		n.direction = int(DIR_V[sdy][sdx])
		n.speed = speed
		n.x = x0
		n.y = y0
		n.end_x = x1
		n.end_y = y1
		return n
	return null


## 怪物专用 makeWalkMotionQueue
static func make_monster_walk_queue(sx: int, sy: int, ex: int, ey: int,
		speed: int, can_move: Callable, make_finder: Callable, max_step: int = 1) -> Array:
	if l_distance2(sx, sy, ex, ey) == 0:
		return []
	var path := parse_move_path(sx, sy, ex, ey, can_move, make_finder, max_step, true)
	if path.size() <= 1:
		return []
	var queue: Array = []
	for i in range(1, path.size()):
		var p0: Vector2i = path[i - 1]
		var p1: Vector2i = path[i]
		var m := make_monster_walk_motion(p0.x, p0.y, p1.x, p1.y, speed, can_move, max_step)
		if m == null:
			return []
		queue.append(m)
	return queue


## 渲染像素偏移（creaturemovable.cpp:216 getShift）
##   仅 MOTION_WALK/RUN/ONHORSE* /MOTION_MON_WALK 有位移
##   其余动作返回 (0,0)
##
##   stepCount = currStep()  —— 怪物只有 MOTION_MON_WALK 返回 1，其他 0
##   shiftDX = GRID_X * stepCount / frameCount
##   framePos = frame + 1（stepped：第 k 帧显示 k+1 的偏移，末帧正好到位）
static func get_shift(motion, frame: int, is_monster: bool) -> Vector2i:
	if motion == null:
		return Vector2i.ZERO
	var t: int = motion.type
	var walk_like := t == MotionType.MOTION_WALK or t == MotionType.MOTION_RUN \
		or t == MotionType.MOTION_ONHORSEWALK or t == MotionType.MOTION_ONHORSERUN \
		or t == MotionType.MOTION_MON_WALK
	if not walk_like:
		return Vector2i.ZERO

	# currStep：怪物 MOTION_MON_WALK = 1；玩家走 1 步 = 1、跑 2 步
	var step := 0
	if is_monster:
		step = 1 if t == MotionType.MOTION_MON_WALK else 0
	else:
		match t:
			MotionType.MOTION_WALK, MotionType.MOTION_ONHORSEWALK:
				step = 1
			MotionType.MOTION_RUN, MotionType.MOTION_ONHORSERUN:
				step = 2
	if step <= 0:
		return Vector2i.ZERO

	var fc: int = motion.frame_count_ex()
	if fc <= 0:
		return Vector2i.ZERO

	var dx: float = float(SysConstRef.SYS_MAPGRIDXP * step) / float(fc)
	var dy: float = float(SysConstRef.SYS_MAPGRIDYP * step) / float(fc)
	var frame_pos := float(frame + 1)
	var sx := int(round(dx * frame_pos))
	var sy := int(round(dy * frame_pos))

	match motion.direction:
		PathFinderRef.DIR_UP:        return Vector2i(0, -sy)
		PathFinderRef.DIR_UPRIGHT:   return Vector2i(sx, -sy)
		PathFinderRef.DIR_RIGHT:     return Vector2i(sx, 0)
		PathFinderRef.DIR_DOWNRIGHT: return Vector2i(sx, sy)
		PathFinderRef.DIR_DOWN:      return Vector2i(0, sy)
		PathFinderRef.DIR_DOWNLEFT:  return Vector2i(-sx, sy)
		PathFinderRef.DIR_LEFT:      return Vector2i(-sx, 0)
		PathFinderRef.DIR_UPLEFT:    return Vector2i(-sx, -sy)
	return Vector2i.ZERO
