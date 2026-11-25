package script

import (
	"fmt"
	"os"
	"regexp"
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

// intDivRe 匹配 `简单表达式 // 简单表达式`。
//
// 允许的操作数形态（覆盖实测的全部 6 处）：
//
//	标识符 / 数字 / 函数调用 / 括号表达式 / 取长度表达式
//	例：buildGold、10000、#dropList、2、(launchTime() + uptime())
//
// 用两个捕获组分别抓左右操作数，改写成 math.floor(L / R)。
var intDivRe = regexp.MustCompile(
	`(\#[A-Za-z_][A-Za-z0-9_\.]*|` + // #var
		`[A-Za-z_][A-Za-z0-9_\.]*(?:\([^()\n]*\))?|` + // ident / ident(args) / a.b
		`\([^()\n]*(?:\([^()\n]*\)[^()\n]*)*\)|` + // (nested)
		`\d+(?:\.\d+)?)` + // number
		`[ \t]*//[ \t]*` +
		`(\#[A-Za-z_][A-Za-z0-9_\.]*|` +
		`[A-Za-z_][A-Za-z0-9_\.]*(?:\([^()\n]*\))?|` +
		`\([^()\n]*(?:\([^()\n]*\)[^()\n]*)*\)|` +
		`\d+(?:\.\d+)?)`)

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

		// ---- 尝试匹配整数除法 ----
		if loc := intDivRe.FindStringSubmatchIndex(src[i:]); loc != nil && loc[0] == 0 {
			left := src[i+loc[2] : i+loc[3]]
			right := src[i+loc[4] : i+loc[5]]
			out.WriteString("math.floor((" + left + ") / (" + right + "))")
			count++
			i += loc[1]
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
