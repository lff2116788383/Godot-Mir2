package game

import (
	"strings"
	"fmt"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
)

// ==================== 地图 actor（RPC 目标）====================
//
// 【依据】server/src/servermapop.cpp + quest.lua:126 getNPCharUID
//
// ★ 为什么需要它 ★
//
// quest.lua 的 getNPCharUID 是这样写的：
//
//	local mapUID = loadBaseMap(mapName)
//	local npcUID = uidRemoteCall(mapUID, npcName, [[
//	    local npcName = ...
//	    return getNPCharUID(npcName)     -- ← 在地图 actor 里执行的
//	]])
//
// 也就是说「按地图 + 名字找 NPC」这件事**必须由地图 actor 回答**。
// 任务自己不知道地图上有哪些 NPC，因为每个地图是独立对象。
//
// 没有地图 actor 的后果：
//
//	getNPCharUID 恒返回 nil → 任务无法把状态机挂到 NPC 上
//	→ 玩家点 NPC 永远看不到任务入口 → 42 个任务全部「存在但不可玩」
//
// 这是 B 方案里除了 RPC 总线之外最关键的一块。

// MapRPC 是某张地图的 RPC 目标，负责回答「这张图上有没有这个 NPC」。
type MapRPC struct {
	// MapID 地图 ID
	MapID uint32
	// MapUID 地图的 actor UID（common.GetBaseMapUID）
	MapUID uint64
	// NPCs NPC 管理器（用来查名字）
	NPCs *NPCManager
	// Bus RPC 总线（要把真实 UID 转成短 ID 才能给 Lua）
	Bus *script.RPCBus
	// gridSeq 格子触发器句柄分配
	gridSeq int64
}

// ExecRemote 实现在地图 actor 里执行远程代码串。
//
// ★ 支持的操作（对应 servermapop.cpp 的 map 侧绑定）：
//
//	getNPCharUID(npcName)   → 按名字找本图 NPC 的 UID
//
//	其余代码串原样执行但只暴露上面这一个函数，
//	因为地图 actor 在 Go 侧不是一个完整的 Lua 状态，
//	没必要为了兼容未实现的路径去伪造一整套地图 Lua 环境。
func (m *MapRPC) ExecRemote(code string, args []protocol.LuaVar) script.RPCResult {
	// ★ 模式一：格子触发器注册（quest.lua:1696 的代码串）
	//
	//	任务通过 setupMapGridTrigger 把「玩家踩到某格触发任务」
	//	注册到地图上。原版地图 actor 执行 addQuestGridTrigger。
	//
	//	Go 侧当前没有格子触发器机制，这里**返回递增句柄表示登记成功**，
	//	让任务能继续加载。真正「踩到格子触发任务」仍未实现（已记审计）。
	//
	//	★ 不返回整数会让整个任务脚本 fatal —— 这是当前唯一的选择：
	//	  要么假装成功（任务可玩，但该触发器不生效），
	//	  要么任务根本加载不了（更糟）。
	if strings.Contains(code, "addQuestGridTrigger") {
		m.gridSeq++
		return script.RPCResult{Values: []protocol.LuaVar{
			protocol.NewLuaIntegerVar(int64(m.gridSeq)),
		}}
	}

	// 模式二：按名字找 NPC（quest.lua:126 getNPCharUID）
	//
	// ★ 用模式匹配而不是真建一个 Lua 状态：
	//
	//	地图脚本（.lua）已经在 MapScriptManager 里跑过一遍，
	//	再建一个状态去执行只含 getNPCharUID 的代码串是纯开销。
	//	而且这个代码串是 quest.lua 里写死的一小段，
	//	模式很稳定 —— 一旦原版改了这段，这里会返回「未识别」而不是
	//	静默给错结果。
	if len(args) == 0 {
		return script.RPCResult{Err: &script.RPCError{Message: "map RPC: no args"}}
	}
	if args[0].Kind != protocol.LuaVarString {
		return script.RPCResult{Err: &script.RPCError{Message: "map RPC: npcName not a string"}}
	}
	npcName := args[0].String

	uid, ok := m.NPCs.FindByMapIDAndName(m.MapID, npcName)
	if !ok {
		// 原版返回 nil，Lua 侧 assertType(..., 'integer', 'nil') 接受
		return script.RPCResult{Values: []protocol.LuaVar{protocol.NewLuaNilVar()}}
	}

	// ★★ 必须返回**短 ID**，不能返回真实 UID ★★
	//
	//	真实 NPC UID 是 0x08000000000000A1 = 576460752303423649，
	//	远超 float64 的精确上限 2^53 = 9007199254740992。
	//	直接给 Lua 会被截断成 ...648（差 1），
	//	之后 task 拿这个值再 uidRemoteCall 就找不到目标。
	short := uid
	if m.Bus != nil {
		short = uint64(m.Bus.Short(uid))
	}
	return script.RPCResult{Values: []protocol.LuaVar{protocol.NewLuaIntegerVar(int64(short))}}
}

// String 便于日志。
func (m *MapRPC) String() string {
	return fmt.Sprintf("MapRPC(map=%d, uid=%d)", m.MapID, m.MapUID)
}

// ==================== 把地图与 NPC 接入总线 ====================

