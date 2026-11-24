package game

import (
	"context"
	"fmt"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// ==================== 登录链路处理器（servicecorenet.cpp）====================
//
// 覆盖 6 条消息：
//
//	CM_LOGIN           -> SM_LOGINOK / SM_LOGINERROR
//	CM_QUERYCHAR       -> SM_QUERYCHAROK / SM_QUERYCHARERROR
//	CM_ONLINE          -> SM_ONLINEOK / SM_ONLINEERROR
//	CM_CREATEACCOUNT   -> SM_CREATEACCOUNTOK / SM_CREATEACCOUNTERROR
//	CM_CHANGEPASSWORD  -> SM_CHANGEPASSWORDOK / SM_CHANGEPASSWORDERROR
//	CM_CREATECHAR      -> SM_CREATECHAROK / SM_CREATECHARERROR
//	CM_DELETECHAR      -> SM_DELETECHAROK / SM_DELETECHARERROR
//
// 与 storage 层的分工：本层只做「协议解码 → 查库/写库 → 协议编码」，
// 不承载任何游戏逻辑。这样所有持久化都走同一套仓储，避免两处 SQL。

// Sender 是发送回包所需的最小接口，避免本包依赖 transport（会造成循环）。
type Sender interface {
	Send(id protocol.SMType, payload []byte, responseID uint64) error
}

// Handler 承载登录链路的依赖。
type Handler struct {
	Sessions *SessionManager
	Accounts *storage.AccountRepository
	Chars    *storage.CharacterRepository
	// Worlds 进场时把玩家放进地图。为 nil 时只做登录不进场（测试/简化模式）
	Worlds *WorldService
}

// NewHandler 装配登录处理器。
func NewHandler(sessions *SessionManager, accounts *storage.AccountRepository, chars *storage.CharacterRepository) *Handler {
	return &Handler{Sessions: sessions, Accounts: accounts, Chars: chars}
}

// 默认出生点（原版 tbl_char 的 fld_map/fld_mapx/fld_mapy 默认值）
const (
	DefaultSpawnMap uint32 = 1
	DefaultSpawnX   int32  = 400
	DefaultSpawnY   int32  = 402
)

// sendPayload 编码并发送一条 SM。
func sendPayload(sender Sender, id protocol.SMType, value any, respID uint64) error {
	payload, err := protocol.AppendWirePayload(nil, value)
	if err != nil {
		return fmt.Errorf("encode %v: %w", id, err)
	}
	return sender.Send(id, payload, respID)
}

// CMLogin 处理登录。
//
// 原版 servicecorenet.cpp:19-63：
//
//  1. 查 tbl_account（account + password）
//  2. 查不到 -> LOGINERR_NOACCOUNT
//  3. 同 account 已在别的 channID 上 -> LOGINERR_MULTILOGIN
//  4. 记入 m_sessionList，回 SM_LOGINOK（空 body）
//
// ★ 不校验 id/password 格式（原版注释明确写了，允许 test/123456）
func (h *Handler) CMLogin(ctx context.Context, channelID, respID uint64, req protocol.CMLogin, sender Sender) error {
	account := req.ID.String()
	password := req.Password.String()

	if account == "" {
		return sendPayload(sender, protocol.SM_LOGINERROR, protocol.SMLoginError{Error: LoginErrEmptyID}, respID)
	}

	ok, err := h.Accounts.Authenticate(ctx, account, password)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	if !ok {
		return sendPayload(sender, protocol.SM_LOGINERROR, protocol.SMLoginError{Error: LoginErrNoAccount}, respID)
	}

	if err := h.Sessions.Login(channelID, account); err != nil {
		// 同账号别处登录
		return sendPayload(sender, protocol.SM_LOGINERROR, protocol.SMLoginError{Error: LoginErrMultiLogin}, respID)
	}

	// SM_LOGINOK 是空 body
	return sender.Send(protocol.SM_LOGINOK, nil, respID)
}

// CMQueryChar 查询账号下的角色。
//
// 原版 servicecorenet.cpp:66-98：未登录 -> QUERYCHARERR_NOLOGIN；
// 查不到角色 -> QUERYCHARERR_NOCHAR；否则回 SM_QUERYCHAROK。
func (h *Handler) CMQueryChar(ctx context.Context, channelID, respID uint64, sender Sender) error {
	session, err := h.Sessions.Get(channelID)
	if err != nil {
		return sendPayload(sender, protocol.SM_QUERYCHARERROR,
			protocol.SMQueryCharError{Error: QueryCharErrNoLogin}, respID)
	}

	ch, err := h.Chars.GetByAccount(ctx, session.Account)
	if err != nil {
		return sendPayload(sender, protocol.SM_QUERYCHARERROR,
			protocol.SMQueryCharError{Error: QueryCharErrNoChar}, respID)
	}

	reply := protocol.SMQueryCharOK{
		Gender: ch.Gender,
		Job:    ch.Job,
		Exp:    ch.Exp,
	}
	if err := reply.Name.Assign([]byte(ch.Name)); err != nil {
		return fmt.Errorf("assign char name: %w", err)
	}
	return sendPayload(sender, protocol.SM_QUERYCHAROK, reply, respID)
}

// CMOnline 进入世界。
//
// 原版 servicecorenet.cpp:101-200：未登录 / 已 online / 无角色 都要报错。
// 这里只负责「鉴权 + 回 SM_ONLINEOK」，实际把玩家放进地图由 World 层负责
// （见 OnlineResult，上层拿到后调用 world.AddEntity）。
func (h *Handler) CMOnline(ctx context.Context, channelID, respID uint64, sender Sender) (OnlineResult, error) {
	session, err := h.Sessions.Get(channelID)
	if err != nil {
		if err := sendPayload(sender, protocol.SM_ONLINEERROR,
			protocol.SMOnlineError{Error: OnlineErrNoLogin}, respID); err != nil {
			return OnlineResult{}, err
		}
		return OnlineResult{}, nil
	}

	ch, err := h.Chars.GetByAccount(ctx, session.Account)
	if err != nil {
		if err := sendPayload(sender, protocol.SM_ONLINEERROR,
			protocol.SMOnlineError{Error: OnlineErrNoChar}, respID); err != nil {
			return OnlineResult{}, err
		}
		return OnlineResult{}, nil
	}

	// 先占位再回包（原版注释：mark online before co_await）
	if err := h.Sessions.MarkOnline(channelID, ch.DBID); err != nil {
		if err := sendPayload(sender, protocol.SM_ONLINEERROR,
			protocol.SMOnlineError{Error: OnlineErrMultiOnline}, respID); err != nil {
			return OnlineResult{}, err
		}
		return OnlineResult{}, nil
	}

	mapUID, err := common.GetBaseMapUID(ch.Map, 0)
	if err != nil {
		return OnlineResult{}, fmt.Errorf("build map uid: %w", err)
	}
	playerUID := common.GetPlayerUID(ch.DBID)

	// ★ 先进地图再回包：落地坐标可能被占用/不可走，需要读回真实位置，
	//   否则客户端拿到的坐标与服务端不一致，进场后第一步就会「被拉回」。
	spawnX, spawnY := ch.MapX, ch.MapY
	if h.Worlds != nil {
		player, err := h.Worlds.Enter(ctx, ch, channelID)
		if err != nil {
			// 进场失败要回滚 online 标记，否则玩家永远上不去
			h.Sessions.Offline(channelID)
			if sendErr := sendPayload(sender, protocol.SM_ONLINEERROR,
				protocol.SMOnlineError{Error: OnlineErrNoChar}, respID); sendErr != nil {
				return OnlineResult{}, sendErr
			}
			return OnlineResult{}, nil
		}
		spawnX, spawnY = int32(player.X), int32(player.Y)
		mapUID = player.MapUID
	}

	reply := protocol.SMOnlineOK{
		UID:    playerUID,
		Gender: ch.Gender != 0,
		Job:    ch.Job,
		MapUID: mapUID,
		Action: common.ActionNode{
			Type:      common.ActionStand, // 刚进场是站立
			Direction: uint8(common.DirectionDown),
			X:         int16(spawnX),
			Y:         int16(spawnY),
			AimX:      int16(spawnX),
			AimY:      int16(spawnY),
		},
	}
	if err := reply.Name.Assign([]byte(ch.Name)); err != nil {
		return OnlineResult{}, fmt.Errorf("assign char name: %w", err)
	}
	if err := sendPayload(sender, protocol.SM_ONLINEOK, reply, respID); err != nil {
		return OnlineResult{}, err
	}

	// 进场后把视野内已有实体推给新玩家（否则他只看得到自己）
	if h.Worlds != nil {
		h.Worlds.SendSnapshot(ctx, ch.Map, playerUID)
	}

	return OnlineResult{
		ChannelID: channelID,
		Character: ch,
		PlayerUID: playerUID,
		MapUID:    mapUID,
		X:         int(spawnX),
		Y:         int(spawnY),
	}, nil
}

// OnlineResult 是 CM_ONLINE 成功后交给 World 层的入世界参数。
type OnlineResult struct {
	ChannelID uint64
	Character storage.Character
	PlayerUID uint64
	MapUID    uint64
	X, Y      int
}

// CMCreateAccount 创建账号。
//
// 原版 servicecorenet / processcreateaccount：账号已存在 -> 错误码 1；
// 格式非法 -> 错误码 2（服务端这里只做非空校验，格式校验留在客户端）。
func (h *Handler) CMCreateAccount(ctx context.Context, respID uint64, req protocol.CMCreateAccount, sender Sender) error {
	account := req.ID.String()
	password := req.Password.String()

	if account == "" || password == "" {
		return sendPayload(sender, protocol.SM_CREATEACCOUNTERROR,
			protocol.SMCreateAccountError{Error: 2}, respID)
	}

	exists, err := h.Accounts.Exists(ctx, account)
	if err != nil {
		return fmt.Errorf("check account: %w", err)
	}
	if exists {
		return sendPayload(sender, protocol.SM_CREATEACCOUNTERROR,
			protocol.SMCreateAccountError{Error: 1}, respID)
	}

	if err := h.Accounts.Create(ctx, account, password); err != nil {
		return sendPayload(sender, protocol.SM_CREATEACCOUNTERROR,
			protocol.SMCreateAccountError{Error: 3}, respID)
	}
	return sender.Send(protocol.SM_CREATEACCOUNTOK, nil, respID)
}

// CMChangePassword 修改密码。
func (h *Handler) CMChangePassword(ctx context.Context, respID uint64, req protocol.CMChangePassword, sender Sender) error {
	account := req.ID.String()
	oldPwd := req.Password.String()
	newPwd := req.PasswordNew.String()

	ok, err := h.Accounts.Authenticate(ctx, account, oldPwd)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	if !ok {
		return sendPayload(sender, protocol.SM_CHANGEPASSWORDERROR,
			protocol.SMChangePasswordError{Error: 1}, respID)
	}
	if err := h.Accounts.ChangePassword(ctx, account, newPwd); err != nil {
		return sendPayload(sender, protocol.SM_CHANGEPASSWORDERROR,
			protocol.SMChangePasswordError{Error: 2}, respID)
	}
	return sender.Send(protocol.SM_CHANGEPASSWORDOK, nil, respID)
}

// CMCreateChar 创建角色。
//
// 原版 processcreatechar：名字非法 / 重名 / 职业非法都报错。
// 一个账号只允许一个角色（原版 tbl_char 按 account 唯一）。
func (h *Handler) CMCreateChar(ctx context.Context, channelID, respID uint64, req protocol.CMCreateChar, sender Sender) error {
	session, err := h.Sessions.Get(channelID)
	if err != nil {
		return sendPayload(sender, protocol.SM_CREATECHARERROR,
			protocol.SMCreateCharError{Error: CreateCharErrBadName}, respID)
	}

	name := req.Name.String()
	if name == "" {
		return sendPayload(sender, protocol.SM_CREATECHARERROR,
			protocol.SMCreateCharError{Error: CreateCharErrBadName}, respID)
	}

	// 已有角色则拒绝（原版查 fld_account 存在即不建）
	if _, err := h.Chars.GetByAccount(ctx, session.Account); err == nil {
		return sendPayload(sender, protocol.SM_CREATECHARERROR,
			protocol.SMCreateCharError{Error: CreateCharErrNameExists}, respID)
	}

	exists, err := h.Chars.ExistsByName(ctx, name)
	if err != nil {
		return fmt.Errorf("check char name: %w", err)
	}
	if exists {
		return sendPayload(sender, protocol.SM_CREATECHARERROR,
			protocol.SMCreateCharError{Error: CreateCharErrNameExists}, respID)
	}

	if _, err := h.Chars.Create(ctx, storage.CreateCharacterParams{
		Account: session.Account,
		Name:    name,
		Gender:  req.Gender,
		Job:     req.Job,
		Map:     DefaultSpawnMap,
		MapX:    DefaultSpawnX,
		MapY:    DefaultSpawnY,
	}); err != nil {
		return sendPayload(sender, protocol.SM_CREATECHARERROR,
			protocol.SMCreateCharError{Error: CreateCharErrBadJob}, respID)
	}
	return sender.Send(protocol.SM_CREATECHAROK, nil, respID)
}

// CMDeleteChar 删除角色。
func (h *Handler) CMDeleteChar(ctx context.Context, channelID, respID uint64, req protocol.CMDeleteChar, sender Sender) error {
	session, err := h.Sessions.Get(channelID)
	if err != nil {
		return sendPayload(sender, protocol.SM_DELETECHARERROR,
			protocol.SMDeleteCharError{Error: 1}, respID)
	}

	// 删角色需校验密码（原版 processselectchar 的删除流程）
	ok, err := h.Accounts.Authenticate(ctx, session.Account, req.Password.String())
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	if !ok {
		return sendPayload(sender, protocol.SM_DELETECHARERROR,
			protocol.SMDeleteCharError{Error: 2}, respID)
	}

	ch, err := h.Chars.GetByAccount(ctx, session.Account)
	if err != nil {
		return sendPayload(sender, protocol.SM_DELETECHARERROR,
			protocol.SMDeleteCharError{Error: 3}, respID)
	}
	if err := h.Chars.Delete(ctx, ch.DBID); err != nil {
		return fmt.Errorf("delete char: %w", err)
	}
	return sender.Send(protocol.SM_DELETECHAROK, nil, respID)
}
