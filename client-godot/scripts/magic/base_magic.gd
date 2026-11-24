extends RefCounted
class_name BaseMagic
## 魔法基类
##
## 【逐行翻译】client/src/basemagic.hpp（197 行）+ basemagic.cpp（44 行）
##
## 原版关键设计（basemagic.hpp:95-120 的注释）：
##   update() 返回 bool 而不是「先 update 再 done()」，
##   因为 update 内部 done() 可能返回 false，之后立刻变 true，
##   会导致 onDone 回调被漏掉。所以统一用 update 的返回值决定是否移除。

const SysConstRef := preload("res://scripts/core/sys_const.gd")

## 魔法 ID（对应 m_magicID）
var magic_id: int = 0
## 魔法记录（对应 m_magicRecord，来自 magicrecord.json）
var magic_record: Dictionary = {}
## 图形条目（对应 MagicGfxEntry）
var gfx_entry: Dictionary = {}
## 图形条目引用（对应 MagicGfxEntryRef，可为空）
var gfx_entry_ref = null
## 音效 ID
var seff_id: int = 0
## 图形方向索引
var gfx_dir_index: int = 0
## 图形帧数
var gfx_frame_count: int = 0

## 累计时间（毫秒）
var m_accu_time: float = 0.0

var _on_done_cbs: Array[Callable] = []
var _on_update_cbs: Array[Callable] = []


## 初始化：对应 BaseMagic::BaseMagic（basemagic.cpp:6）
## magic_record_v / gfx_entry_v 由调用方从数据表查好传入，
## 原版在构造函数里 throw 校验，这里改为返回是否成功
func setup(magic_id_v: int, magic_record_v: Dictionary, gfx_entry_v: Dictionary,
		gfx_entry_ref_v, seff_id_v: int, dir_index: int, frame_count: int) -> bool:
	magic_id = magic_id_v
	magic_record = magic_record_v
	gfx_entry = gfx_entry_v
	gfx_entry_ref = gfx_entry_ref_v
	seff_id = seff_id_v
	gfx_dir_index = dir_index
	gfx_frame_count = frame_count

	# basemagic.cpp:38-42 的断言
	if gfx_dir_index < 0:
		return false
	if gfx_dir_index >= int(gfx_entry.get("gfxDirType", 1)):
		return false
	# 无 gfx 的条目可以没有帧数（由子类决定何时结束）
	var has_gfx := is_gfx_valid()
	if gfx_frame_count <= 0 and not (not has_gfx and not bool(gfx_entry.get("loop", false))):
		return false
	return true


## magicName（basemagic.hpp:58）
func magic_name() -> String:
	return str(magic_record.get("name", ""))


## gfxSeqID（basemagic.hpp:73）
## 无 gfx 返回 -1（对应 SYS_U32NIL）
func gfx_seq_id() -> int:
	if int(gfx_entry.get("gfxID", SysConstRef.SYS_U32NIL)) == SysConstRef.SYS_U32NIL:
		return -1
	# 具体 seqID 由 MagicDB 计算（B9 随纹理批次补）
	return int(gfx_entry.get("gfxID", 0))


func is_gfx_valid() -> bool:
	return int(gfx_entry.get("gfxID", SysConstRef.SYS_U32NIL)) != SysConstRef.SYS_U32NIL


## checkMagic（basemagic.hpp:84）
func check_magic(name: String) -> bool:
	return magic_name() == name


func check_magic_stage(name: String, stage: String) -> bool:
	if magic_name() != name:
		return false
	return str(gfx_entry.get("stage", "")) == stage


## update（basemagic.hpp:122）
## 返回 true 表示已完成，调用方应移除
func update(update_time_ms: float) -> bool:
	m_accu_time += update_time_ms
	_run_on_update()
	if done():
		_run_on_done()
		return true
	return false


## frame（basemagic.hpp:139）
func frame() -> int:
	if bool(gfx_entry.get("loop", false)):
		if gfx_frame_count <= 0:
			return 0
		return abs_frame() % gfx_frame_count
	return abs_frame()


## absFrame（basemagic.hpp:150）
func abs_frame() -> int:
	if gfx_frame_count <= 0:
		return 0
	var speed := float(gfx_entry.get("speed", 100))
	var f := (m_accu_time / 1000.0) * float(SysConstRef.SYS_DEFFPS) * (speed / 100.0)
	return int(round(f))


## addOnDone（basemagic.hpp:155）
func add_on_done(cb: Callable) -> BaseMagic:
	if cb.is_valid():
		_on_done_cbs.append(cb)
	return self


## addTrigger（basemagic.hpp:161）
func add_trigger(cb: Callable) -> BaseMagic:
	if cb.is_valid():
		_on_update_cbs.append(cb)
	return self


## done（basemagic.hpp:167）
func done() -> bool:
	if bool(gfx_entry.get("loop", false)):
		return false
	return abs_frame() >= gfx_frame_count


func _run_on_done() -> void:
	for cb in _on_done_cbs:
		if cb.is_valid():
			cb.call(self)
	_on_done_cbs.clear()


func _run_on_update() -> void:
	var i := 0
	while i < _on_update_cbs.size():
		var cb: Callable = _on_update_cbs[i]
		if cb.is_valid() and bool(cb.call(self)):
			_on_update_cbs.remove_at(i)
		else:
			i += 1
