package game

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 施法（player.cpp:1912 onCMActionSpell）====================
//
// 原版链路：
//
//	CM_ACTION(ACTION_SPELL)
//	  -> 先 dispatchAction 把动作广播给观察者（客户端据此播施法动画）
//	  -> 按 magicID 分派到不同处理（治疗/buff/直接伤害/召唤/火墙…）
//	  -> 需要延迟的技能用 addDelay 包一层，延迟后结算伤害
//	  -> 直接伤害类在延迟帧里再发一条 SM_CASTMAGIC 给观察者
//
// 这里保留了「先广播动作、再结算」的顺序，但把 addDelay 换成了
// 同步结算 + 由上层 tick 驱动的方式 —— Go 侧没有协程调度器那么
// 灵活，硬套 coroutine 会让每个施法都占一个 goroutine 直到冷却结束。
//
// ★ 三条必须照抄的规则（客户端复刻时都踩过）：
//  1. **没学过的技能直接丢弃**，不回包也不报错（原版 fflassert 只在调试版生效）
//  2. **MP 不够就整个动作失败**，且不广播（否则客户端播了动画却掉不出血）
//  3. **冷却未到就丢弃**，但要回 reportStand 让客户端收敛状态，
//     否则客户端会卡在施法动作里（它等不到 SM_ACTION 的应答）
//
// 技能分类依据 magicrecord.inc 的实际字段：
//
//	治疗类：power 非 0 且目标是友方（治愈术/群体治愈术）
//	直接伤害：power 非 0 且需要目标（火球术/雷电术/冰月神掌…）
//	Buff 类：power 全 0 且 checkGround=0（魔法盾/幽灵盾/神圣战甲术…）
//	召唤类：名字含「召唤」（召唤骷髅/召唤神兽）
//	火墙类：checkGround=1 且有持续时长（火墙）

// SpellHandler 处理施法消息。
type SpellHandler struct {
	Players   *PlayerManager
	Worlds    *world.World
	Items     *ItemDB
	Magics    *MagicDB
	Broadcast Broadcaster
	Sessions  *SessionManager
	Logger    *slog.Logger
	// Chars 技能修炼度落库（可为 nil）
	ItemRepo ItemRepository
	// KeySaver 技能快捷键落库（可为 nil）
	KeySaver MagicKeySaver
	// Kills 击杀结算器。
	//
	// ★ 与 ActionHandler.Kills 是**同一个实例**（main 里注入）。
	//	法术击杀和物理击杀的结算逻辑必须完全一致 ——
	//	两套实现迟早会漂移（一边算了归属保护、另一边忘了）。
	Kills *KillSettler
}

// MagicKeySaver 是快捷键持久化的最小接口。
type MagicKeySaver interface {
	UpdateMagicKeyList(ctx context.Context, dbid uint32, keys protocol.SDMagicKeyList) error
}

// 施法失败的原因（与服务端拒绝路径一一对应）。
//
// 这些不是协议错误码 —— SM_CASTMAGIC 是纯广播消息，没有错误回包。
// 失败一律通过 reportStand 让客户端收敛，原因只写日志。
const (
	spellFailUnknownMagic = "unknown magic"
	spellFailNotLearned   = "not learned"
	spellFailNoMP         = "not enough mp"
	spellFailCooling      = "cooling down"
	spellFailBadTarget    = "bad target"
	spellFailOutOfRange   = "out of range"
	spellFailDead         = "caster is dead"
)

