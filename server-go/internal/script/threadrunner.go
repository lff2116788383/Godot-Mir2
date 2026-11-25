package script

// ==================== 线程运行器（对应 server/src/serverluacoroutinerunner.cpp）====================
//
// 【依据】server/src/serverluacoroutinerunner.cpp（909 行）
//         + server/src/serverluacoroutinerunner.lua（481 行，原样复用）
//
// ★★ 这一层是任务系统能否真正跑起来的**唯一硬门槛** ★★
//
// 原版的设计：
//
//	每个 ServerObject（任务 / NPC / 玩家 / 地图）持有一个
//	ServerLuaCoroutineRunner。一个 "runner" 就是一个 Lua 协程，
//	由 {key, seqID} 唯一标识：
//
//	    key   调用方给的「线程组」标识（任务用 rollKey() 分配）
//	    seqID runner 级单调递增序号（m_seqID++）
//
//	Lua 侧的 runThread / closeThread / pause / waitNotify
//	全部建立在这对 C 绑定之上：
//
//	    _RSVD_NAME_runThread(key, func)   -> (newSeqID, closed)
//	    _RSVD_NAME_closeThread(key, seq)  -> (found, selfClose)
//	    _RSVD_NAME_closeThreadThenRun(k, f)
//	    _RSVD_NAME_closeRequested(what)   -> bool
//	    _RSVD_NAME_pauseYielding(msec)      （yielding：从 C 里 yield）
//	    _RSVD_NAME_waitNotify(timeout[, key, seq]) -> result|nil
//	    _RSVD_NAME_pickNotify(maxCount)   -> list
//	    _RSVD_NAME_postNotify / _RSVD_NAME_sendNotify
//
// ★★ 之前实现的三个致命错误（已全部修掉，注释保留作为回归防线）★★
//
//	1. `co.Resume(l, fn)` —— 参数传反了。
//	   gopher-lua 的签名是 (*LState).Resume(th, fn, args...)，
//	   即在**主状态上调 Resume**，第一个参数是「要跑的那个线程」。
//	   写成 co.Resume(l, fn) 是把主状态当线程去 resume，
//	   主状态本身正在运行 → "can not resume a running thread"，
//	   或者线程根本没启动。表现就是**任务状态函数体一行都没跑**，
//	   而日志里完全没有任何错误（Resume 返回的 err 被吞了）。
//
//	2. 返回值顺序反了：原版是 (newSeqID, closed)，实现写成了 (closed, seqID)。
//	   Lua 侧 `local seqID, closed = _RSVD_NAME_runThread(key, func)` 于是拿到
//	   seqID=false、closed=<数字> → closed 为真 → 立刻 yieldToClose，
//	   新线程**在第一次 yield 处就被判定为「要求关闭」并终止**。
//
//	3. 线程 key 用 Go 全局变量传给 Lua。
//	   沙箱把全局环境换成了按协程隔离的代理表（sandbox.lua），
//	   所以 Go 侧 SetGlobal 写的值，协程里读不到；
//	   反过来协程里写的，主状态也读不到。
//	   getThreadKey() 于是恒返回 mainKey，
//	   quest.lua 里所有 `getThreadKey() == 某个 key` 的判断全部失效 ——
//	   setQuestState 认为「调用者不是本状态的状态运行器」，
//	   于是走「不关自己」的分支，状态机彻底不转。
//
// ★ 现在的做法：线程标识不进 Lua 全局，直接放在 Go 的 runner 结构里，
//	由绑定函数读 Go 侧当前执行栈顶的 runner。

