extends SceneTree
## UI 框架自检
##
## 验证：
##   1. UiRoi 的裁剪语义（对应 widget.roi.hpp crop）
##   2. 控件树增删查
##   3. 尺寸自动计算（对应 widget.cpp:722-745 w()/h()）
##   4. 子控件 ROI 逐层收紧
##   5. 按钮三态机（对应 buttonbase.cpp:51-130）
##   6. 滑块值域与步长
##   7. 复选框/radio 语义
##   8. 输入框编辑
##   9. 滚动容器滚动量

const RoiRef := preload("res://scripts/ui/framework/ui_roi.gd")
const WidgetRef := preload("res://scripts/ui/framework/ui_widget.gd")
const ButtonRef := preload("res://scripts/ui/framework/ui_button.gd")
const LabelRef := preload("res://scripts/ui/framework/ui_label.gd")
const CheckboxRef := preload("res://scripts/ui/framework/ui_checkbox.gd")
const SliderRef := preload("res://scripts/ui/framework/ui_slider.gd")
const InputRef := preload("res://scripts/ui/framework/ui_input_line.gd")
const ScrollRef := preload("res://scripts/ui/framework/ui_scroll.gd")

var _pass := 0
var _fail := 0


func _ok(cond: bool, name: String, extra: String = "") -> void:
	if cond:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, ("  " + extra) if extra else "")


func _init() -> void:
	print("=== UI 框架自检 ===")
	_test_roi()
	_test_widget_tree()
	_test_size()
	_test_button()
	_test_slider()
	_test_checkbox()
	_test_input()
	_test_scroll()
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0)


# ==================== ROI ====================

func _test_roi() -> void:
	print("\n[1] UiRoi 裁剪")
	var r := RoiRef.new(10, 20, 100, 50)
	_ok(r.x == 10 and r.y == 20 and r.w == 100 and r.h == 50, "构造正确")
	_ok(not r.empty(), "非空")
	_ok(r.contains(50, 40), "包含内部点")
	_ok(not r.contains(5, 40), "不包含左侧外点")
	_ok(not r.contains(110, 40), "不包含右侧外点（右开区间）")

	# 裁剪：与 (50,0,100,100) 求交 -> x=50, w=60
	var r2 := r.clone().crop(RoiRef.new(50, 0, 100, 100))
	_ok(r2.x == 50, "crop 后 x=50", str(r2.x))
	_ok(r2.w == 60, "crop 后 w=60（10+100-50）", str(r2.w))
	_ok(r2.y == 20 and r2.h == 50, "crop 后 y/h 不变")

	# 完全不相交 -> 空
	var r3 := r.clone().crop(RoiRef.new(500, 500, 10, 10))
	_ok(r3.empty(), "不相交则 empty")

	# 重叠判定
	_ok(r.overlap(RoiRef.new(50, 0, 100, 100)), "overlap 相交为真")
	_ok(not r.overlap(RoiRef.new(500, 500, 10, 10)), "overlap 不相交为假")


# ==================== 控件树 ====================

func _test_widget_tree() -> void:
	print("\n[2] 控件树")
	var root := WidgetRef.new()
	root.widget_name = "Root"

	var a := WidgetRef.new()
	a.widget_name = "A"
	a.w = 30
	a.h = 20
	root.add_widget(a, 10, 10)

	var b := WidgetRef.new()
	b.widget_name = "B"
	root.add_widget(b, 50, 30)

	_ok(root.child_count() == 2, "添加 2 个子控件")
	_ok(root.find_widget("A") == a, "按名查找 A")
	_ok(root.find_widget("B") == b, "按名查找 B")
	_ok(root.find_widget("NotExist") == null, "查找不存在返回 null")

	# 递归查找
	var c := WidgetRef.new()
	c.widget_name = "C"
	b.add_widget(c, 5, 5)
	_ok(root.find_widget("C") == c, "递归查找到 C")

	# 删除
	root.remove_widget(a)
	_ok(root.child_count() == 1, "删除后剩 1 个")
	_ok(root.find_widget("A") == null, "删除后查不到 A")

	root.clear_widgets()
	_ok(root.child_count() == 0, "清空后为 0")

	root.free()


