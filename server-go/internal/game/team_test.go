package game

import (
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// mkTeams 造一个接在 fixture 上的组队管理器。
func mkTeams(f *b5spellFixture) *TeamManager {
	return NewTeamManager(f.Players, f.Cast, discardLogger())
}

// TestCreateTeam 建队后自己成为队长且只有自己一个成员。
func TestCreateTeam(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	tm := mkTeams(f)

	f.Cast.reset()
	if code := tm.Create(f.ctx, alice, protocol.SDTeamReq{MaxMember: 5}); code != TeamErrNone {
		t.Fatalf("建队失败 code=%d", code)
	}

	team := tm.TeamOf(alice.UID)
	if team == nil {
		t.Fatal("应该在队伍里")
	}
	if team.Leader != alice.UID {
		t.Fatalf("队长 = %#x, want %#x", team.Leader, alice.UID)
	}
	if len(team.Members) != 1 {
		t.Fatalf("成员数 = %d, want 1", len(team.Members))
	}
	if n := len(f.Cast.of(alice.ChannelID, protocol.SM_TEAMMEMBERLIST)); n != 1 {
		t.Fatalf("应该推队伍列表，实际 %d 条", n)
	}
}

// TestCreateTeamTwice 已在队伍里不能再建。
func TestCreateTeamTwice(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	tm := mkTeams(f)

	tm.Create(f.ctx, alice, protocol.SDTeamReq{MaxMember: 5})
	if code := tm.Create(f.ctx, alice, protocol.SDTeamReq{MaxMember: 5}); code != TeamErrInTeam {
		t.Fatalf("重复建队应返回 INTEAM，实际 %d", code)
	}
}

// TestJoinTeamFlow 申请 -> 批准 -> 入队。
func TestJoinTeamFlow(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	bob := f.mkPlayer(t, 2, "bob", 101, 100)
	tm := mkTeams(f)

	tm.Create(f.ctx, alice, protocol.SDTeamReq{MaxMember: 5})

	// bob 申请加入 alice 的队伍
	if code := tm.RequestJoin(f.ctx, bob, alice.UID); code != TeamErrNone {
		t.Fatalf("申请失败 code=%d", code)
	}
	team := tm.TeamOf(alice.UID)
	if _, ok := team.Candidates[bob.UID]; !ok {
		t.Fatal("bob 应该在候选集里")
	}
	if tm.TeamOf(bob.UID) != nil {
		t.Fatal("批准前 bob 不该在队伍里")
	}

	// alice（队长）批准 bob
	if code := tm.RequestJoin(f.ctx, alice, bob.UID); code != TeamErrNone {
		t.Fatalf("批准失败 code=%d", code)
	}
	if tm.TeamOf(bob.UID) == nil {
		t.Fatal("批准后 bob 应该在队伍里")
	}
	if len(tm.TeamOf(alice.UID).Members) != 2 {
		t.Fatalf("成员数 = %d, want 2", len(tm.TeamOf(alice.UID).Members))
	}
}

// TestJoinWithoutApplication 没申请过的人不能被批准。
func TestJoinWithoutApplication(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	carol := f.mkPlayer(t, 3, "carol", 102, 100)
	tm := mkTeams(f)

	tm.Create(f.ctx, alice, protocol.SDTeamReq{MaxMember: 5})
	if code := tm.RequestJoin(f.ctx, alice, carol.UID); code != TeamErrBadCandidate {
		t.Fatalf("未申请人应该 BADCANDIDATE，实际 %d", code)
	}
}

// TestTeamFull 人数上限生效。
func TestTeamFull(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	bob := f.mkPlayer(t, 2, "bob", 101, 100)
	carol := f.mkPlayer(t, 3, "carol", 102, 100)
	tm := mkTeams(f)

	tm.Create(f.ctx, alice, protocol.SDTeamReq{MaxMember: 2})
	tm.RequestJoin(f.ctx, bob, alice.UID)
	tm.RequestJoin(f.ctx, alice, bob.UID)

	// 现在 2/2，carol 申请应该满
	if code := tm.RequestJoin(f.ctx, carol, alice.UID); code != TeamErrFull {
		t.Fatalf("满员应该 FULL，实际 %d", code)
	}
}

// TestLeaderDisband 队长离队 = 解散。
func TestLeaderDisband(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	bob := f.mkPlayer(t, 2, "bob", 101, 100)
	tm := mkTeams(f)

	tm.Create(f.ctx, alice, protocol.SDTeamReq{MaxMember: 5})
	tm.RequestJoin(f.ctx, bob, alice.UID)
	tm.RequestJoin(f.ctx, alice, bob.UID)

	if code := tm.Leave(f.ctx, alice, alice.UID); code != TeamErrNone {
		t.Fatalf("解散失败 code=%d", code)
	}
	if tm.TeamOf(alice.UID) != nil {
		t.Fatal("解散后队长不该在队伍里")
	}
	if tm.TeamOf(bob.UID) != nil {
		t.Fatal("解散后队员不该在队伍里")
	}
}

// TestTransferLeader 移交队长后原队长变队员。
func TestTransferLeader(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	bob := f.mkPlayer(t, 2, "bob", 101, 100)
	tm := mkTeams(f)

	tm.Create(f.ctx, alice, protocol.SDTeamReq{MaxMember: 5})
	tm.RequestJoin(f.ctx, bob, alice.UID)
	tm.RequestJoin(f.ctx, alice, bob.UID)

	if code := tm.TransferLeader(f.ctx, alice, bob.UID); code != TeamErrNone {
		t.Fatalf("移交失败 code=%d", code)
	}
	team := tm.TeamOf(bob.UID)
	if team.Leader != bob.UID {
		t.Fatalf("队长 = %#x, want bob %#x", team.Leader, bob.UID)
	}
	if len(team.Members) != 2 {
		t.Fatalf("移交不该改变成员数，实际 %d", len(team.Members))
	}
}

// TestLeaveGameCleansTeam 下线要清理队伍关系。
func TestLeaveGameCleansTeam(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	bob := f.mkPlayer(t, 2, "bob", 101, 100)
	tm := mkTeams(f)

	tm.Create(f.ctx, alice, protocol.SDTeamReq{MaxMember: 5})
	tm.RequestJoin(f.ctx, bob, alice.UID)
	tm.RequestJoin(f.ctx, alice, bob.UID)

	// bob 下线
	tm.OnLeaveGame(f.ctx, bob.UID)
	if tm.TeamOf(bob.UID) != nil {
		t.Fatal("下线后不该还在队伍里")
	}
	team := tm.TeamOf(alice.UID)
	if team == nil {
		t.Fatal("队伍应该还在")
	}
	if len(team.Members) != 1 {
		t.Fatalf("下线后成员数应回到 1，实际 %d", len(team.Members))
	}
}
