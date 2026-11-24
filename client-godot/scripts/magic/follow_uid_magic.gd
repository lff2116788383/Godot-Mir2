extends "res://scripts/magic/base_magic.gd"
class_name FollowUIDMagic
## 追踪型魔法（火球术、雷电等）
##
## 【逐行翻译】client/src/followuidmagic.hpp（88 行）+ followuidmagic.cpp
##
## 原版注释（followuidmagic.cpp:1-20）说明了两个偏移的含义：
##
##   a = (m_x, m_y)          魔法逻辑位置
##   b = a + 帧偏移           实际绘制位置
##   c = a + targetOff()      魔法箭头/火球头，作为命中判定点
##   d = a + tail 偏移        魔法尾部
##
##        b--------+
##       /|     /  |
##      / |    /   |
##     /  |   /    |
##    /   |  L c   |
##   a    |        |
##        +--------+
##
##   d -> c 的向量要指向目标，所以帧要绕 c 旋转。
##   这些 head/tail 偏移不在原图库里，是存在魔法 APNG 文件名里的。

const SysConstRef2 := preload("res://scripts/core/sys_const.gd")
const PathFinderRef2 := preload("res://scripts/action/path_finder.gd")

## 旋转速度：360 度/秒
const ROTATE_DEGREE_SPEED := 360.0

var start_x: int = 0
var start_y: int = 0
var fly_dir_index: int = 0

var x: int = 0
var y: int = 0
var move_speed: int = 0

var uid: int = 0

## 上一次的距离平方，用于判断「飞越目标」
var _last_l_distance2: int = 0x7FFFFFFF
## 上一次飞行偏移
var _last_fly_off = null

## 当前旋转角度（顺时针，绕头部）
var rotate_degree: float = 0.0


## 对应 FollowUIDMagic 构造（followuidmagic.cpp:32）
## 返回是否成功
func setup_follow(magic_id_v: int, magic_record_v: Dictionary, gfx_entry_v: Dictionary,
		gfx_entry_ref_v, seff_id_v: int, dir_index: int, frame_count: int,
		px: int, py: int, fly_dir: int, speed: int, target_uid: int) -> bool:
	if not setup(magic_id_v, magic_record_v, gfx_entry_v, gfx_entry_ref_v, seff_id_v, dir_index, frame_count):
		return false
	if fly_dir < 0 or fly_dir >= 16:
		return false
	start_x = px
	start_y = py
	fly_dir_index = fly_dir
	x = px
	y = py
	move_speed = speed
	uid = target_uid
	return true


## targetOff（followuidmagic.cpp:57）
## 魔法头部相对 m_x/m_y 的偏移，由纹理分析得出（B9 补）
func target_off() -> Vector2i:
	return Vector2i(0, 0)


## targetPLoc（followuidmagic.hpp:73）
func target_p_loc() -> Vector2i:
	var off := target_off()
	return Vector2i(x + off.x, y + off.y)


## location（followuidmagic.hpp:84）
func location() -> Vector2i:
	return Vector2i(x / SysConstRef2.SYS_MAPGRIDXP, y / SysConstRef2.SYS_MAPGRIDYP)


## updateRotateDegree（followuidmagic.cpp:66）
## 转向走最短路径，每次最多转 ROTATE_DEGREE_SPEED * dt
func update_rotate_degree(aim_dx: int, aim_dy: int, dt: float, head: Vector2i = Vector2i.ZERO, tail: Vector2i = Vector2i.ZERO) -> void:
	if aim_dx == 0 and aim_dy == 0:
		return
	if not is_gfx_valid():
		return
	if head == tail:
		rotate_degree = 0.0
		return

	var target_atan := atan2(float(aim_dy), float(aim_dx))
	var self_atan := atan2(float(head.y - tail.y), float(head.x - tail.x))
	var target_degree := (target_atan - self_atan) * 180.0 / PI
	var diff_degree := _remainder(target_degree - rotate_degree, 360.0)
	var step_degree := ROTATE_DEGREE_SPEED * dt / 1000.0
	rotate_degree = fmod(rotate_degree + clampf(diff_degree, -step_degree, step_degree) + 360.0, 360.0)


