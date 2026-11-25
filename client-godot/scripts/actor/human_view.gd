extends Node2D
class_name HumanView
## 其他玩家 / NPC 的表现节点
##
## 【依据】client/src/hero.cpp 动作队列 + hero.cpp:76-150 绘制顺序
##          clientnpc.cpp（NPC 也用 human 外观）
##
## 与 MonsterActor 的差别：
##   - 贴图来自 texture/hero|hair|helmet|weapon（分层），不是 APNG
##   - 动作类型是 MOTION_*（玩家动作），不是 MOTION_MON_*
##
## Godot 差异：把 HumanActor 组合进来，自己只管动作队列与坐标。

const HumanActorRef := preload("res://scripts/actor/human_actor.gd")
const MotionNodeRef := preload("res://scripts/action/motion_node.gd")
const MotionTypeRef := preload("res://scripts/action/motion_type.gd")
const ActionTypeRef := preload("res://scripts/action/action_type.gd")
const PathFinderRef := preload("res://scripts/action/path_finder.gd")
const CreatureMovableRef := preload("res://scripts/action/creature_movable.gd")
const SysConstRef := preload("res://scripts/core/sys_const.gd")
const AudioManagerRef := preload("res://scripts/audio/audio_manager.gd")

const GRID_X := 48
const GRID_Y := 32

var human: HumanActor = null
var _name_label: Label = null
var display_name: String = ""
var hp_ratio: float = 1.0
var is_male: bool = false
var curr_motion = null
var motion_queue: Array = []
var forced_queue: Array = []
var grid := Vector2i.ZERO
var facing: int = 5   ## DIR_DOWN

var _frame_acc := 0.0
var _dead := false

## 地图上下文（由 CoManager 注入）
var _can_move: Callable = Callable()
var _make_finder: Callable = Callable()
var _max_step: int = 10


func setup(assets_root: String, p_dress: int = 0, p_hair: int = 1,
		p_helmet: int = 0, p_weapon: int = 0, p_gender: bool = false) -> void:
	human = HumanActorRef.new()
	human.name = "Human"
	add_child(human)
	human.setup(assets_root)
	is_male = p_gender
	human.set_appearance(p_dress, p_hair, p_helmet, p_weapon, p_gender)
	# 脚底锚点：人形图约 22x63，需向左上偏移使脚落在格中心
	human.anchor_offset = Vector2(-10, -48)


## CoManager 创建实体后注入
func _ensure_name_label() -> void:
	if _name_label != null:
		return
	_name_label = Label.new()
	_name_label.position = Vector2(-34, -76)
	_name_label.size = Vector2(100, 36)
	_name_label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_name_label.add_theme_font_size_override("font_size", 11)
	_name_label.add_theme_color_override("font_color", Color(0.82, 0.9, 1.0))
	_name_label.add_theme_color_override("font_outline_color", Color(0,0,0,0.9))
	_name_label.add_theme_constant_override("outline_size", 2)
	add_child(_name_label)


func set_display_name(nm: String) -> void:
	display_name = nm
	_ensure_name_label()
	_refresh_name_label()


func set_health_ratio(r: float) -> void:
	hp_ratio = clampf(r, 0.0, 1.0)
	_refresh_name_label()


func _refresh_name_label() -> void:
	if _name_label == null:
		return
	_name_label.text = "%s\nHP %d%%" % [display_name, int(hp_ratio * 100.0)]


func set_walk_context(can_move: Callable, make_finder: Callable, max_step: int) -> void:
	_can_move = can_move
	_make_finder = make_finder
	_max_step = max_step


# ==================== 动作应用（hero.cpp:510 parseAction） ====================

func apply_action(action) -> void:
	if action == null or human == null:
		return
	if _dead:
		return
	motion_queue.clear()

	match action.type:
		ActionTypeRef.ACTION_DIE:
			_on_die(action)
		ActionTypeRef.ACTION_STAND:
			_on_stand(action)
		ActionTypeRef.ACTION_MOVE:
			_on_move(action)
		ActionTypeRef.ACTION_SPACEMOVE:
			_on_space_move(action)
		ActionTypeRef.ACTION_HITTED:
			_on_hitted(action)
		ActionTypeRef.ACTION_JUMP:
			_on_jump(action)
		ActionTypeRef.ACTION_ATTACK:
			_on_attack(action)
		ActionTypeRef.ACTION_SPELL:
			_on_spell(action)
		ActionTypeRef.ACTION_SPAWN:
			_on_stand(action)
		_:
			pass

	_begin_next_motion()


func _motion_end() -> Vector3i:
	if curr_motion != null:
		return Vector3i(curr_motion.end_x, curr_motion.end_y, curr_motion.direction)
	return Vector3i(grid.x, grid.y, facing)


