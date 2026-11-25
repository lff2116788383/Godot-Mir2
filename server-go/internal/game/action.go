package game

import (
	"context"
	"fmt"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 玩家动作处理（playernet.cpp:net_CM_ACTION）====================
//
// 原版的链路：
//
//	CM_ACTION 到达
//	  -> 校验 UID 与 mapUID 是自己的
//	  -> 按 action.type 分发到 onCMActionXxx
//	  -> 每种动作自己校验（estimateHop / LDistance2 / 装备检查）
//	  -> requestMove 向地图申请移动（AM_TRYMOVE，返回 AM_ALLOWMOVE）
//	  -> dispatchAction 把动作转发给观察者（AM_ACTION）
//	  -> 地图把它转成 SM_ACTION 发给视野内的玩家
//
// 这里是同样的顺序，只是把 actor 间的 AM_* 换成了直接调用 world 层。
//
// ★ 三个必须照抄的校验（客户端复刻时踩过）：
//  1. **只接收自己 UID + 自己 mapUID 的动作**（否则客户端能操作别人）
//  2. **移动必须是一跳**（estimateHop == 1），服务端不做寻路
//  3. 校验失败要回 **reportStand**（SM_ACTION 站立），而不是静默丢包 ——
//     否则客户端会一直卡在移动动画里

// ActionHandler 处理玩家动作。
type ActionHandler struct {
	Players   *PlayerManager
	Sessions  *SessionManager
	Worlds    *world.World
	Broadcast Broadcaster
	// Spells 施法处理器（ACTION_SPELL 转发给它）。
	// 为 nil 时施法退化成「回站立」，不会崩但技能放不出来。
	Spells *SpellHandler
}

// Broadcaster 把一条 SM 发给指定玩家。
//
// 由 transport 层实现（Session.Send）。抽成接口是为了让 game 包
// 不直接依赖连接细节，测试时可以用假实现。
type Broadcaster interface {
	SendTo(channelID uint64, id protocol.SMType, payload []byte, respID uint64) error
}

// CMAction 处理客户端动作消息。
//
// 返回被广播的动作（供诊断/日志），以及是否处理成功。
func (h *ActionHandler) CMAction(ctx context.Context, channelID uint64, req protocol.CMAction) error {
	player, ok := h.Players.ByChannel(channelID)
	if !ok {
		return nil // 还没 online，忽略（原版也一样）
	}

	// 1. 只接受自己的动作（playernet.cpp:36-38）
	if req.UID != player.UID || req.MapUID != player.MapUID {
		return nil
	}

	action := req.Action

	// 2. 按类型分发
	switch action.Type {
	case common.ActionStand:
		return h.handleStand(ctx, player, action)
	case common.ActionMove:
		return h.handleMove(ctx, player, action)
	case common.ActionAttack:
		return h.handleAttack(ctx, player, action)
	case common.ActionPickUp:
		return h.handlePickUp(ctx, player, action)
	case common.ActionSpell:
		// 施法：交给 SpellHandler（原版 onCMActionSpell）
		if h.Spells == nil {
			return h.reportStand(ctx, player)
		}
		return h.Spells.Cast(ctx, channelID, action)
	case common.ActionMine, common.ActionSpinKick:
		// 挖矿/回旋踢：B6 实现；先回站立让客户端状态收敛
		return h.reportStand(ctx, player)
	default:
		return nil
	}
}

// handleStand 处理站立/重同步（player.cpp:1606）。
//
// 原版注释写得很清楚：
//
//	server get report stand, means client is trying to re-sync
//	try client's current location and always response
func (h *ActionHandler) handleStand(ctx context.Context, p *Player, action common.ActionNode) error {
	nx, ny := int(action.X), int(action.Y)

	// 方向合法就采纳（客户端转身后 local 已经转了）
	if common.DirectionValid(action.Direction) {
		p.Direction = action.Direction
	}

	// 位置与服务器不一致 -> 尝试挪过去，失败则强制回服务器位置
	if nx != p.X || ny != p.Y {
		if !h.tryMove(ctx, p, nx, ny, 3) {
			return h.reportStand(ctx, p)
		}
	}
	return h.reportStand(ctx, p)
}

// handleMove 处理移动（player.cpp:1642）。
//
// 原版顺序：
//
//	estimateHop(stCMA.action.x, stCMA.action.y)   // 报文里的「起点」
//	case 0: 起点就是当前位置 -> 移动到 aim 点
//	case 1: 起点不在当前位置（客户端超前了）-> 先挪到起点再递归
//	default: 回站立
func (h *ActionHandler) handleMove(ctx context.Context, p *Player, action common.ActionNode) error {
	x0, y0 := int(action.X), int(action.Y)
	x1, y1 := int(action.AimX), int(action.AimY)

	switch estimateHop(p.X, p.Y, x0, y0) {
	case 0:
		// 起点 == 当前位置，直接走到目标
		if !h.tryMove(ctx, p, x1, y1, p.moveSpeed()) {
			return h.reportStand(ctx, p)
		}
		// 广播移动动作
		return h.broadcastAction(ctx, p, common.ActionNode{
			Type:      common.ActionMove,
			Speed:     uint16(p.moveSpeed()),
			Direction: p.Direction,
			X:         int16(p.X),
			Y:         int16(p.Y),
			AimX:      int16(p.X),
			AimY:      int16(p.Y),
		})

	case 1:
		// 客户端超前：先把位置拉到起点，再重试
		if !h.tryMove(ctx, p, x0, y0, 3) {
			return h.reportStand(ctx, p)
		}
		return h.handleMove(ctx, p, action)

	default:
		return h.reportStand(ctx, p)
	}
}

// handleAttack 处理攻击（player.cpp:1715）。
//
// 先校验距离，命中则结算并广播；未命中广播 miss。
func (h *ActionHandler) handleAttack(ctx context.Context, p *Player, action common.ActionNode) error {
	aimUID := action.AimUID
	if aimUID == 0 || aimUID == p.UID {
		return nil
	}

	// 同图检查
	aim, ok := h.Players.Get(aimUID)
	if !ok {
		// 可能打的是怪物：交给 world 判断
		return h.attackWorldEntity(ctx, p, aimUID, action)
	}
	if aim.MapID != p.MapID {
		return nil
	}

	// 面向目标
	if dir := world.DirectionBetween(
		world.Point{X: p.X, Y: p.Y}, world.Point{X: aim.X, Y: aim.Y}); dir.Valid() {
		p.Direction = uint8(dir)
	}

	// 距离必须相邻（原版 LDistance2 ∈ {1,2}）
	d2 := world.LDistance2(p.X, p.Y, aim.X, aim.Y)
	if d2 != 1 && d2 != 2 {
		return h.reportStand(ctx, p)
	}

	return h.attackWorldEntity(ctx, p, aimUID, action)
}

// attackWorldEntity 在地图 actor 里执行一次完整伤害结算。
func (h *ActionHandler) attackWorldEntity(ctx context.Context, p *Player, aimUID uint64, action common.ActionNode) error {
	magicID := int(ExtParamMagicID(action))
	// ★ magicID == 0 表示「普通物理攻击」。
	//   原版 extParam.attack.magicID 由客户端填 magicrecord 里的 ID，
	//   但老客户端/异常包可能是 0；而 world 层 DamageNode.Valid() 要求
	//   MagicID != 0，不兜底就会变成「打了但完全没反应」。
	if magicID == 0 {
		magicID = world.MagicIDPlainAttack
	}
	result, err := h.Worlds.Combat(ctx, p.MapID, p.UID, aimUID, magicID)
	if err != nil {
		// 目标不存在/已死/超距：回站立
		return h.reportStand(ctx, p)
	}

	// 广播攻击动作（客户端据此播动画）
	if err := h.broadcastAction(ctx, p, common.ActionNode{
		Type:      common.ActionAttack,
		Direction: p.Direction,
		X:         int16(p.X),
		Y:         int16(p.Y),
		AimX:      int16(action.AimX),
		AimY:      int16(action.AimY),
		AimUID:    aimUID,
	}); err != nil {
		return err
	}

	// 广播伤害/未命中
	//
	// ★ 命中但伤害为 0（被护甲完全抵消）也要推血量：
	//   客户端要靠血量包确认「这一下打中了但没破防」，
	//   否则它会认为攻击没生效、继续重试同样的动作。
	if result.Hit {
		h.broadcastDamage(ctx, p, aimUID, result.Damage)
	} else {
		h.broadcastMiss(ctx, p, aimUID)
	}

	// 打死则广播死亡
	if result.Dead {
		h.broadcastDeath(ctx, p, aimUID)
	}
	return nil
}

// handlePickUp 处理拾取。
func (h *ActionHandler) handlePickUp(ctx context.Context, p *Player, action common.ActionNode) error {
	// 原版 m_pickUpLock 节流（防止狂发包刷物品）
	if p.PickUpLock {
		return nil
	}
	targetX, targetY := int(action.AimX), int(action.AimY)
	if d2 := world.LDistance2(p.X, p.Y, targetX, targetY); d2 > 2 {
		return h.reportStand(ctx, p)
	}

	// 由 world 层执行拾取（地面物品在那里）
	got, err := h.Worlds.PickupOne(ctx, p.MapID, targetX, targetY, p.UID)
	if err != nil || len(got) == 0 {
		return nil
	}

	// 进背包
	for _, itemID := range got {
		p.Inventory = append(p.Inventory, Item{ItemID: itemID, SeqID: 1, Count: 1})
	}
	// 回背包变更 + 地面物品变更
	h.notifyInventory(ctx, p)
	return nil
}

// tryMove 向地图申请移动，成功后同步回 Player。
//
// ★ 关键语义（servermapop.cpp:267-294）：maxStep 是**单跳步长**，不是速度。
//
//	LDistance2 ∈ {1,2}  -> step 1（正交/对角相邻）
//	LDistance2 ∈ {4,8}  -> step 2
//	LDistance2 ∈ {9,18} -> step 3
//	其它                -> 非法
//
// 初版把 moveSpeed()（600ms）当 maxStep 传进去，HopValid 全部失败，
// 玩家看起来「怎么都走不动」。speed 只影响客户端动画节奏，不参与网格校验。
func (h *ActionHandler) tryMove(ctx context.Context, p *Player, dstX, dstY int, _ int) bool {
	step := hopStep(world.LDistance2(p.X, p.Y, dstX, dstY))
	if step == 0 {
		return false
	}
	entity, ok, err := h.Worlds.MoveEntityFull(ctx, p.MapID, p.UID, dstX, dstY, step)
	if err != nil || !ok {
		return false
	}
	p.SyncFromWorld(entity)
	return true
}

// hopStep 把 LDistance2 映射成单跳步长（servermapop.cpp:268-294）。
//
// 返回 0 表示距离非法（不是 1/2/4/8/9/18 之一）。
func hopStep(distance2 int) int {
	switch distance2 {
	case 1, 2:
		return 1
	case 4, 8:
		return 2
	case 9, 18:
		return 3
	default:
		return 0
	}
}

// reportStand 让玩家回到站立状态并发给客户端。
//
// 原版 player.cpp:1116 reportStand -> reportAction(UID, mapUID, makeActionStand())
func (h *ActionHandler) reportStand(ctx context.Context, p *Player) error {
	return h.broadcastAction(ctx, p, common.ActionNode{
		Type:      common.ActionStand,
		Direction: p.Direction,
		X:         int16(p.X),
		Y:         int16(p.Y),
		AimX:      int16(p.X),
		AimY:      int16(p.Y),
	})
}

// broadcastAction 把动作发给「能看到玩家的所有人 + 玩家自己」。
//
// 原版是通过地图的 forwardObserverAction：取源格与目标格的观察者并集。
// 这里向 world 查询观察者，再逐个 Send。
func (h *ActionHandler) broadcastAction(ctx context.Context, p *Player, action common.ActionNode) error {
	// 自己一定收到（客户端要播自己的动画）
	if err := h.sendTo(ctx, p.ChannelID, protocol.SM_ACTION, protocol.SMAction{
		UID: p.UID, MapUID: p.MapUID, Action: action,
	}); err != nil {
		return err
	}

	observers, err := h.Worlds.ForwardAction(ctx, p.MapID, p.UID, p.X, p.Y, p.X, p.Y)
	if err != nil {
		return nil // 观察者查询失败不影响自己
	}
	for _, observerUID := range observers {
		other, ok := h.Players.Get(observerUID)
		if !ok {
			continue
		}
		if err := h.sendTo(ctx, other.ChannelID, protocol.SM_ACTION, protocol.SMAction{
			UID: p.UID, MapUID: p.MapUID, Action: action,
		}); err != nil {
			continue // 单个下发失败不阻断其他
		}
	}
	return nil
}

// broadcastDamage 广播伤害消息。
//
// 注意：真正的伤害数字由客户端根据 SM_ACTION（attack 动作）+ 目标血量变化
// 自己算显示，服务端只需推血量。SM_STRIKEGRID 是「打击格高亮」，
// 落在被击中的格上（不是攻击者位置）。
func (h *ActionHandler) broadcastDamage(ctx context.Context, p *Player, targetUID uint64, damage int32) error {
	// 目标血量变了，重新读一次并推给目标自己
	if e, ok, err := h.Worlds.Entity(ctx, p.MapID, targetUID); err == nil && ok {
		if target, exists := h.Players.Get(targetUID); exists {
			target.SyncFromWorld(e)
			h.notifyHealth(ctx, target)
		}
		// 打击格落在目标位置
		return h.broadcastToAll(ctx, p.MapID, protocol.SM_STRIKEGRID, protocol.SMStrikeGrid{
			X: uint32(e.X), Y: uint32(e.Y),
		})
	}
	return nil
}

// broadcastMiss 广播未命中。
func (h *ActionHandler) broadcastMiss(ctx context.Context, p *Player, targetUID uint64) error {
	return h.broadcastToAll(ctx, p.MapID, protocol.SM_MISS, protocol.SMMiss{UID: targetUID})
}

// broadcastDeath 广播死亡（SM_NOTIFYDEAD）。
func (h *ActionHandler) broadcastDeath(ctx context.Context, p *Player, targetUID uint64) error {
	return h.broadcastToAll(ctx, p.MapID, protocol.SM_NOTIFYDEAD, protocol.SMNotifyDead{UID: targetUID})
}

// broadcastToAll 把一条消息发给同图所有玩家。
func (h *ActionHandler) broadcastToAll(ctx context.Context, mapID uint32, id protocol.SMType, value any) error {
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

// notifyHealth 把血量推给玩家自己（SM_HEALTH）。
//
// ★ SM_HEALTH 是 MessageVariable，body 必须用 cerealf 序列化。
//
//	SDHealth 里有 SDTaggedValMap 字段（map 类型），wire 编码器不支持，
//	走 AppendWirePayload 会报 "unsupported wire type map[int32]int32"，
//	消息静默丢弃 —— 表现就是「打中了但客户端血量不变」。
func (h *ActionHandler) notifyHealth(ctx context.Context, p *Player) {
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

// notifyInventory 把背包推给玩家自己（SM_INVENTORY）。
//
// SM_INVENTORY 的 body 是 cerealf 序列化的 SDInventory{ItemList}。
// 必须走 CerealSerialize —— SM_INVENTORY 是 MessageVariable 类型，
// 与 SM_HEALTH 那种定长消息的编码方式不同。
func (h *ActionHandler) notifyInventory(ctx context.Context, p *Player) {
	if p.ChannelID == 0 {
		return
	}
	payload, err := BuildInventoryPayload(p)
	if err != nil || h.Broadcast == nil {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_INVENTORY, payload, 0)
}

// BuildInventoryPayload 把背包编成 SM_INVENTORY 的 body。
func BuildInventoryPayload(p *Player) ([]byte, error) {
	items := make([]protocol.SDItem, 0, len(p.Inventory))
	for _, it := range p.Inventory {
		items = append(items, protocol.SDItem{
			ItemID: it.ItemID,
			SeqID:  uint32(it.SeqID),
			Count:  uint64(it.Count),
		})
	}
	return protocol.CerealSerialize(protocol.SDInventory{ItemList: items})
}

// sendTo 编码并发送。
func (h *ActionHandler) sendTo(ctx context.Context, channelID uint64, id protocol.SMType, value any) error {
	if channelID == 0 || h.Broadcast == nil {
		return nil
	}
	payload, err := protocol.AppendWirePayload(nil, value)
	if err != nil {
		return fmt.Errorf("encode %v: %w", id, err)
	}
	return h.Broadcast.SendTo(channelID, id, payload, 0)
}

// estimateHop 对应 battleobject.cpp:738。
//
// 返回值：
//
//	0  同格
//	1  一跳可达
//	2  多跳（客户端不该发这种）
//	-1 坐标非法
func estimateHop(selfX, selfY, nx, ny int) int {
	// 严格照抄 battleobject.cpp:738-773。
	//
	// ★ 注意这跟 hopStep 不是同一件事：
	//   estimateHop 判断「报文里的起点 = 我现在的位置吗」，
	//   只有 distance² ∈ {1,2} 时才认为「一跳以内」；
	//   2 格以上一律返回 2（原版进 default 分支后，
	//   玩家 maxStep 通常为 1，1*1²=1 / 2*1²=2 都不匹配 distance²，
	//   所以最终还是返回 2）。
	//
	//   初版把 hopStep 的 {4,8,9,18} 也算成一跳，导致
	//   「客户端报 2 格外起点」这种明显异常被当成正常开局。
	switch world.LDistance2(selfX, selfY, nx, ny) {
	case 0:
		return 0
	case 1, 2:
		return 1
	default:
		return 2
	}
}

// moveSpeed 返回玩家的移动速度（每跳毫秒数）。
//
// 原版取自 hero 的 SYS_*SPEED 常量，这里先给与客户端一致的默认值。
func (p *Player) moveSpeed() int {
	if p.Level >= 40 {
		return 480
	}
	return 600
}

// ExtParamMagicID 从动作扩展参数里取 magicID。
//
// ActionNode.ExtParam 是 8 字节：前 4 字节 magicID（小端）。
// 注意不能写成 ActionNode 的方法 —— common 包外不允许给外部类型加方法。
func ExtParamMagicID(n common.ActionNode) uint32 {
	return uint32(n.ExtParam[0]) |
		uint32(n.ExtParam[1])<<8 |
		uint32(n.ExtParam[2])<<16 |
		uint32(n.ExtParam[3])<<24
}
