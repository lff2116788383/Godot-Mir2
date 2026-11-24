extends Node2D
class_name MonsterActor
## 怪物表现节点
##
## 【逐行翻译】client/src/clientmonster.cpp + clientcreature 的动作队列语义
## 对照：
##   - clientmonster.cpp:57-78   MonsterFrameGfxSeq::gfxID  贴图键合成
##   - clientmonster.cpp:371-399 parseAction 分发
##   - clientmonster.cpp:401-500 onActionDie/Stand/Hitted/SpaceMove/Jump
##   - clientcreature.hpp:120-175 动作队列推进
##
## Godot 差异：
##   原版 draw() 每次重绘几何体；这里用 Sprite2D 换贴图。
##   动作队列推进逻辑（forcedMotion → motion，播完出队）完全照搬。

const ClientMonsterRef := preload("res://scripts/actor/client_monster.gd")
const MotionNodeRef := preload("res://scripts/action/motion_node.gd")
const MotionTypeRef := preload("res://scripts/action/motion_type.gd")
const ActionTypeRef := preload("res://scripts/action/action_type.gd")
const PathFinderRef := preload("res://scripts/action/path_finder.gd")
const SysConstRef := preload("res://scripts/core/sys_const.gd")
const CreatureMovableRef := preload("res://scripts/action/creature_movable.gd")
const MonsterRecordRef := preload("res://scripts/item/monster_record.gd")
const UidRef := preload("res://scripts/core/uid.gd")
const BehaviorsRef := preload("res://scripts/actor/monster_behaviors.gd")
const AudioManagerRef := preload("res://scripts/audio/audio_manager.gd")

## 格尺寸
const GRID_X := 48
const GRID_Y := 32

## 逻辑对象（帧表、gfxID 计算）
var logic: ClientMonster = null

var _spr: Sprite2D = null
var _assets_root: String = ""

## 当前动作
var curr_motion = null
## 待播动作队列（clientcreature.hpp m_motionQueue）
var motion_queue: Array = []
## 强制队列（死亡等不可打断，clientcreature.hpp m_forcedMotionQueue）
var forced_queue: Array = []

var _frame_acc := 0.0
## 当前格坐标
var grid := Vector2i.ZERO
## 朝向
var facing: int = 0
## 是否已死（停止推进）
var dead := false

## 行为描述（来自 monster_behaviors.json，C6 产物）
## 决定：攻击用 ATTACK0/ATTACK1、有无趴站切换、特殊动作
var behavior: Dictionary = {}

## 地图上下文（由 CoManager 注入）
var _can_move: Callable = Callable()
var _make_finder: Callable = Callable()
var _max_step: int = 10


func setup(p_assets_root: String, uid: int, monster_id: int) -> bool:
	_assets_root = p_assets_root
	var mr := MonsterRecordRef.get_record(monster_id)
	if mr == null or mr.is_empty():
		push_warning("怪物记录不存在: %d" % monster_id)
		return false
	logic = ClientMonsterRef.new()
	# 原版按怪物名选子类；这里用 v2 类名提取（client_monster.gd 已实现）
	var gfx_cls := MonsterRecordRef.gfx_class_of(monster_id, mr)
	logic.setup(uid, mr, gfx_cls)
	# C6：读行为差异表（32 个类的差异全在这里）
	# 注意：19 个类本身就是「纯默认行为」，没有自定义项，
	# 此时 gfx_cls 为空，仍要拿到默认行为描述而不是空字典
	behavior = BehaviorsRef.of(gfx_cls) if not gfx_cls.is_empty() else BehaviorsRef.of("")
	# 补上怪物名，便于日志与统计
	if behavior.is_empty():
		behavior = BehaviorsRef.of("")
	behavior["monster_name"] = MonsterRecordRef.name_of(monster_id)
	_spr = Sprite2D.new()
	_spr.centered = false
	_spr.texture_filter = CanvasItem.TEXTURE_FILTER_NEAREST
	add_child(_spr)
	return true


## CoManager 创建实体后注入
func set_walk_context(can_move: Callable, make_finder: Callable, max_step: int) -> void:
	_can_move = can_move
	_make_finder = make_finder
	_max_step = max_step


func monster_name() -> String:
	return logic.monster_name() if logic != null else ""


# ==================== 动作应用（clientmonster.cpp:371） ====================

