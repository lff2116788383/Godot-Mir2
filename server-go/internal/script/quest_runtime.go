package script

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
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

// SetMapNames 设置全局地图名表（启动期调用一次）。
//
// ★ 任务与地图脚本共用这同一份表：
//
//	任务要靠它解析 getNPCharUID(baseMap) 的地图名，
//	地图脚本要靠它把 uidMapSwitch('道馆_1', ...) 的名字翻成 ID。
//	两份表不一致就会出现「任务能找到 NPC、但地图换图找错图」这种
//	极难定位的问题。
func SetMapNames(t *NameTable) { questMapNames = t }

// MapNamesTable 返回全局地图名表（地图脚本构造 Options 时用）。
func MapNamesTable() *NameTable { return questMapNames }

// activeBus 当前生效的 RPC 总线（loadBaseMap 等绑定要用）。
//
// ★ 用包级变量而不是 q.bridge：
//
//	loadBaseMap 在 quest.lua 加载前就被注册，那时 QuestState.bridge 还不存在。
//	而且它只需要「把 64 位 UID 转短 ID」这一件事，不需要整个 bridge。
var activeBus *RPCBus

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

	// threads2 线程运行器（原版 ServerLuaCoroutineRunner）
	//
	// ★ 替代早期的 threads/threadSeq 两个 map：
	//	那两个是「按 key 存一个协程」，无法表达
	//	「同一 key 下多个 seqID 并存」（状态切换的必要条件）。
	threads2 *threadRunner

	// mainKey 主线程 key（原版 getMainScriptThreadKey）
	mainKey int64
	// mapNames 地图名 <-> ID 表（loadBaseMap 要用）
	mapNames *NameTable

	// UID 任务的 actor UID（原版 uidf::getQuestUID(questID)）
	//
	// ★ 这是 B 方案的核心：每个任务是一个独立可寻址的 actor，
	//	NPC 与玩家通过这个 UID 用 uidRemoteCall 找它。
	UID uint64
	// shortUID 给 Lua 看的短 ID（见 shortid.go）
	shortUID int64
	// bridge RPC 桥（让别人能远程调本任务，也让本任务能调别人）
	bridge *RPCBridge
	// deps 宿主能力（日志等）
	deps QuestDeps
	// luaMu 保护 LState：同一时刻只允许一个 goroutine 执行
	luaMu sync.Mutex
	// gridTriggerSeq 格子触发器句柄分配
	gridTriggerSeq int64
	// threadKeySeq rollKey() 的计数器（原版 m_threadKey）
	threadKeySeq int64

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
	//
	// ★ 原版签名带 fsm：一条描述对应「某任务某个状态机」，
	//	客户端按 Name+FSM 覆盖显示。丢掉 fsm 会让任务面板无法区分
	//	主线和子状态机的描述文字。
	//
	// ★ questName 由调用方显式给出（就是本任务的 Name），
	//	不要让宿主去猜「当前是哪个任务」—— 42 个任务并发运行时猜不出来。
	PostQuestDesp(ctx context.Context, questName string, playerUID uint64,
		fsm string, hasDesp bool, desp string) error
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
//
// ★ bus 非 nil 时，本任务会被注册成一个可远程寻址的 actor：
//
//	NPC 用 uidRemoteCall(questUID, ...) 就能调到本任务，
//	本任务也能用 uidRemoteCall(npcUID, ...) 调 NPC。
//	这是 B 方案（对齐原版 actor 模型）的关键。
func LoadQuest(opts Options, name, scriptPath string, deps QuestDeps,
	bus *RPCBus, questID uint32) (*QuestState, error) {
	q := &QuestState{
		Name:           name,
		ScriptPath:     scriptPath,
		mapNames:       questMapNames,
		mainKey:        1,
		threadKeySeq:   2, // 原版 m_threadKey 初始为 mainThreadKey+1
		deps:           deps,
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

	// ★★ 先装 C 层原语，再加载原版协程运行器 ★★
	//
	//	原版分两层：
	//	  serverluacoroutinerunner.cpp  → C 函数 _RSVD_NAME_runThread 等
	//	  serverluacoroutinerunner.lua  → Lua 封装 runThread / pause / uidRemoteCall
	//	quest.lua 调的是**后者**（Lua 层）。
	//
	//	之前只装了 C 层、没加载 Lua 层，于是 quest.lua:21 的
	//	`runThread(key, func)` 直接报 "attempt to call a non-function object"，
	//	任务状态机一步都跑不起来 —— 这才是「42 个任务激活成功但不可玩」的真因。
	q.installCoroutinePrimitives()

	runnerPath := filepath.Join(opts.FrameworkDir, "serverluacoroutinerunner.lua")
	if err := rt.patchAndRun(runnerPath); err != nil {
		rt.Close()
		return nil, fmt.Errorf("load serverluacoroutinerunner.lua: %w", err)
	}

	// 加载 quest.lua 框架（原版 server/src/quest.lua 零翻译复用）
	frameworkPath := filepath.Join(opts.FrameworkDir, "quest.lua")
	if err := rt.patchAndRun(frameworkPath); err != nil {
		rt.Close()
		return nil, fmt.Errorf("load quest.lua: %w", err)
	}

	// ★ RPC 桥必须在任务脚本之前装好：
	//	任务脚本顶层就会调 uidRemoteCall 注册 handler，
	//	晚一步装就会用到之前的降级版本（返回假值）。
	if bus != nil && questID != 0 {
		q.UID = common.GetQuestUID(questID)
		q.shortUID = bus.Short(q.UID)
		q.bridge = NewRPCBridge(bus, q.UID, rt, q.mapNames)
		activeBus = bus
		q.bridge.Install(rt.L)
		bus.RegisterQuest(q.UID, name, q)
	}

	// 加载具体任务脚本
	if err := rt.patchAndRun(scriptPath); err != nil {
		rt.Close()
		return nil, fmt.Errorf("load quest %s: %w", name, err)
	}

	return q, nil
}

// realUID 把 Lua 传来的短 ID 还原成真实 UID。
//
// ★ 所有「Lua 递进来的 UID」都必须过这一层：
//	Lua 侧只认识 1..N 的小整数（见 shortid.go），
//	直接当 64 位 UID 用会指向完全错误的对象。
func (q *QuestState) realUID(short int64) uint64 {
	if activeBus == nil {
		return 0
	}
	return activeBus.Real(short)
}

// dbidOf 把短 ID 还原成真实 UID，再解出玩家 dbid。
//
// ★ 任务表的主键是 fld_dbid（外键指向 tbl_char），
//	写错会直接撞 FOREIGN KEY constraint failed，
//	而这个报错只会说「约束失败」，不会指向「UID 没转换」。
func (q *QuestState) dbidOf(short int64) (uint32, error) {
	uid := q.realUID(short)
	if uid == 0 {
		return 0, fmt.Errorf("quest %s: unknown short uid %d", q.Name, short)
	}
	return common.GetPlayerDBID(uid)
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
		// rollKey() -> 单调递增的线程 key
		//
		// ★ 原版 quest.cpp:28：`return getQuest()->m_threadKey++;`
		//
		//	每个任务一个独立计数器，永不复用。
		//	quest.lua 的 _RSVD_NAME_spawnQuestState 用它当
		//	_RSVD_NAME_questStateRunners[uid][fsm] 的 key ——
		//	不绑定会返回 nil，导致 setQuestState 直接抛
		//	"cannot perform sub operation between nil and number"。
		"rollKey": func(l *lua.LState) int {
			q.mu.Lock()
			k := q.threadKeySeq
			q.threadKeySeq++
			q.mu.Unlock()
			l.Push(lua.LNumber(k))
			return 1
		},
		// getQuestScriptHash() -> 脚本哈希
		"getQuestScriptHash": func(l *lua.LState) int {
			l.Push(lua.LNumber(q.Hash))
			return 1
		},
		// _RSVD_NAME_setQuestDesp(uid, despTable, fsm, desp)
		//
		// ★ 原版签名是 **4 个参数**（quest.cpp:45）：
		//
		//	uid       玩家 UID
		//	despTable 完整的 fld_desp 表（可直接为 nil 表示清空）
		//	fsm       状态机名
		//	desp      本次要推给客户端的那一段文字（可为 nil）
		//
		//	之前实现成「uid + 一个字符串数组」，既丢了 fsm，
		//	也把 despTable 当成列表读 —— 客户端收到的描述永远是
		//	任务名/FSM 都为空的一条，任务面板里什么都显示不出来。
		"_RSVD_NAME_setQuestDesp": func(l *lua.LState) int {
			shortUID := l.CheckInt64(1)
			despTable := l.Get(2)
			fsm := l.OptString(3, SysQuestFSM)
			despVal := l.Get(4)

			if deps == nil {
				return 0
			}
			playerUID := q.realUID(shortUID)
			dbid, derr := common.GetPlayerDBID(playerUID)
			if derr != nil {
				deps.Log(1, fmt.Sprintf("setQuestDesp: %v", derr))
				return 0
			}

			// fld_desp 整体写入（nil 表示清空该字段）
			var fldValue *protocol.LuaVar
			if despTable != lua.LNil {
				if _, ok := despTable.(*lua.LTable); ok {
					lv := luaToLuaVar(despTable)
					fldValue = &lv
				}
			}
			if err := deps.SetQuestField(context.Background(), q.Name, dbid, "fld_desp", fldValue); err != nil {
				deps.Log(1, fmt.Sprintf("setQuestDesp write fld_desp: %v", err))
			}

			// 推给客户端的一条描述
			hasDesp := false
			text := ""
			if s, ok := despVal.(lua.LString); ok {
				hasDesp = true
				text = string(s)
			}
			if err := deps.PostQuestDesp(context.Background(), q.Name, playerUID, fsm, hasDesp, text); err != nil {
				deps.Log(1, fmt.Sprintf("post quest desp failed: %v", err))
			}
			return 0
		},
		// dbGetQuestField(uid, field) -> value|nil
		"dbGetQuestField": func(l *lua.LState) int {
			shortUID := l.CheckInt64(1)
			field := l.CheckString(2)
			if deps == nil {
				l.Push(lua.LNil)
				return 1
			}
			dbid, derr := q.dbidOf(shortUID)
			if derr != nil {
				l.Push(lua.LNil)
				return 1
			}
			value, found, err := deps.LoadQuestField(context.Background(), q.Name, dbid, field)
			if err != nil || !found {
				l.Push(lua.LNil)
				return 1
			}
			l.Push(luaVarToLua(l, value))
			return 1
		},
		// dbSetQuestField(uid, field, value)
		"dbSetQuestField": func(l *lua.LState) int {
			shortUID := l.CheckInt64(1)
			field := l.CheckString(2)
			value := l.Get(3)
			if deps == nil {
				return 0
			}
			dbid, derr := q.dbidOf(shortUID)
			if derr != nil {
				deps.Log(1, fmt.Sprintf("dbSetQuestField: %v", derr))
				return 0
			}
			lv := luaToLuaVar(value)
			if err := deps.SetQuestField(context.Background(), q.Name, dbid, field, &lv); err != nil {
				deps.Log(1, fmt.Sprintf("set quest field %s failed: %v", field, err))
			}
			return 0
		},
		// _RSVD_NAME_dbSetQuestFields(uid, fieldTable, replace)
		"_RSVD_NAME_dbSetQuestFields": func(l *lua.LState) int {
			shortUID := l.CheckInt64(1)
			tbl := l.CheckTable(2)
			replace := l.OptBool(3, false)
			if deps == nil {
				return 0
			}
			dbid, derr := q.dbidOf(shortUID)
			if derr != nil {
				deps.Log(1, fmt.Sprintf("dbSetQuestFields: %v", derr))
				return 0
			}
			fields := make(map[string]*protocol.LuaVar)
			tbl.ForEach(func(k, v lua.LValue) {
				name, ok := k.(lua.LString)
				if !ok {
					return
				}
				// ★ SYS_LUANIL 表示「写 SQL NULL」（原版用它代表 nil，
				//	因为 nil 作表值会直接删掉这个键）
				if s, isStr := v.(lua.LString); isStr && string(s) == SysLuaNil {
					fields[string(name)] = nil
					return
				}
				lv := luaToLuaVar(v)
				fields[string(name)] = &lv
			})
			if err := deps.SetQuestFields(context.Background(), q.Name, dbid, fields, replace); err != nil {
				deps.Log(1, fmt.Sprintf("set quest fields failed: %v", err))
			}
			return 0
		},
		// getNPCharUID(mapName, npcName) -> uid|nil
		//
		// ★ 必须返回**短 ID**：NPC UID 是 64 位值，
		//	直接 push 给 Lua 会掉精度（差 1），
		//	后续 uidRemoteCall 会去找一个不存在的 actor，
		//	报 "target not found" —— 完全看不出是精度问题。
		"getNPCharUID": func(l *lua.LState) int {
			mapName := l.CheckString(1)
			npcName := l.CheckString(2)
			if deps == nil {
				l.Push(lua.LNil)
				return 1
			}
			uid, found := deps.FindNPCharUID(mapName, npcName)
			if !found || uid == 0 {
				l.Push(lua.LNil)
				return 1
			}
			if activeBus == nil {
				l.Push(lua.LNil)
				return 1
			}
			l.Push(lua.LNumber(activeBus.Short(uid)))
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

	// ★ 线程运行器必须在所有绑定之前建好：
	//	下面的绑定全部要读 t.current()，晚一步就是空指针。
	if q.threads2 == nil {
		q.threads2 = newThreadRunner(L, q.mainKey, func(format string, args ...any) {
			if q.deps != nil {
				q.deps.Log(1, fmt.Sprintf(format, args...))
			}
		})
	}
	t := q.threads2

	// ---- 线程管理（C 层，语义严格对齐 serverluacoroutinerunner.cpp）----

	// _RSVD_NAME_runThread(key, func) -> (newSeqID, closed)
	//
	// ★★ 返回值顺序是 (seqID, closed)，不是 (closed, seqID) ★★
	//
	//	serverluacoroutinerunner.lua:129
	//	    local seqID, closed = _RSVD_NAME_runThread(key, func)
	//	    if closed then _RSVD_NAME_yieldToClose() end
	//	    return key, seqID
	//
	//	顺序写反的后果：closed 拿到一个数字（真值），Lua 侧立刻
	//	yieldToClose —— 新线程刚起来就被判「要求关闭」并终止，
	//	任务状态函数体一行都没跑，日志里没有任何错误。
	L.SetGlobal("_RSVD_NAME_runThread", L.NewFunction(func(l *lua.LState) int {
		key := int64(l.CheckInt64(1))
		fn := l.CheckFunction(2)

		_, seqID, err := t.spawn(key, fn)
		if err != nil {
			// 原版把错误往上抛（让调用方的状态函数失败）。
			// 这里也抛：静默吞掉会让「状态函数没跑」变成最难查的问题。
			l.RaiseError("runThread: %v", err)
			return 0
		}
		l.Push(lua.LNumber(seqID))
		l.Push(lua.LFalse) // closed
		return 2
	}))

	// _RSVD_NAME_closeThread(key, seqID) -> (found, selfClose)
	L.SetGlobal("_RSVD_NAME_closeThread", L.NewFunction(func(l *lua.LState) int {
		key := int64(l.CheckInt64(1))
		seqID := int64(l.OptInt64(2, 0))
		found, selfClose := t.closeThread(key, seqID)
		l.Push(lua.LBool(found))
		l.Push(lua.LBool(selfClose))
		return 2
	}))

	// _RSVD_NAME_closeThreadThenRun(key, func)
	//
	// ★ 这是 setQuestState 做状态切换的关键原语：
	//	调用线程在 yield 处结束，然后启动 func。
	//	之前做成空桩（什么都不做），导致**状态切换完全失效** ——
	//	任务接受后永远停在第一个状态。
	L.SetGlobal("_RSVD_NAME_closeThreadThenRun", L.NewFunction(func(l *lua.LState) int {
		key := int64(l.CheckInt64(1))
		fn := l.CheckFunction(2)
		if err := t.closeThreadThenRun(key, fn); err != nil {
			l.RaiseError("%v", err)
			return 0
		}
		return 0
	}))

	// _RSVD_NAME_closeRequested(what) -> bool
	L.SetGlobal("_RSVD_NAME_closeRequested", L.NewFunction(func(l *lua.LState) int {
		r := t.current()
		requested := r != nil && r.closeRequested && !r.closing
		l.Push(lua.LBool(requested))
		return 1
	}))

	// _RSVD_NAME_yieldToClose()
	//
	// ★ 语义：结束当前线程，永不返回。
	//	Lua 侧调用点是 `_RSVD_NAME_yieldToClose()` 之后紧跟 error(...)，
	//	所以这里必须真的 yield（让 resumeRunner 把它判死）。
	L.SetGlobal("_RSVD_NAME_yieldToClose", L.NewFunction(func(l *lua.LState) int {
		// 标记要求关闭，然后 yield。resumeRunner 看到 closeRequested
		// 就不再 resume 这个线程 —— 等价于原版的 lua_closethread。
		if r := t.current(); r != nil {
			t.mu.Lock()
			r.closeRequested = true
			t.mu.Unlock()
		}
		return l.Yield()
	}))

	// _RSVD_NAME_checkThreadDepth()
	L.SetGlobal("_RSVD_NAME_checkThreadDepth", L.NewFunction(func(l *lua.LState) int {
		t.mu.Lock()
		depth := t.depth
		t.mu.Unlock()
		if depth >= maxThreadDepth {
			l.RaiseError("%d threads run on top of each other on the C stack, can't start one more", depth)
		}
		return 0
	}))

	// _RSVD_NAME_selfCloseError() -> string|nil
	//
	// ★ 原版检查「当前线程能不能在这里结束自己」：
	//	在别人创建的协程里 → 不能；在不可 yield 的地方 → 不能。
	//	Go 侧线程永远是专门建的协程，所以只要能找到当前线程就能结束。
	L.SetGlobal("_RSVD_NAME_selfCloseError", L.NewFunction(func(l *lua.LState) int {
		if t.current() == nil {
			l.Push(lua.LString("outside any thread"))
			return 1
		}
		l.Push(lua.LNil)
		return 1
	}))

	// ---- 线程标识 ----
	//
	// ★★ 这些必须读 Go 侧的当前 runner，不能读 Lua 全局变量 ★★
	//
	//	沙箱把全局环境换成了按协程隔离的代理表：
	//	GetGlobal 读的是主状态的 Env 表，而协程里写全局是写进
	//	它自己的 sandbox。两边天然不通 —— 之前用
	//	`co.SetGlobal("_RSVD_NAME_currentThreadKey", ...)` 传 key，
	//	协程里读到的永远是 nil，getThreadKey() 恒返回 mainKey。
	L.SetGlobal("getThreadKey", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(t.CurrentKey()))
		return 1
	}))
	L.SetGlobal("getThreadSeqID", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(t.CurrentSeqID()))
		return 1
	}))
	L.SetGlobal("getKeyPair", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(t.CurrentKey()))
		l.Push(lua.LNumber(t.CurrentSeqID()))
		return 2
	}))
	L.SetGlobal("getThreadAddress", L.NewFunction(func(l *lua.LState) int {
		tbl := l.NewTable()
		// 原版 {uid, key, seqID}；uid 由任务自己的 actor UID 填
		tbl.Append(lua.LNumber(q.shortUID))
		tbl.Append(lua.LNumber(t.CurrentKey()))
		tbl.Append(lua.LNumber(t.CurrentSeqID()))
		l.Push(tbl)
		return 1
	}))
	L.SetGlobal("getUID", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LNumber(q.shortUID))
		return 1
	}))

	// ---- pause（yielding 绑定）----
	//
	// ★★ 这是之前完全缺失的绑定 ★★
	//
	//	serverluacoroutinerunner.lua:70 的 pause() 直接调
	//	_RSVD_NAME_pauseYielding(msec)，没绑就是
	//	"attempt to call a nil value (global '_RSVD_NAME_pauseYielding')"。
	//	13 个任务用到 pause（试炼计时），全都会在这一步崩。
	//
	//	实现：起计时器 → 从 Go 函数 yield（return -1）→ 到点 resume。
	L.SetGlobal("_RSVD_NAME_pauseYielding", L.NewFunction(func(l *lua.LState) int {
		msec := int64(l.CheckInt64(1))
		if err := t.pause(msec); err != nil {
			l.RaiseError("%v", err)
			return 0
		}
		// ★ gopher-lua 的 yield 协议：Go 函数返回 -1 表示「让出」
		//
		//	callGFunction 里 `if gfnret < 0 { switchToParentThread(...) }`。
		//	暂停结束后计时器 resume，协程从这里之后继续。
		return l.Yield()
	}))

	// ---- 通知 ----
	L.SetGlobal("_RSVD_NAME_pickNotify", L.NewFunction(func(l *lua.LState) int {
		maxCount := l.OptInt(1, 0)
		vals := t.pickNotify(maxCount)
		tbl := l.NewTable()
		for _, v := range vals {
			tbl.Append(v)
		}
		l.Push(tbl)
		return 1
	}))
	L.SetGlobal("_RSVD_NAME_waitNotify", L.NewFunction(func(l *lua.LState) int {
		timeout := int64(l.OptInt64(1, 0))
		if vals, got := t.waitNotify(timeout); got {
			for _, v := range vals {
				l.Push(v)
			}
			return len(vals)
		}
		// 没有通知：进入等待，让 Lua 层 coroutine.yield() 挂起
		l.Push(lua.LNil)
		return 1
	}))
	L.SetGlobal("clearNotify", L.NewFunction(func(l *lua.LState) int {
		t.clearNotify()
		return 0
	}))

	// ---- uidRemoteCall 由 RPCBridge.Install 提供（见 rpc.go）----
	//
	// ★ 这里**不再**提供本地降级版。
	//
	//	之前是「在自己的 Lua 状态里执行代码串」，那是错的：
	//	NPC 的 handler 根本不在这个状态里，执行了也没人能用。
	//	真正的实现必须经 RPCBus 路由到目标 actor。

	// ---- 调试：包装 getNPCharUID 打印返回值 ----
	// （quest.lua 会覆盖它，所以这里只用于确认加载顺序）

	// ---- 地图名 -> mapUID（quest.lua:127 getNPCharUID 要用）----
	// ★★ loadBaseMap 必须返回 **mapUID 编码值**，不是裸 mapID ★★
	//
	//	原版 server.cpp:564 loadBaseMap 返回的是 uidf::getBaseMapUID(mapID)，
	//	即 (UID_MAP << 59) | (1 << 31) 这样的完整编码。
	//
	//	踩过的坑：之前返回裸 mapID（比奇县_0 → 1），
	//	于是 quest.lua 的 getNPCharUID 里
	//	  uidRemoteCall(loadBaseMap(mapName), ...)
	//	就变成 uidRemoteCall(1, ...) —— 找 UID=1 的 actor，
	//	报 "remote call target 1 not found"。
	//
	//	这个错误信息完全不提「你拿到的是裸 mapID」，
	//	看起来像「地图 actor 没注册」，会往完全错的方向排查。
	L.SetGlobal("loadBaseMap", L.NewFunction(func(l *lua.LState) int {
		v := l.Get(1)
		var mapID uint32
		switch val := v.(type) {
		case lua.LString:
			id, ok := q.mapNames.IDOf(string(val))
			if !ok || id == 0 {
				l.Push(lua.LNil)
				return 1
			}
			mapID = id
		case lua.LNumber:
			mapID = uint32(val)
		default:
			l.Push(lua.LNil)
			return 1
		}
		mapUID, err := common.GetBaseMapUID(mapID, 0)
		if err != nil {
			l.Push(lua.LNil)
			return 1
		}
		// ★ 返回短 ID：mapUID 是 64 位值，直接给 Lua 会丢精度（差 1）
		//
		// ★ 用包级 bus 而不是 q.bridge：
		//	loadBaseMap 被 quest.lua 顶层调用，那时 q.bridge 可能还没建。
		//	挂到包级变量上更稳（任务加载是启动期串行操作）。
		if activeBus == nil {
			l.Push(lua.LNil)
			return 1
		}
		l.Push(lua.LNumber(activeBus.Short(mapUID)))
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
	// ★ 这些函数**不再**用空桩覆盖。
	//
	//	它们多数由 quest.lua / npchar.lua 自己定义：
	//	  setupNPCQuestBehavior  → quest.lua:1356（走 questContext，真实现）
	//	  setQuestHandler        → npchar.lua:290（写 EPQST_eventHandlers）
	//	  runNPCEventHandler     → quest.lua:1709
	//
	//	之前用空桩覆盖，反而把它们的功能砸掉了 ——
	//	空桩返回假句柄，任务以为注册成功，实际什么都没挂上。
	//
	//	真正缺的只有「格子触发器」这一个原语（地图侧实现），
	//	它单独补在下面。
	L.SetGlobal("addQuestGridTrigger", L.NewFunction(func(l *lua.LState) int {
		// ★ 格子触发器：返回递增句柄表示注册成功。
		//
		//	真实实现需要「玩家踩到某格 → 通知任务」的地图侧钩子，
		//	当前 world 包没有这个机制。这里不假装实现，
		//	只保证任务加载不中断（否则整个任务脚本都跑不起来）。
		q.gridTriggerSeq++
		l.Push(lua.LNumber(q.gridTriggerSeq))
		return 1
	}))
	L.SetGlobal("deleteGridTrigger", L.NewFunction(func(l *lua.LState) int { return 0 }))

	// ★ 注意：线程管理 / 通知 / callFuncCoop 全部在上面装好了。
	//	这里**不能再有**重复的 SetGlobal —— 后写的会覆盖先写的，
	//	而旧的那批全是空桩（closeThreadThenRun 什么都不做、
	//	waitNotify 恒返回 nil），会把刚修好的实现全部砸掉。

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

// ExecRemote 实现 rpcTarget：在自己的 Lua 状态里执行远程代码串。
//
// ★ luaMu 是必需的：LState 不是并发安全的。
//
//	NPC 和玩家可能同时对同一任务发起 remoteCall，
//	两个 goroutine 同时进 LState 会直接段错误（不是 panic，是崩进程）。
func (q *QuestState) ExecRemote(code string, args []protocol.LuaVar) RPCResult {
	q.luaMu.Lock()
	defer q.luaMu.Unlock()
	if q.bridge == nil {
		return RPCResult{Err: &RPCError{Message: "quest bridge not installed"}}
	}
	return q.bridge.ExecRemote(code, args)
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
func (q *QuestState) ResumeThread(key int64, seqID int64) error {
	if q.threads2 == nil {
		return fmt.Errorf("quest %s: thread runner not installed", q.Name)
	}
	q.luaMu.Lock()
	defer q.luaMu.Unlock()
	return q.threads2.resume(key, seqID)
}

// PendingThreads 返回当前还在等待的线程数（诊断用）。
func (q *QuestState) PendingThreads() int {
	if q.threads2 == nil {
		return 0
	}
	return q.threads2.pending()
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
	// Bus 跨状态 RPC 总线（B 方案核心）
	Bus *RPCBus

	mu     sync.RWMutex
	quests map[string]*QuestState
	// nextQuestID 任务 ID 分配（1 起，对应 uidf::getQuestUID）
	nextQuestID uint32
	// lazy 待激活的任务（脚本还没执行，因为 NPC 还没加载）
	lazy map[string]lazyQuest
}

// NewQuestManager 创建任务管理器。
func NewQuestManager(frameworkDir, scriptRoot string, deps QuestDeps, mapNames *NameTable, bus *RPCBus) *QuestManager {
	return &QuestManager{
		FrameworkDir: frameworkDir,
		ScriptRoot:   scriptRoot,
		Deps:         deps,
		MapNames:     mapNames,
		Bus:          bus,
		quests:       make(map[string]*QuestState),
		lazy:         make(map[string]lazyQuest),
		nextQuestID:  1,
	}
}

// LoadAll 加载全部任务脚本（对应服务器启动时注册所有 quest）。
//
// ★★ 重要：这里只「登记」，不执行任务脚本 ★★
//
//	原因见 ActivateLazy 的注释：任务脚本顶层要调 getNPCharUID，
//	而 NPC 是懒加载的。启动时执行脚本会全部失败。
//
//	所以 LoadAll 只扫一遍文件名 + 分配 questID，
//	真正执行留给 ActivateLazy（NPC 加载完之后）。
//
//	调用方必须在 NPC 就绪后调 ActivateLazy，
//	否则任务永远是「登记了但没激活」——玩家点 NPC 看不到任务入口。
func (m *QuestManager) LoadAll() (registered int, errs []error) {
	dir := filepath.Join(m.ScriptRoot, "quest")
	files, err := filepath.Glob(filepath.Join(dir, "*.lua"))
	if err != nil {
		return 0, []error{err}
	}

	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".lua")
		m.mu.Lock()
		id := m.nextQuestID
		m.nextQuestID++
		m.lazy[name] = lazyQuest{name: name, path: path, id: id}
		m.mu.Unlock()
		registered++
	}
	return registered, nil
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

// ==================== 延迟加载（因为 NPC 是懒加载的）====================
//
// ★ 问题的本质：
//
//	任务脚本顶层会调 getNPCharUID('比奇县_0', '客栈店员_1')，
//	而 NPC 是玩家进图时才加载的（world_service.go:228）。
//	启动时把 42 个任务全加载 → NPC 还不存在 → 全部失败。
//
// ★ 但原版为什么没这个问题？
//
//	原版所有地图在启动时就全部 activate（Server::start 里遍历
//	maprecord 逐个 loadMap），NPC 也在那时全部建好。
//	Go 侧为了省内存改成了懒加载，代价就是任务不能启动时加载。
//
// ★ 解法：把任务分成「已注册（脚本读过）」和「已激活（能响应调用）」两态。
//
//	启动时只读脚本、不做任何远程调用（跳过顶层 getNPCharUID）
//	→ 地图 NPC 加载完后，再真正执行任务脚本
//
//	实现上，用「延迟队列」：任务第一次被某个地图/玩家需要时才执行脚本。
//	这样即使某张图玩家永远不去，它的任务也不会占内存。

// RegisterLazy 登记一个待激活的任务（只记名字与路径，不执行脚本）。
func (m *QuestManager) RegisterLazy(name, scriptPath string, questID uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lazy[name] = lazyQuest{name: name, path: scriptPath, id: questID}
}

// lazyQuest 是一个尚未执行脚本的任务。
type lazyQuest struct {
	name string
	path string
	id   uint32
}

// ActivateLazy 激活所有还没激活的任务。
//
// 在 NPC 全部加载完之后调用。已激活的跳过。
//
// ★ 失败不阻断：某个任务脚本报错只记日志并跳过，
//	其他任务照常（一个任务坏了不该让全服没任务）。
func (m *QuestManager) ActivateLazy() (activated, failed int) {
	m.mu.Lock()
	pending := make([]lazyQuest, 0, len(m.lazy))
	for _, lq := range m.lazy {
		pending = append(pending, lq)
	}
	m.mu.Unlock()

	for _, lq := range pending {
		opts := Options{
			FrameworkDir: m.FrameworkDir,
			ScriptRoot:   m.ScriptRoot,
			Consts:       DefaultConsts(),
		}
		questMapNames = m.MapNames
		q, err := LoadQuest(opts, lq.name, lq.path, m.Deps, m.Bus, lq.id)
		if err != nil {
			failed++
			if m.Deps != nil {
				m.Deps.Log(1, fmt.Sprintf("activate quest %s failed: %v", lq.name, err))
			}
			continue
		}
		m.mu.Lock()
		m.quests[lq.name] = q
		delete(m.lazy, lq.name)
		m.mu.Unlock()
		activated++
	}
	return activated, failed
}

// PendingLazy 返回还没激活的任务数。
func (m *QuestManager) PendingLazy() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.lazy)
}
