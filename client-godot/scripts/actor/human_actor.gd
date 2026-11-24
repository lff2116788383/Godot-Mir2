extends Node2D
class_name HumanActor
## 人形角色渲染（主角 / 其他玩家）
##
## 【逐行翻译】client/src/hero.cpp:76-150 的绘制顺序
##
## 原版绘制顺序（hero.cpp 中的链式调用）：
##   1. fnDrawWeapon(true)          —— 武器（身后层，如刀背）
##   2. 影子                        —— shadow 层，alpha 128
##   3. 身体 layer0                 —— 主体
##   4. 武器（若 order==0）          —— 身体前
##   5. 身体 layer1                 —— 覆盖层（如披风）
##   6. 头盔 或 头发                —— 有头盔则不画头发
##   7. fnDrawWeapon(false)         —— 武器（身前层）
##
## 每层都是一个 Sprite2D，通过 humanKey 查纹理。
## 外观数据来自 SDWLDesp（穿戴 + 发型）。

const HumanGfxRef := preload("res://scripts/actor/human_gfx.gd")
const MotionNodeRef2 := preload("res://scripts/action/motion_node.gd")
const UidRef := preload("res://scripts/core/uid.gd")

## 角色在屏幕上的锚点（脚底）
@export var anchor_offset := Vector2(0, 0)

## 外观
var dress: int = 0
var hair: int = 1
var helmet: int = 0
var weapon: int = 0
var hair_color: Color = Color.WHITE
var gender: bool = true

## 当前动作
var motion = null

## 各层 Sprite
var _spr_shadow: Sprite2D
var _spr_body0: Sprite2D
var _spr_body1: Sprite2D
var _spr_weapon_back: Sprite2D
var _spr_weapon_front: Sprite2D
var _spr_head: Sprite2D

var _assets_root: String = ""


func _ready() -> void:
	# 创建顺序即绘制顺序（Godot 按子节点顺序绘制）
	_spr_weapon_back = _make_spr("weapon", 0)
	_spr_shadow = _make_spr("shadow", 1)
	_spr_body0 = _make_spr("body0", 2)
	_spr_body1 = _make_spr("body1", 3)
	_spr_head = _make_spr("head", 4)
	_spr_weapon_front = _make_spr("weapon_front", 5)


func _make_spr(nm: String, z: int) -> Sprite2D:
	var s := Sprite2D.new()
	s.name = nm
	s.centered = false
	s.texture_filter = CanvasItem.TEXTURE_FILTER_NEAREST
	s.z_index = z
	s.visible = false
	add_child(s)
	return s


func setup(assets_root: String) -> void:
	_assets_root = assets_root


## 设置外观（对应 setWLDesp）
func set_appearance(p_dress: int, p_hair: int, p_helmet: int, p_weapon: int,
		p_gender: bool, p_hair_color: Color = Color.WHITE) -> void:
	dress = p_dress
	hair = p_hair
	helmet = p_helmet
	weapon = p_weapon
	gender = p_gender
	hair_color = p_hair_color
	refresh_textures()


## 设置当前动作
func set_motion(m) -> void:
	motion = m
	refresh_textures()


## 推进帧（由外部按 frameDelay 调用）
func advance_frame() -> bool:
	if motion == null:
		return false
	var n: int = motion.frame_count_ex()
	if n <= 0:
		return false
	motion.frame = (motion.frame + 1) % n
	refresh_textures()
	return true


## 按 hero.cpp:76-150 的顺序刷新所有层
func refresh_textures() -> void:
	if motion == null or _assets_root.is_empty():
		return

	var gfx_type: int = motion.gfx_type()
	var gfx_frame: int = motion.gfx_frame()
	var gfx_dir: int = motion.direction

	# ---- 1. 影子（dress 的 shadow 层，alpha 128）----
	var dress_id := HumanGfxRef.gfx_dress_id(dress, gfx_type, gfx_dir)
	_apply(_spr_shadow, "hero", dress_id, true, 0, gfx_frame, Color(1, 1, 1, 0.5))

	# ---- 2. 身体 layer0 / layer1 ----
	_apply(_spr_body0, "hero", dress_id, false, 0, gfx_frame, Color.WHITE)
	_apply(_spr_body1, "hero", dress_id, false, 1, gfx_frame, Color.WHITE)

	# ---- 3. 头盔 或 头发（有头盔不画头发）----
	if helmet >= ProtocolDef.WEAPON_BEGIN:
		var hid := HumanGfxRef.gfx_helmet_id(helmet, gfx_type, gfx_dir)
		_apply(_spr_head, "helmet", hid, false, 0, gfx_frame, Color.WHITE)
	else:
		var hairid := HumanGfxRef.gfx_hair_id(hair, gfx_type, gfx_dir)
		_apply(_spr_head, "hair", hairid, false, 0, gfx_frame, hair_color)

	# ---- 4. 武器（身前身后两层）----
	if weapon >= ProtocolDef.WEAPON_BEGIN:
		var wid := HumanGfxRef.gfx_weapon_id(weapon, gfx_type, gfx_dir)
		_apply(_spr_weapon_front, "weapon", wid, false, 0, gfx_frame, Color.WHITE)
		# glow 层（weapon 的 layer1）
		_apply(_spr_weapon_back, "weapon", wid, false, 1, gfx_frame, Color.WHITE)
	else:
		_spr_weapon_front.visible = false
		_spr_weapon_back.visible = false


func _apply(spr: Sprite2D, layer_dir: String, gfx_id: int, shadow: bool,
		layer: int, frame: int, modulate_color: Color) -> void:
	if gfx_id < 0:
		spr.visible = false
		return
	var r := HumanGfxRef.find_texture(_assets_root, layer_dir, gfx_id, gender, shadow, layer, frame)
	if r.is_empty():
		spr.visible = false
		return
	var tex := TexCache.get_texture(str(r["path"]))
	if tex == null:
		spr.visible = false
		return
	spr.texture = tex
	spr.visible = true
	spr.modulate = modulate_color
	# 原版：drawTexture(tex, startX + dx, startY + dy)
	# dx/dy 是相对脚底锚点的偏移，且是负向（向左上）
	var dx: int = int(r["dx"])
	var dy: int = int(r["dy"])
	# 文件名里的符号位为 0 表示负，1 表示正
	spr.position = Vector2(-dx if dx > 0 else 0, -dy if dy > 0 else 0) + anchor_offset


func set_facing_dir(d: int) -> void:
	if motion != null:
		motion.direction = d
		refresh_textures()


func to_str() -> String:
	return "HumanActor{dress=%d hair=%d helmet=%d weapon=%d}" % [dress, hair, helmet, weapon]


const ProtocolDef := preload("res://scripts/core/protocol_def.gd")