// CM_ACTION 里的 ActionSpell 处理入口。
func (h *SpellHandler) Cast(ctx context.Context, channelID uint64, action common.ActionNode) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	if p.HP <= 0 {
		// 死了不能施法（原版 setHealth(0) 后 m_sdHealth.dead() 为真）
		return nil
	}

	magicID := ExtParamMagicID(action)
	// ★ 与物理攻击不同，施法的 magicID 不能兜底成 1。
	//   magicID=0 说明客户端发了个空动作，兜底成「物理攻击」会让
	//   一个法术动作打出物理伤害，数值与特效全错。
	if magicID == 0 {
		return h.reject(ctx, p, spellFailUnknownMagic, 0)
	}

	rec, ok := h.Magics.Get(magicID)
	if !ok {
		return h.reject(ctx, p, spellFailUnknownMagic, magicID)
	}
	if !p.HasMagic(magicID) {
		return h.reject(ctx, p, spellFailNotLearned, magicID)
	}

	// 冷却：原版靠 magicrecord.coolDown，客户端也有一份，但服务端必须
	// 自己判 —— 否则改客户端就能无冷却连放。
	//
	// ★ 用 `seen && last != 0` 而不是只看 seen：
	//
	//	世界时钟在「地图刚建、ticker 还没跑第一帧」时是 0，
	//	记录下来的冷却值也是 0。若只判 seen，`now(0) - last(0) = 0`
	//	会让刚放过的技能被判成冷却中（或反过来，0 掩盖了真实状态）。
	//	把 0 当作「未记录」是唯一能自洽的处理：
	//	反正 NowMS 为 0 的窗口只有一个 tick 周期（100ms），
	//	代价是这一帧内可以连放 —— 比「永久冷却」安全得多。
	nowMS := h.Worlds.NowMS(ctx, p.MapID)
	if last := p.MagicCooldown[magicID]; last != 0 && nowMS-last < int64(rec.CoolDown) {
		return h.reject(ctx, p, spellFailCooling, magicID)
	}

	// 目标与距离校验
	aimUID := action.AimUID
	if err := h.validateTarget(ctx, p, rec, action); err != nil {
		return h.reject(ctx, p, err.Error(), magicID)
	}

	// MP 校验与扣除
	cost := rec.MPCost(p.MagicTrainLevel(magicID))
	if int(p.MP) < cost {
		return h.reject(ctx, p, spellFailNoMP, magicID)
	}
	p.MP = int32(int(p.MP) - cost)
	if p.MagicCooldown == nil {
		p.MagicCooldown = make(map[uint32]int64)
	}
	p.MagicCooldown[magicID] = nowMS

	// 广播施法动作（自己 + 观察者都要看到，客户端据此播动画）
	if err := h.broadcastAction(ctx, p, action); err != nil {
		return err
	}

	// 结算效果
	if err := h.resolveSpell(ctx, p, rec, action, aimUID); err != nil {
		return err
	}

	// 回血量（MP 变了）
	h.notifyHealth(ctx, p)
	return nil
}

// validateTarget 校验施法距离与目标合法性。
func (h *SpellHandler) validateTarget(ctx context.Context, p *Player, rec MagicRecord, action common.ActionNode) error {
	if rec.CastRange.TypeID == CastRangeLong {
		// CRT_LONG：无距离限制，但仍要求「有目标」时目标存在
		return nil
	}
	distance := rec.CastRange.Distance
	if distance <= 0 {
		return nil
	}

	targetX, targetY := int(action.AimX), int(action.AimY)
	if action.AimUID != 0 {
		if e, ok, err := h.Worlds.Entity(ctx, p.MapID, action.AimUID); err == nil && ok {
			targetX, targetY = e.X, e.Y
		}
	}
	if world.ChebyshevDistance(
		world.Point{X: p.X, Y: p.Y}, world.Point{X: targetX, Y: targetY}) > distance {
		return fmt.Errorf("%s", spellFailOutOfRange)
	}
	return nil
}

// resolveSpell 按技能类别结算效果。
//
// 分类优先级很重要：先判「治疗」，再判「伤害」，最后才是 buff。
// 因为治愈术同时有 power（治疗量）和 castRange，如果按 buff 处理
// 就变成了只挂状态不补血。
func (h *SpellHandler) resolveSpell(ctx context.Context, p *Player, rec MagicRecord, action common.ActionNode, aimUID uint64) error {
	magicID := rec.MagicID

	// 1. 治疗类（治愈术/群体治愈术）：目标是友方，回血不伤敌
	if isHealMagic(rec) {
		return h.castHeal(ctx, p, rec, aimUID)
	}

	// 2. Buff 类（魔法盾/幽灵盾/神圣战甲术/施毒术…）：power 全 0
	if rec.Power[0] == 0 && rec.Power[1] == 0 && rec.AddPower[0] == 0 && rec.AddPower[1] == 0 {
		return h.castBuff(ctx, p, rec, aimUID)
	}

	// 3. 直接伤害类
	if aimUID == 0 {
		// 需要目标却给了 0：火墙这类地面技能例外（checkGround=1 且无目标）
		if rec.CheckGround != 0 {
			return h.castGroundMagic(ctx, p, rec, action)
		}
		return h.reject(ctx, p, spellFailBadTarget, magicID)
	}

	return h.castDamage(ctx, p, rec, aimUID)
}

