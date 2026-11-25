package script

import (
	"os"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// TestMapGridTrigger 验证地图 Lua 状态里的格子触发器**真的能用**。
//
// ★ 这是「困魔咒任务 / 群体治愈术任务」能否真正生效的唯一硬指标：
//
//	它们通过 uidRemoteCall 到地图 actor 执行
//	`addQuestGridTrigger(questName, rectList, handler)`，
//	那个函数定义在 servermap.lua。
//
//	验证四件事：
//	  1. servermap.lua 的 API 都装了（不是 nil）
//	  2. 分配到的 ID 非 0（之前是恒 0，所有触发器互相覆盖）
//	  3. 分配的矩形真的被索引了（at(x,y) 能查到）
//	  4. deleteGridTrigger 能删掉
func TestMapGridTrigger(t *testing.T) {
	root := "E:/Desktop/game/mir2x/server/script"
	if _, err := os.Stat(root); err != nil {
		t.Skip("原版脚本目录不可用")
	}

	host := &stubMapHost{}
	ms, err := LoadMapScript(Options{
		FrameworkDir: "../../content/script",
		ScriptRoot:   root,
		Consts:       DefaultConsts(),
		Bindings: CommonBindings(NewNameTable(nil), NewNameTable(nil),
			NewNameTable(nil), nil),
	}, "比奇县_0", host)
	if err != nil {
		t.Fatalf("加载地图脚本: %v", err)
	}
	defer ms.Close()

	L := ms.Runtime.L
	for _, name := range []string{
		"addGridTrigger", "addQuestGridTrigger", "addUIDGridTrigger",
		"deleteGridTrigger", "runMapThread", "_RSVD_NAME_runGridTrigger",
		"_RSVD_NAME_allocateGridTriggerId", "_RSVD_NAME_getGridTriggerIDList",
	} {
		if _, ok := L.GetGlobal(name).(*lua.LFunction); !ok {
			t.Fatalf("★ servermap.lua 没加载：全局 %q 不是函数", name)
		}
	}

	// ---- 1) 分配一个覆盖 (100,200)..(102,203) 的 3x4 触发器 ----
	// 用 quest.lua 里真实拼出来的那段代码串
	code := `
		local questName, rectList, code = ...
		return addQuestGridTrigger(questName, rectList, load(code)(select(4, ...)))
	`
	rect := protocol.NewLuaArrayVar(
		protocol.LuaVarWrapper{Valid: true, Value: protocol.NewLuaArrayVar(
			protocol.LuaVarWrapper{Valid: true, Value: protocol.NewLuaIntegerVar(100)},
			protocol.LuaVarWrapper{Valid: true, Value: protocol.NewLuaIntegerVar(200)},
			protocol.LuaVarWrapper{Valid: true, Value: protocol.NewLuaIntegerVar(3)},
			protocol.LuaVarWrapper{Valid: true, Value: protocol.NewLuaIntegerVar(4)},
		)},
	)
	handlerCode := protocol.NewLuaStringVar(
		`local questUID, magicName = ... return function(uid, x, y) return true end`)

	res := ms.ExecRemote(code, []protocol.LuaVar{
		protocol.NewLuaStringVar("测试任务"),
		rect,
		handlerCode,
		protocol.NewLuaIntegerVar(42),      // questUID（短 ID）
		protocol.NewLuaStringVar("测试法术"), // magicName
	})
	if res.Err != nil {
		t.Fatalf("addQuestGridTrigger: %v", res.Err)
	}
	if len(res.Values) == 0 || res.Values[0].Kind != protocol.LuaVarInteger {
		t.Fatalf("addQuestGridTrigger 没返回整数 ID: %v", res.Values)
	}
	id := res.Values[0].Integer
	if id == 0 {
		t.Fatal("★ 触发器 ID 是 0 —— allocateGridTriggerId 还是空桩")
	}
	t.Logf("分配到的触发器 ID = %d", id)

	// ---- 2) 矩形覆盖的格子应该都能查到 ----
	if ms.GridTriggerCount() != 1 {
		t.Fatalf("触发器数应为 1，实际 %d", ms.GridTriggerCount())
	}
	for _, pt := range [][2]int{{100, 200}, {102, 203}, {101, 202}} {
		if !ms.FireGridTrigger(1, pt[0], pt[1]) {
			t.Errorf("格 (%d,%d) 应该命中触发器", pt[0], pt[1])
		}
	}
	// 矩形外不应命中
	if ms.FireGridTrigger(1, 99, 200) {
		t.Error("(99,200) 在矩形外，不该命中")
	}
	if ms.FireGridTrigger(1, 103, 200) {
		t.Error("(103,200) 在矩形外（x+w=103 不含）不该命中")
	}
	// ★ x=100,w=3 → 覆盖 100,101,102；x=103 超出

	// ---- 3) 删除后不再命中 ----
	if err := L.DoString("deleteGridTrigger(" + itoa(id) + ")"); err != nil {
		t.Fatalf("deleteGridTrigger: %v", err)
	}
	if ms.GridTriggerCount() != 0 {
		t.Fatalf("删除后触发器数应为 0，实际 %d", ms.GridTriggerCount())
	}
	if ms.FireGridTrigger(1, 100, 200) {
		t.Error("删除后 (100,200) 不该再命中")
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// stubMapHost 是最小的地图宿主（不刷怪、不查格子）。
type stubMapHost struct{}

func (h *stubMapHost) MapID() uint32                              { return 1 }
func (h *stubMapHost) MapName() string                            { return "比奇县_0" }
func (h *stubMapHost) MapSize() (int, int)                        { return 1000, 1000 }
func (h *stubMapHost) CanThrough(x, y int) bool                   { return true }
func (h *stubMapHost) CountThrough(x, y, rw, rh int) int          { return rw * rh }
func (h *stubMapHost) AddMonster(n string, x, y int, s bool) uint64 { return 0 }
func (h *stubMapHost) AddGuard(n string, x, y, d int) uint64      { return 0 }
func (h *stubMapHost) AddSafeZone(x, y, r int)                    {}
func (h *stubMapHost) Alive(uid uint64) bool                      { return false }
func (h *stubMapHost) Log(level int, msg string)                   {}

// GridSwitchDest 测试桩：不配置任何门口
func (h *stubMapHost) GridSwitchDest(x, y int) (uint32, int, int, bool) {
	return 0, 0, 0, false
}

// SwitchPlayer 测试桩：不做真实搬运
func (h *stubMapHost) SwitchPlayer(playerUID uint64, mapID uint32, x, y int) bool {
	return false
}
