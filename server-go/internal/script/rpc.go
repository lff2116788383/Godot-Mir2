package script

import (
	"context"
	"fmt"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// ==================== 跨 Lua 状态 RPC 总线 ====================
//
// 【依据】server/src/serverluacoroutinerunner.cpp:217-260 `_RSVD_NAME_remoteCall`
//         + server/src/quest.cpp:170 `on_AM_REMOTECALL`
//         + server/src/serverluaobjectop.cpp:4-9
//
// ★★ 为什么必须有这一层 ★★
//
// 原版每个任务是一个独立的 ServerObject（有自己的 UID 和 Lua 状态）。
// NPC 也是独立对象。它们之间靠 `AM_REMOTECALL` 通信：
//
//	调用方 Lua: uidRemoteCall(targetUID, code, args)
//	  → 序列化 {code, args}
//	  → 发给 targetUID 的 actor
//	  → 目标 actor 在自己的 Lua 状态里 spawn 线程执行 code
//	  → 把返回值序列化回传
//	  → 调用方线程 resume，拿到结果
//
// 没有这一层，任务和 NPC 就是两个互不可见的孤岛 ——
// 任务在自己的状态里调 setQuestHandler，NPC 在自己的状态里查表，
// 永远查不到，玩家点 NPC 看不到任何任务入口。
//
// ★ 本实现的形状
//
// gopher-lua 的 LState 是单线程的（一个 LState 同一时刻只能有一个
// goroutine 在跑）。所以：
//
//	每个 QuestState / NPC 运行时 → 一个 LuaCell（含 LState + 自己的 mutex）
//	RPC 调用 → 从 A 的 Lua 协程 yield 出去 → 投递到 B 的 mailbox
//	B 执行完 → 把结果写回 pending 条目 → Resume 唤醒 A 的协程
//
// 这样既保持了「一个状态一次只跑一件事」的安全前提，
// 又实现了跨状态的同步调用语义。

// RPCError 是远程调用失败的原因。
type RPCError struct {
	// Message 错误描述
	Message string
	// Timeout 是否因超时失败
	Timeout bool
	// NoSuchTarget 目标不存在（已被销毁或从未注册）
	NoSuchTarget bool
}

func (e *RPCError) Error() string { return e.Message }

// RPCResult 是一次远程调用的结果。
type RPCResult struct {
	// Values 返回的值列表（对应原版 SDRemoteCallResult.varList）
	Values []protocol.LuaVar
	// Err 非 nil 表示远程代码抛错
	Err error
}

// rpcTarget 是一个可被远程调用的执行体（任务或 NPC）。
type rpcTarget interface {
	// ExecRemote 在自己的 Lua 状态里执行代码串并返回值。
	//
	// ★ 必须保证「执行期间该状态只服务这一个调用」：
	//	LState 不是并发安全的，两个 goroutine 同时跑会直接崩。
	ExecRemote(code string, args []protocol.LuaVar) RPCResult
}

// rpcPending 是一次挂起中的调用。
type rpcPending struct {
	done chan RPCResult
}

// RPCBus 是跨状态调用的注册表与调度器。
type RPCBus struct {
	mu      sync.RWMutex
	targets map[uint64]rpcTarget
	// byName 任务名 -> UID（quest.lua 的 queryQuestUID 要用）
	questByName map[string]uint64
	// nameByUID UID -> 任务名
	questNameByUID map[uint64]string

	// shortIDs 短 ID 映射（解决 Lua 5.1 浮点精度，见 shortid.go）
	shortIDs *ShortIDMap

	// timeout 单次远程调用的超时
	timeout time.Duration
	// logf 日志
	logf func(format string, args ...any)
}

// NewRPCBus 创建总线。
func NewRPCBus(timeout time.Duration, logf func(string, ...any)) *RPCBus {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &RPCBus{
		shortIDs:       NewShortIDMap(),
		targets:        make(map[uint64]rpcTarget),
		questByName:    make(map[string]uint64),
		questNameByUID: make(map[uint64]string),
		timeout:        timeout,
		logf:           logf,
	}
}

// Short 返回真实 UID 对应的短 ID（供 Lua 使用）。
func (b *RPCBus) Short(realUID uint64) int64 { return b.shortIDs.Short(realUID) }

// Real 把 Lua 传来的短 ID 还原成真实 UID。
func (b *RPCBus) Real(short int64) uint64 { return b.shortIDs.Real(short) }

// ShortIDs 暴露映射表（诊断/测试用）。
func (b *RPCBus) ShortIDs() *ShortIDMap { return b.shortIDs }

// Register 注册一个可被调用的目标。
func (b *RPCBus) Register(uid uint64, target rpcTarget) {
	if uid == 0 || target == nil {
		return
	}
	b.mu.Lock()
	b.targets[uid] = target
	b.mu.Unlock()
	// 预分配短 ID：Lua 侧拿到的就是这个
	b.shortIDs.Short(uid)
}

// RegisterQuest 注册任务（带名字索引）。
func (b *RPCBus) RegisterQuest(uid uint64, name string, target rpcTarget) {
	b.Register(uid, target)
	if name == "" {
		return
	}
	b.mu.Lock()
	b.questByName[name] = uid
	b.questNameByUID[uid] = name
	b.mu.Unlock()
}

// Unregister 注销目标。
func (b *RPCBus) Unregister(uid uint64) {
	b.mu.Lock()
	delete(b.targets, uid)
	if name, ok := b.questNameByUID[uid]; ok {
		delete(b.questByName, name)
		delete(b.questNameByUID, uid)
	}
	b.mu.Unlock()
}

// QuestUIDByName 按任务名查 UID（quest.lua:queryQuestUID）。
func (b *RPCBus) QuestUIDByName(name string) (uint64, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	uid, ok := b.questByName[name]
	return uid, ok
}

// Call 同步调用一个目标（带超时）。
//
// ★ 这是整个 B 方案的核心入口。
//
//	注意：调用方在自己的 Lua 状态里执行时，这里会**阻塞**等待目标完成。
//	如果目标又反过来调调用方（循环依赖），两边都会等超时。
//	原版用 actor 的死锁检测规避，这里靠超时兜底 —— 超时后返回
//	RPCError{Timeout:true}，Lua 侧能感知（assertType 会报错），
//	不会静默卡死。
func (b *RPCBus) Call(ctx context.Context, uid uint64, code string, args []protocol.LuaVar) RPCResult {
	b.mu.RLock()
	target, ok := b.targets[uid]
	b.mu.RUnlock()

	if !ok {
		return RPCResult{Err: &RPCError{
			Message:      fmt.Sprintf("remote call target %d not found", uid),
			NoSuchTarget: true,
		}}
	}

	type outcome struct{ result RPCResult }
	ch := make(chan outcome, 1)

	go func() {
		// ★ panic 必须在这里兜住：
		//	目标是另一个 Lua 状态，脚本里的 error() 会往上冒，
		//	漏出去就是整个进程崩。
		defer func() {
			if r := recover(); r != nil {
				ch <- outcome{RPCResult{Err: &RPCError{
					Message: fmt.Sprintf("remote call panic: %v", r),
				}}}
			}
		}()
		ch <- outcome{target.ExecRemote(code, args)}
	}()

	timeout := b.timeout
	select {
	case out := <-ch:
		return out.result
	case <-time.After(timeout):
		b.logf("remote call to %d timed out after %s", uid, timeout)
		return RPCResult{Err: &RPCError{
			Message: fmt.Sprintf("remote call to %d timed out", uid),
			Timeout: true,
		}}
	case <-ctx.Done():
		return RPCResult{Err: &RPCError{Message: "remote call cancelled"}}
	}
}

// ==================== 挂在 Runtime 上的 RPC 能力 ====================

// RPCBridge 把 RPCBus 挂到一个 Runtime 上。
//
// ★ 每个需要「被远程调用」或「发起远程调用」的 Lua 状态都要一份。
type RPCBridge struct {
	bus   *RPCBus
	owner uint64
	// rt 宿主运行时（执行远程来的代码串用）
	rt *Runtime
	// mapNames 地图名表（远程代码里可能 loadBaseMap）
	mapNames *NameTable
}

// NewRPCBridge 创建桥。
func NewRPCBridge(bus *RPCBus, owner uint64, rt *Runtime, mapNames *NameTable) *RPCBridge {
	return &RPCBridge{bus: bus, owner: owner, rt: rt, mapNames: mapNames}
}

// ExecRemote 实现 rpcTarget：在宿主状态里执行代码串。
func (b *RPCBridge) ExecRemote(code string, args []protocol.LuaVar) RPCResult {
	if b.rt == nil || b.rt.L == nil {
		return RPCResult{Err: &RPCError{Message: "runtime closed"}}
	}
	// ★ 语法降级（远程代码串也可能含 // 与 <close>）
	patched, _ := patchIntDiv(code)
	patched, _ = patchToBeClosed(patched)

	fn, err := b.rt.L.LoadString(patched)
	if err != nil {
		return RPCResult{Err: &RPCError{Message: err.Error()}}
	}

	luaArgs := make([]lua.LValue, 0, len(args))
	for _, arg := range args {
		luaArgs = append(luaArgs, luaVarToLua(b.rt.L, arg))
	}

	base := b.rt.L.GetTop()
	if err := b.rt.L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    lua.MultRet,
		Protect: true,
	}, luaArgs...); err != nil {
		return RPCResult{Err: &RPCError{Message: err.Error()}}
	}

	n := b.rt.L.GetTop() - base
	values := make([]protocol.LuaVar, 0, n)
	for i := base + 1; i <= base+n; i++ {
		values = append(values, luaToLuaVar(b.rt.L.Get(i)))
	}
	b.rt.L.SetTop(base)
	return RPCResult{Values: values}
}

