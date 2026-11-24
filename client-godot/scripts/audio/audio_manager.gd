extends Node
class_name AudioManager
## 音频系统（音效库 + 背景音乐 + 3D 定位）
##
## 【逐行翻译】client/src/soundeffectdb.cpp（193 行）
##            client/src/soundeffectdb.hpp
##            client/src/bgmusicdb.cpp（56 行）
##            client/src/bgmusicdb.hpp
##            client/src/soundeffecthandle.*
##            client/src/sdldevice.cpp:1533-1710（播放 / 音量 / 3D 定位）
##            client/src/initview.cpp:66-70（加载顺序与权重）
##
## ══════════════ 音效 ID 命名规则（soundeffectdb.cpp 顶部大段注释的结论）══════════════
## 素材根：mir2x_assets-main/sound/，两个子目录：
##   seff/  848 个 .WAV  文件名 = 8 位大写十六进制 seffID
##   bgm/    25 首 .mp3/.wav 文件名 = 8 位 hex + "_" + 原名
##
## 段位分布（实测 848 个文件）：
##   0x00xxxxxx  12 个   UI 短音效
##   0x01xxxxxx  82 个   人形（脚步 / 挥击 / 受击 / 死亡 / 按钮 / 拾取）
##   0x02xxxxxx 647 个   怪物（按 lookID 分块，每块 16 个）
##   0x04xxxxxx 107 个   魔法（按 MagicRecord.seffBase 分块，每块 16 个）
##
## ── 人形段（0x01000000）低 24 位含义 ──
##   1..32    脚步：walk_ground/stone/lawn/rough/wood/cave/room/water 各 4 声
##            1=地面走 2=地面走另侧 3=地面跑 5=石地走 …（原版注释逐条列出）
##            33..36   马匹走跑
##   50..57   挥击：匕首/木剑/剑/刀/斧/锏/棍/空手
##   60..65   受击：匕首/木剑/剑/刀/斧/锏
##   70..73   击中身体：剑/斧/长棍/空手
##   80..83   击中护甲：剑/斧/长棍/空手
##   100..110  交互：石门/熔石/开场/按钮/金钱/吃药…
##   111..118  装备点击：武器/衣服/戒指/手镯/项链/头盔/长袍/物品
##   130..137  魔法特效音（法师类）
##   138..145  人声：男受击/女受击/男死/女死
##
## ── 怪物段（0x02000000）──
##   SYS_MONSEFFBASE(lookID) = 0x02000000 + lookID * 16
##   offset：0=出现 1=站立 2=攻击 4=受击 5=死亡
##
## ── 魔法段（0x04000000）──
##   DBCOM_MAGICGFXSEFFID = 0x04000000 + seffBase * 16 + (stageIndex - MST_BEGIN)
##   stageIndex 顺序见 magicStageID：MST_BEGIN 起的「启动/运行/结束」等
##
## ══════════════ 3D 定位（processrun.cpp:3051 playSoundEffectAt）══════════════
##   dGX = 目标格x - 主角格x，dGY 同理
##   distance = LDistance(dGX, dGY, 0, 0) 取整   —— 注意是「欧氏距离」不是切比雪夫
##   angle    = 90 - 180 * atan2(-dGY, dGX) / π
##   然后 clamp distance 到 255，angle 归一到 [0,360)
##   apply3DPosition：rad = angle*π/180，scaled = distance/255
##     pos = (sin(rad)*scaled, 0, -cos(rad)*scaled)
##
## ══════════════ 音量与并发 ══════════════
##   BGM 单轨（m_bgmTrack），SFX 轨道池（原版 128 条）
##   setBGMVolume / setSoundEffectVolume 各自独立的增益（SDL3_mixer 没有 bus 概念，
##   原版是对每条 SFX 轨道单独设 gain，BGM 轨道不动）
##   repeats == 0 表示无限循环（loops = -1），N 表示播放 N 次（loops = N-1）

## 素材根
const ASSETS_ENV := "MIR2X_ASSETS"
const DEFAULT_ROOT := "E:/Desktop/game/mir2x_assets-main"

## SFX 轨道池大小（sdldevice.cpp 的 128）
const SFX_TRACK_COUNT := 128

