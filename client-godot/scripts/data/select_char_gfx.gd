extends RefCounted
class_name SelectCharGfx
## 选人/建号角色图库（texture/selectchar，74 张 APNG）
##
## 【逐行翻译】client/src/processselectchar.cpp:250-320
##            client/src/processcreatechar.cpp:150-230
##            client/src/pngtexoffdb.hpp:55 humanKey
##            client/src/selectcharframecount.inc
##            common/src/jobf.hpp
##
## ══════════════ 文件名编码（实测 74 张全部符合）══════════════
##   文件名 = [8 位 hex 键][6 位 hex 偏移].PNG
##   键位（与 pngtexoffdb.hpp:55 的 humanKey 同构）：
##     bits 3..8   anim    0=站立未选 1=选中动画 2=站立已选 3=取消选中 4=建号动画
##     bit  9      layer   1 = 武器/翅膀叠加层
##     bit  10     shadow  1 = 投影
##     bit  11     gender  1 = 男, 0 = 女
##                 ★ 实测校准：00000808（gender位=1）实载 20 帧，
##                   对应帧表「战士·男·anim1 = 20」；00000008（位=0）实载 11 帧，
##                   对应「战士·女·anim1 = 11」。所以位值 1 是男，不是女。
##                   与 C++ 一致：m_smChar.gender 是 bool，_gender_M = true，
##                   而 humanKey 里就是 (to_u32(gender) << 11)
##     bits 12..19 job     jobGfxIndex：0=战士 1=道士 2=法师
##   尾部 6 位 = dx(3) + dy(3)，12 位补码（apng_loader.decode_file_name 已实现）
##
##   公式：key = (anim << 3) | (layer << 9) | (shadow << 10) | (gender << 11) | (job << 12)
##
## ══════════════ 帧数（selectcharframecount.inc，30 条全抄）══════════════
##   (job, gender, anim) -> 帧数
##   注意同一职业男女帧数不同，且 anim 1/3 差异很大（战士男 20 帧、女 11 帧）
##
## ══════════════ 播放时序（processselectchar.cpp）══════════════
##   absFrame = round(aniTime / 200.0)   —— 每帧 200ms
##   gfxFrame = absFrame % frameCount
##   switchCharGfx：在 absFrame % frameCount == 0 时按随机规则切 anim
##     anim 0 → 1（50% 概率）/ 停留
##     anim 1 → 2（必定）
##     anim 2 → 3（50% 概率）/ 停留
##     anim 3 → 0（必定）
##   anim 切到 1 时播选中音效：
##     seffID = 0x00010000 | (gender << 4) | (job << 8) | 1   // 1 = select

const ProtocolDefRef := preload("res://scripts/core/protocol_def.gd")
const ApngRef := preload("res://scripts/data/apng_loader.gd")
const ProgUseRef := preload("res://scripts/data/proguse_tex.gd")

## 动画阶段（processselectchar.cpp 注释）
const ANIM_STAND_INACTIVE := 0   ## 站立，未选中
const ANIM_ON_SELECT := 1        ## 选中动画
const ANIM_STAND_ACTIVE := 2     ## 站立，已选中
const ANIM_ON_DESELECT := 3     ## 取消选中
const ANIM_ON_CREATE := 4        ## 建号预览（ProcessCreateChar 用）
const ANIM_COUNT := 5

## 每帧毫秒（absFrame = round(aniTime / 200)）
const FRAME_MS := 200.0

## 帧表：(job, gender, anim) -> 帧数
## 逐条照抄 selectcharframecount.inc，共 30 条
const FRAME_TABLE := {
	# 战士 · 女
	[1, false, 0]: 11, [1, false, 1]: 11, [1, false, 2]: 11,
	[1, false, 3]: 12, [1, false, 4]: 16,
	# 战士 · 男
	[1, true, 0]: 11, [1, true, 1]: 20, [1, true, 2]: 12,
	[1, true, 3]: 8, [1, true, 4]: 18,
	# 道士 · 女
	[2, false, 0]: 11, [2, false, 1]: 17, [2, false, 2]: 11,
	[2, false, 3]: 10, [2, false, 4]: 15,
	# 道士 · 男
	[2, true, 0]: 11, [2, true, 1]: 17, [2, true, 2]: 20,
	[2, true, 3]: 12, [2, true, 4]: 17,
	# 法师 · 女
	[4, false, 0]: 11, [4, false, 1]: 18, [4, false, 2]: 11,
	[4, false, 3]: 9, [4, false, 4]: 17,
	# 法师 · 男
	[4, true, 0]: 11, [4, true, 1]: 12, [4, true, 2]: 11,
	[4, true, 3]: 11, [4, true, 4]: 15,
}

