package game

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
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
	// Monsters 怪物表（getMonsterName / getMonsterID 要用）
	//
	// nil 时退化成空表（脚本里的名字判断全部为 false，但不崩）。
	Monsters *world.MonsterDB
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
		npcUID := buildNPCUID(mapID, def.LookID, def.X, def.Y)

		// ★ 空脚本也要建 NPC 实体。
		//
		//	原版 npc/ 目录里有 179 个 0 字节脚本（占位）。
		//	它们是「在地图上站着但没有任何对话」的 NPC。
		//
		//	最初的做法是 `if isZeroSizeFile { continue }` —— 直接跳过。
		//	后果：
		//	  1. 玩家看不到这些 NPC（本该站着的人消失了）
		//	  2. 任务要 getNPCharUID('边境城市_01','上官小姐_1') 时查不到
		//	     → 该任务整个加载失败
		//
		//	实测：边境城市_01 有 21 个 NPC，跳过 10 个空脚本后只剩 11 个，
		//	「战士初出江湖」等多个任务因此挂不上。
		//
		//	正确做法：照样建实例，只是不加载 Lua 脚本（Script 为 nil）。
		if isZeroSizeFile(def.ScriptPath) {
			m.mu.Lock()
			if _, exists := m.npcs[npcUID]; !exists {
				m.npcs[npcUID] = &npcInstance{
					Def: def, NPCUID: npcUID, MapID: mapID,
				}
				m.byMap[mapID] = append(m.byMap[mapID], npcUID)
			}
			m.mu.Unlock()
			continue
		}
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
				m.Logger.Warn("load npc failed", "map", mapName, "npc", def.Name, "error", err)
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
// FindByMapAndName 按地图名 + NPC 名找 UID（任务系统的 getNPCharUID 用）。
//
// ★ 遍历而不是建索引：NPC 总数 340 个、只在任务加载/注册时查，
//	建二级索引的复杂度不值得。真成瓶颈再加。
func (m *NPCManager) FindByMapAndName(mapName, npcName string) (uint64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for uid, inst := range m.npcs {
		if inst.Def.MapName == mapName && inst.Def.Name == npcName {
			return uid, true
		}
	}
	return 0, false
}

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
// ★★ 必须带 mapID ★★
//
//	最初的实现是 buildNPCUID(lookID, x, y) —— 只用外观+坐标，
//	**不含地图**。后果：不同地图上相同坐标、相同外观的 NPC
//	会算出完全相同的 UID，后加载的直接覆盖先加载的。
//
//	实测：340 个 NPC 只存活 91 个，另外 249 个被静默覆盖。
//	表现是「某些地图的 NPC 点不动 / 任务挂不上」，
//	而且日志里看不到任何错误（map[npcUID] = ... 只是覆盖）。
//
//	修复：把 mapID 揉进 id 段，保证全局唯一。
func buildNPCUID(mapID uint32, lookID uint32, x, y int) uint64 {
	if lookID == 0 {
		lookID = 1 // id 不能为 0
	}
	// ★ seq 用 mapID 打散：
	//	seq 是 31 位，x/y 各占 10 位（0..1023），
	//	剩下的高位放 mapID，避免跨图冲突。
	seq := (mapID&0x7FF)<<20 | (uint32(x)&0x3FF)<<10 | (uint32(y) & 0x3FF)
	if seq == 0 {
		seq = 1
	}
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
	// ★ 从怪物表反查名字（之前返回空表，是个真实缺陷）
	//
	//	空表的后果：脚本里 getMonsterName(id) 恒为 nil，
	//	`if getMonsterName(id) == '半兽人'` 这类判断全部走 else 分支 ——
	//	任务/召唤类 NPC 静默不工作，且**没有任何报错**。
	//
	//	现在从 world.MonsterDB 反查（它在启动时已加载 364 条）。
	if m.Monsters == nil {
		return script.NewNameTable(nil)
	}
	return script.NewNameTable(m.Monsters.NameTable())
}