## 怪物音效偏移（monsterrecord.hpp:10-14）
const MONSEFF_SPAWN := 0
const MONSEFF_STAND := 1
const MONSEFF_ATTACK := 2
const MONSEFF_HITTED := 4
const MONSEFF_DIE := 5
const SYS_SEFFSIZE := 16

# ==================== 音效 ID 构造（全部逐条对应原版）====================

## SYS_MONSEFFBASE(lookID)
static func mon_seff_base(look_id: int) -> int:
	return 0x02000000 + look_id * SYS_SEFFSIZE


## 怪物音效 id
static func mon_seff(look_id: int, offset: int) -> int:
	return mon_seff_base(look_id) + offset


## 魔法音效 id（DBCOM_MAGICGFXSEFFID）
## seff_base 来自 MagicRecord.seffBase；stage_index 从 MST_BEGIN(0) 起
static func magic_seff(seff_base: int, stage_index: int) -> int:
	return 0x04000000 + seff_base * 16 + stage_index


## 脚步音效（hero.cpp:323-326）
## MOTION_WALK 1 / RUN 3 / 马走 33 / 马跑 35
const STEP_WALK := 1
const STEP_RUN := 3
const STEP_HORSE_WALK := 33
const STEP_HORSE_RUN := 35


## 动作类型常量（严格对齐 scripts/action/motion_type.gd，勿凭记忆填）
const MOTION_ONEHSWING := 12
const MOTION_SPEARVSWING := 14
const MOTION_SPEARHSWING := 15
const MOTION_HITTED := 16
const MOTION_WALK := 22
const MOTION_RUN := 23
const MOTION_ONHORSEWALK := 31
const MOTION_ONHORSERUN := 32

## hero.cpp:327-333 的七种挥击动作（按 motion_type.gd 的实际值）
## ONEVSWING=10 TWOHSWING=13 ONEHSWING=12 TWOVSWING=11
## RANDSWING=18 SPEARHSWING=15 SPEARVSWING=14
const SWING_MOTIONS := [
	10, 11, 12, 13, 14, 15, 18,
]


static func step_seff(motion_type: int, frame: int) -> int:
	## hero.cpp:254-262 的 fnPlayStepSound：frame==1 放 base，frame==4 放 base+1
	var base := 0
	match motion_type:
		MOTION_WALK:        base = 0x01000000 + STEP_WALK
		MOTION_RUN:         base = 0x01000000 + STEP_RUN
		MOTION_ONHORSEWALK: base = 0x01000000 + STEP_HORSE_WALK
		MOTION_ONHORSERUN:  base = 0x01000000 + STEP_HORSE_RUN
		_:  return -1
	if frame == 1:
		return base
	if frame == 4:
		return base + 1
	return -1


## 挥击音效（hero.cpp:271-281，按武器 category 判定）
## 返回 0x01010000 + n；无武器 -> 空手 57
static func attack_seff(weapon_category: String) -> int:
	match weapon_category:
		"匕首": return 0x01010000 + 50
		"木剑": return 0x01010000 + 51
		"剑":   return 0x01010000 + 52
		"刀":   return 0x01010000 + 53
		"斧":   return 0x01010000 + 54
		"锏":   return 0x01010000 + 55
		"棍":   return 0x01010000 + 56
		_:      return 0x01010000 + 57


## 击中怪物时的受击音（clientmonster.cpp:130-135）：固定 0x01010000+61（木剑）
const MON_HITTED_BY := 0x01010000 + 61


## 人形受击的武器声（hero.cpp 的 fnPlayHittedSound，按攻击者的武器 category）
static func struck_weapon_seff(weapon_category: String) -> int:
	match weapon_category:
		"匕首": return 0x01010000 + 60
		"木剑": return 0x01010000 + 61
		"剑":   return 0x01010000 + 62
		"刀":   return 0x01010000 + 63
		"斧":   return 0x01010000 + 64
		"锏":   return 0x01010000 + 65
		_:      return 0x01010000 + 61


## 击中身体 / 护甲的判定（hero.cpp：fromUID 是怪物则看是否有衣服）
static func struck_body_seff(has_dress: bool) -> int:
	return 0x01010000 + (83 if has_dress else 73)