## update（followuidmagic.cpp:94）
## 需要外部提供目标位置与上一帧位置，故用参数传入
##    target_p_loc_now : 目标当前头部位置（null 表示目标消失）
## 返回 true 表示完成
func update_follow(dt: float, target_p_loc_now) -> bool:
	var dx := 0
	var dy := 0
	var aim_dx := 0
	var aim_dy := 0

	if target_p_loc_now != null:
		var dst: Vector2i = target_p_loc_now
		var src := target_p_loc()
		var xdiff := dst.x - src.x
		var ydiff := dst.y - src.y

		_last_l_distance2 = _l_distance2(dst.x, dst.y, src.x, src.y)
		if xdiff == 0 and ydiff == 0:
			dx = 0; dy = 0; aim_dx = 0; aim_dy = 0
		else:
			var off := _dir_off(xdiff, ydiff, move_speed)
			dx = off.x; dy = off.y; aim_dx = xdiff; aim_dy = ydiff
	else:
		_last_l_distance2 = 0x7FFFFFFF
		if _last_fly_off != null:
			dx = _last_fly_off.x
			dy = _last_fly_off.y
		else:
			var off2 := _dir16_off(fly_dir_index, move_speed)
			dx = off2.x
			dy = off2.y
		aim_dx = dx
		aim_dy = dy

	if aim_dx != 0 or aim_dy != 0:
		update_rotate_degree(aim_dx, aim_dy, dt)

	x += dx
	y += dy
	_last_fly_off = Vector2i(dx, dy)
	return super.update(dt)


## done（followuidmagic.cpp:167）
## 两条结束条件：
##   1. 飞行格数超过 255
##   2. 头部接近目标（<24,<16）或已飞越（距离变大）
func done_follow(target_p_loc_now) -> bool:
	var start_gx := start_x / SysConstRef2.SYS_MAPGRIDXP
	var start_gy := start_y / SysConstRef2.SYS_MAPGRIDYP
	var curr_gx := x / SysConstRef2.SYS_MAPGRIDXP
	var curr_gy := y / SysConstRef2.SYS_MAPGRIDYP

	if _l_distance(start_gx, start_gy, curr_gx, curr_gy) > 255:
		return true

	if target_p_loc_now != null:
		var dst: Vector2i = target_p_loc_now
		var src := target_p_loc()
		var near := absi(src.x - dst.x) < 24 and absi(src.y - dst.y) < 16
		var flew_past := _l_distance2(src.x, src.y, dst.x, dst.y) > _last_l_distance2
		return near or flew_past
	return false


# ---------------- 数学工具（mathf.hpp） ----------------

## LDistance2：max^2 + min^2
static func _l_distance2(x0: int, y0: int, x1: int, y1: int) -> int:
	var dx := absi(x1 - x0)
	var dy := absi(y1 - y0)
	var hi := maxi(dx, dy)
	var lo := mini(dx, dy)
	return hi * hi + lo * lo


## LDistance：切比雪夫距离
static func _l_distance(x0: int, y0: int, x1: int, y1: int) -> int:
	return maxi(absi(x1 - x0), absi(y1 - y0))


## pathf::getDirOff（pathf.cpp）：16 方向偏移 × 距离
static func _dir_off(dx: int, dy: int, distance: int) -> Vector2i:
	var dir := _dir16_index(dx, dy)
	return _dir16_off(dir, distance)


## getDir16Off（pathf.cpp:167）
static func _dir16_off(dir_index: int, distance: int) -> Vector2i:
	const OFF := [
		[0.0, -1.0], [0.382683432365090, -0.923879532511287],
		[0.707106781186547, -0.707106781186547], [0.923879532511287, -0.382683432365090],
		[1.0, 0.0], [0.923879532511287, 0.382683432365090],
		[0.707106781186547, 0.707106781186547], [0.382683432365090, 0.923879532511287],
		[0.0, 1.0], [-0.382683432365090, 0.923879532511287],
		[-0.707106781186547, 0.707106781186548], [-0.923879532511287, 0.382683432365090],
		[-1.0, 0.0], [-0.923879532511287, -0.382683432365090],
		[-0.707106781186547, -0.707106781186547], [-0.382683432365090, -0.923879532511287],
	]
	var i := clampi(dir_index, 0, 15)
	return Vector2i(int(round(OFF[i][0] * distance)), int(round(OFF[i][1] * distance)))


## 由位移求 16 方向索引（pathf.cpp 的 findNearestPoint）
static func _dir16_index(dx: int, dy: int) -> int:
	if dx == 0 and dy == 0:
		return 0
	# 用角度就近匹配
	var ang := atan2(float(dy), float(dx))
	var deg := fmod(rad_to_deg(ang) + 360.0 + 11.25, 360.0)
	return int(deg / 22.5) % 16


## std::remainder 的等价实现
static func _remainder(a: float, b: float) -> float:
	if b == 0.0:
		return a
	return a - b * round(a / b)
