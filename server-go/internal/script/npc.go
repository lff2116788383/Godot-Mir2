package script

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// ==================== NPC 脚本的 C 函数绑定 ====================
//
// 对应 server/src/npchar.cpp 的 bindFunction 列表。
//
// 分两类：
//
//	必需（NPC 对话跑得起来）：uidPostXML/uidQueryName/getNPCName/getNPCFullName…
//	占位（会话/仓库/交易/修理）：先返回安全的默认值 + 日志，不让脚本崩
//
// ★ 占位为什么不直接不绑定：
//
//	Lua 里调一个 nil 会抛 "attempt to call a nil value"，整个事件处理中断，
//	玩家点了 NPC 什么都收不到。绑定成「返回 nil 的空函数」，
//	脚本会走进 else 分支继续往下跑，至少能看到对话。
//	这些函数在 REPLICA_AUDIT 里逐条记为待实现。

// NPCHost 是 NPC 运行时需要的宿主能力（由 game 包实现）。
//
// 抽成接口而不是直接依赖 game：script 包不该 import game
// （game 要 import script），否则循环依赖。
type NPCHost interface {
	// PostXML 把一段 XML 布局发给玩家（SM_NPCXMLLAYOUT）
	PostXML(playerUID uint64, eventPath string, xml string) error
	// PostInvOpCost 发送背包操作费用（SM_INVOPCOST）
	PostInvOpCost(playerUID uint64, invOp int, itemID, seqID uint32, cost uint32) error
	// PostStartInvOp 发起一次背包操作（SM_STARTINVOP）
	PostStartInvOp(playerUID uint64, invOp int, queryTag, commitTag string, typeList []string) error
	// PostStartInput 让客户端弹出输入框（SM_STARTINPUT）
	PostStartInput(playerUID uint64, title, commitTag string, security bool) error
	// PostSell 让客户端弹出出售列表（SM_SELLITEMLIST）
	PostSell(playerUID uint64, itemIDs []uint32, costList []InvOpCost) error

	// QueryName 查询玩家名字
	QueryName(playerUID uint64) (string, bool)
	// QueryLevel 查询玩家等级
	QueryLevel(playerUID uint64) (int, bool)
	// QueryGold 查询玩家金币
	QueryGold(playerUID uint64) (uint64, bool)
	// QueryRedName 判断玩家是否红名
	QueryRedName(playerUID uint64) bool
	// QueryItemDuration 查询物品持久度
	QueryItemDuration(playerUID uint64, itemID, seqID uint32) (cur, max int, ok bool)

	// RepairItem 修理背包物品（special=true 为特殊修理）
	//
	// ★ 由脚本传 cost 进来而不是服务端自己算：
	//
	//	原版的定价在 include/invop.lua 里（(max-curr)*30，特殊修理 ×3），
	//	是**脚本层策略**而不是协议规则。写在服务端会让
	//	「改个修理价格」变成改代码重编译。
	RepairItem(playerUID uint64, itemID, seqID uint32, special bool, cost uint64) bool
	// SecureItem 把背包物品寄存到仓库
	SecureItem(playerUID uint64, itemID, seqID uint32) bool
	// PostSecuredItemList 把仓库列表推给玩家
	PostSecuredItemList(playerUID uint64) error

	// RemoveGold 扣金币
	RemoveGold(playerUID uint64, amount uint64) bool
	// GrantGold 给金币
	GrantGold(playerUID uint64, amount uint64) bool

	// SpaceMove 传送玩家
	SpaceMove(playerUID uint64, mapID uint32, x, y int) error

	// RemoteCall 在「玩家范围」执行一段 Lua 代码（uidRemoteCall 的核心）。
	//
	// ★ 原版语义（serverluacoroutinerunner.cpp:217）：
	//
	//	向目标 actor 发 AM_REMOTECALL {code, args}，
	//	目标 actor 在自己的 Lua 状态里把 code 当 chunk 执行，
	//	取 varargs 做参数，把返回值序列化回来。
	//
	//	「在自己的 Lua 状态里」是关键：玩家的金币/背包/公会都归玩家 actor 管，
	//	NPC 的 Lua 状态看不到这些。所以 uidRemoteCall 不是 RPC，
	//	而是「让目标 actor 执行我写的代码」。
	//
	// Go 侧没有 per-actor Lua 状态（那需要给每个玩家起一个 gopher-lua，
	// 内存与加载成本都太高），所以改成「按代码串识别意图 + 在 Go 里执行等价操作」。
	// 见 remote_call.go 的说明与已知限制。
	RemoteCall(targetUID uint64, code string, args []RemoteArg) ([]RemoteArg, error)

	// Log 写日志（对应 addLogString）
	Log(level int, msg string)
}

