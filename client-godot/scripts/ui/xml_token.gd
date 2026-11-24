extends RefCounted
class_name XmlToken
## 排版单元（TOKEN）
##
## 【逐行翻译】client/src/token.hpp
##
## 每个 TOKEN 是一个可排版的最小单位（一个汉字、一个 emoji 帧、一张图）。
## 尺寸模型（token.hpp:1-20 的 ASCII 图）：
##
##   +-------------+------(*: X, Y)
##   |             |
##   |           W1      W       W2
##   |     --->|   |<--------->|   |<--
##   +-------> +---*-----------+---+  --------
##             |   |  .    .   |   |  ^    ^
##             |   |           |   |  |    |
##             |   |  /\_/\    |   |  | H1 | H
##             |   |=( 0w0 )=  |   |  |    |
##             |   |  )   (  //|   |  |    |
##             |   | (__ __)// |   |  v    |
##             +---+   .       +---+  -    |
##                 |    .      |        H2 v
##                 +-----------+      --------
##
##   (X, Y) 是 token 的锚点（左上角基准）
##   W1/W2 是左右自然留白，H1/H2 是基线上下的高度
##   width() = W1 + W + W2

## token 类型（对应 union 的三个成员）
const KIND_UTF8CHAR := 0
const KIND_EMOJI := 1
const KIND_IMAGE := 2

var kind: int = KIND_UTF8CHAR

## 属于哪个 leaf（段落片段）
var leaf: int = 0
## 选择 ID（用于点击选中）
var select_id: int = 0

## 静态尺寸信息（初始化后不变）
var w: int = 0
var h: int = 0

## 布局后的状态
var x: int = 0
var y: int = 0
var w1: int = 0
var w2: int = 0
var h1: int = 0
var h2: int = 0

## 内容键：utf8char / image 用 key；emoji 用 key + 帧信息
var key: int = 0
var emoji_frame_count: int = 0
var emoji_frame: int = 0
var emoji_delay: int = 0
var emoji_tick: int = 0


## _TokenBox::width()（token.hpp:60）
func full_width() -> int:
	return w1 + w + w2


func set_size(pw: int, ph: int) -> void:
	w = pw
	h = ph


func set_padding(left: int, right: int) -> void:
	w1 = left
	w2 = right


func set_heights(up: int, down: int) -> void:
	h1 = up
	h2 = down


func to_str() -> String:
	return "Token{kind=%d w=%d h=%d w1=%d w2=%d h1=%d h2=%d @(%d,%d)}" % [
		kind, w, h, w1, w2, h1, h2, x, y
	]
