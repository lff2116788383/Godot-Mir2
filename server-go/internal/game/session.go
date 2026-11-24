package game

import (
	"errors"
	"sync"
)

// ==================== 登录会话（servicecorenet.cpp 的 LoginSession）====================
//
// 原版 ServiceCore 维护 m_sessionList: channID -> LoginSession{account, dbid}。
// 这里照搬同样的语义，但改成按会话 ID 索引，并且线程安全
// （Go 的 transport 是每连接一个 goroutine，不像原版是单 actor）。
//
// 三个阶段严格串行，任何一步失败都不应推进：
//
//	CM_LOGIN     -> SM_LOGINOK    session.account 填上
//	CM_QUERYCHAR -> SM_QUERYCHAROK 用 account 查角色
//	CM_ONLINE    -> SM_ONLINEOK  session.dbid 填上，真正进世界
//
// ★ 几个必须照抄的判定：
//  1. 登录**不校验 id/password 格式**（原版注释明确写了，允许 test/123456）
//  2. 同一 account 重复登录返回 LOGINERR_MULTILOGIN
//  3. CM_ONLINE 时若 dbid 已存在返回 ONLINEERR_MULTIONLINE

// 原版错误码（protocoldef.hpp 的 LOGINERR_* / QUERYCHARERR_* / ONLINEERR_*）
const (
	LoginErrNone        = 0
	LoginErrNoAccount   = 1 // 账号或密码错
	LoginErrMultiLogin  = 2 // 同账号已在别处登录
	LoginErrEmptyID     = 3
	LoginErrBadPassword = 4

	QueryCharErrNone    = 0
	QueryCharErrNoLogin = 1 // 尚未 CM_LOGIN
	QueryCharErrNoChar  = 2 // 该账号下没有角色

	OnlineErrNone        = 0
	OnlineErrNoLogin     = 1
	OnlineErrMultiOnline = 2
	OnlineErrNoChar      = 3

	CreateCharErrNone       = 0
	CreateCharErrBadName    = 1
	CreateCharErrNameExists = 2
	CreateCharErrBadJob     = 3
)

// Session 是一个已连接客户端的登录态。
type Session struct {
	// ChannelID 连接标识（transport.Session.ID()）
	ChannelID uint64
	// Account 登录成功后的账号名
	Account string
	// DBID 进入世界后的角色库 ID。0 表示还没 online
	DBID uint32
}

// SessionManager 维护所有连接的登录态。
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[uint64]*Session
	// accountIndex 用于查「同账号是否已在别处登录」
	accountIndex map[string]uint64
}

// NewSessionManager 创建会话管理器。
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions:     make(map[uint64]*Session),
		accountIndex: make(map[string]uint64),
	}
}

// ErrSessionNotFound 表示会话不存在（连接已断或未登录）。
var ErrSessionNotFound = errors.New("session not found")

// ErrMultiLogin 表示同账号已在别处登录。
var ErrMultiLogin = errors.New("account already logged in elsewhere")

// ErrMultiOnline 表示该角色已在游戏中。
var ErrMultiOnline = errors.New("character already online")

// Login 记录账号登录成功。
//
// 原版 servicecorenet.cpp:44-56 的逻辑：
//
//	for (existChannID, session) in m_sessionList:
//	    if session.account == account: return LOGINERR_MULTILOGIN
//	m_sessionList[channID] = LoginSession{account}
func (m *SessionManager) Login(channelID uint64, account string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.accountIndex[account]; ok && existing != channelID {
		return ErrMultiLogin
	}
	m.sessions[channelID] = &Session{ChannelID: channelID, Account: account}
	m.accountIndex[account] = channelID
	return nil
}

// Get 取会话快照。
func (m *SessionManager) Get(channelID uint64) (Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[channelID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return *s, nil
}

// MarkOnline 记录角色进入世界。
//
// 原版 servicecorenet.cpp:141 的注释写着「mark online before co_await」——
// 是为了让并发的第二次 CM_ONLINE 撞上 MULTIONLINE。这里同样先占位。
func (m *SessionManager) MarkOnline(channelID uint64, dbid uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[channelID]
	if !ok {
		return ErrSessionNotFound
	}
	if s.DBID != 0 {
		return ErrMultiOnline
	}
	s.DBID = dbid
	return nil
}

// Offline 回滚 online 标记（进场失败时用）。
//
// 不清 DBID 会让玩家卡在「已在线但不在世界里」的状态，
// 之后所有 CM_ONLINE 都被判 MULTIONLINE，永远上不去。
func (m *SessionManager) Offline(channelID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[channelID]; ok {
		s.DBID = 0
	}
}

// IsCharacterOnline 判断某角色是否已在某个会话里 online。
func (m *SessionManager) IsCharacterOnline(dbid uint32) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.DBID == dbid {
			return true
		}
	}
	return false
}

// Drop 清理会话（连接断开时调用）。
func (m *SessionManager) Drop(channelID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[channelID]
	if !ok {
		return
	}
	// 只有索引指向自己时才删，避免误删别人的
	if owner, exists := m.accountIndex[s.Account]; exists && owner == channelID {
		delete(m.accountIndex, s.Account)
	}
	delete(m.sessions, channelID)
}

// Count 返回当前会话数。
func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// Reset 清空所有会话（测试用）。
func (m *SessionManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = make(map[uint64]*Session)
	m.accountIndex = make(map[string]uint64)
}