// RemoteArg 是跨范围调用的一个参数（对应 luaf::luaVar 的子集）。
//
// 只保留脚本里实际用到的三种类型：数字、字符串、布尔。
// 表/函数无法跨 actor 传递，原版也是靠 cerealf 序列化支持有限类型的。
type RemoteArg struct {
	Kind  RemoteArgKind
	Num   float64
	Str   string
	Bool  bool
	IsNil bool
}

// RemoteArgKind 是 RemoteArg 的类型标签。
type RemoteArgKind uint8

const (
	RemoteArgNil RemoteArgKind = iota
	RemoteArgNumber
	RemoteArgString
	RemoteArgBool
)

// RemoteNumber / RemoteString / RemoteBool 是构造参数的小工具。
func RemoteNumber(v float64) RemoteArg {
	return RemoteArg{Kind: RemoteArgNumber, Num: v}
}
func RemoteString(v string) RemoteArg {
	return RemoteArg{Kind: RemoteArgString, Str: v}
}
func RemoteBool(v bool) RemoteArg {
	return RemoteArg{Kind: RemoteArgBool, Bool: v}
}

// AsNumber / AsString 是取值辅助（类型不符返回零值）。
func (a RemoteArg) AsNumber() float64 {
	if a.Kind == RemoteArgNumber {
		return a.Num
	}
	return 0
}

func (a RemoteArg) AsString() string {
	if a.Kind == RemoteArgString {
		return a.Str
	}
	return ""
}

// InvOpCost 是一条背包操作花费（SDCostItem）。
type InvOpCost struct {
	ItemID uint32
	Count  uint64
}

// NPCScript 是一个已加载的 NPC 脚本实例。
type NPCScript struct {
	Runtime *Runtime

	// NPCUID 这个 NPC 的 UID
	NPCUID uint64
	// Name NPC 名（文件名里的那一段）
	Name string
	// FullName 地图名.NPC名（getNPCFullName 的返回值）
	FullName string
	// MapName 地图名
	MapName string
	// ScriptPath 脚本文件路径
	ScriptPath string

	host NPCHost
}

// LoadNPC 加载一个 NPC 脚本。
//
// 步骤严格照原版 NPChar 构造 + LuaThreadRunner 构造：
//
//  1. 建 Lua 状态，注入 base.lua 与常量
//  2. 绑定 NPC 专属函数（uidPostXML / getNPCName …）
//  3. 加载 npchar.lua（事件分发核心）
//  4. 加载具体 NPC 的地图脚本（它会调 setEventHandler）
func LoadNPC(opts Options, npc NPCInfo, host NPCHost) (*NPCScript, error) {
	// 把 NPC 相关绑定并进 Options.Bindings
	bindings := make(map[string]lua.LGFunction, len(opts.Bindings)+40)
	for k, v := range opts.Bindings {
		bindings[k] = v
	}

	s := &NPCScript{
		NPCUID:     npc.UID,
		Name:       npc.Name,
		FullName:   npc.MapName + "." + npc.Name,
		MapName:    npc.MapName,
		ScriptPath: npc.ScriptPath,
		host:       host,
	}
	for name, fn := range s.bindings() {
		bindings[name] = fn
	}
	opts.Bindings = bindings

	rt, err := NewRuntime(opts)
	if err != nil {
		return nil, err
	}
	s.Runtime = rt

	// 沙箱（getTLSTable / 协程独立 globals）必须在 npchar.lua 之前装好
	if err := rt.InstallSandbox(); err != nil {
		rt.Close()
		return nil, err
	}
	if err := rt.LoadNPCFramework(); err != nil {
		rt.Close()
		return nil, err
	}
	if err := rt.LoadScript(npc.ScriptPath); err != nil {
		rt.Close()
		return nil, err
	}
	return s, nil
}

