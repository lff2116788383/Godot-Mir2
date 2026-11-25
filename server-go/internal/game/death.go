package game

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 死亡与复活（playernet.cpp:144/203）====================
//
// 两条消息：
//
//	CM_REQUESTDIE    -> requestDie() -> goDie()  -> 血量归零 -> SM_DEADFADEOUT
//	CM_REQUESTREVIVE -> 前提是已经死了 -> 传回复活点 -> SM_REVIVE
//
// 原版的关键设计与「看起来多余但必须照抄」的点：
//
//  1. **死亡不是一个动作，是血量归零的副作用**
//     goDie() 做的就是 setHealth(0)，然后 makeActionDie 广播。
//     所以「怪物打死玩家」和「玩家自杀」走的是同一条路径，
//     SM_DEADFADEOUT 也只在这一处发。
//
//  2. **复活是「系统传送」而不是动作**
//     playernet.cpp:170 的注释写得很明确：
//     "revive is a system relocation, not an action"
//     所以复活流程是：先把尸体挪到复活点，再拉起来。
//     如果反过来（先拉起来再走），玩家会「站在死亡地点复活」，
//     如果死亡点在墙里或者怪物堆里，就会卡住或者立刻被打死。
//
//  3. **复活后血量是 10%，不是满血**
//     setHealth(max(1, MaxHP/10), max(1, MaxMP/10))
//     注意两个 max(1, ...)：低级角色 MaxHP 是 3x 的时候，
//     3/10 = 0 会让玩家复活即死。
//
//  4. **复活点坐标兜底**
//     地图记录里没配 revivePoint 时用「道馆_1 (405,120)」。
//     这是原版硬编码的默认出生点。

// 复活相关常量。
const (
	// DefaultReviveMapID 是没配复活点时用的地图（道馆_1）
	DefaultReviveMapID uint32 = 3
	// DefaultReviveX/Y 是道馆_1 的默认复活坐标
	DefaultReviveX = 405
	DefaultReviveY = 120
	// ReviveHealthDivisor 复活后血量 = MaxHP / 这个数
	ReviveHealthDivisor = 10
)

// DeathHandler 处理死亡与复活。
type DeathHandler struct {
	Players   *PlayerManager
	Worlds    *world.World
	Broadcast Broadcaster
	Logger    *slog.Logger
	Chars     CharacterWriter
}

// CharacterWriter 是死亡/复活/经验落库所需的最小接口。
type CharacterWriter interface {
	UpdatePosition(ctx context.Context, dbid, mapID uint32, x, y int32) error
	UpdateHealth(ctx context.Context, dbid uint32, hp, mp int32) error
	UpdateExp(ctx context.Context, dbid uint32, exp uint64) error
}

// RequestDie 处理 CM_REQUESTDIE（玩家主动求死 / 客户端血量归零上报）。
//
// 原版 requestDie() 就是 goDie()，没有额外校验 —— 谁都可以求死。
// 已经死透了（HP 已为 0）时直接返回，避免重复广播死亡。
func (h *DeathHandler) RequestDie(ctx context.Context, channelID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	if p.HP <= 0 {
		return nil
	}
	return h.die(ctx, p)
}

// die 把玩家置死并广播死亡。
func (h *DeathHandler) die(ctx context.Context, p *Player) error {
	// 1. 血量归零（对齐 BattleObject::goDie 的 setHealth(0)）
	p.HP = 0
	if err := h.Worlds.MutateEntity(ctx, p.MapID, p.UID, func(e *world.Entity) error {
		e.HP = 0
		return nil
	}); err != nil {
		h.log("sync death hp failed", p, err)
	}

	// 2. 血量同步（客户端据此把角色画成尸体）
	h.notifyHealth(ctx, p)

	// 3. 死亡动作广播（makeActionDie）
	dieAction := common.ActionNode{
		Type:      common.ActionDie,
		Direction: p.Direction,
		X:         int16(p.X),
		Y:         int16(p.Y),
		AimX:      int16(p.X),
		AimY:      int16(p.Y),
	}
	h.broadcastToAll(ctx, p.MapID, protocol.SM_ACTION, protocol.SMAction{
		UID: p.UID, MapUID: p.MapUID, Action: dieAction,
	})

	// 4. SM_NOTIFYDEAD + SM_DEADFADEOUT
	//
	// 两条消息的分工：
	//
	//	SM_NOTIFYDEAD   告诉所有人「这个 UID 死了」（含自己）
	//	SM_DEADFADEOUT 告诉地图「尸体该淡出了」，带位置
	//
	// 只发前者，客户端会保留一具永远不消失的尸体；
	// 只发后者，血量条不会变红。
	//
	// ★ SM_DEADFADEOUT 只有 24 字节（UID+mapUID+X+Y），
	//   原版结构里没有 viewRange 字段（servermsg.hpp:243）。
	//   早期照客户端逻辑以为要带视野，多写一个字段会让整包错位。
	h.broadcastToAll(ctx, p.MapID, protocol.SM_NOTIFYDEAD, protocol.SMNotifyDead{UID: p.UID})
	h.broadcastToAll(ctx, p.MapID, protocol.SM_DEADFADEOUT, protocol.SMDeadFadeOut{
		UID:    p.UID,
		MapUID: p.MapUID,
		X:      uint32(p.X),
		Y:      uint32(p.Y),
	})

	// 5. 落库（死亡状态要持久化，否则重登会「满血诈尸」）
	if h.Chars != nil {
		if err := h.Chars.UpdateHealth(ctx, p.DBID, p.HP, p.MP); err != nil {
			h.log("save dead health failed", p, err)
		}
	}
	return nil
}