## 人声受击（hero.cpp:288）：0x01030000 + (gender ? 138 : 139)
static func struck_voice_seff(gender: int) -> int:
	return 0x01030000 + (138 if gender != 0 else 139)


## 装备点击音（hero.cpp:1575-1583 setWLItem 的 playSound）
static func equip_click_seff(wltype: int) -> int:
	match wltype:
		3:  return 0x01020000 + 111   ## WLG_WEAPON
		1:  return 0x01020000 + 112   ## WLG_DRESS
		8, 9: return 0x01020000 + 113 ## WLG_RING0/1
		6, 7: return 0x01020000 + 114 ## WLG_ARMRING0/1
		5:  return 0x01020000 + 115   ## WLG_NECKLACE
		2:  return 0x01020000 + 116   ## WLG_HELMET
		_:  return 0x01020000 + 118


## 背包相关（invpack.cpp）
const SEFF_PICK_GOLD := 0x01020000 + 106   ## 拾取金钱
const SEFF_USE_ITEM := 0x01020000 + 108    ## 使用物品
const SEFF_BUTTON := 0x00000000 + 0        ## 按钮（原版用 texAniOpt 高字节，见下）


## 3D 定位参数（processrun.cpp:3063-3074）
## 返回 { distance: int, angle: int }
static func spatial_param(dgx: int, dgy: int) -> Dictionary:
	if dgx == 0 and dgy == 0:
		return {"distance": 0, "angle": 0}
	# LDistance(dGX, dGY, 0, 0) = sqrt(dGX² + dGY²)
	var dist: int = int(round(sqrt(float(dgx * dgx + dgy * dgy))))
	# angle = 90 - 180 * atan2(-dGY, dGX) / π
	var angle: float = 90.0 - 180.0 * atan2(float(-dgy), float(dgx)) / PI
	var ai: int = int(round(angle))
	ai = ((ai % 360) + 360) % 360
	return {"distance": mini(dist, 255), "angle": ai}


## 3D 位置向量（sdldevice.cpp:1696 apply3DPosition）
## 0 = 北(-z)，90 = 东(+x)，180 = 南(+z)，270 = 西(-x)
static func spatial_pos(distance: int, angle: int) -> Vector3:
	var rad: float = float(angle) * PI / 180.0
	var scaled: float = float(distance) / 255.0
	return Vector3(sin(rad) * scaled, 0.0, -cos(rad) * scaled)


# ==================== 运行时 ====================

## 是否禁用音频（对应 ClientArgParser::disableAudio）
var enabled := true
## BGM 音量 0..1
var bgm_volume := 0.5
## 音效音量 0..1
var sfx_volume := 0.6
## 当前 BGM id（0 表示无）
var cur_bgm := -1

## 素材根
var assets_root := ""

## BGM 播放器
var _bgm_player: AudioStreamPlayer = null
## SFX 播放器池
var _sfx_players: Array = []
var _sfx_cursor := 0
## 缓存：seff_id -> AudioStream（null 表示确认没有）
var _seff_cache: Dictionary = {}
var _bgm_cache: Dictionary = {}

## headless 标记（无播放器，不能真出声，但允许加载校验）
var _headless := false

## 统计
var _play_count := 0
var _miss_count := 0


func _ready() -> void:
	assets_root = _resolve_root()
	# headless（无音频驱动）时不建播放器：
	# Godot 在 --headless + AUDIO_DRIVER=Dummy 下创建 128 个 AudioStreamPlayer
	# 会直接崩进程（实测退出码 1 且无任何输出），所以显式跳过。
	if DisplayServer.get_name() == "headless":
		# 仍然可以「加载」音频流用于校验，只是不能真的出声
		enabled = false
		_headless = true
		return
	_bgm_player = AudioStreamPlayer.new()
	_bgm_player.name = "BgmPlayer"
	_bgm_player.bus = "Master"
	add_child(_bgm_player)
	for i in range(SFX_TRACK_COUNT):
		var p := AudioStreamPlayer.new()
		p.name = "Sfx%d" % i
		add_child(p)
		_sfx_players.append(p)