// NPCInfo 是加载 NPC 所需的静态信息。
type NPCInfo struct {
	UID        uint64
	Name       string
	MapName    string
	ScriptPath string
	// MapID / X / Y / LookID / GfxDir 来自文件名
	MapID  uint32
	X, Y   int
	LookID uint32
	GfxDir uint8
}

// HandleEvent 处理一次 NPC 事件（对应 on_AM_NPCEVENT 的核心）。
//
// ★ 必须在协程里跑，不能直接 CallGlobal：
//
//	npchar.lua:403 的 `getTLSTable().uid = from` 在协程外调用会
//	直接 error('call getTLSTable() in main thread')。
//	第一次实现就是直接调用，报的错是"attempt to call a non-function object"
//	—— 看起来像函数没绑定，实际是 getTLSTable 内部主动 throw。
func (s *NPCScript) HandleEvent(playerUID uint64, path string, event string, value *string) error {
	res := s.SpawnEvent(playerUID, path, event, value)
	if res.Err != nil {
		s.host.Log(1, fmt.Sprintf("npc %s event %q failed: %v", s.FullName, event, res.Err))
		return res.Err
	}
	return nil
}

// HasEventHandler 判断这个 NPC 是否装了事件处理器（原版 hasEventHandler()）。
func (s *NPCScript) HasEventHandler() bool {
	ret, err := s.Runtime.CallGlobal("hasEventHandler")
	if err != nil || len(ret) == 0 {
		return false
	}
	b, ok := ret[0].(lua.LBool)
	return ok && bool(b)
}

// Close 释放脚本状态。
func (s *NPCScript) Close() {
	if s.Runtime != nil {
		s.Runtime.Close()
	}
}

// ==================== 绑定实现 ====================

func (s *NPCScript) bindings() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		// ---- NPC 自身信息 ----
		"getNPCName": s.luaGetNPCName,
		"getNPCFullName": func(l *lua.LState) int {
			l.Push(lua.LString(s.FullName))
			return 1
		},
		"getNPCMapName":     s.luaGetNPCMapName,
		"getSubukGuildName": func(l *lua.LState) int { l.Push(lua.LString("")); return 1 },
		"setNPCSell":        s.luaSetNPCSell,
		"addNPCSell":        func(l *lua.LState) int { return 0 },
		"clearNPCSell":      func(l *lua.LState) int { return 0 },

		// ---- 发消息给客户端 ----
		"uidPostXML":             s.luaPostXML,
		"uidPostXMLString":       s.luaPostXML,
		"uidPostInvOpCost":       s.luaPostInvOpCost,
		"uidPostStartInvOp":      s.luaPostStartInvOp,
		"uidPostStartInput":      s.luaPostStartInput,
		"uidPostSell":            s.luaPostSell,
		"uidPostAuctionItemList": func(l *lua.LState) int { return 0 },

		// ---- 查询玩家 ----
		"uidQueryName":         s.luaQueryName,
		"uidQueryLevel":        s.luaQueryLevel,
		"uidQueryGold":         s.luaQueryGold,
		"uidQueryRedName":      s.luaQueryRedName,
		"uidQueryItemDuration": s.luaQueryItemDuration,

		// ---- 金币 ----
		"uidRemoveGold": s.luaRemoveGold,
		"uidGrantGold":  s.luaGrantGold,

		// ---- 传送 ----
		"uidSpaceMove":    s.luaSpaceMove,
		"uidReqSpaceMove": s.luaSpaceMove,

		// ---- 未实现的占位（返回 nil / false，不抛错）----
		"uidRemoteCall":          s.luaRemoteCall,
		"uidStartTrade":          luaReturnNothing,
		"uidStartRepair":         luaReturnNothing,
		"uidRepairItem":          s.luaRepairItem,
		"uidStartSecure":         luaReturnNothing,
		"uidSecureItem":          s.luaSecureItem,
		"uidPostSecuredItemList": s.luaPostSecuredItemList,
		"uidShowSecuredItemList": luaReturnNothing,
		"uidGrant":               luaReturnNothing,
		"uidRemove":              luaReturnNothing,
		"uidDBSetKey":            func(l *lua.LState) int { l.Push(lua.LTrue); return 1 },
		"uidDBGetKey":            func(l *lua.LState) int { l.Push(lua.LNil); return 1 },
		"uidGetXMLSeqID":         func(l *lua.LState) int { l.Push(lua.LNumber(1)); return 1 },
	}
}

