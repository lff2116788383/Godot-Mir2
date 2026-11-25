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

// patchAndRun 读文件 → 语法降级 → 执行。
//
// 与 DoFile 的差别只在中间多一步改写。行数不变，
// 所以脚本报错时的行号仍对应原文件。
func (rt *Runtime) patchAndRun(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(raw)
	patched, count := patchIntDiv(src)
	if count == 0 {
		return rt.L.DoFile(path)
	}
	// 加一行 shebang 注释说明被降级过，方便排查
	patched = "-- [mir2x-go] " + fmt.Sprint(count) + " 处 `//` 已降级为 math.floor\n" + patched
	if err := rt.L.DoString(patched); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
