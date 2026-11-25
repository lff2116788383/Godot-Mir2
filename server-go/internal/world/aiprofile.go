package world

// ==================== 怪物子类 AI 配置表 ====================
//
// 原版用 C++ 继承实现子类，19 个文件共 803 行：
//
//	serverzumataurus.cpp      222  祖玛教主（远程地狱火 + 5 秒火墙）
//	servertaodog.cpp          167  神兽（喷火 + 跟随主人 + 主人死则死）
//	serverguard.cpp           140  守卫（只打红名，已在 BehaveMode 里实现）
//	serverminotaurguardian.cpp 122 牛魔守卫（强化攻击）
//	serverzumamonster.cpp      46  祖玛小怪
//	servercannibalplant.cpp    35  食人花（不移动）
//	serveranthealer.cpp        71  蚁后（治疗周围）
//	... 其余 13 个
//
// Go 侧没有继承，用**数据驱动的 profile**。理由：
// 子类之间的差异其实只有「有没有远程技能」「冷却多久」「要不要跟随主人」
// 这几点，砍掉继承层次反而更容易看出「这个 Boss 和普通怪差在哪」。

// ProfileByMonsterName 按怪物名返回 AI 差异配置。
//
// 返回 nil 表示「标准怪」：追击 + 攻击 + 游荡。
//
// ★ 为什么按名字而不是按 MonsterID：
//
//	MonsterID 是 monsterrecord.inc 的数组下标，重新生成表会整体错位；
//	名字是稳定的语义标识。性能上这张表只有十几项，
//	而且只在刷怪时查一次（不是每次 AI 决策），字符串比较完全无压力。
func ProfileByMonsterName(name string) *AIProfile {
	return aiProfiles[name]
}

