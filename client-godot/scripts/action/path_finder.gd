extends RefCounted
class_name PathFinder
## 8 方向双向 A* 寻路
##
## 【本文件是 common/src/pathf.hpp + pathf.cpp 的逐行翻译】
## 对照实现：
##   - pathf.hpp:163-173  方向工具函数
##   - pathf.hpp:191-300  AStarPathFinder 内部结构（InnNode / InnPQNode / InnPQ）
##   - pathf.hpp:390-470  公共接口
##   - pathf.cpp:212-278  expand_f()  正向扩展
##   - pathf.cpp:280-380  expand_r()  反向扩展
##   - pathf.cpp:382-450  search()    主循环与停止条件
##   - pathf.cpp:452-495  getPathNode() 回溯路径
##
## 翻译原则：
##   1. 函数名、变量名、控制流与原版一一对应，便于逐行核对
##   2. 不"优化"、不"简化"，原版怎么做就怎么做
##   3. 原版 std::priority_queue 换成数组 + 取最小（语义等价，小顶堆）
##   4. 原版 InnNode 位域打包成 int64 作键，这里用 "x,y,dir" 字符串键

# ---------------- 常量（protocoldef.hpp） ----------------

const DIR_NONE := 0
const DIR_BEGIN := 1
const DIR_UP := 1
const DIR_UPRIGHT := 2
const DIR_RIGHT := 3
const DIR_DOWNRIGHT := 4
const DIR_DOWN := 5
const DIR_DOWNLEFT := 6
const DIR_LEFT := 7
const DIR_UPLEFT := 8
const DIR_END := 9

const PF_NONE := 0
const PF_OBSTACLE := 1
const PF_OCCUPIED := 2
const PF_LOCKED := 3
const PF_FREE := 4

## getFrontGLoc 偏移表（pathf.hpp:123）
const DX := [0, 1, 1, 1, 0, -1, -1, -1]
const DY := [-1, -1, 0, 1, 1, 1, 0, -1]


# ---------------- 方向工具（pathf.hpp:163-173） ----------------

static func dir_valid(d: int) -> bool:
	return d >= DIR_BEGIN and d < DIR_END


## pathf.hpp:94 getNextDir
static func get_next_dir(d: int, diff: int = 1) -> int:
	var dir_count := DIR_END - DIR_BEGIN
	return DIR_BEGIN + ((((d - DIR_BEGIN + diff) % dir_count) + dir_count) % dir_count)


## pathf.hpp:104 getDirDiff（顺时针）
static func get_dir_diff(src: int, dst: int) -> int:
	var dir_count := DIR_END - DIR_BEGIN
	return (((dst - src) % dir_count) + dir_count) % dir_count


## pathf.hpp:112 getDirAbsDiff（取最小）
static func get_dir_abs_diff(src: int, dst: int) -> int:
	var d := get_dir_diff(src, dst)
	return mini(DIR_END - DIR_BEGIN - d, d)


## pathf.hpp:79 getBackDir
static func get_back_dir(d: int) -> int:
	match d:
		DIR_UP: return DIR_DOWN
		DIR_DOWN: return DIR_UP
		DIR_LEFT: return DIR_RIGHT
		DIR_RIGHT: return DIR_LEFT
		DIR_UPLEFT: return DIR_DOWNRIGHT
		DIR_UPRIGHT: return DIR_DOWNLEFT
		DIR_DOWNLEFT: return DIR_UPRIGHT
		DIR_DOWNRIGHT: return DIR_UPLEFT
	return DIR_NONE


## pathf.hpp:123 getFrontGLoc
static func get_front_g_loc(x: int, y: int, d: int, length: int = 1) -> Vector2i:
	var i: int = d - DIR_BEGIN
	return Vector2i(x + length * int(DX[i]), y + length * int(DY[i]))


## pathf.hpp:128 getBackGLoc
static func get_back_g_loc(x: int, y: int, d: int, length: int = 1) -> Vector2i:
	return get_front_g_loc(x, y, d, -length)


## pathf.hpp:143 getOffDir
static func get_off_dir(src_x: int, src_y: int, dst_x: int, dst_y: int) -> int:
	var dx := signi(dst_x - src_x)
	var dy := signi(dst_y - src_y)
	if dx == 0 and dy == 0:
		return DIR_NONE
	if dy == -1:
		return DIR_UPLEFT if dx == -1 else (DIR_UP if dx == 0 else DIR_UPRIGHT)
	if dy == 0:
		return DIR_LEFT if dx == -1 else DIR_RIGHT
	return DIR_DOWNLEFT if dx == -1 else (DIR_DOWN if dx == 0 else DIR_DOWNRIGHT)


