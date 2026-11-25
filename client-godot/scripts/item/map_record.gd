extends RefCounted
class_name MapRecord
## 地图静态表（maprecord.inc -> maprecord.json）
## 地图二进制文件名沿用资源仓库：mapID 1 -> map/0.bin

static var _data: Dictionary = {}
static var _loaded := false

static func load_data(path: String = "res://data/maprecord.json") -> bool:
        if _loaded:
                return true
        if not FileAccess.file_exists(path):
                push_error("地图表不存在: " + path)
                return false
        var f := FileAccess.open(path, FileAccess.READ)
        if f == null:
                return false
        var parsed = JSON.parse_string(f.get_as_text())
        f.close()
        if not parsed is Dictionary:
                return false
        _data = parsed.get("records", {})
        _loaded = true
        return true

static func find(map_id: int) -> Dictionary:
        if not _loaded:
                load_data()
        return _data.get(str(map_id), {})

static func path_for(map_id: int) -> String:
        var root := OS.get_environment("MIR2X_ASSETS")
        if root.is_empty():
                root = "E:/Desktop/game/mir2x_assets-main"
        return "%s/map/%d.bin" % [root, map_id - 1]

static func name_of(map_id: int) -> String:
        return str(find(map_id).get("name", ""))

static func bgm_of(map_id: int) -> int:
        return int(find(map_id).get("bgmID", -1))

static func minimap_of(map_id: int) -> int:
        return int(find(map_id).get("miniMapID", -1))

static func switches_of(map_id: int) -> Array:
        var sw = find(map_id).get("switchList", {})
        return sw.get("_array", []) if sw is Dictionary else []

static func count() -> int:
        if not _loaded:
                load_data()
        return _data.size()
