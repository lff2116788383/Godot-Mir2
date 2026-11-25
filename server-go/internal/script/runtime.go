package script

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// ==================== Lua 运行时（对应 common/src/luamodule.cpp）====================
//
// 原版用 sol2 把 C++ 函数绑到 Lua 全局，这里是 gopher-lua 的等价绑定。
//
// 分层（严格照原版）：
//
//	base.lua      common/src/luamodule.lua —— 所有模块共用的工具函数
//	              （assertType / tableEmpty / splitString / guardScope …）
//	npchar.lua    server/src/npchar.lua    —— NPC 事件分发核心
//	              （setEventHandler / _RSVD_NAME_npc_main / hasEventHandler …）
//	具体 NPC 脚本  server/script/npc/<地图>.<NPC名>.GLOC_x_y_d.LOOK_n.lua
//
// ★ 为什么必须原样加载原版 .lua 而不能在 Go 里重写逻辑：
//
//	NPC 对话的菜单生成、事件路径解析（SYS_EPDEF/EPQST/EPUID 三种）、
//	红名拦截、SYS_CHECKACTIVE 判定，这些散落在 700 行 Lua 里，
//	而且 341 个 NPC 脚本全部依赖它们的精确语义（比如「只有一个入口时
//	不生成菜单直接跳进去」）。在 Go 里重写一遍等于把 700 行翻译成另一种
//	语言，还得和 341 个脚本对拍 —— 直接用解释器跑原版脚本反而更可靠。

// Runtime 是一个独立的 Lua 状态。
//
// 每个 NPC 一个 Runtime：原版也是每个 NPChar 一个 LuaThreadRunner，
// 因为脚本里的 setEventHandler 是写全局变量的 —— 共享状态会让
// 后加载的 NPC 覆盖前一个的事件表。
type Runtime struct {
	L *lua.LState

	frameworkDir string
	scriptRoot   string

	mu sync.Mutex
	// loaded 已加载的模块（require 缓存由 Lua 自己管，这里只记主脚本）
	loaded bool
}

// Options 是 Runtime 的创建参数。
type Options struct {
	// FrameworkDir 基础脚本目录（内含 base.lua / npchar.lua）
	FrameworkDir string
	// ScriptRoot 原版脚本根目录（内含 npc/ include/ api/），用于 package.path
	ScriptRoot string
	// Bindings 由上层注入的 C 函数（uidPostXML 等）
	Bindings map[string]lua.LGFunction
	// Consts 注入到全局的常量（SYS_ENTER 等）
	Consts map[string]lua.LValue
}

// NewRuntime 创建一个 Runtime 并注入基础设施。
//
// ★ package.path 必须同时包含两个目录，顺序也有讲究：
//
//	framework 在前 —— NPC 脚本里 `require('include.dialog')` 走的是
//	                 原版目录，但 base.lua/npchar.lua 由我们自己 DoFile
//	                  加载，不走 require，所以顺序其实不影响它们。
//	scriptRoot 在后 —— include/ npc/ api/ 这些子目录都在原版目录下。
//
// 少写 scriptRoot 的后果：脚本第一行的 require 直接失败，
// 整个 NPC 加载中断，日志里只有一句 "module 'include.dialog' not found"。
func NewRuntime(opts Options) (*Runtime, error) {
	L := lua.NewState(lua.Options{
		// 不让脚本用 os/io 这类库：NPC 脚本只需要 string/table/math
		SkipOpenLibs: true,
	})
	// 只开必要的库
	//
	// ★ 必须开 Package 库：脚本里全是 require('include.dialog')，
	//   没有 package 表就等于「require 不存在」，连 base.lua 都跑不完。
	//   初版为了"安全"只开了 base/table/string/math/coroutine，
	//   结果第一行就报 attempt to index a non-table object(nil) with key 'path'。
	for _, pair := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
		{lua.CoroutineLibName, lua.OpenCoroutine},
		// ★ 常量名是 LoadLibName 不是 PackageLibName：
		//   gopher-lua 里 package 库的常量叫 LoadLibName = "package"
		//   （linit.go）。按直觉写 PackageLibName 会编译不过。
		{lua.LoadLibName, lua.OpenPackage},
	} {
		L.Push(L.NewFunction(pair.fn))
		L.Push(lua.LString(pair.name))
		L.Call(1, 0)
	}

	rt := &Runtime{L: L, frameworkDir: opts.FrameworkDir, scriptRoot: opts.ScriptRoot}

	// 补 5.2+/5.3+ 标准库（table.pack / math.type / utf8 …）
	// 必须在 base.lua 之前执行：base.lua 第 29 行就调 table.pack
	installCompat(L)

	// package.path 指向原版脚本根目录（对应 serverluamodule.cpp 的拼接）
	if opts.ScriptRoot != "" {
		if err := L.DoString(fmt.Sprintf(`package.path = package.path .. ';%s/?.lua;%s/?/init.lua'`,
			opts.ScriptRoot, opts.ScriptRoot)); err != nil {
			L.Close()
			return nil, fmt.Errorf("set package.path: %w", err)
		}
	}

	// 常量
	for name, value := range opts.Consts {
		L.SetGlobal(name, value)
	}
	// 绑定函数
	for name, fn := range opts.Bindings {
		L.SetGlobal(name, L.NewFunction(fn))
	}

	// 基础工具函数（原版 common/src/luamodule.lua）
	basePath := filepath.Join(opts.FrameworkDir, "base.lua")
	if err := L.DoFile(basePath); err != nil {
		L.Close()
		return nil, fmt.Errorf("load base.lua: %w", err)
	}
	return rt, nil
}