## 方向偏移序列 [-n/2, n/2]，对应 pathf.cpp 的 getDirDiffList()
static func get_dir_diff_list(n: int) -> Array:
	var result := []
	var half := n / 2
	for i in range(-half, half + 1):
		result.append(i)
	return result


# ---------------- 节点（pathf.hpp:216-260） ----------------

class InnNode:
	var x: int
	var y: int
	var dir: int

	func _init(px: int = 0, py: int = 0, pdir: int = 0) -> void:
		x = px
		y = py
		dir = pdir

	func eq(px: int, py: int) -> bool:
		return x == px and y == py

	func eq_dir(px: int, py: int, pdir: int) -> bool:
		return eq(px, py) and dir == pdir

	func key() -> String:
		return "%d,%d,%d" % [x, y, dir]

	func dup() -> InnNode:
		return InnNode.new(x, y, dir)


class InnPQNode:
	var node: InnNode
	var cost: float

	func _init(pnode: InnNode = null, pcost: float = 0.0) -> void:
		node = pnode
		cost = pcost


## 原版基于 std::priority_queue（小顶堆）；这里用数组 + 线性取最小
class InnPQ:
	var c: Array = []

	func empty() -> bool:
		return c.is_empty()

	## 最小元素的索引（维护小顶堆语义）
	func _min_index() -> int:
		var best := 0
		for i in range(1, c.size()):
			if c[i].cost < c[best].cost:
				best = i
		return best

	## top() 必须返回最小值，原版是 std::priority_queue::top()
	func top() -> InnPQNode:
		if c.is_empty():
			return null
		return c[_min_index()]

	func push(n: InnPQNode) -> void:
		c.append(n)

	## pick() = top() + pop()，原版实现
	func pick() -> InnPQNode:
		if c.is_empty():
			return null
		var best := _min_index()
		var t: InnPQNode = c[best]
		c.remove_at(best)
		return t

	func pop() -> void:
		if c.is_empty():
			return
		c.remove_at(_min_index())

	func size() -> int:
		return c.size()

	func clear() -> void:
		c.clear()


# ---------------- AStarPathFinder（pathf.hpp:191） ----------------

var _src_node: InnNode
var _dst_x: int = 0
var _dst_y: int = 0

var _check_turn: int = 1
var _max_step: int = 1
var _one_step_cost: Callable
var _grid_checker: Callable
var _search_count: int = 0

var _cost_f: Dictionary = {}
var _cost_r: Dictionary = {}
var _parent_f: Dictionary = {}
var _parent_r: Dictionary = {}
var _pq_f: InnPQ
var _pq_r: InnPQ
var _done_cost = null
var _done_node = null
## pathf.hpp:399-400 势函数基准值
var _pi_f_s: float = 0.0
var _pi_r_t: float = 0.0


func _init(grid_checker: Callable, max_step: int = 1, check_turn: int = 1, one_step_cost: Callable = Callable(), search_count: int = 0) -> void:
	_grid_checker = grid_checker
	_max_step = clampi(max_step, 1, 3)
	_check_turn = check_turn
	_one_step_cost = one_step_cost
	_search_count = search_count
	_pq_f = InnPQ.new()
	_pq_r = InnPQ.new()


## pathf.hpp search()
func search(src_x: int, src_y: int, src_dir: int, dst_x: int, dst_y: int) -> Dictionary:
	_cost_f.clear()
	_cost_r.clear()
	_parent_f.clear()
	_parent_r.clear()
	_pq_f.clear()
	_pq_r.clear()
	_done_cost = null
	_done_node = null

	_src_node = InnNode.new(src_x, src_y, src_dir)
	_dst_x = dst_x
	_dst_y = dst_y

	# pathf.cpp:382-383
	_pi_f_s = _pi(_src_node.x, _src_node.y, _dst_x, _dst_y)
	_pi_r_t = _pi(_dst_x, _dst_y, _src_node.x, _src_node.y)

	# pathf.cpp:394 正向起点
	for d in get_dir_diff_list(8 if _check_turn == 1 else 1):
		var first := InnNode.new(_src_node.x, _src_node.y, get_next_dir(_src_node.dir, int(d)))
		_cost_f[first.key()] = 0.0
		_pq_f.push(InnPQNode.new(first, 0.0))

	# pathf.cpp:410 反向起点
	for d in get_dir_diff_list(1 if _check_turn == 0 else 8):
		var last := InnNode.new(_dst_x, _dst_y, get_next_dir(_src_node.dir, int(d)))
		_cost_r[last.key()] = 0.0
		_pq_r.push(InnPQNode.new(last, 0.0))

	var c := 0
	while (_search_count <= 0 or c < _search_count) and not _pq_f.empty() and not _pq_r.empty():
		c += 1
		_expand_f()
		if _pq_f.empty():
			return {"has_path": _has_path(), "done_search": true}
		if _check_stop():
			return {"has_path": _has_path(), "done_search": true}

		_expand_r()
		if _pq_r.empty():
			return {"has_path": _has_path(), "done_search": true}
		if _check_stop():
			return {"has_path": _has_path(), "done_search": true}

	return {"has_path": _has_path(), "done_search": _pq_f.empty() or _pq_r.empty()}


