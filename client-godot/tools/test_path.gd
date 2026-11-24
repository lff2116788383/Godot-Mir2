extends SceneTree
## 寻路与方向工具的离线自检
## 用法：godot --headless --path client-godot --script tools/test_path.gd
##
## 验收项：
##   1. 方向工具与原版 pathf.hpp 定义一致
##   2. 空地直线 / 对角路径正确
##   3. 障碍物绕行
##   4. 不可达返回空
##   5. 路径每步相邻且可走

var _passed := 0
var _failed := 0


func _init() -> void:
	print("=== 寻路自检 ===")
	_test_dir_tools()
	_test_straight()
	_test_diagonal()
	_test_obstacle()
	_test_unreachable()
	_test_path_valid()
	print("=== 结果: 通过 %d, 失败 %d ===" % [_passed, _failed])
	quit(0 if _failed == 0 else 1)


func _ok(cond: bool, name: String, detail: String = "") -> void:
	if cond:
		_passed += 1
		print("  [PASS] ", name)
	else:
		_failed += 1
		print("  [FAIL] ", name, "  ", detail)


func _test_dir_tools() -> void:
	print("[1] 方向工具")
	_ok(PathFinder.DIR_UP == 1, "DIR_UP=1")
	_ok(PathFinder.DIR_UPRIGHT == 2, "DIR_UPRIGHT=2")
	_ok(PathFinder.DIR_RIGHT == 3, "DIR_RIGHT=3")
	_ok(PathFinder.DIR_DOWN == 5, "DIR_DOWN=5")
	_ok(PathFinder.DIR_UPLEFT == 8, "DIR_UPLEFT=8")
	# get_front_g_loc：上=(0,-1) 右=(1,0)
	_ok(PathFinder.get_front_g_loc(10, 10, PathFinder.DIR_UP) == Vector2i(10, 9), "上移 y-1")
	_ok(PathFinder.get_front_g_loc(10, 10, PathFinder.DIR_RIGHT) == Vector2i(11, 10), "右移 x+1")
	_ok(PathFinder.get_front_g_loc(10, 10, PathFinder.DIR_UPRIGHT) == Vector2i(11, 9), "右上")
	_ok(PathFinder.get_front_g_loc(10, 10, PathFinder.DIR_DOWNLEFT) == Vector2i(9, 11), "左下")
	# 循环：上 的下一方向是 右上
	_ok(PathFinder.get_next_dir(PathFinder.DIR_UP, 1) == PathFinder.DIR_UPRIGHT, "上->右上")
	_ok(PathFinder.get_next_dir(PathFinder.DIR_UP, -1) == PathFinder.DIR_UPLEFT, "上->左上(回绕)")
	# 角差：上(1) 与 下(5) 相差 4；上(1) 与 右下(4) 相差 3 -> 取 3
	_ok(PathFinder.get_dir_abs_diff(PathFinder.DIR_UP, PathFinder.DIR_DOWN) == 4, "上下角差4")
	_ok(PathFinder.get_dir_abs_diff(PathFinder.DIR_UP, PathFinder.DIR_RIGHT) == 2, "上右角差2")
	_ok(PathFinder.get_off_dir(0, 0, 1, -1) == PathFinder.DIR_UPRIGHT, "位移->右上")


func _make_checker_open(w: int, h: int) -> Callable:
	return func(x: int, y: int) -> int:
		if x < 0 or y < 0 or x >= w or y >= h:
			return PathFinder.PF_NONE
		return PathFinder.PF_FREE


func _test_straight() -> void:
	print("[2] 直线路径")
	var f := PathFinder.new(_make_checker_open(50, 50), 1)
	var p := f.find_path(Vector2i(10, 10), Vector2i(13, 10))
	_ok(p.size() == 3, "距离3 -> 3步", "实际 %d" % p.size())
	_ok(p.size() > 0 and p[0] == Vector2i(11, 10), "首步正确")
	_ok(p.size() > 0 and p[-1] == Vector2i(13, 10), "末步=终点")


func _test_diagonal() -> void:
	print("[3] 对角路径")
	var f := PathFinder.new(_make_checker_open(50, 50), 1)
	var p := f.find_path(Vector2i(10, 10), Vector2i(13, 13))
	_ok(p.size() == 3, "对角距离3 -> 3步", "实际 %d" % p.size())
	if p.size() == 3:
		_ok(p[-1] == Vector2i(13, 13), "末步正确")
		# 每步都应是对角前进
		var ok := true
		var cur := Vector2i(10, 10)
		for q in p:
			if absi(q.x - cur.x) != 1 or absi(q.y - cur.y) != 1:
				ok = false
			cur = q
		_ok(ok, "每步均为对角")


func _test_obstacle() -> void:
	print("[4] 障碍绕行")
	# 竖墙 x=12, y in [0,20]，从 (10,10) 到 (14,10) 必须绕
	var checker := func(x: int, y: int) -> int:
		if x < 0 or y < 0 or x >= 50 or y >= 50:
			return PathFinder.PF_NONE
		if x == 12 and y <= 20:
			return PathFinder.PF_OBSTACLE
		return PathFinder.PF_FREE
	var f := PathFinder.new(checker, 1)
	var p := f.find_path(Vector2i(10, 10), Vector2i(14, 10))
	_ok(p.size() > 0, "能绕行")
	if p.size() > 0:
		var blocked := false
		for q in p:
			if q.x == 12 and q.y <= 20:
				blocked = true
		_ok(not blocked, "路径未穿墙")
		_ok(p[-1] == Vector2i(14, 10), "到达终点")


func _test_unreachable() -> void:
	print("[5] 完全阻隔")
	# 全高墙，不可达
	var checker := func(x: int, y: int) -> int:
		if x < 0 or y < 0 or x >= 20 or y >= 20:
			return PathFinder.PF_NONE
		if x == 10:
			return PathFinder.PF_OBSTACLE
		return PathFinder.PF_FREE
	var f := PathFinder.new(checker, 1)
	var p := f.find_path(Vector2i(5, 10), Vector2i(15, 10))
	_ok(p.is_empty(), "不可达返回空", "实际 %d 步" % p.size())


func _test_path_valid() -> void:
	print("[6] 随机路径合法性")
	var checker := _make_checker_open(40, 40)
	var f := PathFinder.new(checker, 1)
	var bad := 0
	for i in range(60):
		var sx := randi() % 30
		var sy := randi() % 30
		var dx := sx + (randi() % 8) + 1
		var dy := sy + (randi() % 8) + 1
		var p := f.find_path(Vector2i(sx, sy), Vector2i(dx, dy))
		if p.is_empty():
			bad += 1
			continue
		var cur := Vector2i(sx, sy)
		for q in p:
			if maxi(absi(q.x - cur.x), absi(q.y - cur.y)) != 1:
				bad += 1
				break
			cur = q
		if p[-1] != Vector2i(dx, dy):
			bad += 1
	_ok(bad == 0, "60 条随机路径全部合法且相邻", "异常 %d 条" % bad)