// castDamage 结算一次法术伤害。
//
// 与物理攻击的区别：
//
//	物理：用 stats.DC + DCHit，受 AC 减免
//	法术：法师用 MC + MCHit，道士用 SC（道术），受 MAC 减免
//
// ★ 道士技能用 SC 而不是 MC：
//
//	原版 getAttackDamage 里「灵魂火符/冰月神掌/冰月震天」走 randPickSC，
//	其余法术走 randPickMC。这两组属性在装备上完全独立（道袍给 SC、
//	魔法长袍给 MC），混用会让道士技能伤害永远是 0。
func (h *SpellHandler) castDamage(ctx context.Context, p *Player, rec MagicRecord, aimUID uint64) error {
	// 目标必须存在且同图
	target, ok, err := h.Worlds.Entity(ctx, p.MapID, aimUID)
	if err != nil || !ok {
		return h.rejectStand(ctx, p)
	}
	if target.HP <= 0 {
		return h.rejectStand(ctx, p)
	}

	// 伤害区间来自技能本身 power + addPower（等级越高追加越多）
	// 原版 power 是基础区间，addPower 按修炼等级叠加。
	trainLevel := p.MagicTrainLevel(rec.MagicID)
	minDmg := rec.Power[0] + rec.AddPower[0]*trainLevel
	maxDmg := rec.Power[1] + rec.AddPower[1]*trainLevel
	if maxDmg < minDmg {
		maxDmg = minDmg
	}

	// 道士技能走 SC，其余走 MC —— 见上面注释
	useSC := isTaoistMagic(rec)
	stats := p.CombatStatsFor(h.Items)
	var atkBounds world.AbilityBounds
	var atkHit int
	if useSC {
		atkBounds = world.AbilityBounds{Min: minDmg + stats.SC.Min, Max: maxDmg + stats.SC.Max}
		atkHit = stats.MCHit // 道术命中与魔法命中同源
	} else {
		atkBounds = world.AbilityBounds{Min: minDmg + stats.MC.Min, Max: maxDmg + stats.MC.Max}
		atkHit = stats.MCHit
	}

	// 元素抗性由 h.bindElementResolver() 在构造时一次性注入，
	// 这里不能每帧重设 —— world.elemOfMagic 是包级变量，
	// 每个 goroutine 都写一遍就是数据竞争（-race 会报）。

	result, err := h.Worlds.CombatWithBounds(ctx, p.MapID, p.UID, aimUID, rec.MagicID,
		atkBounds, atkHit)
	if err != nil {
		return h.rejectStand(ctx, p)
	}

	// 广播施法表现（SM_CASTMAGIC）—— 原版在延迟帧里发，
	// 这里同步发，客户端拿到时动画刚开始，观感一致。
	if err := h.broadcastCastMagic(ctx, p, rec, aimUID, 0, 0); err != nil {
		return err
	}

	if result.Hit {
		h.broadcastDamageEvent(ctx, p, aimUID, result)
	} else {
		h.broadcastToAll(ctx, p.MapID, protocol.SM_MISS, protocol.SMMiss{UID: aimUID})
	}
	if result.Dead {
		h.broadcastToAll(ctx, p.MapID, protocol.SM_NOTIFYDEAD, protocol.SMNotifyDead{UID: aimUID})
		// 打死怪物要结算掉落与经验
		h.settleKill(ctx, p, aimUID)
	}
	return nil
}

