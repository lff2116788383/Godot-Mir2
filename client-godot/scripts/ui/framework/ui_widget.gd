extends Node2D
class_name UiWidget
## UI 控件基类
##
## 【逐行翻译】client/src/widget.hpp（585 行）+ widget.cpp（1194 行）
## 对照：
##   - widget.hpp:340-360  InstAttrs：show/active/focus + 四个回调
##   - widget.cpp:646-674  draw / drawRoot / drawChild / drawAsChild
##   - widget.cpp:693       drawDefault —— 逐个绘制子控件
##   - widget.cpp:722-760   w()/h() —— 未指定尺寸时按子控件包住计算
##   - widget.roi.hpp       ROIMap 逐层裁剪
##
## ── C++ 模板 vs GDScript 的映射 ──
## 原版 VarInt = variant<int, function<int()>, function<int(const Widget*)>, ...>
## 即「一个值可以写死，也可以是个函数，每次求值时算」。
##
## GDScript 的 Callable 可以直接扛这个语义：
##   - 传 int       -> 固定值
##   - 传 Callable  -> 每次求值时调用（参数 (widget)，可忽略）
## 用 _eval_int() 等辅助函数统一取值，调用方无需关心是哪种。
##
## ── 坐标系 ──
## 原版：控件自身不带绝对坐标，位置由父控件的 drawAsChild 递归加出来。
## 这里为了复用 Godot 的 SceneTree（变换、可见性、输入），
## 采用 position 相对父节点的做法，但绘制仍然走 ROI 裁剪（draw_self_roi）。

# ==================== 属性（对应 InstAttrs） ====================

## 对应 VarBool show —— 可为 bool 或 Callable
var show = true
## 对应 VarBool active
var active = true
## 是否响应焦点（原版 focus / moveOnFocus）
var focus := false
var move_on_focus := true

## 尺寸。null 表示按子控件包住自动计算（原版 VarSizeOpt）
var w = null
var h = null

## 对齐方向（DIR_UPLEFT = 左上角对齐）
var dir_align: int = 1   ## PathFinder.DIR_UPLEFT

## 名称（原版 name，用于 getWidget 查找）
var widget_name: String = ""

# ==================== 回调（对应 InstAttrs 四个函数） ====================

## void draw(const Widget *, ROIMap)
var on_draw: Callable = Callable()
## void update(Widget *, double)
var on_update: Callable = Callable()
## bool processEvent(Widget *, InputEvent, bool, ROIMap)
var on_event: Callable = Callable()
## void afterResize(Widget *)
var on_after_resize: Callable = Callable()

# ==================== 子控件 ====================

## 子控件列表（保持插入顺序 = 绘制顺序）
var _children: Array[UiWidget] = []
## 名字索引
var _name_index: Dictionary = {}


# ==================== 属性求值（对应 evalInt/evalSize/evalBool） ====================

## 对应 Widget::evalInt / evalSize
## 支持：int / float / Callable(widget) / null
static func eval_size(v, self_widget: UiWidget, fallback: int = 0) -> int:
	if v == null:
		return fallback
	if v is int:
		return v
	if v is float:
		return int(v)
	if v is Callable:
		if v.get_argument_count() == 0:
			return int(v.call())
		return int(v.call(self_widget))
	return fallback


## 对应 Widget::evalBool
static func eval_bool(v, self_widget: UiWidget, fallback: bool = true) -> bool:
	if v == null:
		return fallback
	if v is bool:
		return v
	if v is Callable:
		if v.get_argument_count() == 0:
			return bool(v.call())
		return bool(v.call(self_widget))
	return fallback


func is_shown() -> bool:
	return eval_bool(show, self, true)


func is_active() -> bool:
	return eval_bool(active, self, true)


# ==================== 尺寸（对应 Widget::w() / h()） ====================

## widget.cpp:722  int Widget::w() const
## 未指定 w 时，按「所有子控件覆盖的最大宽度」计算
func size_w() -> int:
	if w != null:
		return eval_size(w, self, 0)
	return max_child_cover_w()


## widget.cpp:728  int Widget::h() const
func size_h() -> int:
	if h != null:
		return eval_size(h, self, 0)
	return max_child_cover_h()


## widget.cpp:735  maxChildCoverWExcept
func max_child_cover_w() -> int:
	var mx := 0
	for c in _children:
		if not c.is_shown():
			continue
		if not c.is_local_shown():
			continue
		mx = maxi(mx, c.position.x + c.size_w())
	return mx