func luaReturnNothing(l *lua.LState) int { return 0 }

// luaGetNPCName 对应 npchar.cpp:88 getNPCName([skipSuffix])。
//
// 默认把 "阿天_1" 截成 "阿天"（去掉 _1 这种序号后缀）。
// 传 true 才返回原名。这是脚本里最常调的：对话正文「我是%s」用它。
func (s *NPCScript) luaGetNPCName(l *lua.LState) int {
	skip := true
	if l.GetTop() >= 1 {
		skip = l.ToBool(1)
	}
	name := s.Name
	if skip {
		for i := 0; i < len(name); i++ {
			if name[i] == '_' {
				name = name[:i]
				break
			}
		}
	}
	l.Push(lua.LString(name))
	return 1
}

// luaGetNPCMapName 对应 npchar.cpp:113。
func (s *NPCScript) luaGetNPCMapName(l *lua.LState) int {
	skip := true
	if l.GetTop() >= 1 {
		skip = l.ToBool(1)
	}
	name := s.MapName
	if skip {
		for i := 0; i < len(name); i++ {
			if name[i] == '_' {
				name = name[:i]
				break
			}
		}
	}
	l.Push(lua.LString(name))
	return 1
}

// luaSetNPCSell 对应 npchar.cpp:60 setNPCSell(itemNameList)。
//
// 脚本用它声明「这个 NPC 卖什么」，客户端商店列表由此生成。
// 当前版本只记录日志 —— 真正的售卖表在 shopkeeper 扩展里，
// 与 game.ShopHandler 的价格逻辑对齐后再接。
func (s *NPCScript) luaSetNPCSell(l *lua.LState) int {
	tbl := l.CheckTable(1)
	names := make([]string, 0, tbl.Len())
	tbl.ForEach(func(_, v lua.LValue) {
		names = append(names, v.String())
	})
	s.host.Log(0, fmt.Sprintf("npc %s sell list: %d items", s.FullName, len(names)))
	return 0
}

// luaPostXML 对应 npchar.cpp:337 uidPostXMLString(uid, path, xml)。
func (s *NPCScript) luaPostXML(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	path := l.OptString(2, "")
	xmlStr := l.CheckString(3)
	if err := s.host.PostXML(uid, path, xmlStr); err != nil {
		s.host.Log(1, fmt.Sprintf("npc %s post xml failed: %v", s.FullName, err))
	}
	return 0
}

// luaPostInvOpCost 对应 npchar.cpp:441 postInvOpCost。
func (s *NPCScript) luaPostInvOpCost(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	invOp := l.CheckInt(2)
	itemID := uint32(l.CheckInt64(3))
	seqID := uint32(l.CheckInt64(4))
	cost := uint32(l.CheckInt64(5))
	_ = s.host.PostInvOpCost(uid, invOp, itemID, seqID, cost)
	return 0
}

// luaPostStartInvOp 对应 npchar.cpp:419 postStartInvOp。
func (s *NPCScript) luaPostStartInvOp(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	invOp := l.CheckInt(2)
	queryTag := l.CheckString(3)
	commitTag := l.CheckString(4)
	typeList := []string{}
	if tbl, ok := l.Get(5).(*lua.LTable); ok {
		tbl.ForEach(func(_, v lua.LValue) {
			typeList = append(typeList, v.String())
		})
	}
	_ = s.host.PostStartInvOp(uid, invOp, queryTag, commitTag, typeList)
	return 0
}

// luaPostStartInput 对应 npchar.cpp:474 postStartInput。
func (s *NPCScript) luaPostStartInput(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	title := l.CheckString(2)
	commitTag := l.CheckString(3)
	security := l.OptBool(4, false)
	_ = s.host.PostStartInput(uid, title, commitTag, security)
	return 0
}