func _has_path() -> bool:
	return _done_cost != null


## pathf.cpp:426 fnCheckStop
func _check_stop() -> bool:
	if _done_cost == null:
		return false
	if _pq_f.empty() or _pq_r.empty():
		return false
	return _pq_f.top().cost + _pq_r.top().cost >= float(_done_cost)


## pathf.cpp:212 expand_f
func _expand_f() -> void:
	var curr_node: InnPQNode = _pq_f.pick()
	if curr_node == null:
		return
	var curr: InnNode = curr_node.node

	var prev_node = null
	if curr.eq(_src_node.x, _src_node.y):
		prev_node = null
	elif _parent_f.has(curr.key()):
		prev_node = _parent_f[curr.key()]
	else:
		return

	while not _pq_f.empty():
		var t := _pq_f.top()
		if t != null and _cost_f.has(t.node.key()) and float(_cost_f[t.node.key()]) < t.cost:
			_pq_f.pop()
		else:
			break

	for step_size in range(1, _max_step + 1):
		for d in get_dir_diff_list(8):
			var next_dir: int = get_next_dir(curr.dir, int(d))
			var next_loc := get_front_g_loc(curr.x, curr.y, next_dir, step_size)
			var next_node := InnNode.new(next_loc.x, next_loc.y, DIR_BEGIN if _check_turn == 0 else next_dir)

			if prev_node != null and prev_node.eq_dir(next_node.x, next_node.y, next_node.dir):
				continue

			var src_dir: int = next_dir if _check_turn == 0 else curr.dir
			var hop_cost = _call_one_step_cost(curr.x, curr.y, src_dir, next_node.x, next_node.y)
			if hop_cost == null:
				continue

			# pathf.cpp:254 reducedCost = hopCost - pf(curr) + pf(next)
			var reduced_cost: float = float(hop_cost) - _pf(curr_node.node.x, curr_node.node.y) + _pf(next_node.x, next_node.y)
			if reduced_cost < 0.0:
				reduced_cost = 0.0
			var new_cost: float = curr_node.cost + reduced_cost
			var k := next_node.key()
			if not _cost_f.has(k) or new_cost < float(_cost_f[k]):
				_cost_f[k] = new_cost
				_parent_f[k] = curr.dup()
				_pq_f.push(InnPQNode.new(next_node, new_cost))

			if _cost_r.has(k):
				_update_done_cost(next_node, new_cost + float(_cost_r[k]))


## pathf.cpp:280 expand_r
func _expand_r() -> void:
	var curr_node: InnPQNode = _pq_r.pick()
	if curr_node == null:
		return
	var curr: InnNode = curr_node.node

	var prev_node = null
	if curr.eq(_dst_x, _dst_y):
		prev_node = null
	elif _parent_r.has(curr.key()):
		prev_node = _parent_r[curr.key()]
	else:
		return

	while not _pq_r.empty():
		var t := _pq_r.top()
		if t != null and _cost_r.has(t.node.key()) and float(_cost_r[t.node.key()]) < t.cost:
			_pq_r.pop()
		else:
			break

	for step_size in range(1, _max_step + 1):
		for d_from in get_dir_diff_list(8 if _check_turn == 0 else 1):
			var from_dir: int = get_next_dir(curr.dir, int(d_from))
			var from_loc := get_back_g_loc(curr.x, curr.y, from_dir, step_size)

			for d in get_dir_diff_list(1 if _check_turn == 0 else 8):
				var from_node := InnNode.new(from_loc.x, from_loc.y, DIR_BEGIN if _check_turn == 0 else get_next_dir(curr.dir, int(d)))

				if prev_node != null and prev_node.eq_dir(from_node.x, from_node.y, from_node.dir):
					continue

				var src_dir: int = from_dir if _check_turn == 0 else from_node.dir
				var hop_cost = _call_one_step_cost(from_node.x, from_node.y, src_dir, curr.x, curr.y)
				if hop_cost == null:
					continue

				var reduced_cost: float = float(hop_cost) - _pr(curr_node.node.x, curr_node.node.y) + _pr(from_node.x, from_node.y)
				if reduced_cost < 0.0:
					reduced_cost = 0.0
				var new_cost: float = curr_node.cost + reduced_cost
				var k := from_node.key()
				if not _cost_r.has(k) or new_cost < float(_cost_r[k]):
					_cost_r[k] = new_cost
					_parent_r[k] = curr.dup()
					_pq_r.push(InnPQNode.new(from_node, new_cost))

				if _cost_f.has(k):
					_update_done_cost(from_node, new_cost + float(_cost_f[k]))


