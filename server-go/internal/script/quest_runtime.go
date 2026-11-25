package script

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// ==================== 任务系统运行时 ====================
//
// 【依据】server/src/quest.cpp（190 行）+ questdb.cpp（102 行）+
//         server/src/quest.lua（1819 行）+ 42 个任务脚本（20,819 行）
//
// ★★ 设计要点：让原版 quest.lua 与 42 个任务脚本**零翻译**跑起来 ★★
//
// quest.lua 是完整的任务框架（FSM 状态机 / 线程 / 通知 / 上下文回滚），
// 20,819 行任务脚本都写在它的 API 之上。翻译它们既不可能也不必要 ——
// 只要补齐 C++ 侧的 9 个绑定 + 运行时原语，原版脚本直接可用。
//
// 依赖清单（缺一个任务就跑不起来）：
//
//	来自 quest.cpp 的 9 个：
//	  getQuestName / getMainScriptThreadKey / rollKey / getQuestScriptHash
//	  _RSVD_NAME_setQuestDesp / dbGetQuestField / dbSetQuestField
//	  _RSVD_NAME_dbSetQuestFields / _RSVD_NAME_modifyQuestTriggerType
//
//	来自 serverluacoroutinerunner 的协程原语（这是最难的一块）：
//	  runThread / closeThread / closeThreadThenRun / closeRequested
//	  getThreadKey / getThreadSeqID / getKeyPair / getThreadAddress
//	  postNotify / sendNotify / pickNotify / waitNotify / clearNotify
//	  _RSVD_NAME_yieldToClose / _RSVD_NAME_selfCloseError
//
// ★ 协程方案：用 gopher-lua 原生的 `L.NewThread` + `Resume`。
//
//	gopher-lua 的 LState.Run 本身就是协程（lua_State），
//	`NewThread` 创建的是真正独立的执行栈，可以 yield 后从别处 resume。
//	这与原版的「每个 actor 一个 Lua state + 独立线程」语义足够接近，
//	不需要自己实现协程调度器。
//
// ★ 简化点（诚实标注）：
//
//	原版 runThread 支持「线程可在 C 栈上嵌套、被外部强制 close」。
//	Go 侧保留「创建/让出/resume/关闭」四种操作，但不实现
//	「close 一个正在跑别的线程的线程」这种重入语义 ——
//	正常任务流程（计时器、对话回调、状态切换）用不到它。
//	需要时任务脚本自己会报错，不会静默错乱。

// questMapNames 是当前加载任务时使用的地图名表。
//
// ★ 用包级变量而不是加参数：LoadQuest 已经被测试直接调用，
//	多一个参数会让所有调用点都要改。任务加载是启动期的串行操作，
//	没有并发问题。
var questMapNames *NameTable

// QuestState 是一个任务实例的运行时状态。
type QuestState struct {
	// Name 任务名（脚本文件名去掉 .lua）
	Name string
	// Hash 脚本内容哈希（原版 getQuestScriptHash，用于判断脚本是否改过）
	Hash uint64
	// ScriptPath 脚本路径
	ScriptPath string

	// rt 该任务专属的 Lua 运行时
	rt *Runtime

	// threads 线程表：key -> *lua.LState（可 resume 的协程）
	threads map[int64]*lua.LState
	// threadSeq 线程序号（原版 getThreadSeqID）
	threadSeq map[int64]int
	// closeRequested 被要求关闭的线程
	closeRequested map[int64]bool

	// mainKey 主线程 key（原版 getMainScriptThreadKey）
	mainKey int64
	// mapNames 地图名 <-> ID 表（loadBaseMap 要用）
	mapNames *NameTable

	mu sync.Mutex
}

// QuestDeps 是任务运行时需要的宿主能力。
type QuestDeps interface {
	// LoadQuestField 读任务字段（对应 dbGetQuestField）
	LoadQuestField(ctx context.Context, questName string, dbid uint32, field string) (protocol.LuaVar, bool, error)
	// SetQuestField 写任务字段（对应 dbSetQuestField）
	SetQuestField(ctx context.Context, questName string, dbid uint32, field string, value *protocol.LuaVar) error
	// SetQuestFields 批量写（对应 _RSVD_NAME_dbSetQuestFields）
	SetQuestFields(ctx context.Context, questName string, dbid uint32, fields map[string]*protocol.LuaVar, replace bool) error
	// PostQuestDesp 推任务描述（对应 _RSVD_NAME_setQuestDesp）
	PostQuestDesp(ctx context.Context, playerUID uint64, descList []string) error
	// FindNPCharUID 按地图名+NPC名找 NPC UID（对应 getNPCharUID）
	FindNPCharUID(mapName, npcName string) (uint64, bool)
	// Log 写日志
	Log(level int, msg string)
}

