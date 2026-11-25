// npccheck 验证原版 NPC 脚本能否在 Go 侧加载与执行。
//
// 这不是「测试」而是**验证工具**：原版 341 个 NPC 脚本要在
// Lua 解释器里跑起来，缺一个绑定函数就会整批失败。
// 用它一眼看出「哪些能跑、哪些缺什么」。
//
// 用法：
//
//	go run ./tools/npccheck            # 全量扫描
//	go run ./tools/npccheck 道馆_1     # 只看一张图
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"

	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
)

// printHost 把脚本产出的 XML 打到控制台。
type printHost struct {
	verbose bool
}

func (h printHost) PostXML(uid uint64, path, xml string) error {
	if h.verbose {
		fmt.Printf(">>> SM_NPCXMLLAYOUT uid=%d path=%q\n%s\n", uid, path, xml)
	}
	return nil
}
func (printHost) PostInvOpCost(uint64, int, uint32, uint32, uint32) error { return nil }
func (printHost) PostStartInvOp(uint64, int, string, string, []string) error {
	return nil
}
func (printHost) PostStartInput(uint64, string, string, bool) error   { return nil }
func (printHost) PostSell(uint64, []uint32, []script.InvOpCost) error { return nil }
func (printHost) QueryName(uint64) (string, bool)                     { return "测试角色", true }
func (printHost) QueryLevel(uint64) (int, bool)                       { return 22, true }
func (printHost) QueryGold(uint64) (uint64, bool)                     { return 5000, true }
func (printHost) QueryRedName(uint64) bool                            { return false }
func (printHost) QueryItemDuration(uint64, uint32, uint32) (int, int, bool) {
	return 0, 0, false
}
func (printHost) RepairItem(uint64, uint32, uint32, bool, uint64) bool { return true }
func (printHost) SecureItem(uint64, uint32, uint32) bool               { return true }
func (printHost) PostSecuredItemList(uint64) error                     { return nil }
func (printHost) RemoveGold(uint64, uint64) bool                       { return true }
func (printHost) GrantGold(uint64, uint64) bool                        { return true }
func (printHost) SpaceMove(uint64, uint32, int, int) error             { return nil }
func (h printHost) Log(level int, msg string) {
	if h.verbose {
		fmt.Printf("[lua %d] %s\n", level, msg)
	}
}

func main() {
	root := script.ResolveScriptRoot()
	fw := script.ResolveFrameworkDir()
	if root == "" {
		fmt.Println("找不到原版脚本目录，请设 MIR2X_SCRIPT")
		os.Exit(1)
	}

	byMap, err := script.ScanNPCDir(root + "/npc")
	if err != nil {
		fmt.Println("扫描失败:", err)
		os.Exit(1)
	}

	onlyMap := ""
	if len(os.Args) > 1 {
		onlyMap = os.Args[1]
	}

	var list []script.NPCDef
	var mapNames []string
	for m, defs := range byMap {
		if onlyMap != "" && m != onlyMap {
			continue
		}
		mapNames = append(mapNames, m)
		list = append(list, defs...)
	}
	sort.Strings(mapNames)
	fmt.Printf("地图 %d 张，NPC 共 %d 个\n", len(mapNames), len(list))
	if len(list) == 0 {
		fmt.Println("没有 NPC")
		os.Exit(1)
	}

	// 记录表
	itemsTbl := loadNameTable("content/items.json", "")
	mapsTbl := script.NewNameTable(loadIncNames("E:/Desktop/game/mir2x/common/src/maprecord.inc"))
	monstersTbl := script.NewNameTable(loadIncNames("E:/Desktop/game/mir2x/common/src/monsterrecord.inc"))
	fmt.Printf("记录表：物品 %d / 怪物 %d / 地图 %d\n",
		itemsTbl.Count(), monstersTbl.Count(), mapsTbl.Count())

	var logFn func(int, string)
	common := script.CommonBindings(itemsTbl, monstersTbl, mapsTbl, func(level int, msg string) {
		if logFn != nil {
			logFn(level, msg)
		}
	})

	okCount, emptyCount := 0, 0
	var failed []string

	for _, def := range list {
		// 空脚本是原版占位（179 个），不算失败
		info, err := os.Stat(def.ScriptPath)
		if err != nil || info.Size() == 0 {
			emptyCount++
			continue
		}

		npc, err := script.LoadNPC(script.Options{
			FrameworkDir: fw,
			ScriptRoot:   root,
			Consts:       script.DefaultConsts(),
			Bindings:     common,
		}, script.NPCInfo{
			Name: def.Name, MapName: def.MapName, ScriptPath: def.ScriptPath,
			X: def.X, Y: def.Y, LookID: def.LookID, GfxDir: uint8(def.GfxDir),
		}, printHost{})
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", def.Name, err))
			continue
		}
		if !npc.HasEventHandler() {
			failed = append(failed, def.Name+": 没有事件处理器")
			npc.Close()
			continue
		}
		okCount++
		npc.Close()
	}

	fmt.Printf("\n=== 非空脚本 %d 个：成功 %d / 失败 %d；空占位 %d 个 ===\n",
		okCount+len(failed), okCount, len(failed), emptyCount)
	for _, f := range failed {
		fmt.Println("  FAIL", f)
	}

	// 挑一个 NPC 跑 SYS_ENTER，展示真实产出
	runDemo(root, fw, common, list)
}

// runDemo 跑一个 NPC 的点击事件，打印它产出的对话 XML。
func runDemo(root, fw string, common map[string]lua.LGFunction, list []script.NPCDef) {
	demoName := "阿天_1"
	for _, def := range list {
		if def.Name != demoName {
			continue
		}
		npc, err := script.LoadNPC(script.Options{
			FrameworkDir: fw, ScriptRoot: root,
			Consts:   script.DefaultConsts(),
			Bindings: common,
		}, script.NPCInfo{
			Name: def.Name, MapName: def.MapName, ScriptPath: def.ScriptPath,
			X: def.X, Y: def.Y, LookID: def.LookID, GfxDir: uint8(def.GfxDir),
		}, printHost{verbose: true})
		if err != nil {
			fmt.Println("加载 demo 失败:", err)
			return
		}
		defer npc.Close()
		fmt.Printf("\n=== 跑 %s.%s 的 SYS_ENTER ===\n", def.MapName, def.Name)
		if err := npc.HandleEvent(12345, "", script.SysEnter, nil); err != nil {
			fmt.Println("执行失败:", err)
		}
		return
	}
}

// loadNameTable 从 JSON 读名字表；incPath 非空时改用 inc 解析。
func loadNameTable(jsonPath, incPath string) *script.NameTable {
	if incPath != "" {
		return script.NewNameTable(loadIncNames(incPath))
	}
	tbl, err := script.LoadNameTableFromJSON(jsonPath)
	if err != nil {
		fmt.Println("读记录表失败:", jsonPath, err)
		return script.NewNameTable(nil)
	}
	return tbl
}

// loadIncNames 从 maprecord.inc / monsterrecord.inc 提取「ID -> 名字」。
func loadIncNames(path string) map[uint32]string {
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("读 inc 失败:", path, err)
		return nil
	}
	if strings.Contains(path, "monsterrecord") {
		return script.ParseMonsterRecordInc(string(raw))
	}
	return script.ParseMapRecordInc(string(raw))
}