## pathf.hpp:477 pi() —— 启发式下界
func _pi(src_x: int, src_y: int, dst_x: int, dst_y: int) -> float:
	var d: int = maxi(absi(dst_x - src_x), absi(dst_y - src_y))
	var lowb_eps := 0.00001
	return (1.0 + 0.1 * float(_max_step) - lowb_eps) * float(d / _max_step) \
		+ (1.0 + 0.1 * 1.0 - lowb_eps) * float(d % _max_step)


## pathf.hpp:496 pf() —— 正向 reduced cost 基准
func _pf(x: int, y: int) -> float:
	return (_pi(x, y, _dst_x, _dst_y) - _pi(x, y, _src_node.x, _src_node.y) + _pi_r_t) / 2.0


## pathf.hpp:501 pr() —— 反向 reduced cost 基准
func _pr(x: int, y: int) -> float:
	return (_pi(x, y, _src_node.x, _src_node.y) - _pi(x, y, _dst_x, _dst_y) + _pi_f_s) / 2.0


func _update_done_cost(node: InnNode, cost: float) -> void:
	if _done_cost == null or cost < float(_done_cost):
		_done_cost = cost
		_done_node = node.dup()


func _call_one_step_cost(sx: int, sy: int, src_dir: int, dx: int, dy: int):
	if _one_step_cost.is_null():
		return _default_one_step_cost(sx, sy, src_dir, dx, dy)
	return _one_step_cost.call(sx, sy, src_dir, dx, dy)


## 默认代价：对应 ProcessRun::oneStepCost
func _default_one_step_cost(sx: int, sy: int, src_dir: int, dx: int, dy: int):
	var grid: int = int(_grid_checker.call(dx, dy))
	var hop_dir := get_off_dir(sx, sy, dx, dy)
	var hop_size := maxi(absi(dx - sx), absi(dy - sy))
	var grid_extra_pen := 0.0

	match grid:
		PF_FREE:
			pass
		PF_OCCUPIED, PF_LOCKED:
			grid_extra_pen += 100.0
		PF_NONE, PF_OBSTACLE:
			return null
		_:
			return null

	var turn_pen := 0.0
	if src_dir != DIR_NONE and hop_dir != DIR_NONE:
		turn_pen = get_dir_abs_diff(src_dir, hop_dir) * 0.01
	return 1.00 + float(hop_size) * 0.10 + grid_extra_pen + turn_pen


## pathf.cpp:452 getPathNode
func get_path_node() -> Array[Vector2i]:
	var result: Array[Vector2i] = []
	if _done_node == null:
		return result

	var first: Array[Vector2i] = []
	var curr: InnNode = _done_node
	var guard := 0
	while not curr.eq(_src_node.x, _src_node.y) and guard < 20000:
		guard += 1
		if not _parent_f.has(curr.key()):
			break
		var p: InnNode = _parent_f[curr.key()]
		first.push_front(Vector2i(p.x, p.y))
		if p.eq(_src_node.x, _src_node.y):
			break
		curr = p
	result.append_array(first)
	result.append(Vector2i(_done_node.x, _done_node.y))

	curr = _done_node
	guard = 0
	while not curr.eq(_dst_x, _dst_y) and guard < 20000:
		guard += 1
		if not _parent_r.has(curr.key()):
			break
		var p2: InnNode = _parent_r[curr.key()]
		result.append(Vector2i(p2.x, p2.y))
		if p2.eq(_dst_x, _dst_y):
			break
		curr = p2

	# 去掉起点，客户端移动只需要"下一步"
	if not result.is_empty() and result[0] == Vector2i(_src_node.x, _src_node.y):
		result.remove_at(0)
	return result


## 便捷封装
func find_path(src: Vector2i, dst: Vector2i, src_dir: int = DIR_NONE) -> Array[Vector2i]:
	var empty: Array[Vector2i] = []
	if src == dst:
		return empty
	var r := search(src.x, src.y, src_dir if src_dir > 0 else DIR_BEGIN, dst.x, dst.y)
	if not bool(r["has_path"]):
		return empty
	return get_path_node()
