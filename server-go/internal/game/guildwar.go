package game

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// ==================== 公会战 ====================
//
// 【依据】player.cpp:3544-3620（declareGuildWar / refreshGuildWarList /
// scheduleGuildWarEnd / atGuildWar）+ guilddb.cpp:436-520
//
// ★★ 一个必须先纠正的认识：**原版没有「宣战」网络消息**。★★
//
// 本仓库早期审计写着「协议表缺 CM_DECLAREGUILDWAR，所以公会战做不了」。
// 这个判断是错的。翻 servermsg.hpp / clientmsg.hpp 全套协议，
// 与公会战相关的**只有** `SM_GUILDWARLIST`（服务端 → 客户端，推送战列表），
// 没有任何 `CM_*WAR*`。
//
// 原版的宣战入口是 **Lua 绑定**：
//
//	player.cpp:102  bindFunction("declareGuildWar", [..](enemyName, gold) { ... })
//
// 调用链是：
//
//	NPC 脚本（比奇县_行政官员）
//	  → server.player.declareGuildWar(uid, 敌会名, 金币)   [api/player.lua:58]
//	  → uidRemoteCall(uid, 敌会名, 金币, "return declareGuildWar(...)")
//	  → 在**玩家 actor** 的 Lua 里执行 → C++ 的 declareGuildWar()
//
// 所以 Go 侧要做公会战，缺的不是协议，而是：
//  1. 玩家范围的 Lua 运行时（`server` 表 + 39 个 player 绑定）
//  2. uidRemoteCall 的跨 actor 实现
//  3. declareGuildWar 这个绑定本身
//
// 这里先把 (3) 的业务逻辑做完整并可直接调用，(1)(2) 单独接（见 remote_call.go）。
// 这样无论入口是 Lua 还是将来的 GM 命令/HTTP 后台，逻辑只有一份。

// GuildWarDurationSec 是一场公会战的时长（sysconst.hpp:108 SYS_GUILDWARTIME）。
//
// ★ 重复宣战会**重置**结束时间（guilddb.cpp 用 insert or replace +
// now + SYS_GUILDWARTIME），所以它是「刷新」而不是「叠加」。
const GuildWarDurationSec int64 = 60 * 60

// GuildWarPlayer 是公会战需要的最小玩家能力。
//
// 用接口而不是 *Player：公会战要「给指定 dbid 的玩家推消息」，
// 在线与否无关；测试里也不需要构造完整 Player。
type GuildWarPlayer interface {
	GuildWarDBID() uint32
	GuildWarName() string
}

// guildWarState 是每个在线玩家的战列表现状（对应 m_guildWarList）。
//
// ★ 为什么要缓存而不是每次都查库：
//
//	refreshGuildWarList 需要判断「这一场是不是新宣的」来决定是否弹
//	「你的行会正在和 X 行会进行战争，还剩 N 分钟」。
//	如果每次都从库里拉，就没有「上次看到的是什么」这个基准，
//	每 tick 都会弹一遍同样的话。
type guildWarState struct {
	guildID uint32
	wars    []protocol.SDGuildWar
}

// GuildWarManager 管理公会战的宣战、推送与到期。
type GuildWarManager struct {
	Players   *PlayerManager
	Broadcast Broadcaster
	Repo      *storage.GuildRepository
	Golds     GoldRepository

	mu sync.Mutex
	// states dbid -> 该玩家上次看到的战列表
	states map[uint32]*guildWarState
	// timers warKey -> 已安排的到期定时器（防重复安排）
	timers map[string]*time.Timer
}

// NewGuildWarManager 构造公会战管理器。
func NewGuildWarManager(players *PlayerManager, broadcast Broadcaster,
	repo *storage.GuildRepository, golds GoldRepository) *GuildWarManager {
	return &GuildWarManager{
		Players: players, Broadcast: broadcast, Repo: repo, Golds: golds,
		states: make(map[uint32]*guildWarState),
		timers: make(map[string]*time.Timer),
	}
}

