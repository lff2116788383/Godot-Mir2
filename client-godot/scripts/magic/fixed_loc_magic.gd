extends "res://scripts/magic/base_magic.gd"
class_name FixedLocMagic
## 固定位置型魔法（火墙、火焰灰烬、瞬息移动等）
##
## 【逐行翻译】client/src/fixedlocmagic.hpp（401 行）
##
## 这类魔法不移动，只按时间推进帧与透明度。
## 子类通过 override abs_frame() / done() 改变播放节奏。

const SysConstRef3 := preload("res://scripts/core/sys_const.gd")

var x: int = 0
var y: int = 0


func setup_fixed(magic_id_v: int, magic_record_v: Dictionary, gfx_entry_v: Dictionary,
		gfx_entry_ref_v, seff_id_v: int, dir_index: int, frame_count: int,
		px: int, py: int) -> bool:
	if not setup(magic_id_v, magic_record_v, gfx_entry_v, gfx_entry_ref_v, seff_id_v, dir_index, frame_count):
		return false
	x = px
	y = py
	return true


## "固定" 类型校验（fixedlocmagic.hpp:30）
func is_fixed_type() -> bool:
	return str(gfx_entry.get("type", "")) == "固定"


func location() -> Vector2i:
	return Vector2i(x / SysConstRef3.SYS_MAPGRIDXP, y / SysConstRef3.SYS_MAPGRIDYP)


# ==================== FireAshEffect_RUN ====================
## 【逐行翻译】fixedlocmagic.hpp:45-110
## 火焰灰烬：随机旋转角、随机帧偏移、三段透明度（淡入/持续/淡出）

class FireAshEffect extends FixedLocMagic:
	var rotate: int = 0
	var abs_frame_off: int = 0
	## 三段时长：[淡入, 持续, 淡出]
	var alpha_time: Array[int] = [2000, 5000, 3000]

	func setup_ash(magic_id_v: int, magic_record_v: Dictionary, gfx_entry_v: Dictionary,
			gfx_entry_ref_v, seff_id_v: int, frame_count: int,
			px: int, py: int, t1: int = 2000, t2: int = 5000, t3: int = 3000) -> bool:
		# 原版用 std::rand() % 5 作为 gfxDirIndex
		var dir_idx := randi() % 5
		if not setup_fixed(magic_id_v, magic_record_v, gfx_entry_v, gfx_entry_ref_v,
				seff_id_v, dir_idx, frame_count, px, py):
			return false
		rotate = randi() % 360
		abs_frame_off = randi() % 10
		alpha_time = [t1, t2, t3]
		return t1 > 0 and t2 >= 0 and t3 > 0

	## absFrame（fixedlocmagic.hpp:75）
	func abs_frame() -> int:
		return super.abs_frame() + abs_frame_off

	## done（fixedlocmagic.hpp:80）
	func done() -> bool:
		return m_accu_time >= float(alpha_time[0] + alpha_time[1] + alpha_time[2])

	## getPlainModColor（fixedlocmagic.hpp:85）
	## 返回 0-255 的 alpha 值，调用方自行转 Color
	func plain_alpha() -> int:
		var a := 0.0
		var t0 := float(alpha_time[0])
		var t1 := float(alpha_time[1])
		var t2 := float(alpha_time[2])
		if m_accu_time < t0:
			a = m_accu_time / t0
		elif m_accu_time < t0 + t1:
			a = 1.0
		elif m_accu_time < t0 + t1 + t2:
			a = 1.0 - (m_accu_time - t0 - t1) / t2
		else:
			a = 0.0
		return int(round(255.0 * a))


# ==================== FireWall_RUN ====================
## 【逐行翻译】fixedlocmagic.hpp:112+
## 火墙：有淡入淡出阶段

class FireWall extends FixedLocMagic:
	var abs_frame_off: int = 0
	var fade_duration: int = 0
	var fade_start_time: float = 0.0

	func setup_wall(magic_id_v: int, magic_record_v: Dictionary, gfx_entry_v: Dictionary,
			gfx_entry_ref_v, seff_id_v: int, frame_count: int,
			px: int, py: int, frame_off: int = 0) -> bool:
		if not setup_fixed(magic_id_v, magic_record_v, gfx_entry_v, gfx_entry_ref_v,
				seff_id_v, 0, frame_count, px, py):
			return false
		abs_frame_off = frame_off
		return true

	## absFrame override
	func abs_frame() -> int:
		return super.abs_frame() + abs_frame_off


# ==================== 通用固定魔法 ====================
## 普通固定位置魔法（无特殊节奏）
class PlainFixedLoc extends FixedLocMagic:
	pass