func max_child_cover_h() -> int:
	var mx := 0
	for c in _children:
		if not c.is_shown():
			continue
		if not c.is_local_shown():
			continue
		mx = maxi(mx, c.position.y + c.size_h())
	return mx


## 自身及所有祖先都 show（原版 localShow 的递归含义）
func is_local_shown() -> bool:
	if not is_shown():
		return false
	var p := get_parent()
	while p != null:
		if p is UiWidget and not p.is_shown():
			return false
		p = p.get_parent()
	return true


# ==================== 控件树（对应 WidgetTreeNode） ====================

## widget.hpp:200  addChild
## dir/x/y 决定本控件相对父控件的位置
func add_widget(child: UiWidget, p_x = 0, p_y = 0) -> UiWidget:
	if child == null:
		return null
	_children.append(child)
	if not child.widget_name.is_empty():
		_name_index[child.widget_name] = child
	add_child(child)
	# 位置在 addChild 之后再设，保证节点已在树上
	child.position = Vector2(eval_size(p_x, child, 0), eval_size(p_y, child, 0))
	child.z_index = _children.size()
	return child


## widget.cpp  removeChild
func remove_widget(child: UiWidget) -> void:
	if child == null:
		return
	_children.erase(child)
	if not child.widget_name.is_empty():
		_name_index.erase(child.widget_name)
	if is_instance_valid(child):
		child.queue_free()


func clear_widgets() -> void:
	for c in _children.duplicate():
		remove_widget(c)


func child_count() -> int:
	return _children.size()


func get_children_widgets() -> Array[UiWidget]:
	return _children.duplicate()


## 对应 Widget::getWidget(name)
func find_widget(nm: String) -> UiWidget:
	if _name_index.has(nm):
		return _name_index[nm]
	for c in _children:
		var r := c.find_widget(nm)
		if r != null:
			return r
	return null


func has_widget(nm: String) -> bool:
	return find_widget(nm) != null


# ==================== 绘制（对应 draw / drawDefault / drawAsChild） ====================

## widget.cpp:646  void Widget::draw(ROIMap) const
## 有 on_draw 就调它，否则走默认（画子控件）
func draw_roi(parent_roi: UiRoi) -> void:
	if not is_shown():
		return
	var my := child_roi(parent_roi)
	if my.empty():
		return

	if on_draw.is_valid():
		on_draw.call(self, my)
	else:
		draw_default(my)


## widget.cpp:693  void Widget::drawDefault(ROIMap) const
## 默认实现：把所有可见子控件画一遍
func draw_default(my_roi: UiRoi) -> void:
	for c in _children:
		if c.is_shown():
			c.draw_roi(my_roi)


## widget.cpp:673  void Widget::drawAsChild
## 子控件的 ROI = 父 ROI 与「子控件自身矩形」求交
func child_roi(parent_roi: UiRoi) -> UiRoi:
	var r := UiRoi.new(
		parent_roi.x + int(position.x),
		parent_roi.y + int(position.y),
		size_w(),
		size_h())
	return r.crop(parent_roi)


## 根节点绘制入口（对应 drawRoot）
func draw_root(screen_roi: UiRoi) -> void:
	draw_roi(screen_roi)


# ==================== 更新与事件 ====================

## 每帧更新（对应 Widget::update）
func update_widget(dt: float) -> void:
	if not is_active():
		return
	if on_update.is_valid():
		on_update.call(self, dt)
	for c in _children:
		c.update_widget(dt)


## 事件处理（对应 processEvent）
## 返回 true 表示事件已被消费，不再向父/兄弟传递
func process_event(event: InputEvent, my_roi: UiRoi) -> bool:
	if not is_active() or not is_shown():
		return false

	# 让子控件先处理（后添加的在上面，优先响应）
	var i := _children.size() - 1
	while i >= 0:
		var c := _children[i]
		if c.is_shown():
			var cri := c.child_roi(my_roi)
			if not cri.empty():
				if c.process_event(event, my_roi):
					return true
		i -= 1

	if on_event.is_valid():
		return bool(on_event.call(self, event, false, my_roi))
	return false


## 尺寸变化回调（对应 afterResize）
func after_resize() -> void:
	if on_after_resize.is_valid():
		on_after_resize.call(self)
	for c in _children:
		c.after_resize()


# ==================== 调试 ====================

func dump_tree(indent: String = "") -> void:
	print("%s%s(%s) pos=%s size=%dx%d show=%s" % [
		indent, widget_name if not widget_name.is_empty() else "Widget",
		get_script().resource_path.get_file() if get_script() != null else "?",
		str(position), size_w(), size_h(), str(is_shown())])
	for c in _children:
		c.dump_tree(indent + "  ")