// DeclareWar 宣战（player.cpp:3544 declareGuildWar）。
//
// 返回 GUILDERR_* 码。**金币只在宣战成功时扣**（原版注释明确：
// "gold is taken only when the war gets declared, the leader is checked in database"）。
//
// 流程（顺序不能改）：
//  1. 校验金币够不够（不够直接 NOGOLD，不碰 DB）
//  2. dbDeclareGuildWar：校验是会长、目标会存在、不是自己
//  3. 扣金币
//  4. 刷新**双方**全体成员的战列表
func (m *GuildWarManager) DeclareWar(ctx context.Context, leaderDBID uint32,
	enemyName string, cost uint64, now int64) (storage.GuildErrorCode, error) {

	if m.Repo == nil {
		return storage.GuildErrNoGuild, nil
	}
	if now == 0 {
		now = time.Now().Unix()
	}

	leader, online := m.Players.ByDBID(leaderDBID)
	if !online {
		// 离线不能宣战（原版是在玩家 actor 里跑的，必然在线）
		return storage.GuildErrNoPlayer, nil
	}
	if leader.Gold < cost {
		return storage.GuildErrNoGold, nil
	}

	enemyID, code, err := m.Repo.DeclareWar(ctx, leaderDBID, enemyName, now)
	if err != nil {
		return code, fmt.Errorf("declare guild war: %w", err)
	}
	if code != storage.GuildErrNone {
		return code, nil
	}

	if cost > 0 {
		// ★ 用条件扣减而不是 Set(gold-cost)：
		//   校验与扣减之间可能被别的路径改过金币（拍卖成交等），
		//   Set 会把那次变化覆盖掉。
		remaining, ok, err := removeGoldAtomically(ctx, m.Golds, leader, cost)
		if err != nil || !ok {
			// 金币在宣战过程中被花掉了 —— 战已经建了，只能如实返回
			leader.Gold = remaining
			if payErr := m.pushGold(ctx, leader); payErr != nil {
				return storage.GuildErrNone, payErr
			}
			return storage.GuildErrNoGold, nil
		}
		leader.Gold = remaining
		if err := m.pushGold(ctx, leader); err != nil {
			return storage.GuildErrNone, err
		}
	}

	// 双方全体成员刷新（原版把两个会的成员 dbid 合并后再 updateGuild）
	m.refreshMembers(ctx, leaderDBID, enemyID)
	return storage.GuildErrNone, nil
}

// refreshMembers 刷新两个公会的在线成员战列表 + 公会信息（updateGuild）。
func (m *GuildWarManager) refreshMembers(ctx context.Context, dbids ...uint32) {
	seen := make(map[uint32]bool)
	for _, guildID := range dbids {
		if guildID == 0 || seen[guildID] {
			continue
		}
		seen[guildID] = true
		members, err := m.Repo.LoadMemberList(ctx, guildID)
		if err != nil {
			continue
		}
		for _, dbid := range members {
			if _, online := m.Players.ByDBID(dbid); online {
				_ = m.RefreshWarList(ctx, dbid, false)
			}
		}
	}
}

