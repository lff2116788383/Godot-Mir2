package game

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// fakeSender 记录发出的消息，用于断言回包内容。
type fakeSender struct {
	sent   []sentMessage
	failed bool
}

type sentMessage struct {
	ID      protocol.SMType
	Payload []byte
	RespID  uint64
}

func (f *fakeSender) Send(id protocol.SMType, payload []byte, responseID uint64) error {
	if f.failed {
		return context.Canceled
	}
	f.sent = append(f.sent, sentMessage{ID: id, Payload: payload, RespID: responseID})
	return nil
}

func (f *fakeSender) last() (sentMessage, bool) {
	if len(f.sent) == 0 {
		return sentMessage{}, false
	}
	return f.sent[len(f.sent)-1], true
}

func (f *fakeSender) reset() { f.sent = nil }

func newTestHandler(t *testing.T) (*Handler, *storage.Store) {
	t.Helper()
	ctx := context.Background()
	store, err := storage.Open(ctx, storage.OpenOptions{
		Path: filepath.Join(t.TempDir(), "login.db3"), CreateIfMissing: true,
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	sessions := NewSessionManager()
	h := NewHandler(sessions,
		storage.NewAccountRepository(store),
		storage.NewCharacterRepository(store),
	)
	return h, store
}

// mustString 把字符串塞进 StaticBuffer。
func mustString[C common.Capacity](t *testing.T, value string) common.StaticBuffer[C] {
	t.Helper()
	var buf common.StaticBuffer[C]
	if err := buf.Assign([]byte(value)); err != nil {
		t.Fatalf("assign %q: %v", value, err)
	}
	return buf
}

func TestLoginFlowHappyPath(t *testing.T) {
	ctx := context.Background()
	h, store := newTestHandler(t)

	// 建账号
	accounts := storage.NewAccountRepository(store)
	if err := accounts.Create(ctx, "test", "123456"); err != nil {
		t.Fatalf("create account: %v", err)
	}
	// 建角色
	chars := storage.NewCharacterRepository(store)
	if _, err := chars.Create(ctx, storage.CreateCharacterParams{
		Account: "test", Name: "测试角色", Gender: 1, Job: 1,
		Map: DefaultSpawnMap, MapX: DefaultSpawnX, MapY: DefaultSpawnY,
	}); err != nil {
		t.Fatalf("create char: %v", err)
	}

	sender := &fakeSender{}

	// --- CM_LOGIN ---
	loginReq := protocol.CMLogin{
		ID:       mustString[common.Cap64](t, "test"),
		Password: mustString[common.Cap64](t, "123456"),
	}
	if err := h.CMLogin(ctx, 1, 100, loginReq, sender); err != nil {
		t.Fatalf("CMLogin: %v", err)
	}
	msg, ok := sender.last()
	if !ok || msg.ID != protocol.SM_LOGINOK {
		t.Fatalf("expected SM_LOGINOK, got %v (ok=%v)", msg.ID, ok)
	}
	if len(msg.Payload) != 0 {
		t.Fatalf("SM_LOGINOK should have empty body, got %d bytes", len(msg.Payload))
	}
	if msg.RespID != 100 {
		t.Fatalf("respID = %d, want 100", msg.RespID)
	}

	// --- CM_QUERYCHAR ---
	sender.reset()
	if err := h.CMQueryChar(ctx, 1, 101, sender); err != nil {
		t.Fatalf("CMQueryChar: %v", err)
	}
	msg, _ = sender.last()
	if msg.ID != protocol.SM_QUERYCHAROK {
		t.Fatalf("expected SM_QUERYCHAROK, got %v", msg.ID)
	}
	var qc protocol.SMQueryCharOK
	n, err := protocol.ReadWirePayload(&qc, msg.Payload)
	if err != nil {
		t.Fatalf("decode SMQueryCharOK: %v", err)
	}
	if n != len(msg.Payload) {
		t.Fatalf("consumed %d of %d", n, len(msg.Payload))
	}
	if qc.Name.String() != "测试角色" {
		t.Fatalf("name = %q, want 测试角色", qc.Name.String())
	}
	if qc.Gender != 1 || qc.Job != 1 {
		t.Fatalf("gender/job = %d/%d, want 1/1", qc.Gender, qc.Job)
	}

	// --- CM_ONLINE ---
	sender.reset()
	result, err := h.CMOnline(ctx, 1, 102, sender)
	if err != nil {
		t.Fatalf("CMOnline: %v", err)
	}
	msg, _ = sender.last()
	if msg.ID != protocol.SM_ONLINEOK {
		t.Fatalf("expected SM_ONLINEOK, got %v", msg.ID)
	}

	var online protocol.SMOnlineOK
	if _, err := protocol.ReadWirePayload(&online, msg.Payload); err != nil {
		t.Fatalf("decode SMOnlineOK: %v", err)
	}
	wantUID := common.GetPlayerUID(result.Character.DBID)
	if online.UID != wantUID {
		t.Fatalf("online uid = %#x, want %#x", online.UID, wantUID)
	}
	if online.Name.String() != "测试角色" {
		t.Fatalf("online name = %q", online.Name.String())
	}
	if !online.Gender {
		t.Fatal("gender should be true (male)")
	}
	if online.Job != 1 {
		t.Fatalf("online job = %d, want 1", online.Job)
	}
	wantMapUID, _ := common.GetBaseMapUID(DefaultSpawnMap, 0)
	if online.MapUID != wantMapUID {
		t.Fatalf("map uid = %#x, want %#x", online.MapUID, wantMapUID)
	}
	if online.Action.Type != common.ActionStand {
		t.Fatalf("action type = %v, want STAND", online.Action.Type)
	}
	if online.Action.Direction != common.DirectionDown {
		t.Fatalf("direction = %d, want DOWN(%d)", online.Action.Direction, common.DirectionDown)
	}
	if int(online.Action.X) != int(DefaultSpawnX) || int(online.Action.Y) != int(DefaultSpawnY) {
		t.Fatalf("spawn = (%d,%d), want (%d,%d)", online.Action.X, online.Action.Y, DefaultSpawnX, DefaultSpawnY)
	}

	// 会话应记录 dbid
	session, _ := h.Sessions.Get(1)
	if session.DBID != result.Character.DBID {
		t.Fatalf("session dbid = %d, want %d", session.DBID, result.Character.DBID)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	ctx := context.Background()
	h, store := newTestHandler(t)
	accounts := storage.NewAccountRepository(store)
	if err := accounts.Create(ctx, "test", "123456"); err != nil {
		t.Fatalf("create account: %v", err)
	}

	sender := &fakeSender{}
	req := protocol.CMLogin{
		ID:       mustString[common.Cap64](t, "test"),
		Password: mustString[common.Cap64](t, "wrong"),
	}
	if err := h.CMLogin(ctx, 1, 1, req, sender); err != nil {
		t.Fatalf("CMLogin: %v", err)
	}
	msg, _ := sender.last()
	if msg.ID != protocol.SM_LOGINERROR {
		t.Fatalf("expected SM_LOGINERROR, got %v", msg.ID)
	}
	var e protocol.SMLoginError
	if _, err := protocol.ReadWirePayload(&e, msg.Payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if e.Error != LoginErrNoAccount {
		t.Fatalf("error = %d, want %d", e.Error, LoginErrNoAccount)
	}
	// 失败不应建会话
	if h.Sessions.Count() != 0 {
		t.Fatalf("sessions = %d, want 0", h.Sessions.Count())
	}
}

func TestLoginMissingAccount(t *testing.T) {
	ctx := context.Background()
	h, _ := newTestHandler(t)
	sender := &fakeSender{}
	req := protocol.CMLogin{
		ID:       mustString[common.Cap64](t, "nobody"),
		Password: mustString[common.Cap64](t, "123456"),
	}
	if err := h.CMLogin(ctx, 1, 1, req, sender); err != nil {
		t.Fatalf("CMLogin: %v", err)
	}
	if msg, _ := sender.last(); msg.ID != protocol.SM_LOGINERROR {
		t.Fatalf("expected SM_LOGINERROR, got %v", msg.ID)
	}
}

func TestLoginMultiAccount(t *testing.T) {
	ctx := context.Background()
	h, store := newTestHandler(t)
	accounts := storage.NewAccountRepository(store)
	if err := accounts.Create(ctx, "test", "123456"); err != nil {
		t.Fatalf("create account: %v", err)
	}

	req := protocol.CMLogin{
		ID:       mustString[common.Cap64](t, "test"),
		Password: mustString[common.Cap64](t, "123456"),
	}
	// 第一个连接登录成功
	if err := h.CMLogin(ctx, 1, 1, req, &fakeSender{}); err != nil {
		t.Fatalf("first login: %v", err)
	}
	// 第二个连接同账号必须被拒
	sender := &fakeSender{}
	if err := h.CMLogin(ctx, 2, 2, req, sender); err != nil {
		t.Fatalf("second login: %v", err)
	}
	msg, _ := sender.last()
	if msg.ID != protocol.SM_LOGINERROR {
		t.Fatalf("expected SM_LOGINERROR, got %v", msg.ID)
	}
	var e protocol.SMLoginError
	_, _ = protocol.ReadWirePayload(&e, msg.Payload)
	if e.Error != LoginErrMultiLogin {
		t.Fatalf("error = %d, want MULTILOGIN(%d)", e.Error, LoginErrMultiLogin)
	}
}

func TestQueryCharWithoutLogin(t *testing.T) {
	ctx := context.Background()
	h, _ := newTestHandler(t)
	sender := &fakeSender{}
	if err := h.CMQueryChar(ctx, 99, 1, sender); err != nil {
		t.Fatalf("CMQueryChar: %v", err)
	}
	msg, _ := sender.last()
	if msg.ID != protocol.SM_QUERYCHARERROR {
		t.Fatalf("expected SM_QUERYCHARERROR, got %v", msg.ID)
	}
	var e protocol.SMQueryCharError
	_, _ = protocol.ReadWirePayload(&e, msg.Payload)
	if e.Error != QueryCharErrNoLogin {
		t.Fatalf("error = %d, want NOLOGIN(%d)", e.Error, QueryCharErrNoLogin)
	}
}

func TestQueryCharNoCharacter(t *testing.T) {
	ctx := context.Background()
	h, store := newTestHandler(t)
	accounts := storage.NewAccountRepository(store)
	if err := accounts.Create(ctx, "empty", "123456"); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := h.Sessions.Login(1, "empty"); err != nil {
		t.Fatalf("session login: %v", err)
	}

	sender := &fakeSender{}
	if err := h.CMQueryChar(ctx, 1, 1, sender); err != nil {
		t.Fatalf("CMQueryChar: %v", err)
	}
	var e protocol.SMQueryCharError
	msg, _ := sender.last()
	_, _ = protocol.ReadWirePayload(&e, msg.Payload)
	if e.Error != QueryCharErrNoChar {
		t.Fatalf("error = %d, want NOCHAR(%d)", e.Error, QueryCharErrNoChar)
	}
}

func TestOnlineWithoutLogin(t *testing.T) {
	ctx := context.Background()
	h, _ := newTestHandler(t)
	sender := &fakeSender{}
	if _, err := h.CMOnline(ctx, 42, 1, sender); err != nil {
		t.Fatalf("CMOnline: %v", err)
	}
	msg, _ := sender.last()
	if msg.ID != protocol.SM_ONLINEERROR {
		t.Fatalf("expected SM_ONLINEERROR, got %v", msg.ID)
	}
	var e protocol.SMOnlineError
	_, _ = protocol.ReadWirePayload(&e, msg.Payload)
	if e.Error != OnlineErrNoLogin {
		t.Fatalf("error = %d, want NOLOGIN(%d)", e.Error, OnlineErrNoLogin)
	}
}

func TestCreateAccount(t *testing.T) {
	ctx := context.Background()
	h, _ := newTestHandler(t)

	sender := &fakeSender{}
	req := protocol.CMCreateAccount{
		ID:       mustString[common.Cap64](t, "newuser"),
		Password: mustString[common.Cap64](t, "pwd123"),
	}
	if err := h.CMCreateAccount(ctx, 1, req, sender); err != nil {
		t.Fatalf("CMCreateAccount: %v", err)
	}
	if msg, _ := sender.last(); msg.ID != protocol.SM_CREATEACCOUNTOK {
		t.Fatalf("expected SM_CREATEACCOUNTOK, got %v", msg.ID)
	}

	// 重复创建必须报错
	sender.reset()
	if err := h.CMCreateAccount(ctx, 2, req, sender); err != nil {
		t.Fatalf("second CMCreateAccount: %v", err)
	}
	msg, _ := sender.last()
	if msg.ID != protocol.SM_CREATEACCOUNTERROR {
		t.Fatalf("expected SM_CREATEACCOUNTERROR, got %v", msg.ID)
	}

	// 空账号被拒
	sender.reset()
	empty := protocol.CMCreateAccount{
		ID:       mustString[common.Cap64](t, ""),
		Password: mustString[common.Cap64](t, "x"),
	}
	if err := h.CMCreateAccount(ctx, 3, empty, sender); err != nil {
		t.Fatalf("empty CMCreateAccount: %v", err)
	}
	if msg, _ := sender.last(); msg.ID != protocol.SM_CREATEACCOUNTERROR {
		t.Fatalf("empty account should error, got %v", msg.ID)
	}
}

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	h, store := newTestHandler(t)
	accounts := storage.NewAccountRepository(store)
	if err := accounts.Create(ctx, "user", "old123"); err != nil {
		t.Fatalf("create account: %v", err)
	}

	// 密码错 -> 错误
	sender := &fakeSender{}
	bad := protocol.CMChangePassword{
		ID:          mustString[common.Cap64](t, "user"),
		Password:    mustString[common.Cap64](t, "wrong"),
		PasswordNew: mustString[common.Cap64](t, "new456"),
	}
	if err := h.CMChangePassword(ctx, 1, bad, sender); err != nil {
		t.Fatalf("CMChangePassword: %v", err)
	}
	if msg, _ := sender.last(); msg.ID != protocol.SM_CHANGEPASSWORDERROR {
		t.Fatalf("expected SM_CHANGEPASSWORDERROR, got %v", msg.ID)
	}

	// 正确旧密码 -> 成功，且新密码可登录
	sender.reset()
	good := protocol.CMChangePassword{
		ID:          mustString[common.Cap64](t, "user"),
		Password:    mustString[common.Cap64](t, "old123"),
		PasswordNew: mustString[common.Cap64](t, "new456"),
	}
	if err := h.CMChangePassword(ctx, 2, good, sender); err != nil {
		t.Fatalf("CMChangePassword good: %v", err)
	}
	if msg, _ := sender.last(); msg.ID != protocol.SM_CHANGEPASSWORDOK {
		t.Fatalf("expected SM_CHANGEPASSWORDOK, got %v", msg.ID)
	}
	ok, err := accounts.Authenticate(ctx, "user", "new456")
	if err != nil || !ok {
		t.Fatalf("new password should authenticate: ok=%v err=%v", ok, err)
	}
}

func TestCreateChar(t *testing.T) {
	ctx := context.Background()
	h, store := newTestHandler(t)
	accounts := storage.NewAccountRepository(store)
	if err := accounts.Create(ctx, "player", "pwd"); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := h.Sessions.Login(1, "player"); err != nil {
		t.Fatalf("session login: %v", err)
	}

	sender := &fakeSender{}
	req := protocol.CMCreateChar{
		Name:   mustString[common.Cap64](t, "新角色"),
		Job:    2,
		Gender: 0, // 女
	}
	if err := h.CMCreateChar(ctx, 1, 1, req, sender); err != nil {
		t.Fatalf("CMCreateChar: %v", err)
	}
	if msg, _ := sender.last(); msg.ID != protocol.SM_CREATECHAROK {
		t.Fatalf("expected SM_CREATECHAROK, got %v", msg.ID)
	}

	// 角色应能查到，且出生点是默认值
	chars := storage.NewCharacterRepository(store)
	ch, err := chars.GetByAccount(ctx, "player")
	if err != nil {
		t.Fatalf("get char: %v", err)
	}
	if ch.Name != "新角色" || ch.Job != 2 || ch.Gender != 0 {
		t.Fatalf("char = %+v", ch)
	}
	if ch.Map != DefaultSpawnMap || ch.MapX != DefaultSpawnX || ch.MapY != DefaultSpawnY {
		t.Fatalf("spawn = (%d,%d,%d)", ch.Map, ch.MapX, ch.MapY)
	}

	// 已有角色再建 -> 拒绝
	sender.reset()
	if err := h.CMCreateChar(ctx, 1, 2, req, sender); err != nil {
		t.Fatalf("second CMCreateChar: %v", err)
	}
	if msg, _ := sender.last(); msg.ID != protocol.SM_CREATECHARERROR {
		t.Fatalf("expected SM_CREATECHARERROR, got %v", msg.ID)
	}
}

func TestDeleteChar(t *testing.T) {
	ctx := context.Background()
	h, store := newTestHandler(t)
	accounts := storage.NewAccountRepository(store)
	if err := accounts.Create(ctx, "del", "pwd123"); err != nil {
		t.Fatalf("create account: %v", err)
	}
	chars := storage.NewCharacterRepository(store)
	if _, err := chars.Create(ctx, storage.CreateCharacterParams{
		Account: "del", Name: "待删", Gender: 1, Job: 1,
		Map: DefaultSpawnMap, MapX: 10, MapY: 10,
	}); err != nil {
		t.Fatalf("create char: %v", err)
	}
	if err := h.Sessions.Login(1, "del"); err != nil {
		t.Fatalf("session login: %v", err)
	}

	// 密码错 -> 拒绝
	sender := &fakeSender{}
	bad := protocol.CMDeleteChar{Password: mustString[common.Cap64](t, "wrong")}
	if err := h.CMDeleteChar(ctx, 1, 1, bad, sender); err != nil {
		t.Fatalf("CMDeleteChar: %v", err)
	}
	if msg, _ := sender.last(); msg.ID != protocol.SM_DELETECHARERROR {
		t.Fatalf("expected SM_DELETECHARERROR, got %v", msg.ID)
	}
	if _, err := chars.GetByName(ctx, "待删"); err != nil {
		t.Fatal("character should still exist after failed delete")
	}

	// 密码对 -> 删除成功
	sender.reset()
	good := protocol.CMDeleteChar{Password: mustString[common.Cap64](t, "pwd123")}
	if err := h.CMDeleteChar(ctx, 1, 2, good, sender); err != nil {
		t.Fatalf("CMDeleteChar good: %v", err)
	}
	if msg, _ := sender.last(); msg.ID != protocol.SM_DELETECHAROK {
		t.Fatalf("expected SM_DELETECHAROK, got %v", msg.ID)
	}
	if _, err := chars.GetByName(ctx, "待删"); err == nil {
		t.Fatal("character should be deleted")
	}
}

func TestLoginProtocolRoundTrip(t *testing.T) {
	// 确保 CMLogin 的 wire 解码能接受真实客户端发来的字节
	req := protocol.CMLogin{
		ID:       mustString[common.Cap64](t, "test@test.com"),
		Password: mustString[common.Cap64](t, "123456"),
	}
	payload, err := protocol.AppendWirePayload(nil, req)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var decoded protocol.CMLogin
	n, err := protocol.ReadWirePayload(&decoded, payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("consumed %d of %d", n, len(payload))
	}
	if decoded.ID.String() != "test@test.com" || decoded.Password.String() != "123456" {
		t.Fatalf("decoded = %+v", decoded)
	}
}