import (
	"fmt"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// threadAddr 是原版的 {uid, key, seqID} 三元组（getThreadAddress）。
type threadAddr struct {
	key   int64
	seqID int64
}

// waitKind 是线程当前等待的类型（原版 LuaThreadHandle::waitType）。
type waitKind uint8

const (
	waitNone   waitKind = iota
	waitPause           // 在 pause() 里等计时器
	waitNotify          // 在 waitNotify() 里等通知
	waitCoop            // 在 uidRemoteCall()/sendNotify() 里等回包
	waitClose           // 被要求关闭，在 yield 处结束
)

// luaRunner 是原版 LuaThreadHandle 的等价物：一个可恢复的 Lua 协程。
type luaRunner struct {
	key   int64
	seqID int64

	// co 协程本体
	co *lua.LState

	// wait 当前等待类型
	wait waitKind
	// notifyList 已收到但还没被取走的通知（原版 notifyList）
	notifyList []lua.LValue
	// notifyTimer 当前通知超时计时器
	notifyTimer *time.Timer
	// pauseTimer 当前 pause 计时器
	pauseTimer *time.Timer

	// closeRequested 被要求关闭（原版 closeRequested）
	//
	// ★ 与 closing 的区别：closeRequested 表示「有人想关我，
	//	我在下一个 yield 处自己结束」；closing 表示「正在关」。
	closeRequested bool
	closing        bool

	// onStack 正在 C 栈上执行（不能立刻重置）
	onStack bool
	// dead 已结束
	dead bool

	// afterClose 被 closeThreadThenRun 设置：本线程结束后要启动的新线程
	afterClose *luaRunner

	// host 宿主
	host *threadRunner
}

// threadRunner 是原版 ServerLuaCoroutineRunner 的等价物。
//
// ★ 每个 Lua 状态一份：任务一个、NPC 一个、地图一个。
//	它管理该状态里的全部线程。
type threadRunner struct {
	L *lua.LState

	// mu 保护下面的表（同一状态可能被不同 goroutine 触碰）
	mu sync.Mutex

	// runners key -> seqID -> runner
	//
	// ★ 用两级 map 而不是单 map：原版 m_runnerList 是
	//	multimap<key, LuaThreadHandle>，同一 key 可以并存多个 seqID
	//	（状态切换时旧的还没退栈，新的已经起来）。
	runners map[int64]map[int64]*luaRunner

	// nextSeqID runner 级序号（原版 m_seqID，初始 1）
	nextSeqID int64
	// mainKey 主线程 key（任务用 getMainScriptThreadKey）
	mainKey int64

	// curr 当前正在执行的 runner（原版 m_currRunner）
	//
	// ★ 这是全局单值：原版一个 runner 对象同时只跑一个线程。
	//	Go 侧我们用 luaMu 保证同一时刻只有一个 goroutine 进 LState，
	//	所以单值够用。
	curr *luaRunner

	// depth C 栈上嵌套的线程数（原版 m_threadDepth）
	depth int

	// onDelete 线程结束时回调（任务用来清理状态运行器登记）
	onThreadDone func(r *luaRunner)
	// logf 日志
	logf func(format string, args ...any)
}

// maxThreadDepth 是 C 栈上允许嵌套的最大线程数（原版 64）。
const maxThreadDepth = 64

// newThreadRunner 创建线程运行器。
func newThreadRunner(L *lua.LState, mainKey int64, logf func(string, ...any)) *threadRunner {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &threadRunner{
		L:         L,
		runners:   make(map[int64]map[int64]*luaRunner),
		nextSeqID: 1,
		mainKey:   mainKey,
		logf:      logf,
	}
}

// current 返回当前正在执行的 runner（可能为 nil，表示在主线程）。
func (t *threadRunner) current() *luaRunner {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.curr
}

// CurrentKey 返回当前线程 key；不在任何线程里时返回主线程 key。
func (t *threadRunner) CurrentKey() int64 {
	if r := t.current(); r != nil {
		return r.key
	}
	return t.mainKey
}

// CurrentSeqID 返回当前线程序号；不在任何线程里时返回 0。
func (t *threadRunner) CurrentSeqID() int64 {
	if r := t.current(); r != nil {
		return r.seqID
	}
	return 0
}

// spawn 启动一个新线程，跑到它第一次 yield 或结束。
//
// 返回 (key, seqID, err)。
//
// ★ 对应原版 ServerLuaCoroutineRunner::spawn(key, func)。
func (t *threadRunner) spawn(key int64, fn *lua.LFunction, args ...lua.LValue) (int64, int64, error) {
	t.mu.Lock()
	seqID := t.nextSeqID
	t.nextSeqID++
	t.mu.Unlock()

	return t.spawnWithAddr(threadAddr{key: key, seqID: seqID}, fn, args...)
}

// spawnWithAddr 用给定地址启动线程。
func (t *threadRunner) spawnWithAddr(addr threadAddr, fn *lua.LFunction, args ...lua.LValue) (int64, int64, error) {
	if fn == nil {
		return addr.key, addr.seqID, fmt.Errorf("spawn: nil function")
	}

	// ★ 深度检查（原版 checkThreadDepth）
	//
	//	一个状态切换链如果完全不 yield，会在 C 栈上无限嵌套。
	//	原版限 64，超过就抛错。
	t.mu.Lock()
	if t.depth >= maxThreadDepth {
		t.mu.Unlock()
		return addr.key, addr.seqID, fmt.Errorf(
			"%d threads run on top of each other on the C stack, can't start one more", t.depth)
	}
	t.depth++
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.depth--
		t.mu.Unlock()
	}()

	// 建协程
	co, _ := t.L.NewThread()

	r := &luaRunner{key: addr.key, seqID: addr.seqID, co: co, host: t}

	t.mu.Lock()
	if t.runners[addr.key] == nil {
		t.runners[addr.key] = make(map[int64]*luaRunner)
	}
	t.runners[addr.key][addr.seqID] = r
	prev := t.curr
	t.curr = r
	t.mu.Unlock()

	// ★★ 正确的调用方式：主状态 Resume 子线程 ★★
	//
	//	gopher-lua: func (ls *LState) Resume(th *LState, fn *LFunction, args ...LValue)
	//	ls 是「发起 resume 的状态」，th 是「被 resume 的线程」。
	st, err, _ := t.L.Resume(co, fn, args...)

	t.mu.Lock()
	t.curr = prev
	r.onStack = false
	dead := st != lua.ResumeYield
	if dead {
		r.dead = true
		delete(t.runners[addr.key], addr.seqID)
		if len(t.runners[addr.key]) == 0 {
			delete(t.runners, addr.key)
		}
	}
	afterClose := r.afterClose
	r.afterClose = nil
	selfClose := r.closeRequested && dead
	t.mu.Unlock()

	// 线程结束回调
	if dead && t.onThreadDone != nil {
		t.onThreadDone(r)
	}

	// closeThreadThenRun：调用线程已经结束，启动它留下的新线程
	if afterClose != nil {
		if _, _, cerr := t.spawnWithAddr(
			threadAddr{key: afterClose.key, seqID: afterClose.seqID},
			afterClose.funcValue()); cerr != nil {
			t.logf("closeThreadThenRun start failed: %v", cerr)
		}
	}

	if err != nil {
		return addr.key, addr.seqID, err
	}
	// ★ selfClose 语义：本线程在第一次 yield 前就被要求关闭
	//
	//	原版是从 resumeRunner 里检测，Lua 侧 runThread 靠 second 返回值
	//	判断要不要 yieldToClose。这里让 spawn 返回时把「本该结束但没结束」
	//	的情况报出来。
	if selfClose {
		return addr.key, addr.seqID, fmt.Errorf("thread closed during spawn")
	}
	return addr.key, addr.seqID, nil
}