// RequestRevive 处理 CM_REQUESTREVIVE。
//
// 顺序严格照抄原版的四步：
//
//  1. 校验「确实死了」（没死就不许复活，否则等于免费回城）
//  2. 算出复活点（地图配置 / 道馆_1 兜底）
//  3. **先把尸体挪过去**（requestRevive 是 spaceMove/mapSwitch）
//  4. 再拉起来 + 发 SM_REVIVE + 恢复 10% 血
func (h *DeathHandler) RequestRevive(ctx context.Context, channelID uint64) error {
	p, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	if p.HP > 0 {
		// 原版 `if(!m_sdHealth.dead()) co_return;`
		return nil
	}

	reviveMapID, reviveX, reviveY := h.revivePoint(p)

	// ★ 先移动后复活：见文件头注释第 2 点
	if err := h.relocate(ctx, p, reviveMapID, reviveX, reviveY); err != nil {
		h.log("revive relocate failed", p, err)
		return nil
	}

	// 恢复 10% 血（两个 max(1, ...) 缺一不可）
	hp := p.MaxHP / ReviveHealthDivisor
	if hp < 1 {
		hp = 1
	}
	mp := p.MaxMP / ReviveHealthDivisor
	if mp < 1 {
		mp = 1
	}
	p.HP, p.MP = hp, mp

	if err := h.Worlds.MutateEntity(ctx, p.MapID, p.UID, func(e *world.Entity) error {
		e.HP, e.MP = p.HP, p.MP
		e.X, e.Y = p.X, p.Y
		e.Direction = world.Direction(p.Direction)
		return nil
	}); err != nil {
		h.log("sync revive state failed", p, err)
	}

	// SM_REVIVE：客户端据此把尸体重新拉成站立状态
	h.broadcastToAll(ctx, p.MapID, protocol.SM_REVIVE, protocol.SMRevive{
		MapUID:    p.MapUID,
		X:         uint32(p.X),
		Y:         uint32(p.Y),
		Direction: uint16(p.Direction),
	})

	// 站起来（onRevive 里发 makeActionStand）
	h.broadcastToAll(ctx, p.MapID, protocol.SM_ACTION, protocol.SMAction{
		UID: p.UID, MapUID: p.MapUID, Action: actionStand(p),
	})

	h.notifyHealth(ctx, p)

	// 落库
	if h.Chars != nil {
		if err := h.Chars.UpdatePosition(ctx, p.DBID, p.MapID, int32(p.X), int32(p.Y)); err != nil {
			h.log("save revive position failed", p, err)
		}
		if err := h.Chars.UpdateHealth(ctx, p.DBID, p.HP, p.MP); err != nil {
			h.log("save revive health failed", p, err)
		}
	}
	return nil
}

// revivePoint 返回复活点。
//
// 当前版本还没接 maprecord 的 revivePoint 字段（需要 maprecord.json
// 里的配表），所以统一走兜底值。等 maprecord 接入后替换这里即可，
// 调用方不用改。
func (h *DeathHandler) revivePoint(p *Player) (mapID uint32, x, y int) {
	return DefaultReviveMapID, DefaultReviveX, DefaultReviveY
}