# ==================== 尺寸计算 ====================

func _test_size() -> void:
	print("\n[3] 尺寸自动计算")
	var box := WidgetRef.new()
	# 不指定 w/h，按子控件包住
	var c1 := WidgetRef.new()
	c1.w = 40
	c1.h = 30
	box.add_widget(c1, 100, 50)

	_ok(box.size_w() == 140, "宽度 = 100 + 40", str(box.size_w()))
	_ok(box.size_h() == 80, "高度 = 50 + 30", str(box.size_h()))

	# 指定了就用自己的
	var box2 := WidgetRef.new()
	box2.w = 200
	box2.h = 100
	box2.add_widget(WidgetRef.new(), 500, 500)
	_ok(box2.size_w() == 200, "指定 w 时按 w")
	_ok(box2.size_h() == 100, "指定 h 时按 h")

	# 动态求值：Callable
	var box3 := WidgetRef.new()
	var n := 77
	box3.w = func(_self): return n
	_ok(box3.size_w() == 77, "Callable 求值 w", str(box3.size_w()))



# ==================== 按钮 ====================

func _test_button() -> void:
	print("\n[4] 按钮三态机")
	var btn := ButtonRef.new("Btn", 100, 30)
	var clicked := [0]
	btn.on_click = func(_w): clicked[0] += 1
	btn.on_draw = Callable()

	_ok(btn.get_state() == ButtonRef.BEVENT_OFF, "初始 OFF")

	var roi := RoiRef.new(0, 0, 100, 30)

	# 悬停进入
	var mm := InputEventMouseMotion.new()
	mm.position = Vector2(50, 15)
	btn.process_event(mm, roi)
	_ok(btn.get_state() == ButtonRef.BEVENT_ON, "悬停 -> ON")

	# 悬停离开
	mm.position = Vector2(200, 200)
	btn.process_event(mm, roi)
	_ok(btn.get_state() == ButtonRef.BEVENT_OFF, "离开 -> OFF")

	# 按下
	var md := InputEventMouseButton.new()
	md.button_index = MOUSE_BUTTON_LEFT
	md.pressed = true
	md.position = Vector2(50, 15)
	_ok(btn.process_event(md, roi), "按下被消费")
	_ok(btn.get_state() == ButtonRef.BEVENT_DOWN, "按下 -> DOWN")

	# 松开 -> 触发点击
	var mu := InputEventMouseButton.new()
	mu.button_index = MOUSE_BUTTON_LEFT
	mu.pressed = false
	mu.position = Vector2(50, 15)
	_ok(btn.process_event(mu, roi), "松开被消费")
	_ok(clicked[0] == 1, "点击回调触发 1 次", str(clicked[0]))

	# 区域外松开不触发
	mu.position = Vector2(300, 300)
	btn.process_event(mu, roi)
	_ok(clicked[0] == 1, "区域外松开不触发")



# ==================== 滑块 ====================

func _test_slider() -> void:
	print("\n[5] 滑块")
	var sl := SliderRef.new("Sl", 0, 100, 50)
	var changes := []
	sl.on_change = func(_w, v): changes.append(v)

	_ok(sl.value == 50, "初始值 50")
	_ok(absf(sl.value_ratio() - 0.5) < 0.01, "比例 0.5")

	sl.set_value(80)
	_ok(sl.value == 80, "设为 80")
	sl.set_value(200)
	_ok(sl.value == 100, "超上限被夹到 100")
	sl.set_value(-50)
	_ok(sl.value == 0, "低于下限被夹到 0")

	_ok(changes.size() >= 3, "变化回调触发", str(changes.size()))

	# 步长
	var sl2 := SliderRef.new("Sl2", 0, 100, 0)
	sl2.step = 10
	sl2.set_value(37)
	_ok(sl2.value == 40, "步长 10 时 37 -> 40", str(sl2.value))



# ==================== 复选框 ====================

