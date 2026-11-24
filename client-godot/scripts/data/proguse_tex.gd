extends RefCounted
class_name ProgUseTex
## UI 图库（ProgUseDB）加载
##
## 【逐行翻译】client/src/progusedb.hpp + progusedb.cpp
##
## 原版：
##   ProgUseDB::retrieve(uint32_t texID) -> SDL_Texture *
##   贴图库文件 res/proguse.zsdb，键是 4 字节十六进制字符串（hexstr::to_string，小写）
##
## ── 素材实测（推翻此前的错误结论）──
##   mir2x_assets-main/texture/proguse/ 共 1509 张 PNG，文件名就是 8 位十六进制 texID。
##   尺寸完全吻合 C++ 常量：
##     00000012.PNG = 800x133  （controlboard 左右背景，注释写 "800 x 133"）
##     00000013.PNG = 456x131  （controlboard 中部，注释写 "456 x 131"）
##     00000022.PNG = 127x41   （controlboard 标题，注释写 "127 x 41"）
##     06000000.PNG = 328x466  （myherostateboard 面板框）
##   所以 UI 贴图是齐全的，面板可以做到像素级还原，不需要纯色块替代。
##
## ── texID 段位含义（从各面板的 retireve() 调用归纳）──
##   0x00000000-0x0000FFFF  UI 基础件（按钮三态、背景、条）
##   0x01000000-0x01FFFFFF  头像/人物立绘（CBFace 用 0x010007CF 做兜底）
##   0x04000000-0x04FFFFFF  动画（CBTitle 的 m_arcAni 用 0x04000000）
##   0x06000000-0x06FFFFFF  大面板框（人物状态）
##   0x0B000000-0x0BFFFFFF  快捷栏按钮（texAniOpt）

const ASSETS_ENV := "MIR2X_ASSETS"
const DEFAULT_ROOT := "E:/Desktop/game/mir2x_assets-main"

## texID -> Texture2D（null 表示确认不存在，避免重复查文件）
static var _cache: Dictionary = {}
## 子目录名
static var _assets_root: String = ""
## 统计
static var _hit := 0
static var _miss := 0


## 初始化：定位素材根目录
static func setup(root: String = "") -> void:
	if not root.is_empty() and DirAccess.dir_exists_absolute(root):
		_assets_root = root
		return
	var env := OS.get_environment(ASSETS_ENV)
	if not env.is_empty() and DirAccess.dir_exists_absolute(env):
		_assets_root = env
		return
	if DirAccess.dir_exists_absolute(DEFAULT_ROOT):
		_assets_root = DEFAULT_ROOT
		return
	_assets_root = ""


static func assets_root() -> String:
	if _assets_root.is_empty():
		setup()
	return _assets_root


## 对应 ProgUseDB::retrieve(texID)
## 返回 null 表示图库里没有这张图（原版同样是返回 nullptr，调用处都做了判空）
static func retrieve(tex_id: int) -> Texture2D:
	if tex_id < 0:
		return null
	if _cache.has(tex_id):
		_hit += 1
		return _cache[tex_id]

	var root := assets_root()
	if root.is_empty():
		_cache[tex_id] = null
		_miss += 1
		return null

	# 原版 key 是 hexstr::to_string<uint32_t,4>(texID, buf, true) —— 8 位大写十六进制
	var p := "%s/texture/proguse/%08X.PNG" % [root, tex_id]
	if not FileAccess.file_exists(p):
		_cache[tex_id] = null
		_miss += 1
		return null

	var img := Image.load_from_file(p)
	if img == null:
		_cache[tex_id] = null
		_miss += 1
		return null
	var tex := ImageTexture.create_from_image(img)
	_cache[tex_id] = tex
	_miss += 1
	return tex


## 判断图库里是否有这张图（不加载）
static func has_tex(tex_id: int) -> bool:
	var root := assets_root()
	if root.is_empty():
		return false
	if _cache.has(tex_id):
		return _cache[tex_id] != null
	return FileAccess.file_exists("%s/texture/proguse/%08X.PNG" % [root, tex_id])


## 取贴图尺寸（没有则返回 Vector2i.ZERO）
static func tex_size(tex_id: int) -> Vector2i:
	var t := retrieve(tex_id)
	if t == null:
		return Vector2i.ZERO
	return Vector2i(t.get_width(), t.get_height())


## 把 texID 当「动画首帧」取图
## 原版 TexAniBoard 的 texAniOpt 高 8 位是动画组，低 24 位是组内序号；
## 素材里动画是以完整 8 位 id 命名的静态帧，取首帧等价于取这张图。
static func retrieve_ani(ani_opt: int, frame: int = 0) -> Texture2D:
	return retrieve(ani_opt + frame)


## 按区域裁切绘制（对应 GfxCropBoard / drawTexture 的 src 版本）
## 用于进度条（HP/MP/等级/负重）：整图是满条，按比例裁下面一截
##   src: 源矩形（相对贴图）
##   dx,dy,dw,dh: 目标矩形
static func blit_crop(canvas, tex: Texture2D, src: Rect2i,
		dx: int, dy: int, dw: int, dh: int) -> void:
	if tex == null or canvas == null:
		return
	if src.size.x <= 0 or src.size.y <= 0 or dw <= 0 or dh <= 0:
		return
	canvas.tex_draw_region(tex, Rect2(src.position, src.size), dx, dy, dw, dh)


static func stats() -> Dictionary:
	var loaded := 0
	for v in _cache.values():
		if v != null:
			loaded += 1
	return {
		"root": assets_root(),
		"queried": _cache.size(),
		"loaded": loaded,
		"hit": _hit,
		"miss": _miss,
	}


static func clear() -> void:
	_cache.clear()
	_hit = 0
	_miss = 0