func _resolve_root() -> String:
	var env := OS.get_environment(ASSETS_ENV)
	if not env.is_empty() and DirAccess.dir_exists_absolute(env):
		return env
	if DirAccess.dir_exists_absolute(DEFAULT_ROOT):
		return DEFAULT_ROOT
	return ""


# ==================== 加载 ====================

## 音效文件路径（seff/{ID:08X}.WAV，大小写都要试）
func _seff_path(seff_id: int) -> String:
	if assets_root.is_empty():
		return ""
	var h := "%08X" % seff_id
	# 素材里扩展名不统一（.WAV / .wav），逐个试
	for ext in [".WAV", ".wav"]:
		var p := "%s/sound/seff/%s%s" % [assets_root, h, ext]
		if FileAccess.file_exists(p):
			return p
	return ""


## BGM 文件路径（bgm/{ID:08X}_xxx.mp3，名字带后缀，必须扫目录匹配前缀）
func _bgm_path(bgm_id: int) -> String:
	if assets_root.is_empty():
		return ""
	var dir_path := "%s/sound/bgm" % assets_root
	var d := DirAccess.open(dir_path)
	if d == null:
		return ""
	var prefix := "%08X_" % bgm_id
	d.list_dir_begin()
	var fn := d.get_next()
	while not fn.is_empty():
		if fn.to_upper().begins_with(prefix.to_upper()) and not d.current_is_dir():
			d.list_dir_end()
			return dir_path.path_join(fn)
		fn = d.get_next()
	d.list_dir_end()
	return ""


## 取音效流（带缓存）
## 注意：加载不受 enabled 影响 —— 音频被禁用（如 headless）时仍要能读素材做校验，
## 只是 play_seff 会直接返回 false。原版 disableAudio 是直接不加载，
## 这里为了「素材可达性可测」把两件事拆开。
func get_seff(seff_id: int) -> AudioStream:
	if seff_id < 0:
		return null
	if _seff_cache.has(seff_id):
		return _seff_cache[seff_id]
	var p := _seff_path(seff_id)
	if p.is_empty():
		_seff_cache[seff_id] = null
		_miss_count += 1
		return null
	var st := _load_stream(p)
	_seff_cache[seff_id] = st
	if st == null:
		_miss_count += 1
	return st


func _load_stream(path: String) -> AudioStream:
	var ext := path.get_extension().to_lower()
	if ext == "wav":
		return AudioStreamWAV.load_from_file(path)
	elif ext == "mp3":
		return AudioStreamMP3.load_from_file(path)
	elif ext == "ogg":
		return AudioStreamOggVorbis.load_from_file(path)
	return null


# ==================== 播放音效 ====================

## 播放音效（对应 SDLDevice::playSoundEffect）
## distance: 0..255，-1 表示不播（原版 distance < 0 直接返回）
## angle: 度，会归一到 [0,360)
## repeats: 0 = 无限循环，N = 播 N 次
func play_seff(seff_id: int, distance: int = 0, angle: int = 0,
		repeats: int = 1) -> bool:
	if not enabled:
		return false
	if distance < 0:
		return false
	var st := get_seff(seff_id)
	if st == null:
		return false

	var p = _pick_player()
	if p == null:
		return false

	# 3D 定位：2D 播放器用音量/声像模拟
	var d: int = mini(distance, 255)
	var a: int = ((angle % 360) + 360) % 360
	var atten: float = 1.0 - float(d) / 255.0 * 0.75   ## 远处最多衰减到 25%
	var pos3 := spatial_pos(d, a)
	# pos3.x 是左右分量（-1..1），映射成声像
	p.volume_db = linear_to_db(maxf(0.001, atten * sfx_volume))
	p.pan = clampf(pos3.x, -1.0, 1.0)
	p.stream = st
	p.play()
	_play_count += 1
	return true


## 按格坐标播放（对应 ProcessRun::playSoundEffectAt）
## 需要主角位置
var hero_gx := 0
var hero_gy := 0


func play_seff_at(seff_id: int, gx: int, gy: int, repeats: int = 1) -> bool:
	if seff_id < 0 or seff_id == 0xFFFFFFFF:
		return false
	var sp := spatial_param(gx - hero_gx, gy - hero_gy)
	return play_seff(seff_id, int(sp["distance"]), int(sp["angle"]), repeats)


