package game

import (
	"context"
	"log/slog"
	"sync"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// ==================== 组队（内存态）====================
//
// 【依据】playernet.cpp:1273-1391 + player.hpp:100-103
//
// 原版组队**不入库**，是纯内存状态：
//
//	uint64_t              m_teamLeader;
//	std::vector<uint64_t> m_teamMemberList;   // 队长是权威，队员是副本
//	SDTeamReq             m_teamRequirement;
//	std::set<uint64_t>    m_teamCandidateSet; // 待批准申请
//
// ★ 为什么必须有个集中管理器，不能照抄「每个 Player 一份」：
//
//	原版是 actor 模型，队长和队员各自持有副本，靠 AM_TEAMUPDATE
//	互相通知保持一致。Go 侧每个连接一个 goroutine，各持一份必然
//	出现「两个人的队伍列表不一样」——而且很难复现。
//
//	改成「一个 TeamManager 掌管所有队伍」，所有读写都在同一把锁下，
//	天然一致。代价是所有队伍操作串行，但组队是低频操作（几秒一次），
//	完全不是瓶颈。

// TeamErrorCode 对应原版 TEAMERR_*（protocoldef.hpp:496-508）。
const (
	TeamErrNone           uint8 = 0
	TeamErrInTeam         uint8 = 1
	TeamErrNotLeader      uint8 = 2
	TeamErrBadRequirement uint8 = 3
	TeamErrNoTeam         uint8 = 4
	TeamErrFull           uint8 = 5
	TeamErrRequirement    uint8 = 6
	TeamErrBadCandidate   uint8 = 7
	TeamErrTransferLeader uint8 = 8
	TeamErrKicked         uint8 = 9
	TeamErrDisbanded      uint8 = 10
)

// MaxTeamMember 是队伍人数上限（原版 SDTeamReq.maxMember 的合法上限）。
//
// 传奇的组队上限是 11（1 队长 + 10 队员），限制过大会让界面排不下。
const MaxTeamMember = 11

// Team 是一支队伍。
type Team struct {
	Leader  uint64   // 队长 UID
	Members []uint64 // 全员 UID（含队长）
	Req     protocol.SDTeamReq
	// Candidates 待批准的申请者 UID
	//
	// 原版是 m_teamCandidateSet。有它才能区分「这个人申请过」和
	// 「随便发个 UID 想混进来」—— 前者批准，后者 TEAMERR_BADCANDIDATE。
	Candidates map[uint64]struct{}
}

// TeamManager 掌管所有队伍。
type TeamManager struct {
	mu sync.Mutex
	// byUID UID -> 所在队伍（全员索引，便于 O(1) 查「我在哪个队」）
	byUID map[uint64]*Team
	// byLeader 队长 UID -> 队伍
	byLeader map[uint64]*Team

	Players   *PlayerManager
	Broadcast Broadcaster
	Logger    *slog.Logger
}

// NewTeamManager 创建组队管理器。
func NewTeamManager(players *PlayerManager, broadcast Broadcaster, logger *slog.Logger) *TeamManager {
	return &TeamManager{
		byUID:     make(map[uint64]*Team),
		byLeader:  make(map[uint64]*Team),
		Players:   players,
		Broadcast: broadcast,
		Logger:    logger,
	}
}

// TeamOf 返回某玩家所在的队伍（不在队伍返回 nil）。
func (m *TeamManager) TeamOf(uid uint64) *Team {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byUID[uid]
}

// Create 创建队伍（CM_CREATETEAM）。
func (m *TeamManager) Create(ctx context.Context, p *Player, req protocol.SDTeamReq) uint8 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.byUID[p.UID]; exists {
		return TeamErrInTeam
	}
	req = m.normalizeReq(req)
	if req.MaxMember <= 0 {
		return TeamErrBadRequirement
	}

	team := &Team{
		Leader:     p.UID,
		Members:    []uint64{p.UID},
		Req:        req,
		Candidates: make(map[uint64]struct{}),
	}
	m.byUID[p.UID] = team
	m.byLeader[p.UID] = team

	m.pushTeam(ctx, team)
	return TeamErrNone
}

