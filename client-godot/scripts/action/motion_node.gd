extends RefCounted
class_name MotionNode
## 动作节点：驱动动画播放的运行时状态
##
## 【逐行翻译】client/src/motionnode.hpp（198 行）
## 相关的关键实现：
##   - motionnode.hpp:164-167  frameDelay()   帧间隔
##   - motionnode.hpp:169-177  getSeqID()     动作序列 ID
##   - motionnode.hpp:179-197  gfxType/gfxFrame
##   - clientcreature.hpp:149  updateMotion() 帧推进
##   - clientcreature.hpp:132  getFrameCountEx
##
## 设计说明：原版 MotionNode 是带 unique_ptr<MotionEffect> 和 std::list<trigger> 的
## 重量级对象；这里保留同样的字段语义，effect 与 trigger 用 Variant/Callable 承载，
## 因为 Godot 侧的 effect 需要节点（渲染）而 trigger 需要闭包（逻辑）。

const SysConstRef := preload("res://scripts/core/sys_const.gd")

var type: int = 0
var direction: int = 0
var speed: int = 100        ## SYS_DEFSPEED
var x: int = -1
var y: int = -1
var end_x: int = -1
var end_y: int = -1
var frame: int = 0

## 扩展参数（对应 motionnode.hpp:34-71 的 MotionExtParam）
var ext_magic_id: int = 0        ## spell / swing
var ext_attack_motion: int = 0   ## attack
var ext_from_uid: int = 0        ## hitted
var ext_fade_out: int = 0        ## die（update 期间可变）
var ext_combined_frames: Array = []   ## combined: [[gfxType, gfxFrame], ...]

## 动作特效（对应 std::unique_ptr<MotionEffect>）
var effect = null

## 触发器队列（对应 std::list<std::function<bool(MotionNode*)>>）
## 每个元素是 Callable，返回 true 表示触发并移除
var _trigger_list: Array[Callable] = []

## 动作序号（对应 m_seq，全局自增）
static var _roll_motion_seq: int = 1
var m_seq: int = 0


func _init() -> void:
	# motionnode.hpp:98-109：全局自增，到 UINT32_MAX 回绕到 1
	if _roll_motion_seq >= 0xFFFFFFFF:
		_roll_motion_seq = 1
	else:
		_roll_motion_seq += 1
	m_seq = _roll_motion_seq


## operator bool（motionnode.hpp:116）
func is_valid() -> bool:
	return (type >= MotionType.MOTION_BEGIN and type < MotionType.MOTION_END) \
		or (type >= MotionType.MOTION_MON_BEGIN and type < MotionType.MOTION_MON_END) \
		or (type >= MotionType.MOTION_NPC_BEGIN and type < MotionType.MOTION_NPC_END) \
		or (type >= MotionType.MOTION_EXT_BEGIN and type < MotionType.MOTION_EXT_END)


## runTrigger（motionnode.hpp:125）
## 遍历触发器，返回 true 的被移除
func run_trigger() -> void:
	var i := 0
	while i < _trigger_list.size():
		var cb: Callable = _trigger_list[i]
		if cb.is_valid() and bool(cb.call(self)):
			_trigger_list.remove_at(i)
		else:
			i += 1


## addTrigger（motionnode.hpp:137）
func add_trigger(add_before: bool, cb: Callable) -> void:
	if not cb.is_valid():
		return
	if add_before:
		_trigger_list.push_front(cb)
	else:
		_trigger_list.push_back(cb)


func trigger_count() -> int:
	return _trigger_list.size()


## frameDelay（motionnode.hpp:164）
## 返回毫秒。speed 是「默认速度的百分比」，先夹到 [20,500] 再换算
func frame_delay() -> float:
	var s := clampi(speed, SysConstRef.SYS_MINSPEED, SysConstRef.SYS_MAXSPEED)
	return (1000.0 / float(SysConstRef.SYS_DEFFPS)) / (float(s) / 100.0)


## getSeqID（motionnode.hpp:169）
func get_seq_id() -> int:
	return (type << 48) | (m_seq << 16)


## getSeqFrameID（motionnode.hpp:174）
func get_seq_frame_id() -> int:
	return (type << 48) | (m_seq << 16) | frame


## gfxType（motionnode.hpp:179）
func gfx_type(frame_opt: int = -1) -> int:
	if type == MotionType.MOTION_EXT_COMBINED:
		var f: int = frame if frame_opt < 0 else frame_opt
		if f >= 0 and f < ext_combined_frames.size():
			return int(ext_combined_frames[f][0])
		return type
	return type