// luaPostSell 对应 SM_SELLITEMLIST 的推送。
//
// 脚本里的签名是 uidPostSell(uid, itemList)，itemList 是
// {itemID, seqID, cost} 的数组。
func (s *NPCScript) luaPostSell(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	tbl := l.CheckTable(2)
	var itemIDs []uint32
	var costs []InvOpCost
	tbl.ForEach(func(_, v lua.LValue) {
		row, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		itemID := uint32(lua.LVAsNumber(row.RawGetInt(1)))
		cost := uint64(lua.LVAsNumber(row.RawGetInt(3)))
		itemIDs = append(itemIDs, itemID)
		costs = append(costs, InvOpCost{ItemID: itemID, Count: cost})
	})
	_ = s.host.PostSell(uid, itemIDs, costs)
	return 0
}

func (s *NPCScript) luaQueryName(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	if name, ok := s.host.QueryName(uid); ok {
		l.Push(lua.LString(name))
	} else {
		l.Push(lua.LNil)
	}
	return 1
}

func (s *NPCScript) luaQueryLevel(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	if level, ok := s.host.QueryLevel(uid); ok {
		l.Push(lua.LNumber(level))
	} else {
		l.Push(lua.LNumber(0))
	}
	return 1
}

func (s *NPCScript) luaQueryGold(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	if gold, ok := s.host.QueryGold(uid); ok {
		l.Push(lua.LNumber(gold))
	} else {
		l.Push(lua.LNumber(0))
	}
	return 1
}

func (s *NPCScript) luaQueryRedName(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	l.Push(lua.LBool(s.host.QueryRedName(uid)))
	return 1
}

func (s *NPCScript) luaQueryItemDuration(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	itemID := uint32(l.CheckInt64(2))
	seqID := uint32(l.CheckInt64(3))
	if cur, max, ok := s.host.QueryItemDuration(uid, itemID, seqID); ok {
		l.Push(lua.LNumber(cur))
		l.Push(lua.LNumber(max))
		return 2
	}
	l.Push(lua.LNil)
	return 1
}

func (s *NPCScript) luaRemoveGold(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	amount := uint64(l.CheckInt64(2))
	l.Push(lua.LBool(s.host.RemoveGold(uid, amount)))
	return 1
}

func (s *NPCScript) luaGrantGold(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	amount := uint64(l.CheckInt64(2))
	l.Push(lua.LBool(s.host.GrantGold(uid, amount)))
	return 1
}

func (s *NPCScript) luaSpaceMove(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	// 地图参数可以是 ID 或名字；当前只支持数字
	mapArg := l.Get(2)
	var mapID uint32
	switch v := mapArg.(type) {
	case lua.LNumber:
		mapID = uint32(v)
	case lua.LString:
		s.host.Log(1, fmt.Sprintf("uidSpaceMove by map name %q not supported yet", string(v)))
		l.Push(lua.LFalse)
		return 1
	default:
		l.Push(lua.LFalse)
		return 1
	}
	x := l.CheckInt(3)
	y := l.CheckInt(4)
	if err := s.host.SpaceMove(uid, mapID, x, y); err != nil {
		l.Push(lua.LFalse)
		return 1
	}
	l.Push(lua.LTrue)
	return 1
}

// luaRemoteCall 是跨范围调用（uidRemoteCall）。
//
// 原版签名（serverluacoroutinerunner.lua:290）：
//
//	uidRemoteCall(uid, ..., code)   -- 最后一个参数是代码串，前面全是 code 的实参
//
// 例：
//
//	player.getGold(uid)          → uidRemoteCall(uid, [[ return getGold() ]])
//	player.declareGuildWar(uid, name, gold)
//	                             → uidRemoteCall(uid, name, gold,
//	                                     [[ return declareGuildWar(...) ]])
//	player.addItem(uid, item, n) → uidRemoteCall(uid, item, n,
//	                                     [[ local item, count = ... return addItem(item, count) ]])
//
// ★ 代码串里用 `...` 取参数，所以实参顺序必须原样传过去，不能自己重排。
//
// ★ 返回值是**多值**：原版返回 sol::as_args(resList)，调用方用
// `local a, b = uidRemoteCall(...)` 接。gopher-lua 侧直接 push 多个值即可。
func (s *NPCScript) luaRemoteCall(l *lua.LState) int {
	top := l.GetTop()
	if top < 2 {
		s.host.Log(1, "uidRemoteCall: needs at least (uid, code)")
		l.Push(lua.LNil)
		return 1
	}

	targetUID := uint64(l.CheckInt64(1))
	code := l.CheckString(top)

	// 中间的都是实参
	args := make([]RemoteArg, 0, top-2)
	for i := 2; i < top; i++ {
		args = append(args, luaToRemoteArg(l.Get(i)))
	}

	results, err := s.host.RemoteCall(targetUID, code, args)
	if err != nil {
		// 原版这里 fatalPrintf，会让整个协程失败。
		// Go 侧退化成「记日志 + 返回 nil」：
		// 一个远程调用失败不该把 NPC 对话整个打断（玩家会卡在对话里）。
		//
		// ★ 但要把错误打出来 —— 否则「NPC 说已宣战但实际没宣」这种
		//   静默失败极难排查。
		s.host.Log(1, fmt.Sprintf("uidRemoteCall to %d failed: %v (code=%s)",
			targetUID, err, shortenCode(code)))
		l.Push(lua.LNil)
		return 1
	}

	if len(results) == 0 {
		l.Push(lua.LNil)
		return 1
	}
	for _, r := range results {
		l.Push(remoteArgToLua(l, r))
	}
	return len(results)
}