var aiProfiles = map[string]*AIProfile{
	// ==================== 全部 22 个子类 ====================
	//
	// ★★ 名称必须与 monsterrecord.inc 完全一致 ★★
	//
	// 这张表的 key 是**原版 enableaddco.cpp:addMonster 里
	// DBCOM_MONSTERID(u8"...") 的那串名字**，逐个对照抄下来。
	//
	// 曾经踩过的坑：凭「感觉」写成「牛魔守卫」「沉船领主」「根须蜘蛛」，
	// 结果 24 个配置里有 11 个在怪物表里根本不存在 ——
	// 查表返回 nil，静默退化成普通怪，**没有任何报错**。
	// 表现是「Boss 不掉血也不放技能」，排查时完全想不到是名字写错了。
	//
	// 真实名称（enableaddco.cpp 的 29 个 case 去重后 22 个）：
	//
	//	变异骷髅 超强骷髅 神兽 食人花 角蝇 栗子树 沙漠树魔 掷斧骷髅
	//	触角神魔 沙鬼 僵尸_1 蚂蚁道士 沃玛教主 触龙神 祖玛雕像
	//	祖玛教主 爆裂蜘蛛 幻影蜘蛛 赤月恶魔 霸王教主 潘夜左护卫 潘夜右护卫

	// ---- 祖玛教主：远程法师型 Boss ----
	//
	// 【依据】serverzumataurus.cpp:7-56：
	//
	//	if(inDCCastRange(target, 地狱火.castRange))                   地狱火
	//	else if(火墙 冷却已过 && inDCCastRange(target, 火墙.castRange)) 火墙
	//	else                                                          追击
	//
	// 顺序不能反：地狱火是主力输出，火墙是「够不到时补一发」。
	"祖玛教主": {
		Specials: []SpellAbility{
			{Name: "地狱火", MagicID: 102, CastRange: 6, CoolDownMS: 1000},
			{Name: "火墙", MagicID: 101, CastRange: 6, CoolDownMS: 5000},
		},
		// 索敌比视野远 2 格：玩家还没看到它就被盯上（Boss 的压迫感）
		AggroRangeOverride: 12,
	},

	// ---- 神兽：道士宠物 ----
	//
	// 【依据】servertaodog.cpp:9-47
	"神兽": {
		Specials: []SpellAbility{
			{Name: "喷火", MagicID: 77, CastRange: 2, CoolDownMS: 1200},
		},
		FollowMaster:     true,
		DieWithMaster:    true,
		IdleStandAfterMS: 30000,
	},

	// ---- 食人花：不能移动的炮台 ----
	//
	// 【依据】servercannibalplant.cpp：canMove() 恒 false。
	// 用 WalkWait 设成很大值达到同样效果（见 immobileMonsters）。
	"食人花": {
		Specials: []SpellAbility{
			{Name: "食人花_攻击", MagicID: MagicIDPlainAttack, CastRange: 2, CoolDownMS: 1000},
		},
	},

	// ---- 沃玛教主：电光 + 雷电术（远程双技能）----
	//
	// 【依据】serverwoomataurus.cpp:22-70：
	//   电光射程内 → 电光
	//   雷电术射程内 → 雷电术，命中后 300ms 对周围友军各补一发（连锁）
	"沃玛教主": {
		Specials: []SpellAbility{
			{Name: "沃玛教主_电光", MagicID: 103, CastRange: 7, CoolDownMS: 1200},
			{Name: "沃玛教主_雷电术", MagicID: 104, CastRange: 5, CoolDownMS: 3000},
		},
		// 雷电术命中后的连锁：对 1 格内其他目标再打一发
		ChainOnHit: []SpellAbility{
			{Name: "沃玛教主_雷电术", MagicID: 104, CastRange: 1, CoolDownMS: 0},
		},
		AggroRangeOverride: 11,
	},

	// ---- 潘夜左/右护卫：火/电 双修远程 ----
	//
	// 【依据】serverminotaurguardian.cpp（122 行）：
	//   左护卫用火魔杖 + 火球术，右护卫用电魔杖 + 雷电术。
	//   两者互相「看见对方就补一发」（互为友军支援）。
	"潘夜左护卫": {
		Specials: []SpellAbility{
			{Name: "潘夜左护卫_火魔杖", MagicID: 112, CastRange: 2, CoolDownMS: 900},
			{Name: "潘夜左护卫_火球术", MagicID: 113, CastRange: 6, CoolDownMS: 2000},
		},
	},
	"潘夜右护卫": {
		Specials: []SpellAbility{
			{Name: "潘夜右护卫_电魔杖", MagicID: 114, CastRange: 2, CoolDownMS: 900},
			{Name: "潘夜右护卫_雷电术", MagicID: 115, CastRange: 6, CoolDownMS: 2000},
		},
	},

	// ---- 蚂蚁道士：自愈 + 治疗友军 ----
	//
	// 【依据】serveranthealer.cpp:5-60：
	//   hp < maxHP → updateHealth(20)，等 1 秒
	//   视野内每个友军 → +10HP/+5MP
	//
	// ★ 这是唯一一个「会奶队友」的怪。没有它，
	//   蚂蚁群就只是「一群普通怪」，打起来毫无层次。
	"蚂蚁道士": {
		SelfHealHP:         20,
		SelfHealMP:         10,
		SelfHealIntervalMS: 1000,
		HealAllyHP:         10,
		HealAllyMP:         5,
		HealAllyMagicID:    107,
		HealAllyRange:      6,
	},

	// ---- 沙漠树魔：远程喷刺（扎根不动）----
	//
	// 【依据】serversandcactus.cpp:7-35
	"沙漠树魔": {
		Specials: []SpellAbility{
			{Name: "沙漠树魔_喷刺", MagicID: 108, CastRange: 5, CoolDownMS: 1500},
		},
	},

	// ---- 赤月恶魔：远程暗系 ----
	//
	// 【依据】serverredmoonevil.cpp
	"赤月恶魔": {
		Specials: []SpellAbility{
			{Name: "赤月恶魔_攻击", MagicID: 110, CastRange: 7, CoolDownMS: 2000},
		},
		AggroRangeOverride: 11,
	},

	// ---- 霸王教主：沉船领主原型，远程 + 大索敌 ----
	//
	// 【依据】servershipwrecklord.cpp（65 行）
	"霸王教主": {
		Specials: []SpellAbility{
			{Name: "霸王教主_攻击", MagicID: 109, CastRange: 6, CoolDownMS: 1800},
		},
		AggroRangeOverride: 12,
	},

	// ---- 爆裂蜘蛛：死时自爆 ----
	//
	// 【依据】serverbombspider.cpp：死亡时对周围造成伤害。
	// ★ 真正的「死亡触发」需要一个死亡钩子，当前 KillSettler 没有。
	//   这里用 1 格射程的 0 冷却技能近似（死前最后一次攻击）。
	//   已知限制见 REPLICA_AUDIT。
	"爆裂蜘蛛": {
		Specials: []SpellAbility{
			{Name: "爆裂蜘蛛_自爆", MagicID: 111, CastRange: 1, CoolDownMS: 0},
		},
	},

	// ---- 变异骷髅 / 超强骷髅：道士召唤物 ----
	//
	// 【依据】servertaoskeleton.cpp / servertaoskeletonext.cpp：
	//   跟随主人、主人死则自己死；超强骷髅攻速更快。
	"变异骷髅": {
		FollowMaster:  true,
		DieWithMaster: true,
	},
	"超强骷髅": {
		FollowMaster:  true,
		DieWithMaster: true,
	},

	// ---- 以下为「已核对、确认无特殊行为」的标准怪 ----
	//
	// 显式列成空结构而不是省略，是为了让查表的人看到
	// 「这个怪有人对过了」—— 省略的话下次还会有人重新核对一遍。
	"角蝇":   {},
	"栗子树":  {},
	"掷斧骷髅": {},
	"触角神魔": {},
	"沙鬼":   {},
	"僵尸_1": {},
	"触龙神":  {},
	"祖玛雕像": {},
	"祖玛卫士": {},
	"幻影蜘蛛": {},
}

// immobileMonsters 是完全不能移动的怪物（原版 canMove() 恒 false）。
//
// ★ 单独列一张表而不是塞进 AIProfile：
//
//	「不能移动」是 monsterrecord 的属性（改 WalkWait），
//	不是 AI 决策的一部分。混进 profile 会让 ApplyProfile 里
//	出现「怪物的移动性由 AI 配置决定」这种奇怪的耦合。
var immobileMonsters = map[string]bool{
	"食人花":  true,
	"沙漠树魔": true, // 沙虫是扎根的
	"树妖":   true,
	"邪恶触须": true,
}

// ApplyProfile 把 profile 的配置合并进 MonsterRecord。
//
// ★ 食人花的「不能移动」通过 WalkWait 实现：
//
//	WalkWait 是「两次移动之间的最小毫秒数」。
//	设成 1 小时 = 1 tick 内不可能再移动，效果等同于 cannot move。
//	比加一个 CanMove 字段更省 —— 后者要在 chaseStep / followMaster
//	每一处都判一次，漏一处就又能走了。
const immobileWalkWaitMS = 3600 * 1000

// ApplyProfile 把子类配置应用到怪物记录上（刷怪时调一次）。
func ApplyProfile(rec *MonsterRecord, name string) {
	profile := ProfileByMonsterName(name)
	if profile == nil {
		return
	}
	rec.Profile = profile

	// 不能移动的怪（食人花/沙虫/树妖/触须）
	if immobileMonsters[name] {
		rec.WalkWait = immobileWalkWaitMS
	}
}
