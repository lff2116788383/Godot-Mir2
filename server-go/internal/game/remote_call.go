package game

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// ==================== uidRemoteCall 的玩家侧执行 ====================
//
// 【依据】serverluacoroutinerunner.cpp:217 + player.cpp:60-300 的 bindFunction 列表
//
// ★★ 先讲清楚这里为什么不是「真的远程调用」★★
//
// 原版每个 player/monster/npc 都是一个 actor，各自带一个 Lua state。
// `uidRemoteCall(playerUID, args..., code)` 会把 code 发给玩家 actor，
// **在玩家的 Lua state 里执行** —— 所以 code 里能直接调 getGold()/addItem()，
// 那些绑定是注册在 player.cpp 的（属于玩家范围）。
//
// Go 侧没有 per-actor Lua state：给几千个在线玩家各起一个 gopher-lua
// 既费内存（每个 state 起步几 MB），初始化又要跑一遍 base.lua +
// 340 个 NPC 的 require 链。所以这里改了实现策略：
//
//	**按代码串识别意图，在 Go 里执行等价操作。**
//
// 好处是零 Lua 开销；代价是「只支持已知的代码串形态」。
// 这不是偷懒 —— 脚本用的是 api/player.lua 里那几个**固定模板**
// （`return getGold()`、`local item, count = ... return addItem(item, count)`），
// 全库 16 个不同形态，穷举完全可行。
//
// ★ 未识别的代码串会**返回错误并打日志**，不静默返回 nil。
//
//	静默返回 nil 的后果：`if uidRemoteCall(...) then` 走 else 分支，
//	脚本会认为「操作失败」并给玩家发错误提示 —— 看起来像业务失败，
//	实际是引擎不支持。日志是唯一的排查线索。
//
// 【已知限制】见 REPLICA_AUDIT。