// LoadNPCFramework 加载 npchar.lua（NPC 事件分发核心）。
//
// 必须在加载具体 NPC 脚本之前调用：具体脚本开头就调 setEventHandler，
// 而那个函数是 npchar.lua 定义的。
//
// ★ 顺序：npchar.lua 之后、具体脚本之前装 `server` 表。
//
//	具体脚本的第一行就是 `local dialog = require(...)` +
//	`server.player.xxx(...)`。server 表晚一步装，脚本就报
//	"attempt to index a nil value (global 'server')"。
func (rt *Runtime) LoadNPCFramework() error {
	path := filepath.Join(rt.frameworkDir, "npchar.lua")
	if err := rt.L.DoFile(path); err != nil {
		return fmt.Errorf("load npchar.lua: %w", err)
	}
	// 装 server 表（api/player.lua 等零翻译复用）
	if err := rt.LoadServerTable(); err != nil {
		return err
	}
	rt.mu.Lock()
	rt.loaded = true
	rt.mu.Unlock()
	return nil
}

// LoadServerTable 注入 `server` 表（server.player / server.npc / server.utils）。
//
// ★ 用 pcall 包裹每个 require：某个 api 模块自身语法有问题时，
// 只是那个子表缺失，不会让整个 NPC 加载失败。
//
//	（比奇县行政官员这种关键 NPC 依赖 server.player；
//	 即使 api.npc 坏了也不该连累它。）
func (rt *Runtime) LoadServerTable() error {
	if rt.scriptRoot == "" {
		// 没有脚本根目录就装不了 api/*.lua ——
		// 此时 server 表只有空壳，脚本调 server.player.xxx 会报 nil 索引。
		// 这是「无脚本模式」下的预期行为（只有内置测试会走到）。
		return rt.L.DoString("server = server or {}")
	}
	script := ServerTableBindings()
	if err := rt.L.DoString(script); err != nil {
		return fmt.Errorf("install server table: %w", err)
	}
	return nil
}

// LoadScript 加载一个具体 NPC 的地图脚本（原版 execFile(m_initScriptName)）。
func (rt *Runtime) LoadScript(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("npc script not found %s: %w", path, err)
	}
	// 用 patchAndRun 而不是 DoFile：
	// 原版脚本里有 Lua 5.3 才支持的 `//` 整除运算符，
	// gopher-lua 是 5.1 语法，直接 DoFile 会编译期报错。
	if err := rt.patchAndRun(path); err != nil {
		return fmt.Errorf("run npc script %s: %w", path, err)
	}
	return nil
}