func apply_action(action) -> void:
	if action == null or logic == null:
		return

	# clientmonster.cpp:377 死亡后只播完强制队列，不再接受新动作
	for m in forced_queue:
		if m != null and m.type == MotionTypeRef.MOTION_MON_DIE:
			return

	motion_queue.clear()

	match action.type:
		ActionTypeRef.ACTION_DIE:
			_on_action_die(action)
		ActionTypeRef.ACTION_STAND:
			_on_action_stand(action)
		ActionTypeRef.ACTION_HITTED:
			_on_action_hitted(action)
		ActionTypeRef.ACTION_JUMP:
			_on_action_jump(action)
		ActionTypeRef.ACTION_MOVE:
			_on_action_move(action)
		ActionTypeRef.ACTION_ATTACK:
			_on_action_attack(action)
		ActionTypeRef.ACTION_SPAWN:
			_on_action_spawn(action)
		ActionTypeRef.ACTION_TRANSF:
			_on_action_transf(action)
		ActionTypeRef.ACTION_SPACEMOVE:
			_on_action_space_move(action)
		_:
			pass

	_begin_next_motion()


## clientmonster.cpp:518 onActionSpawn
func _on_action_spawn(action) -> void:
	var d: int = action.direction
	if not PathFinderRef.dir_valid(d):
		d = PathFinderRef.DIR_UP
	curr_motion = MotionNodeRef.new()
	curr_motion.type = MotionTypeRef.MOTION_MON_STAND
	curr_motion.direction = d
	curr_motion.x = action.x
	curr_motion.y = action.y
	_set_grid(action.x, action.y)


## clientmonster.cpp:512 onActionMove
func _on_action_move(action) -> void:
	var end := _motion_end()
	# 怪物用专用版本：动作类型是 MOTION_MON_WALK（怪物没有 RUN）
	motion_queue = CreatureMovableRef.make_monster_walk_queue(
		end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		_can_move, _make_finder, 1)
	var mn = CreatureMovableRef.make_monster_walk_motion(
		action.x, action.y, action.aim_x, action.aim_y, action.speed, _can_move, 1)
	if mn != null:
		motion_queue.append(mn)


## clientmonster.cpp:534 onActionAttack
func _on_action_attack(action) -> void:
	var end := _motion_end()
	motion_queue = CreatureMovableRef.make_monster_walk_queue(
		end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		_can_move, _make_finder, 1)
	# 原版还需要 findUID(aimUID) 取目标位置决定朝向
	# 这里退化为用目标格推算（若无目标信息则用 aim 坐标）
	var d: int = PathFinderRef.get_off_dir(action.x, action.y, action.aim_x, action.aim_y)
	if not PathFinderRef.dir_valid(d):
		d = end.z
	var mn := MotionNodeRef.new()
	mn.type = MotionTypeRef.MOTION_MON_ATTACK0
	mn.direction = d
	mn.x = action.x
	mn.y = action.y
	motion_queue.append(mn)


## clientmonster.cpp:465 onActionStand
func _on_action_stand(action) -> void:
	var end := _motion_end()
	motion_queue = CreatureMovableRef.make_monster_walk_queue(
		end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		_can_move, _make_finder, 1)
	var mn := MotionNodeRef.new()
	mn.type = MotionTypeRef.MOTION_MON_STAND
	mn.direction = action.direction
	mn.x = action.x
	mn.y = action.y
	motion_queue.append(mn)


## clientmonster.cpp:481 onActionHitted
func _on_action_hitted(action) -> void:
	var end := _motion_end()
	motion_queue = CreatureMovableRef.make_monster_walk_queue(
		end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		_can_move, _make_finder, 1)
	var mn := MotionNodeRef.new()
	mn.type = MotionTypeRef.MOTION_MON_HITTED
	mn.direction = action.direction
	mn.x = action.x
	mn.y = action.y
	mn.ext_from_uid = action.ext_from_uid
	motion_queue.append(mn)


## clientmonster.cpp:401 onActionDie
## 注意：进入 forced_queue，不可被新动作打断
func _on_action_die(action) -> void:
	var end := _motion_end()
	var walk := CreatureMovableRef.make_monster_walk_queue(
		end.x, end.y, action.x, action.y, SysConstRef.SYS_MAXSPEED,
		_can_move, _make_finder, 1)
	for m in walk:
		forced_queue.append(m)

	var de := _motion_end()
	var mn := MotionNodeRef.new()
	mn.type = MotionTypeRef.MOTION_MON_DIE
	mn.direction = de.z if PathFinderRef.dir_valid(de.z) else PathFinderRef.DIR_UP
	mn.x = de.x
	mn.y = de.y
	forced_queue.append(mn)
	dead = true


## clientmonster.cpp:492 onActionSpaceMove
func _on_action_space_move(action) -> void:
	forced_queue.clear()
	curr_motion = MotionNodeRef.new()
	curr_motion.type = MotionTypeRef.MOTION_MON_STAND
	curr_motion.direction = facing
	curr_motion.x = action.aim_x
	curr_motion.y = action.aim_y
	_set_grid(action.aim_x, action.aim_y)