// funcValue 取出 afterClose 携带的函数。
//
// ★ 为了让 luaRunner 结构体保持简单，afterClose 存的是
//	一个「已经绑好函数的 runner 壳」，函数放在 notifyList[0] 里。
//	（用 notifyList 复用现有字段，避免再加一个字段。）
func (r *luaRunner) funcValue() *lua.LFunction {
	if len(r.notifyList) > 0 {
		if fn, ok := r.notifyList[0].(*lua.LFunction); ok {
			return fn
		}
	}
	return nil
}

// resume 恢复一个等待中的线程。
//
// ★ 对应原版 resume(key, seqID)：只恢复「被挂起」的线程，
//	正在跑的（onStack）或已死的（dead）都拒绝。
func (t *threadRunner) resume(key, seqID int64) error {
	if r := t.find(key, seqID); r != nil {
		return t.resumeRunner(r)
	}
	return fmt.Errorf("thread %d:%d not found", key, seqID)
}

// find 查找线程（seqID 为 0 表示该 key 下任意一个）。
func (t *threadRunner) find(key, seqID int64) *luaRunner {
	t.mu.Lock()
	defer t.mu.Unlock()
	m, ok := t.runners[key]
	if !ok {
		return nil
	}
	if seqID != 0 {
		return m[seqID]
	}
	for _, r := range m {
		return r
	}
	return nil
}

