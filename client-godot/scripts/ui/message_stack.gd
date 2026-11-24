extends RefCounted
class_name MessageStack
## 消息栈（聊天/系统提示的显示队列）
##
## 【逐行翻译】client/src/messagestackboard.cpp + messagestackboard.hpp
##
## 逻辑部分只有「队列管理」，其余是 XML 排版与绘制：
##   - addXMLMessage：超上限时移除最旧的，再追加新的
##   - removeFrontMessage / clear
##
## 原版结构：每条消息是一个 Message{typeset, wrapper}，
## typeset 是 XML 排版结果（B8），wrapper 是带 margin 与背景的容器。
## 这里只翻译队列语义，排版与绘制交给上层。

## 单条消息
class Entry:
	## 原始 XML 文本
	var xml: String = ""
	## 是否系统消息
	var is_system: bool = false
	## 时间戳（毫秒）
	var time_ms: int = 0


## 上限（0 表示不限）
var entry_limit: int = 0
## 条目间距
var item_space: int = 0
## 内边距
var margin: int = 0
## 背景色 / 边框色（各层由渲染处理）
var bg_color: Color = Color(0, 0, 0, 0.6)
var border_color: Color = Color(1, 1, 1, 0.3)
var corner: int = 0
## 是否显示时间
var show_time: bool = false

var m_message_list: Array = []


func setup(limit: int = 0, space: int = 0, margin_v: int = 0) -> void:
	entry_limit = limit
	item_space = space
	margin = margin_v


## addXMLMessage（messagestackboard.cpp:42）
func add_xml_message(xml: String, is_system: bool = false) -> void:
	# 超上限则移除最旧的（messagestackboard.cpp:44-46）
	while entry_limit > 0 and m_message_list.size() >= entry_limit:
		remove_front_message()

	var e := Entry.new()
	e.xml = xml
	e.is_system = is_system
	e.time_ms = Time.get_ticks_msec()
	m_message_list.append(e)


## addMessage（messagestackboard.cpp:34）
## 纯文本转成段落 XML
func add_message(text: String) -> void:
	add_xml_message("<par>%s</par>" % text.xml_escape())


## 移除最旧的一条
func remove_front_message() -> void:
	if not m_message_list.is_empty():
		m_message_list.remove_at(0)


## 清空（messagestackboard.cpp 的 clear）
func clear() -> void:
	m_message_list.clear()


func size() -> int:
	return m_message_list.size()


func messages() -> Array:
	return m_message_list


## 取最近 N 条
func latest(n: int) -> Array:
	if n <= 0:
		return []
	var start := maxi(0, m_message_list.size() - n)
	return m_message_list.slice(start)