func set_hero_loc(gx: int, gy: int) -> void:
	hero_gx = gx
	hero_gy = gy


## 取一条空闲轨道（轮转，原版是真的找 free track；这里简化成 round-robin）
func _pick_player():
	if _sfx_players.is_empty():
		return null
	# 优先找没在播的
	for i in range(_sfx_players.size()):
		var p = _sfx_players[i]
		if not p.playing:
			return p
	# 全忙则顶掉最老的
	var p2 = _sfx_players[_sfx_cursor % _sfx_players.size()]
	_sfx_cursor += 1
	return p2


# ==================== BGM ====================

## 播放 BGM（对应 SDLDevice::playBGM；repeats 0 = 无限循环）
func play_bgm(bgm_id: int, repeats: int = 0) -> bool:
	if not enabled:
		return false
	if bgm_id == cur_bgm and _bgm_player != null and _bgm_player.playing:
		return true
	var st := get_bgm(bgm_id)
	if st == null:
		return false
	cur_bgm = bgm_id
	_bgm_player.stream = st
	_bgm_player.volume_db = linear_to_db(maxf(0.001, bgm_volume))
	_bgm_player.play()

	# 循环次数（Godot 的 AudioStream 没有 per-play loops，靠 stream.loop 控制）
	if st is AudioStreamWAV:
		(st as AudioStreamWAV).loop_mode = AudioStreamWAV.LOOP_FORWARD if repeats == 0 else AudioStreamWAV.LOOP_DISABLED
	elif st is AudioStreamMP3:
		(st as AudioStreamMP3).loop = (repeats == 0)
	elif st is AudioStreamOggVorbis:
		(st as AudioStreamOggVorbis).loop = (repeats == 0)
	return true


func get_bgm(bgm_id: int) -> AudioStream:
	if bgm_id < 0:
		return null
	if _bgm_cache.has(bgm_id):
		return _bgm_cache[bgm_id]
	var p := _bgm_path(bgm_id)
	if p.is_empty():
		_bgm_cache[bgm_id] = null
		_miss_count += 1
		return null
	var st := _load_stream(p)
	_bgm_cache[bgm_id] = st
	if st == null:
		_miss_count += 1
	return st


func stop_bgm() -> void:
	if _bgm_player != null:
		_bgm_player.stop()
	cur_bgm = -1


func set_bgm_volume(v: float) -> void:
	bgm_volume = clampf(v, 0.0, 1.0)
	if _bgm_player != null and _bgm_player.playing:
		_bgm_player.volume_db = linear_to_db(maxf(0.001, bgm_volume))


func set_sfx_volume(v: float) -> void:
	sfx_volume = clampf(v, 0.0, 1.0)


## 停所有音效（对应 stopSoundEffect，只停 SFX 不动 BGM）
func stop_all_seff() -> void:
	for p in _sfx_players:
		p.stop()


# ==================== 场景联动 ====================

## 切图播 BGM（processrun.cpp:930-936：地图记录变了且 bgmID 有效才换）
func on_map_changed(new_bgm_id: int) -> void:
	if new_bgm_id < 0:
		return
	if new_bgm_id == cur_bgm:
		return
	play_bgm(new_bgm_id, 0)


## 登录 / 建号 / 选人的固定 BGM（processrun / processlogin / processcreatechar）
const BGM_LOGIN := 0x00040007        ## processlogin.cpp:160
const BGM_CREATE_CHAR := 0x00040001  ## processcreatechar.cpp:69
const BGM_OPENING := 0x00040006      ## 开场


# ==================== 统计 / 调试 ====================

func stats() -> Dictionary:
	var cached_ok := 0
	for v in _seff_cache.values():
		if v != null:
			cached_ok += 1
	return {
		"root": assets_root,
		"enabled": enabled,
		"seff_queried": _seff_cache.size(),
		"seff_loaded": cached_ok,
		"bgm_queried": _bgm_cache.size(),
		"played": _play_count,
		"missed": _miss_count,
		"playing_sfx": _playing_count(),
	}


func _playing_count() -> int:
	var n := 0
	for p in _sfx_players:
		if p.playing:
			n += 1
	return n