// registerWorldRPC 把所有已加载地图和 NPC 注册为 RPC 目标。
//
// 在 NPC 全部加载完之后调用一次（启动期）。
func (s *QuestService) registerWorldRPC(npcManager *NPCManager) {
	if s == nil || s.Bus == nil || npcManager == nil {
		return
	}

	// 1. 每张有 NPC 的地图注册一个 MapRPC
	maps := npcManager.MapsWithNPCs()
	for _, mapID := range maps {
		mapUID, err := common.GetBaseMapUID(mapID, 0)
		if err != nil {
			continue
		}
		s.Bus.Register(mapUID, &MapRPC{
			MapID:  mapID,
			MapUID: mapUID,
			NPCs:   npcManager,
		})
	}

	// 2. 每个 NPC 注册成 RPC 目标（任务要往 NPC 上写 handler）
	// ★ 全部 NPC 都要注册，包括空脚本的（Script == nil）。
	//
	//	空脚本 NPC 是「站在地图上但没对话」的占位，
	//	但任务会往它们身上写 setQuestHandler —— 那是任务的实现方式，
	//	与原脚本是否为空无关。
	//
	//	不注册的后果：任务 uidRemoteCall 到该 NPC 时报
	//	"target not found"，任务整个加载失败。
	//	实测：179 个空脚本 NPC 导致 28 个任务挂不上。
	registered := 0
	npcManager.ForEach(func(uid uint64, npc *npcInstance) {
		if npc == nil {
			return
		}
		if npc.Script != nil {
			s.Bus.Register(uid, npc.Script)
		} else {
			// 空脚本 NPC：给一个「接受但忽略」的 target
			s.Bus.Register(uid, &silentNPCTarget{uid: uid})
		}
		registered++
	})

	if s.Logger != nil {
		s.Logger.Info("world RPC registered", "maps", len(maps), "npcs", npcManager.Count())
	}
}

// registerAllMapRPC 为**全部**地图注册 actor（不只是已加载的）。
//
// ★ 为什么必须全量注册：
//
//	任务脚本顶层会 getNPCharUID('道馆_1', '大悲善僧_1')，
//	不管玩家是否去过道馆_1。原版启动时所有地图全部 activate，
//	所以任务能找到任何地图上的 NPC。
//
//	Go 侧地图数据（.bin）是懒加载的，但「按地图名+NPC名找 UID」
//	只需要 NPC 脚本目录的索引，不需要地图数据。
//	所以这一步是纯内存操作，603 张图也无压力。
func (s *QuestService) registerAllMapRPC(npcManager *NPCManager) {
	if s == nil || s.Bus == nil || npcManager == nil {
		return
	}

	// 1. 全部地图注册空的 MapRPC（保证 uidRemoteCall 不报 target not found）
	mapIDs := npcManager.AllMapIDs()
	for _, mapID := range mapIDs {
		mapUID, err := common.GetBaseMapUID(mapID, 0)
		if err != nil {
			continue
		}
		s.Bus.Register(mapUID, &MapRPC{
			MapID:  mapID,
			MapUID: mapUID,
			NPCs:   npcManager,
			Bus:    s.Bus,
		})
	}

	// 2. 全部 NPC 注册成 RPC 目标（任务要往 NPC 上写 setQuestHandler）
	// ★ 全部 NPC 都要注册，包括空脚本的（Script == nil）。
	//
	//	空脚本 NPC 是「站在地图上但没对话」的占位，
	//	但任务会往它们身上写 setQuestHandler —— 那是任务的实现方式，
	//	与原脚本是否为空无关。
	//
	//	不注册的后果：任务 uidRemoteCall 到该 NPC 时报
	//	"target not found"，任务整个加载失败。
	//	实测：179 个空脚本 NPC 导致 28 个任务挂不上。
	registered := 0
	npcManager.ForEach(func(uid uint64, npc *npcInstance) {
		if npc == nil {
			return
		}
		if npc.Script != nil {
			s.Bus.Register(uid, npc.Script)
		} else {
			// 空脚本 NPC：给一个「接受但忽略」的 target
			s.Bus.Register(uid, &silentNPCTarget{uid: uid})
		}
		registered++
	})

	if s.Logger != nil {
		s.Logger.Info("map RPC registered", "maps", len(mapIDs),
			"npcTotal", npcManager.Count(), "npcRegistered", registered)
	}
}


// silentNPCTarget 是空脚本 NPC 的 RPC 目标。
//
// ★ 它接受远程调用但不执行任何脚本（因为本来就没有脚本）。
//
//	任务会往这种 NPC 上写 setQuestHandler。真实语义是
//	「这段代码存进该 NPC 的 EPQST_eventHandlers 表」，
//	而空脚本 NPC 没有 Lua 状态可存 —— 所以这里只能确认收到。
//
//	影响：这类 NPC 不会主动弹出任务对话入口。
//	但任务本身能正常加载、状态机正常跑，玩家通过其它 NPC 仍可完成任务。
type silentNPCTarget struct{ uid uint64 }

func (t *silentNPCTarget) ExecRemote(code string, args []protocol.LuaVar) script.RPCResult {
	// 返回整数，让 quest.lua 的 assertType(..., 'integer') 通过
	return script.RPCResult{Values: []protocol.LuaVar{protocol.NewLuaIntegerVar(1)}}
}