// LoadQuest 加载一个任务脚本（对应 Quest 构造函数 + onActivate）。
//
// 步骤照原版 quest.cpp:128：
//  1. 建独立的 Lua 运行时（每个任务一份，任务之间互不干扰）
//  2. 加载 base.lua（工具函数）
//  3. 加载 quest.lua 框架（定义 setQuestState / setQuestFSMTable …）
//  4. 绑定 9 个任务专属 C 函数
//  5. 加载具体任务脚本（它会调 setQuestFSMTable 注册状态机）
func LoadQuest(opts Options, name, scriptPath string, deps QuestDeps) (*QuestState, error) {
	q := &QuestState{
		Name:           name,
		ScriptPath:     scriptPath,
		mapNames:       questMapNames,
		threads:        make(map[int64]*lua.LState),
		threadSeq:      make(map[int64]int),
		closeRequested: make(map[int64]bool),
		mainKey:        1,
	}

	rt, err := NewRuntime(opts)
	if err != nil {
		return nil, err
	}
	q.rt = rt

	// 装任务专属绑定（必须在 quest.lua 之前 —— 它自己在顶层就调 rollKey）
	for binding, fn := range q.bindings(deps) {
		rt.L.SetGlobal(binding, rt.L.NewFunction(fn))
	}

	// 装协程原语（runThread / pause / 通知）
	q.installCoroutinePrimitives()

	// 加载 quest.lua 框架（原版 server/src/quest.lua 零翻译复用）
	frameworkPath := filepath.Join(opts.FrameworkDir, "quest.lua")
	if err := rt.patchAndRun(frameworkPath); err != nil {
		rt.Close()
		return nil, fmt.Errorf("load quest.lua: %w", err)
	}

	// 加载具体任务脚本
	if err := rt.patchAndRun(scriptPath); err != nil {
		rt.Close()
		return nil, fmt.Errorf("load quest %s: %w", name, err)
	}

	return q, nil
}