// castHeal 治疗类技能（治愈术）。
//
// 治疗量 = power + addPower*trainLevel，直接加到目标 HP 上，不超过 MaxHP。
// 目标为 0 时治疗自己（客户端点自己施法就是这样）。
func (h *SpellHandler) castHeal(ctx context.Context, p *Player, rec MagicRecord, aimUID uint64) error {
	if aimUID == 0 {
		aimUID = p.UID
	}
	trainLevel := p.MagicTrainLevel(rec.MagicID)
	heal := rec.Power[0] + rec.AddPower[0]*trainLevel
	if heal <= 0 {
		heal = rec.Power[1]
	}

	h.broadcastCastMagic(ctx, p, rec, aimUID, 0, 0)

	// 目标是自己：直接改 Player 并同步世界实体
	if aimUID == p.UID {
		p.HP += int32(heal)
		if p.HP > p.MaxHP {
			p.HP = p.MaxHP
		}
		if err := h.syncHealth(ctx, p); err != nil {
			return err
		}
		h.notifyHealth(ctx, p)
		return nil
	}

	// 目标是别的玩家
	if target, ok := h.Players.Get(aimUID); ok && target.MapID == p.MapID {
		target.HP += int32(heal)
		if target.HP > target.MaxHP {
			target.HP = target.MaxHP
		}
		if err := h.syncHealth(ctx, target); err != nil {
			return err
		}
		h.notifyHealth(ctx, target)
	}
	return nil
}

// castBuff 挂 buff（魔法盾/幽灵盾/神圣战甲术/施毒术）。
//
// 原版的 buff 来自 buffrecord.inc 按魔法名查表；
// 这里先用「魔法名 -> buff 类型」的映射，完整的 buffact 细分留给后续批次。
func (h *SpellHandler) castBuff(ctx context.Context, p *Player, rec MagicRecord, aimUID uint64) error {
	buffType, ok := buffTypeOfMagic(rec.Name)
	if !ok {
		// 没有对应 buff 记录：仍然播施法动画，但不挂状态。
		// 直接丢弃会让「阴阳法环」这类纯特效技能看起来没反应。
		return h.broadcastCastMagic(ctx, p, rec, p.UID, 0, 0)
	}

	target := p
	if aimUID != 0 && aimUID != p.UID {
		if other, found := h.Players.Get(aimUID); found && other.MapID == p.MapID {
			target = other
		}
	}

	h.broadcastCastMagic(ctx, p, rec, target.UID, 0, 0)

	// BuffID 用 buffType 直接当 ID，state=ON
	target.Buffs = append(target.Buffs, buffType)
	return h.broadcastToAll(ctx, p.MapID, protocol.SM_BUFF, protocol.SMBuff{
		UID: target.UID, Type: buffType, State: 1,
	})
}

// castGroundMagic 地面技能（火墙/地狱火这类 checkGround=1 且无目标的）。
//
// 火墙的真实实现（world/firewall.go）需要 dropItemList 之外的
// 一段持续伤害逻辑，这里先只广播施法表现，火墙本体留给后续批次。
func (h *SpellHandler) castGroundMagic(ctx context.Context, p *Player, rec MagicRecord, action common.ActionNode) error {
	return h.broadcastCastMagic(ctx, p, rec, 0, int(action.AimX), int(action.AimY))
}

// settleKill 结算击杀奖励（经验 + 掉落）。
//
// 委托给共享的 KillSettler —— 物理攻击走同一份逻辑。
// 两套实现会漂移：一边加了归属保护、另一边忘了，是很难查的缺陷。
func (h *SpellHandler) settleKill(ctx context.Context, killer *Player, targetUID uint64) {
	if h.Kills == nil {
		return
	}
	h.Kills.Settle(ctx, killer, targetUID)
}

// ==================== 共用辅助 ====================

// reject 记录失败原因并让客户端回到站立状态。
//
// ★ 为什么冷却/MP 不足也要回 reportStand：
//
//	客户端按下技能后进入「施法中」状态，等待 SM_ACTION 应答。
//	服务端什么都不发，客户端就一直卡在那个动作里，
//	表现是「按一次技能后角色定住不动，要再点一次地面才恢复」。
func (h *SpellHandler) reject(ctx context.Context, p *Player, reason string, magicID uint32) error {
	h.logReject(p, reason, magicID)
	return h.rejectStand(ctx, p)
}

