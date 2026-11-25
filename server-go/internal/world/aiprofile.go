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

	// ---- 祖玛弓箭手：远程物理 ----
	"祖玛弓箭手": {
		Specials: []SpellAbility{
			{Name: "射箭", MagicID: 99, CastRange: 8, CoolDownMS: 1500},
		},
	},

	// ---- 神兽：宠物 ----
	//
	// 【依据】servertaodog.cpp:9-47：
	//
	//	有目标 → trackAttackUID
	//	无目标 → 主人已死则自己死；否则 followMaster
	//	空闲超过 30 秒 → setStandMode(false) 回站立
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
	// 用 WalkWait 设成一个很大的值来达到同样效果 ——
	// 加一个 CanMove 字段会引入「有的怪不能动但能转身」这种
	// 需要单独处理的状态，用超长 walkWait 更简单且行为一致。
	"食人花": {
		Specials: []SpellAbility{
			{Name: "食人花_攻击", MagicID: MagicIDPlainAttack, CastRange: 2, CoolDownMS: 1000},
		},
	},
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

	// 食人花这类「炮台怪」：把它变成不能移动
	if name == "食人花" {
		rec.WalkWait = immobileWalkWaitMS
	}
}