// shortenCode 把代码串压成一行便于日志（去掉换行与多余空白）。
func shortenCode(code string) string {
	out := make([]rune, 0, 80)
	lastSpace := false
	for _, r := range code {
		if r == '\n' || r == '\t' || r == '\r' {
			r = ' '
		}
		if r == ' ' {
			if lastSpace {
				continue
			}
			lastSpace = true
		} else {
			lastSpace = false
		}
		out = append(out, r)
		if len(out) >= 80 {
			return string(out) + "…"
		}
	}
	return string(out)
}

// luaToRemoteArg 把 Lua 值转成可跨范围的参数。
func luaToRemoteArg(v lua.LValue) RemoteArg {
	switch v.Type() {
	case lua.LTNil:
		return RemoteArg{Kind: RemoteArgNil, IsNil: true}
	case lua.LTNumber:
		return RemoteNumber(float64(v.(lua.LNumber)))
	case lua.LTString:
		return RemoteString(string(v.(lua.LString)))
	case lua.LTBool:
		return RemoteBool(bool(v.(lua.LBool)))
	default:
		// 表/函数/用户数据无法跨范围：原版靠 cerealf 也会失败
		return RemoteArg{Kind: RemoteArgNil, IsNil: true}
	}
}

// remoteArgToLua 把返回值转回 Lua 值。
func remoteArgToLua(l *lua.LState, a RemoteArg) lua.LValue {
	switch a.Kind {
	case RemoteArgNumber:
		return lua.LNumber(a.Num)
	case RemoteArgString:
		return lua.LString(a.Str)
	case RemoteArgBool:
		return lua.LBool(a.Bool)
	default:
		return lua.LNil
	}
}

// luaRepairItem 对应 player.cpp:188 的 repairItem 绑定。
//
// Lua 签名：uidRepairItem(uid, itemID, seqID, special, cost)
//
// ★ cost 是脚本算好的价格（invop.lua 里的 (max-curr)*30，特殊 ×3）。
// 服务端负责「扣钱 + 恢复持久」，不负责定价。
func (s *NPCScript) luaRepairItem(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	itemID := uint32(l.CheckInt64(2))
	seqID := uint32(l.CheckInt64(3))
	special := l.CheckBool(4)
	cost := uint64(l.OptInt64(5, 0))

	l.Push(lua.LBool(s.host.RepairItem(uid, itemID, seqID, special, cost)))
	return 1
}

// luaSecureItem 对应 player.cpp:178 的 secureItem 绑定。
func (s *NPCScript) luaSecureItem(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	itemID := uint32(l.CheckInt64(2))
	seqID := uint32(l.CheckInt64(3))
	s.host.SecureItem(uid, itemID, seqID)
	return 0
}

// luaPostSecuredItemList 把控仓库列表推给客户端。
//
// 对应 player.cpp 里 querySecuredItemList 那一带的绑定：
// NPC 打开仓库面板时调用，客户端据此画出「已寄存物品」列表。
func (s *NPCScript) luaPostSecuredItemList(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	_ = s.host.PostSecuredItemList(uid)
	return 0
}
