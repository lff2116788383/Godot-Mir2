extends Node
## 移动自测节点：挂在场景里运行，Autoload 正常可用
## 触发方式：加 --run-test move 启动参数

const ASSETS := "E:/Desktop/game/mir2x_assets-main"

var _pass := 0
var _fail := 0


func run(main: Node) -> void:
	print("=== 移动端到端测试 ===")
	var map = main.get("_map")
	if map == null:
		print("[FAIL] 地图未加载")
		get_tree().quit(1)
		return
	_ok(true, "地图已加载 %dx%d" % [map.w, map.h])

	var player = main.get("_player")
	if player == null:
		print("[FAIL] 主角未创建")
		get_tree().quit(1)
		return
	_ok(true, "主角已创建")

	var start: Vector2i = player.grid
	print("[info] 起点 = ", start)

	var target := Vector2i(-1, -1)
	for r in range(6, 13):
		for dy in range(-r, r + 1):
			for dx in range(-r, r + 1):
				if absi(dx) != r and absi(dy) != r:
					continue
				var g: Vector2i = start + Vector2i(dx, dy)
				if main.call("_is_walkable", g):
					target = g
					break
			if target.x >= 0:
				break
		if target.x >= 0:
			break

	if target.x < 0:
		print("[FAIL] 附近找不到可走目标")
		get_tree().quit(1)
		return
	print("[info] 目标 = ", target, " 距离 = ", maxi(absi(target.x - start.x), absi(target.y - start.y)))

	var ok: bool = player.request_move(target)
	_ok(ok, "request_move 返回成功")

	var waited := 0
	while player.is_moving() and waited < 600:
		await get_tree().process_frame
		waited += 1

	_ok(not player.is_moving(), "移动已结束（等待 %d 帧）" % waited)
	_ok(player.grid == target, "到达目标格", "实际 %s" % str(player.grid))
	_ok(player.facing >= PathFinder.DIR_BEGIN and player.facing < PathFinder.DIR_END, "朝向合法 = %d" % player.facing)

	var cam: Camera2D = main.get_node("Camera2D")
	var expect_pos: Vector2 = main.get_node("MapView").grid_to_world(player.grid)
	_ok(cam.position.distance_to(expect_pos) < 1.0, "相机已跟随")

	# 第二轮：长距离，验证多格路径与逐格推进
	var start2: Vector2i = player.grid
	var target2 := Vector2i(-1, -1)
	for r in range(12, 25):
		for dy in range(-r, r + 1):
			for dx in range(-r, r + 1):
				if absi(dx) != r and absi(dy) != r:
					continue
				var g2: Vector2i = start2 + Vector2i(dx, dy)
				if main.call("_is_walkable", g2):
					target2 = g2
					break
			if target2.x >= 0:
				break
		if target2.x >= 0:
			break
	if target2.x >= 0:
		var dist: int = maxi(absi(target2.x - start2.x), absi(target2.y - start2.y))
		print("[info] 第二轮：从 %s 到 %s 距离 %d" % [start2, target2, dist])
		var steps: Array[Vector2i] = []
		player.grid_changed.connect(func(g: Vector2i): steps.append(g))
		var ok2: bool = player.request_move(target2)
		_ok(ok2, "长距离 request_move 成功")
		var w2 := 0
		while player.is_moving() and w2 < 2000:
			await get_tree().process_frame
			w2 += 1
		_ok(player.grid == target2, "长距离到达目标", "实际 %s" % str(player.grid))
		_ok(steps.size() >= dist - 1, "逐格推进（%d 次格变化，距离 %d）" % [steps.size(), dist])
	else:
		print("[warn] 找不到长距离目标，跳过第二轮")

	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	get_tree().quit(0 if _fail == 0 else 1)


func _ok(c: bool, name: String, detail: String = "") -> void:
	if c:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, "  ", detail)
