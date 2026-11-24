extends Node
## 全局游戏状态：当前地图、玩家、实体表
## 只存数据，不做渲染

signal map_changed(map_uid: int)
signal self_uid_changed(uid: int)

const SYS_MAPGRIDXP := 48
const SYS_MAPGRIDYP := 32
const SYS_PLY_VIEWR := 15

## 素材根目录，可用环境变量 MIR2X_ASSETS 覆盖
var assets_root: String = ""

var self_uid: int = 0
var self_name: String = ""
var current_map_uid: int = 0
var current_map_grid := Vector2i.ZERO

## uid -> 实体数据字典
var actors: Dictionary = {}


func _ready() -> void:
	assets_root = OS.get_environment("MIR2X_ASSETS")
	if assets_root.is_empty():
		assets_root = "E:/Desktop/game/mir2x_assets-main"
	print("[Game] assets_root = ", assets_root, " exists=", DirAccess.dir_exists_absolute(assets_root))


func map_texture_path(tex_id: int) -> String:
	return "%s/texture/map/%08X.PNG" % [assets_root, tex_id]


func set_self(uid: int, pname: String) -> void:
	self_uid = uid
	self_name = pname
	self_uid_changed.emit(uid)
