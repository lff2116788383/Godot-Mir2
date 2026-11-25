package script

import (
	lua "github.com/yuin/gopher-lua"
)

// ==================== Lua 版本兼容层 ====================
//
// 原版服务端用官方 Lua（5.3+ 语义），脚本里大量使用 5.2+ 才有的标准库：
//
//	table.pack / table.unpack   （5.2+，5.1 只有 unpack）
//	math.type                   （5.3+）
//	string.format("%q")          （5.1 有，但个别实现不完整）
//	utf8 库                      （5.3+）
//
// gopher-lua 实现的是 Lua 5.1 语义，这些函数直接不存在。
// 表现是 assertType 里第一行 `table.pack(...)` 就报
// "attempt to call a non-function object"。
//
// ★ 为什么补兼容层而不是改脚本：
//
//	改写 base.lua 里的 assertType 能让它跑起来，但 341 个 NPC 脚本
//	里还有大量 `math.type(x) == 'integer'` 这类调用，逐个改写等于
//	把"直接复用原版脚本"变成"维护一个 fork"，上游一更新就冲突。
//	补一层 polyfill 是唯一能保持「脚本零修改」的做法。
//
// 文档（replication-work-document 5.5）也是这个方向：
// 「现有脚本原则上不改；对确实存在平台相关的脚本，提供兼容层，
//   而不是批量改写脚本」。

// installCompat 给 Lua 状态补上 5.2+/5.3+ 的标准库函数。
func installCompat(L *lua.LState) {
	installTablePack(L)
	installMathType(L)
	installUTF8Stub(L)
	installCoroutineCompat(L)
}

// installTablePack 补 table.pack / table.unpack。
//
// Lua 5.2 的 table.pack 返回带 n 字段的表：
//
//	local t = table.pack(1, nil, 3)
//	-- t = {1, nil, 3, n = 3}
//
// ★ n 字段是这套代码的关键：base.lua 的 assertType 用
// `typestrs.n` 决定循环次数，用 #t 会因为中间有 nil 而截断。
func installTablePack(L *lua.LState) {
	tbl := L.GetGlobal("table").(*lua.LTable)

	// table.pack(...)
	tbl.RawSetString("pack", L.NewFunction(func(l *lua.LState) int {
		n := l.GetTop()
		result := l.CreateTable(n, 1)
		for i := 1; i <= n; i++ {
			result.RawSetInt(i, l.Get(i))
		}
		result.RawSetString("n", lua.LNumber(n))
		l.Push(result)
		return 1
	}))

	// table.unpack(t [, i [, j]]) —— 5.1 的全局 unpack 是同一个东西
	tbl.RawSetString("unpack", L.NewFunction(func(l *lua.LState) int {
		t := l.CheckTable(1)
		i := l.OptInt(2, 1)
		j := l.OptInt(3, t.Len())
		if i > j {
			return 0
		}
		for k := i; k <= j; k++ {
			l.Push(t.RawGetInt(k))
		}
		return j - i + 1
	}))

	// 全局 unpack 也补上（Lua 5.1 本来就有，gopher-lua 也有，但确保存在）
	L.SetGlobal("unpack", L.GetField(tbl, "unpack"))
}

// installMathType 补 math.type（Lua 5.3+）。
//
// 脚本里到处是 `math.type(var) == 'integer'` 判断参数是整数还是浮点。
// gopher-lua 只有 LNumber（float64），所以要按「能否无损转成 int64」判定：
//
//	math.type(3)   -> 'integer'
//	math.type(3.5) -> 'float'
//	math.type(3.0) -> 'integer'  （与 Lua 5.3 不同，但更符合脚本意图）
//
// ★ 与真 Lua 5.3 的差异：5.3 里 3.0 是 float。这里判成 integer。
//
//	理由：脚本用 math.type 的目的是「区分整数 ID 和带小数的数值」，
//	3.0 当整数用不会出错；反过来把 3 判成 float 会让
//	assertType(x, 'integer') 大面积失败。
func installMathType(L *lua.LState) {
	mathTbl := L.GetGlobal("math").(*lua.LTable)
	mathTbl.RawSetString("type", L.NewFunction(func(l *lua.LState) int {
		v := l.Get(1)
		num, ok := v.(lua.LNumber)
		if !ok {
			l.Push(lua.LNil)
			return 1
		}
		f := float64(num)
		if f == float64(int64(f)) && f >= -9.007199254740992e15 && f <= 9.007199254740992e15 {
			l.Push(lua.LString("integer"))
		} else {
			l.Push(lua.LString("float"))
		}
		return 1
	}))

	// math.tointeger（5.3+）：能转就转，不能转返回 nil
	mathTbl.RawSetString("tointeger", L.NewFunction(func(l *lua.LState) int {
		num, ok := l.Get(1).(lua.LNumber)
		if !ok {
			l.Push(lua.LNil)
			return 1
		}
		f := float64(num)
		if f == float64(int64(f)) {
			l.Push(lua.LNumber(int64(f)))
		} else {
			l.Push(lua.LNil)
		}
		return 1
	}))

	// math.maxinteger / math.mininteger 常量
	mathTbl.RawSetString("maxinteger", lua.LNumber(9223372036854775807))
	mathTbl.RawSetString("mininteger", lua.LNumber(-9223372036854775808))
}