// relocate 把玩家挪到指定地图坐标（复活用的系统传送）。
//
// 同图：直接改坐标（不走寻路 —— 复活是瞬移，不是走路）。
// 跨图：当前版本只有一张图常驻，跨图逻辑需要地图切换协议，
// 留给后续批次；这里退化成「在当前图找一个安全格」。
//
// ★ 坐标搜不到时必须保证「一定能复活成功」，不能 return 错误了事。
//
//	复活失败会让玩家永久卡在尸体状态 —— 血量 0、不能施法、
//	不能再点复活（因为 RequestRevive 会重试但同样失败）。
//	所以搜索范围是「先目标点附近 5 格，再全图螺旋」，
//	只要地图上还有一格能站，复活就不会失败。
func (h *DeathHandler) relocate(ctx context.Context, p *Player, mapID uint32, x, y int) error {
	if mapID != p.MapID {
		// 【待办】跨图复活需要 AM_SWITCHMAP 链路，B6 接入
		h.log(fmt.Sprintf("cross-map revive not supported yet (want map %d)", mapID), p, nil)
	}

	destX, destY, found := h.findFreeGrid(ctx, p, x, y)
	if !found {
		return fmt.Errorf("no free grid anywhere on map %d for revive", p.MapID)
	}

	// ★ 必须用 Teleport 而不是 MoveEntity：
	//   复活是瞬移（可能跨 300 格），MoveEntity 的 HopValid 只接受
	//   单跳/两跳/三跳的精确距离，传任何 maxStep 都无法表达「任意距离」。
	entity, ok, err := h.Worlds.Teleport(ctx, p.MapID, p.UID, destX, destY)
	if err != nil || !ok {
		return fmt.Errorf("teleport to revive point: %w", err)
	}
	p.SyncFromWorld(entity)
	return nil
}

// findFreeGrid 从 (x,y) 开始找一个可站立的格。
//
// 第一轮：目标点附近 maxRadius 格内（保持「在复活点附近」的语义）
// 第二轮：全图螺旋（保证一定能找到，避免玩家永久卡尸体）
func (h *DeathHandler) findFreeGrid(ctx context.Context, p *Player, x, y int) (int, int, bool) {
	if h.Worlds.CanOccupy(ctx, p.MapID, p.UID, x, y) {
		return x, y, true
	}
	const maxRadius = 5
	for radius := 1; radius <= maxRadius; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				// 只走环形边缘，避免重复检查内圈
				if abs(dx) != radius && abs(dy) != radius {
					continue
				}
				if h.Worlds.CanOccupy(ctx, p.MapID, p.UID, x+dx, y+dy) {
					return x + dx, y + dy, true
				}
			}
		}
	}

	// 目标点周边没有可站格（比如复活点配置在另一张图）：
	// 退化成从当前位置向外扩散，保证玩家能站起来。
	for radius := 1; radius <= 100; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if abs(dx) != radius && abs(dy) != radius {
					continue
				}
				if h.Worlds.CanOccupy(ctx, p.MapID, p.UID, p.X+dx, p.Y+dy) {
					return p.X + dx, p.Y + dy, true
				}
			}
		}
	}
	return 0, 0, false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// KillByDamage 是外部（战斗结算）通知玩家死亡的入口。
//
// world 层把 HP 扣到 0 之后，game 层必须补上「广播死亡 + 落库」，
// 否则玩家血量是 0 但客户端还画着活人。
func (h *DeathHandler) KillByDamage(ctx context.Context, uid uint64) {
	p, ok := h.Players.Get(uid)
	if !ok {
		return
	}
	// HP 已经在 world 层扣到 0，这里只做副作用广播
	if p.HP > 0 {
		// world 侧已归零但 Player 缓存没同步 —— 读一次真实值
		if e, found, err := h.Worlds.Entity(ctx, p.MapID, uid); err == nil && found {
			p.SyncFromWorld(e)
		}
	}
	if p.HP > 0 {
		return
	}
	h.broadcastToAll(ctx, p.MapID, protocol.SM_NOTIFYDEAD, protocol.SMNotifyDead{UID: p.UID})
	h.broadcastToAll(ctx, p.MapID, protocol.SM_DEADFADEOUT, protocol.SMDeadFadeOut{
		UID:    p.UID,
		MapUID: p.MapUID,
		X:      uint32(p.X),
		Y:      uint32(p.Y),
	})
	if h.Chars != nil {
		if err := h.Chars.UpdateHealth(ctx, p.DBID, p.HP, p.MP); err != nil {
			h.log("save death health failed", p, err)
		}
	}
}

// ==================== 辅助 ====================

func (h *DeathHandler) notifyHealth(ctx context.Context, p *Player) {
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

func (h *DeathHandler) broadcastToAll(ctx context.Context, mapID uint32, id protocol.SMType, value any) {
	payload, err := protocol.AppendWirePayload(nil, value)
	if err != nil {
		return
	}
	if h.Broadcast == nil {
		return
	}
	for _, other := range h.Players.Snapshot() {
		if other.MapID != mapID {
			continue
		}
		_ = h.Broadcast.SendTo(other.ChannelID, id, payload, 0)
	}
}

func (h *DeathHandler) log(msg string, p *Player, err error) {
	if h.Logger == nil {
		return
	}
	attrs := []any{"player", p.Name, "uid", common.GetUIDString(p.UID)}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	h.Logger.Warn(msg, attrs...)
}
