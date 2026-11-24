extends SceneTree
## 端到端移动测试：加载真实地图，模拟点击，验证逐格移动
## 用法：godot --headless --path client-godot --script tools/test_move.gd

const ASSETS := "E:/Desktop/game/mir2x_assets-main"

var _pass := 0
var _fail := 0


func _init() -> void:
	print("=== 移动端到端测试 ===")

	# 手动挂 Autoload（--script 模式不生效）
	var game: Node = load("res://scripts/core/game.gd").new()
	game.name = "Game"
	root.add_child(game)
	game.set("assets_root", _assets_root())

	var tex: Node = load("res://scripts/data/tex_cache.gd").new()
	tex.name = "TexCache"
	root.add_child(tex)

	var main: Node = load("res://scenes/main.tscn").instantiate()
	root.add_child(main)

	for i in range(10):
		await process_frame

	var map = main.get("_map")
	if map == null:
		print("[FAIL] 地图未加载")
		quit(1)
		return
	_ok(true, "地图已加载 %dx%d" % [map.w, map.h])

	var player = main.get("_player")
	if player == null:
		print("[FAIL] 主角未创建")
		quit(1)
		return
	_ok(true, "主角已创建")

	var start: Vector2i = player.grid
	print("[info] 起点 = ", start)

	# 找一个 5 格内的可走目标
	var main_script := main
	var target := Vector2i(-1, -1)
	for dy in range(-6, 7):
		for dx in range(-6, 7):
			if dx == 0 and dy == 0:
				continue
			var g := start + Vector2i(dx, dy)
			if main_script.call("_is_walkable", g):
				target = g
				break
		if target.x >= 0:
			break

	if target.x < 0:
		print("[FAIL] 附近找不到可走目标")
		quit(1)
		return
	print("[info] 目标 = ", target)

	# 请求移动
	var ok: bool = player.request_move(target)
	_ok(ok, "request_move 返回成功")

	# 等移动结束（最多 5 秒）
	var waited := 0
	while player.is_moving() and waited < 300:
		await process_frame
		waited += 1

	_ok(not player.is_moving(), "移动已结束")
	_ok(player.grid == target, "到达目标格", "实际 %s" % str(player.grid))
	_ok(player.facing > 0 and player.facing < PathFinder.DIR_END, "朝向合法 = %d" % player.facing)

	# 验证相机跟随
	var cam: Camera2D = main.get_node("Camera2D")
	var expect_pos: Vector2 = main.get_node("MapView").grid_to_world(player.grid)
	_ok(cam.position.distance_to(expect_pos) < 1.0, "相机已跟随")

	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0 if _fail == 0 else 1)


func _assets_root() -> String:
	var e := OS.get_environment("MIR2X_ASSETS")
	return e if not e.is_empty() else ASSETS


func _ok(c: bool, name: String, detail: String = "") -> void:
	if c:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, "  ", detail)