// installUTF8Stub 补一个最小可用的 utf8 库。
//
// NPC 脚本里用 utf8.len 算字数（决定文本换行）。
// 完整 UTF-8 库工作量不小，这里先实现最常用的几个，
// 其余返回保守值 —— 宁可字数算错也不让脚本崩。
func installUTF8Stub(L *lua.LState) {
	utf8Tbl := L.NewTable()

	// utf8.len(s) —— 返回字符数
	utf8Tbl.RawSetString("len", L.NewFunction(func(l *lua.LState) int {
		s := l.CheckString(1)
		l.Push(lua.LNumber(countRunes(s)))
		return 1
	}))

	// utf8.char(...) —— 把码点转字符串
	utf8Tbl.RawSetString("char", L.NewFunction(func(l *lua.LState) int {
		n := l.GetTop()
		var buf []rune
		for i := 1; i <= n; i++ {
			buf = append(buf, rune(l.CheckInt(i)))
		}
		l.Push(lua.LString(string(buf)))
		return 1
	}))

	// utf8.offset(s, n [, i]) —— 第 n 个字符的字节偏移
	utf8Tbl.RawSetString("offset", L.NewFunction(func(l *lua.LState) int {
		s := l.CheckString(1)
		n := l.CheckInt(2)
		if n == 0 {
			l.Push(lua.LNumber(1))
			return 1
		}
		idx := 0
		if n < 0 {
			// 从末尾数
			runes := []rune(s)
			pos := len(runes) + n + 1
			if pos < 1 || pos > len(runes) {
				l.Push(lua.LNil)
				return 1
			}
			for i := 0; i < pos-1; i++ {
				idx += len(string(runes[i]))
			}
			l.Push(lua.LNumber(idx + 1))
			return 1
		}
		for i := 0; i < n-1; i++ {
			if idx >= len(s) {
				l.Push(lua.LNil)
				return 1
			}
			idx++
			for idx < len(s) && (s[idx]&0xC0) == 0x80 {
				idx++
			}
		}
		l.Push(lua.LNumber(idx + 1))
		return 1
	}))

	L.SetGlobal("utf8", utf8Tbl)
}

// countRunes 统计 UTF-8 字符数（无效字节按 1 个算，不报错）。
func countRunes(s string) int {
	n := 0
	for i := 0; i < len(s); {
		n++
		if s[i] < 0x80 {
			i++
		} else if s[i]&0xE0 == 0xC0 {
			i += 2
		} else if s[i]&0xF0 == 0xE0 {
			i += 3
		} else if s[i]&0xF8 == 0xF0 {
			i += 4
		} else {
			i++
		}
	}
	return n
}

// installCoroutineCompat 修正 coroutine.running 的 5.1/5.2 语义差异。
//
// Lua 5.1:  coroutine.running() -> thread | nil      （主线程返回 nil）
// Lua 5.2+: coroutine.running() -> thread, ismain    （主线程返回 (main, true)）
//
// 原版沙箱脚本写的是 5.2 语义：
//
//	local threadId, inMainThread = coroutine.running()
//	if inMainThread then error('call getTLSTable() in main thread') end
//
// gopher-lua 是 5.1 语义，主线程返回 nil，于是 inMainThread 也是 nil，
// 判断成立 → 不报错 → 继续走 _RSVD_NAME_G_threadSandBox[nil]，那是个
// 合法但所有主线程共享的槽位，TLS 就退化成全局变量。
//
// 症状很隐蔽：单个玩家对话正常，两个玩家同时点同一个 NPC 时
// 后者的 uid 覆盖前者，对话内容串号。
//
// 修法：包装 running，让它返回 5.2 形态。
func installCoroutineCompat(L *lua.LState) {
	coTbl := L.GetGlobal("coroutine").(*lua.LTable)
	orig := coTbl.RawGetString("running")

	coTbl.RawSetString("running", L.NewFunction(func(l *lua.LState) int {
		l.Push(orig)
		if err := l.PCall(0, 2, nil); err != nil {
			l.Push(lua.LNil)
			l.Push(lua.LTrue)
			return 2
		}
		// gopher-lua 返回 (thread, nil) 表示协程内，(nil, nil) 表示主线程
		co := l.Get(-2)
		ismain := l.Get(-1)
		l.Pop(2)

		// 归一成 5.2 形态
		if co == lua.LNil || ismain == lua.LTrue {
			l.Push(lua.LNil)
			l.Push(lua.LTrue)
			return 2
		}
		l.Push(co)
		l.Push(lua.LFalse)
		return 2
	}))
}