func (h *SpellHandler) logReject(p *Player, reason string, magicID uint32) {
	if h.Logger == nil {
		return
	}
	h.Logger.Debug("spell rejected",
		"player", p.Name, "magic", magicID, "reason", reason)
}

func (h *SpellHandler) rejectStand(ctx context.Context, p *Player) error {
	return h.broadcastAction(ctx, p, actionStand(p))
}

// actionStand 构造站立动作。
func actionStand(p *Player) common.ActionNode {
	return common.ActionNode{
		Type:      common.ActionStand,
		Direction: p.Direction,
		X:         int16(p.X),
		Y:         int16(p.Y),
		AimX:      int16(p.X),
		AimY:      int16(p.Y),
	}
}

// broadcastAction 把动作发给「自己 + 同图观察者」。
func (h *SpellHandler) broadcastAction(ctx context.Context, p *Player, action common.ActionNode) error {
	payload := protocol.SMAction{UID: p.UID, MapUID: p.MapUID, Action: action}
	if err := h.sendTo(ctx, p.ChannelID, protocol.SM_ACTION, payload); err != nil {
		return err
	}
	observers, err := h.Worlds.ForwardAction(ctx, p.MapID, p.UID, p.X, p.Y, p.X, p.Y)
	if err != nil {
		return nil
	}
	for _, uid := range observers {
		other, ok := h.Players.Get(uid)
		if !ok || other.ChannelID == p.ChannelID {
			continue
		}
		_ = h.sendTo(ctx, other.ChannelID, protocol.SM_ACTION, payload)
	}
	return nil
}

// broadcastCastMagic 广播 SM_CASTMAGIC（施法特效）。
//
// Speed 用 SYS_DEFSPEED=100（battleobject.hpp MagicSpeed）；
// 客户端拿它算动画时长，传 0 会让特效瞬间闪完。
func (h *SpellHandler) broadcastCastMagic(ctx context.Context, p *Player, rec MagicRecord,
	aimUID uint64, aimX, aimY int) error {
	if aimX == 0 && aimY == 0 {
		aimX, aimY = p.X, p.Y
	}
	payload := protocol.SMCastMagic{
		UID:        p.UID,
		MapUID:     p.MapUID,
		Magic:      uint8(rec.MagicID),
		MagicParam: uint8(p.MagicTrainLevel(rec.MagicID)),
		Speed:      uint8(common.DefaultSpeed),
		Direction:  p.Direction,
		X:          uint16(p.X),
		Y:          uint16(p.Y),
		AimX:       uint16(aimX),
		AimY:       uint16(aimY),
		AimUID:     aimUID,
	}
	return h.broadcastToAll(ctx, p.MapID, protocol.SM_CASTMAGIC, payload)
}

func (h *SpellHandler) broadcastDamageEvent(ctx context.Context, p *Player, targetUID uint64, result world.AttackResult) {
	if e, ok, err := h.Worlds.Entity(ctx, p.MapID, targetUID); err == nil && ok {
		if target, exists := h.Players.Get(targetUID); exists {
			target.SyncFromWorld(e)
			h.notifyHealth(ctx, target)
		}
		_ = h.broadcastToAll(ctx, p.MapID, protocol.SM_STRIKEGRID, protocol.SMStrikeGrid{
			X: uint32(e.X), Y: uint32(e.Y),
		})
	}
}

func (h *SpellHandler) broadcastGroundItems(ctx context.Context, mapID uint32, x, y int, itemIDs []uint32) {
	_ = h.broadcastToAll(ctx, mapID, protocol.SM_GROUNDITEMIDLIST, protocol.SDGroundItemIDList{
		GridItemIDList: []protocol.SDGridItemIDList{{X: int32(x), Y: int32(y), ItemIDList: itemIDs}},
	})
}

// gainExp 加经验并按需升级。
func (h *SpellHandler) gainExp(ctx context.Context, p *Player, exp uint64) {
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
		if err := h.Worlds.SetStats(ctx, p.MapID, p.UID, p.CombatStatsFor(h.Items)); err != nil {
			h.logReject(p, "set stats after level up", 0)
		}
	}
	_ = h.broadcastToAll(ctx, p.MapID, protocol.SM_EXP, protocol.SMExp{Exp: p.Exp})
	h.notifyHealth(ctx, p)
}

