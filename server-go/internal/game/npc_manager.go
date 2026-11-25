package game

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== NPC 对话（server/src/npchar.cpp）====================
//
// 链路：
//
//	CM_NPCEVENT  -> 找到 NPC 的 Lua 实例 -> 协程跑 _RSVD_NAME_npc_main
//	             -> 脚本调 uidPostXML -> 我们发 SM_NPCXMLLAYOUT
//
// ★ NPC 不需要「对话状态」在 Go 侧维护。
//
//	原版把对话状态放在 Lua 侧的 _RSVD_NAME_EPUID_eventHandlers 里
//	（按 uid 索引的 quest handler），Go 只做「把点击事件转给脚本」。
//	所以这里也没有任何对话树/状态机代码 —— 全部在脚本里。
//
// ★ 每个 NPC 一个 Lua state，不是每个玩家一个：
//
//	同一个 NPC 的所有玩家共享它的 setEventHandler 表，
//	玩家之间的隔离靠 getTLSTable()（按协程索引）。
//	这是原版的设计，也意味着 NPC 脚本不能把玩家状态存全局变量。

// NPCE 错误码（protocoldef.hpp）。
const (
	NPCEStallSelling = 1
	NPCETooFar       = 2
)

// NPCManager 管理地图上所有 NPC 的 Lua 实例。
type NPCManager struct {
	mu   sync.RWMutex
	npcs map[uint64]*npcInstance // npcUID -> 实例
	// byMap mapID -> []uid，按地图索引便于进场时下发
	byMap map[uint32][]uint64

	FrameworkDir string
	ScriptRoot   string
	Items        *ItemDB
	Magics       *MagicDB
	Logger       *slog.Logger
}

// npcInstance 是一个 NPC 的运行时。
type npcInstance struct {
	Def    script.NPCDef
	NPCUID uint64
	MapID  uint32
	Script *script.NPCScript
	Host   *npcHostAdapter
}

// NewNPCManager 创建管理器。
func NewNPCManager(items *ItemDB, magics *MagicDB, logger *slog.Logger) *NPCManager {
	return &NPCManager{
		npcs:         make(map[uint64]*npcInstance),
		byMap:        make(map[uint32][]uint64),
		FrameworkDir: script.ResolveFrameworkDir(),
		ScriptRoot:   script.ResolveScriptRoot(),
		Items:        items,
		Magics:       magics,
		Logger:       logger,
	}
}

// LoadMap 为一整张地图加载全部 NPC 脚本。
//
// 对应 servermap.cpp:1701 loadNPChar。
//
// 返回成功加载的 NPC 列表（用于生成地图上的实体）。
func (m *NPCManager) LoadMap(ctx context.Context, mapID uint32, mapName string,
	hostFactory func(uid uint64) *npcHostAdapter) ([]npcInstance, error) {

	byMap, err := script.ScanNPCDir(m.ScriptRoot + "/npc")
	if err != nil {
		return nil, fmt.Errorf("scan npc dir: %w", err)
	}
	defs := byMap[mapName]
	if len(defs) == 0 {
		return nil, nil
	}

	// 记录表（物品/怪物/地图的名字<->ID，脚本靠它查东西）
	itemTbl := m.itemsNameTable()
	monsterTbl := m.monsterNameTable()
	mapTbl := m.mapNameTable()

	var logMu sync.Mutex
	commonBindings := script.CommonBindings(itemTbl, monsterTbl, mapTbl,
		func(level int, msg string) {
			logMu.Lock()
			defer logMu.Unlock()
			if m.Logger != nil {
				m.Logger.Debug("npc script", "map", mapName, "msg", msg)
			}
		})

	var loaded []npcInstance
	for _, def := range defs {
		// 空脚本是原版占位，跳过（没有实体也没意义）
		if isZeroSizeFile(def.ScriptPath) {
			continue
		}

		npcUID := buildNPCUID(def.LookID, def.X, def.Y)
		host := hostFactory(npcUID)

		inst, err := script.LoadNPC(script.Options{
			FrameworkDir: m.FrameworkDir,
			ScriptRoot:   m.ScriptRoot,
			Consts:       script.DefaultConsts(),
			Bindings:     commonBindings,
		}, script.NPCInfo{
			UID: npcUID, Name: def.Name, MapName: def.MapName,
			ScriptPath: def.ScriptPath, MapID: mapID,
			X: def.X, Y: def.Y, LookID: def.LookID, GfxDir: uint8(def.GfxDir),
		}, host)
		if err != nil {
			if m.Logger != nil {
				m.Logger.Warn("load npc failed", "npc", def.Name, "error", err)
			}
			continue
		}
		host.setScript(inst)

		m.mu.Lock()
		m.npcs[npcUID] = &npcInstance{
			Def: def, NPCUID: npcUID, MapID: mapID, Script: inst, Host: host,
		}
		m.byMap[mapID] = append(m.byMap[mapID], npcUID)
		m.mu.Unlock()

		loaded = append(loaded, npcInstance{Def: def, NPCUID: npcUID, MapID: mapID, Script: inst, Host: host})
	}

	if m.Logger != nil {
		m.Logger.Info("npc loaded", "map", mapName, "count", len(loaded), "skippedEmpty", len(defs)-len(loaded))
	}
	return loaded, nil
}