// CallGlobal 调用一个全局函数，返回它的返回值。
func (rt *Runtime) CallGlobal(name string, args ...lua.LValue) ([]lua.LValue, error) {
	fn, ok := rt.L.GetGlobal(name).(*lua.LFunction)
	if !ok {
		return nil, fmt.Errorf("global %q is not a function", name)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.L.Push(fn)
	for _, arg := range args {
		rt.L.Push(arg)
	}
	if err := rt.L.PCall(len(args), lua.MultRet, nil); err != nil {
		return nil, err
	}
	n := rt.L.GetTop()
	ret := make([]lua.LValue, n)
	for i := 0; i < n; i++ {
		ret[i] = rt.L.Get(i + 1)
	}
	rt.L.Pop(n)
	return ret, nil
}

// Close 释放 Lua 状态。
func (rt *Runtime) Close() {
	if rt.L != nil {
		rt.L.Close()
		rt.L = nil
	}
}

// ==================== 常量注入 ====================

// SYS 常量（common/src/sysconst.hpp:171-199）。
//
// ★ 这些字符串必须逐字一致：
//
//	脚本里用它们做 table key（`handler[SYS_ENTER]`），
//	Go 侧用同一个字符串判断事件类型。抄错一个字符，
//	表现就是「点了 NPC 没反应」而且没有任何报错。
const (
	SysEnter        = "_RSVD_NAME_ENTER_90360178872"
	SysExit         = "_RSVD_NAME_EXIT__14208236065"
	SysEPDef        = "_RSVD_NAME_EVENT_PATH_DEF_2965316381"
	SysEPUID        = "_RSVD_NAME_EVENT_PATH_UID_3623042653"
	SysEPQST        = "_RSVD_NAME_EVENT_PATH_QST_6329204623"
	SysHide         = "_RSVD_NAME_HIDE_3194448323"
	SysLabel        = "_RSVD_NAME_LABEL_78921733019"
	SysCheckActive  = "_RSVD_NAME_CHECK_ACTIVE_4054544333"
	SysAllowRedName = "_RSVD_NAME_ALLOW_REDNAME_761479946"
	SysSpaceMove    = "_RSVD_NAME_SPACE_MOVE_683761479946"
	SysGoldName     = "金币（小）"
)

// MaxNPCDistance 是玩家与 NPC 的最大交互格距（sysconst.hpp:128）。
const MaxNPCDistance = 10

// DefaultConsts 返回要注入 Lua 的常量表。
func DefaultConsts() map[string]lua.LValue {
	return map[string]lua.LValue{
		"SYS_ENTER":          lua.LString(SysEnter),
		"SYS_EXIT":           lua.LString(SysExit),
		"SYS_EPDEF":          lua.LString(SysEPDef),
		"SYS_EPUID":          lua.LString(SysEPUID),
		"SYS_EPQST":          lua.LString(SysEPQST),
		"SYS_HIDE":           lua.LString(SysHide),
		"SYS_LABEL":          lua.LString(SysLabel),
		"SYS_CHECKACTIVE":    lua.LString(SysCheckActive),
		"SYS_ALLOWREDNAME":   lua.LString(SysAllowRedName),
		"SYS_SPACEMOVE":      lua.LString(SysSpaceMove),
		"SYS_GOLDNAME":       lua.LString(SysGoldName),
		"SYS_MAXNPCDISTANCE": lua.LNumber(MaxNPCDistance),
		// 日志级别（server.hpp LOGTYPE_*）
		"LOGTYPE_INFO":    lua.LNumber(0),
		"LOGTYPE_WARNING": lua.LNumber(1),
		"LOGTYPE_FATAL":   lua.LNumber(2),

		// ---- 方向（protocoldef.hpp:20-45 dir8_t）----
		//
		// ★ 地图脚本大量用 DIR_DOWNLEFT 之类当 addGuard 的第 4 个参数。
		//   不注入的后果不是「守卫朝向错了」，而是
		//   `bad argument #4 to addGuard (number expected, got nil)`
		//   —— 整张地图的脚本加载直接失败，一只怪都刷不出来。
		"DIR_NONE":      lua.LNumber(0),
		"DIR_BEGIN":     lua.LNumber(1),
		"DIR_UP":        lua.LNumber(1),
		"DIR_UPRIGHT":   lua.LNumber(2),
		"DIR_RIGHTUP":   lua.LNumber(2),
		"DIR_RIGHT":     lua.LNumber(3),
		"DIR_DOWNRIGHT": lua.LNumber(4),
		"DIR_RIGHTDOWN": lua.LNumber(4),
		"DIR_DOWN":      lua.LNumber(5),
		"DIR_DOWNLEFT":  lua.LNumber(6),
		"DIR_LEFTDOWN":  lua.LNumber(6),
		"DIR_LEFT":      lua.LNumber(7),
		"DIR_UPLEFT":    lua.LNumber(8),
		"DIR_LEFTUP":    lua.LNumber(8),
		"DIR_END":       lua.LNumber(9),

		// ---- 安全区类型（sysconst.hpp:195）----
		// addSafeZone 的第一个参数，脚本用它区分「要加哪种区域」
		"SYS_SAFEZONE": lua.LString("_RSVD_NAME_SAFE_ZONE_5182730946"),

		// ---- 背包操作类型（protocoldef.hpp:464-466）----
		"INVOP_TRADE":  lua.LNumber(1),
		"INVOP_SECURE": lua.LNumber(2),
		"INVOP_REPAIR": lua.LNumber(3),

		// ---- UID 类型（uidf.hpp）----
		"UID_NONE":  lua.LNumber(0),
		"UID_BEGIN": lua.LNumber(0),
		"UID_COR":   lua.LNumber(0),
		"UID_MAP":   lua.LNumber(1),
		"UID_PLY":   lua.LNumber(2),
		"UID_MON":   lua.LNumber(3),
		"UID_NPC":   lua.LNumber(4),
		"UID_RCV":   lua.LNumber(6),
		"UID_END":   lua.LNumber(7),
	}
}
