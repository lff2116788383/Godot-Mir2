package storage

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestGuildLifecycleAndWar(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, dbids := openCharacters(t, "leader", "member", "enemy")
	repo := NewGuildRepository(store)

	guildID, code, err := repo.Create(ctx, dbids[0], "alpha")
	if err != nil || code != GuildErrNone || guildID == 0 {
		t.Fatalf("create guild = %d, %d, %v", guildID, code, err)
	}
	membership, ok, err := repo.LoadMembership(ctx, dbids[0])
	if err != nil || !ok || membership.GuildID != guildID || membership.Rank != GuildRankLeader {
		t.Fatalf("membership = %+v, %v, %v", membership, ok, err)
	}

	if code, err = repo.Invite(ctx, dbids[0], dbids[1]); err != nil || code != GuildErrNone {
		t.Fatalf("invite = %d, %v", code, err)
	}
	if code, err = repo.AcceptInvite(ctx, dbids[1], guildID); err != nil || code != GuildErrNone {
		t.Fatalf("accept = %d, %v", code, err)
	}
	info, err := repo.LoadInfo(ctx, guildID)
	if err != nil || info.Name != "alpha" || len(info.MemberList) != 2 {
		t.Fatalf("guild info = %+v, %v", info, err)
	}
	if code, err = repo.SetNotice(ctx, dbids[0], "welcome"); err != nil || code != GuildErrNone {
		t.Fatalf("set notice = %d, %v", code, err)
	}
	if code, err = repo.SetMemberTitle(ctx, dbids[0], dbids[1], "officer"); err != nil || code != GuildErrNone {
		t.Fatalf("set title = %d, %v", code, err)
	}

	enemyID, code, err := repo.Create(ctx, dbids[2], "beta")
	if err != nil || code != GuildErrNone || enemyID == 0 {
		t.Fatalf("create enemy = %d, %d, %v", enemyID, code, err)
	}
	warEnemy, code, err := repo.DeclareWar(ctx, dbids[0], "beta", time.Now().Unix())
	if err != nil || code != GuildErrNone || warEnemy != enemyID {
		t.Fatalf("declare war = %d, %d, %v", warEnemy, code, err)
	}
	atWar, err := repo.AtWar(ctx, guildID, enemyID, time.Now().Unix())
	if err != nil || !atWar {
		t.Fatalf("at war = %v, %v", atWar, err)
	}
	wars, err := repo.LoadWarList(ctx, guildID, time.Now().Unix())
	if err != nil || len(wars) != 1 || wars[0].GuildID != enemyID {
		t.Fatalf("wars = %+v, %v", wars, err)
	}

	if code, err = repo.Kick(ctx, dbids[0], dbids[1]); err != nil || code != GuildErrNone {
		t.Fatalf("kick = %d, %v", code, err)
	}
	if code, err = repo.Invite(ctx, dbids[0], dbids[1]); err != nil || code != GuildErrNone {
		t.Fatalf("reinvite = %d, %v", code, err)
	}
	if code, err = repo.AcceptInvite(ctx, dbids[1], guildID); err != nil || code != GuildErrNone {
		t.Fatalf("reaccept = %d, %v", code, err)
	}
	if code, err = repo.TransferLeader(ctx, dbids[0], dbids[1]); err != nil || code != GuildErrNone {
		t.Fatalf("transfer = %d, %v", code, err)
	}
	newMembership, ok, err := repo.LoadMembership(ctx, dbids[1])
	if err != nil || !ok || newMembership.Rank != GuildRankLeader {
		t.Fatalf("new leader = %+v, %v, %v", newMembership, ok, err)
	}

	others, code, err := repo.Disband(ctx, dbids[1])
	if err != nil || code != GuildErrNone || !reflect.DeepEqual(others, []uint32{dbids[0]}) {
		t.Fatalf("disband = %v, %d, %v", others, code, err)
	}
	if _, ok, err := repo.LoadMembership(ctx, dbids[1]); err != nil || ok {
		t.Fatalf("guild after disband = %v, %v", ok, err)
	}
}

func TestGuildValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		ok    bool
	}{
		{"valid", "alpha", true},
		{"empty", "", false},
		{"space", "a b", false},
		{"xml", "a<b", false},
		{"tooLong", "1234567890123456789012345678901", true},
		{"tooLong2", "12345678901234567890123456789012", false},
	}
	for _, testCase := range cases {
		if got := GuildNameValid(testCase.value); got != testCase.ok {
			t.Fatalf("%s GuildNameValid(%q) = %v", testCase.name, testCase.value, got)
		}
	}
	if !GuildNoticeValid("line1\nline2") {
		t.Fatal("newline should be valid in guild notice")
	}
	if GuildNoticeValid("bad\x00notice") {
		t.Fatal("NUL should be invalid in guild notice")
	}
}
