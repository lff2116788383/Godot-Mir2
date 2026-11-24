extends RefCounted
class_name ProtocolDef
## 协议枚举全集
##
## 【逐行翻译】common/src/protocoldef.hpp（632 行）
## 说明：纯枚举，取值必须与 C++ 完全一致，顺序不可改。

# ---------------- 8 方向（protocoldef.hpp:20） ----------------
##               0
##            7     1
##          6    +--> 2
##            5  |  3
##               V
##               4
const DIR_NONE := 0
const DIR_BEGIN := 1
const DIR_UP := 1
const DIR_UPRIGHT := 2
const DIR_RIGHTUP := 2
const DIR_RIGHT := 3
const DIR_DOWNRIGHT := 4
const DIR_RIGHTDOWN := 4
const DIR_DOWN := 5
const DIR_DOWNLEFT := 6
const DIR_LEFTDOWN := 6
const DIR_LEFT := 7
const DIR_UPLEFT := 8
const DIR_LEFTUP := 8
const DIR_END := 9

# ---------------- 寻路格类型（protocoldef.hpp:47） ----------------
const PF_NONE := 0      ## grid outside of map
const PF_BEGIN := 1
const PF_OBSTACLE := 1
const PF_OCCUPIED := 2
const PF_LOCKED := 3
const PF_FREE := 4
const PF_END := 5

# ---------------- 动作扩展（protocoldef.hpp:59） ----------------
const ACTEXT_NONE := 0
const ACTEXT_FLYIN := 1
const ACTEXT_FLYOUT := 2
const ACTEXT_FADEIN := 3
const ACTEXT_FADEOUT := 4

# ---------------- LookID（protocoldef.hpp:70） ----------------
const LID_BEGIN := 0
const LID_END := 2048

# ---------------- 效果类型（protocoldef.hpp:76） ----------------
const EFF_NONE := 0
const EFF_HPSTEAL := (1 << 8)
const EFF_MPSTEAL := (2 << 8)
const EFF_PUNCH := (3 << 8)
const EFF_STONE := (4 << 8)

# ---------------- 性别（protocoldef.hpp:86） ----------------
const GT_NONE := 0
const GT_MALE := 1
const GT_FEMALE := 2

# ---------------- 怪物攻击类型（protocoldef.hpp:93） ----------------
const MA_NONE := 0
const MA_NORMAL := 1
const MA_DOGZ := 2
const MA_ATTACKALL := 3

# ---------------- 装备 ID 范围（protocoldef.hpp:101-127） ----------------
const WEAPON_NONE := 0
const WEAPON_BEGIN := 1
const WEAPON_END := 256

const HELMET_NONE := 0
const HELMET_BEGIN := 1
const HELMET_END := 256

const DRESS_NONE := -1
const DRESS_BEGIN := 0     ## zero uses naked gfx
const DRESS_END := 256

const HAIR_NONE := 0
const HAIR_BEGIN := 1
const HAIR_END := 256

# ---------------- NPC 错误（protocoldef.hpp:129） ----------------
const NPCE_NONE := 0
const NPCE_TOOFAR := 1
const NPCE_BADEVENTID := 2
const NPCE_STALLSELLING := 3

# ---------------- 职业（protocoldef.hpp:137，位掩码） ----------------
const JOB_WARRIOR := 1 << 0
const JOB_TAOIST := 1 << 1
const JOB_WIZARD := 1 << 2
const JOB_VALMASK := JOB_WARRIOR | JOB_TAOIST | JOB_WIZARD

# ---------------- 穿戴槽位（protocoldef.hpp:175） ----------------
const WLG_NONE := 0
const WLG_BEGIN := 1

const WLG_L_BEGIN := WLG_BEGIN
const WLG_DRESS := WLG_L_BEGIN
const WLG_HELMET := 2
const WLG_WEAPON := 3
const WLG_L_END := 4

const WLG_W_BEGIN := WLG_L_END
const WLG_SHOES := WLG_W_BEGIN
const WLG_NECKLACE := 5
const WLG_ARMRING0 := 6
const WLG_ARMRING1 := 7
const WLG_RING0 := 8
const WLG_RING1 := 9
const WLG_TORCH := 10
const WLG_CHARM := 11
const WLG_W_END := 12

const WLG_END := WLG_W_END