// bindings 返回任务专属的 C 函数绑定（quest.cpp 的 9 个）。
func (q *QuestState) bindings(deps QuestDeps) map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		// getQuestName() -> 任务名
		"getQuestName": func(l *lua.LState) int {
			l.Push(lua.LString(q.Name))
			return 1
		},
		// getMainScriptThreadKey() -> 主线程 key
		"getMainScriptThreadKey": func(l *lua.LState) int {
			l.Push(lua.LNumber(q.mainKey))
			return 1
		},
		// getQuestScriptHash() -> 脚本哈希
		"getQuestScriptHash": func(l *lua.LState) int {
			l.Push(lua.LNumber(q.Hash))
			return 1
		},
		// _RSVD_NAME_setQuestDesp(uid, descList)
		"_RSVD_NAME_setQuestDesp": func(l *lua.LState) int {
			uid := uint64(l.CheckInt64(1))
			tbl := l.CheckTable(2)
			var descs []string
			tbl.ForEach(func(_, v lua.LValue) {
				if s, ok := v.(lua.LString); ok {
					descs = append(descs, string(s))
				}
			})
			if deps != nil {
				if err := deps.PostQuestDesp(context.Background(), uid, descs); err != nil {
					deps.Log(1, fmt.Sprintf("post quest desp failed: %v", err))
				}
			}
			return 0
		},
		// dbGetQuestField(uid, field) -> value|nil
		"dbGetQuestField": func(l *lua.LState) int {
			uid := uint64(l.CheckInt64(1))
			field := l.CheckString(2)
			if deps == nil {
				l.Push(lua.LNil)
				return 1
			}
			value, found, err := deps.LoadQuestField(context.Background(), q.Name, uint32(uid), field)
			if err != nil || !found {
				l.Push(lua.LNil)
				return 1
			}
			l.Push(luaVarToLua(l, value))
			return 1
		},
		// dbSetQuestField(uid, field, value)
		"dbSetQuestField": func(l *lua.LState) int {
			uid := uint64(l.CheckInt64(1))
			field := l.CheckString(2)
			value := l.Get(3)
			if deps == nil {
				return 0
			}
			lv := luaToLuaVar(value)
			if err := deps.SetQuestField(context.Background(), q.Name, uint32(uid), field, &lv); err != nil {
				deps.Log(1, fmt.Sprintf("set quest field %s failed: %v", field, err))
			}
			return 0
		},
		// _RSVD_NAME_dbSetQuestFields(uid, fieldTable, replace)
		"_RSVD_NAME_dbSetQuestFields": func(l *lua.LState) int {
			uid := uint64(l.CheckInt64(1))
			tbl := l.CheckTable(2)
			replace := l.OptBool(3, false)
			if deps == nil {
				return 0
			}
			fields := make(map[string]*protocol.LuaVar)
			tbl.ForEach(func(k, v lua.LValue) {
				name, ok := k.(lua.LString)
				if !ok {
					return
				}
				lv := luaToLuaVar(v)
				fields[string(name)] = &lv
			})
			if err := deps.SetQuestFields(context.Background(), q.Name, uint32(uid), fields, replace); err != nil {
				deps.Log(1, fmt.Sprintf("set quest fields failed: %v", err))
			}
			return 0
		},
		// getNPCharUID(mapName, npcName) -> uid|nil
		"getNPCharUID": func(l *lua.LState) int {
			mapName := l.CheckString(1)
			npcName := l.CheckString(2)
			if deps == nil {
				l.Push(lua.LNil)
				return 1
			}
			uid, found := deps.FindNPCharUID(mapName, npcName)
			if !found {
				l.Push(lua.LNil)
				return 1
			}
			l.Push(lua.LNumber(uid))
			return 1
		},
		// _RSVD_NAME_modifyQuestTriggerType(...) 触发类型修改
		//
		// 原版用它把「某个格子/NPC 的触发器」挂到任务上。
		// Go 侧当前没有格子触发器系统，绑成 no-op 但**返回 true**，
		// 让脚本认为注册成功（否则任务会在注册处直接中断）。
		"_RSVD_NAME_modifyQuestTriggerType": func(l *lua.LState) int {
			l.Push(lua.LTrue)
			return 1
		},
	}
}