## gfxFrame（motionnode.hpp:189）
func gfx_frame(frame_opt: int = -1) -> int:
	if type == MotionType.MOTION_EXT_COMBINED:
		var f: int = frame if frame_opt < 0 else frame_opt
		if f >= 0 and f < ext_combined_frames.size():
			return int(ext_combined_frames[f][1])
		return f
	return frame if frame_opt < 0 else frame_opt


func to_str() -> String:
	return "MotionNode{%s dir=%d spd=%d (%d,%d)->(%d,%d) f=%d/%d}" % [
		MotionType.motion_name(type), direction, speed, x, y, end_x, end_y, frame, 0
	]


# ==================== 帧数表 ====================

## Hero::getFrameGfxSeq 的 count（hero.cpp:1281-1316）
## 【逐行翻译】只取 count 字段，原版还有 gfxType/gfxFrame 偏移表，
## 那些属于贴图选择（L0 但需 pngtexoffdb 配合），等 B9 渲染批次补
const FRAME_COUNT := {
	MotionType.MOTION_STAND: 4,
	MotionType.MOTION_ARROWATTACK: 6,
	MotionType.MOTION_SPELL0: 5,
	MotionType.MOTION_SPELL1: 5,
	MotionType.MOTION_HOLD: 1,
	MotionType.MOTION_PUSHBACK: 1,
	MotionType.MOTION_PUSHBACKFLY: 1,
	MotionType.MOTION_ATTACKMODE: 3,
	MotionType.MOTION_CUT: 2,
	MotionType.MOTION_ONEVSWING: 6,
	MotionType.MOTION_TWOVSWING: 6,
	MotionType.MOTION_ONEHSWING: 6,
	MotionType.MOTION_TWOHSWING: 6,
	MotionType.MOTION_SPEARVSWING: 6,
	MotionType.MOTION_SPEARHSWING: 6,
	MotionType.MOTION_HITTED: 3,
	MotionType.MOTION_WHEELWIND: 10,
	MotionType.MOTION_RANDSWING: 10,
	MotionType.MOTION_SPINKICK: 10,
	MotionType.MOTION_DIE: 10,
	MotionType.MOTION_ONHORSEDIE: 10,
	MotionType.MOTION_WALK: 6,
	MotionType.MOTION_RUN: 6,
	MotionType.MOTION_MOODEPO: 6,
	MotionType.MOTION_ROLL: 10,
	MotionType.MOTION_FISHSTAND: 4,
	MotionType.MOTION_FISHHAND: 3,
	MotionType.MOTION_FISHTHROW: 8,
	MotionType.MOTION_FISHPULL: 8,
	MotionType.MOTION_ONHORSESTAND: 4,
	MotionType.MOTION_ONHORSEWALK: 6,
	MotionType.MOTION_ONHORSERUN: 6,
	MotionType.MOTION_ONHORSEHITTED: 3,
}


## Hero::getFrameCount（hero.hpp:128）
func frame_count() -> int:
	if type == MotionType.MOTION_EXT_COMBINED:
		return ext_combined_frames.size()
	if not (type >= MotionType.MOTION_BEGIN and type < MotionType.MOTION_END):
		return 0
	if not (direction >= PathFinder.DIR_BEGIN and direction < PathFinder.DIR_END):
		return 0
	return int(FRAME_COUNT.get(type, 0))


## getFrameCountEx（clientcreature.hpp:132）
## 帧数为 0 视为非法动作；原版 throw，这里返回 0 由调用方判断
func frame_count_ex() -> int:
	return frame_count()


## updateMotion（clientcreature.hpp:149）
## 返回 true 表示动作继续有效（未结束）
## allow_switch_motion: 到末帧后是否允许切换下一动作
func update_motion(allow_switch_motion: bool) -> bool:
	if frame + 1 < frame_count_ex():
		return advance_frame()
	if allow_switch_motion:
		return false    ## 调用方负责切换动作
	return true         ## 停在末帧


## advanceMotionFrame（clientcreature.cpp:37）
func advance_frame() -> bool:
	var n := frame_count_ex()
	if n <= 0:
		return false
	frame = (frame + 1) % n
	run_trigger()
	return true


## moving（hero.cpp:1319）
func is_moving_motion() -> bool:
	return type == MotionType.MOTION_RUN \
		or type == MotionType.MOTION_WALK \
		or type == MotionType.MOTION_ONHORSERUN \
		or type == MotionType.MOTION_ONHORSEWALK