# ---------------- 拍卖分类（protocoldef.hpp:200） ----------------
const AUCTIONCAT_NONE := 0
const AUCTIONCAT_BEGIN := 1
const AUCTIONCAT_ALL := 1
const AUCTIONCAT_DRESS := 2
const AUCTIONCAT_WEAPON := 3
const AUCTIONCAT_NECKLACE := 4
const AUCTIONCAT_HELMET := 5
const AUCTIONCAT_RING := 6
const AUCTIONCAT_ARMRING := 7
const AUCTIONCAT_SHOES := 8
const AUCTIONCAT_POTION := 9
const AUCTIONCAT_BOOK := 10
const AUCTIONCAT_OTHER := 11
const AUCTIONCAT_END := 12

# ---------------- 直接交易错误（protocoldef.hpp:220） ----------------
const DTRADEERR_NONE := 0
const DTRADEERR_BADTARGET := 1
const DTRADEERR_NOTALLOWED := 2
const DTRADEERR_TARGETNOTALLOWED := 3
const DTRADEERR_BUSY := 4
const DTRADEERR_TARGETBUSY := 5
const DTRADEERR_DEAD := 6
const DTRADEERR_TARGETDEAD := 7
const DTRADEERR_TOOFAR := 8
const DTRADEERR_REJECTED := 9
const DTRADEERR_OFFLINE := 10
const DTRADEERR_BADOFFER := 11
const DTRADEERR_BADGOLD := 12
const DTRADEERR_LOCKED := 13
const DTRADEERR_NOTREADY := 14
const DTRADEERR_COMMITFAILED := 15
const DTRADEERR_STALLSELLING := 16

# ---------------- 摊位错误（protocoldef.hpp:241） ----------------
const STALLERR_NONE := 0
const STALLERR_NOTALLOWED := 1
const STALLERR_BUSY := 2
const STALLERR_DEAD := 3
const STALLERR_SELLING := 4
const STALLERR_NOTSELLING := 5
const STALLERR_FULL := 6
const STALLERR_EMPTY := 7
const STALLERR_BADITEM := 8
const STALLERR_BADPRICE := 9
const STALLERR_BADNAME := 10
const STALLERR_BADCOUNT := 11
const STALLERR_NOGOLD := 12
const STALLERR_SELFBUY := 13
const STALLERR_STALE := 14
const STALLERR_DBFAILED := 15

# ---------------- 行会（protocoldef.hpp:261-291） ----------------
const GUILDRANK_NONE := 0
const GUILDRANK_BEGIN := 1
const GUILDRANK_LEADER := 1
const GUILDRANK_MEMBER := 2
const GUILDRANK_END := 3

const GUILDERR_NONE := 0
const GUILDERR_INGUILD := 1
const GUILDERR_NOGUILD := 2
const GUILDERR_NOTLEADER := 3
const GUILDERR_BADNAME := 4
const GUILDERR_NAMETAKEN := 5
const GUILDERR_NOGOLD := 6
const GUILDERR_NOITEM := 7
const GUILDERR_BADTARGET := 8
const GUILDERR_TARGETINGUILD := 9
const GUILDERR_NOTMEMBER := 10
const GUILDERR_LEADERLEAVE := 11   ## leader transfers the guild or disbands it, never just leaves
const GUILDERR_NOINVITE := 12
const GUILDERR_REJECTED := 13
const GUILDERR_NOPLAYER := 14
const GUILDERR_TARGETNOTALLOWED := 15
const GUILDERR_BADTITLE := 16
const GUILDERR_BADNOTICE := 17
const GUILDERR_SELFWAR := 18

# ---------------- 属性（protocoldef.hpp:293） ----------------
const ATR_NONE := 0
const ATR_BEGIN := 1
const ATR_DC := 1
const ATR_MC := 2
const ATR_SC := 3
const ATR_AC := 4
const ATR_MAC := 5
const ATR_HP := 6
const ATR_MP := 7
const ATR_HPR := 8
const ATR_MPR := 9
const ATR_HIT := 10
const ATR_SPD := 11
const ATR_END := 12

# ---------------- 购买错误（protocoldef.hpp:311） ----------------
const BUYERR_NONE := 0
const BUYERR_BEGIN := 1
const BUYERR_BADITEM := 1
const BUYERR_LOCKED := 2
const BUYERR_SOLDOUT := 3
const BUYERR_INSUFFCIENT := 4
const BUYERR_STALLSELLING := 5
const BUYERR_BADCOUNT := 6
const BUYERR_END := 7

# ---------------- 穿戴错误（protocoldef.hpp:324-365） ----------------
const EQWERR_NONE := 0
const EQWERR_BEGIN := 1
const EQWERR_NOITEM := 1
const EQWERR_BADITEM := 2
const EQWERR_BADWLTYPE := 3
const EQWERR_INSUFF := 4
const EQWERR_STALLSELLING := 5
const EQWERR_END := 6