// syncHealth 把 Player 的血量写回世界实体。
func (h *SpellHandler) syncHealth(ctx context.Context, p *Player) error {
	return h.Worlds.MutateEntity(ctx, p.MapID, p.UID, func(e *world.Entity) error {
		e.HP = p.HP
		e.MP = p.MP
		e.MaxHP = p.MaxHP
		e.MaxMP = p.MaxMP
		return nil
	})
}

func (h *SpellHandler) notifyHealth(ctx context.Context, p *Player) {
	if p.ChannelID == 0 || h.Broadcast == nil {
		return
	}
	payload, err := protocol.CerealSerialize(protocol.SDHealth{
		UID: p.UID, HP: p.HP, MP: p.MP, MaxHP: p.MaxHP, MaxMP: p.MaxMP,
	})
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_HEALTH, payload, 0)
}

func (h *SpellHandler) broadcastToAll(ctx context.Context, mapID uint32, id protocol.SMType, value any) error {
	payload, err := protocol.AppendWirePayload(nil, value)
	if err != nil {
		return fmt.Errorf("encode %v: %w", id, err)
	}
	if h.Broadcast == nil {
		return nil
	}
	for _, other := range h.Players.Snapshot() {
		if other.MapID != mapID {
			continue
		}
		_ = h.Broadcast.SendTo(other.ChannelID, id, payload, 0)
	}
	return nil
}

func (h *SpellHandler) sendTo(ctx context.Context, channelID uint64, id protocol.SMType, value any) error {
	if channelID == 0 || h.Broadcast == nil {
		return nil
	}
	payload, err := protocol.AppendWirePayload(nil, value)
	if err != nil {
		return fmt.Errorf("encode %v: %w", id, err)
	}
	return h.Broadcast.SendTo(channelID, id, payload, 0)
}

// ==================== 技能分类谓词 ====================

// isHealMagic 判断是否治疗类技能。
//
// 依据 magicrecord.inc：治愈术/群体治愈术的 elem 是「神圣」，
// 且 power 是治疗量而非伤害。用名字判最稳 —— 元素神圣还有
// 神圣战甲术这种 buff 技能，光看 elem 会误判。
func isHealMagic(rec MagicRecord) bool {
	switch rec.Name {
	case "治愈术", "群体治愈术", "痊愈术":
		return true
	}
	return false
}

// isTaoistMagic 判断道士技能是否走 SC（道术）而不是 MC。
//
// 【依据】player.cpp:1265 getAttackDamage：
//
//	case 灵魂火符/冰月神掌/冰月震天: randPickSC() * elemRatio
//	case 雷电术/火球术/大火球/疾光电影/地狱火/冰沙掌: randPickMC() * elemRatio
//
// 也就是说「道士也能用的法术」里面只有这三个是 SC，
// 别的道术技能（施毒术/召唤骷髅）不产生直接伤害。
func isTaoistMagic(rec MagicRecord) bool {
	switch rec.Name {
	case "灵魂火符", "冰月神掌", "冰月震天":
		return true
	}
	return false
}

// buffTypeOfMagic 把魔法名映射到 buff 类型（BFT_*）。
//
// 【依据】player.cpp:1922-1930：治愈术/施毒术/幽灵盾/神圣战甲术
// 用 DBCOM_BUFFID(魔法名) 查 buffrecord，buffID 就是魔法名索引。
//
// ★ buff ID 不是魔法 ID：buffrecord.inc 与 magicrecord.inc 是两张表，
//
//	只是同名。这里返回的是 buff 表里的索引，不能直接用 magicID。
func buffTypeOfMagic(name string) (uint32, bool) {
	switch name {
	case "魔法盾":
		return BuffTypeShield, true
	case "幽灵盾":
		return BuffTypeGhostArmor, true
	case "神圣战甲术":
		return BuffTypeHolyArmor, true
	case "施毒术":
		return BuffTypePoison, true
	case "阴阳法环":
		return BuffTypeYinYang, true
	}
	return 0, false
}