// resumeRunner 是恢复线程的唯一入口。
func (t *threadRunner) resumeRunner(r *luaRunner) error {
	if r == nil || r.co == nil {
		return fmt.Errorf("resume: nil runner")
	}

	t.mu.Lock()
	if r.dead {
		t.mu.Unlock()
		return fmt.Errorf("resume: thread %d:%d is dead", r.key, r.seqID)
	}
	if r.onStack {
		t.mu.Unlock()
		return fmt.Errorf("resume: thread %d:%d is running", r.key, r.seqID)
	}
	r.wait = waitNone
	if r.pauseTimer != nil {
		r.pauseTimer.Stop()
		r.pauseTimer = nil
	}
	if r.notifyTimer != nil {
		r.notifyTimer.Stop()
		r.notifyTimer = nil
	}
	prev := t.curr
	t.curr = r
	t.onStackSetLocked(r, true)
	t.mu.Unlock()

	// 恢复传参：yield 的返回值
	//
	// ★ 被要求关闭的线程在这里结束：原版是 resumeRunner 检测
	//	closeRequested 后直接 lua_closethread。这里等价地
	//	不再 resume，直接标记结束。
	t.mu.Lock()
	closing := r.closeRequested
	t.mu.Unlock()
	if closing {
		t.mu.Lock()
		r.dead = true
		r.closing = true
		delete(t.runners[r.key], r.seqID)
		if len(t.runners[r.key]) == 0 {
			delete(t.runners, r.key)
		}
		t.curr = prev
		t.mu.Unlock()
		if t.onThreadDone != nil {
			t.onThreadDone(r)
		}
		return nil
	}

	st, err, _ := t.L.Resume(r.co, nil)

	t.mu.Lock()
	t.curr = prev
	r.onStack = false
	if st != lua.ResumeYield {
		r.dead = true
		delete(t.runners[r.key], r.seqID)
		if len(t.runners[r.key]) == 0 {
			delete(t.runners, r.key)
		}
	}
	afterClose := r.afterClose
	r.afterClose = nil
	t.mu.Unlock()

	if st != lua.ResumeYield && t.onThreadDone != nil {
		t.onThreadDone(r)
	}

	if afterClose != nil {
		if _, _, cerr := t.spawnWithAddr(
			threadAddr{key: afterClose.key, seqID: afterClose.seqID},
			afterClose.funcValue()); cerr != nil {
			t.logf("closeThreadThenRun start failed: %v", cerr)
		}
	}
	return err
}

func (t *threadRunner) onStackSetLocked(r *luaRunner, v bool) { r.onStack = v }

// closeThread 关闭线程（seqID 0 表示该 key 下全部）。
//
// 返回 (found, selfClose)。
//
// ★ 调用者正在被关时，只标记 closeRequested，由 Lua 侧 yield 结束自己
//	（原版语义：selfClose 为真时 closeThread() 不返回）。
func (t *threadRunner) closeThread(key, seqID int64) (bool, bool) {
	t.mu.Lock()
	curr := t.curr
	selfClose := curr != nil && curr.key == key && (seqID == 0 || curr.seqID == seqID)

	m, ok := t.runners[key]
	if !ok {
		t.mu.Unlock()
		return false, selfClose
	}

	var victims []*luaRunner
	if seqID != 0 {
		if r, ok := m[seqID]; ok {
			victims = append(victims, r)
		}
	} else {
		for _, r := range m {
			victims = append(victims, r)
		}
	}
	t.mu.Unlock()

	if len(victims) == 0 {
		return false, selfClose
	}

	for _, r := range victims {
		if r == curr {
			// 自己：只标记，不立刻销毁（要在 yield 处结束）
			t.mu.Lock()
			r.closeRequested = true
			t.mu.Unlock()
			continue
		}
		t.mu.Lock()
		onStack := r.onStack
		if onStack {
			r.closeRequested = true
			t.mu.Unlock()
			continue
		}
		r.closing = true
		r.dead = true
		delete(t.runners[r.key], r.seqID)
		if len(t.runners[r.key]) == 0 {
			delete(t.runners, r.key)
		}
		if r.notifyTimer != nil {
			r.notifyTimer.Stop()
		}
		if r.pauseTimer != nil {
			r.pauseTimer.Stop()
		}
		t.mu.Unlock()
		if t.onThreadDone != nil {
			t.onThreadDone(r)
		}
	}
	return true, selfClose
}

// closeThreadThenRun 让调用线程结束后启动新线程。
func (t *threadRunner) closeThreadThenRun(key int64, fn *lua.LFunction) error {
	t.mu.Lock()
	curr := t.curr
	if curr == nil {
		t.mu.Unlock()
		return fmt.Errorf("closeThreadThenRun() called outside any thread")
	}
	seqID := t.nextSeqID
	t.nextSeqID++

	// 用 notifyList 暂存函数（见 funcValue）
	holder := &luaRunner{key: key, seqID: seqID, notifyList: []lua.LValue{fn}}
	curr.afterClose = holder
	t.mu.Unlock()

	// 关闭自己：只标记，Lua 侧在 yield 处结束
	t.mu.Lock()
	curr.closeRequested = true
	t.mu.Unlock()
	return nil
}

