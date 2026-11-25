extends Node2D
class_name MagicEffectActor
## 魔法图形播放节点：MagicDB 选图 -> APNG 拆帧 -> 按 speed 播放
## P2：支持固定/跟随/附着、起点到目标飞行、朝向旋转和循环生命周期

const MagicDBRef := preload("res://scripts/magic/magic_db.gd")
const ApngLoaderRef := preload("res://scripts/data/apng_loader.gd")
const GRID_X := 48
const GRID_Y := 32

var magic_id := 0
var gfx_entry: Dictionary = {}
var frames: Array = []
var frame := 0
var acc := 0.0
var frame_ms := 80.0
var loop := false
var sprite: Sprite2D

var _start_grid := Vector2i.ZERO
var _aim_grid := Vector2i.ZERO
var _follow_node = null
var _entry_type := "固定"
var _travel := false
var _attach := false
var _travel_t := 0.0
var _travel_duration := 0.35
var _life_ms := 0.0
var _dead := false

func setup(p_magic_id: int, p_gfx_entry: Dictionary, grid: Vector2i,
                dir: int, is_loop: bool = false,
                start_grid: Vector2i = Vector2i(-1, -1), follow_node = null) -> void:
        magic_id = p_magic_id
        gfx_entry = p_gfx_entry
        loop = is_loop
        _aim_grid = grid
        _start_grid = start_grid if start_grid.x >= 0 else grid
        _follow_node = follow_node
        _entry_type = str(gfx_entry.get("type", "固定"))
        _travel = _entry_type == "跟随"
        _attach = _entry_type == "附着"
        _travel_duration = maxf(0.2, float(gfx_entry.get("frameCount", 4)) * 0.08)
        z_index = 60
        position = _grid_center(_start_grid if _travel else _aim_grid)
        sprite = Sprite2D.new()
        sprite.centered = true
        sprite.texture_filter = CanvasItem.TEXTURE_FILTER_NEAREST
        add_child(sprite)
        var speed := float(gfx_entry.get("speed", 100))
        frame_ms = maxf(20.0, 120.0 * 100.0 / maxf(1.0, speed))
        var gfx_id := int(gfx_entry.get("gfxID", -1))
        var path := ""
        var root := OS.get_environment("MIR2X_ASSETS")
        if root.is_empty():
                root = "E:/Desktop/game/mir2x_assets-main"
        if gfx_id >= 0:
                path = MagicDBRef.find_file(root, gfx_id, dir)
                if not path.is_empty():
                        frames = ApngLoaderRef.load_frames("%s/texture/magic/%s" % [root, path])
        if not frames.is_empty():
                sprite.texture = frames[0]
                var info := MagicDBRef.parse_file_name(path)
                sprite.offset = Vector2(-info.dx, -info.dy)
        else:
                var circle := Polygon2D.new()
                var pts := PackedVector2Array()
                for i in range(12):
                        pts.append(Vector2(cos(TAU * i / 12.0), sin(TAU * i / 12.0)) * 12.0)
                circle.polygon = pts
                circle.color = Color(0.4, 0.65, 1.0, 0.55)
                add_child(circle)
        _update_travel(0.0)

func _grid_center(g: Vector2i) -> Vector2:
        return Vector2(g.x * GRID_X + GRID_X / 2, g.y * GRID_Y + GRID_Y / 2)

func _target_position() -> Vector2:
        if _follow_node != null and is_instance_valid(_follow_node):
                var node: Node2D = _follow_node
                return node.position + Vector2(0, -16)
        return _grid_center(_aim_grid)

func _update_travel(dt: float) -> void:
        if _attach or _travel:
                position = _target_position()
        if _travel:
                _travel_t = clampf(_travel_t + dt / _travel_duration, 0.0, 1.0)
                var target := _target_position()
                position = _grid_center(_start_grid).lerp(target, _travel_t)
                var delta := target - position
                if delta.length_squared() > 1.0:
                        rotation = delta.angle() + PI * 0.5

func _process(dt: float) -> void:
        if _dead:
                return
        _life_ms += dt * 1000.0
        if loop and _life_ms >= 4000.0:
                queue_free()
                _dead = true
                return
        _update_travel(dt)
        acc += dt * 1000.0
        while acc >= frame_ms:
                acc -= frame_ms
                frame += 1
                if frames.is_empty():
                        if frame >= 4:
                                if loop:
                                        frame = 0
                                else:
                                        queue_free()
                                        _dead = true
                                        return
                elif frame >= frames.size():
                        if loop:
                                frame = 0
                        else:
                                queue_free()
                                _dead = true
                                return
                if not frames.is_empty():
                        sprite.texture = frames[frame]
