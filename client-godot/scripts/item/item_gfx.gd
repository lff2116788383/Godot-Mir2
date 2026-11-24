extends RefCounted
class_name ItemGfx
## 物品图标加载
##
## 【逐行翻译】client/src/inventoryboard.cpp:182-200  drawItem
##            client/src/itembox.cpp                  物品格
##
## 关键规则（inventoryboard.cpp:191）：
##   贴图 id = pkgGfxID | 0X01000000
##   即物品图标来自 0x01xxxxxx 段的图库，低 24 位是 pkgGfxID
##
## 素材实测：assets/texture/item/{id:08X}.PNG
##   尺寸各不相同（8x12 / 16x10 / ...），所以不能用固定格尺寸绘制，
##   必须按图实际大小居中放进格子里。
##
## Godot 差异：从目录直接读 PNG（素材已按 id 命名，无需图库解码）。

const ItemRecordRef := preload("res://scripts/item/item_record.gd")

## 缓存：tex_key -> Texture2D
static var _cache: Dictionary = {}
static var _assets_root: String = ""


static func setup(p_assets_root: String) -> void:
	_assets_root = p_assets_root


## 由物品 id 取图标贴图
## 对应 g_itemDB->retrieve(DBCOM_ITEMRECORD(itemID).pkgGfxID | 0X01000000)
static func icon_of_item(item_id: int) -> Texture2D:
	var pkg := ItemRecordRef.pkg_gfx_id(item_id)
	if pkg <= 0:
		return null
	return icon_of_pkg(pkg)


## 由 pkgGfxID 取图标
static func icon_of_pkg(pkg_gfx_id: int) -> Texture2D:
	if pkg_gfx_id <= 0 or _assets_root.is_empty():
		return null
	if _cache.has(pkg_gfx_id):
		return _cache[pkg_gfx_id]

	# 素材里文件名就是 pkgGfxID 的 8 位十六进制（实测 00000078.PNG = 金币）
	var p := "%s/texture/item/%08X.PNG" % [_assets_root, pkg_gfx_id]
	if not FileAccess.file_exists(p):
		# 尝试带 0x01000000 前缀的命名
		p = "%s/texture/item/%08X.PNG" % [_assets_root, pkg_gfx_id | 0x01000000]
		if not FileAccess.file_exists(p):
			_cache[pkg_gfx_id] = null
			return null

	var img := Image.load_from_file(p)
	if img == null:
		_cache[pkg_gfx_id] = null
		return null
	var tex := ImageTexture.create_from_image(img)
	_cache[pkg_gfx_id] = tex
	return tex


## 按格居中绘制的偏移（原版 drawItem：图标在格内居中）
static func center_offset(tex: Texture2D, cell_w: int, cell_h: int) -> Vector2i:
	if tex == null:
		return Vector2i.ZERO
	return Vector2i((cell_w - tex.get_width()) / 2, (cell_h - tex.get_height()) / 2)


static func stats() -> Dictionary:
	var n := 0
	for v in _cache.values():
		if v != null:
			n += 1
	return {"cached": _cache.size(), "loaded": n}
