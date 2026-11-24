extends "res://scripts/magic/base_magic.gd"
class_name AttachMagic
## 附着型魔法（雷电术、阴阳法环、治疗等）
##
## 【逐行翻译】client/src/attachmagic.hpp（60 行）
##
## 这类魔法附着在角色身上，跟随角色位置绘制，本身不移动。
## 与原版的差异：原版子类各自重写 drawShift 处理偏移，这里只保留类型标识，
## 具体偏移由渲染层（B9）按 gfxEntry 数据计算。

const SysConstRef4 := preload("res://scripts/core/sys_const.gd")


func setup_attach(magic_id_v: int, magic_record_v: Dictionary, gfx_entry_v: Dictionary,
		gfx_entry_ref_v, seff_id_v: int, dir_index: int = 0, frame_count: int = 0) -> bool:
	if not setup(magic_id_v, magic_record_v, gfx_entry_v, gfx_entry_ref_v, seff_id_v, dir_index, frame_count):
		return false
	# attachmagic.hpp:18 —— 必须是「附着」类型
	return str(gfx_entry.get("type", "")) == "附着"


func is_attach_type() -> bool:
	return str(gfx_entry.get("type", "")) == "附着"


# ==================== 特定附着魔法 ====================
## 【逐行翻译】attachmagic.hpp:23-60
## 这些子类的差别只在于用哪个 name/stage 构造，以及 drawShift 的偏移

## 雷电术（attachmagic.hpp:23）
class Thunderbolt extends AttachMagic:
	const MAGIC_NAME := "雷电术"
	const STAGE := "运行"


## 阴阳法环（attachmagic.hpp:42）
class TaoYellowBlueRing extends AttachMagic:
	const MAGIC_NAME := "阴阳法环"
	const STAGE := "持续"


## 蚂蚁道士治疗（attachmagic.hpp:50）
class AntHealing extends AttachMagic:
	const MAGIC_NAME := "蚂蚁道士_治疗"
	const STAGE := "运行"