## 职业显示名（jobf.hpp:jobName）
const JOB_NAMES := {
	1: ["战士"],
	2: ["道士"],
	4: ["法师"],
}

## 图库缓存：key -> { frames: Array[Texture2D], dx: int, dy: int }
static var _cache: Dictionary = {}
static var _assets_root := ""
static var _loaded_files := 0


static func setup(root: String = "") -> void:
	_assets_root = root if not root.is_empty() else ProgUseRef.assets_root()


static func assets_root() -> String:
	if _assets_root.is_empty():
		setup()
	return _assets_root


# ==================== 键编码 ====================

## 由 5 个维度拼键（与 C++ PNGTexOffDB::humanKey 同构）
## 注意 gfxID 参数在 C++ 里是「job<<9 | anim<<3」，这里直接给三个分量更直观
static func make_key(job_index: int, gender_male: bool, anim: int,
		shadow: bool = false, layer: int = 0) -> int:
	return ((anim & 0x3F) << 3) \
		| ((layer & 0x1) << 9) \
		| ((1 if shadow else 0) << 10) \
		| ((1 if gender_male else 0) << 11) \
		| ((job_index & 0xFF) << 12)


## 反向解码（测试与调试用）
static func decode_key(key: int) -> Dictionary:
	return {
		"anim": (key >> 3) & 0x3F,
		"layer": (key >> 9) & 0x1,
		"shadow": (key >> 10) & 0x1,
		"gender": (key >> 11) & 0x1,   # 1=男 0=女
		"job": (key >> 12) & 0xFF,
	}


## jobf::firstJob
static func first_job(job: int) -> int:
	if job & ProtocolDefRef.JOB_WARRIOR:
		return ProtocolDefRef.JOB_WARRIOR
	if job & ProtocolDefRef.JOB_TAOIST:
		return ProtocolDefRef.JOB_TAOIST
	if job & ProtocolDefRef.JOB_WIZARD:
		return ProtocolDefRef.JOB_WIZARD
	return 0


## jobf::jobGfxIndex（取第一个，本引擎单职业）
static func job_gfx_index(job: int) -> int:
	var fj := first_job(job)
	match fj:
		ProtocolDefRef.JOB_WARRIOR: return 0
		ProtocolDefRef.JOB_TAOIST: return 1
		ProtocolDefRef.JOB_WIZARD: return 2
	return 0


## jobf::jobValid
static func job_valid(job: int) -> bool:
	return (job & (ProtocolDefRef.JOB_WARRIOR | ProtocolDefRef.JOB_TAOIST | ProtocolDefRef.JOB_WIZARD)) != 0


## jobf::jobName
static func job_name(job: int) -> String:
	var names: Array = []
	if job & ProtocolDefRef.JOB_WARRIOR:
		names.append("战士")
	if job & ProtocolDefRef.JOB_TAOIST:
		names.append("道士")
	if job & ProtocolDefRef.JOB_WIZARD:
		names.append("法师")
	return "".join(names) if not names.is_empty() else ""


## 取帧数（selectcharframecount.inc 查询）
static func frame_count(job: int, gender_male: bool, anim: int) -> int:
	var fj := first_job(job)
	if not job_valid(fj):
		return 0
	return int(FRAME_TABLE.get([fj, gender_male, anim], 0))


## 角色 gfx 基址（processselectchar.cpp:charGfxBaseID）
## = (jobGfxIndex << 9) + (anim << 3)
static func gfx_base_id(job: int, anim: int) -> int:
	return (job_gfx_index(job) << 9) + (anim << 3)


## 选中/建号音效（processselectchar.cpp:switchCharGfx 尾部）
## on_select: 1 = 选人界面, 0 = 建号界面
static func anim_seff(job: int, gender_male: bool, on_select: bool) -> int:
	var off_gender := 0 if gender_male else 1
	var off_job := job_gfx_index(job)
	return 0x00010000 | (off_gender << 4) | (off_job << 8) | (1 if on_select else 0)


# ==================== 贴图加载 ====================

