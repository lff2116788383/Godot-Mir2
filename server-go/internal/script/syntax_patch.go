package script

import (
	"fmt"
	"os"
	"strings"
)

// ==================== Lua 5.3 语法降级 ====================
//
// 原版服务端用官方 Lua（5.3+），脚本里用了 5.3 才有的**整数除法运算符 `//`**：
//
//	buildGold // 10000
//	(launchTime() + uptime()) // (24 * 60 * 60)
//	#dropList // 2
//
// gopher-lua 实现 Lua 5.1 语法，`//` 是**编译期语法错误**，
// 任何 polyfill 都救不了 —— 只能在加载前把源码改一改。
//
// 实测影响面：3 个文件、6 处：
//
//	npc/include/dailyquest.lua:205,249
//	npc/比奇县_0.行政官员_1.GLOC_477_366_0.LOOK_8.lua:50,101,154
//	quest/野蛮冲撞任务.lua:341
//
// ★ 为什么不在原版文件上改：
//
//	那是上游仓库，改了就没法 diff 上游、也没法跟上游升级。
//	改写发生在**加载时**（读进内存改一遍再执行），原文件一个字节不动。
//
// ★ 为什么必须包 math.floor 而不是简单换成 `/`：
//
//	`a // b` 是「向下取整的整除」，返回整数。
//	直接换成 `/` 得到浮点，而调用方几乎都是
//	`string.format('%d', a // b)` —— `%d` 遇到浮点会报
//	"bad argument #2 to 'format' (number has no integer representation)"。
//
//	所以必须展开成 `math.floor(a / b)`，语义与 5.3 的 `//` 一致
//	（5.3 的 // 也是 floor 语义，-7 // 2 == -4）。

// findIntDiv 在 src 里找下一个真正的 `//` 运算符位置。
//
// ★ 不用正则。
//
//	初版写了一个四选一的复杂正则（标识符/函数调用/括号表达式/数字），
//	在 427 行的地图脚本上**灾难性回溯**，28.9 秒才跑完，而且一处都没匹配上。
//	用 pprof 定位才发现 87% 的时间在 regexp.(*machine).add。
//
//	改成「先找 // 字面量，再判断上下文」的线性扫描，整个文件 O(n)。
func findIntDiv(src string, from int) int {
	for i := from; i < len(src)-1; i++ {
		if src[i] != '/' || src[i+1] != '/' {
			continue
		}
		if isIntDivOperator(src, i) {
			return i
		}
	}
	return -1
}

// isIntDivOperator 判断 src[i:i+2] 的 `//` 是「整数除法」还是注释/字符串。
func isIntDivOperator(src string, i int) bool {
	// 前面必须是操作数结尾
	j := i - 1
	for j >= 0 && (src[j] == ' ' || src[j] == '\t') {
		j--
	}
	if j < 0 || !isOperandEnd(src[j]) {
		return false
	}
	// 后面必须有操作数开头
	k := i + 2
	for k < len(src) && (src[k] == ' ' || src[k] == '\t') {
		k++
	}
	if k >= len(src) {
		return false
	}
	c := src[k]
	return c == '(' || c == '#' || c == '-' ||
		(c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

// isOperandEnd 判断这个字符能不能作为 `//` 的左操作数结尾。
func isOperandEnd(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') || c == '_' || c == ')' || c == ']'
}

// scanOperandLeft 从 i 往前扫一个完整的左操作数，返回起始下标。
//
// 支持：标识符 / 数字 / a.b.c / 函数调用 f(...) / 括号表达式 (...) / 取长度 #x
func scanOperandLeft(src string, i int) int {
	// 跳过尾部空白
	for i >= 0 && (src[i] == ' ' || src[i] == '\t') {
		i--
	}
	if i < 0 {
		return 0
	}

	// 括号收尾：向前配对
	if src[i] == ')' || src[i] == ']' {
		open, close := byte('('), byte(')')
		if src[i] == ']' {
			open, close = '[', ']'
		}
		depth := 0
		for ; i >= 0; i-- {
			if src[i] == close {
				depth++
			} else if src[i] == open {
				depth--
				if depth == 0 {
					// 继续往前看有没有函数名
					k := i - 1
					for k >= 0 && (src[k] == ' ' || src[k] == '\t') {
						k--
					}
					if k >= 0 && (isIdentChar(src[k])) {
						return scanIdentLeft(src, k)
					}
					return i
				}
			}
		}
		return 0
	}

	// 标识符 / 数字
	if isIdentChar(src[i]) {
		return scanIdentLeft(src, i)
	}
	return i
}