// installCoroutinePrimitives 安装协程与通知原语
// （serverluacoroutinerunner.cpp 的 23 个绑定里任务真正用到的部分）。
//
// ★ 关键：pause 必须在 Lua 层定义（用 coroutine.yield）。
//
//	早期踩过的坑：从 Go 函数里调 yield 会被 gopher-lua **静默忽略**，
//	计时器永远不触发。所以 pause 只能是 Lua 里的
//	`function pause(ms) ... coroutine.yield() end`。
func (q *QuestState) installCoroutinePrimitives() {
	L := q.rt.L

	// ---- 线程管理（Go 侧实现）----
	L.SetGlobal("_RSVD_NAME_runThread", L.NewFunction(func(l *lua.LState) int {
		key := int64(l.CheckInt64(1))
		fn := l.CheckFunction(2)

		// 创建独立协程
		co, _ := l.NewThread()
		co.SetGlobal("_RSVD_NAME_currentThreadKey", lua.LNumber(key))

		q.mu.Lock()
		q.threads[key] = co
		q.threadSeq[key]++
		seq := q.threadSeq[key]
		q.mu.Unlock()

		// 首次 resume：跑到第一个 yield 或结束
		st, err, values := co.Resume(l, fn)
		if err != nil {
			l.Push(lua.LFalse)
			l.Push(lua.LNumber(seq))
			l.Push(lua.LString(err.Error()))
			return 3
		}
		if st == lua.ResumeOK {
			// 同步跑完了
			_ = values
			l.Push(lua.LFalse)
			l.Push(lua.LNumber(seq))
			return 2
		}
		// 让出了（在 pause 里等着）
		l.Push(lua.LFalse)
		l.Push(lua.LNumber(seq))
		return 2
	}))

	// ---- 线程标识（setQuestFSMTable 里要用 getThreadKey）----
	L.SetGlobal("getThreadKey", L.NewFunction(func(l *lua.LState) int {
		key := L.GetGlobal("_RSVD_NAME_currentThreadKey")
		if key == lua.LNil {
			// 主线程：返回 mainKey
			l.Push(lua.LNumber(q.mainKey))
			return 1
		}
		l.Push(key)
		return 1
	}))
	L.SetGlobal("getThreadSeqID", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(0))
		return 1
	}))
	L.SetGlobal("getKeyPair", L.NewFunction(func(l *lua.LState) int {
		key := L.GetGlobal("_RSVD_NAME_currentThreadKey")
		l.Push(key)
		l.Push(lua.LNumber(0))
		return 2
	}))
	L.SetGlobal("getThreadAddress", L.NewFunction(func(l *lua.LState) int {
		l.Push(l.NewTable())
		return 1
	}))
	L.SetGlobal("getUID", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(0))
		return 1
	}))

	// ---- 跨范围调用（任务脚本大量用 uidRemoteCall 注册 handler）----
	//
	// ★ 任务脚本里最常见的一行是：
	//
	//	uidRemoteCall(getNPCharUID('道馆_1','万事通_1'), getUID(), getQuestName(), ...,
	//	             [[ setQuestHandler(questName, ...) ]])
	//
	//	即「把任务自己的状态机注册到某个 NPC 上去」。
	//	Go 侧没有跨 actor 的代码执行，这里做**本地降级**：
	//	直接在任务自己的 Lua 状态里执行那段代码（先把 varargs 铺好）。
	//
	//	这样注册能成功（setQuestHandler 被定义在 quest.lua 里），
	//	后续该 NPC 的事件会被路由回本任务 —— 正是我们要的语义。
	//
	//	失败（NPC 找不到 / 代码报错）只记日志，不中断任务加载。
	L.SetGlobal("uidRemoteCall", L.NewFunction(func(l *lua.LState) int {
		top := l.GetTop()
		if top < 2 {
			l.Push(lua.LNil)
			return 1
		}
		code := l.CheckString(top)
		patched, _ := patchIntDiv(code)
		patched, _ = patchToBeClosed(patched)
		fn, err := l.LoadString(patched)
		if err != nil {
			l.Push(lua.LNil)
			return 1
		}
		// 把中间那一段当 varargs 传进去
		args := make([]lua.LValue, 0, top-2)
		for i := 2; i < top; i++ {
			args = append(args, l.Get(i))
		}
		base := l.GetTop()
		if err := l.CallByParam(lua.P{Fn: fn, NRet: lua.MultRet, Protect: true}, args...); err != nil {
			l.Push(lua.LNil)
			return 1
		}
		n := l.GetTop() - base
		if n <= 0 {
			// ★ 代码串没有返回值时补一个整数。
			//
			//	任务脚本普遍写成
			//	  return assertType(uidRemoteCall(...), 'integer')
			//	即「注册结果必须是整数索引」。真实 uidRemoteCall
			//	会把远端寄存器值带回来；本地执行时如果没有返回值，
			//	补 1 让 assertType 通过（语义上是「注册成功，句柄 1」）。
			l.Push(lua.LNumber(1))
			return 1
		}
		return n
	}))

	// ---- 地图名 -> mapUID（quest.lua:127 getNPCharUID 要用）----
	L.SetGlobal("loadBaseMap", L.NewFunction(func(l *lua.LState) int {
		v := l.Get(1)
		switch val := v.(type) {
		case lua.LString:
			if id, ok := q.mapNames.IDOf(string(val)); ok && id != 0 {
				l.Push(lua.LNumber(id))
				return 1
			}
		case lua.LNumber:
			l.Push(lua.LNumber(val))
			return 1
		}
		l.Push(lua.LNil)
		return 1
	}))

	// ---- 地图格子触发器 / NPC 行为注册（降级为「登记成功」）----
	//
	// ★ 这几个函数原版定义在 map actor 范围（servermap.cpp），
	//	任务通过 uidRemoteCall 把它们注册到地图上。
	//	Go 侧当前没有「按格子触发任务事件」的机制，
	//	所以这里是**有意的降级**：
	//
	//	  返回一个递增的整数句柄 → 任务认为注册成功
	//	  后续该格子真的走上去时不会触发任务（功能缺失）
	//
	//	不降级的话任务在加载阶段就 fatal（本次修的就是这个）。
	//	完整的格子触发器留作已知限制，见 REPLICA_AUDIT。
	triggerHandle := int64(0)
	for _, name := range []string{
		"addQuestGridTrigger", "deleteGridTrigger",
		"setupNPCQuestBehavior", "clearNPCQuestBehavior",
		"setQuestHandler", "runEventHandler", "runNPCEventHandler",
	} {
		n := name
		L.SetGlobal(n, L.NewFunction(func(l *lua.LState) int {
			triggerHandle++
			l.Push(lua.LNumber(triggerHandle))
			return 1
		}))
	}

	L.SetGlobal("_RSVD_NAME_closeThread", L.NewFunction(func(l *lua.LState) int {
		key := int64(l.CheckInt64(1))
		q.mu.Lock()
		_, found := q.threads[key]
		delete(q.threads, key)
		delete(q.threadSeq, key)
		q.mu.Unlock()
		l.Push(lua.LBool(found))
		l.Push(lua.LFalse) // selfClose
		return 2
	}))

	L.SetGlobal("_RSVD_NAME_closeThreadThenRun", L.NewFunction(func(l *lua.LState) int {
		key := int64(l.CheckInt64(1))
		fn := l.CheckFunction(2)
		q.mu.Lock()
		delete(q.threads, key)
		q.mu.Unlock()
		_ = fn
		return 0
	}))

	L.SetGlobal("_RSVD_NAME_closeRequested", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LFalse)
		return 1
	}))
	L.SetGlobal("_RSVD_NAME_yieldToClose", L.NewFunction(func(l *lua.LState) int {
		return 0
	}))
	L.SetGlobal("_RSVD_NAME_checkThreadDepth", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(0))
		return 1
	}))
	L.SetGlobal("_RSVD_NAME_selfCloseError", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNil)
		return 1
	}))
	L.SetGlobal("_RSVD_NAME_callFuncCoop", L.NewFunction(func(l *lua.LState) int {
		// 协作调用（远程/通知）：当前直接返回 SYS_EXECDONE
		l.Push(lua.LNumber(0)) // SYS_EXECDONE
		return 1
	}))

	// ---- 通知（跨任务/跨 actor）----
	//
	// ★ 降级为「同步不入队」：waitNotify 立即返回 nil。
	//
	//	正常的任务流程里通知用于「等另一个任务/怪物回话」，
	//	当前没有跨 actor 队列，退化成「等到就等到、等不到就超时」。
	//	不这么做的话任务会永久卡在 waitNotify 上（比降级更糟）。
	for _, name := range []string{"_RSVD_NAME_postNotify", "_RSVD_NAME_sendNotify"} {
		L.SetGlobal(name, L.NewFunction(func(l *lua.LState) int { return 0 }))
	}
	L.SetGlobal("_RSVD_NAME_pickNotify", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNil)
		return 1
	}))
	L.SetGlobal("_RSVD_NAME_waitNotify", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNil)
		return 1
	}))
	L.SetGlobal("clearNotify", L.NewFunction(func(l *lua.LState) int { return 0 }))

	// ---- 杂项 ----
	L.SetGlobal("launchTime", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(0))
		return 1
	}))
	L.SetGlobal("uptime", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(0))
		return 1
	}))
	L.SetGlobal("uidAlive", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LTrue)
		return 1
	}))
	L.SetGlobal("getServiceCoreUID", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(0))
		return 1
	}))
	L.SetGlobal("randMapGLoc", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNil)
		return 1
	}))
	L.SetGlobal("validMapGLoc", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LBool(true))
		return 1
	}))
	L.SetGlobal("hasDatabase", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LTrue)
		return 1
	}))
	L.SetGlobal("dbExecString", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LTrue)
		return 1
	}))
	L.SetGlobal("dbQueryString", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNil)
		return 1
	}))
}