// RefreshWarList 刷新某玩家的战列表（player.cpp:3567 refreshGuildWarList）。
//
// postAll=true 时把每一场都重新说一遍（登录时用），
// false 时只对**新增**的战争弹提示。
//
// ★ 「新增」的判定用 SDGuildWar 全字段比较（GuildID+Name+EndTime）：
//
//	原版注释：「a war declared again has a new end time, it counts as a new war」。
//	所以重复宣战同一家会重新弹一次提示 —— 这是符合直觉的，
//	玩家会看到「还剩 60 分钟」而不是以为还是上次那场。
func (m *GuildWarManager) RefreshWarList(ctx context.Context, dbid uint32, postAll bool) error {
	if m.Repo == nil {
		return nil
	}
	p, online := m.Players.ByDBID(dbid)
	if !online {
		return nil
	}

	member, inGuild, err := m.Repo.LoadMembership(ctx, dbid)
	if err != nil {
		return err
	}
	guildID := uint32(0)
	if inGuild {
		guildID = member.GuildID
	}

	now := time.Now().Unix()
	var warList []protocol.SDGuildWar
	if inGuild {
		warList, err = m.Repo.LoadWarList(ctx, guildID, now)
		if err != nil {
			return err
		}
	}

	m.mu.Lock()
	prev := m.states[dbid]
	var prevWars []protocol.SDGuildWar
	if prev != nil && prev.guildID == guildID {
		prevWars = prev.wars
	}
	m.states[dbid] = &guildWarState{guildID: guildID, wars: warList}
	m.mu.Unlock()

	// 只有新增的战争才弹 SM_TEXT
	for _, war := range warList {
		isNew := !containsWar(prevWars, war)
		if isNew {
			// 到期定时器：每场战争安排一次（重复宣战换了 EndTime 就是新的 key）
			m.scheduleWarEnd(dbid, war)
		}
		if isNew || postAll {
			remainMin := uint64(0)
			if war.EndTime > uint64(now) {
				// 原版：(endTime - now + 59) / 60，向上取整算「还剩几分钟」
				remainMin = uint64(war.EndTime-uint64(now)+59) / 60
			}
			m.sendText(p, fmt.Sprintf("你的行会正在和%s行会进行行会战争，还剩%d分钟",
				war.GuildName, remainMin))
		}
	}

	// 一定要推列表 —— 客户端靠它画「交战行会」面板
	payload, err := protocol.CerealSerialize(protocol.SDGuildWarList{
		GuildID: guildID,
		WarList: warList,
	})
	if err != nil {
		return err
	}
	m.sendTo(p, protocol.SM_GUILDWARLIST, payload)
	return nil
}

// scheduleWarEnd 在战争到期时弹提示并刷新（player.cpp:3592 scheduleGuildWarEnd）。
//
// ★ 原版是「到期后再查一次库」而不是「到期就清掉」：
//
//	因为宣战用 insert or replace 会改 fld_endtime，
//	旧定时器到点时 war.EndTime 已经不是它记的那个值了 ——
//	所以原版先比 epoch() < war.endTime，没到就**重新安排**。
//	这里用「key 含 EndTime」达到同样效果：重新宣战会生成新 key 新定时器，
//	旧定时器的 key 在 m_guildWarList 里已经找不到，直接退出。
func (m *GuildWarManager) scheduleWarEnd(dbid uint32, war protocol.SDGuildWar) {
	key := fmt.Sprintf("%d|%d|%d", dbid, war.GuildID, war.EndTime)
	m.mu.Lock()
	if _, exists := m.timers[key]; exists {
		m.mu.Unlock()
		return
	}
	var delayMS int64
	now := time.Now().Unix()
	if int64(war.EndTime) > now {
		delayMS = (int64(war.EndTime) - now) * 1000
	}
	// 原版 +500ms 缓冲，避免和宣战同一秒的竞态
	delayMS += 500
	warCopy := war
	timer := time.AfterFunc(time.Duration(delayMS)*time.Millisecond, func() {
		m.expireWar(dbid, warCopy, key)
	})
	m.timers[key] = timer
	m.mu.Unlock()
}

// expireWar 处理一场到期战争。
func (m *GuildWarManager) expireWar(dbid uint32, war protocol.SDGuildWar, key string) {
	m.mu.Lock()
	delete(m.timers, key)
	// 这个玩家还看得见这场战争吗？看不见说明已经换会/重新宣战
	state := m.states[dbid]
	stillListed := state != nil && containsWar(state.wars, war)
	m.mu.Unlock()

	if !stillListed {
		return
	}

	p, online := m.Players.ByDBID(dbid)
	if !online {
		m.mu.Lock()
		delete(m.states, dbid)
		m.mu.Unlock()
		return
	}

	m.sendText(p, fmt.Sprintf("你的行会和%s行会的行会战争结束了", war.GuildName))
	_ = m.RefreshWarList(context.Background(), dbid, false)
}

