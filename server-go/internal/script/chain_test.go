package script

import (
	"strings"

	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// fakeMap 模拟地图 actor：回答 getNPCharUID。
type fakeMap struct {
	uid     uint64
	npcs    map[string]uint64
	bus     *RPCBus
	gridSeq int64
}

func (m *fakeMap) ExecRemote(code string, args []protocol.LuaVar) RPCResult {
	// 格子触发器注册（quest.lua:1696）
	if strings.Contains(code, "addQuestGridTrigger") {
		m.gridSeq++
		return RPCResult{Values: []protocol.LuaVar{protocol.NewLuaIntegerVar(m.gridSeq)}}
	}
	if len(args) == 0 || args[0].Kind != protocol.LuaVarString {
		fmt.Printf("[fakeMap map=%d] ★ 参数不是字符串: %v\n", m.uid, args)
		return RPCResult{Values: []protocol.LuaVar{protocol.NewLuaNilVar()}}
	}
	if _, ok := m.npcs[args[0].String]; !ok {
		fmt.Printf("[fakeMap map=%d] ★ 找不到 NPC %q (共%d个) args总数=%d\n", m.uid, args[0].String, len(m.npcs), len(args))
		for i, a := range args {
			fmt.Printf("   args[%d] kind=%v val=%v\n", i, a.Kind, a.String)
		}
	}
	if uid, ok := m.npcs[args[0].String]; ok {
		return RPCResult{Values: []protocol.LuaVar{
			protocol.NewLuaIntegerVar(m.bus.Short(uid)),
		}}
	}
	return RPCResult{Values: []protocol.LuaVar{protocol.NewLuaNilVar()}}
}

// fakeNPC 模拟 NPC actor：记录收到的远程调用。
type fakeNPC struct {
	name  string
	mu    chan struct{}
	calls int
}

func (n *fakeNPC) ExecRemote(code string, args []protocol.LuaVar) RPCResult {
	n.calls++
	return RPCResult{Values: []protocol.LuaVar{protocol.NewLuaIntegerVar(1)}}
}

// TestAllQuestsCrossState 验证全部 42 个任务在「独立 actor + 跨状态 RPC」下能加载。
//
// ★ 这是 B 方案（对齐原版 actor 模型）的验收测试：
//
//	每个任务一个独立 Lua 状态 + 独立 UID
//	任务能通过地图 actor 找到 NPC
//	任务能把 setQuestHandler 路由到 NPC 上
func TestAllQuestsCrossState(t *testing.T) {
	raw, err := os.ReadFile("E:/Desktop/game/mir2x/common/src/maprecord.inc")
	if err != nil {
		t.Skip("maprecord.inc 不可用")
	}
	questMapNames = NewNameTable(ParseMapRecordInc(string(raw)))
	activeBus = nil

	bus := NewRPCBus(3*time.Second, func(f string, a ...any) {
		t.Logf("RPC: "+f, a...)
	})
	activeBus = bus

	root := "E:/Desktop/game/mir2x/server/script"

	// ★ 用真实的 NPC 脚本目录构建 地图→NPC 映射
	//
	//	任务脚本会调用 getNPCharUID('比奇县_0', '客栈店员_1')，
	//	这里必须让 fakeMap 能真的回答出来。
	npcByMap, err2 := ScanNPCDir(filepath.Join(root, "npc"))
	if err2 != nil {
		t.Skipf("scan npc dir: %v", err2)
	}

	// 按地图名建 actor
	mapNameToID := map[string]uint32{}
	for mapID := uint32(1); mapID < 2000; mapID++ {
		if name, ok := questMapNames.NameOf(mapID); ok && name != "" {
			mapNameToID[name] = mapID
		}
	}

	// ★ 先为**全部地图**注册 actor（即使没有 NPC）
	//
	//	真实场景里地图 actor 总是存在（玩家进图就加载），
	//	只是可能没有 NPC。任务脚本里的 clearMapGridTrigger 之类
	//	会对「没 NPC 的地图」也发 uidRemoteCall。
	registered := map[string]bool{}
	for mapName, mapID := range mapNameToID {
		mapUID, err := common.GetBaseMapUID(mapID, 0)
		if err != nil {
			continue
		}
		bus.Register(mapUID, &fakeMap{uid: mapUID, bus: bus, npcs: map[string]uint64{}})
		registered[mapName] = true
	}
	fmt.Printf("已注册 %d 张地图 actor\n", len(registered))

	npcTotal := 0
	for mapName, defs := range npcByMap {
		mapID, ok := mapNameToID[mapName]
		if !ok {
			continue
		}
		mapUID, err := common.GetBaseMapUID(mapID, 0)
		if err != nil {
			continue
		}
		npcs := make(map[string]uint64, len(defs))
		for i, d := range defs {
			if d.Name == "" {
				continue
			}
			// 造一个稳定的假 UID
			npcUID := uint64(0x0800000000000000) | uint64(mapID)<<16 | uint64(i+1)
			npcs[d.Name] = npcUID
			bus.Register(npcUID, &fakeNPC{name: d.Name})
			npcTotal++
		}
		bus.Register(mapUID, &fakeMap{uid: mapUID, bus: bus, npcs: npcs}) // 覆盖：带 NPC 的版本
	}
	fmt.Printf("已注册地图 actor + %d 个 NPC actor\n", npcTotal)
	// 诊断：道馆_1 上有没有大悲善僧_1
	if defs, ok := npcByMap["道馆_1"]; ok {
		found := false
		for _, d := range defs {
			if d.Name == "大悲善僧_1" {
				found = true
			}
		}
		fmt.Printf("道馆_1 NPC 数=%d, 含大悲善僧_1=%v\n", len(defs), found)
	} else {
		fmt.Printf("★ 道馆_1 没有 NPC 条目\n")
	}
	if id, ok := mapNameToID["道馆_1"]; ok {
		fmt.Printf("道馆_1 → mapID=%d\n", id)
	} else {
		fmt.Printf("★ 道馆_1 不在 mapNameToID 里\n")
	}
	files, _ := filepath.Glob(filepath.Join(root, "quest", "*.lua"))

	loaded, failed := 0, 0
	var errs []string
	for i, f := range files {
		opts := Options{FrameworkDir: "../../content/script", ScriptRoot: root, Consts: DefaultConsts()}
		// ★ 任务名不能带 .lua 后缀
		//
		//	原版 getQuestName() 返回的是脚本名去掉扩展名。
		//	带后缀会让 quest.lua 的 getNPCharUID 把「任务名」当 NPC 名用，
		//	报错还会显示「找不到 NPC "困魔咒任务.lua"」—— 完全看不出是命名问题。
		questName := strings.TrimSuffix(filepath.Base(f), ".lua")
		q, err := LoadQuest(opts, questName, f, nil, bus, uint32(i+1))
		if err != nil {
			failed++
			errs = append(errs, fmt.Sprintf("%s: %v", filepath.Base(f), err))
			continue
		}
		defer q.Close()
		loaded++
	}

	fmt.Printf("任务加载: %d 成功 / %d 失败\n", loaded, failed)
	for i, e := range errs {
		if i < 3 {
			fmt.Printf("  FAIL %s\n", e)
		}
	}
	fmt.Printf("短 ID 映射数: %d\n", bus.ShortIDs().Count())

	if failed > 0 {
		t.Fatalf("%d 个任务加载失败", failed)
	}

}