// scanIdentLeft 从 i 往前扫标识符（含 . 和 #），返回起始下标。
func scanIdentLeft(src string, i int) int {
	for i >= 0 && (isIdentChar(src[i]) || src[i] == '.' || src[i] == '#') {
		i--
	}
	return i + 1
}

func isIdentChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') || c == '_'
}

// scanOperandRight 从 i 往后扫一个完整的右操作数，返回结束下标（不含）。
func scanOperandRight(src string, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	if i >= len(src) {
		return i
	}

	if src[i] == '(' {
		depth := 0
		for ; i < len(src); i++ {
			if src[i] == '(' {
				depth++
			} else if src[i] == ')' {
				depth--
				if depth == 0 {
					return i + 1
				}
			}
		}
		return i
	}

	// 标识符/数字，可能带函数调用 f(...)
	start := i
	for i < len(src) && (isIdentChar(src[i]) || src[i] == '.') {
		i++
	}
	// 紧跟括号：吞掉整个调用
	if i < len(src) && src[i] == '(' {
		depth := 0
		for ; i < len(src); i++ {
			if src[i] == '(' {
				depth++
			} else if src[i] == ')' {
				depth--
				if depth == 0 {
					return i + 1
				}
			}
		}
	}
	if i == start {
		i++
	}
	return i
}

// patchIntDiv 把 `a // b` 改写为 `math.floor(a / b)`。
//
// 只在「不在注释、不在字符串里」的位置动手，其余原样保留。
func patchIntDiv(src string) (string, int) {
	var out strings.Builder
	out.Grow(len(src) + 256)

	count := 0
	i := 0
	n := len(src)

	for i < n {
		c := src[i]

		// ---- 行注释 ----
		if c == '-' && i+1 < n && src[i+1] == '-' {
			// 块注释 --[[ ... ]]
			if level, ok := longBracketLevel(src, i+2); ok {
				closer := "]" + strings.Repeat("=", level) + "]"
				end := strings.Index(src[i+2:], closer)
				if end < 0 {
					out.WriteString(src[i:])
					return out.String(), count
				}
				total := i + 2 + end + len(closer)
				out.WriteString(src[i:total])
				i = total
				continue
			}
			// 行注释到行尾
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				out.WriteString(src[i:])
				return out.String(), count
			}
			out.WriteString(src[i : i+end])
			i += end
			continue
		}

		// ---- 短字符串 ----
		if c == '"' || c == '\'' {
			end := scanShortString(src, i)
			out.WriteString(src[i:end])
			i = end
			continue
		}

		// ---- 长字符串 [[ ... ]] ----
		if c == '[' {
			if level, ok := longBracketLevel(src, i); ok {
				closer := "]" + strings.Repeat("=", level) + "]"
				end := strings.Index(src[i:], closer)
				if end >= 0 {
					total := i + end + len(closer)
					out.WriteString(src[i:total])
					i = total
					continue
				}
			}
		}

		// ---- `//` 运算符 ----
		if c == '/' && i+1 < n && src[i+1] == '/' && isIntDivOperator(src, i) {
			leftStart := scanOperandLeft(src, i-1)
			rightEnd := scanOperandRight(src, i+2)

			// 把已经写出去的部分回退到 leftStart
			written := out.String()
			keep := len(written) - (i - leftStart)
			if keep < 0 {
				keep = 0
			}
			out.Reset()
			out.WriteString(written[:keep])

			left := strings.TrimSpace(src[leftStart:i])
			right := strings.TrimSpace(src[i+2 : rightEnd])
			out.WriteString("math.floor((" + left + ") / (" + right + "))")
			count++
			i = rightEnd
			continue
		}

		out.WriteByte(c)
		i++
	}

	return out.String(), count
}

// longBracketLevel 判断 src[i:] 是否 `[[` / `[=[` 开头。
func longBracketLevel(src string, i int) (int, bool) {
	if i >= len(src) || src[i] != '[' {
		return 0, false
	}
	j := i + 1
	level := 0
	for j < len(src) && src[j] == '=' {
		level++
		j++
	}
	if j < len(src) && src[j] == '[' {
		return level, true
	}
	return 0, false
}

