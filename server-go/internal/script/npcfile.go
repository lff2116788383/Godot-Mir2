package script

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

// ==================== NPC 脚本发现（servermap.cpp:1701 loadNPChar）====================
//
// 文件名格式（原版注释写得非常清楚）：
//
//	<地图名>.GLOC_x_y_dir.<NPC名>.LOOK_id.lua
//
// 实测例子：
//
//	道馆_1.阿天_1.GLOC_396_116_0.LOOK_18.lua
//	比奇县_0.屠夫.GLOC_445_400_2.LOOK_11.lua
//
// 注意「地图名」里带下划线（道馆_1 是完整地图名，不是 道馆 + 1），
// 所以正则不能用「第一个下划线切分」，必须按 .GLOC_ 定位。
//
// ★ 为什么用正则而不是 split：
//
//	地图名 "道馆_1" 含下划线，NPC 名 "阿天_1" 也含下划线，
//	按 "_" 切分得到的段数不固定。用 .GLOC_x_y_d.LOOK_n.lua 这个
//	固定后缀反向定位，才是原版的做法（用的是 std::regex_match）。

// npcFileRe 是原版的文件名正则（servermap.cpp:1713）。
//
//	分组 1 = NPC 名
//	分组 2,3 = 格坐标
//	分组 4 = NPC 朝向（可能不是 8 向，取值 0..7）
//	分组 5 = LOOK ID
var npcFileRe = regexp.MustCompile(`^(.*)\.GLOC_(\d+)_(\d+)_(\d+)\.LOOK_(\d+)\.lua$`)

// NPCDef 是一个 NPC 的静态定义（还没加载脚本）。
type NPCDef struct {
	// MapName 地图名（从文件名取，如「道馆_1」）
	MapName string
	// Name NPC 名（如「阿天_1」）
	Name string
	// X, Y 格坐标
	X, Y int
	// GfxDir 朝向 0..7
	GfxDir int
	// LookID 外观 ID
	LookID uint32
	// ScriptPath 脚本绝对路径
	ScriptPath string
}

// ParseNPCFileName 解析一个 NPC 脚本文件名。
func ParseNPCFileName(fileName string) (NPCDef, bool) {
	m := npcFileRe.FindStringSubmatch(fileName)
	if m == nil {
		return NPCDef{}, false
	}
	// m[1] 是「地图名.NPC名」整体，需要再切
	prefix := m[1]
	// 从右往左找第一个 '.'：NPC 名里不可能含 '.'，
	// 但地图名理论上可以，所以用 LastIndex 而不是 Index
	dot := -1
	for i := len(prefix) - 1; i >= 0; i-- {
		if prefix[i] == '.' {
			dot = i
			break
		}
	}
	if dot <= 0 || dot >= len(prefix)-1 {
		return NPCDef{}, false
	}

	x, err1 := strconv.Atoi(m[2])
	y, err2 := strconv.Atoi(m[3])
	dir, err3 := strconv.Atoi(m[4])
	look, err4 := strconv.Atoi(m[5])
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return NPCDef{}, false
	}

	return NPCDef{
		MapName: prefix[:dot],
		Name:    prefix[dot+1:],
		X:       x,
		Y:       y,
		GfxDir:  dir,
		LookID:  uint32(look),
	}, true
}

// ScanNPCDir 扫描 NPC 脚本目录，返回按地图分组的结果。
//
// 目录结构（server/script/npc/）：
//
//	道馆_1.阿天_1.GLOC_396_116_0.LOOK_18.lua
//	比奇县_0.屠夫.GLOC_445_400_2.LOOK_11.lua
//	...
//
// 341 个文件全部平铺在一个目录里（原版也是）。
func ScanNPCDir(dir string) (map[string][]NPCDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read npc dir %s: %w", dir, err)
	}

	byMap := make(map[string][]NPCDef)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".lua" {
			continue
		}
		def, ok := ParseNPCFileName(name)
		if !ok {
			// 不符合命名规范的文件直接跳过（原版是 throw，
			// 但目录里可能有 README 之类，跳过更稳）
			continue
		}
		def.ScriptPath = filepath.Join(dir, name)
		byMap[def.MapName] = append(byMap[def.MapName], def)
	}

	// 按坐标排序，保证每次启动的 NPC 顺序一致
	for _, list := range byMap {
		sort.Slice(list, func(i, j int) bool {
			if list[i].Y != list[j].Y {
				return list[i].Y < list[j].Y
			}
			if list[i].X != list[j].X {
				return list[i].X < list[j].X
			}
			return list[i].Name < list[j].Name
		})
	}
	return byMap, nil
}

// NPCCountByMap 返回每个地图的 NPC 数量（诊断用）。
func NPCCountByMap(byMap map[string][]NPCDef) map[string]int {
	out := make(map[string]int, len(byMap))
	for name, list := range byMap {
		out[name] = len(list)
	}
	return out
}

// TotalNPCCount 返回 NPC 总数。
func TotalNPCCount(byMap map[string][]NPCDef) int {
	total := 0
	for _, list := range byMap {
		total += len(list)
	}
	return total
}

// ResolveScriptRoot 解析原版脚本根目录（内含 npc/ include/ api/ 等子目录）。
//
// 优先环境变量 MIR2X_SCRIPT，其次几个约定位置。
//
// ★ 为什么不把 341 个 NPC 脚本复制进仓库：
//
//	它们合计约 10 万行，复制进来就会产生「原版更新了但副本没同步」
//	的漂移。直接指向原版目录，脚本修正立刻生效，
//	「是否与原文一致」也变成 diff 就能回答的问题。
//
//	只有 base.lua / npchar.lua 两份基础设施脚本是复制的 ——
//	它们是运行时的约定（Go 侧的绑定函数名要和它们对上），
//	必须有稳定副本，并用 md5 校验未被改动。
func ResolveScriptRoot() string {
	if root := os.Getenv("MIR2X_SCRIPT"); root != "" {
		return root
	}
	candidates := []string{
		"E:/Desktop/game/mir2x/server/script",
		"../../mir2x/server/script",
		"../mir2x/server/script",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}

// ResolveFrameworkDir 返回基础脚本目录（本仓库内的 content/script）。
func ResolveFrameworkDir() string {
	if dir := os.Getenv("MIR2X_LUA_FRAMEWORK"); dir != "" {
		return dir
	}
	candidates := []string{"content/script", "../../content/script"}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return "content/script"
}