// Get 按 UID 取 NPC。
func (m *NPCManager) Get(npcUID uint64) (*npcInstance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.npcs[npcUID]
	return inst, ok
}

// Count 返回已加载的 NPC 数量。
func (m *NPCManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.npcs)
}

// Close 释放所有 Lua 状态。
func (m *NPCManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inst := range m.npcs {
		if inst.Script != nil {
			inst.Script.Close()
		}
	}
	m.npcs = make(map[uint64]*npcInstance)
	m.byMap = make(map[uint32][]uint64)
}

// buildNPCUID 生成 NPC 的 UID。
//
// 原版 buildNPCUID(lookID, peerIndex) 的 peerIndex 是**进程/分片编号**
// （4 bit，0..15），不是坐标。同一张图里 lookID 会重复
// （4 个六面神石都是 LOOK_56），所以必须用 seq 区分。
//
// ★ 初版把 x/y 当 peerIndex 传进去，396 > 15 直接返回 error，
//
//	于是全部走了兜底分支，日志里显示成 uid=ERR_xxx，
//	而且所有 NPC 的 UID 撞在一起（AOI 里互相覆盖）。
//
// 正确做法：id=lookID，peerIndex=0，seq 用坐标算出的唯一序号。
// 坐标 (x,y) 在 600x600 地图里 → seq = x*1024+y+1，不会重复。
func buildNPCUID(lookID uint32, x, y int) uint64 {
	if lookID == 0 {
		lookID = 1 // id 不能为 0
	}
	seq := uint32(x)*1024 + uint32(y) + 1
	uid, err := common.BuildNPCUID(lookID, 0, seq)
	if err == nil {
		return uid
	}
	// 兜底：手工构造类型前缀 + 唯一序号
	return uint64(4)<<59 | uint64(lookID&0xFFFFFF)<<35 | uint64(seq&0x7FFFFFFF)<<4 | 1
}

// itemsNameTable 从物品表构造名字表。
func (m *NPCManager) itemsNameTable() *script.NameTable {
	if m.Items == nil {
		return script.NewNameTable(nil)
	}
	records := make(map[uint32]string, m.Items.Count())
	for id := uint32(1); id <= uint32(m.Items.Count())+1; id++ {
		if rec, ok := m.Items.Get(id); ok {
			records[id] = rec.Name
		}
	}
	return script.NewNameTable(records)
}

func (m *NPCManager) monsterNameTable() *script.NameTable {
	// 怪物表当前没在 game 层加载（怪物数据在 world.MonsterRecord）
	// 先返回空表：getMonsterName 返回 nil，脚本里的 hasChar(nil) 为 false，
	// 「怪物召唤商人」这类 NPC 会走 else 分支不会崩。
	return script.NewNameTable(nil)
}

func (m *NPCManager) mapNameTable() *script.NameTable {
	return script.NewNameTable(nil)
}

// ==================== 宿主适配（script.NPCHost 的实现）====================

// npcHostAdapter 把 game 层能力暴露给 Lua。
type npcHostAdapter struct {
	NPCUID uint64
	MapID  uint32

	Script    *script.NPCScript
	Players   *PlayerManager
	Worlds    *world.World
	Broadcast Broadcaster
	Items     *ItemDB
	Logger    *slog.Logger
	// Durability 修理（NPC 修理要恢复持久并扣钱）
	Durability *DurabilityHandler
	// Secured 仓库（NPC 寄存/取回）
	Secured *SecuredHandler

	mu sync.Mutex
}

func (h *npcHostAdapter) setScript(s *script.NPCScript) {
	h.mu.Lock()
	h.Script = s
	h.mu.Unlock()
}

