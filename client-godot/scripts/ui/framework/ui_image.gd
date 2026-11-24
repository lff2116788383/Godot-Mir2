extends UiWidget
class_name UiImage
## 图片控件
##
## 【逐行翻译】client/src/imageboard.hpp + imageboard.cpp（235 行）
## 原版的 ImageBoard 支持：
##   - 一张贴图（texID）
##   - 一组贴图的动画（texAni，按 frame 选）
##   - 九宫格拉伸（drawTexture 的 src/dst 版本）
##   - 按 dir 的对齐方式
##
## Godot 差异：贴图由 tex_provider 回调提供，保持「贴图 id -> Texture2D」的解耦。

## 贴图 id（单张时用）
var tex_id: int = -1
## 动画帧（texAni 时用）：{ "ani_id": int, "frame": int }
var ani_id: int = -1
var ani_frame: int = 0
## 动画总帧数（用于自动播放）
var ani_count: int = 0
var ani_fps: float = 10.0
var _ani_acc := 0.0
var auto_animate := false

## 缩放（1.0 = 原尺寸）
var scale_x := 1.0
var scale_y := 1.0
## 调制色
var modulate_color: Color = Color.WHITE
## 对齐（DIR_*）
var align_dir: int = 1

## 贴图提供者：Callable(widget, tex_id, frame) -> Texture2D
var tex_provider: Callable = Callable()


func _init(p_name: String = "", p_tex_id: int = -1) -> void:
	widget_name = p_name
	tex_id = p_tex_id


## 当前应显示的贴图
func current_tex():
	if not tex_provider.is_valid():
		return null
	if ani_id >= 0:
		return tex_provider.call(self, ani_id, ani_frame)
	return tex_provider.call(self, tex_id, 0)


## 自动播放动画（对应 imageboard 的 ani 驱动）
func update_widget(dt: float) -> void:
	super.update_widget(dt)
	if not auto_animate or ani_count <= 1:
		return
	_ani_acc += dt
	var interval := 1.0 / maxf(ani_fps, 0.1)
	while _ani_acc >= interval:
		_ani_acc -= interval
		ani_frame = (ani_frame + 1) % ani_count


## 原始尺寸（缩放前）
func raw_size() -> Vector2i:
	var t = current_tex()
	if t == null:
		return Vector2i(0, 0)
	return Vector2i(t.get_width(), t.get_height())


func size_w() -> int:
	if w != null:
		return eval_size(w, self, 0)
	var rs := raw_size()
	return int(rs.x * scale_x)


func size_h() -> int:
	if h != null:
		return eval_size(h, self, 0)
	var rs := raw_size()
	return int(rs.y * scale_y)


## 绘制
func draw_self(canvas, my_roi) -> void:
	var t = current_tex()
	if t == null:
		return
	var dw := int(t.get_width() * scale_x)
	var dh := int(t.get_height() * scale_y)
	var ox := 0
	var oy := 0
	match align_dir:
		1:  ox = -dw / 2        # UP：水平居中
		2:  ox = -dw            # UPRIGHT：左下角
		3:  ox = -dw; oy = -dh / 2   # RIGHT：右中
		4:  ox = -dw; oy = -dh  # DOWNRIGHT：右下角
		5:  ox = -dw / 2; oy = -dh   # DOWN：上对齐居中
		6:  oy = -dh            # DOWNLEFT：右上角
		7:  oy = -dh / 2        # LEFT：左中
		_:  pass                # UPLEFT：左上角
	canvas.tex_draw_region(t, Rect2(0, 0, t.get_width(), t.get_height()),
		my_roi.x + ox, my_roi.y + oy, dw, dh, modulate_color)
