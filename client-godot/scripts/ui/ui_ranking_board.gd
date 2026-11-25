extends UiWidget
class_name UiRankingBoard
## 排行榜面板（SDRankingList）

const BG := Color(0.07, 0.07, 0.1, 0.96)
const BORDER := Color(0.5, 0.45, 0.3, 1.0)

var entries: Array = []
var rank_type: int = 0
var on_close: Callable = Callable()

func _init(p_name: String = "RankingBoard") -> void:
        widget_name = p_name
        w = 420
        h = 440
        show = false
        on_event = _handle_event

func set_canvas(_canvas) -> void:
        pass


func set_entries(p_type: int, p_entries: Array) -> void:
        rank_type = p_type
        entries = p_entries.duplicate()
        show = true

func draw_self(canvas, roi) -> void:
        canvas.draw_rect_c(BG, roi.x, roi.y, roi.w, roi.h, true)
        canvas.draw_border(BORDER, roi.x, roi.y, roi.w, roi.h, 2)
        var title := "等级排行" if rank_type == 1 else "金币排行"
        canvas.draw_string_c(title, roi.x + 12, roi.y + 8, Color(0.95, 0.85, 0.5), null, 16)
        canvas.draw_string_c("", roi.x + roi.w - 22, roi.y + 7, Color(0.9, 0.75, 0.5), null, 18)
        var heads := ["名次", "角色", "等级", "金币"]
        var cxs := [18, 80, 230, 300]
        var y: int = int(roi.y) + 40
        for i in range(heads.size()):
                canvas.draw_string_c(heads[i], roi.x + int(cxs[i]), y, Color(0.75, 0.75, 0.82), null, 12)
        y += 22
        for i in range(mini(entries.size(), 18)):
                var e: Dictionary = entries[i]
                canvas.draw_string_c(str(i + 1), roi.x + 18, y, Color(0.9, 0.85, 0.7), null, 12)
                canvas.draw_string_c(str(e.get("name", "?")), roi.x + 80, y, Color(0.9, 0.88, 0.82), null, 12)
                canvas.draw_string_c(str(e.get("level", 0)), roi.x + 230, y, Color(0.82, 0.82, 0.88), null, 12)
                canvas.draw_string_c(str(e.get("gold", 0)), roi.x + 300, y, Color(0.95, 0.84, 0.45), null, 12)
                y += 20

func _handle_event(_w, event: InputEvent, _valid: bool, roi) -> bool:
        if not show:
                return false
        if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
                if roi.contains(int(event.position.x), int(event.position.y)):
                        if event.position.x > roi.x + roi.w - 32 and event.position.y < roi.y + 32:
                                show = false
                                if on_close.is_valid(): on_close.call(self)
                        return true
        return false