// PostXML 发 SM_NPCXMLLAYOUT。
//
// ★ 这是 NPC 对话的唯一出口 —— 脚本里所有 dialog.post 最终都到这里。
//
//	XML 里的事件 ID 原版会用 AES 加密（防改包点未授权选项），
//	客户端只是原样回传，所以加密与否不影响链路；
//	当前版本先不加密（记在 REPLICA_AUDIT 待办）。
func (h *npcHostAdapter) PostXML(playerUID uint64, eventPath, xml string) error {
	p, ok := h.Players.Get(playerUID)
	if !ok || p.ChannelID == 0 {
		return fmt.Errorf("player %s not online", common.GetUIDString(playerUID))
	}
	payload, err := protocol.CerealSerialize(protocol.SDNPCXMLLayout{
		NPCUID:    h.NPCUID,
		EventPath: eventPath,
		XMLLayout: xml,
	})
	if err != nil {
		return err
	}
	if h.Broadcast == nil {
		return nil
	}
	return h.Broadcast.SendTo(p.ChannelID, protocol.SM_NPCXMLLAYOUT, payload, 0)
}

// PostInvOpCost 发 SM_INVOPCOST（背包操作费用）。
func (h *npcHostAdapter) PostInvOpCost(playerUID uint64, invOp int, itemID, seqID uint32, cost uint32) error {
	p, ok := h.Players.Get(playerUID)
	if !ok || p.ChannelID == 0 || h.Broadcast == nil {
		return nil
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMInvOpCost{
		InvOp: uint32(invOp), ItemID: itemID, SeqID: seqID, Cost: cost,
	})
	if err != nil {
		return err
	}
	return h.Broadcast.SendTo(p.ChannelID, protocol.SM_INVOPCOST, payload, 0)
}

// PostStartInvOp 发 SM_STARTINVOP。
func (h *npcHostAdapter) PostStartInvOp(playerUID uint64, invOp int, queryTag, commitTag string, typeList []string) error {
	p, ok := h.Players.Get(playerUID)
	if !ok || p.ChannelID == 0 || h.Broadcast == nil {
		return nil
	}
	payload, err := protocol.CerealSerialize(protocol.SDStartInvOp{
		InvOp:     int32(invOp),
		UID:       h.NPCUID,
		QueryTag:  queryTag,
		CommitTag: commitTag,
		TypeList:  typeList,
	})
	if err != nil {
		return err
	}
	return h.Broadcast.SendTo(p.ChannelID, protocol.SM_STARTINVOP, payload, 0)
}

// PostStartInput 发 SM_STARTINPUT（弹输入框）。
func (h *npcHostAdapter) PostStartInput(playerUID uint64, title, commitTag string, security bool) error {
	p, ok := h.Players.Get(playerUID)
	if !ok || p.ChannelID == 0 || h.Broadcast == nil {
		return nil
	}
	payload, err := protocol.CerealSerialize(protocol.SDStartInput{
		UID:       h.NPCUID,
		Title:     title,
		CommitTag: commitTag,
		Security:  security,
	})
	if err != nil {
		return err
	}
	return h.Broadcast.SendTo(p.ChannelID, protocol.SM_STARTINPUT, payload, 0)
}

// PostSell 发 SM_SELLITEMLIST。
func (h *npcHostAdapter) PostSell(playerUID uint64, itemIDs []uint32, costList []script.InvOpCost) error {
	p, ok := h.Players.Get(playerUID)
	if !ok || p.ChannelID == 0 || h.Broadcast == nil || h.Items == nil {
		return nil
	}
	list := protocol.SDSellItemList{NPCUID: h.NPCUID}
	for _, id := range itemIDs {
		if _, exists := h.Items.Get(id); !exists {
			continue
		}
		list.List = append(list.List, protocol.SDSellItem{
			Item:     protocol.SDItem{ItemID: id, Count: 1},
			CostList: []protocol.SDCostItem{{ItemID: GoldItemID, Count: 1}},
		})
	}
	payload, err := protocol.CerealSerialize(list)
	if err != nil {
		return err
	}
	return h.Broadcast.SendTo(p.ChannelID, protocol.SM_SELLITEMLIST, payload, 0)
}

func (h *npcHostAdapter) QueryName(playerUID uint64) (string, bool) {
	p, ok := h.Players.Get(playerUID)
	if !ok {
		return "", false
	}
	return p.Name, true
}

func (h *npcHostAdapter) QueryLevel(playerUID uint64) (int, bool) {
	p, ok := h.Players.Get(playerUID)
	if !ok {
		return 0, false
	}
	return p.Level, true
}

func (h *npcHostAdapter) QueryGold(playerUID uint64) (uint64, bool) {
	p, ok := h.Players.Get(playerUID)
	if !ok {
		return 0, false
	}
	return p.Gold, true
}