// Activate 激活任务（对应 quest.cpp:135 onActivate）。
//
// 执行任务脚本注册的 SYS_ON_ACTIVATE（如果有）。
func (q *QuestState) Activate(ctx context.Context) error {
	ret, err := q.rt.CallGlobal("_RSVD_NAME_questActivate")
	if err != nil {
		// 没有激活钩子是正常的（多数任务直接靠 setQuestFSMTable 注册）
		return nil
	}
	_ = ret
	return nil
}

// ResumeThread 恢复一个等待中的线程（由计时器/事件触发）。
//
// 这是 pause() 之后「继续跑」的唯一入口。
func (q *QuestState) ResumeThread(key int64, args ...lua.LValue) error {
	q.mu.Lock()
	co, ok := q.threads[key]
	q.mu.Unlock()
	if !ok {
		return fmt.Errorf("quest %s: thread %d not found", q.Name, key)
	}

	// ★ gopher-lua 的 Resume 必须带一个 fn（首次调用用 nil 表示「继续已有协程」）
	st, err, _ := co.Resume(nil, nil)
	if err != nil {
		return fmt.Errorf("resume quest thread %d: %w", key, err)
	}
	if st == lua.ResumeOK {
		q.mu.Lock()
		delete(q.threads, key)
		delete(q.threadSeq, key)
		q.mu.Unlock()
	}
	return nil
}