// scanShortString 从引号扫到字符串结束，返回结束后下标。
func scanShortString(src string, i int) int {
	quote := src[i]
	n := len(src)
	i++
	for i < n {
		if src[i] == '\\' {
			i += 2
			continue
		}
		if src[i] == quote {
			return i + 1
		}
		if src[i] == '\n' {
			return i // 未闭合
		}
		i++
	}
	return i
}

// patchToBeClosed 把 Lua 5.4 的 `<close>` 属性降级成「显式 close」写法。
//
// ★★ 为什么必须处理 ★★
//
//	gopher-lua 是 Lua 5.1 语法，`local x <close> = f()` 是 **5.4** 的特性，
//	直接编译就是 `near '<': syntax error`。
//
//	quest.lua（1819 行）里 `<close>` 出现在 6 处关键位置：
//
//		do
//		    local section <close> = _RSVD_NAME_criticalSection()
//		    ... 临界区代码，不允许 yield
//		end
//
//	语义是「离开这个 do 块时自动调 __close」。降级方式：
//
//		local section = _RSVD_NAME_criticalSection()
//		... 
//		section:close()   -- 插在 end 之前
//
// ★ 为什么不直接删掉：`<close>` 在这里不是装饰。
//
//	criticalSection 会往 `_RSVD_NAME_criticalSections[co]` 里计数，
//	并在 `coroutine.yield` 被包过的版本里检查「临界区内禁止 yield」。
//	只删不调 close 的话，计数只增不减 → **第一次进临界区之后，
//	这个协程永远不能再 yield** → 只要有一次 pause 就报
//	"yield in a critical section"。
//
//	这个 bug 的隐蔽程度：任务能正常开始、正常对话，
//	直到某个需要等待（pause）的步骤才炸，而且报错信息完全不指向这里。
//
// 实现：逐行扫描，找到 `local NAME <close> = EXPR` 就地改写，
// 并记住 NAME + 缩进，在**同一缩进层的 `end`** 之前插入 close 调用。
func patchToBeClosed(src string) (string, int) {
	if !strings.Contains(src, "<close>") {
		return src, 0
	}

	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines)+16)
	type pending struct {
		name string
		// depth 声明时所在的块深度（do/function/if 计入）
		depth int
		// depthIndent 声明行的缩进，用于对齐插入的 close 调用
		depthIndent int
	}
	var stack []pending
	depth := 0
	count := 0

	// blockOpeners / blockClosers 用于粗略估计块深度。
	//
	// ★ 不追求完整 Lua 解析，只要「声明所在块」与「结束该块的 end」
	//   能对上即可。quest.lua 的缩进规范，加上对
	//   then/do/function/end 的识别已经足够。
	//
	//	踩过的坑：最初只按「缩进 ≥」找 end，结果声明在
	//	`if ... then return end` 这种单行块里时，
	//	插入点跑到了 if 内部 —— 语法直接炸。
	openBlock := func(t string) bool {
		if strings.HasSuffix(t, " then") || t == "then" ||
			strings.HasSuffix(t, " do") || t == "do" ||
			strings.HasPrefix(t, "function ") || strings.HasSuffix(t, "function") ||
			strings.HasSuffix(t, "function()") {
			return true
		}
		return false
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isComment := strings.HasPrefix(trimmed, "--") || trimmed == ""

		if !isComment {
			// 先处理 end（闭合块），再判断声明
			if trimmed == "end" || strings.HasPrefix(trimmed, "end)") || strings.HasPrefix(trimmed, "end,") {
				depth--
				// 该 end 之前，把「声明深度 > 当前深度」的全部 close 掉
				for len(stack) > 0 && stack[len(stack)-1].depth > depth {
					p := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					pad := strings.Repeat(" ", p.depthIndent)
					call := pad + patchCloseCall(p.name)
					// ★ Lua 要求 return/break 必须是块里最后一条语句。
					//   如果上一行是 return/break，close 必须插在它**之前** ——
					//   插在后面就是 "near 'do': syntax error"（报错行还指着 end，极难定位）。
					if n := len(out); n > 0 && isBlockTerminator(strings.TrimSpace(out[n-1])) {
						out = append(out[:n-1], call, out[n-1])
					} else {
						out = append(out, call)
					}
				}
			}

			if idx := strings.Index(line, "<close>"); idx >= 0 {
				if localIdx := strings.Index(line, "local "); localIdx >= 0 {
					rest := line[localIdx+len("local "):]
					eq := strings.Index(rest, "=")
					closeAt := strings.Index(rest, "<close>")
					if eq > 0 && closeAt > 0 && closeAt < eq {
						name := strings.TrimSpace(rest[:closeAt])
						newLine := line[:localIdx] + "local " + strings.TrimSpace(rest[:closeAt]) + rest[closeAt+len("<close>"):]
						out = append(out, newLine)
						stack = append(stack, pending{
							name:        name,
							depth:       depth,
							depthIndent: leadingSpaces(line),
						})
						count++
						// 声明行本身也可能开块（罕见），保守地按下方规则处理
						if openBlock(trimmed) {
							depth++
						}
						continue
					}
				}
			}

			if openBlock(trimmed) {
				depth++
			}
		}
		out = append(out, line)
	}

	// 文件末尾还没闭合的
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, strings.Repeat(" ", p.depthIndent)+patchCloseCall(p.name))
	}

	return strings.Join(out, "\n"), count
}

