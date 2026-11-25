package game

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// ==================== 任务系统（game 层的宿主实现）====================
//
// 【依据】quest.cpp / questdb.cpp / quest.lua + 42 个任务脚本（20,819 行）
//
// 分工：
//
//	script 包  负责「把 quest.lua 和 42 个任务脚本跑起来」（Lua 侧）
//	本文件     负责「给 Lua 提供宿主能力」（Go 侧）
//
// ★★ 关键理解：任务脚本本身**不需要翻译** ★★
//
//	42 个任务脚本全部写在 quest.lua 的 API 之上（setQuestState /
//	setQuestFSMTable / dbGetQuestVar …）。只要 Go 侧把
//	quest.cpp 的 9 个 C 绑定 + 协程原语补齐，这 2 万行原版脚本直接可用。
//
//	这与 NPC 脚本的处理方式一致（零翻译复用），也是本项目的一贯策略：
//	**能复用原版脚本的，绝不翻译**。

// QuestHost 实现 script.QuestDeps，把 Go 的存储/推送能力暴露给任务脚本。
type QuestHost struct {
	Players   *PlayerManager
	Broadcast Broadcaster
	Repo      *storage.QuestRepository
	NPCs      *NPCManager
	Logger    *slog.Logger
}

// LoadQuestField 读任务字段（dbGetQuestField）。
func (h *QuestHost) LoadQuestField(ctx context.Context, questName string,
	dbid uint32, field string) (protocol.LuaVar, bool, error) {
	if h.Repo == nil {
		return protocol.LuaVar{}, false, nil
	}
	return h.Repo.LoadField(ctx, questTableName(questName), dbid, field)
}

// SetQuestField 写单个任务字段（dbSetQuestField）。
func (h *QuestHost) SetQuestField(ctx context.Context, questName string,
	dbid uint32, field string, value *protocol.LuaVar) error {
	if h.Repo == nil {
		return nil
	}
	return h.Repo.UpdateFields(ctx, questTableName(questName), dbid,
		map[string]*protocol.LuaVar{field: value}, false)
}

// SetQuestFields 批量写（_RSVD_NAME_dbSetQuestFields）。
func (h *QuestHost) SetQuestFields(ctx context.Context, questName string,
	dbid uint32, fields map[string]*protocol.LuaVar, replace bool) error {
	if h.Repo == nil || len(fields) == 0 {
		return nil
	}
	return h.Repo.UpdateFields(ctx, questTableName(questName), dbid, fields, replace)
}

// PostQuestDesp 推任务描述（_RSVD_NAME_setQuestDesp）。
//
// ★ 原版签名带 fsm：一条描述对应「某任务某个状态机」。
//	客户端按 Name+FSM 覆盖显示，缺了它们任务面板分不清
//	主线描述和子状态机描述，而且同名任务会互相覆盖。
func (h *QuestHost) PostQuestDesp(ctx context.Context, questName string, playerUID uint64,
	fsm string, hasDesp bool, desp string) error {
	if h.Broadcast == nil {
		return nil
	}
	p, ok := h.Players.Get(playerUID)
	if !ok || p.ChannelID == 0 {
		return nil
	}
	payload, err := protocol.CerealSerialize(protocol.SDQuestDespUpdate{
		Name:    questName,
		FSM:     fsm,
		HasDesp: hasDesp,
		Desp:    desp,
	})
	if err != nil {
		return err
	}
	_ = h.Broadcast.SendTo(p.ChannelID, protocol.SM_QUESTDESPUPDATE, payload, 0)
	return nil
}

// FindNPCharUID 按地图名 + NPC 名找 UID（getNPCharUID 的 Go 侧）。
func (h *QuestHost) FindNPCharUID(mapName, npcName string) (uint64, bool) {
	if h.NPCs == nil {
		return 0, false
	}
	return h.NPCs.FindByMapAndName(mapName, npcName)
}

// Log 写日志。
func (h *QuestHost) Log(level int, msg string) {
	if h.Logger == nil {
		return
	}
	if level >= 1 {
		h.Logger.Warn("quest", "msg", msg)
		return
	}
	h.Logger.Debug("quest", "msg", msg)
}

// questTableName 把任务名映射成库表名。
//
// ★ 原版每个任务一张表（dbCreateQuestTable(questName)），
//	表名就是任务名。但 SQLite 表名不能用中文以外的任意字符，
//	而任务名全是中文 —— 必须统一加前缀并转义。
//
//	这里用 `quest_<md5前缀>` 而不是直接用中文：
//	中文表名在部分 SQLite 构建/驱动组合下会出问题，
//	而用哈希前缀既稳定又不引入非法字符。
func questTableName(questName string) string {
	return "quest_" + shortHash(questName)
}

// shortHash 取任务名的短哈希（16 位十六进制，足够避免碰撞）。
func shortHash(s string) string {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var hash uint64 = offset64
	for i := 0; i < len(s); i++ {
		hash ^= uint64(s[i])
		hash *= prime64
	}
	return fmt.Sprintf("%016x", hash)
}

// ==================== 任务管理器（game 层封装）====================

// QuestService 是任务系统的对外入口。
//
// 职责：
//   - 启动时加载全部 42 个任务
//   - 为每个任务建库表
//   - 暴露「玩家上线/下线/死亡/升级」等触发点
type QuestService struct {
	Scripts *script.QuestManager
	// Bus 跨状态 RPC 总线（NPC 要靠它找到任务）
	Bus *script.RPCBus
	Host    *QuestHost
	Repo    *storage.QuestRepository
	Logger  *slog.Logger
	// Scripts2 地图脚本管理器（格子触发器要落到地图 Lua 状态）
	//
	// ★ 名字带 2 是为了不和上面的 Scripts（任务管理器）混淆：
	//	两者都是「脚本管理器」，但一个是 quest/*.lua，一个是 map/*.lua。
	Scripts2 *MapScriptManager
}