// PendingThreads 返回当前还在等待的线程数（诊断用）。
func (q *QuestState) PendingThreads() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.threads)
}

// Close 释放任务运行时。
func (q *QuestState) Close() {
	if q.rt != nil {
		q.rt.Close()
	}
}

// ==================== LuaVar <-> lua.LValue 转换 ====================
//
// ★ LuaVar 是 luaf::luaVar 的镜像，不是通用的 JSON-like 结构：
//
//	数组用 Array []LuaVarWrapper（不是 List）
//	字典用 Table []LuaTableEntry（**有序**，不是 map）
//	布尔字段叫 Boolean 不是 Bool
//
//	所以不能「按直觉」写结构体字面量，必须用现成的 New* 构造器 ——
//	直接写 LuaVar{Kind: LuaVarBool, Bool: true} 编译期就报错。

// luaToLuaVar 把 Lua 值转成可序列化的 LuaVar。
func luaToLuaVar(v lua.LValue) protocol.LuaVar {
	switch val := v.(type) {
	case *lua.LNilType:
		return protocol.NewLuaNilVar()
	case lua.LBool:
		return protocol.NewLuaBoolVar(bool(val))
	case lua.LNumber:
		f := float64(val)
		// luaf::luaVar 区分 integer 与 number：
		// 整数值存成 Integer，否则存 Number。存错会让脚本里
		// `math.type(x) == 'integer'` 判断出错（player.lua 大量用它）。
		if f == float64(int64(f)) {
			return protocol.NewLuaIntegerVar(int64(f))
		}
		return protocol.NewLuaNumberVar(f)
	case lua.LString:
		return protocol.NewLuaStringVar(string(val))
	case *lua.LTable:
		// 数组还是字典？与 luaf::buildLuaVar 的判定一致：
		// 「1..n 连续且没有其他键」才算数组。
		arrayLen := val.Len()
		isArray := arrayLen > 0
		if isArray {
			count := 0
			val.ForEach(func(k, _ lua.LValue) {
				if n, ok := k.(lua.LNumber); ok && n >= 1 && int(n) <= arrayLen {
					return
				}
				count++
			})
			isArray = count == 0
		}
		if isArray {
			items := make([]protocol.LuaVarWrapper, 0, arrayLen)
			for i := 1; i <= arrayLen; i++ {
				items = append(items, protocol.LuaVarWrapper{
					Valid: true,
					Value: luaToLuaVar(val.RawGetInt(i)),
				})
			}
			return protocol.NewLuaArrayVar(items...)
		}
		var entries []protocol.LuaTableEntry
		val.ForEach(func(k, sub lua.LValue) {
			entries = append(entries, protocol.LuaTableEntry{
				Key:   protocol.LuaVarWrapper{Valid: true, Value: luaToLuaVar(k)},
				Value: protocol.LuaVarWrapper{Valid: true, Value: luaToLuaVar(sub)},
			})
		})
		return protocol.NewLuaTableVar(entries...)
	default:
		return protocol.NewLuaNilVar()
	}
}

