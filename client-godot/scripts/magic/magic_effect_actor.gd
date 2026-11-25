extends Node2D
class_name MagicEffectActor
## 魔法图形播放节点：MagicDB 选图 -> APNG 拆帧 -> 按 speed 播放

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

func setup(p_magic_id: int, p_gfx_entry: Dictionary, grid: Vector2i, dir: int, is_loop: bool = false) -> void:
        magic_id = p_magic_id
        gfx_entry = p_gfx_entry
        loop = is_loop
        z_index = 60
        position = Vector2(grid.x * GRID_X + GRID_X / 2, grid.y * GRID_Y + GRID_Y / 2)
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
                # 没有贴图时仍保留可见反馈
                var circle := Polygon2D.new()
                var pts := PackedVector2Array()
                for i in range(12):
                        pts.append(Vector2(cos(TAU * i / 12.0), sin(TAU * i / 12.0)) * 12.0)
                circle.polygon = pts
                circle.color = Color(0.4, 0.65, 1.0, 0.55)
                add_child(circle)

func _process(dt: float) -> void:
        acc += dt * 1000.0
        while acc >= frame_ms:
                acc -= frame_ms
                frame += 1
                if frames.is_empty():
                        if frame >= 4:
                                if loop: frame = 0
                                else: queue_free()
                                return
                elif frame >= frames.size():
                        if loop:
                                frame = 0
                        else:
                                queue_free()
                                return
                if not frames.is_empty():
                        sprite.texture = frames[frame]