// remoteArgEcho 把参数原样返回（用于 `return getGold()` 这类无参调用前
// 先确认参数表长度）。
var remoteCallPatterns = []struct {
	name  string
	re    *regexp.Regexp
	arity int // code 期待的参数个数（-1 表示可变）
	run   func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error)
}{
	// ---- 查询类 ----
	{
		name: "getGold", arity: 0,
		re: regexp.MustCompile(`^returngetGold\(\)$`),
		run: func(h *npcHostAdapter, p *Player, _ []script.RemoteArg) ([]script.RemoteArg, error) {
			return []script.RemoteArg{script.RemoteNumber(float64(p.Gold))}, nil
		},
	},
	{
		name: "getLevel", arity: 0,
		re:   regexp.MustCompile(`^returngetLevel\(\)$`),
		run: func(h *npcHostAdapter, p *Player, _ []script.RemoteArg) ([]script.RemoteArg, error) {
			return []script.RemoteArg{script.RemoteNumber(float64(p.Level))}, nil
		},
	},
	{
		name: "getName", arity: 0,
		re:   regexp.MustCompile(`^returngetName\(\)$`),
		run: func(h *npcHostAdapter, p *Player, _ []script.RemoteArg) ([]script.RemoteArg, error) {
			return []script.RemoteArg{script.RemoteString(p.Name)}, nil
		},
	},
	{
		name: "getGender", arity: 0,
		re:   regexp.MustCompile(`^returngetGender\(\)$`),
		run: func(h *npcHostAdapter, p *Player, _ []script.RemoteArg) ([]script.RemoteArg, error) {
			return []script.RemoteArg{script.RemoteNumber(float64(p.Gender))}, nil
		},
	},
	{
		name: "getGuildName", arity: 0,
		// 不在公会时返回 nil（原版返回 std::nullopt → Lua nil）
		re: regexp.MustCompile(`^returngetGuildName\(\)$`),
		run: func(h *npcHostAdapter, p *Player, _ []script.RemoteArg) ([]script.RemoteArg, error) {
			if h.Guilds == nil {
				return []script.RemoteArg{{Kind: script.RemoteArgNil, IsNil: true}}, nil
			}
			member, found, err := h.Guilds.LoadMembership(context.Background(), p.DBID)
			if err != nil || !found || member.GuildID == 0 {
				return []script.RemoteArg{{Kind: script.RemoteArgNil, IsNil: true}}, nil
			}
			return []script.RemoteArg{script.RemoteString(member.GuildName)}, nil
		},
	},
	{
		name: "isGuildLeader", arity: 0,
		re:   regexp.MustCompile(`^returnisGuildLeader\(\)$`),
		run: func(h *npcHostAdapter, p *Player, _ []script.RemoteArg) ([]script.RemoteArg, error) {
			if h.Guilds == nil {
				return []script.RemoteArg{script.RemoteBool(false)}, nil
			}
			member, found, err := h.Guilds.LoadMembership(context.Background(), p.DBID)
			if err != nil || !found {
				return []script.RemoteArg{script.RemoteBool(false)}, nil
			}
			// GUILDRANK_LEADER == 1
			return []script.RemoteArg{script.RemoteBool(member.Rank == storage.GuildRankLeader)}, nil
		},
	},
	{
		name: "getJobList", arity: 0,
		re:   regexp.MustCompile(`^returngetJobList\(\)$`),
		run: func(h *npcHostAdapter, p *Player, _ []script.RemoteArg) ([]script.RemoteArg, error) {
			// 多职业（原版 getJobList 返回 list）当前只支持单职业：
			// 返回一个元素的「表」在 RemoteArg 里表达不了，
			// 所以退化成一个字符串（hasJob 的实现依赖遍历，
			// 见下面的 hasJob 分支，它直接查 p.Job 而不走这个）。
			return []script.RemoteArg{script.RemoteString(jobName(p.Job))}, nil
		},
	},
	{
		name: "getRedName", arity: 0,
		re:   regexp.MustCompile(`^returngetRedName\(\)$`),
		run: func(h *npcHostAdapter, p *Player, _ []script.RemoteArg) ([]script.RemoteArg, error) {
			return []script.RemoteArg{script.RemoteBool(p.PKPoint >= SysPKPointRedName)}, nil
		},
	},
	{
		name: "getViewRange", arity: 0,
		re:   regexp.MustCompile(`^returngetViewRange\(\)$`),
		run: func(h *npcHostAdapter, p *Player, _ []script.RemoteArg) ([]script.RemoteArg, error) {
			return []script.RemoteArg{script.RemoteNumber(float64(p.ViewRange))}, nil
		},
	},

	// ---- 带参 ----
	{
		name: "hasMagic", arity: 1,
		// player.lua: `local n = ... return hasMagic(n)`
		re: regexp.MustCompile(`^local\w+=\.\.\.returnhasMagic\(\w+\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("hasMagic needs 1 arg")
			}
			name := args[0].AsString()
			if h.Magics != nil {
				if id, found := h.Magics.FindByName(name); found {
					return []script.RemoteArg{script.RemoteBool(p.HasMagic(id))}, nil
				}
			}
			return []script.RemoteArg{script.RemoteBool(false)}, nil
		},
	},
	{
		name: "removeGold", arity: 1,
		// player.lua: `local count = ... return removeGold(count)`
		re: regexp.MustCompile(`^local\w+=\.\.\.returnremoveGold\(\w+\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("removeGold needs 1 arg")
			}
			amount := uint64(args[0].AsNumber())
			ok := h.RemoveGold(p.UID, amount)
			return []script.RemoteArg{script.RemoteBool(ok)}, nil
		},
	},
	{
		name: "addExp", arity: 1,
		// player.lua: `local exp = ... return addExp(exp)`
		re: regexp.MustCompile(`^local\w+=\.\.\.returnaddExp\(\w+\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("addExp needs 1 arg")
			}
			if h.Shops == nil {
				return []script.RemoteArg{script.RemoteBool(false)}, nil
			}
			h.Shops.GainExp(context.Background(), p, uint64(args[0].AsNumber()))
			return []script.RemoteArg{script.RemoteBool(true)}, nil
		},
	},
	{
		name: "addItem", arity: 2,
		// player.lua: `local item, count = ... return addItem(item, count)`
		re: regexp.MustCompile(`^local(\w+),(\w+)=\.\.\.returnaddItem\(\w+,\w+\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("addItem needs 2 args")
			}
			itemID := uint32(args[0].AsNumber())
			count := uint32(args[1].AsNumber())
			if count == 0 {
				count = 1
			}
			if h.Items == nil {
				return []script.RemoteArg{script.RemoteBool(false)}, nil
			}
			rec, found := h.Items.Get(itemID)
			if !found {
				return []script.RemoteArg{script.RemoteBool(false)}, nil
			}
			p.AddItem(rec, Item{ItemID: itemID, Count: count})
			h.pushInventory(p)
			return []script.RemoteArg{script.RemoteBool(true)}, nil
		},
	},
	{
		name: "removeItem", arity: 3,
		// player.lua: `local item, seq, count = ... return removeItem(item, seq, count)`
		re: regexp.MustCompile(`^local\w+,\w+,\w+=\.\.\.returnremoveItem\([^)]*\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("removeItem needs 3 args")
			}
			itemID := uint32(args[0].AsNumber())
			seqID := uint32(args[1].AsNumber())
			count := uint32(args[2].AsNumber())
			ok := p.removeItem(itemID, seqID, count)
			if ok {
				h.pushInventory(p)
			}
			return []script.RemoteArg{script.RemoteBool(ok)}, nil
		},
	},
	{
		name: "hasItem", arity: 3,
		// player.lua: `local item, seq, count = ... return hasItem(item, seq, count)`
		re: regexp.MustCompile(`^local\w+,\w+,\w+=\.\.\.returnhasItem\([^)]*\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("hasItem needs 3 args")
			}
			itemID := uint32(args[0].AsNumber())
			count := uint32(args[2].AsNumber())
			return []script.RemoteArg{script.RemoteBool(p.HasItemCount(itemID, count))}, nil
		},
	},

	// ---- 公会（P2 关键路径）----
	{
		name: "createGuild", arity: 3,
		// player.lua: `return uidRemoteCall(uid, guildName, gold, itemID, [[ return createGuild(...) ]])`
		//
		// 参数顺序：(guildName, gold, itemID)
		re: regexp.MustCompile(`^returncreateGuild\(\.\.\.\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("createGuild needs (name, gold, itemID)")
			}
			if h.Guilds == nil {
				return []script.RemoteArg{script.RemoteNumber(float64(storage.GuildErrNoGuild))}, nil
			}
			name := args[0].AsString()
			goldCost := uint64(args[1].AsNumber())
			itemCost := uint32(args[2].AsNumber())

			if p.Gold < goldCost {
				return []script.RemoteArg{script.RemoteNumber(float64(storage.GuildErrNoGold))}, nil
			}
			// 建会需要一个道具（沃玛号角）
			if itemCost != 0 && !p.HasItemCount(itemCost, 1) {
				return []script.RemoteArg{script.RemoteNumber(float64(storage.GuildErrNoItem))}, nil
			}

			guildID, code, err := h.Guilds.Create(context.Background(), p.DBID, name)
			if err != nil || code != storage.GuildErrNone {
				return []script.RemoteArg{script.RemoteNumber(float64(code))}, nil
			}
			// ★ 建会成功后才扣金币与道具（原版 createGuild 注释：
			//   "gold and one item are taken only when the guild gets created"）
			if goldCost > 0 {
				if remaining, ok, rmErr := removeGoldAtomically(context.Background(), h.Golds, p, goldCost); rmErr == nil && ok {
					p.Gold = remaining
					h.sendGold(p)
				}
			}
			if itemCost != 0 {
				// 号角不可堆叠，按 itemID 找第一格扣 1 个
				if idx, it := p.findItem(itemCost, 0); it != nil {
					_ = p.removeItem(it.ItemID, uint32(it.SeqID), 1)
					_ = idx
					h.pushInventory(p)
				} else {
					p.RemoveItemByID(itemCost, 1)
					h.pushInventory(p)
				}
			}
			_ = guildID
			return []script.RemoteArg{script.RemoteNumber(float64(storage.GuildErrNone))}, nil
		},
	},
	{
		name: "declareGuildWar", arity: 2,
		// player.lua: `return uidRemoteCall(uid, enemyGuildName, gold, [[ return declareGuildWar(...) ]])`
		//
		// 参数顺序：(enemyName, goldCost)
		//
		// ★ 这是 P2 公会战**唯一的玩家入口**（没有 CM_DECLAREGUILDWAR 协议）。
		//   返回 GUILDERR_* 码，脚本据此给玩家不同的对话分支。
		re: regexp.MustCompile(`^returndeclareGuildWar\(\.\.\.\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("declareGuildWar needs (enemyName, gold)")
			}
			if h.GuildWars == nil || h.Guilds == nil {
				return []script.RemoteArg{script.RemoteNumber(float64(storage.GuildErrNoGuild))}, nil
			}
			enemyName := args[0].AsString()
			cost := uint64(args[1].AsNumber())
			code, err := h.GuildWars.DeclareWar(context.Background(), p.DBID, enemyName, cost, 0)
			if err != nil {
				h.Log(1, fmt.Sprintf("declareGuildWar failed: %v", err))
			}
			return []script.RemoteArg{script.RemoteNumber(float64(code))}, nil
		},
	},
	{
		name: "hasJob", arity: 1,
		// 注意：player.lua 里的 hasJob 是纯 Lua 实现（遍历 getJobList），
		// 不走 uidRemoteCall。这里保留一个分支以防脚本直接写
		// `return hasJob(...)`。
		re: regexp.MustCompile(`^local\w+=\.\.\.returnhasJob\(\w+\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("hasJob needs 1 arg")
			}
			return []script.RemoteArg{script.RemoteBool(args[0].AsString() == jobName(p.Job))}, nil
		},
	},

	// ---- 装备/穿戴（技能教头与任务脚本会用）----
	{
		name: "addBoundItem", arity: 1,
		// player.lua: `return uidRemoteCall(uid, item, [[ return addBoundItem(...) ]])`
		//
		// 原版给的是「绑定物品」（EA_BIND），穿上就脱不下来。
		// RemoteArg 表达不了 extAttrList，所以这里退化成「给一件普通物品」，
		// 并在日志里说明 —— 任务奖励的装备玩家能自由交易，
		// 这是与原文的已知差异（不影响主流程，但要记）。
		re: regexp.MustCompile(`^returnaddBoundItem\(\.\.\.\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("addBoundItem needs 1 arg")
			}
			itemID := uint32(args[0].AsNumber())
			if h.Items == nil {
				return []script.RemoteArg{script.RemoteNumber(0)}, nil
			}
			rec, found := h.Items.Get(itemID)
			if !found {
				return []script.RemoteArg{script.RemoteNumber(0)}, nil
			}
			p.AddItem(rec, Item{ItemID: itemID, Count: 1})
			h.pushInventory(p)
			h.Log(1, fmt.Sprintf("addBoundItem: gave %s as a plain item (bind attr not supported)", rec.Name))
			// 返回新物品的 seqID（脚本可能拿它后续 removeWearItem）
			if len(p.Inventory) == 0 {
				return []script.RemoteArg{script.RemoteNumber(0)}, nil
			}
			return []script.RemoteArg{script.RemoteNumber(float64(p.Inventory[len(p.Inventory)-1].SeqID))}, nil
		},
	},
	{
		name: "removeWearItem", arity: 1,
		// player.lua: `return uidRemoteCall(uid, wlType, [[ return removeWearItem(...) ]])`
		re: regexp.MustCompile(`^returnremoveWearItem\(\.\.\.\)$`),
		run: func(h *npcHostAdapter, p *Player, args []script.RemoteArg) ([]script.RemoteArg, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("removeWearItem needs 1 arg")
			}
			wlType := int(args[0].AsNumber())
			it := p.Wear[wlType]
			if it == nil {
				return []script.RemoteArg{script.RemoteBool(false)}, nil
			}
			delete(p.Wear, wlType)
			// 卸装后属性要重算，否则「脱下武器攻击力还在」
			if h.Worlds != nil {
				stats := p.CombatStatsFor(h.Items)
				_ = h.Worlds.RefreshStats(context.Background(), p.MapID, p.UID, stats)
			}
			h.pushInventory(p)
			return []script.RemoteArg{script.RemoteBool(true)}, nil
		},
	},
	{
		name: "getTeamLeader", arity: 0,
		// 组队系统在 P0 做过，但 RemoteArg 无法表达 UID 之外的复杂结构。
		// 返回 nil（脚本里 `if leader then` 会走 false 分支）。
		re: regexp.MustCompile(`^returngetTeamLeader\(\)$`),
		run: func(h *npcHostAdapter, p *Player, _ []script.RemoteArg) ([]script.RemoteArg, error) {
			return []script.RemoteArg{{Kind: script.RemoteArgNil, IsNil: true}}, nil
		},
	},
}

