extends RefCounted
class_name SysConst
## 系统常量
##
## 【逐行翻译】common/src/sysconst.hpp
## 对照：sysconst.hpp:48-251
## 说明：纯常量，不做任何改动。原版 constexpr 在这里是 const。

# ---------------- 帧率与刷新（sysconst.hpp:48-50） ----------------
const SYS_PI := 3.14159265359
const SYS_DEFFPS := 10   ## frame per second, decides many motion/animation speed
const SYS_DEFRPS := 30   ## refresh per second, decides screen refresh rate

# ---------------- 目标区域间距（sysconst.hpp:52-53） ----------------
const SYS_TARGETRGN_GAPX := 10
const SYS_TARGETRGN_GAPY := 8

# ---------------- 地图格尺寸（sysconst.hpp:55-56） ----------------
const SYS_MAPGRIDXP := 48
const SYS_MAPGRIDYP := 32

# ---------------- 物件切片（sysconst.hpp:58-59） ----------------
const SYS_OBJMAXW := 3
const SYS_OBJMAXH := 25

# ---------------- 视野（sysconst.hpp:61-69） ----------------
const SYS_MAXR := 40
const SYS_MAPVISIBLEW := 60
const SYS_MAPVISIBLEH := 40
const SYS_MAPVISIBLECD := 100

const SYS_MAX_VIEWR := 40   ## for addDeleteList creation
const SYS_PLY_VIEWR := 15
const SYS_NPC_VIEWR := 8
const SYS_MON_VIEWR := 10   ## monster default view range

const SYS_TARGETSEC := 60   ## monster targeting expire time

# ---------------- 上限（sysconst.hpp:75-94） ----------------
const SYS_MAXDROPITEM := 10
const SYS_MAXTEAMMEMBER := 100
const SYS_MAXDROPITEMGRID := 81

const SYS_MINSPEED := 20
const SYS_DEFSPEED := 100
const SYS_MAXSPEED := 500

const SYS_INVGRIDGW := 10
const SYS_INVGRIDGH := 10
const SYS_INVGRIDPW := 38
const SYS_INVGRIDPH := 38
const SYS_INVGRIDMAXHOLD := 99

const SYS_MAXPRICE := (1 << 60) - 1
const SYS_MAXITEMCOUNT := SYS_MAXPRICE
const SYS_MAXGOLD := 0x7FFFFFFFFFFFFFFF   ## INT64_MAX

const SYS_MAXTARGET := 8
const SYS_MAXACTOR := 65521

# ---------------- 字符串尺寸（sysconst.hpp:96-110） ----------------
const SYS_IDSIZE := 64
const SYS_PWDSIZE := 64
const SYS_NAMESIZE := 64
const SYS_DELIVERYRECORDSIZE := 32
const SYS_AUCTIONNOTESIZE := 128
const SYS_AUCTIONLIFETIME := 7 * 24 * 60 * 60    ## 7 days
const SYS_AUCTIONTAXRATE := 2
const SYS_STALLMAXITEM := 36
const SYS_STALLNAMESIZE := 48
const SYS_GUILDNAMESIZE := 32
const SYS_GUILDTITLESIZE := 32
const SYS_GUILDNOTICESIZE := 1024
const SYS_GUILDWARTIME := 60 * 60
const SYS_SEFFSIZE := 16

# ---------------- 特殊值（sysconst.hpp:116-126） ----------------
const SYS_U32NIL := 0xFFFFFFFF
## GDScript 的 int 是有符号 64 位，无法表示 0xFFFFFFFFFFFFFFFF。
## 原版用于「无效 uint64 uid」，这里用 -1（位模式完全相同）代替。
const SYS_U64NIL := -1

const SYS_MAXDBID := 0xFFFFFF00
const SYS_CHATDBID_SYSTEM := SYS_MAXDBID + 1
const SYS_CHATDBID_GROUP := SYS_MAXDBID + 2
const SYS_CHATDBID_AI := SYS_MAXDBID + 3

const SYS_NAME_CHATDBID_SYSTEM := "系统助手"
const SYS_NAME_CHATDBID_GROUP := "群助手"
const SYS_NAME_CHATDBID_AI := "AI助手"

const SYS_MAXNPCDISTANCE := 10
const SYS_GOLDNAME := "金币（小）"