// mapNameTable 返回地图名 <-> ID 表。
//
// ★ 之前返回空表，是个真实缺陷：
//
//	quest.lua:127 getNPCharUID 第一句就是 `loadBaseMap(mapName)`，
//	空表让所有「按地图名找 NPC」的调用返回 nil →
//	任务无法把状态机挂到 NPC 上 → 42 个任务里有 2 个直接加载失败，
//	其余任务在运行期也找不到 NPC。
//
//	数据源是 world 包的全局表（启动时 SetMapNames 注入，603 条）。
func (m *NPCManager) mapNameTable() *script.NameTable {
	// world 包的全局表是 1..N 连续 ID，反查一遍
	records := make(map[uint32]string, 700)
	for id := uint32(1); id < 2000; id++ {
		if name := world.MapNameOf(id); name != "" {
			records[id] = name
		}
	}
	return script.NewNameTable(records)
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

	// ---- 以下供 uidRemoteCall 使用（P2 公会/任务路径）----
	//
	// ★ 这些不是「可选的美化」：公会战唯一的玩家入口是
	//   server.player.declareGuildWar → uidRemoteCall → 这里。
	//   缺了 GuildWars，宣战永远返回 GUILDERR_NOGUILD。
	Guilds    *storage.GuildRepository
	GuildWars *GuildWarManager
	Magics    *MagicDB
	Shops     *ShopHandler
	Golds     GoldRepository

	mu sync.Mutex
}

// RemoteCall 在玩家范围执行一段 Lua 代码（uidRemoteCall 的核心）。
//
// 详见 remote_call.go 的实现说明。
func (h *npcHostAdapter) RemoteCall(targetUID uint64, code string,
	args []script.RemoteArg) ([]script.RemoteArg, error) {
	return h.executeRemoteCall(targetUID, code, args)
}

// pushInventory 把整包背包推给客户端（SM_INVENTORY）。
//
// ★ 用 SM_INVENTORY（整包）而不是 SM_UPDATEITEM（增量）：
//
//	remoteCall 里的 addItem/removeItem 是脚本驱动的任意改动
//	（可能一次加多件、也可能合并到已有格子），
//	自己算增量容易漏。整包推几十件物品也就几百字节，
//	在「玩家点 NPC 对话」这个低频场景完全可接受。
func (h *npcHostAdapter) pushInventory(p *Player) {
	if h.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	list := make([]protocol.SDItem, 0, len(p.Inventory))
	for _, it := range p.Inventory {
		list = append(list, toSDItem(it))
	}
	if payload, err := protocol.AppendWirePayload(nil, list); err == nil {
		_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_INVENTORY, payload, 0)
	}
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

// FindByMapIDAndName 按地图 ID + NPC 名找 UID（地图 actor 的回答）。
func (m *NPCManager) FindByMapIDAndName(mapID uint32, npcName string) (uint64, bool) {
	if m.Logger != nil {
		m.Logger.Debug("find npc", "mapID", mapID, "name", npcName, "total", len(m.npcs))
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for uid, inst := range m.npcs {
		if inst.MapID == mapID && inst.Def.Name == npcName {
			return uid, true
		}
	}
	return 0, false
}

// MapsWithNPCs 返回有 NPC 的地图 ID 列表。
func (m *NPCManager) MapsWithNPCs() []uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := make(map[uint32]bool, len(m.byMap))
	out := make([]uint32, 0, len(m.byMap))
	for mapID := range m.byMap {
		if !seen[mapID] {
			seen[mapID] = true
			out = append(out, mapID)
		}
	}
	return out
}

// ForEach 遍历全部 NPC。
func (m *NPCManager) ForEach(fn func(uid uint64, npc *npcInstance)) {
	m.mu.RLock()
	snapshot := make(map[uint64]*npcInstance, len(m.npcs))
	for k, v := range m.npcs {
		snapshot[k] = v
	}
	m.mu.RUnlock()
	for uid, npc := range snapshot {
		fn(uid, npc)
	}
}


