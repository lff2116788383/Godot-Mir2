package game

import "testing"

func TestSessionLoginBasic(t *testing.T) {
	m := NewSessionManager()

	if err := m.Login(1, "alice"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	s, err := m.Get(1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if s.Account != "alice" {
		t.Fatalf("account = %q, want alice", s.Account)
	}
	if s.DBID != 0 {
		t.Fatalf("dbid should be 0 before online, got %d", s.DBID)
	}
	if m.Count() != 1 {
		t.Fatalf("count = %d, want 1", m.Count())
	}
}

func TestSessionMultiLoginRejected(t *testing.T) {
	m := NewSessionManager()
	if err := m.Login(1, "alice"); err != nil {
		t.Fatalf("Login 1: %v", err)
	}
	// 同账号从另一连接登录必须被拒（原版 LOGINERR_MULTILOGIN）
	if err := m.Login(2, "alice"); err != ErrMultiLogin {
		t.Fatalf("second login err = %v, want ErrMultiLogin", err)
	}

	// 同一连接重复登录同一账号是允许的（幂等）
	if err := m.Login(1, "alice"); err != nil {
		t.Fatalf("same-channel relogin should succeed: %v", err)
	}

	// 不同账号可以并存
	if err := m.Login(2, "bob"); err != nil {
		t.Fatalf("different account should succeed: %v", err)
	}
	if m.Count() != 2 {
		t.Fatalf("count = %d, want 2", m.Count())
	}
}

func TestSessionMarkOnline(t *testing.T) {
	m := NewSessionManager()

	// 未登录就 online
	if err := m.MarkOnline(9, 100); err != ErrSessionNotFound {
		t.Fatalf("online without login err = %v, want ErrSessionNotFound", err)
	}

	if err := m.Login(1, "alice"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := m.MarkOnline(1, 100); err != nil {
		t.Fatalf("MarkOnline: %v", err)
	}
	s, _ := m.Get(1)
	if s.DBID != 100 {
		t.Fatalf("dbid = %d, want 100", s.DBID)
	}

	// 再次 online 同会话必须报 MULTIONLINE
	if err := m.MarkOnline(1, 100); err != ErrMultiOnline {
		t.Fatalf("double online err = %v, want ErrMultiOnline", err)
	}

	// IsCharacterOnline 应能查到
	if !m.IsCharacterOnline(100) {
		t.Fatal("character 100 should be online")
	}
	if m.IsCharacterOnline(999) {
		t.Fatal("character 999 should not be online")
	}
}

func TestSessionDropCleansIndex(t *testing.T) {
	m := NewSessionManager()
	if err := m.Login(1, "alice"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	_ = m.MarkOnline(1, 100)

	m.Drop(1)
	if m.Count() != 0 {
		t.Fatalf("count after drop = %d, want 0", m.Count())
	}
	if m.IsCharacterOnline(100) {
		t.Fatal("dropping session should clear online state")
	}
	// 掉线后同账号可以重新登录（accountIndex 已清理）
	if err := m.Login(2, "alice"); err != nil {
		t.Fatalf("relogin after drop should succeed: %v", err)
	}

	// 重复 Drop 不应 panic
	m.Drop(1)
	// Drop 不存在的会话
	m.Drop(999)
}

func TestSessionDropDoesNotStealIndex(t *testing.T) {
	// 场景：alice 从 conn1 掉线又重新从 conn2 登录，
	// 此时 Drop(conn1) 不能把 conn2 的索引删掉
	m := NewSessionManager()
	if err := m.Login(1, "alice"); err != nil {
		t.Fatalf("Login 1: %v", err)
	}
	m.Drop(1)
	if err := m.Login(2, "alice"); err != nil {
		t.Fatalf("Login 2: %v", err)
	}
	// 手工塞一个残留 conn1 场景：模拟乱序 Drop
	m.sessions[1] = &Session{ChannelID: 1, Account: "alice"}
	m.Drop(1)

	// conn2 的索引必须还在
	if err := m.Login(3, "alice"); err != ErrMultiLogin {
		t.Fatalf("conn2 should still own alice, err = %v", err)
	}
}

func TestSessionGetMissing(t *testing.T) {
	m := NewSessionManager()
	if _, err := m.Get(42); err != ErrSessionNotFound {
		t.Fatalf("Get missing err = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionReset(t *testing.T) {
	m := NewSessionManager()
	_ = m.Login(1, "a")
	_ = m.Login(2, "b")
	m.Reset()
	if m.Count() != 0 {
		t.Fatalf("count after reset = %d, want 0", m.Count())
	}
	if err := m.Login(3, "a"); err != nil {
		t.Fatalf("login after reset should succeed: %v", err)
	}
}
