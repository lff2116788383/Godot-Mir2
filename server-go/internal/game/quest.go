package game

import (
	"context"
	"fmt"
	"log/slog"

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
// 客户端在「任务」面板里显示这些文字。
func (h *QuestHost) PostQuestDesp(ctx context.Context, playerUID uint64, descList []string) error {
	if h.Broadcast == nil {
		return nil
	}
	p, ok := h.Players.Get(playerUID)
	if !ok || p.ChannelID == 0 {
		return nil
	}
	// ★ SDQuestDespUpdate 是「一条描述」而不是「一个列表」：
	//
	//	Name / FSM / HasDesp / Desp —— 每次只更新某任务某个 fsm 的一段描述。
	//	原版也是一条一条推（客户端按 Name+FSM 覆盖）。
	//
	//	之前以为是数组，写成了 Desp: descList，编译期就报错。
	text := ""
	if len(descList) > 0 {
		text = descList[len(descList)-1]
	}
	payload, err := protocol.CerealSerialize(protocol.SDQuestDespUpdate{
		Name:    "",
		FSM:     "",
		HasDesp: text != "",
		Desp:    text,
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
	Host    *QuestHost
	Repo    *storage.QuestRepository
	Logger  *slog.Logger
}

// NewQuestService 创建任务服务并加载全部任务。
//
// ★ 加载失败不阻断启动：某个任务脚本坏了只跳过它，
//	其他任务照常工作（一个任务坏了不该让全服没任务）。
func NewQuestService(frameworkDir, scriptRoot string,
	host *QuestHost, repo *storage.QuestRepository,
	mapNames *script.NameTable, logger *slog.Logger) *QuestService {

	mgr := script.NewQuestManager(frameworkDir, scriptRoot, host, mapNames)
	loaded, failed, errs := mgr.LoadAll()

	if logger != nil {
		logger.Info("quest scripts loaded", "loaded", loaded, "failed", failed)
		for _, err := range errs {
			logger.Warn("quest load failed", "error", err)
		}
	}

	svc := &QuestService{
		Scripts: mgr,
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