## 取一组图层（frames 数组 + 偏移）
## 返回 { frames: Array, dx: int, dy: int }，缺图返回 null
static func retrieve(job_index: int, gender_male: bool, anim: int,
		shadow: bool = false, layer: int = 0) -> Variant:
	var key := make_key(job_index, gender_male, anim, shadow, layer)
	if _cache.has(key):
		return _cache[key]

	var root := assets_root()
	if root.is_empty():
		_cache[key] = null
		return null

	# 文件名 = 8 位 hex key + 6 位 hex 偏移，扫目录匹配前缀
	var p := _find_file(root, key)
	if p.is_empty():
		_cache[key] = null
		return null

	var info := ApngRef.decode_file_name(p.get_file().get_basename())
	var frames := ApngRef.load_frames(p)
	if frames.is_empty():
		_cache[key] = null
		return null

	var rec := {
		"frames": frames,
		"dx": int(info.get("dx", 0)),
		"dy": int(info.get("dy", 0)),
	}
	_cache[key] = rec
	_loaded_files += 1
	return rec


## 按 8 位 hex 前缀找文件（偏移 6 位不参与匹配）
static func _find_file(root: String, key: int) -> String:
	var dir_path := "%s/texture/selectchar" % root
	var d := DirAccess.open(dir_path)
	if d == null:
		return ""
	var prefix := "%08X" % key
	d.list_dir_begin()
	var fn := d.get_next()
	while not fn.is_empty():
		if fn.length() >= 14 and not d.current_is_dir() and fn.substr(0, 8) == prefix:
			d.list_dir_end()
			return dir_path.path_join(fn)
		fn = d.get_next()
	d.list_dir_end()
	return ""


## 一次画一个角色（原版 fnDrawTexture 的四层顺序）
##   1. 本体（humanKey(gfxID, gender)）
##   2. 叠加层（alpha 150）
##   3. 本体（再画一次，原版就是这么写的）
##   4. 投影（layer 1）
## 返回是否有任何层画成功
static func draw_char(canvas, job: int, gender_male: bool, anim: int,
		abs_frame: int, draw_x: int, draw_y: int,
		active: bool = true) -> bool:
	var fj := first_job(job)
	if not job_valid(fj):
		return false
	var n := frame_count(job, gender_male, anim)
	if n <= 0:
		return false
	var fi: int = abs_frame % n
	var ji := job_gfx_index(job)

	var drew := false
	# 本体
	if _blit(canvas, ji, gender_male, anim, fi, draw_x, draw_y, active, false, false, 0):
		drew = true
		# 叠加层（武器/翅膀），alpha 150
		_blit(canvas, ji, gender_male, anim, fi, draw_x, draw_y, active, true, false, 1)
		# 再画本体
		_blit(canvas, ji, gender_male, anim, fi, draw_x, draw_y, active, false, false, 0)
		# 投影
		_blit(canvas, ji, gender_male, anim, fi, draw_x, draw_y, active, false, true, 0)
	return drew


static func _blit(canvas, job_index: int, gender_male: bool, anim: int, frame: int,
		draw_x: int, draw_y: int, active: bool, alpha: bool, shadow: bool,
		layer: int) -> bool:
	var rec = retrieve(job_index, gender_male, anim, shadow, layer)
	if rec == null:
		return false
	var frames: Array = rec["frames"]
	if frames.is_empty():
		return false
	var tex: Texture2D = frames[clampi(frame, 0, frames.size() - 1)]
	if tex == null:
		return false

	# 调制色（processcreatechar.cpp:drawChar 的 modColor）
	var col := Color(1, 1, 1, 150.0 / 255.0 if alpha else 1.0)
	if not active:
		col = Color(128.0 / 255.0, 128.0 / 255.0, 128.0 / 255.0,
			150.0 / 255.0 if alpha else 1.0)

	canvas.tex_draw(tex, draw_x + int(rec["dx"]), draw_y + int(rec["dy"]), col)
	return true


## 该 (job,gender,anim) 是否有图（不加载，只查文件）
static func has_char(job: int, gender_male: bool, anim: int) -> bool:
	var fj := first_job(job)
	if not job_valid(fj):
		return false
	var root := assets_root()
	if root.is_empty():
		return false
	return not _find_file(root, make_key(job_gfx_index(job), gender_male, anim)).is_empty()


static func stats() -> Dictionary:
	return {"root": assets_root(), "loaded": _loaded_files, "cached": _cache.size()}


static func clear() -> void:
	_cache.clear()
	_loaded_files = 0
