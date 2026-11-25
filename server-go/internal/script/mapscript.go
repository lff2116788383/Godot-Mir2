package script

import (
	"fmt"
	"math/rand"
	"os"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// ==================== 地图脚本（server/src/servermap.cpp:88-560）====================
//
// 地图脚本负责「这张图上放什么」：
//
//	addSafeZone(SYS_SAFEZONE, {{x=423,y=102,r=30}})   安全区
//	addGuard('大刀卫士', 371, 160, DIR_DOWNLEFT)      守卫
//	local addMonCo = addmon.monGener({...})           刷怪配置
//	coroutine.resume(addMonCo)                        启动刷怪协程
//
// 最后那两行是关键：`monGener` 返回一个**协程**，脚本自己 resume 它，
// 协程里是个 `while true do ... end` 循环，每轮检查「该刷怪了吗」。
// 原版靠 actor 定时器周期性地 resume 这个协程。
//
// ★ 这个设计决定了 Go 侧的实现方式：
//
//	不能在加载脚本时跑完就完事 —— 那个 while true 会永远卡住加载。
//	必须把返回的协程**存下来**，由世界时钟（MapTicker）定期 resume。
//	见 MapScript.Tick()。

// MapScriptHost 是地图脚本需要的宿主能力。
//
// 与 NPCHost 分开：地图脚本不接触任何玩家——除了换图，
// 换图必须由宿主真正把玩家挪过去（脚本只表达意图）。
type MapScriptHost interface {
	// MapID 当前地图 ID
	MapID() uint32
	// MapName 当前地图名（getMapName 用）
	MapName() string
	// MapSize 地图尺寸（w, h）
	MapSize() (int, int)
	// CanThrough 某格是否可走（randGLoc 用）
	CanThrough(x, y int) bool
	// CountThrough 统计矩形区域内可走格数（countGLoc 用）
	//
	// ★ 为什么单独一个批量方法而不是在 Lua 里循环调 CanThrough：
	//
	//	道馆_1 的刷怪配置是 20 种怪 × 285 个 38×38 区域 = 41 万格。
	//	逐格回调 CanThrough 时，光跨 Lua↔Go 边界就要 30 秒
	//	（gopher-lua 走 AST 解释，比 C Lua 慢一个数量级）。
	//	批量一次算完，耗时降到毫秒级。
	CountThrough(x, y, w, h int) int

	// AddMonster 刷一只怪，返回 UID（0 表示失败）
	AddMonster(name string, x, y int, strictLoc bool) uint64
	// AddGuard 放一个守卫，返回 UID
	AddGuard(name string, x, y int, direction int) uint64
	// AddSafeZone 加安全区（x,y,r 都是格坐标）
	AddSafeZone(x, y, radius int)
	// Alive 判断 UID 是否还活着（uidAlive）
	Alive(uid uint64) bool

	// GridSwitchDest 查询某格配置的换图目的地（hasGridSwitchDest 用）。
	//
	// ★ 数据来源是 maprecord.inc 的 switchList：
	//	控制台里「走到门口自动进下一张图」就是靠这张表。
	//	返回 false 表示这格不是门口。
	GridSwitchDest(x, y int) (endMapID uint32, endX, endY int, ok bool)

	// SwitchPlayer 把玩家传到指定地图坐标（uidGridMapSwitch / uidMapSwitch）。
	//
	// ★ 脚本只表达「应该传走」，真正搬运由宿主做：
	//	换图要处理「离开旧图 AOI → 进入新图 AOI → 推地图信息给客户端」，
	//	那是世界层的职责，脚本层拿不到这些。
	SwitchPlayer(playerUID uint64, mapID uint32, x, y int) bool

	// Log 写日志
	Log(level int, msg string)
}

// MapScript 是一张地图的脚本实例。
type MapScript struct {
	Runtime *Runtime
	Host    MapScriptHost
	MapName string

	// mainCo 地图脚本的 main() 协程。
	//
	// ★ 地图脚本的约定是「定义 main() 函数，由宿主驱动」：
	//
	//	function main()
	//	    while true do
	//	        local rc = coroutine.resume(addMonCo)
	//	        pause(1000 * 5)      -- 让出，5 秒后继续
	//	    end
	//	end
	//
	//	原版在 actor 里把 main() 当协程跑，pause() 里 yield，
	//	定时器到点再 resume。Go 侧同样：mainCo 是协程，
	//	Tick 里检查 pause 到期后 resume。
	mainCo *lua.LState
	// pauseUntilMS pause() 的到期时刻（世界时钟）
	pauseUntilMS int64
	// glocCache countGLoc 的结果缓存，key = (x,y,w,h)
	//
	// 一张图的刷怪配置在加载时会把同一批区域反复扫很多遍
	// （每种怪都有自己的 loc 列表，但区域大量重叠）。
	// 缓存把这部分从 49 万次降到实际不同的区域数。
	glocCache map[[4]int]int

	// nowMS 当前世界时钟（Tick/Start 更新，luaPause 读它算到期时刻）
	nowMS int64
	// started 是否已启动 main 协程
	started bool
	// finished main 协程是否已跑完
	finished bool

	// gridTriggers 格子触发器表（servermap.lua 用）
	//
	// ★ 一张地图一个：触发器是「属于这张图」的，
	//	不同图上的同坐标格互不影响。
	gridTriggers *gridTriggerStore

	// threads 地图 Lua 状态的线程运行器（runMapThread 用）
	threads *threadRunner
	// safeZones 安全区表（servermap.lua 的 addSafeZone 用）
	safeZones *safeZoneStore
	// runtimeMu 保护 LState（RPC 与 Tick 可能并发进来）
	runtimeMu sync.Mutex
}

// LoadMapScript 加载一张地图的脚本。
//
// 路径（servermap.cpp:577）：
//
//	<scriptRoot>/map/<地图名>.lua      主脚本
//	<scriptRoot>/map/default.lua      主脚本不存在时的默认脚本
func LoadMapScript(opts Options, mapName string, host MapScriptHost) (*MapScript, error) {
	bindings := make(map[string]lua.LGFunction, len(opts.Bindings)+30)
	for k, v := range opts.Bindings {
		bindings[k] = v
	}

	s := &MapScript{
		Host:      host,
		MapName:   mapName,
		glocCache: make(map[[4]int]int),
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

	// 地图脚本同样需要沙箱（getTLSTable 之类）
	if err := rt.InstallSandbox(); err != nil {
		rt.Close()
		return nil, err
	}
	// pause() 必须在 Lua 层定义（gopher-lua 不支持从 Go 函数 yield）
	if err := s.installPause(); err != nil {
		rt.Close()
		return nil, err
	}

	// ★★ 加载原版 servermap.lua ★★
	//
	//	它定义 addGridTrigger / addQuestGridTrigger /
	//	_RSVD_NAME_runGridTrigger / addSafeZone 等地图侧 API。
	//	任务通过 uidRemoteCall 到地图 actor 执行
	//	`addQuestGridTrigger(questName, rectList, handler)`
	//	—— 那个函数就在这里定义。
	//
	//	不加载它的后果：那 2 个用格子触发器的任务能加载成功，
	//	但玩家走到该被挡住的门口时**什么都不发生**（直接穿过去）。
	//
	// ★★ 顺序极其重要 ★★
	//
	//	servermap.lua **重新定义**了 addSafeZone（第 411 行），
	//	新实现会调 _RSVD_NAME_allocateSafeZoneId。
	//	所以必须**先装全部 C 绑定，再加载它**：
	//	反过来会在第一个地图脚本的第一行就崩在 servermap.lua:415。
	s.installFlattenHelpers()
	s.installSafeZones()
	s.installGridTriggers()
	gridPath := fmt.Sprintf("%s/servermap.lua", opts.FrameworkDir)
	if fileExists(gridPath) {
		if err := rt.patchAndRun(gridPath); err != nil {
			rt.Close()
			return nil, fmt.Errorf("load servermap.lua: %w", err)
		}
	}

	// 地图脚本的线程原语（runThread / pause 的 C 层）
	s.installMapThreadPrimitives()

	// 选脚本：<地图名>.lua 优先，否则 default.lua
	path := fmt.Sprintf("%s/map/%s.lua", opts.ScriptRoot, mapName)
	if !fileExists(path) {
		path = fmt.Sprintf("%s/map/default.lua", opts.ScriptRoot)
	}
	if !fileExists(path) {
		// 没有脚本的地图（比如测试 fixture）是正常的，不算错误
		return s, nil
	}

	if err := rt.patchAndRun(path); err != nil {
		rt.Close()
		return nil, fmt.Errorf("run map script %s: %w", path, err)
	}
	return s, nil
}

// Start 启动地图脚本的 main() 协程。
//
// 单独一个 Start 而不是在 LoadMapScript 里做：
//
//	加载阶段只应该「读脚本、建状态」，真正的运行时行为
//	（刷怪、守卫行动）要等服务端就绪后才开始。
//	否则测试里加载地图就会立刻刷一堆怪。
func (s *MapScript) Start(nowMS int64) {
	if s.started || s.Runtime == nil {
		return
	}
	fn, ok := s.Runtime.L.GetGlobal("main").(*lua.LFunction)
	if !ok {
		// 没有 main() 的脚本（比如只放 NPC 的图）是正常的
		s.started = true
		s.finished = true
		return
	}
	co, _ := s.Runtime.L.NewThread()
	s.mainCo = co
	s.started = true

	// 首次 resume：跑 main() 到第一个 pause()
	s.nowMS = nowMS
	state, err, _ := s.Runtime.L.Resume(co, fn)
	if err != nil || state == lua.ResumeError {
		s.Host.Log(1, fmt.Sprintf("map script main() error: %v", err))
		s.finished = true
		return
	}
	if state == lua.ResumeOK {
		s.finished = true
		return
	}
	// 停在 pause()：从 _pending 读时长算到期时刻
	s.pauseUntilMS = nowMS + s.takePauseMS()
}

// nowMS 当前世界时钟（pause 绑定用它算到期时刻）。
//
// 用字段而不是传参：Lua 绑定函数的签名是固定的 (L *LState) int，
// 没法额外传 nowMS。Tick 每次 resume 前更新它。

// Tick 推进地图脚本。
//
// 对应原版 actor 定时器对 main() 协程的周期性 resume：
// pause(msec) 里 yield，到期后 resume。
func (s *MapScript) Tick(nowMS int64) {
	if !s.started || s.finished || s.mainCo == nil {
		return
	}
	if nowMS < s.pauseUntilMS {
		return
	}

	s.nowMS = nowMS
	state, err, _ := s.Runtime.L.Resume(s.mainCo, nil)
	if err != nil || state == lua.ResumeError {
		s.Host.Log(1, fmt.Sprintf("map script main() error: %v", err))
		s.finished = true
		return
	}
	if state == lua.ResumeOK {
		// main() 跑完了（地图脚本一般 while true，不会到这里）
		s.finished = true
		return
	}
	s.pauseUntilMS = nowMS + s.takePauseMS()
}

// SpawnerCount 返回是否持有刷怪协程（诊断用，0 或 1）。
func (s *MapScript) SpawnerCount() int {
	if s.mainCo != nil && !s.finished {
		return 1
	}
	return 0
}

// Close 释放 Lua 状态。
func (s *MapScript) Close() {
	if s.Runtime != nil {
		s.Runtime.Close()
		s.Runtime = nil
	}
}

// ==================== 绑定 ====================

func (s *MapScript) bindings() map[string]lua.LGFunction {
	return map[string]lua.LGFunction{
		// ---- 地图信息 ----
		"getMapID": func(l *lua.LState) int {
			l.Push(lua.LNumber(s.Host.MapID()))
			return 1
		},
		"getMapName": func(l *lua.LState) int {
			l.Push(lua.LString(s.Host.MapName()))
			return 1
		},
		"getMapSize": func(l *lua.LState) int {
			w, h := s.Host.MapSize()
			l.Push(lua.LNumber(w))
			l.Push(lua.LNumber(h))
			return 2
		},

		// ---- 格子查询 ----
		"countGLoc":  s.luaCountGLoc,
		"randGLoc":   s.luaRandGLoc,
		"canThrough": s.luaCanThrough,
		"uidAlive":   s.luaUIDAlive,
		"rollKey":    func(l *lua.LState) int { l.Push(lua.LNumber(0)); return 1 },

		// ---- 刷怪 ----
		"addMonster": s.luaAddMonster,
		"addGuard":   s.luaAddGuard,

		// ---- 安全区 ----
		"addSafeZone": s.luaAddSafeZone,

		// ---- 协程让出 ----
		//
		// pause 不在 Go 里绑定，见 installPause 的说明。

		// ---- 换图（格子触发器的放行动作）----
		//
		// ★ 安全区与格子触发器**不在这里**装：
		//	它们在 installSafeZones / installGridTriggers 里单独装，
		//	且必须在加载 servermap.lua **之前**完成。
		//	早期这里有一批空桩（allocateGridTriggerId 恒返回 0、
		//	isSafeGrid 恒 false），会把真实现覆盖掉 ——
		//	表现是「触发器分配到了 ID，但格子查不到」。

		// hasGridSwitchDest(x, y)：这格配了换图目的地吗
		//
		// ★ servermap.lua 的 addGridTrigger 注释说得很清楚：
		//	「a grid with any trigger stops auto-switching and the script decides instead」
		//	—— 有触发器的格子要停掉自动换图，由脚本显式调
		//	uidGridMapSwitch 放行。所以这个查询是那条路径的前提。
		"hasGridSwitchDest": func(l *lua.LState) int {
			_, _, _, ok := s.Host.GridSwitchDest(l.CheckInt(1), l.CheckInt(2))
			l.Push(lua.LBool(ok))
			return 1
		},

		// uidGridMapSwitch(uid, x, y)：把玩家送到这格配置的目的地
		"uidGridMapSwitch": func(l *lua.LState) int {
			shortUID := l.CheckInt64(1)
			x := l.CheckInt(2)
			y := l.CheckInt(3)
			mapID, endX, endY, ok := s.Host.GridSwitchDest(x, y)
			if !ok || mapID == 0 {
				l.Push(lua.LFalse)
				return 1
			}
			l.Push(lua.LBool(s.Host.SwitchPlayer(s.realUID(shortUID), mapID, endX, endY)))
			return 1
		},

		// uidMapSwitch(uid, mapName, x, y)：按名字换图（脚本显式指定目的地）
		"uidMapSwitch": func(l *lua.LState) int {
			shortUID := l.CheckInt64(1)
			mapName := l.CheckString(2)
			x := l.CheckInt(3)
			y := l.CheckInt(4)
			mapID, ok := s.mapIDOf(mapName)
			if !ok || mapID == 0 {
				l.Push(lua.LFalse)
				return 1
			}
			l.Push(lua.LBool(s.Host.SwitchPlayer(s.realUID(shortUID), mapID, x, y)))
			return 1
		},
	}
}

// mapIDOf 按地图名查 ID。
//
// ★ 用包级 questMapNames（与任务共用同一份 603 张图的名字表）：
//	地图脚本与任务脚本必须看到同一套 ID，否则网格坐标对不上。
func (s *MapScript) mapIDOf(name string) (uint32, bool) {
	if questMapNames == nil {
		return 0, false
	}
	return questMapNames.IDOf(name)
}

// realUID 把 Lua 的短 ID 还原成真实 UID。
//
// ★ 地图侧的 uidGridMapSwitch/uidMapSwitch 收到的 uid 也是短 ID：
//	脚本从 getUID() 或事件参数里拿的都是 Lua 可见值。
func (s *MapScript) realUID(short int64) uint64 {
	if activeBus == nil {
		return uint64(short)
	}
	if real := activeBus.Real(short); real != 0 {
		return real
	}
	return uint64(short)
}

// luaCountGLoc 对应 servermap.cpp:141 countGLoc([x,y,w,h])。
//
// 不带参数数全图，带四个参数数矩形区域。
// 返回「可走的格数」，脚本用它判断这块区域能不能刷怪。
//
// ★ 必须有缓存，否则加载一张图要 30 秒。
//
//	实测：道馆_1 有 20 种怪 × 285 个刷新区域，每个区域 38×38=1444 格，
//	合计 **49 万次** CanThrough 调用。每次跨 Lua↔Go 边界约 60ns，
//	加上 Lua 侧循环开销，整张图加载耗时 30.8 秒。
//
//	优化成「一次调用就把整块区域算完」+ 结果缓存：
//	同一区域配置在一次加载里可能只算一次，但跨图/重载会复用，
//	所以缓存 key 用 (x,y,w,h)。
func (s *MapScript) luaCountGLoc(l *lua.LState) int {
	w, h := s.Host.MapSize()
	x0, y0, rw, rh := 0, 0, w, h

	switch l.GetTop() {
	case 0:
		// 全图
	case 4:
		x0 = l.CheckInt(1)
		y0 = l.CheckInt(2)
		rw = l.CheckInt(3)
		rh = l.CheckInt(4)
	default:
		// 参数个数不对：原版抛错，这里返回 0 让脚本走「区域无效」分支
		l.Push(lua.LNumber(0))
		return 1
	}

	if rw <= 0 || rh <= 0 {
		l.Push(lua.LNumber(0))
		return 1
	}

	key := [4]int{x0, y0, rw, rh}
	if cached, ok := s.glocCache[key]; ok {
		l.Push(lua.LNumber(cached))
		return 1
	}

	// 一次批量统计（不再逐格跨边界）
	count := s.Host.CountThrough(x0, y0, rw, rh)
	s.glocCache[key] = count

	l.Push(lua.LNumber(count))
	return 1
}

// luaRandGLoc 对应 servermap.cpp:203 randGLoc([x,y,w,h])。
//
// 在区域内随机取一个**可走**的格，返回 (x, y)；取不到返回 nil。
//
// ★ 原版是「一直随机直到命中」，地图大且区域全阻挡时会死循环。
//
//	这里改成「最多试 200 次，失败就顺序扫一遍」——
//	行为等价（能取到就取到），但不会卡死服务端。
func (s *MapScript) luaRandGLoc(l *lua.LState) int {
	w, h := s.Host.MapSize()
	x0, y0, rw, rh := 0, 0, w, h

	if l.GetTop() >= 4 {
		x0 = l.CheckInt(1)
		y0 = l.CheckInt(2)
		rw = l.CheckInt(3)
		rh = l.CheckInt(4)
	}
	if rw <= 0 || rh <= 0 {
		l.Push(lua.LNil)
		l.Push(lua.LNil)
		return 2
	}

	// 先随机试
	const maxTries = 200
	for i := 0; i < maxTries; i++ {
		x := x0 + s.randInt(rw)
		y := y0 + s.randInt(rh)
		if x < w && y < h && s.Host.CanThrough(x, y) {
			l.Push(lua.LNumber(x))
			l.Push(lua.LNumber(y))
			return 2
		}
	}

	// 兜底：顺序扫，保证「区域里真有空地就一定能拿到」
	for y := y0; y < y0+rh && y < h; y++ {
		for x := x0; x < x0+rw && x < w; x++ {
			if s.Host.CanThrough(x, y) {
				l.Push(lua.LNumber(x))
				l.Push(lua.LNumber(y))
				return 2
			}
		}
	}

	l.Push(lua.LNil)
	l.Push(lua.LNil)
	return 2
}

// luaCanThrough 判断某格是否可走。
func (s *MapScript) luaCanThrough(l *lua.LState) int {
	x := l.CheckInt(1)
	y := l.CheckInt(2)
	l.Push(lua.LBool(s.Host.CanThrough(x, y)))
	return 1
}

func (s *MapScript) luaUIDAlive(l *lua.LState) int {
	uid := uint64(l.CheckInt64(1))
	l.Push(lua.LBool(s.Host.Alive(uid)))
	return 1
}

// luaAddMonster 对应 servermap.cpp:345 addMonster(名字|ID, [x, y, [strictLoc]])。
//
// ★ 这是全脚本最热的调用（753 处 addMonCo 最终都走到这里）。
//
//	参数形态有三种，必须都支持：
//	  addMonster('半兽人')                      随机位置
//	  addMonster('半兽人', x, y)                指定位置
//	  addMonster('半兽人', x, y, true)          指定位置且必须精确落位
func (s *MapScript) luaAddMonster(l *lua.LState) int {
	name := luaToString(l.Get(1))
	if name == "" {
		l.Push(lua.LNumber(0))
		return 1
	}

	x, y := -1, -1
	strict := false
	switch l.GetTop() {
	case 1:
		// 随机位置
	case 3:
		x = l.CheckInt(2)
		y = l.CheckInt(3)
	default:
		if l.GetTop() >= 3 {
			x = l.CheckInt(2)
			y = l.CheckInt(3)
			if l.GetTop() >= 4 {
				strict = l.ToBool(4)
			}
		}
	}

	uid := s.Host.AddMonster(name, x, y, strict)
	l.Push(lua.LNumber(uid))
	return 1
}

// luaAddGuard 对应 servermap.cpp:431 addGuard(名字|ID, x, y, direction)。
func (s *MapScript) luaAddGuard(l *lua.LState) int {
	name := luaToString(l.Get(1))
	x := l.CheckInt(2)
	y := l.CheckInt(3)
	dir := l.CheckInt(4)
	if name == "" {
		l.Push(lua.LNumber(0))
		return 1
	}
	uid := s.Host.AddGuard(name, x, y, dir)
	l.Push(lua.LNumber(uid))
	return 1
}

// luaAddSafeZone 对应 addSafeZone(SYS_SAFEZONE, {{x=,y=,r=}, ...})。
//
// ★ 第一个参数是 SYS_SAFEZONE 常量，不是地图 ID。
//
//	脚本写的是 `addSafeZone(SYS_SAFEZONE, {{x=423,y=102,r=30}})`,
//	原版这个参数用来区分「要加哪种区域」，当前只有安全区一种。
//	忽略它不影响功能，但要能吃下这个参数（否则参数位移全错）。
func (s *MapScript) luaAddSafeZone(l *lua.LState) int {
	tbl := l.CheckTable(2)
	tbl.ForEach(func(_, v lua.LValue) {
		circle, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		x := luaInt(circle.RawGetString("x"))
		y := luaInt(circle.RawGetString("y"))
		r := luaInt(circle.RawGetString("r"))
		s.Host.AddSafeZone(x, y, r)
	})
	return 0
}

// ==================== 辅助 ====================

// randInt 返回 [0, n) 的随机数。
//
// 用包级 rand 而不是 crypto/rand：刷怪位置不需要密码学强度，
// 但需要速度（每 tick 可能几百次）。种子在包初始化时播一次。
func (s *MapScript) randInt(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

// luaInt 把 Lua 值转成 int（nil / 非数字返回 0）。
func luaInt(v lua.LValue) int {
	if n, ok := v.(lua.LNumber); ok {
		return int(n)
	}
	return 0
}

// luaToString 把 Lua 值转成字符串（数字也接受，因为 addMonster 允许传 monsterID）。
func luaToString(v lua.LValue) string {
	switch val := v.(type) {
	case lua.LString:
		return string(val)
	case lua.LNumber:
		// 数字形态：怪物 ID，交给 Host 处理（Host 会按 ID 查表）
		return fmt.Sprintf("%d", int(val))
	}
	return ""
}

// fileExists 判断文件是否存在。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// installMapThreadPrimitives 装地图 Lua 状态的线程原语。
//
// ★ 为什么地图也要一套：
//
//	servermap.lua 的 runMapThread(func) 直接调 runThread(key, func)
//	（该文件第 8 行），而 runThread 是 serverluacoroutinerunner.lua 定义的。
//	地图脚本里 `uidRemoteCall` 的异步回包也是靠线程 resume 完成的。
//
//	不做这一步，runMapThread 就是 nil —— 任何用到它的脚本路径
//	（格子触发器的 retire 通知、换图协程）都会直接崩。
func (s *MapScript) installMapThreadPrimitives() {
	if s.threads == nil {
		s.threads = newThreadRunner(s.Runtime.L, 1, func(format string, args ...any) {
			s.Host.Log(1, fmt.Sprintf(format, args...))
		})
	}
	t := s.threads
	L := s.Runtime.L

	L.SetGlobal("_RSVD_NAME_runThread", L.NewFunction(func(l *lua.LState) int {
		key := int64(l.CheckInt64(1))
		fn := l.CheckFunction(2)
		_, seqID, err := t.spawn(key, fn)
		if err != nil {
			l.RaiseError("runThread: %v", err)
			return 0
		}
		l.Push(lua.LNumber(seqID))
		l.Push(lua.LFalse)
		return 2
	}))
	L.SetGlobal("_RSVD_NAME_closeThread", L.NewFunction(func(l *lua.LState) int {
		key := int64(l.CheckInt64(1))
		seqID := int64(l.OptInt64(2, 0))
		found, selfClose := t.closeThread(key, seqID)
		l.Push(lua.LBool(found))
		l.Push(lua.LBool(selfClose))
		return 2
	}))
	L.SetGlobal("_RSVD_NAME_closeThreadThenRun", L.NewFunction(func(l *lua.LState) int {
		if err := t.closeThreadThenRun(int64(l.CheckInt64(1)), l.CheckFunction(2)); err != nil {
			l.RaiseError("%v", err)
		}
		return 0
	}))
	L.SetGlobal("_RSVD_NAME_closeRequested", L.NewFunction(func(l *lua.LState) int {
		r := t.current()
		l.Push(lua.LBool(r != nil && r.closeRequested && !r.closing))
		return 1
	}))
	L.SetGlobal("_RSVD_NAME_yieldToClose", L.NewFunction(func(l *lua.LState) int {
		if r := t.current(); r != nil {
			t.mu.Lock()
			r.closeRequested = true
			t.mu.Unlock()
		}
		return l.Yield()
	}))
	L.SetGlobal("_RSVD_NAME_checkThreadDepth", L.NewFunction(func(l *lua.LState) int {
		t.mu.Lock()
		depth := t.depth
		t.mu.Unlock()
		if depth >= maxThreadDepth {
			l.RaiseError("%d threads on the C stack", depth)
		}
		return 0
	}))
	L.SetGlobal("_RSVD_NAME_selfCloseError", L.NewFunction(func(l *lua.LState) int {
		if t.current() == nil {
			l.Push(lua.LString("outside any thread"))
			return 1
		}
		l.Push(lua.LNil)
		return 1
	}))
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
		tbl.Append(lua.LNumber(0))
		tbl.Append(lua.LNumber(t.CurrentKey()))
		tbl.Append(lua.LNumber(t.CurrentSeqID()))
		l.Push(tbl)
		return 1
	}))
	L.SetGlobal("_RSVD_NAME_pauseYielding", L.NewFunction(func(l *lua.LState) int {
		if err := t.pause(int64(l.CheckInt64(1))); err != nil {
			l.RaiseError("%v", err)
		}
		return l.Yield()
	}))
	L.SetGlobal("_RSVD_NAME_pickNotify", L.NewFunction(func(l *lua.LState) int {
		tbl := l.NewTable()
		for _, v := range t.pickNotify(l.OptInt(1, 0)) {
			tbl.Append(v)
		}
		l.Push(tbl)
		return 1
	}))
	L.SetGlobal("_RSVD_NAME_waitNotify", L.NewFunction(func(l *lua.LState) int {
		if vals, got := t.waitNotify(int64(l.OptInt64(1, 0))); got {
			for _, v := range vals {
				l.Push(v)
			}
			return len(vals)
		}
		l.Push(lua.LNil)
		return 1
	}))
	L.SetGlobal("clearNotify", L.NewFunction(func(l *lua.LState) int {
		t.clearNotify()
		return 0
	}))

	// ★ 加载原版协程运行器的 Lua 层（runThread / pause / uidRemoteCall 的封装）
	runnerPath := fmt.Sprintf("%s/serverluacoroutinerunner.lua", s.frameworkDirOf())
	if fileExists(runnerPath) {
		if err := s.Runtime.patchAndRun(runnerPath); err != nil {
			s.Host.Log(1, fmt.Sprintf("load serverluacoroutinerunner.lua: %v", err))
		}
	}
}

