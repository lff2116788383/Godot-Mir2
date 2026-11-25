package game

import (
	"context"
	"log/slog"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 击杀结算（物理攻击 / 法术共用）====================
//
// 【为什么需要单独一个模块】
//
//	最初只有 SpellHandler.settleKill，因为「法术打死怪」是最先做的路径。
//	但玩家用**普通物理攻击**打死怪时走的是 ActionHandler.handleAttack →
//	attackWorldEntity，那条路径**只广播了 SM_NOTIFYDEAD，没有结算掉落和经验**。
//
//	后果非常严重且不容易发现：
//
//	  战士（近战职业）打死怪不掉东西、不得经验，
//	  只有法师用火球打死才有奖励 —— 「打怪没意义」。
//	  而客户端看起来一切正常（怪确实死了、尸体也在），
//	  所以这个 bug 不会表现为报错，只会表现为「这游戏打怪不长经验」。
//
//	所以结算逻辑必须放在两个 handler 都能用的地方。
//	这里做成一个独立结构体，由 main 装配后同时注入 ActionHandler
//	和 SpellHandler。

// KillSettler 负责「怪物死亡后该发生什么」。
//
// 职责边界：
//   - 结算掉落（按怪物记录 roll）
//   - 掉落落地（带归属保护期）
//   - 击杀者加经验、按需升级
//
// 不负责：死亡广播（SM_NOTIFYDEAD 由调用方发，因为玩家死亡也要走同一条消息）。
type KillSettler struct {
	Players   *PlayerManager
	Worlds    *world.World
	Items     *ItemDB
	Broadcast Broadcaster
	Logger    *slog.Logger
	// GuildWars 公会战判定（用来免 PK 点）
	//
	// ★ 没有它的话，公会战双方互杀会一路刷成红名：
	//
	//	原版 playerop.cpp:671 ——
	//	  if(isPlayer(target) && hasAggression(target) && !atGuildWar(target)) addPKPoint(...)
	//
	//	「killing a member of a guild my guild is at war with is no murder」
	//	漏了这一个判断，公会战就变成「谁先动手谁变红名」，
	//	而红名会掉装备、不能进城、被守卫打 —— 等于没人敢打公会战。
	GuildWars *GuildWarManager
	// Chars 角色持久化（PK 点要落库，否则重登清零）
	Chars PKPointWriter
}

// PKPointWriter 是 PK 点落库的最小接口。
type PKPointWriter interface {
	UpdatePKPoint(ctx context.Context, dbid uint32, pkPoint int32) error
}

// SysPKPointPerKill 是杀人增加的 PK 点（sysconst.hpp:136）。
const SysPKPointPerKill int32 = 100

// SysPKPointRedName 是红名阈值（sysconst.hpp:137）。
const SysPKPointRedName int32 = 200

// Settle 结算一次击杀。
//
// 返回 true 表示这次调用**真的结算了**（首次），false 表示已结算过或目标无效。
//
// 幂等性由 world.CommitDeath 保证（Entity.DropSettled 标记），
// 调用方不需要自己去重 —— 这点很重要，因为击杀路径有多条：
//
//	物理攻击打死 → 这条路径
//	法术打死     → SpellHandler.Cast
//	tick 里检查到 HP<=0 → MapTicker
//
// 任何一条路径重复调用都只会结算一次。
func (s *KillSettler) Settle(ctx context.Context, killer *Player, targetUID uint64) bool {
	if s == nil || s.Worlds == nil || killer == nil {
		return false
	}
	return s.settle(ctx, killer.MapID, killer, targetUID)
}

// SettleNoKiller 结算「不是玩家打死的」死亡（守卫杀怪、怪互相杀）。
//
// 只落地掉落，不给任何人经验。
//
// ★ 为什么也要结算：
//
//	守卫把半兽人打死，那个半兽人身上该掉的东西一样要掉。
//	不结算的话「守卫帮你打死了怪，东西凭空消失」——
//	玩家会以为被系统吞了。
func (s *KillSettler) SettleNoKiller(ctx context.Context, mapID uint32, targetUID uint64) bool {
	if s == nil || s.Worlds == nil {
		return false
	}
	return s.settle(ctx, mapID, nil, targetUID)
}

// settle 是 Settle / SettleNoKiller 的共同实现。
func (s *KillSettler) settle(ctx context.Context, mapID uint32, killer *Player, targetUID uint64) bool {
	result, ok, err := s.Worlds.CommitDeath(ctx, mapID, targetUID)
	if err != nil {
		s.log("commit death failed", killer, err)
		return false
	}
	if !ok {
		return false
	}

	// 1. 掉落落地
	if ids := s.resolveDropItems(result.Drops); len(ids) > 0 {
		// ★ 掉落归属：传 Owner 而不是 killer。
		//
		//	两者通常相同，但不总是 —— 法师放了个火墙烧死怪，
		//	而战士最后补了一刀的话，Owner 是伤害总量最高的人。
		//	原版归属就是按「仇恨最高」算的，不是「最后一击」。
		owner := result.Owner
		if owner == 0 && killer != nil {
			owner = killer.UID
		}
		if _, err := s.Worlds.DropItemOwned(ctx, mapID, result.X, result.Y, ids, owner); err != nil {
			s.log("drop item failed", killer, err)
		} else {
			s.broadcastGroundItems(ctx, mapID, result.X, result.Y, ids)
		}
	}

	// 2. 经验
	//
	// ★ 经验给 killer 而不是 Owner：
	//
	//	原版 dispatchOffenderExp 是按仇恨列表分摊的（多人都打则有份），
	//	当前版本简化成「最后一击的人独得」。差别会在多人围殴时体现，
	//	已在 REPLICA_AUDIT 记为已知简化。
	//
	// killer 为 nil（守卫杀怪）时不给经验 —— 守卫不需要升级。
	if result.Exp > 0 && killer != nil {
		s.gainExp(ctx, killer, result.Exp)
	}

	// 3. 杀玩家的 PK 点（playerop.cpp:671 on_AM_NOTIFYDEAD）
	//
	// 只有「杀的是玩家」才加。杀怪不加。
	s.applyPKPoint(ctx, killer, targetUID)
	return true
}

// applyPKPoint 处理「杀了另一个玩家」的 PK 点。
//
// 原版判定（playerop.cpp:671）：
//
//	if(amND.UID && uidf::isPlayer(amND.UID) && hasAggression(amND.UID) && !atGuildWar(amND.UID)){
//	    addPKPoint(SYS_PKPOINTPERKILL);
//	}
//
// 三个条件缺一不可：
//
//	isPlayer(target)  —— 杀怪不加 PK 点
//	hasAggression     —— **对方先动手打我的，我反杀不加 PK 点**（正当防卫）
//	!atGuildWar       —— 公会战中的敌人不算谋杀
//
// ★ 第二个条件最容易漏，漏了的后果是「被偷袭的人反杀反而变红名」——
// 玩家会彻底不敢还手，PK 系统直接失效。
//
// ★ Go 侧当前没有 m_aggressionList，所以「正当防卫」这一条**未实现**：
// 只有「目标先攻击过击杀者」才算正当防卫。目前退化成
// 「杀任何非同会非敌对玩家都加 PK 点」，比原版更严。
// 已在 REPLICA_AUDIT 记录（而不是假装做了）。
func (s *KillSettler) applyPKPoint(ctx context.Context, killer *Player, targetUID uint64) {
	if s == nil || killer == nil || targetUID == 0 {
		return
	}
	// 目标是玩家吗？（UID 高 4 位是类型）
	if !common.IsPlayerUID(targetUID) {
		return
	}
	targetDBID, err := common.GetPlayerDBID(targetUID)
	if err != nil || targetDBID == 0 || targetDBID == killer.DBID {
		return
	}

	// 公会战中杀敌不算谋杀
	if s.GuildWars != nil && s.GuildWars.AtWar(ctx, killer.DBID, targetDBID) {
		return
	}

	killer.PKPoint += SysPKPointPerKill

	// 落库：不存的话重登清零，红名惩罚形同虚设
	if s.Chars != nil {
		if err := s.Chars.UpdatePKPoint(ctx, killer.DBID, killer.PKPoint); err != nil {
			s.log("persist pk point failed", killer, err)
		}
	}

	if s.Logger != nil && killer.PKPoint >= SysPKPointRedName {
		s.Logger.Info("player became red-named",
			"name", killer.Name, "pk_point", killer.PKPoint)
	}
}

// resolveDropItems 把掉落名换成 itemID。
func (s *KillSettler) resolveDropItems(drops []world.DropResult) []uint32 {
	if len(drops) == 0 || s.Items == nil {
		return nil
	}
	out := make([]uint32, 0, len(drops))
	for _, d := range drops {
		// DropResult.Item 是名字（monsterrecord 里写的是 u8"..."），
		// 物品表是 ID 索引，必须转一次。
		id, found := s.Items.FindByName(d.Item)
		if !found {
			if s.Logger != nil {
				s.Logger.Warn("drop item not in item db", "name", d.Item)
			}
			continue
		}
		out = append(out, id)
	}
	return out
}

// gainExp 加经验并按需升级。
func (s *KillSettler) gainExp(ctx context.Context, p *Player, exp uint64) {
	if exp == 0 {
		return
	}
	p.Exp += exp
	newLevel := DisplayLevel(p.Exp)
	if newLevel > p.Level {
		p.Level = newLevel
		// 升级要重算 HP 上限与战斗属性
		p.MaxHP = 30 + int32(newLevel)*5
		p.HP = p.MaxHP
		if err := s.Worlds.RefreshStats(ctx, p.MapID, p.UID, p.CombatStatsFor(s.Items)); err != nil {
			s.log("set stats after level up", p, err)
		}
	}
	s.broadcastAll(ctx, p.MapID, protocol.SM_EXP, protocol.SMExp{Exp: p.Exp})
	s.notifyHealth(ctx, p)
}

// broadcastGroundItems 下发「这一格有这些东西了」。
func (s *KillSettler) broadcastGroundItems(ctx context.Context, mapID uint32, x, y int, itemIDs []uint32) {
	s.broadcastAll(ctx, mapID, protocol.SM_GROUNDITEMIDLIST, protocol.SDGroundItemIDList{
		GridItemIDList: []protocol.SDGridItemIDList{{X: int32(x), Y: int32(y), ItemIDList: itemIDs}},
	})
}

func (s *KillSettler) broadcastAll(ctx context.Context, mapID uint32, id protocol.SMType, value any) {
	if s.Broadcast == nil {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, value)
	if err != nil {
		return
	}
	for _, other := range s.Players.Snapshot() {
		if other.MapID != mapID || other.ChannelID == 0 {
			continue
		}
		_ = s.Broadcast.SendTo(other.ChannelID, id, payload, 0)
	}
}

func (s *KillSettler) notifyHealth(ctx context.Context, p *Player) {
	if p.ChannelID == 0 || s.Broadcast == nil {
		return
	}
	payload, err := protocol.CerealSerialize(protocol.SDHealth{
		UID: p.UID, HP: p.HP, MP: p.MP, MaxHP: p.MaxHP, MaxMP: p.MaxMP,
	})
	if err != nil {
		return
	}
	_ = s.Broadcast.SendTo(p.ChannelID, protocol.SM_HEALTH, payload, 0)
}

func (s *KillSettler) log(msg string, p *Player, err error) {
	if s.Logger == nil {
		return
	}
	args := []any{"msg", msg}
	if p != nil {
		args = append(args, "player", p.Name)
	}
	if err != nil {
		args = append(args, "error", err)
	}
	s.Logger.Warn("kill settle", args...)
}