## clientmonster.cpp:500 onActionJump
func _on_action_jump(action) -> void:
	forced_queue.clear()
	curr_motion = MotionNodeRef.new()
	curr_motion.type = MotionTypeRef.MOTION_MON_STAND
	curr_motion.direction = action.direction
	curr_motion.x = action.x
	curr_motion.y = action.y
	_set_grid(action.x, action.y)


## clientmonster.cpp:492 onActionTransf
## 原版对怪物直接 panic，这里按「切 standMode」处理（对应 ClientStandMonster）
func _on_action_transf(action) -> void:
	# 只有 7 个类有趴站切换（TaoDog/ZumaTaurus/CannibalPlant 等），
	# 其余类收到 TRANSF 时应忽略（原版基类抛 panic，这里温和处理）
	if logic != null and bool(behavior.get("has_stand_mode", false)):
		logic.stand_mode = not logic.stand_mode
	_refresh_texture()


# ==================== 队列推进 ====================

## 取当前运动终点（clientcreature.cpp motionEndGLoc().at(1)）
## 返回 Vector3i(x, y, dir)
func _motion_end() -> Vector3i:
	if curr_motion != null:
		return Vector3i(curr_motion.end_x, curr_motion.end_y, curr_motion.direction)
	return Vector3i(grid.x, grid.y, facing)


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
	# 强制队列优先
	if not forced_queue.is_empty():
		_switch_to(forced_queue.pop_front())
	elif not motion_queue.is_empty():
		_switch_to(motion_queue.pop_front())
	_frame_acc = 0.0


func _physics_process(dt: float) -> void:
	if curr_motion == null or logic == null:
		return
	_advance_animation(dt)


## 动作推进，严格照搬 clientcreature.hpp:150 updateMotion + creaturemovable.cpp:191 moveNextMotion
##
##   updateMotion(allowSwitch):
##     frame + 1 < count  -> advanceMotionFrame()（frame++）
##     allowSwitch        -> moveNextMotion()（换动作；队列空则新建 idle）
##     否则               -> true（停在末帧，死亡等）
func _advance_animation(dt: float) -> void:
	var delay: float = curr_motion.frame_delay()
	if delay <= 0.0:
		delay = 100.0
	_frame_acc += dt * 1000.0

	while _frame_acc >= delay:
		_frame_acc -= delay
		var n: int = logic.frame_count(curr_motion.type)
		if n <= 0:
			break

		if curr_motion.frame + 1 < n:
			# advanceMotionFrame：纯 frame++
			curr_motion.frame += 1
			_refresh_texture()
			_play_frame_sound()
		else:
			# 末帧：allowSwitchMotion=true 时切动作
			_move_next_motion()
			# 若切到了新的 idle，帧已归 0，继续循环下一轮
			if curr_motion == null:
				break


## 【C8】怪物逐帧音效（clientmonster.cpp:100-177）
##
## 原版在 update 里的触发条件：
##   MOTION_MON_SPAWN  frame==0 -> getSeffID(MONSEFF_SPAWN)
##   MOTION_MON_STAND  不发声（注释：shall only play when show up）
##   MOTION_MON_ATTACK0/1  frame==0 -> getSeffID(MONSEFF_ATTACK)
##   MOTION_MON_HITTED frame==0 -> getSeffID(MONSEFF_HITTED)
##                                + 按攻击者类型追加武器声
##   MOTION_MON_DIE    frame==0 -> getSeffID(MONSEFF_DIE)
##
## 音效 id = 0x02000000 + lookID * 16 + offset（怪物表里没有 seff 覆盖配置，
## 848 个音效文件里 0x02 段有 647 个，正好覆盖 40 个 lookID × 16）
func _play_frame_sound() -> void:
	var audio = get_node_or_null("/root/Audio")
	if audio == null or curr_motion == null or logic == null:
		return
	var mt: int = curr_motion.type
	var fr: int = curr_motion.frame
	if fr != 0:
		return

	var look := int(logic.look_id()) if logic.has_method("look_id") else 0
	var gx: int = curr_motion.x
	var gy: int = curr_motion.y

	if mt == MotionTypeRef.MOTION_MON_SPAWN:
		audio.play_seff_at(AudioManagerRef.mon_seff(look, AudioManagerRef.MONSEFF_SPAWN), gx, gy)
	elif mt == MotionTypeRef.MOTION_MON_ATTACK0 or mt == MotionTypeRef.MOTION_MON_ATTACK1:
		audio.play_seff_at(AudioManagerRef.mon_seff(look, AudioManagerRef.MONSEFF_ATTACK), gx, gy)
	elif mt == MotionTypeRef.MOTION_MON_HITTED:
		audio.play_seff_at(AudioManagerRef.mon_seff(look, AudioManagerRef.MONSEFF_HITTED), gx, gy)
		# 被玩家打时补武器声（clientmonster.cpp:133-150）
		if hitted_from_is_player:
			audio.play_seff_at(AudioManagerRef.MON_HITTED_BY, gx, gy)
	elif mt == MotionTypeRef.MOTION_MON_DIE:
		audio.play_seff_at(AudioManagerRef.mon_seff(look, AudioManagerRef.MONSEFF_DIE), gx, gy)