// frameworkDirOf 返回框架目录（installMapThreadPrimitives 要拼路径）。
//
// ★ 用 Runtime 上记录的值，不再往 MapScript 加一个字段：
//	框架目录在 LoadMapScript 时已经通过 Options 传进来了。
func (s *MapScript) frameworkDirOf() string {
	if s.Runtime == nil {
		return ""
	}
	return s.Runtime.frameworkDir
}

// installPause 在 Lua 里定义 pause(msec)。
//
// ★ 为什么不能用 Go 绑定实现 pause：
//
//	gopher-lua **不支持从 Go 函数里 yield**。
//	实测 `l.Yield(...)` 在 Go 绑定的函数里调用后，
//	Resume 直接返回 ResumeOK，整个 while 循环一次跑完 ——
//	yield 被静默忽略。症状是 main() 里的 `while true` 变成死循环，
//	Start() 永不返回，地图加载卡死（日志停在 "npc spawned"）。
//
//	而 **Lua 层的 coroutine.yield 是正常的**（Resume 返回 ResumeYield）。
//	所以 pause 必须在 Lua 里定义，让它调 coroutine.yield。
//
// 实现方式：
//
//  1. Lua 里：pause(msec) 把 msec 作为 yield 值让出
//
//  2. Go 里：resume 后从返回值里读到 msec，算出到期时刻
//
//     这样 pause 完全是 Lua 语义，Go 只负责「什么时候 resume」。
func (s *MapScript) installPause() error {
	if s.Runtime == nil || s.Runtime.L == nil {
		return nil
	}
	// 用 tab 做一个小桥：pause 把 msec 存进 pendingPauseMS 再 yield
	src := `
_pending = 0
function pause(msec)
    if type(msec) ~= 'number' then msec = 1000 end
    if msec < 0 then msec = 3600 * 1000 end
    _pending = msec
    coroutine.yield(msec)
end
`
	if err := s.Runtime.L.DoString(src); err != nil {
		return err
	}
	return nil
}

// takePauseMS 读取 pause 请求的时长（毫秒）。
//
// 直接读 Lua 全局变量，不调函数 —— 调函数要新建协程，
// 而 gopher-lua 的 NewThread 返回 (state, cancelFunc) 两个值，
// 在主协程外调自己又容易踩到重入。
// pause 已经把 msec 写进 _pending，这里读出来再清零。
func (s *MapScript) takePauseMS() int64 {
	if s.Runtime == nil || s.Runtime.L == nil {
		return 0
	}
	v := s.Runtime.L.GetGlobal("_pending")
	ms := int64(0)
	if n, ok := v.(lua.LNumber); ok {
		ms = int64(n)
	}
	s.Runtime.L.SetGlobal("_pending", lua.LNumber(0))
	return ms
}
