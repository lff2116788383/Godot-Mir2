extends RefCounted
class_name UiUnderBody
## 角色脚下信息（名字 / 行会 / 配偶 / 称号，自上而下居中排列）
##
## 【逐行翻译】client/src/gui/underbodyboard/underbodyboard.cpp（138 行）
##
## ── 结构 ──
##   m_name      名字，颜色取 hero->getNameColor()
##   m_guild     行会名，有头衔时格式 "%s[%s]"
##   m_spouse    配偶（原版无婚姻系统，textFunc 直接返回空 -> 永不显示）
##   m_titleList 称号列表（ItemFlex，居中）
##   四者放在一个 ItemFlex（align=CENTER）里纵向堆叠。
##
## ── 每行显隐 ──
##   原版每行都 .show = [this]{ return !m_xxx.empty(); }
##   即空字符串就隐藏自己 —— 所以布局必须跳过不可见的行，
##   否则名字和行会之间会出现空档。
##
## ── 名字颜色（hero.cpp:1687 getNameColor）──
##   红名在攻城战期间仍是红色；否则如果本行会在攻城战里，
##   用 getGuildWarNameColor(本行会) 的颜色。
##   这里由外部传颜色进来，逻辑交给上层判定。

const LINE_H := 14


## 组装行内容，返回可显示的字符串数组
static func build_lines(name: String, guild: String, guild_title: String,
		spouse: String, titles: Array) -> Array:
	var out: Array = []
	if not name.is_empty():
		out.append({"text": name, "kind": "name"})
	# 行会：有头衔则 "行会[头衔]"
	var g := ""
	if not guild.is_empty():
		if guild_title.is_empty():
			g = guild
		else:
			g = "%s[%s]" % [guild, guild_title]
	if not g.is_empty():
		out.append({"text": g, "kind": "guild"})
	if not spouse.is_empty():
		out.append({"text": spouse, "kind": "spouse"})
	for t in titles:
		if not str(t).is_empty():
			out.append({"text": str(t), "kind": "title"})
	return out


## 在 (cx, top_y) 居中绘制
## 返回总高度
static func draw(canvas, cx: int, top_y: int, lines: Array,
		name_color: Color = Color(1, 1, 1)) -> int:
	var y := top_y
	for ln in lines:
		var txt := str(ln.get("text", ""))
		var kind := str(ln.get("kind", ""))
		var col := name_color
		var size := 14
		match kind:
			"guild":
				col = name_color.lerp(Color(0.75, 0.82, 1.0), 0.35)
				size = 13
			"spouse":
				col = Color(1, 0.7, 0.85)
				size = 13
			"title":
				col = Color(1, 0.9, 0.55)
				size = 12
			_:
				col = name_color
				size = 14
		var tw: int = int(canvas.measure_text(txt, null, size).x)
		canvas.draw_string_shadow(txt, cx - tw / 2, y, col, Color(0, 0, 0, 0.9), 1, null, size)
		y += LINE_H
	return y - top_y
