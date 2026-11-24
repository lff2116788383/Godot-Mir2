extends SceneTree
const MF := preload("res://scripts/actor/monster_frames.gd")
const MT := preload("res://scripts/action/motion_type.gd")
func _init() -> void:
	for t in [MT.MOTION_MON_STAND, MT.MOTION_MON_WALK, MT.MOTION_MON_ATTACK0, MT.MOTION_MON_DIE]:
		print("motion %d -> count=%d  seq=%s" % [t, int(MF.frame_info("", t, false).get("count", -1)),
			str(MF.frame_info("", t, false))])
	quit(0)