// AllMapIDs 返回全部已知地图 ID（来自地图名表）。
//
// ★ 为什么需要「全部」而不只是「已加载」：
//
//	任务脚本会在启动时 getNPCharUID('道馆_1', ...)，
//	不管玩家是否去过道馆_1。所以每一张图的 actor 都得能应答。
func (m *NPCManager) AllMapIDs() []uint32 {
	ids := make([]uint32, 0, 700)
	for id := uint32(1); id < 2000; id++ {
		if world.MapNameOf(id) != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// loadMapDefs 按已解析的 NPC 定义加载脚本（预加载用，不生成世界实体）。
//
// ★ 与 LoadMap 的差别：不需要 hostFactory，用一个空 host 即可。
//
//	预加载的目的是「让任务能找到 NPC 并挂 handler」，
//	此时玩家还没进图，NPC 的所有 uidXXX 回调都不会被调用。
//	等玩家真的进图时，LoadMap 会重新走一遍并换成真正的 host。
func (m *NPCManager) loadMapDefs(ctx context.Context, mapID uint32, mapName string,
	defs []script.NPCDef) (loaded int, failed int) {

	itemTbl := m.itemsNameTable()
	monsterTbl := m.monsterNameTable()
	mapTbl := m.mapNameTable()

	commonBindings := script.CommonBindings(itemTbl, monsterTbl, mapTbl,
		func(level int, msg string) {})

	for _, def := range defs {
		npcUID := buildNPCUID(mapID, def.LookID, def.X, def.Y)

		// 空脚本：只登记实体，不加载 Lua
		if isZeroSizeFile(def.ScriptPath) {
			m.mu.Lock()
			if _, exists := m.npcs[npcUID]; !exists {
				m.npcs[npcUID] = &npcInstance{Def: def, NPCUID: npcUID, MapID: mapID}
				m.byMap[mapID] = append(m.byMap[mapID], npcUID)
			}
			m.mu.Unlock()
			loaded++
			continue
		}

		// 已加载过就跳过（玩家进图时会再 LoadMap 一次）
		m.mu.RLock()
		_, exists := m.npcs[npcUID]
		m.mu.RUnlock()
		if exists {
			continue
		}

		inst, err := script.LoadNPC(script.Options{
			FrameworkDir: m.FrameworkDir,
			ScriptRoot:   m.ScriptRoot,
			Consts:       script.DefaultConsts(),
			Bindings:     commonBindings,
		}, script.NPCInfo{
			UID: npcUID, Name: def.Name, MapName: def.MapName,
			ScriptPath: def.ScriptPath, MapID: mapID,
			X: def.X, Y: def.Y, LookID: def.LookID, GfxDir: uint8(def.GfxDir),
		}, nil)
		if err != nil {
			// ★ 不能静默 continue：预加载失败会让任务挂不上 NPC，
			//	而且「loaded=339 但内存里只有 91」这种矛盾极难排查。
			failed++
			if m.Logger != nil && failed <= 3 {
				m.Logger.Warn("preload npc failed", "map", mapName, "npc", def.Name, "error", err)
			}
			continue
		}

		m.mu.Lock()
		m.npcs[npcUID] = &npcInstance{
			Def: def, NPCUID: npcUID, MapID: mapID, Script: inst,
		}
		m.byMap[mapID] = append(m.byMap[mapID], npcUID)
		m.mu.Unlock()
		loaded++
	}
	return loaded, failed
}

// PreloadAll 预加载全部地图的 NPC 脚本（不生成实体）。
//
// ★★ 这是让任务系统能在启动时工作的前提 ★★
//
//	任务脚本顶层会 getNPCharUID(mapName, npcName)，
//	而 getNPCharUID 是通过地图 actor 查 NPC 的。
//	如果 NPC 是玩家进图才加载，启动时任务一个都挂不上。
//
//	原版启动时所有地图 activate、所有 NPC 建好，就是这个道理。
//
//	代价：339 个 NPC × 一个 Lua 状态 ≈ 每个约 100KB，
//	总共约 34MB。可以接受（原版也是全量加载）。
func (m *NPCManager) PreloadAll(ctx context.Context) (loaded int, failed int) {
	byMap, err := script.ScanNPCDir(filepath.Join(m.ScriptRoot, "npc"))
	if err != nil {
		if m.Logger != nil {
			m.Logger.Warn("preload npc scan failed", "error", err)
		}
		return 0, 0
	}

	mapNameToID := make(map[string]uint32, 700)
	for id := uint32(1); id < 2000; id++ {
		if name := world.MapNameOf(id); name != "" {
			mapNameToID[name] = id
		}
	}

	for mapName, defs := range byMap {
		mapID, ok := mapNameToID[mapName]
		if !ok {
			continue
		}
		l, f := m.loadMapDefs(ctx, mapID, mapName, defs)
		loaded += l
		failed += f
	}
	return loaded, failed
}