func _on_stand(action) -> void:
	var end := _motion_end()
	motion_queue = CreatureMovableRef.make_walk_motion_queue(
		end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		false, _can_move, _make_finder, _max_step)
	var m := MotionNodeRef.new()
	m.type = MotionTypeRef.MOTION_STAND
	m.direction = action.direction
	m.x = action.x
	m.y = action.y
	m.end_x = m.x
	m.end_y = m.y
	motion_queue.append(m)


func _on_move(action) -> void:
	var end := _motion_end()
	motion_queue = CreatureMovableRef.make_walk_motion_queue(
		end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		false, _can_move, _make_finder, _max_step)
	var mn = CreatureMovableRef.make_walk_motion(
		action.x, action.y, action.aim_x, action.aim_y, action.speed, false, _can_move)
	if mn != null:
		motion_queue.append(mn)


func _on_attack(action) -> void:
	var end := _motion_end()
	motion_queue = CreatureMovableRef.make_walk_motion_queue(
		end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		false, _can_move, _make_finder, _max_step)
	var d: int = PathFinderRef.get_off_dir(action.x, action.y, action.aim_x, action.aim_y)
	if not PathFinderRef.dir_valid(d):
		d = end.z
	var m := MotionNodeRef.new()
	# hero.cpp:1108 默认物理攻击：单手位 ONEVSWING，双手位 TWOVSWING
	# 武器是否双手由 WLDesp 决定，待外观同步接入后细化
	m.type = MotionTypeRef.MOTION_ONEVSWING
	m.direction = d
	m.x = action.x
	m.y = action.y
	m.end_x = m.x
	m.end_y = m.y
	m.ext_magic_id = action.ext_magic_id
	m.ext_modifier_id = action.ext_modifier_id
	motion_queue.append(m)


func _on_spell(action) -> void:
	var end := _motion_end()
	motion_queue = CreatureMovableRef.make_walk_motion_queue(
		end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		false, _can_move, _make_finder, _max_step)
	var d: int = PathFinderRef.get_off_dir(action.x, action.y, action.aim_x, action.aim_y)
	if not PathFinderRef.dir_valid(d):
		d = end.z
	var m := MotionNodeRef.new()
	# 施法动作：原版按魔法是否有方向分 SPELL0/SPELL1，默认 SPELL0
	m.type = MotionTypeRef.MOTION_SPELL0
	m.direction = d
	m.x = action.x
	m.y = action.y
	m.end_x = m.x
	m.end_y = m.y
	m.ext_magic_id = action.ext_magic_id
	m.ext_modifier_id = action.ext_modifier_id
	motion_queue.append(m)


func _on_hitted(action) -> void:
	var end := _motion_end()
	motion_queue = CreatureMovableRef.make_walk_motion_queue(
		end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		false, _can_move, _make_finder, _max_step)
	var m := MotionNodeRef.new()
	m.type = MotionTypeRef.MOTION_HITTED
	m.direction = action.direction
	m.x = action.x
	m.y = action.y
	m.end_x = m.x
	m.end_y = m.y
	m.ext_from_uid = action.ext_from_uid
	motion_queue.append(m)


func _on_jump(action) -> void:
	forced_queue.clear()
	curr_motion = MotionNodeRef.new()
	curr_motion.type = MotionTypeRef.MOTION_STAND
	curr_motion.direction = action.direction
	curr_motion.x = action.x
	curr_motion.y = action.y
	_set_grid(action.x, action.y)


func _on_die(action) -> void:
	var end := _motion_end()
	for m in CreatureMovableRef.make_walk_motion_queue(
			end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		false, _can_move, _make_finder, _max_step):
		forced_queue.append(m)
	var de := _motion_end()
	var m := MotionNodeRef.new()
	m.type = MotionTypeRef.MOTION_DIE
	m.direction = de.z if PathFinderRef.dir_valid(de.z) else facing
	m.x = de.x
	m.y = de.y
	m.end_x = m.x
	m.end_y = m.y
	forced_queue.append(m)
	_dead = true


func _on_space_move(action) -> void:
	forced_queue.clear()
	curr_motion = MotionNodeRef.new()
	curr_motion.type = MotionTypeRef.MOTION_STAND
	curr_motion.direction = facing
	curr_motion.x = action.aim_x
	curr_motion.y = action.aim_y
	_set_grid(action.aim_x, action.aim_y)


# ==================== 队列推进 ====================

## ★ 角色按 y 排序绘制（processrun.cpp:352-359 的 LocHashTable 语义）
##
## 原版把实体按格坐标分组后逐格绘制，效果等价于「y 越大越后画」。
## Godot 里用 z_index 表达：z = y，同 y 由加入顺序决定 —— 与原版一致。
## 这样"人站在树后会被树挡住、站在树前会挡住树"才对。
## 范围检查：地图最多 800 行，z_index 上限 4096，够用。
func _set_grid(x: int, y: int) -> void:
	grid = Vector2i(x, y)
	position = Vector2(x * GRID_X, y * GRID_Y)
	# z 交给 Godot 的 y_sort（见 co_manager / main 里对父节点的设置），
	# 这里不再手工算 z_index —— 手工取模会打断全局顺序。
	# 角色与角色之间由 y_sort 按 position.y 排序，等价于原版按格 y 分组。
	z_index = 0