func (h *npcHostAdapter) QueryRedName(playerUID uint64) bool {
	p, ok := h.Players.Get(playerUID)
	if !ok {
		return false
	}
	// PK 值 ≥ 100 视为红名（原版 PKPOINT_REDNAME）
	return p.PKPoint >= 100
}

func (h *npcHostAdapter) QueryItemDuration(playerUID uint64, itemID, seqID uint32) (int, int, bool) {
	p, ok := h.Players.Get(playerUID)
	if !ok {
		return 0, 0, false
	}
	_, it := p.findItem(itemID, uint32(seqID))
	if it == nil {
		return 0, 0, false
	}
	// 没有持久概念的物品（药品/材料）返回 false，
	// 原版 queryItemDuration 对这类物品返回 nil。
	if !it.HasDurability() {
		return 0, 0, false
	}
	return int(it.Durability), int(it.MaxDurability), true
}

// RepairItem 修理背包物品：扣钱 + 恢复持久。
//
// ★ 顺序是「先修后扣钱」还是「先扣钱后修」？
//
//	原版是先判断能否修（不打钱），再 repairInventoryItem，再调脚本扣钱。
//	这里统一成：修成功才扣钱。反过来的话，扣了钱但修失败（比如
//	物品在扣钱到修理之间被交易走）会让玩家白花钱。
func (h *npcHostAdapter) RepairItem(playerUID uint64, itemID, seqID uint32, special bool, cost uint64) bool {
	p, ok := h.Players.Get(playerUID)
	if !ok || h.Durability == nil {
		return false
	}
	if cost > 0 && p.Gold < cost {
		return false // 钱不够
	}
	if _, repaired := h.Durability.RepairItem(p, itemID, uint64(seqID), special); !repaired {
		return false
	}
	if cost > 0 {
		p.Gold -= cost
		h.Durability.notifyGold(p)
	}
	return true
}

// SecureItem 把背包物品寄存到仓库。
func (h *npcHostAdapter) SecureItem(playerUID uint64, itemID, seqID uint32) bool {
	if h.Secured == nil {
		return false
	}
	p, ok := h.Players.Get(playerUID)
	if !ok {
		return false
	}
	return h.Secured.Secure(context.Background(), p, itemID, seqID)
}

// PostSecuredItemList 推开仓库物品列表。
func (h *npcHostAdapter) PostSecuredItemList(playerUID uint64) error {
	if h.Secured == nil {
		return nil
	}
	p, ok := h.Players.Get(playerUID)
	if !ok {
		return nil
	}
	return h.Secured.ShowList(context.Background(), p)
}

func (h *npcHostAdapter) RemoveGold(playerUID uint64, amount uint64) bool {
	p, ok := h.Players.Get(playerUID)
	if !ok || p.Gold < amount {
		return false
	}
	p.Gold -= amount
	h.sendGold(p)
	return true
}

func (h *npcHostAdapter) GrantGold(playerUID uint64, amount uint64) bool {
	p, ok := h.Players.Get(playerUID)
	if !ok {
		return false
	}
	p.Gold += amount
	if p.Gold > MaxGold {
		p.Gold = MaxGold
	}
	h.sendGold(p)
	return true
}

func (h *npcHostAdapter) sendGold(p *Player) {
	if h.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMGold{Gold: p.Gold})
	if err != nil {
		return
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_GOLD, payload, 0)
}

func (h *npcHostAdapter) SpaceMove(playerUID uint64, mapID uint32, x, y int) error {
	if h.Worlds == nil {
		return fmt.Errorf("world not ready")
	}
	_, _, err := h.Worlds.Teleport(context.Background(), mapID, playerUID, x, y)
	return err
}

func (h *npcHostAdapter) Log(level int, msg string) {
	if h.Logger == nil {
		return
	}
	switch level {
	case 0:
		h.Logger.Info("npc", "uid", common.GetUIDString(h.NPCUID), "msg", msg)
	case 1:
		h.Logger.Warn("npc", "uid", common.GetUIDString(h.NPCUID), "msg", msg)
	default:
		h.Logger.Error("npc", "uid", common.GetUIDString(h.NPCUID), "msg", msg)
	}
}

// isZeroSizeFile 判断脚本是否为空占位文件。
//
// 原版 341 个 NPC 脚本里有 179 个是 0 字节的占位（地图上摆了 NPC
// 但没写对话，比如「士官_1」「小老板_1」）。这些不该算加载失败。
func isZeroSizeFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return info.Size() == 0
}