const GWERR_NONE := 0
const GWERR_BEGIN := 1
const GWERR_NOITEM := 1
const GWERR_BIND := 2
const GWERR_STALLSELLING := 3
const GWERR_END := 4

const EQBERR_NONE := 0
const EQBERR_BEGIN := 1
const EQBERR_NOITEM := 1
const EQBERR_BADITEM := 2
const EQBERR_BADITEMTYPE := 3
const EQBERR_BADSLOT := 4
const EQBERR_STALLSELLING := 5
const EQBERR_END := 6

const GBERR_NONE := 0
const GBERR_BEGIN := 1
const GBERR_NOITEM := 1
const GBERR_STALLSELLING := 2
const GBERR_END := 3

# ---------------- 账号/登录错误（protocoldef.hpp:367-441） ----------------
const CRTACCERR_NONE := 0
const CRTACCERR_BEGIN := 1
const CRTACCERR_ACCOUNTEXIST := 1
const CRTACCERR_BADACCOUNT := 2
const CRTACCERR_BADPASSWORD := 3
const CRTACCERR_END := 4

const CHGPWDERR_NONE := 0
const CHGPWDERR_BEGIN := 1
const CHGPWDERR_BADACCOUNT := 1
const CHGPWDERR_BADPASSWORD := 2
const CHGPWDERR_BADNEWPASSWORD := 3
const CHGPWDERR_BADACCOUNTPASSWORD := 4
const CHGPWDERR_END := 5

const LOGINERR_NONE := 0
const LOGINERR_BEGIN := 1
const LOGINERR_NOACCOUNT := 1
const LOGINERR_MULTILOGIN := 2
const LOGINERR_END := 3

const CRTCHARERR_NONE := 0
const CRTCHARERR_BEGIN := 1
const CRTCHARERR_NOLOGIN := 1
const CRTCHARERR_CHAREXIST := 2
const CRTCHARERR_NAMEEXIST := 3
const CRTCHARERR_BADNAME := 4
const CRTCHARERR_BADGENDER := 5
const CRTCHARERR_BADJOB := 6
const CRTCHARERR_DBERROR := 7
const CRTCHARERR_END := 8

const QUERYCHARERR_NONE := 0
const QUERYCHARERR_BEGIN := 1
const QUERYCHARERR_NOLOGIN := 1
const QUERYCHARERR_NOCHAR := 2
const QUERYCHARERR_END := 3

const DELCHARERR_NONE := 0
const DELCHARERR_BEGIN := 1
const DELCHARERR_NOLOGIN := 1
const DELCHARERR_NOCHAR := 2
const DELCHARERR_BADPASSWORD := 3
const DELCHARERR_DBERROR := 4
const DELCHARERR_HOSTED := 5
const DELCHARERR_END := 6

const ONLINEERR_NONE := 0
const ONLINEERR_BEGIN := 1
const ONLINEERR_NOLOGIN := 1
const ONLINEERR_MULTIONLINE := 2
const ONLINEERR_NOCHAR := 3
const ONLINEERR_AMERROR := 4
const ONLINEERR_END := 5

# ---------------- Buff（protocoldef.hpp:443-458） ----------------
const BFT_NONE := 0
const BFT_BEGIN := 1
const BFT_SHIELD := 1
const BFT_END := 2

const BFS_NONE := 0
const BFS_BEGIN := 1
const BFS_ON := 1
const BFS_OFF := 2
const BFS_END := 3

# ---------------- 背包操作（protocoldef.hpp:460） ----------------
const INVOP_NONE := 0
const INVOP_BEGIN := 1
const INVOP_TRADE := 1
const INVOP_SECURE := 2
const INVOP_REPAIR := 3
const INVOP_STALL := 4    ## client only, select items to put on stall
const INVOP_END := 5

# ---------------- 攻击模式（protocoldef.hpp:471） ----------------
const ATKMODE_NONE := 0
const ATKMODE_BEGIN := 1
const ATKMODE_PEACE := 1
const ATKMODE_GROUP := 2
const ATKMODE_GUILD := 3
const ATKMODE_ALL := 4
const ATKMODE_END := 5


## attackModeName（protocoldef.hpp:483）
static func attack_mode_name(mode: int) -> String:
	match mode:
		ATKMODE_PEACE: return "和平"
		ATKMODE_GROUP: return "编队"
		ATKMODE_GUILD: return "行会"
		ATKMODE_ALL: return "全体"
	return "未知"


