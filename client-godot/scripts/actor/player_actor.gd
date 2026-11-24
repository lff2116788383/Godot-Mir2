extends Node2D
class_name PlayerActor
## 主角：逐格移动 + 8 方向朝向
##
## 移动模型（对照 client/src/myhero.cpp 的动作队列思路）：
##   点击 -> A* 求路径 -> 逐格推进 -> 到达终点
##
## 为什么不用 Tween：
##   初版用「每格一个 create_tween + await」实现，实测长路径（6 格以上）
##   会堆积并卡死（600 帧未走完 6 格）。改为 _process 内按时间累积推进，
##   路径长度对性能无影响。
##
## 移动速度：原版由 MotionNode 的帧数决定（SYS_DEFSPEED=100）。
## 这里先用固定 ms_per_grid，等 S4 动作状态机完成后改为按动画帧数推进。

signal grid_changed(grid: Vector2i)
signal move_finished
signal facing_changed(dir: int)

const MotionNodeScript := preload("res://scripts/action/motion_node.gd")
const MotionTypeScript := preload("res://scripts/action/motion_type.gd")

const GRID_X := 48
const GRID_Y := 32

@export var ms_per_grid: int = 240

var grid := Vector2i.ZERO
var facing: int = PathFinder.DIR_DOWN

var _path: Array[Vector2i] = []
var _moving := false
var _map_view: MapView = null
var _visual: Node2D = null
var _acc := 0.0
var _seg_from := Vector2.ZERO
var _seg_to := Vector2.ZERO

## 当前动作（MotionNode），由外部注入 HumanActor 用于播放动画
var _motion = null
## 外部动画节点（HumanActor），负责贴图刷新
var _anim_node = null
## 动作帧累积时间（毫秒）
var _frame_acc := 0.0

var grid_checker: Callable


## 绑定动画节点（HumanActor）
func bind_animation(node) -> void:
	_anim_node = node


## 切动作：type 用 MotionType，自动同步 direction / 坐标
func set_motion_type(motion_type: int) -> void:
	var m = MotionNodeScript.new()
	m.type = motion_type
	m.direction = facing
	m.x = grid.x
	m.y = grid.y
	m.end_x = grid.x
	m.end_y = grid.y
	_motion = m
	_frame_acc = 0.0
	if _anim_node != null:
		_anim_node.set_motion(m)


func setup(map_view: MapView, checker: Callable) -> void:
	_map_view = map_view
	grid_checker = checker


func _ready() -> void:
	_visual = Node2D.new()
	var body := Polygon2D.new()
	body.polygon = PackedVector2Array([
		Vector2(-7, 6), Vector2(7, 6), Vector2(7, -6), Vector2(-7, -6)
	])
	body.color = Color(0.95, 0.35, 0.25)
	_visual.add_child(body)

	var arrow := Polygon2D.new()
	arrow.polygon = PackedVector2Array([
		Vector2(0, -9), Vector2(4, -3), Vector2(-4, -3)
	])
	arrow.color = Color(1.0, 0.9, 0.3)
	_visual.add_child(arrow)
	_visual.set_meta("arrow", arrow)
	add_child(_visual)


## 隐藏占位方块（接入真实贴图后调用）
func hide_placeholder() -> void:
	if _visual != null:
		_visual.visible = false


func set_grid(g: Vector2i, dir: int = -1) -> void:
	grid = g
	if _map_view != null:
		position = _map_view.grid_to_world(g)
		_seg_from = position
		_seg_to = position
	if dir > 0:
		set_facing(dir)
	grid_changed.emit(g)


func set_facing(d: int) -> void:
	if d == facing or d < PathFinder.DIR_BEGIN or d >= PathFinder.DIR_END:
		return
	facing = d
	if _motion != null:
		_motion.direction = d
		if _anim_node != null:
			_anim_node.set_facing_dir(d)
	var i: int = d - PathFinder.DIR_BEGIN
	var off_x: int = PathFinder.DX[i]
	var off_y: int = PathFinder.DY[i]
	var arrow: Polygon2D = _visual.get_meta("arrow")
	if arrow != null:
		arrow.rotation = atan2(float(off_y), float(off_x)) + PI * 0.5
	facing_changed.emit(d)


func is_moving() -> bool:
	return _moving


## 请求走到目标格；返回是否成功排入路径
func request_move(dst: Vector2i, max_step: int = 1) -> bool:
	if _moving:
		return false
	if grid_checker.is_null():
		return false

	var finder := PathFinder.new(grid_checker, max_step)
	var path := finder.find_path(grid, dst, facing)
	if path.is_empty():
		return grid == dst

	_path = path
	_moving = true
	_acc = 0.0
	_seg_from = position
	set_motion_type(MotionTypeScript.MOTION_WALK)   # 走路动画
	_begin_segment()
	return true


func _begin_segment() -> void:
	if _path.is_empty():
		_end_move()
		return
	var nxt: Vector2i = _path[0]
	var d := PathFinder.get_off_dir(grid.x, grid.y, nxt.x, nxt.y)
	if d != PathFinder.DIR_NONE:
		set_facing(d)
	_seg_from = position
	_seg_to = _map_view.grid_to_world(nxt)


func _process(dt: float) -> void:
	# 动画帧无论是否在移动都要推进（站立也有 4 帧待机动画）
	_advance_animation(dt)

	if not _moving:
		return
	var dur := ms_per_grid / 1000.0
	if dur <= 0.0:
		dur = 0.001
	_acc += dt

	# 用 while 消化累积时间：一帧内可跨越整格（掉帧时保证不卡）
	var guard := 0
	while _acc >= dur and _moving and guard < 16:
		guard += 1
		_acc -= dur
		# 落格
		if _path.is_empty():
			_end_move()
			return
		var nxt: Vector2i = _path.pop_front()
		grid = nxt
		position = _seg_to
		grid_changed.emit(grid)
		if _path.is_empty():
			_end_move()
			return
		_begin_segment()

	if _moving:
		var t := clampf(_acc / dur, 0.0, 1.0)
		position = _seg_from.lerp(_seg_to, t)


## 按原版 frameDelay 推进动画帧
func _advance_animation(dt: float) -> void:
	if _motion == null or _anim_node == null:
		return
	var delay: float = _motion.frame_delay()
	_frame_acc += dt * 1000.0
	while _frame_acc >= delay:
		_frame_acc -= delay
		if not _anim_node.advance_frame():
			break


func _end_move() -> void:
	_moving = false
	_acc = 0.0
	set_motion_type(MotionTypeScript.MOTION_STAND)  # 回到站立
	move_finished.emit()


func stop() -> void:
	_path.clear()
	_end_move()