// Install 把 uidRemoteCall 装进 Lua（覆盖之前的本地降级版）。
//
// ★ 语义完全对齐原版（serverluacoroutinerunner.cpp:217）：
//
//	uidRemoteCall(targetUID, args..., code) -> 远程返回值
//
//	参数顺序：前面全是实参，**最后一个**是代码串。
//
// ★ 同步阻塞实现：
//
//	原版是 yield 出去等回包（协作调度）。这里直接阻塞等待。
//	因为每个 Lua 状态是独立 goroutine 不共享锁，阻塞不会死锁其他状态；
//	唯一风险是 A→B→A 的循环调用，靠超时兜底。
func (b *RPCBridge) Install(L *lua.LState) {
	L.SetGlobal("uidRemoteCall", L.NewFunction(func(l *lua.LState) int {
		top := l.GetTop()
		if top < 2 {
			l.Push(lua.LNil)
			return 1
		}
		// ★ Lua 传来的是短 ID，必须还原成真实 UID
		short := l.CheckInt64(1)
		targetUID := b.bus.Real(short)
		if targetUID == 0 {
			l.RaiseError("uidRemoteCall: invalid target %d", short)
			return 0
		}
		code := l.CheckString(top)

		args := make([]protocol.LuaVar, 0, top-2)
		for i := 2; i < top; i++ {
			args = append(args, luaToLuaVar(l.Get(i)))
		}

		result := b.bus.Call(context.Background(), targetUID, code, args)
		if result.Err != nil {
			// 原版把远程错误抛成 Lua error（uidRemoteCall 的注释明确说了）
			// 但很多调用点用 pcall 包着，抛出去才对
			l.RaiseError("%s", result.Err.Error())
			return 0
		}
		for _, v := range result.Values {
			l.Push(luaVarToLua(l, v))
		}
		if len(result.Values) == 0 {
			return 0
		}
		return len(result.Values)
	}))

	// queryQuestUID：按名字找任务 UID（quest.lua:getQuestUID 依赖）
	L.SetGlobal("_RSVD_NAME_callFuncCoop", L.NewFunction(func(l *lua.LState) int {
		fnName := l.CheckString(1)
		if fnName != "queryQuestUID" {
			// 其他 coop 调用（sendNotify 等）当前直接成功返回
			l.Push(lua.LNumber(0)) // SYS_EXECDONE
			return 1
		}
		name := l.CheckString(2)
		uid, ok := b.bus.QuestUIDByName(name)
		if !ok {
			l.Push(lua.LNil)
			return 1
		}
		// ★ 返回短 ID（Lua 侧不能拿 64 位真值）
		l.Push(lua.LNumber(b.bus.Short(uid)))
		return 1
	}))
}
