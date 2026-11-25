package game

import (
	"context"
	"fmt"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 背包/装备消息处理（playernet.cpp）====================
//
// 覆盖 6 条：
//
//	CM_REQUESTEQUIPWEAR -> SM_EQUIPWEAR / SM_EQUIPWEARERROR
//	CM_REQUESTGRABWEAR  -> SM_GRABWEAR  / SM_GRABWEARERROR
//	CM_REQUESTEQUIPBELT -> SM_EQUIPBELT / SM_EQUIPBELTERROR
//	CM_REQUESTGRABBELT  -> SM_GRABBELT  / SM_GRABBELTERROR
//	CM_DROPITEM         -> 地面出现物品（SM_GROUNDITEMIDLIST）
//	CM_CONSUMEITEM      -> SM_HEALTH / SM_INVENTORY
//
// 装备错误码（protocoldef.hpp:326-364）：
//
//	EQWERR_NOITEM=1  EQWERR_BADITEM=2  EQWERR_BADWLTYPE=3
//	EQWERR_INSUFF=4  EQWERR_STALLSELLING=5
//	GWERR_NOITEM=1   GWERR_BIND=2      GWERR_STALLSELLING=3
//	EQBERR_NOITEM=1  EQBERR_BADITEM=2  EQBERR_BADITEMTYPE=3
//	EQBERR_BADSLOT=4 EQBERR_STALLSELLING=5
//	GBERR_NOITEM=1   GBERR_STALLSELLING=2

const (
	EQWErrNone         = 0
	EQWErrNoItem       = 1
	EQWErrBadItem      = 2
	EQWErrBadWLType    = 3
	EQWErrInsuff       = 4
	EQWErrStallSelling = 5

	GWErrNone         = 0
	GWErrNoItem       = 1
	GWErrBind         = 2
	GWErrStallSelling = 3

	EQBErrNone         = 0
	EQBErrNoItem       = 1
	EQBErrBadItem      = 2
	EQBErrBadItemType  = 3
	EQBErrBadSlot      = 4
	EQBErrStallSelling = 5

	GBErrNone         = 0
	GBErrNoItem       = 1
	GBErrStallSelling = 2
)

// InventoryHandler 处理背包与装备消息。
type InventoryHandler struct {
	Players   *PlayerManager
	Items     *ItemDB
	Worlds    *world.World
	Broadcast Broadcaster
	// Items 的持久化（可为 nil，测试时可省）
	ItemRepo ItemRepository
	Sessions *SessionManager
}

// ItemRepository 是装备/背包落库所需的最小接口。
//
// 抽成接口而不是直接依赖 storage.ItemRepository，是为了让测试
// 可以不建数据库就跑背包逻辑（纯内存验证）。
type ItemRepository interface {
	PutInventoryItem(ctx context.Context, dbid uint32, item protocol.SDItem) error
	RemoveInventoryItem(ctx context.Context, dbid, itemID, seqID uint32) (bool, error)
	PutWearItem(ctx context.Context, dbid uint32, slot int32, item protocol.SDItem) error
	RemoveWearItem(ctx context.Context, dbid uint32, slot int32) (bool, error)
	PutBeltItem(ctx context.Context, dbid uint32, slot int32, item protocol.SDItem) error
	RemoveBeltItem(ctx context.Context, dbid uint32, slot int32) (bool, error)
}

// playerOf 取调用者并校验在线。
func (h *InventoryHandler) playerOf(channelID uint64) (*Player, bool) {
	p, ok := h.Players.ByChannel(channelID)
	return p, ok
}

// toSDItem 把内部 Item 转成协议层 SDItem。
//
// ★ 必须带 Duration：存储层是按 Duration[2] 落库的，
//
//	转换时丢掉的话，装备每次「穿上/脱下/重登」都会恢复满持久 ——
//	磨损系统等于不存在，而且不会有任何报错。
func toSDItem(it Item) protocol.SDItem {
	return protocol.SDItem{
		ItemID:   it.ItemID,
		SeqID:    uint32(it.SeqID),
		Count:    uint64(it.Count),
		Duration: [2]uint64{uint64(it.Durability), uint64(it.MaxDurability)},
	}
}

// fromSDItem 把协议层 SDItem 转成内部 Item。
func fromSDItem(it protocol.SDItem) Item {
	return Item{
		ItemID:        it.ItemID,
		SeqID:         uint64(it.SeqID),
		Count:         uint32(it.Count),
		Durability:    uint32(it.Duration[0]),
		MaxDurability: uint32(it.Duration[1]),
	}
}

// sendSM 用 cerealf 序列化并发送。
//
// ★ 只用于 **MessageVariable** 类消息（SM_EQUIPWEAR / SM_GRABWEAR /
//
//	SM_EQUIPBELT / SM_GRABBELT / SM_BELT）。
//	错误类消息（SM_EQUIPWEARERROR 等）是 MessageFixedCompressed，
//	必须走 sendWire，否则客户端解码会错位。
//
// 参数必须是 protocol.PortableMarshaler —— CerealSerialize 要求这个接口，
// 传 any 编译不过。
func (h *InventoryHandler) sendSM(channelID uint64, id protocol.SMType, value protocol.PortableMarshaler) {
	if h.Broadcast == nil || channelID == 0 {
		return
	}
	payload, err := protocol.CerealSerialize(value)
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(channelID, id, payload, 0)
}

// sendWire 用定长 wire 编码发送（MessageFixedCompressed 类消息）。
//
// 错误类消息（SM_EQUIPWEARERROR/SM_GRABWEARERROR/SM_EQUIPBELTERROR/
// SM_GRABBELTERROR）都是定长 10 字节，必须走这里。
func (h *InventoryHandler) sendWire(channelID uint64, id protocol.SMType, value any) {
	if h.Broadcast == nil || channelID == 0 {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, value)
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(channelID, id, payload, 0)
}

// ==================== 穿装备 ====================

// RequestEquipWear 穿装备（playernet.cpp:998）。
//
// 顺序严格照抄：
//
//  1. 查物品记录，不存在 -> BADITEM
//  2. 槽位非法 -> BADWLTYPE
//  3. 物品不能穿这个槽 -> BADWLTYPE
//  4. 背包里没有 -> NOITEM
//  5. 等级/职业不够 -> INSUFF
//  6. 记下当前该槽的装备 -> 装新的 -> 从背包移除 -> 落库 -> 推 SM_EQUIPWEAR
//  7. 旧装备回背包
func (h *InventoryHandler) RequestEquipWear(ctx context.Context, channelID uint64, req protocol.CMRequestEquipWear) error {
	p, ok := h.playerOf(channelID)
	if !ok {
		return nil
	}

	fail := func(code uint16) error {
		h.sendWire(channelID, protocol.SM_EQUIPWEARERROR, protocol.SMEquipWearError{
			ItemID: req.ItemID, SeqID: req.SeqID, Error: code,
		})
		return nil
	}

	rec, exists := h.Items.Get(req.ItemID)
	if !exists {
		return fail(EQWErrBadItem)
	}

	wltype := int(req.WLType)
	if !WLGValid(wltype) {
		return fail(EQWErrBadWLType)
	}
	if !rec.Wearable(wltype) {
		return fail(EQWErrBadWLType)
	}

	seqID := uint32(req.SeqID)
	idx, it := p.findItem(req.ItemID, seqID)
	if it == nil {
		return fail(EQWErrNoItem)
	}
	if !canWear(rec, p.Level, p.Job) {
		return fail(EQWErrInsuff)
	}

	// 先复制（removeWholeItem 会让 it 失效）
	newItem := Item{ItemID: it.ItemID, SeqID: it.SeqID, Count: it.Count}
	_ = idx

	old := p.wearItem(wltype)

	// 装新的
	p.setWearItem(wltype, &newItem)
	p.removeWholeItem(req.ItemID, seqID)

	// 落库
	if h.ItemRepo != nil {
		_, _ = h.ItemRepo.RemoveInventoryItem(ctx, p.DBID, req.ItemID, seqID)
		_ = h.ItemRepo.PutWearItem(ctx, p.DBID, int32(wltype), toSDItem(newItem))
	}

	// 推装备成功
	h.sendSM(channelID, protocol.SM_EQUIPWEAR, protocol.SDEquipWear{
		UID:    p.UID,
		WLType: int32(wltype),
		Item:   toSDItem(newItem),
	})

	// 旧装备回背包
	if old != nil {
		p.addItemNoRec(*old)
		if h.ItemRepo != nil {
			_ = h.ItemRepo.PutInventoryItem(ctx, p.DBID, toSDItem(*old))
		}
	}

	h.pushInventory(channelID, p)
	return nil
}

// ==================== 脱装备 ====================

// RequestGrabWear 脱装备（playernet.cpp:1145）。
//
// 原版注释：脱下来的装备**永远进背包**，服务端不区分「手上拿着」还是「在背包里」。
func (h *InventoryHandler) RequestGrabWear(ctx context.Context, channelID uint64, req protocol.CMRequestGrabWear) error {
	p, ok := h.playerOf(channelID)
	if !ok {
		return nil
	}

	fail := func(code uint16) error {
		h.sendWire(channelID, protocol.SM_GRABWEARERROR, protocol.SMGrabWearError{Error: code})
		return nil
	}

	wltype := int(req.WLType)
	if !WLGValid(wltype) {
		return fail(GWErrNoItem)
	}

	curr := p.wearItem(wltype)
	if curr == nil {
		return fail(GWErrNoItem)
	}

	item := *curr
	p.setWearItem(wltype, nil)
	p.addItemNoRec(item)

	if h.ItemRepo != nil {
		_, _ = h.ItemRepo.RemoveWearItem(ctx, p.DBID, int32(wltype))
		_ = h.ItemRepo.PutInventoryItem(ctx, p.DBID, toSDItem(item))
	}

	h.sendSM(channelID, protocol.SM_GRABWEAR, protocol.SDGrabWear{
		WLType: int32(wltype),
		Item:   toSDItem(item),
	})
	h.pushInventory(channelID, p)
	return nil
}

// ==================== 装备到腰带 ====================

// RequestEquipBelt 把物品放进腰带（playernet.cpp:1081）。
func (h *InventoryHandler) RequestEquipBelt(ctx context.Context, channelID uint64, req protocol.CMRequestEquipBelt) error {
	p, ok := h.playerOf(channelID)
	if !ok {
		return nil
	}

	fail := func(code uint16) error {
		h.sendWire(channelID, protocol.SM_EQUIPBELTERROR, protocol.SMEquipBeltError{
			ItemID: req.ItemID, SeqID: req.SeqID, Error: code,
		})
		return nil
	}

	rec, exists := h.Items.Get(req.ItemID)
	if !exists {
		return fail(EQBErrBadItem)
	}
	if !rec.Beltable() {
		return fail(EQBErrBadItemType)
	}

	slot := int(req.Slot)
	if slot < 0 || slot >= beltSlotCount {
		return fail(EQBErrBadSlot)
	}

	seqID := uint32(req.SeqID)
	_, it := p.findItem(req.ItemID, seqID)
	if it == nil {
		return fail(EQBErrNoItem)
	}

	newItem := Item{ItemID: it.ItemID, SeqID: it.SeqID, Count: it.Count}
	old := p.beltItem(slot)

	p.setBeltItem(slot, &newItem)
	p.removeWholeItem(req.ItemID, seqID)

	if h.ItemRepo != nil {
		_, _ = h.ItemRepo.RemoveInventoryItem(ctx, p.DBID, req.ItemID, seqID)
		_ = h.ItemRepo.PutBeltItem(ctx, p.DBID, int32(slot), toSDItem(newItem))
	}

	h.sendSM(channelID, protocol.SM_EQUIPBELT, protocol.SDEquipBelt{
		Slot: int32(slot),
		Item: toSDItem(newItem),
	})

	if old != nil {
		p.addItemNoRec(*old)
		if h.ItemRepo != nil {
			_ = h.ItemRepo.PutInventoryItem(ctx, p.DBID, toSDItem(*old))
		}
	}

	h.pushInventory(channelID, p)
	h.pushBelt(channelID, p)
	return nil
}

// RequestGrabBelt 从腰带取下（playernet.cpp:1194）。
func (h *InventoryHandler) RequestGrabBelt(ctx context.Context, channelID uint64, req protocol.CMRequestGrabBelt) error {
	p, ok := h.playerOf(channelID)
	if !ok {
		return nil
	}

	fail := func(code uint16) error {
		h.sendWire(channelID, protocol.SM_GRABBELTERROR, protocol.SMGrabBeltError{Error: code})
		return nil
	}

	slot := int(req.Slot)
	if slot < 0 || slot >= beltSlotCount {
		return fail(GBErrNoItem)
	}
	curr := p.beltItem(slot)
	if curr == nil {
		return fail(GBErrNoItem)
	}

	item := *curr
	p.setBeltItem(slot, nil)
	p.addItemNoRec(item)

	if h.ItemRepo != nil {
		_, _ = h.ItemRepo.RemoveBeltItem(ctx, p.DBID, int32(slot))
		_ = h.ItemRepo.PutInventoryItem(ctx, p.DBID, toSDItem(item))
	}

	h.sendSM(channelID, protocol.SM_GRABBELT, protocol.SDGrabBelt{
		Slot: int32(slot),
		Item: toSDItem(item),
	})
	h.pushInventory(channelID, p)
	h.pushBelt(channelID, p)
	return nil
}

// ==================== 丢弃 ====================

// DropItem 丢弃背包物品（playernet.cpp:1471）。
//
// 流程：从背包移除 -> 向地图投 AM_DROPITEMLIST -> 地面出现物品。
func (h *InventoryHandler) DropItem(ctx context.Context, channelID uint64, req protocol.CMDropItem) error {
	p, ok := h.playerOf(channelID)
	if !ok {
		return nil
	}

	count := uint32(req.Count)
	if count == 0 {
		count = 1
	}

	if !p.hasCount(req.ItemID, req.SeqID, count) {
		return nil // 原版 fflassert(dropItem)，这里静默忽略
	}

	_, it := p.findItem(req.ItemID, req.SeqID)
	if it == nil {
		return nil
	}

	// ★ 先落地，成功后再扣背包。
	//
	// 不能「先扣再落」：落地失败（地图不可走、world 未装配、网络异常）
	// 会导致物品凭空消失。初版就是先 removeItem 再 DropItem，
	// 失败时用 addItemNoRec 归还 —— 结果归还的是**新格**（seqID 变了），
	// 原本那一格的数量已经扣掉，玩家实际少了东西。
	if h.Worlds == nil {
		// 没有地图上下文（纯背包测试/离线场景）：不做掉落，背包不动
		return nil
	}
	if _, err := h.Worlds.DropItem(ctx, p.MapID, p.X, p.Y, []uint32{req.ItemID}); err != nil {
		return nil // 落地失败：背包不动，物品不丢
	}

	// 落地成功，扣背包
	if !p.removeItem(req.ItemID, req.SeqID, count) {
		return nil
	}
	if h.ItemRepo != nil {
		_, _ = h.ItemRepo.RemoveInventoryItem(ctx, p.DBID, req.ItemID, req.SeqID)
	}

	h.pushInventory(channelID, p)
	return nil
}

// ==================== 使用物品 ====================

// ConsumeItem 使用背包物品（playernet.cpp:1509）。
//
// 目前支持：恢复药水（回血回蓝）、秘籍（学魔法）。
// 普通技能书是原料，不能直接学 —— 原版 isMagicBook 才可学。
func (h *InventoryHandler) ConsumeItem(ctx context.Context, channelID uint64, req protocol.CMConsumeItem) error {
	p, ok := h.playerOf(channelID)
	if !ok {
		return nil
	}

	count := uint32(req.Count)
	if count == 0 {
		count = 1
	}

	rec, exists := h.Items.Get(req.ItemID)
	if !exists {
		return nil
	}
	if !p.hasCount(req.ItemID, req.SeqID, count) {
		return nil
	}

	consumedOK := false
	switch {
	case rec.IsPotion():
		// 一次只喝一瓶（原版 consumePotion 单次调用）
		consumedOK = p.consumePotion(rec)
		if consumedOK {
			count = 1
		}
	case rec.IsMagicBook():
		// 学魔法：把 magic 名字记进已学列表
		consumedOK = h.learnMagic(p, rec)
		if consumedOK {
			count = 1
		}
	default:
		// 其它类型暂不支持（原版也是 TODO）
		return nil
	}

	if !consumedOK {
		return nil
	}

	p.removeItem(req.ItemID, req.SeqID, count)
	if h.ItemRepo != nil {
		_, _ = h.ItemRepo.RemoveInventoryItem(ctx, p.DBID, req.ItemID, req.SeqID)
	}

	// 推血量与背包
	h.pushHealth(channelID, p)
	h.pushInventory(channelID, p)
	return nil
}

// learnMagic 让玩家学会秘籍对应的魔法。
//
// 返回是否学会（已会则不重复学，返回 false 避免白消耗一本书）。
func (h *InventoryHandler) learnMagic(p *Player, rec ItemRecord) bool {
	if rec.Book == nil || rec.Book.Magic == "" {
		return false
	}
	magicName := rec.Book.Magic
	// 已学列表存的是 magicID；这里用名字查表
	magicID, ok := h.Items.FindByName(magicName)
	if !ok {
		return false
	}
	for _, m := range p.Magics {
		if m == magicID {
			return false // 已学会
		}
	}
	p.Magics = append(p.Magics, magicID)
	return true
}

// ==================== 下发辅助 ====================

// pushInventory 推背包全量。
func (h *InventoryHandler) pushInventory(channelID uint64, p *Player) {
	payload, err := BuildInventoryPayload(p)
	if err != nil || h.Broadcast == nil {
		return
	}
	_ = h.Broadcast.SendTo(channelID, protocol.SM_INVENTORY, payload, 0)
}

// pushBelt 推腰带全量。
func (h *InventoryHandler) pushBelt(channelID uint64, p *Player) {
	h.sendSM(channelID, protocol.SM_BELT, BuildBeltPayload(p))
}

// pushHealth 推血量。
func (h *InventoryHandler) pushHealth(channelID uint64, p *Player) {
	payload, err := protocol.CerealSerialize(protocol.SDHealth{
		UID: p.UID, HP: p.HP, MP: p.MP, MaxHP: p.MaxHP, MaxMP: p.MaxMP,
	})
	if err != nil || h.Broadcast == nil {
		return
	}
	_ = h.Broadcast.SendTo(channelID, protocol.SM_HEALTH, payload, 0)
}

// BuildBeltPayload 把腰带编成 SM_BELT 的 body。
//
// SM_BELT 的 body 是 cerealf 序列化的 SDBelt{List [6]optional<SDItem>}。
func BuildBeltPayload(p *Player) protocol.SDBelt {
	// SDBelt.List 是 [6]SDItem（值数组），空格用 itemID=0 表示
	var list [beltSlotCount]protocol.SDItem
	for i := 0; i < beltSlotCount; i++ {
		if it := p.beltItem(i); it != nil {
			list[i] = toSDItem(*it)
		}
	}
	return protocol.SDBelt{List: list}
}

// FormatEquipError 把错误码转成可读文字（日志用）。
func FormatEquipError(code uint16) string {
	switch code {
	case EQWErrNone:
		return "ok"
	case EQWErrNoItem:
		return "背包里没有该物品"
	case EQWErrBadItem:
		return "物品不存在"
	case EQWErrBadWLType:
		return "该物品不能穿到这个位置"
	case EQWErrInsuff:
		return "等级或职业不满足"
	case EQWErrStallSelling:
		return "摆摊中不能换装备"
	default:
		return fmt.Sprintf("未知错误 %d", code)
	}
}