func _begin_next_motion() -> void:
	if not forced_queue.is_empty():
		curr_motion = forced_queue.pop_front()
	elif not motion_queue.is_empty():
		curr_motion = motion_queue.pop_front()

	if curr_motion != null:
		facing = curr_motion.direction
		_set_grid(curr_motion.x, curr_motion.y)
		_frame_acc = 0.0
		if human != null:
			human.set_motion(curr_motion)


## 动作推进，照搬 clientcreature.hpp:150 updateMotion + creaturemovable.cpp:191
func _physics_process(dt: float) -> void:
	if curr_motion == null or human == null:
		return
	var delay: float = curr_motion.frame_delay()
	if delay <= 0.0:
		delay = 100.0
	_frame_acc += dt * 1000.0

	while _frame_acc >= delay:
		_frame_acc -= delay
		var n: int = curr_motion.frame_count_ex()
		if n <= 0:
			break
		if curr_motion.frame + 1 < n:
			curr_motion.frame += 1
			human.refresh_textures()
			_play_frame_sound()
		else:
			_move_next_motion()
			if curr_motion == null:
				break


## 【C8】逐帧音效（hero.cpp:240-338 的 CallOnExitHelper 语义）
##
## 原版在一个动作的「序号帧变化」时触发音效，判定顺序：
##   1. 脚步：MOTION_WALK/RUN/ONHORSEWALK/ONHORSERUN，frame==1 放 base、frame==4 放 base+1
##   2. 挥击：七种 SWING 动作，frame==0 按武器 category 选音
##   3. 受击：MOTION_HITTED，人声 + 武器/身体/护甲声
##
## Godot 差异：原版用 seqFrameID 判重，这里在「帧号真的推进了」之后调用，
## 等价于同一个序号帧只触发一次。
func _play_frame_sound() -> void:
	var audio = _audio()
	if audio == null or curr_motion == null:
		return
	var mt: int = curr_motion.type
	var fr: int = curr_motion.frame
	var gx: int = curr_motion.x
	var gy: int = curr_motion.y

	# 1. 脚步
	var step_id: int = AudioManagerRef.step_seff(mt, fr)
	if step_id >= 0:
		audio.play_seff_at(step_id, gx, gy)
		return

	# 2. 挥击（frame 0）
	if fr == 0 and mt in AudioManagerRef.SWING_MOTIONS:
		var cat := _weapon_category()
		audio.play_seff_at(AudioManagerRef.attack_seff(cat), gx, gy)
		return

	# 3. 受击（frame 0）
	if fr == 0 and mt == MotionTypeRef.MOTION_HITTED:
		# 人声（性别由外观决定，这里默认男声）
		audio.play_seff_at(AudioManagerRef.struck_voice_seff(gender), gx, gy)
		audio.play_seff_at(AudioManagerRef.struck_body_seff(has_dress), gx, gy)


## 取 AudioManager（Autoload）
func _audio():
	var t := get_node_or_null("/root/Audio")
	return t


## 武器类别（用于选挥击音效）
## 由外部（Game 或 actor_factory）注入，因为没有完整背包时不查表
var weapon_category := ""
var gender := 0
var has_dress := false


func _weapon_category() -> String:
	return weapon_category


## creaturemovable.cpp:191
func _move_next_motion() -> void:
	if not forced_queue.is_empty():
		_switch_to(forced_queue.pop_front())
		return
	if not motion_queue.is_empty():
		if CreatureMovableRef.motion_queue_valid(curr_motion, motion_queue):
			_switch_to(motion_queue.pop_front())
			return
		motion_queue.clear()
	# 队列空 -> makeIdleMotion
	_switch_to(_make_idle_motion())


func _switch_to(m) -> void:
	curr_motion = m
	facing = m.direction
	_set_grid(m.x, m.y)
	if human != null:
		human.set_motion(m)


func _make_idle_motion():
	var m = MotionNodeRef.new()
	m.type = MotionTypeRef.MOTION_STAND
	m.direction = facing if PathFinderRef.dir_valid(facing) else PathFinderRef.DIR_DOWN
	m.x = grid.x
	m.y = grid.y
	m.end_x = m.x
	m.end_y = m.y
	# 关键：end_x/end_y 必须同步设置，否则 motionEndGLoc 读到 -1，
	# 后续动作的起点会变成 (-1,-1)，导致 makeWalkMotionQueue 全空
	m.end_x = grid.x
	m.end_y = grid.y
	return m


func set_appearance(p_dress: int, p_hair: int, p_helmet: int, p_weapon: int,
		p_gender: bool) -> void:
	if human != null:
		human.set_appearance(p_dress, p_hair, p_helmet, p_weapon, p_gender)