// AtWar 判断两个玩家是否处于交战状态（player.cpp:3616 atGuildWar）。
//
// ★ 用途是**免 PK 点**（playerop.cpp:671）：
//
//	killing a member of a guild my guild is at war with is no murder
//
//	没有这条，同会战双方互杀会一路刷成红名，
//	公会战就变成了「谁先动手谁吃亏」。
func (m *GuildWarManager) AtWar(ctx context.Context, playerDBID, targetDBID uint32) bool {
	if m.Repo == nil || playerDBID == 0 || targetDBID == 0 || playerDBID == targetDBID {
		return false
	}
	mine, okA, err := m.Repo.LoadMembership(ctx, playerDBID)
	if err != nil || !okA {
		return false
	}
	theirs, okB, err := m.Repo.LoadMembership(ctx, targetDBID)
	if err != nil || !okB {
		return false
	}
	if mine.GuildID == theirs.GuildID {
		return false
	}
	atWar, err := m.Repo.AtWar(ctx, mine.GuildID, theirs.GuildID, time.Now().Unix())
	if err != nil {
		return false
	}
	return atWar
}

// OnPlayerLogin 登录时推一次完整战列表（原版 refreshGuildWarList(true)）。
func (m *GuildWarManager) OnPlayerLogin(ctx context.Context, dbid uint32) {
	_ = m.RefreshWarList(ctx, dbid, true)
}

// Forget 玩家下线/退会时清掉缓存。
func (m *GuildWarManager) Forget(dbid uint32) {
	m.mu.Lock()
	delete(m.states, dbid)
	m.mu.Unlock()
}

// ==================== 辅助 ====================

func containsWar(list []protocol.SDGuildWar, war protocol.SDGuildWar) bool {
	for _, w := range list {
		if w.GuildID == war.GuildID && w.GuildName == war.GuildName && w.EndTime == war.EndTime {
			return true
		}
	}
	return false
}

// removeGoldAtomically 原子地扣金币，返回扣后余额与是否成功。
//
// 没有 Golds 实现（测试/无库模式）时退化成内存扣减。
func removeGoldAtomically(ctx context.Context, golds GoldRepository,
	p *Player, amount uint64) (uint64, bool, error) {
	if amount == 0 {
		return p.Gold, true, nil
	}
	if golds == nil {
		if p.Gold < amount {
			return p.Gold, false, nil
		}
		return p.Gold - amount, true, nil
	}
	remover, ok := golds.(interface {
		Remove(ctx context.Context, dbid uint32, amount uint64) (uint64, bool, error)
	})
	if !ok {
		// 接口只声明了 Load/Set 的实现：退回「先查后写」
		if p.Gold < amount {
			return p.Gold, false, nil
		}
		next := p.Gold - amount
		return next, true, golds.Set(ctx, p.DBID, next)
	}
	remaining, ok, err := remover.Remove(ctx, p.DBID, amount)
	if err != nil {
		return p.Gold, false, err
	}
	return remaining, ok, nil
}

// pushGold 把金币变化推给客户端（SM_GOLD）。
//
// ★ SM_GOLD 是**定长**消息（MessageFixedCompressed），必须走 AppendWirePayload；
// 用 CerealSerialize 会编码成变长格式，客户端解析直接越界。
func (m *GuildWarManager) pushGold(ctx context.Context, p *Player) error {
	if m.Broadcast == nil || p.ChannelID == 0 {
		return nil
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMGold{Gold: p.Gold})
	if err != nil {
		return err
	}
	_ = m.Broadcast.SendTo(p.ChannelID, protocol.SM_GOLD, payload, 0)
	return nil
}

// sendText 发一条 SM_TEXT（原版 postNetMessage(SM_TEXT, str_printf(...))）。
//
// ★ SM_TEXT 是 MessageVariable，body 就是**裸 UTF-8 字符串**，
// 没有结构体头、也不做 Cerealf 包装（见 durability.go:278 同样的用法）。
func (m *GuildWarManager) sendText(p *Player, text string) {
	m.sendTo(p, protocol.SM_TEXT, []byte(text))
}

func (m *GuildWarManager) sendTo(p *Player, id protocol.SMType, payload []byte) {
	if m.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	_ = m.Broadcast.SendTo(p.ChannelID, id, payload, 0)
}