// pause 让当前线程睡 msec 毫秒。
//
// ★ 实现方式：注册计时器 → 从 Go 函数 return -1（gopher-lua 的 yield 协议）
//	→ 计时器到点后 resume。
//
//	这必须在**协程内**调用。主线程调用会报
//	"can not yield from outside of a coroutine"（与原版一致）。
func (t *threadRunner) pause(msec int64) error {
	r := t.current()
	if r == nil {
		return fmt.Errorf("pause() called outside any thread")
	}
	if msec <= 0 {
		return nil
	}
	t.mu.Lock()
	r.wait = waitPause
	if r.pauseTimer != nil {
		r.pauseTimer.Stop()
	}
	r.pauseTimer = time.AfterFunc(time.Duration(msec)*time.Millisecond, func() {
		t.mu.Lock()
		if r.dead || r.closeRequested {
			t.mu.Unlock()
			return
		}
		r.pauseTimer = nil
		r.wait = waitNone
		t.mu.Unlock()
		if err := t.resumeRunner(r); err != nil {
			t.logf("resume paused thread %d:%d: %v", r.key, r.seqID, err)
		}
	})
	t.mu.Unlock()
	return nil
}

// waitNotify 等一个通知。
//
// 返回 (values, got)：got=false 表示超时/没有通知，Lua 侧应 yield 等待。
//
// ★ 对应原版 _RSVD_NAME_waitNotify：
//	先看队列里有就取走；没有就进入等待并（可选）起超时计时器。
func (t *threadRunner) waitNotify(timeout int64) ([]lua.LValue, bool) {
	r := t.current()
	if r == nil {
		return nil, false
	}
	t.mu.Lock()
	if len(r.notifyList) > 0 {
		vals := r.notifyList
		r.notifyList = nil
		t.mu.Unlock()
		return vals, true
	}
	r.wait = waitNotify
	if timeout > 0 {
		if r.notifyTimer != nil {
			r.notifyTimer.Stop()
		}
		r.notifyTimer = time.AfterFunc(time.Duration(timeout)*time.Millisecond, func() {
			t.mu.Lock()
			if r.dead || r.closeRequested || r.wait != waitNotify {
				t.mu.Unlock()
				return
			}
			r.notifyTimer = nil
			r.wait = waitNone
			t.mu.Unlock()
			if err := t.resumeRunner(r); err != nil {
				t.logf("resume notify-waiting thread %d:%d: %v", r.key, r.seqID, err)
			}
		})
	}
	t.mu.Unlock()
	return nil, false
}

// pickNotify 取走最多 maxCount 条通知。
func (t *threadRunner) pickNotify(maxCount int) []lua.LValue {
	r := t.current()
	if r == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(r.notifyList) == 0 {
		return nil
	}
	if maxCount <= 0 || maxCount >= len(r.notifyList) {
		vals := r.notifyList
		r.notifyList = nil
		return vals
	}
	vals := r.notifyList[:maxCount]
	rest := make([]lua.LValue, len(r.notifyList)-maxCount)
	copy(rest, r.notifyList[maxCount:])
	r.notifyList = rest
	return vals
}

// addNotify 往某线程投递通知（对应 postNotify 的接收侧）。
func (t *threadRunner) addNotify(key, seqID int64, vals []lua.LValue) bool {
	r := t.find(key, seqID)
	if r == nil {
		return false
	}
	t.mu.Lock()
	waiting := r.wait == waitNotify
	r.notifyList = append(r.notifyList, vals...)
	if t.curr == r {
		// 自己给自己投递：不 resume，等它自己回到 waitNotify 检查
		t.mu.Unlock()
		return true
	}
	if waiting {
		r.wait = waitNone
	}
	t.mu.Unlock()
	if waiting {
		if err := t.resumeRunner(r); err != nil {
			t.logf("resume notified thread %d:%d: %v", r.key, r.seqID, err)
		}
	}
	return true
}

// clearNotify 清空当前线程的通知队列。
func (t *threadRunner) clearNotify() {
	if r := t.current(); r != nil {
		t.mu.Lock()
		r.notifyList = nil
		t.mu.Unlock()
	}
}

// closeAll 关闭全部线程（运行时销毁时调用）。
func (t *threadRunner) closeAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, m := range t.runners {
		for _, r := range m {
			r.closing = true
			r.dead = true
			if r.pauseTimer != nil {
				r.pauseTimer.Stop()
			}
			if r.notifyTimer != nil {
				r.notifyTimer.Stop()
			}
		}
		delete(t.runners, key)
	}
}

// pending 返回还在等待的线程数（诊断用）。
func (t *threadRunner) pending() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, m := range t.runners {
		n += len(m)
	}
	return n
}