## 被击来源是否是玩家（由外部在收到 ACTION_HITTED 时置位）
var hitted_from_is_player := false


## creaturemovable.cpp:191 moveNextMotion
##   强制队列优先 -> 普通队列 -> 队列空则新建 idle 动作
func _move_next_motion() -> void:
	if not forced_queue.is_empty():
		_switch_to(forced_queue.pop_front())
		return

	if not motion_queue.is_empty():
		if CreatureMovableRef.motion_queue_valid(curr_motion, motion_queue):
			_switch_to(motion_queue.pop_front())
			return
		# 队列不衔接：清空并回 idle（原版打 warning）
		motion_queue.clear()

	# 队列空 -> makeIdleMotion：新建站立动作，frame 从 0 开始
	# 这就是站立动画会循环的原因
	_switch_to(_make_idle_motion())


## 切换动作：逻辑坐标落到新动作的 end 位置
## 原版：动作结束时 currMotion 换成下一个，新动作的 x/y 即上一动作的 endX/endY
func _switch_to(m) -> void:
	# 先把「上一个动作」的终点落地（行走类动作结束后应停在 end）
	var landed := false
	if curr_motion != null and curr_motion.end_x >= 0:
		_set_grid(curr_motion.end_x, curr_motion.end_y)
		landed = true
	curr_motion = m
	facing = m.direction
	# 新动作若自带合法起点，则以其起点为准（保证链条衔接）；
	# 但 idle 动作的 x/y 是「构造时快照」的 grid，落地后不应再覆盖回去
	if not landed and m.x >= 0:
		_set_grid(m.x, m.y)
	_refresh_texture()


## 怪物 idle 动作（MOTION_MON_STAND）
func _make_idle_motion():
	var m = MotionNodeRef.new()
	m.type = MotionTypeRef.MOTION_MON_STAND
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


# ==================== 贴图刷新（clientmonster.cpp:57） ====================

func _refresh_texture() -> void:
	if _spr == null or logic == null or curr_motion == null:
		return
	var seq := logic.gfx_id(curr_motion.type, curr_motion.direction, curr_motion.frame)
	if seq.is_empty():
		_spr.visible = false
		return
	var key: int = int(seq["key"])
	var frame: int = int(seq["frame"])
	var path := _texture_path(key)
	if path.is_empty():
		_spr.visible = false
		return

	# 怪物是 APNG：一个 key 一个文件，内含多帧
	# 【依据】pngtexoffdb.cpp:26  loadPNGAnimation -> frameList[frame]
	var frames := ApngLoaderRef.load_frames(path)
	if frames.is_empty():
		_spr.visible = false
		return
	var tex: Texture2D = frames[clampi(frame, 0, frames.size() - 1)]
	if tex == null:
		_spr.visible = false
		return

	_spr.texture = tex
	_spr.visible = true

	# 按文件名里的 DX/DY 定位（pngtexoffdb.cpp:31-36 的 12 位补码）
	var info := ApngLoaderRef.decode_file_name(path.get_file().get_basename())
	var dx: int = int(info.get("dx", 0))
	var dy: int = int(info.get("dy", 0))
	# Sprite 是本节点的子节点，position 必须是「相对父节点」的局部偏移。
	# 本节点已在 (grid.x*48, grid.y*32)，故这里只需 (dx, dy) + 向上抬高度。

	# 行走中的像素位移（creaturemovable.cpp:216 getShift）
	# 逻辑坐标只在动作切换时落格，走动过程靠这里的像素偏移表现
	var shift := CreatureMovableRef.get_shift(curr_motion, curr_motion.frame, true)
	_spr.position = Vector2(dx + shift.x, dy + shift.y) - Vector2(0, tex.get_height())


## 怪物贴图路径：texture/monster/{KEY:08X}{DX}{DY}.PNG
## 目录扫描建索引，避免每次拼文件名
func _texture_path(key: int) -> String:
	if _path_index.is_empty():
		_build_index()
	return _path_index.get(key, "")


func _build_index() -> void:
	var dir := "%s/texture/monster" % _assets_root
	var da := DirAccess.open(dir)
	if da == null:
		return
	da.list_dir_begin()
	var fn := da.get_next()
	while fn != "":
		if fn.to_upper().ends_with(".PNG") and fn.length() >= 12:
			var k := fn.substr(0, 8).hex_to_int()
			_path_index[k] = "%s/%s" % [dir, fn]
		fn = da.get_next()
	da.list_dir_end()


const ApngLoaderRef := preload("res://scripts/data/apng_loader.gd")

## KEY -> 绝对路径
var _path_index: Dictionary = {}