// RequestJoin 处理 CM_REQUESTJOIN （请求加入某人/批准某人）。
//
// 原版语义很绕，三种情况都走同一条消息（playernet.cpp:1280-1337）：
//
//	uid == 自己，且已在队伍里  → TEAMERR_INTEAM
//	uid == 自己，且不在队伍里  → 自己开一个单人队
//	自己是队长，uid 是申请人  → 批准入队
//	自己是队长，uid 不是申请人 → TEAMERR_BADCANDIDATE
//	自己在队伍里但不是队长    → TEAMERR_INTEAM
//	自己不在队伍里，uid 是别人 → 向对方发起申请（进对方候选集）
func (m *TeamManager) RequestJoin(ctx context.Context, p *Player, targetUID uint64) uint8 {
	if targetUID == 0 {
		return TeamErrBadCandidate
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	mine, inTeam := m.byUID[p.UID]

	// 情况 1：目标是自己
	if targetUID == p.UID {
		if inTeam {
			return TeamErrInTeam
		}
		team := &Team{
			Leader:     p.UID,
			Members:    []uint64{p.UID},
			Req:        protocol.SDTeamReq{MaxMember: MaxTeamMember},
			Candidates: make(map[uint64]struct{}),
		}
		m.byUID[p.UID] = team
		m.byLeader[p.UID] = team
		m.pushTeam(ctx, team)
		return TeamErrNone
	}

	// 情况 2：我是队长 → 批准申请人
	if inTeam && mine.Leader == p.UID {
		if _, isCandidate := mine.Candidates[targetUID]; !isCandidate {
			return TeamErrBadCandidate
		}
		delete(mine.Candidates, targetUID)
		return m.addMemberLocked(ctx, mine, targetUID)
	}

	// 情况 3：我在队伍里但不是队长
	if inTeam {
		return TeamErrInTeam
	}

	// 情况 4：我不在队伍里 → 向对方的队伍申请
	targetTeam, targetInTeam := m.byUID[targetUID]
	if !targetInTeam {
		return TeamErrNoTeam
	}
	if len(targetTeam.Members) >= int(targetTeam.Req.MaxMember) {
		return TeamErrFull
	}
	targetTeam.Candidates[targetUID] = struct{}{}
	_ = targetTeam.Candidates[targetUID]
	targetTeam.Candidates[p.UID] = struct{}{}

	// 通知队长「有人申请」。原版走 AM_REQUESTJOINTEAM 把申请者资料推给队长。
	m.notifyJoinRequest(p, targetTeam.Leader)
	return TeamErrNone
}

// Leave 处理 CM_REQUESTLEAVETEAM。
//
// 原版两种路径（playernet.cpp:1340-1370）：
//
//	我是队长：移出指定成员（或解散自己）→ 通知其余成员 → 推新列表
//	我是队员：直接走人，不等队长（队长可能离线）
func (m *TeamManager) Leave(ctx context.Context, p *Player, targetUID uint64) uint8 {
	if targetUID == 0 {
		targetUID = p.UID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	team, inTeam := m.byUID[p.UID]
	if !inTeam {
		return TeamErrNoTeam
	}

	// 队长踢人 / 解散
	if team.Leader == p.UID {
		if targetUID == p.UID {
			// 队长自己走 = 解散
			m.disbandLocked(ctx, team)
			return TeamErrNone
		}
		// 移出某人
		if !containsUID(team.Members, targetUID) {
			return TeamErrBadCandidate
		}
		m.removeMemberLocked(ctx, team, targetUID)
		// 把被踢的人清出索引（他的队伍列表要变成空）
		m.pushEmptyTeam(targetUID)
		m.pushTeam(ctx, team)
		return TeamErrNone
	}

	// 队员自己走
	if targetUID != p.UID {
		return TeamErrNotLeader
	}
	m.removeMemberLocked(ctx, team, p.UID)
	m.pushEmptyTeam(p.UID)
	m.pushTeam(ctx, team)
	return TeamErrNone
}

// TransferLeader 移交队长（CM_TRANSFERTEAMLEADER）。
func (m *TeamManager) TransferLeader(ctx context.Context, p *Player, targetUID uint64) uint8 {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, inTeam := m.byUID[p.UID]
	if !inTeam {
		return TeamErrNoTeam
	}
	if team.Leader != p.UID {
		return TeamErrNotLeader
	}
	if targetUID == p.UID || !containsUID(team.Members, targetUID) {
		return TeamErrTransferLeader
	}

	delete(m.byLeader, team.Leader)
	team.Leader = targetUID
	m.byLeader[targetUID] = team

	m.pushTeam(ctx, team)
	return TeamErrNone
}

// SetRequirement 改队伍要求（CM_SETTEAMREQUIREMENT）。
func (m *TeamManager) SetRequirement(ctx context.Context, p *Player, req protocol.SDTeamReq) uint8 {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, inTeam := m.byUID[p.UID]
	if !inTeam {
		return TeamErrNoTeam
	}
	if team.Leader != p.UID {
		return TeamErrNotLeader
	}
	req = m.normalizeReq(req)
	if req.MaxMember <= 0 {
		return TeamErrBadRequirement
	}
	// 上限不能小于当前人数（否则队伍人数超限）
	if int(req.MaxMember) < len(team.Members) {
		req.MaxMember = int32(len(team.Members))
	}
	team.Req = req
	m.pushTeam(ctx, team)
	return TeamErrNone
}

// QueryTeamInfo 查某人的队伍信息（CM_QUERYTEAMINFO）。
func (m *TeamManager) QueryTeamInfo(ctx context.Context, asker *Player, targetUID uint64) {
	m.mu.Lock()
	team, inTeam := m.byUID[targetUID]
	if !inTeam {
		m.mu.Unlock()
		return
	}
	leader := team.Leader
	memberCount := len(team.Members)
	req := team.Req
	m.mu.Unlock()

	leaderPlayer, ok := m.Players.Get(leader)
	if !ok {
		return
	}
	payload, err := protocol.CerealSerialize(protocol.SDTeamInfo{
		Leader:      leader,
		Name:        leaderPlayer.Name,
		Job:         int32(leaderPlayer.Job),
		Level:       int32(leaderPlayer.Level),
		MemberCount: int32(memberCount),
		Req:         req,
	})
	if err != nil {
		return
	}
	m.sendTo(asker, protocol.SM_TEAMINFO, payload)
}

// OnLeaveGame 玩家下线/离开时清理队伍关系。
//
// ★ 必须调用，否则下线的人会永远挂在队伍列表里，
// 队长看到的是一个已经不在线的成员，且永远占着名额。
func (m *TeamManager) OnLeaveGame(ctx context.Context, uid uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, inTeam := m.byUID[uid]
	if !inTeam {
		return
	}
	if team.Leader == uid {
		m.disbandLocked(ctx, team)
		return
	}
	m.removeMemberLocked(ctx, team, uid)
	m.pushTeam(ctx, team)
}

// ==================== 内部 ====================

// normalizeReq 修正非法要求（上限钳到 [1, MaxTeamMember]）。
func (m *TeamManager) normalizeReq(req protocol.SDTeamReq) protocol.SDTeamReq {
	if req.MaxMember <= 0 {
		req.MaxMember = MaxTeamMember
	}
	if req.MaxMember > MaxTeamMember {
		req.MaxMember = MaxTeamMember
	}
	return req
}

// addMemberLocked 把某人加进队伍（调用方已持锁）。
func (m *TeamManager) addMemberLocked(ctx context.Context, team *Team, uid uint64) uint8 {
	if _, already := m.byUID[uid]; already {
		return TeamErrInTeam
	}
	if len(team.Members) >= int(team.Req.MaxMember) {
		return TeamErrFull
	}
	team.Members = append(team.Members, uid)
	m.byUID[uid] = team
	m.pushTeam(ctx, team)
	return TeamErrNone
}

// removeMemberLocked 从队伍移出某人（调用方已持锁）。
func (m *TeamManager) removeMemberLocked(ctx context.Context, team *Team, uid uint64) {
	for i, member := range team.Members {
		if member == uid {
			team.Members = append(team.Members[:i], team.Members[i+1:]...)
			break
		}
	}
	delete(m.byUID, uid)
	// 队长走了要顺位给下一个
	if team.Leader == uid && len(team.Members) > 0 {
		delete(m.byLeader, team.Leader)
		team.Leader = team.Members[0]
		m.byLeader[team.Leader] = team
	}
}

// disbandLocked 解散队伍（调用方已持锁）。
func (m *TeamManager) disbandLocked(ctx context.Context, team *Team) {
	members := append([]uint64(nil), team.Members...)
	for _, uid := range members {
		delete(m.byUID, uid)
	}
	delete(m.byLeader, team.Leader)
	// 通知所有原成员「队伍没了」
	for _, uid := range members {
		m.pushEmptyTeam(uid)
	}
}

// pushTeam 把队伍成员列表推给所有成员（SM_TEAMMEMBERLIST）。
func (m *TeamManager) pushTeam(ctx context.Context, team *Team) {
	list := protocol.SDTeamMemberList{
		TeamLeader: team.Leader,
		MemberList: make([]protocol.SDTeamPlayer, 0, len(team.Members)),
		Req:        team.Req,
	}
	for _, uid := range team.Members {
		member, ok := m.Players.Get(uid)
		if !ok {
			// 成员不在线：仍然占位，用零值填（客户端显示为离线）
			list.MemberList = append(list.MemberList, protocol.SDTeamPlayer{UID: uid})
			continue
		}
		list.MemberList = append(list.MemberList, protocol.SDTeamPlayer{
			UID:   uid,
			Level: uint32(member.Level),
			Job:   int32(member.Job),
			Name:  member.Name,
		})
	}
	payload, err := protocol.CerealSerialize(list)
	if err != nil {
		return
	}
	for _, uid := range team.Members {
		if p, ok := m.Players.Get(uid); ok {
			m.sendTo(p, protocol.SM_TEAMMEMBERLIST, payload)
		}
	}
}

// pushEmptyTeam 把「空队伍列表」推给某人（他被移出队伍了）。
func (m *TeamManager) pushEmptyTeam(uid uint64) {
	if p, ok := m.Players.Get(uid); ok {
		payload, err := protocol.CerealSerialize(protocol.SDTeamMemberList{})
		if err == nil {
			m.sendTo(p, protocol.SM_TEAMMEMBERLIST, payload)
		}
	}
}

// notifyJoinRequest 通知队长「有人申请入队」。
//
// 原版把申请者资料（等级/职业/已学技能）推给队长，队长据此决定收不收。
// 这里用 SM_TEAMINFO 推一个精简版 —— 客户端主要看名字和等级。
func (m *TeamManager) notifyJoinRequest(applicant *Player, leaderUID uint64) {
	leader, ok := m.Players.Get(leaderUID)
	if !ok {
		return
	}
	payload, err := protocol.CerealSerialize(protocol.SDTeamInfo{
		Leader:      applicant.UID,
		Name:        applicant.Name,
		Job:         int32(applicant.Job),
		Level:       int32(applicant.Level),
		MemberCount: 1,
	})
	if err != nil {
		return
	}
	m.sendTo(leader, protocol.SM_TEAMINFO, payload)
}

func (m *TeamManager) sendTo(p *Player, id protocol.SMType, payload []byte) {
	if m.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}
	_ = m.Broadcast.SendTo(p.ChannelID, id, payload, 0)
}

// pushTeamError 发组队错误码（SM_TEAMERROR）。
func (m *TeamManager) pushTeamError(p *Player, code uint8) {
	if code == TeamErrNone {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMTeamError{Error: code})
	if err == nil {
		m.sendTo(p, protocol.SM_TEAMERROR, payload)
	}
}

func containsUID(list []uint64, uid uint64) bool {
	for _, v := range list {
		if v == uid {
			return true
		}
	}
	return false
}

// ==================== 路由入口（channelID -> Player -> 业务 -> 错误码）====================

func (m *TeamManager) handleJoin(ctx context.Context, channelID uint64, targetUID uint64) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.pushTeamError(p, m.RequestJoin(ctx, p, targetUID))
	return nil
}

func (m *TeamManager) handleLeave(ctx context.Context, channelID uint64, targetUID uint64) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.pushTeamError(p, m.Leave(ctx, p, targetUID))
	return nil
}

func (m *TeamManager) handleQueryInfo(ctx context.Context, channelID uint64, targetUID uint64) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.QueryTeamInfo(ctx, p, targetUID)
	return nil
}

func (m *TeamManager) handleTransfer(ctx context.Context, channelID uint64, targetUID uint64) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.pushTeamError(p, m.TransferLeader(ctx, p, targetUID))
	return nil
}

func (m *TeamManager) handleCreate(ctx context.Context, channelID uint64, req protocol.SDTeamReq) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.pushTeamError(p, m.Create(ctx, p, req))
	return nil
}

func (m *TeamManager) handleSetRequirement(ctx context.Context, channelID uint64, req protocol.SDTeamReq) error {
	p, ok := m.Players.ByChannel(channelID)
	if !ok {
		return nil
	}
	m.pushTeamError(p, m.SetRequirement(ctx, p, req))
	return nil
}
