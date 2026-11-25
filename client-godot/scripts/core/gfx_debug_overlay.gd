extends Node2D
class_name GfxDebugOverlay
## P2 调试层：F3 显示网格、实体碰撞框、坐标与缓存统计

var enabled := false
var player = null
var co_manager = null
var obj_layer = null
var font: Font = ThemeDB.fallback_font
var hover_grid := Vector2i(-1, -1)
var hover_hit = null
var hover_items: Array = []

func setup(p_player, p_co_manager, p_obj_layer) -> void:
        player = p_player
        co_manager = p_co_manager
        obj_layer = p_obj_layer

func toggle() -> void:
        enabled = not enabled
        visible = enabled
        queue_redraw()

func _process(_dt: float) -> void:
        if enabled:
                queue_redraw()

func _draw() -> void:
        if not enabled:
                return
        var cam := get_viewport().get_camera_2d()
        if cam == null:
                return
        var center := cam.get_screen_center_position()
        var vp := get_viewport_rect().size / cam.zoom
        var x0 := int(center.x - vp.x * 0.5)
        var y0 := int(center.y - vp.y * 0.5)
        var x1 := int(center.x + vp.x * 0.5)
        var y1 := int(center.y + vp.y * 0.5)
        var grid_col := Color(0.2, 0.9, 0.2, 0.22)
        var gx0 := int(floor(x0 / 48.0))
        var gx1 := int(ceil(x1 / 48.0))
        var gy0 := int(floor(y0 / 32.0))
        var gy1 := int(ceil(y1 / 32.0))
        for gx in range(gx0, gx1 + 1):
                draw_line(Vector2(gx * 48, y0), Vector2(gx * 48, y1), grid_col, 1.0)
        for gy in range(gy0, gy1 + 1):
                draw_line(Vector2(x0, gy * 32), Vector2(x1, gy * 32), grid_col, 1.0)
        if co_manager != null:
                for uid in co_manager.all_uids():
                        var node = co_manager.find_uid(uid)
                        if node == null:
                                continue
                        var p: Vector2 = node.position
                        draw_rect(Rect2(p - Vector2(18, 22), Vector2(36, 44)), Color(1, 0.4, 0.2, 0.8), false, 1.0)
                        draw_string(font, p + Vector2(-24, -32), "0x%X" % int(uid),
                                HORIZONTAL_ALIGNMENT_LEFT, -1, 10, Color(1, 1, 0.6))
        if player != null:
                var pp: Vector2 = player.position
                draw_rect(Rect2(pp - Vector2(20, 26), Vector2(40, 52)), Color(0.2, 1.0, 0.2, 1.0), false, 2.0)
                draw_string(font, Vector2(x0 + 12, y0 + 24),
                        "grid=(%d,%d) entities=%d fps=%d" % [player.grid.x, player.grid.y,
                        co_manager.count() if co_manager != null else 0, Engine.get_frames_per_second()],
                        HORIZONTAL_ALIGNMENT_LEFT, -1, 14, Color(0.8, 1.0, 0.8))
