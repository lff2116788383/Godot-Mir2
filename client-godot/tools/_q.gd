extends SceneTree
const H := preload("res://scripts/actor/human_gfx.gd")
func _init() -> void:
	var gid = H.gfx_dress_id(0, 35-34, 1)   # dress=0, MOTION_STAND=1 -> gm=0, dir=1 -> 0
	print("gid=", gid, " dress=", (gid>>9)&0xFF, " motion=", (gid>>3)&0x3F, " dir=", gid&0x7)
	var r = H.find_texture("E:/Desktop/game/mir2x_assets-main", "hero", gid, false, false, 0, 0)
	print("found=", r)
	quit(0)