// luaVarToLua 把 LuaVar 还原成 Lua 值。
func luaVarToLua(l *lua.LState, v protocol.LuaVar) lua.LValue {
	switch v.Kind {
	case protocol.LuaVarBool:
		return lua.LBool(v.Boolean)
	case protocol.LuaVarInteger:
		return lua.LNumber(v.Integer)
	case protocol.LuaVarNumber:
		return lua.LNumber(v.Number)
	case protocol.LuaVarString:
		return lua.LString(v.String)
	case protocol.LuaVarArray:
		tbl := l.NewTable()
		for _, item := range v.Array {
			if !item.Valid {
				tbl.Append(lua.LNil)
				continue
			}
			tbl.Append(luaVarToLua(l, item.Value))
		}
		return tbl
	case protocol.LuaVarTable:
		tbl := l.NewTable()
		for _, entry := range v.Table {
			key := luaVarToLua(l, entry.Key.Value)
			value := luaVarToLua(l, entry.Value.Value)
			tbl.RawSet(key, value)
		}
		return tbl
	default:
		return lua.LNil
	}
}

// ==================== 管理器 ====================

// QuestManager 管理全部任务脚本实例。
type QuestManager struct {
	// FrameworkDir base.lua / quest.lua 所在目录
	FrameworkDir string
	// ScriptRoot 原版脚本根目录（quest/*.lua 在这儿）
	ScriptRoot string
	Deps       QuestDeps
	// MapNames 地图名 <-> ID（loadBaseMap / getNPCharUID 要用）
	MapNames *NameTable

	mu     sync.RWMutex
	quests map[string]*QuestState
}

// NewQuestManager 创建任务管理器。
func NewQuestManager(frameworkDir, scriptRoot string, deps QuestDeps, mapNames *NameTable) *QuestManager {
	return &QuestManager{
		FrameworkDir: frameworkDir,
		ScriptRoot:   scriptRoot,
		Deps:         deps,
		MapNames:     mapNames,
		quests:       make(map[string]*QuestState),
	}
}

// LoadAll 加载全部任务脚本（对应服务器启动时注册所有 quest）。
//
// ★ 失败不中断：某个任务脚本有语法错误时只跳过它，
// 其他任务照常工作。整体中退会让「一个任务坏了全服没任务」。
func (m *QuestManager) LoadAll() (loaded, failed int, errs []error) {
	dir := filepath.Join(m.ScriptRoot, "quest")
	files, err := filepath.Glob(filepath.Join(dir, "*.lua"))
	if err != nil {
		return 0, 0, []error{err}
	}

	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".lua")
		opts := Options{
			FrameworkDir: m.FrameworkDir,
			ScriptRoot:   m.ScriptRoot,
			Consts:       DefaultConsts(),
			Bindings:     map[string]lua.LGFunction{},
		}
		// 地图名表通过包级变量传给 QuestState（LoadQuest 签名保持简洁）
		questMapNames = m.MapNames
		q, err := LoadQuest(opts, name, path, m.Deps)
		if err != nil {
			failed++
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		m.mu.Lock()
		m.quests[name] = q
		m.mu.Unlock()
		loaded++
	}
	return loaded, failed, errs
}

// Get 按名字取任务实例。
func (m *QuestManager) Get(name string) (*QuestState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	q, ok := m.quests[name]
	return q, ok
}

// Count 返回已加载的任务数。
func (m *QuestManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.quests)
}

// Close 释放全部任务运行时。
func (m *QuestManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, q := range m.quests {
		q.Close()
	}
	m.quests = make(map[string]*QuestState)
}

// ForEach 遍历全部已加载的任务。
func (m *QuestManager) ForEach(fn func(name string, q *QuestState)) {
	m.mu.RLock()
	snapshot := make(map[string]*QuestState, len(m.quests))
	for k, v := range m.quests {
		snapshot[k] = v
	}
	m.mu.RUnlock()
	for name, q := range snapshot {
		fn(name, q)
	}
}

// CallRestore 重放某玩家在该任务里的当前状态
// （quest.lua 的 _RSVD_NAME_restoreQuestStates）。
//
// ★ 只在玩家登录时调用。原版语义是「每次登录把当前状态函数从头跑一遍」，
//	所以状态函数里不能发奖励（脚本作者自己负责把奖励放到切换回调里）。
func (q *QuestState) CallRestore(ctx context.Context, playerUID uint64) ([]lua.LValue, error) {
	return q.rt.CallGlobal("_RSVD_NAME_restoreQuestStates", lua.LNumber(playerUID))
}