// jobName 把职业编号转成脚本用的名字（player.lua 里 getJobList 返回的字符串）。
func jobName(job uint8) string {
	switch job {
	case 1:
		return "战士"
	case 2:
		return "道士"
	case 3:
		return "法师"
	default:
		return ""
	}
}

// normalizeRemoteCode 把代码串压成一行、去掉所有多余空白，便于正则匹配。
//
// ★★ 这里有个必须处理的问题：原版脚本写法**不统一** ★★
//
//	player.lua 里同一个函数写成两种：
//	  uidRemoteCall(uid, [[ return getGold () ]])      ← 名字和括号之间有空格
//	  uidRemoteCall(uid, [[ return getLevel() ]])      ← 没有
//
//	写死 `getGold\(\)` 的正则会漏掉前者（`getGold ()`）。
//	所以规整时必须**删掉所有空白**，而不只是把连续空白压成一个。
//
//	顺带解决另一个坑：脚本用 [[ ]] 跨行写代码，换行会留在字符串里，
//	直接匹配 `^return getGold` 会失败。
func normalizeRemoteCode(code string) string {
	var b strings.Builder
	b.Grow(len(code))
	for _, r := range code {
		switch r {
		case '\n', '\r', '\t', ' ', '\v', '\f':
			continue // 全部丢掉
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// executeRemoteCall 是 RemoteCall 的实现主体。
func (h *npcHostAdapter) executeRemoteCall(targetUID uint64, code string,
	args []script.RemoteArg) ([]script.RemoteArg, error) {

	// 1. 目标必须是玩家（怪物/NPC 的 remoteCall 目前不需要）
	//
	// ★ 这里每一步都要能有 nil 检查：
	//
	//	remoteCall 是**脚本驱动的入口**，340 个 NPC 脚本里任何一处
	//	传了错误的 uid 都会走到这里。缺一个 nil 检查就是一个
	//	「玩家点 NPC → 服务端 panic」的崩溃路径。
	if !common.IsPlayerUID(targetUID) {
		return nil, fmt.Errorf("remote call target %d is not a player", targetUID)
	}
	if h.Players == nil {
		return nil, fmt.Errorf("player manager not configured")
	}
	p, ok := h.Players.Get(targetUID)
	if !ok {
		return nil, fmt.Errorf("player %d not online", targetUID)
	}

	// 2. 规整代码串后匹配
	norm := normalizeRemoteCode(code)
	for _, pattern := range remoteCallPatterns {
		if !pattern.re.MatchString(norm) {
			continue
		}
		if pattern.arity >= 0 && len(args) != pattern.arity {
			return nil, fmt.Errorf("%s expects %d args, got %d", pattern.name, pattern.arity, len(args))
		}
		return pattern.run(h, p, args)
	}

	// 3. 未识别：明确报错（不静默返回 nil）
	return nil, fmt.Errorf("unsupported remote call code: %s", shortenRemoteCode(norm))
}

// shortenRemoteCode 截断过长的代码串用于错误信息。
func shortenRemoteCode(code string) string {
	const maxLen = 120
	if len(code) <= maxLen {
		return code
	}
	return code[:maxLen] + "…"
}

// 确保 math / strconv 被使用（参数解析可能扩展到它们）。
var (
	_ = math.Abs
	_ = strconv.Itoa
)