func _test_checkbox() -> void:
	print("\n[6] 复选框")
	var cb := CheckboxRef.new("Cb", false, "")
	var states := []
	cb.on_toggle = func(_w, v): states.append(v)

	_ok(not cb.checked, "初始未选")

	var roi := RoiRef.new(0, 0, 30, 30)
	var md := InputEventMouseButton.new()
	md.button_index = MOUSE_BUTTON_LEFT
	md.pressed = true
	md.position = Vector2(10, 10)
	cb.process_event(md, roi)
	var mu := InputEventMouseButton.new()
	mu.button_index = MOUSE_BUTTON_LEFT
	mu.pressed = false
	mu.position = Vector2(10, 10)
	cb.process_event(mu, roi)
	_ok(cb.checked, "点击后选中")
	_ok(states.size() == 1 and states[0] == true, "回调收到 true")

	# 再点取消
	cb.process_event(md, roi)
	cb.process_event(mu, roi)
	_ok(not cb.checked, "再点取消选中")

	# radio 模式：不能被取消
	var rb := CheckboxRef.new("Rb", true, "")
	rb.radio_mode = true
	rb.process_event(md, roi)
	rb.process_event(mu, roi)
	_ok(rb.checked, "radio 模式点击后仍选中")



# ==================== 输入框 ====================

func _test_input() -> void:
	print("\n[7] 输入框")
	var ip := InputRef.new("Ip", "")
	var changes := []
	ip.on_change = func(_w, t): changes.append(t)

	_ok(ip.text.is_empty(), "初始为空")
	ip.insert_text("abc")
	_ok(ip.text == "abc", "插入 abc")
	_ok(ip.cursor == 3, "光标在末尾")
	ip.insert_text("de")
	_ok(ip.text == "abcde", "再插入 de")

	# 退格
	ip.backspace()
	_ok(ip.text == "abcd", "退格删 d", ip.text)

	# 光标移动后插入
	ip.cursor = 1
	ip.insert_text("X")
	_ok(ip.text == "aXbcd", "光标处插入", ip.text)
	_ok(ip.cursor == 2, "光标后移")

	# 删除
	ip.cursor = 2
	ip.delete_forward()
	_ok(ip.text == "aXcd", "delete 删 b", ip.text)

	# 最大长度
	var ip2 := InputRef.new("Ip2", "")
	ip2.max_length = 4
	ip2.insert_text("abcdefgh")
	_ok(ip2.text.length() == 4, "超长被截断到 4", ip2.text)

	# 空内容退格无副作用
	var ip3 := InputRef.new("Ip3", "")
	_ok(not ip3.backspace(), "空内容退格返回 false")



# ==================== 滚动容器 ====================

func _test_scroll() -> void:
	print("\n[8] 滚动容器")
	var sc := ScrollRef.new("Sc", 100, 100)
	sc.content_h = 300

	var roi := RoiRef.new(0, 0, 100, 100)
	_ok(sc.max_scroll_y(roi) == 200, "最大滚动 300-100=200", str(sc.max_scroll_y(roi)))

	# 滑块几何
	var geo := sc.v_thumb_geo(roi)
	_ok(not geo.is_empty(), "有滑块几何")
	if not geo.is_empty():
		var expect_h := int(100 * (100.0 / 300.0))
		expect_h = maxi(20, expect_h)
		_ok(int(geo["h"]) == expect_h, "滑块高度按比例 %d" % expect_h, str(geo["h"]))

	# 滚轮
	var wheel := InputEventMouseButton.new()
	wheel.button_index = MOUSE_BUTTON_WHEEL_DOWN
	wheel.pressed = true
	sc.process_event(wheel, roi)
	_ok(sc.scroll_y == 30, "滚轮下滚 30", str(sc.scroll_y))
	sc.process_event(wheel, roi)
	_ok(sc.scroll_y == 60, "再滚 30 -> 60", str(sc.scroll_y))

	# 滚出界被夹住
	for i in range(20):
		sc.process_event(wheel, roi)
	_ok(sc.scroll_y == 200, "滚到底被夹在 200", str(sc.scroll_y))

