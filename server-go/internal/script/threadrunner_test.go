package script

import (
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// TestPauseYielding 验证「Go 函数里 return l.Yield() 能从协程 yield 出去」。
//
// ★ 这是 pause() 能否工作的**唯一前提**。
//
//	gopher-lua 的协议（vm.go callGFunction）：
//	    gfnret := frame.Fn.GFunction(L)
//	    if gfnret < 0 { switchToParentThread(L, L.GetTop(), false, false) }
//	即 Go 函数返回负数 → 立即从当前协程 yield 回父状态。
//	L.Yield(...) 的返回值恰好是 -1。
//
// ★ 早期版本在这里踩过坑，注释里写过「从 Go 函数里调 yield 会被静默忽略」，
//	于是把 pause 改成纯 Lua 的 coroutine.yield()。那个结论是**错的**：
//	真正的问题是没有绑定 _RSVD_NAME_pauseYielding（函数不存在，
//	调用直接报 "attempt to call a nil value"），而不是 yield 无效。
//	本测试把这条结论钉死，避免以后再退回错误实现。
func TestPauseYielding(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	var resumed int
	var co *lua.LState
	co, _ = L.NewThread()
	L.SetGlobal("_test_pause", L.NewFunction(func(l *lua.LState) int {
		msec := l.CheckInt64(1)
		_ = msec
		// 起一个「一会儿之后 resume」的假计时器
		go func() {
			time.Sleep(20 * time.Millisecond)
			resumed++
			_, _, _ = L.Resume(co, nil)
		}()
		return l.Yield()
	}))

	// 在协程里跑：pause → 打点 → pause → 打点
	fn, err := L.LoadString(`
		__trace = {}
		_test_pause(10)
		__trace[#__trace + 1] = 'after1'
		_test_pause(10)
		__trace[#__trace + 1] = 'after2'
		return #__trace
	`)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	st, err, vals := L.Resume(co, fn)
	if err != nil {
		t.Fatalf("first resume: %v", err)
	}
	if st != lua.ResumeYield {
		t.Fatalf("expected first resume to yield, got state=%v", st)
	}

	// 等待计时器把它推完
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if resumed >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = vals

	// 检查 trace
	tr, ok := co.GetGlobal("__trace").(*lua.LTable)
	if !ok {
		// 沙箱没装，全局在 co.Env 上；用 L 读也行
		tr, ok = L.GetGlobal("__trace").(*lua.LTable)
	}
	if !ok {
		t.Fatal("__trace 不是表")
	}
	if tr.Len() == 0 {
		t.Fatalf("pause 之后函数体没有继续执行 —— Go 侧 yield 无效")
	}
	t.Logf("trace=%d resumed=%d", tr.Len(), resumed)
	if tr.Len() != 2 {
		t.Fatalf("expected 2 trace entries, got %d (resumed=%d)", tr.Len(), resumed)
	}

	_ = resumed
}