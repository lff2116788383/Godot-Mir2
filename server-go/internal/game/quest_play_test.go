package game

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// TestQuestHandlerInstalledInNPC 验证任务 handler 真的进了 NPC 的 Lua 状态。
//
// ★ 这是「任务是否可玩」的唯一硬指标。
//
//	链路：任务脚本 --uidRemoteCall--> NPC 状态执行 setQuestHandler
//	     --> 写进 _RSVD_NAME_EPQST_eventHandlers[quest][event]
//	     --> npchar.lua:609 hasQuestHandler 才查得到
//	     --> 玩家点 NPC 才能看到任务入口
//
//	handler 没进去时，服务端不报任何错，玩家只看到
//	「我听不懂你在说什么」—— 这是最隐蔽的失效。
func TestQuestHandlerInstalledInNPC(t *testing.T) {
	scriptRoot := "E:/Desktop/game/mir2x/server/script"
	if _, err := os.Stat(scriptRoot); err != nil {
		t.Skip("原版脚本目录不可用")
	}

	ctx := context.Background()
	store, err := storage.Open(ctx, storage.OpenOptions{
		Path: t.TempDir() + "/q.db3", CreateIfMissing: true,
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	items, _ := LoadItemDB("../../content/items.json")
	magics, _ := LoadMagicDB("../../content/magics.json")
	mdb, _ := world.LoadMonsterDB("../../content/monsters.json")

	// ★ 必须先注入地图名表。
	//
	//	世界包的 mapID↔名字 是显式初始化的（world.SetMapNames），
	//	真实服务端在 main 里调，测试里不调就全是空字符串 ——
	//	表现为「AllMapIDs 返回 0 张图」→ 地图 actor 一个都没注册
	//	→ 任务 getNPCharUID 报 target not found。
	// ★★ 必须先建角色 ★★
	//
	//	任务表主键 fld_dbid 是外键指向 tbl_char(fld_dbid)，
	//	库里没有这个角色时任何写都会报 FOREIGN KEY constraint failed。
	//	而这个报错只会说「约束失败」，看不出「角色不存在」。
	//
	//	同时这也验证了短 ID → dbid 的转换是对的：
	//	角色 dbid 是 1，短 ID 也是 1，两者恰好相等时看不出错；
	//	换成 dbid=7 就能区分。
	chars := storage.NewCharacterRepository(store)
	if aerr := storage.NewAccountRepository(store).Create(ctx, "testacct", "pw123456"); aerr != nil {
		t.Fatalf("建账号: %v", aerr)
	}
	ch, err := chars.Create(ctx, storage.CreateCharacterParams{
		Account: "testacct", Name: "测试角色", Gender: 0, Job: 1,
		Map: 1, MapX: 100, MapY: 100,
	})
	if err != nil {
		t.Fatalf("建角色: %v", err)
	}
	playerUID := common.GetPlayerUID(ch.DBID)
	fmt.Printf("角色 dbid=%d uid=%d\n", ch.DBID, playerUID)

	world.SetMapNames(LoadMapNames(""))

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	npcMgr := NewNPCManager(items, magics, logger)
	npcMgr.Monsters = mdb
	npcMgr.ScriptRoot = scriptRoot
	npcMgr.FrameworkDir = script.ResolveFrameworkDir()

	// ★ 必须全量预加载：任务脚本会引用各图的 NPC，
	//	只加载一张图会导致大量任务激活失败。
	// ★ 全量加载 NPC 目录里出现的每一张图
	byMap, err := script.ScanNPCDir(scriptRoot + "/npc")
	if err != nil {
		t.Fatalf("扫描 NPC 目录: %v", err)
	}
	// ★ 用 LoadMapNames 拿到 完整 mapID→名字，不要用 world.MapNameOf：
	//	后者只认 world 包初始化过的表，测试里没初始化，恒返回空。
	idToName := LoadMapNames("")
	mapNameToID := map[string]uint32{}
	for id, name := range idToName {
		mapNameToID[name] = id
	}
	fmt.Printf("地图名表 %d 条\n", len(mapNameToID))
	total := 0
	for mapName := range byMap {
		mapID, ok := mapNameToID[mapName]
		if !ok {
			continue
		}
		insts, lerr := npcMgr.LoadMap(ctx, mapID, mapName, func(uid uint64) *npcHostAdapter {
			return &npcHostAdapter{NPCUID: uid, Logger: logger}
		})
		if lerr != nil {
			continue
		}
		total += len(insts)
	}
	fmt.Printf("全量加载 %d 张图共 %d 个 NPC\n", len(byMap), total)

	// ★ 注册全部地图 actor（任务 getNPCharUID 要靠它）
	//	不注册就会报 "remote call target ... not found"。
	host := &QuestHost{Repo: storage.NewQuestRepository(store), NPCs: npcMgr, Logger: logger}
	svc := NewQuestService(script.ResolveFrameworkDir(), scriptRoot, host, host.Repo,
		script.NewNameTable(LoadMapNames("")), logger)
	defer svc.Close()

	// 必须先注册地图 actor（ActivateAfterWorldLoaded 内部会调 registerAllMapRPC）
	//
	// ★ 传 nil 表示「测试里没有地图脚本管理器」：
	//	格子触发器会明确报错，而不是假装成功。
	svc.ActivateAfterWorldLoaded(npcMgr, nil)
	if svc.Scripts.PendingLazy() > 0 {
		t.Logf("仍有 %d 个任务未激活", svc.Scripts.PendingLazy())
	}

	// ★★ 模拟玩家接任务，验证 handler 真的进 NPC ★★
	//
	//	setupNPCQuestBehavior 是在任务的 [SYS_ENTER] 里调用的
	//	（见 乞丐任务.lua:7），也就是「玩家接任务那一刻」才装 handler。
	//
	//	所以启动后「0 个 handler」是正常的，必须先模拟一次接任务。
	playerUIDStr := playerUID
	questName := "乞丐任务"
	script.SetDebugRPC(true)
	qs, _ := svc.Scripts.Get(questName)
	if qs == nil {
		t.Fatalf("任务 %s 未激活", questName)
	}
	fmt.Printf("模拟玩家 %d 接任务 %s ...\n", playerUIDStr, questName)
	enterRes := qs.ExecRemote(
		// ★ 不要传 from：新玩家当前状态是 SYS_LUANIL，
		//	from=SYS_ENTER 不匹配会直接 return false（静默什么都不做）。
		// ★ 必须包在 coroutine 里：
		//	setQuestState 内部会 runThread + yield，
		//	主线程直接调会报 "can not yield from outside of a coroutine"。
		"local uid = ... " +
			"local co = coroutine.create(function() " +
			"  setQuestState{uid=uid, state=SYS_ENTER} " +
			"end) " +
			"local ok, err = coroutine.resume(co) " +
			"return tostring(ok) .. '|' .. tostring(err) .. '|probe=' .. tostring(_G.__probe_count)",
		[]protocol.LuaVar{protocol.NewLuaIntegerVar(svc.Bus.Short(playerUIDStr))})
	fmt.Printf("setQuestState(SYS_ENTER): err=%v vals=%v\n", enterRes.Err, enterRes.Values)

	// ★ 验证状态真的写进了数据库（不是只跑了内存）
	repo := storage.NewQuestRepository(store)
	stateVal, found, serr := repo.LoadField(ctx, questTableName(questName), ch.DBID, "fld_states")
	if serr != nil {
		t.Fatalf("读回任务状态: %v", serr)
	}
	if !found {
		t.Fatal("★ 任务状态没有写进数据库 —— dbSetQuestFields 链路断了")
	}
	fmt.Printf("数据库里的任务状态: kind=%v\n", stateVal.Kind)

	// ★ 再验证 fld_desp 也写进去了（setQuestDesp 的完整链路）
	despVal, despFound, derr := repo.LoadField(ctx, questTableName(questName), ch.DBID, "fld_desp")
	if derr != nil {
		t.Fatalf("读回任务描述: %v", derr)
	}
	fmt.Printf("数据库里的任务描述: found=%v kind=%v\n", despFound, despVal.Kind)

	// ★ 找一个装了任务 handler 的 NPC，直接问它的 Lua 状态
	checked, installed := 0, 0
	npcMgr.ForEach(func(uid uint64, npc *npcInstance) {
		if npc == nil || npc.Script == nil || checked >= 200 {
			return
		}
		checked++
		names := questNamesInNPC(npc)
		if len(names) > 0 {
			installed++
			if installed <= 3 {
				fmt.Printf("  NPC %s(%s) 装了 %d 个任务: %v\n",
					npc.Def.Name, npc.Def.MapName, len(names), names)
			}
		}
	})
	fmt.Printf("检查 %d 个 NPC，其中 %d 个装了任务 handler\n", checked, installed)

	if installed == 0 {
		t.Fatal("★ 没有任何 NPC 装上任务 handler —— 任务不可玩")
	}
}

// questNamesInNPC 读出 NPC Lua 状态里注册了 handler 的任务名。
func questNamesInNPC(npc *npcInstance) []string {
	if npc == nil || npc.Script == nil {
		return nil
	}
	return npc.Script.QuestNamesWithHandlers()
}
