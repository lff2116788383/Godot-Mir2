package script

import (
	"fmt"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// ==================== 协程执行器（server/src/serverluacoroutinerunner.cpp）====================
//
// 原版的每条 NPC 事件都在**独立协程**里跑，不是主线程直接调用：
//
//	NPChar::on_AM_NPCEVENT -> m_luaRunner->spawn(uid, "return _RSVD_NAME_npc_main(...)")
//
// 这个设计不是"为了并发"，而是沙箱机制的前提：
//
//	getTLSTable() 的第一句就是
//
//	    local threadId, inMainThread = coroutine.running()
//	    if inMainThread then error('call getTLSTable() in main thread') end
//
//	也就是说**在主线程调 _RSVD_NAME_npc_main 必然报错**。
//	每个协程有自己的一份 TLS（uid / startTime），
//	所以并发处理多个玩家的对话时不会互相覆盖。
//
// ★ 第一次跑就撞上这个：直接 CallGlobal 报
//   "npchar.lua:403: attempt to call a non-function object"，
//   而 403 行正是 `getTLSTable().uid = from`。
//
// Go 侧对应实现：用一个常驻协程池？不需要 —— NPC 事件是短同步逻辑，
// 这里每次 spawn 一个 coroutine，跑完就丢，语义与原版一致。

// spawnResult 是一次协程执行的产物。
type spawnResult struct {
	// Yields 协程主动 yield 的次数（原版用来做 sleep 之类的挂起）
	Yields int
	// Err 最终错误（含脚本 raise）
	Err error
}

// SpawnEvent 在独立协程里执行 NPC 事件入口。
//
// 与原版 spawn(uid, code) 的差异：
//
//	原版把代码拼成字符串 "return _RSVD_NAME_npc_main(uid, path, event, value)"
//	再 load 执行。这里直接构造闭包调函数，避免字符串拼接的转义风险
//	（NPC 名里带引号就会炸）。
func (s *NPCScript) SpawnEvent(playerUID uint64, path string, event string, value *string) spawnResult {
	L := s.Runtime.L

	// 构造参数
	args := make([]lua.LValue, 0, 4)
	args = append(args, lua.LNumber(playerUID))
	if path == "" {
		args = append(args, lua.LNil)
	} else {
		args = append(args, lua.LString(path))
	}
	args = append(args, lua.LString(event))
	if value == nil {
		args = append(args, lua.LNil)
	} else {
		args = append(args, lua.LString(*value))
	}

	s.Runtime.mu.Lock()
	defer s.Runtime.mu.Unlock()

	// 取入口函数
	fn, ok := L.GetGlobal("_RSVD_NAME_npc_main").(*lua.LFunction)
	if !ok {
		return spawnResult{Err: fmt.Errorf("_RSVD_NAME_npc_main is not a function")}
	}

	// ★ 关键：用 NewThread + Resume 起协程，而不是直接 PCall
	//
	//	直接调 → coroutine.running() 返回 nil → getTLSTable 报错
	//	协程里调 → 返回 thread 对象 → 正常
	//
	// gopher-lua 的签名是 Resume(th, fn, args...)，
	// 参数直接展开传，不用先 Push。
	co, _ := L.NewThread()

	res := spawnResult{}
	state, err, _ := L.Resume(co, fn, args...)
	if err != nil {
		res.Err = fmt.Errorf("npc script resume: %w", err)
		return res
	}
	for state == lua.ResumeYield {
		res.Yields++
		if res.Yields > maxYieldsPerEvent {
			res.Err = fmt.Errorf("npc script yielded more than %d times", maxYieldsPerEvent)
			return res
		}
		// 当前版本没有真正可挂起的外部调用（uidRemoteCall 是空实现），
		// 继续 resume 直到跑完。接上 uidRemoteCall 后，这里要等 actor 回应再 resume。
		state, err, _ = L.Resume(co, nil)
		if err != nil {
			res.Err = fmt.Errorf("npc script resume: %w", err)
			return res
		}
	}
	if state == lua.ResumeError {
		res.Err = fmt.Errorf("npc script error: %v", err)
	}
	return res
}

// maxYieldsPerEvent 是单次事件允许的最大 yield 次数。
//
// 对应原版的脚本超时保护：防止脚本写死循环
// `while true do coroutine.yield() end` 把服务端卡住。
const maxYieldsPerEvent = 1000

// InstallSandbox 加载原版沙箱脚本（common/src/luamodule.cpp:22 的内嵌字符串）。
//
// 这份脚本做三件事：
//
//  1. 定义 getTLSTable() —— 每个协程独立的 TLS（uid / startTime）
//  2. 把全局环境换成带 __index/__newindex 的代理表：
//     协程内写全局变量只写自己的 sandbox，不污染别的协程
//  3. 定义 autoClearTLSTable() —— 协程结束时清 TLS
//
// ★ 必须在这里做，不能推迟到 npchar.lua 之后：
//
//	npchar.lua 第 403 行的 getTLSTable() 是运行时才调，
//	但 setEventHandler 里已经有 `assert(hasChar(event))` 这种
//	依赖全局环境的判断。环境代理不先装好，脚本里
//	写全局变量的行为就不对。
func (rt *Runtime) InstallSandbox() error {
	path := ftPath(rt.frameworkDir, "sandbox.lua")
	if err := rt.L.DoFile(path); err != nil {
		return fmt.Errorf("load sandbox.lua: %w", err)
	}
	return nil
}

// ftPath 是 filepath.Join 的薄包装（避免在多个文件重复 import）。
func ftPath(dir, name string) string {
	return dir + "/" + name
}

// nowMS 返回当前毫秒时间戳（脚本 getNanoTstamp 用）。
func nowMS() int64 {
	return time.Now().UnixNano() / 1e6
}