# ---------------- PK（sysconst.hpp:136-140） ----------------
## pk point never decays by itself, player pays a redemption NPC to bring it down
const SYS_PKPOINTPERKILL := 100
const SYS_PKPOINTREDNAME := 200
const SYS_PKAGGRESSIONTIME := 120

# ---------------- 耐久（sysconst.hpp:148-150） ----------------
const SYS_WEAPONDURALOSSODDS := 200   ## 1/N chance to lose one durability point per attack
const SYS_ARMORDURALOSSODDS := 300    ## 1/N chance to lose one durability point per hit taken
const SYS_DURALOSSRATE := 10

# ---------------- 等级（sysconst.hpp:211-251） ----------------
const SYS_MAXLEVEL := 1024


## 从 0 级练到 level 级所需总经验（sysconst.hpp:212）
static func SYS_SUMEXP(level: int) -> int:
	if level == 0:
		return 0
	var a := 100
	var b := 100
	var c := 100
	var d := 1000
	return a * level * level * level + b * level * level + c * level + d


## 由经验求等级信息（sysconst.hpp:228）
## 返回 {level, expInLevel, expForNextLevel}
static func SYS_LEVEL(exp: int) -> Dictionary:
	var max_exp := SYS_SUMEXP(SYS_MAXLEVEL)
	if exp >= max_exp:
		return {
			"level": SYS_MAXLEVEL,
			"exp": 0,
			"next": SYS_SUMEXP(SYS_MAXLEVEL + 1) - max_exp,
		}

	# 二分等价于原版 upper_bound
	var level := 0
	for lv in range(0, SYS_MAXLEVEL + 1):
		if SYS_SUMEXP(lv) > exp:
			level = maxi(0, lv - 1)
			break
		else:
			level = lv

	var base := SYS_SUMEXP(level)
	return {
		"level": level,
		"exp": exp - base,
		"next": SYS_SUMEXP(level + 1) - base,
	}


## SYS_MONSEFFBASE（sysconst.hpp:111）
static func SYS_MONSEFFBASE(look_id: int) -> int:
	return 0x02000000 + look_id * SYS_SEFFSIZE


# ---------------- 事件名（sysconst.hpp:169-209） ----------------
const SYS_NPCERROR := "_RSVD_NAME_NPC_ERROR_45421406723"
const SYS_ENTER := "_RSVD_NAME_ENTER_90360178872"
const SYS_DONE := "_RSVD_NAME_DONE__06562813788"
const SYS_EXIT := "_RSVD_NAME_EXIT__14208236065"
const SYS_ABORT := "_RSVD_NAME_ABORT_72061294738"

const SYS_EXECDONE := "_RSVD_NAME_EXEC_DONE___1952748411"
const SYS_EXECCLOSE := "_RSVD_NAME_EXEC_CLOSE__8553768451"
const SYS_EXECBADUID := "_RSVD_NAME_EXEC_BADUID_7259708252"
const SYS_EXECERROR := "_RSVD_NAME_EXEC_ERROR__3069184527"

const SYS_HIDE := "_RSVD_NAME_HIDE_3194448323"
const SYS_LABEL := "_RSVD_NAME_LABEL_78921733019"
const SYS_CHECKACTIVE := "_RSVD_NAME_CHECK_ACTIVE_4054544333"
const SYS_ALLOWREDNAME := "_RSVD_NAME_ALLOW_REDNAME_761479946"

const SYS_SPACEMOVE := "_RSVD_NAME_SPACE_MOVE_683761479946"
const SYS_SAFEZONE := "_RSVD_NAME_SAFE_ZONE_5182730946"

const SYS_EPDEF := "_RSVD_NAME_EVENT_PATH_DEF_2965316381"
const SYS_EPUID := "_RSVD_NAME_EVENT_PATH_UID_3623042653"
const SYS_EPQST := "_RSVD_NAME_EVENT_PATH_QST_6329204623"

const SYS_COOP := "_RSVD_NAME_COOP_293173013"
const SYS_QSTFSM := "_RSVD_NAME_QST_FSM_4194347313"
const SYS_FLAGVAL := "_RSVD_NAME_FLAG_VAL_8192362390"

const SYS_AFRESP := "_RSVD_NAME_AFRESP_8368138412597"
const SYS_GIRESP := "_RSVD_NAME_GIRESP_2961748350172"
const SYS_DELIVERY := "_RSVD_NAME_DELIVERY_5639021748316"