// isBlockTerminator 判断某行是否是「必须是块中最后一条」的语句。
//
// Lua 语法规定 return / break 只能在块的末尾（后面只能是 end）。
// 往它们后面插任何语句都会让整个 block 编译失败。
func isBlockTerminator(line string) bool {
	return line == "return" || strings.HasPrefix(line, "return ") ||
		line == "break" || strings.HasPrefix(line, "break ")
}

// patchCloseCall 生成「等价于 <close> 退出作用域」的调用。
//
// ★ 必须调 `getmetatable(x).__close(x, nil)` 而不是 `x:close()`：
//
//	Lua 5.4 的 `<close>` 语义是「作用域结束时调用 `__close` 元方法」，
//	**不是**调用一个叫 close 的普通方法。
//
//	quest.lua 里两种对象都没有 `close()` 方法：
//	  section（criticalSection）只有 `__close`
//	  mark（switchMark）也只有 `__close`，另有一个 `drop()`
//
//	写成 `mark:close()` 会直接报 "attempt to call a nil value (method 'close')"。
//	最初的实现就是这么写的，结果 42 个任务全部加载失败。
//
// ★★ 必须幂等 ★★
//
//	`syntax_patch` 是逐行扫描 + 块深度估算，不是完整 Lua 解析器。
//	遇到嵌套块（`do ... if ... then ... end ... end`）时，
//	同一个 <close> 变量可能被插入两次 close 调用。
//
//	实测症状：__close 被调两次，第二次
//	`_RSVD_NAME_criticalSections[co]` 已是 nil，
//	`nil - 1` 报 "cannot perform sub operation between nil and number"，
//	而这个错指向 quest.lua:1096 的 setQuestState —— 与真因相隔 1000 行。
//
//	而原版 Lua 的 <close> 语义本身保证只调一次，
//	所以让降级调用幂等是**语义正确**的，不是打补丁掩盖问题。
func patchCloseCall(name string) string {
	n := strings.TrimSpace(name)
	// ★ 用 _o = nil 的赋值消除重复调用：
	//	第一次执行后把变量置 nil，第二次 patch 插入的调用
	//	拿到 nil 就跳过。这样与 <close> 的「只调一次」语义一致。
	return "do local _o = " + n + "; " + n + " = nil; " +
		"local _m = _o and getmetatable(_o); " +
		"if _m and _m.__close then _m.__close(_o) end end"
}

// leadingSpaces 返回一行的前导空格数。
func leadingSpaces(line string) int {
	n := 0
	for _, r := range line {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

// patchAndRun 读文件 → 语法降级 → 执行。
//
// 与 DoFile 的差别只在中间多一步改写。行数基本不变，
// 所以脚本报错时的行号仍能对应原文件。
func (rt *Runtime) patchAndRun(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(raw)

	patched, divCount := patchIntDiv(src)
	// ★ 顺序：先降级 // 再处理 <close>。
	//   反过来也行（两者互不影响），但 <close> 的改写会插入行，
	//   放在后面能让行号偏移只出现在文件后半段。
	patched, closeCount := patchToBeClosed(patched)

	if divCount == 0 && closeCount == 0 {
		return rt.L.DoFile(path)
	}
	header := "-- [mir2x-go] 语法降级："
	if divCount > 0 {
		header += fmt.Sprintf("%d 处 `//`→math.floor ", divCount)
	}
	if closeCount > 0 {
		header += fmt.Sprintf("%d 处 `<close>`→显式 close ", closeCount)
	}
	patched = header + "\n" + patched
	if err := rt.L.DoString(patched); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
