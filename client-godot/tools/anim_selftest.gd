extends Node
## B10 动作播放自检：走路/站立切换、帧推进、朝向同步
## 触发：--run-test anim

var _pass := 0
var _fail := 0


func run(main: Node) -> void:
	print("=== 动作播放自检 ===")
	var player = main.get("_player")
	if player == null:
		print("[FAIL] 主角未创建")
		get_tree().quit(1)
		return

	var human = main.get("_human")
	if human == null:
		print("[FAIL] 人形未创建")
		get_tree().quit(1)
		return

	# 1. 初始应为 STAND
	var m0 = player.get("_motion")
	_ok(m0 != null, "初始动作已设置")
	_ok(m0 != null and m0.type == MotionTypeScript.MOTION_STAND, "初始为 STAND", "实际 %d" % (m0.type if m0 else -1))
	_ok(m0 != null and m0.frame_count_ex() == 4, "STAND 帧数 4", "实际 %d" % (m0.frame_count_ex() if m0 else 0))

	# 2. 帧推进：等一会儿，帧应变化
	# STAND 帧延迟 100ms，等 1 秒（60fps 约 60 帧）应推进若干帧
	var f_start: int = m0.frame
	var seen := {}
	for i in range(70):
		await get_tree().process_frame
		seen[m0.frame] = true
	var f_after: int = m0.frame
	_ok(seen.size() >= 2, "站立动画帧在推进（观察到 %d 个不同帧）" % seen.size(), "仅 %d 个" % seen.size())
	# 4 帧循环，结束时可能恰好回到起始帧，故只断言「看到过多个帧」
	_ok(seen.size() >= 4, "4 帧循环全部出现", "观察到 %d 个" % seen.size())

	# 3. 走路：请求移动，动作应切到 WALK
	var start: Vector2i = player.grid
	var target := Vector2i(-1, -1)
	for r in range(2, 10):
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
		print("[warn] 找不到目标，跳过走路测试")
		_finish()
		return

	var ok_move: bool = player.request_move(target)
	_ok(ok_move, "移动请求成功")

	var m_walk = player.get("_motion")
	_ok(m_walk != null and m_walk.type == MotionTypeScript.MOTION_WALK, "移动中动作切为 WALK",
		"实际 %d" % (m_walk.type if m_walk else -1))
	_ok(m_walk != null and m_walk.frame_count_ex() == 6, "WALK 帧数 6",
		"实际 %d" % (m_walk.frame_count_ex() if m_walk else 0))

	# 4. 等移动结束，动作应回到 STAND
	var waited := 0
	while player.is_moving() and waited < 1200:
		await get_tree().process_frame
		waited += 1
	_ok(not player.is_moving(), "移动结束")
	_ok(player.grid == target, "到达目标", "实际 %s" % str(player.grid))

	var m_end = player.get("_motion")
	_ok(m_end != null and m_end.type == MotionTypeScript.MOTION_STAND, "移动后回到 STAND",
		"实际 %d" % (m_end.type if m_end else -1))

	# 5. 朝向同步：动作 direction 应与 player.facing 一致
	_ok(m_end != null and m_end.direction == player.facing, "动作朝向与主角朝向一致",
		"motion.dir=%d facing=%d" % [m_end.direction if m_end else -1, player.facing])

	_finish()


func _finish() -> void:
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	get_tree().quit(0 if _fail == 0 else 1)


func _ok(c: bool, name: String, detail: String = "") -> void:
	if c:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, "  ", detail)


const MotionTypeScript := preload("res://scripts/action/motion_type.gd")