// buff 类型常量（buffrecord.inc 的 BFT_* 子集）。
const (
	BuffTypeShield     uint32 = 1 // BFT_SHIELD 魔法盾
	BuffTypeGhostArmor uint32 = 2 // 幽灵盾
	BuffTypeHolyArmor  uint32 = 3 // 神圣战甲术
	BuffTypePoison     uint32 = 4 // 施毒术
	BuffTypeYinYang    uint32 = 5 // 阴阳法环
)

// ==================== 快捷键（CM_SETMAGICKEY）====================

// MagicKey 是一个技能快捷键绑定（magicID -> 按键字符）。
type MagicKey struct {
	MagicID uint32
	Key     uint8
}

// SetMagicKey 处理 CM_SETMAGICKEY（playernet.cpp:1581）。
//
// 原版流程：
//
//	if(SDMagicKeyList::validKey(key)) dbUpdateMagicKey(magicID, key);
//
// 两条隐含校验，缺了就会写脏数据：
//
//  1. **key 必须合法**：只有 a-z / 0 / 7-9 可用。
//     1-6 被腰带快捷键占用（serdesmsg.hpp:1148 的注释：
//     "keys 1 ~ 6 are reserved to consume belt items"），
//     允许设置会让「按 1 喝药」和「按 1 放技能」冲突。
//  2. **必须是已学的技能**：原版 dbUpdateMagicKey 里
//     fflassert(m_sdLearnedMagicList.has(magicID)) ——
//     未学技能设快捷键，客户端技能栏会出现一个永远灰色的图标。
func (h *SpellHandler) SetMagicKey(ctx context.Context, channelID uint64, req protocol.CMSetMagicKey) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}

	if !ValidMagicKey(req.Key) {
		h.logReject(p, "invalid magic key", req.MagicID)
		return nil
	}
	if !p.HasMagic(req.MagicID) {
		h.logReject(p, "set key for unlearned magic", req.MagicID)
		return nil
	}

	if p.MagicKeys == nil {
		p.MagicKeys = make(map[uint32]uint8)
	}
	// 同一个键只能绑一个技能：换绑前先把旧绑定删掉
	for id, key := range p.MagicKeys {
		if key == req.Key && id != req.MagicID {
			delete(p.MagicKeys, id)
		}
	}
	p.MagicKeys[req.MagicID] = req.Key

	// 落库（整张表覆盖写，与原版 dbUpdateMagicKey 一致）
	if h.KeySaver != nil {
		keys := protocol.SDMagicKeyList{KeyList: make(map[uint32]uint8, len(p.MagicKeys))}
		for id, key := range p.MagicKeys {
			keys.KeyList[id] = key
		}
		if err := h.KeySaver.UpdateMagicKeyList(ctx, p.DBID, keys); err != nil {
			h.logReject(p, "save magic keys failed: "+err.Error(), req.MagicID)
		}
	}
	return nil
}

// ValidMagicKey 对应 SDMagicKeyList::validKey。
//
// ★ 1-6 不是「暂时没用到」而是「已被腰带占用」：
//
//	serdesmsg.hpp:1148 明确写着 keys 1 ~ 6 are reserved to consume
//	belt items。放行会让两个功能抢同一个键，客户端没法区分意图。
func ValidMagicKey(key uint8) bool {
	return (key >= 'a' && key <= 'z') || key == '0' || (key >= '7' && key <= '9')
}

// BindElementResolver 把魔法表接到 world 的元素解析钩子上。
//
// 为什么必须是「一次性绑定」而不是「每次施法设置」：
//
//	world.elemOfMagic 是包级变量，SetMagicElementResolver 直接赋值。
//	每个连接一个 goroutine，若每帧都写一遍，就是典型的数据竞争
//	（go test -race 会直接报 DATA RACE），而且解析器可能被改成
//	别的玩家的表 —— 多张地图用不同魔法表时会算错抗性。
//
// 调用时机：main 装配阶段（LoadMagicDB 之后、服务启动之前）。
func BindElementResolver(magics *MagicDB) {
	if magics == nil {
		return
	}
	world.SetMagicElementResolver(func(id int) world.Element {
		if rec, ok := magics.Get(uint32(id)); ok {
			return world.ElementFromName(rec.Elem)
		}
		return world.ElemNone
	})
}