// NewQuestService 创建任务服务并加载全部任务。
//
// ★ 加载失败不阻断启动：某个任务脚本坏了只跳过它，
//	其他任务照常工作（一个任务坏了不该让全服没任务）。
func NewQuestService(frameworkDir, scriptRoot string,
	host *QuestHost, repo *storage.QuestRepository,
	mapNames *script.NameTable, logger *slog.Logger) *QuestService {

	// ★ RPCBus 是 B 方案的核心：任务之间、任务与 NPC 之间靠它互相寻址。
	bus := script.NewRPCBus(5*time.Second, func(format string, args ...any) {
		if logger != nil {
			logger.Warn(fmt.Sprintf(format, args...))
		}
	})
	mgr := script.NewQuestManager(frameworkDir, scriptRoot, host, mapNames, bus)
	registered, errs := mgr.LoadAll()

	if logger != nil {
		logger.Info("quest scripts registered", "count", registered,
			"note", "waiting for NPC load to activate")
		for _, err := range errs {
			logger.Warn("quest scan failed", "error", err)
		}
	}

	svc := &QuestService{
		Scripts: mgr,
		Bus:     bus,
		Host:    host,
		Repo:    repo,
		Logger:  logger,
	}
	// 为每个成功加载的任务建表（原版 dbCreateQuestTable）
	if repo != nil {
		svc.ensureTables()
	}
	return svc
}

// ensureTables 为每个任务建状态表。
func (s *QuestService) ensureTables() {
	if s.Scripts == nil || s.Repo == nil {
		return
	}
	created := 0
	s.Scripts.ForEach(func(name string, _ *script.QuestState) {
		if err := s.Repo.CreateTable(context.Background(), questTableName(name)); err == nil {
			created++
		}
	})
	if s.Logger != nil && created > 0 {
		s.Logger.Info("quest tables ensured", "count", created)
	}
}

// OnPlayerLogin 玩家上线：重放该玩家所有任务的当前状态
// （原版 _RSVD_NAME_restoreQuestStates）。
//
// ★ 这是任务系统的核心行为：**每次登录都把当前状态函数重跑一遍**。
//
//	原版注释说得很明确：
//	  "Every login runs the current state function of each fsm again, from the start.
//	   So a state function gives no reward and takes nothing it can't take twice."
//
//	所以任务脚本自己会把「发奖励」放在「切换到该状态」的回调里，
//	而不是放在状态函数本体里。我们只要照实重放即可。
func (s *QuestService) OnPlayerLogin(ctx context.Context, p *Player) {
	if s.Scripts == nil || p == nil {
		return
	}
	s.Scripts.ForEach(func(name string, q *script.QuestState) {
		// 调 quest.lua 的 _RSVD_NAME_restoreQuestStates(uid)
		if _, err := q.CallRestore(ctx, p.UID); err != nil {
			if s.Logger != nil {
				s.Logger.Debug("restore quest state failed", "quest", name, "player", p.Name, "error", err)
			}
		}
	})
}

// Count 返回已加载的任务数（诊断用）。
func (s *QuestService) Count() int {
	if s.Scripts == nil {
		return 0
	}
	return s.Scripts.Count()
}

// Close 释放全部任务运行时。
func (s *QuestService) Close() {
	if s.Scripts != nil {
		s.Scripts.Close()
	}
}

// ActivateAfterWorldLoaded 在地图与 NPC 加载完成之后激活全部任务。
//
// ★ 调用时机是整个 B 方案的关键。
//
//	顺序必须是：
//	  1. 加载地图脚本（刷怪/守卫）
//	  2. 加载该图全部 NPC（任务要往 NPC 上挂 handler）
//	  3. 注册地图 actor + NPC 到 RPC 总线
//	  4. **然后**才执行任务脚本
//
//	任何一步提前，任务脚本顶层的 getNPCharUID 就会返回 nil，
//	任务无法挂到 NPC 上 —— 玩家点 NPC 看不到任务入口。
//
// ★ scripts 是地图脚本管理器：任务的格子触发器要落到真实的
//	地图 Lua 状态里（servermap.lua 的 addQuestGridTrigger）。
//	传 nil 时格子触发器会报错而不是假装成功。
func (s *QuestService) ActivateAfterWorldLoaded(npcManager *NPCManager, scripts *MapScriptManager) {
	if s == nil || s.Scripts == nil {
		return
	}

	// 先把「全部地图」的 actor 注册进总线
	//
	// ★ 必须注册**所有**地图，不只是已加载的：
	//
	//	任务脚本会 getNPCharUID('道馆_1', '大悲善僧_1')，
	//	不管玩家有没有去过道馆_1。原版所有地图在启动时全部 activate，
	//	所以任务能找到任何一张图的 NPC。
	//
	//	Go 侧地图是懒加载的，但「回答某个 NPC 在哪张图/UID 多少」
	//	只需要名字表 + NPC 定义，不需要地图数据文件加载。
	//	所以这里注册全部 603 张图的 actor。
	s.registerAllMapRPC(npcManager, scripts)

	activated, failed := s.Scripts.ActivateLazy()
	if s.Logger != nil {
		s.Logger.Info("quest scripts activated", "activated", activated, "failed", failed,
			"pending", s.Scripts.PendingLazy())
	}

	// 为激活的任务建表
	s.ensureTables()
}
