extends Node
## PNG 纹理运行时加载 + LRU 缓存
##
## 为什么需要：
## 实测单张 map 瓦片 PNG 加载 ≈ 0.86 ms（2000 张样本）。
## 一屏 60x40 = 2400 格，全量加载需 2 秒，不可接受。
## 因此必须缓存 + 限制容量，并优先走 LRU 复用。

const MAX_ENTRIES := 4096

var _cache: Dictionary = {}          # path -> Texture2D
var _lru: Array[String] = []         # 最旧在前
var _hits := 0
var _miss := 0
var _evict := 0


## 取纹理；加载失败返回 null（不抛错，地图里有空洞是正常的）
func get_texture(path: String) -> Texture2D:
	if _cache.has(path):
		_hits += 1
		_touch(path)
		return _cache[path]

	_miss += 1
	if not FileAccess.file_exists(path):
		return null

	var img := Image.load_from_file(path)
	if img == null:
		return null
	img.convert(Image.FORMAT_RGBA8)
	var tex := ImageTexture.create_from_image(img)
	_cache[path] = tex
	_lru.append(path)
	_trim()
	return tex


func _touch(path: String) -> void:
	var i := _lru.find(path)
	if i >= 0:
		_lru.remove_at(i)
	_lru.append(path)


func _trim() -> void:
	while _cache.size() > MAX_ENTRIES and not _lru.is_empty():
		var old: String = _lru.pop_front()
		_cache.erase(old)
		_evict += 1


## 预加载一批路径（用于进图时预热视野）
func preload_batch(paths: Array) -> int:
	var loaded := 0
	for p in paths:
		if get_texture(p) != null:
			loaded += 1
	return loaded


func stats() -> Dictionary:
	return {
		"cached": _cache.size(),
		"hits": _hits,
		"miss": _miss,
		"evict": _evict,
		"hit_rate": (float(_hits) / float(max(1, _hits + _miss))),
	}


func clear() -> void:
	_cache.clear()
	_lru.clear()
	_hits = 0
	_miss = 0
	_evict = 0