# ---------------- 组队（protocoldef.hpp:494-509） ----------------
const TEAMERR_NONE := 0
const TEAMERR_BEGIN := 1
const TEAMERR_INTEAM := 1
const TEAMERR_NOTLEADER := 2
const TEAMERR_BADREQUIREMENT := 3
const TEAMERR_NOTEAM := 4
const TEAMERR_FULL := 5
const TEAMERR_REQUIREMENT := 6
const TEAMERR_BADCANDIDATE := 7
const TEAMERR_TRANSFERLEADER := 8
const TEAMERR_KICKED := 9
const TEAMERR_DISBANDED := 10
const TEAMERR_END := 11

# ---------------- 好友（protocoldef.hpp:511-542） ----------------
const AF_NONE := -1
const AF_BEGIN := 0
const AF_INVALID := 0
const AF_BLOCKED := 1
const AF_ACCEPTED := 2
const AF_REJECTED := 3
const AF_PENDING := 4
const AF_EXIST := 5
const AF_END := 6

const FR_NONE := -1
const FR_BEGIN := 0
const FR_ACCEPT := 0
const FR_REJECT := 1
const FR_VERIFY := 2
const FR_END := 3

const BP_NONE := 0
const BP_BEGIN := 1
const BP_INVALID := 1
const BP_DONE := 2
const BP_EXIST := 3
const BP_END := 4

# ---------------- 聊天对端（protocoldef.hpp:544） ----------------
const CPR_NONE := 0
const CPR_BEGIN := 1
const CPR_SPECIAL := 1
const CPR_PLAYER := 2
const CPR_GROUP := 3
const CPR_END := 4

# ---------------- IME / 窗口（protocoldef.hpp:555-573） ----------------
const IME_NONE := 0
const IME_BEGIN := 1
const IME_DISABLE := 1
const IME_EMBEDED := 2
const IME_SYSTEM := 3
const IME_END := 4

const WMT_NONE := 0
const WMT_BEGIN := 1
const WMT_WINDOWED := 1
const WMT_MAXIMIZED := 2
const WMT_FULLSCREEN := 3
const WMT_END := 4

# ---------------- 掉落标记（protocoldef.hpp:575） ----------------
const DIRF_HIGHLIGHT := 1 << 0
const DIRF_FILTER := 1 << 1
const DIRF_VALMASK := DIRF_HIGHLIGHT | DIRF_FILTER

# ---------------- 排行榜（protocoldef.hpp:583） ----------------
const RANKING_LEVEL := 0
const RANKING_GOLD := 1
const RANKING_END := 2

# ---------------- 其他错误（protocoldef.hpp:590-630） ----------------
const CDERR_NONE := 0
const CDERR_BEGIN := 1
const CDERR_UNKNOWN_ERROR := 1
const CDERR_INVALID_CLAIM := 2
const CDERR_INVALID_RECORD := 3
const CDERR_GOLDFULL := 4
const CDERR_END := 5

const AUCTIONREGERR_NONE := 0
const AUCTIONREGERR_BEGIN := 1
const AUCTIONREGERR_BADITEM := 1
const AUCTIONREGERR_BADPRICE := 2
const AUCTIONREGERR_BADNOTE := 3
const AUCTIONREGERR_STALLSELLING := 4
const AUCTIONREGERR_END := 5

const AUCTIONBUYERR_NONE := 0
const AUCTIONBUYERR_BEGIN := 1
const AUCTIONBUYERR_BADITEM := 1
const AUCTIONBUYERR_UNAVAILABLE := 2
const AUCTIONBUYERR_INSUFFICIENT := 3
const AUCTIONBUYERR_OWNITEM := 4
const AUCTIONBUYERR_END := 5

const AUCTIONUNREGERR_NONE := 0
const AUCTIONUNREGERR_BEGIN := 1
const AUCTIONUNREGERR_BADITEM := 1
const AUCTIONUNREGERR_UNAVAILABLE := 2
const AUCTIONUNREGERR_END := 3


# ---------------- 便捷判定 ----------------

## valmaskCheck（protocoldef.hpp:6）
static func valmask_check(val_mask: int, value: int) -> bool:
	return (value & ~val_mask) == 0


static func dir_valid(d: int) -> bool:
	return d >= DIR_BEGIN and d < DIR_END


static func job_valid(job: int) -> bool:
	return valmask_check(JOB_VALMASK, job) and job != 0
