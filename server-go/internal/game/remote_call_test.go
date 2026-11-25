package game

import (
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
)

// TestRemoteCallMatchesRealScriptCodes 是本轮最要紧的一项验证：
//
//	uidRemoteCall 的代码串**来自原版脚本原文**，我们的匹配器必须认得出。
//
// 这里的每条 code 都是从 server/script/api/player.lua 里**逐字抄出来**的
// （包括 `getGold ()` 这种名字与括号之间带空格的写法）。
// 有一条认不出，对应的 NPC 功能就会静默失败。
func TestRemoteCallMatchesRealScriptCodes(t *testing.T) {
	// 直接对匹配表做验证，不需要真的 Player / DB。
	// 只检查「能否识别」，不检查返回值正确性（那是别的测试）。
	cases := []struct {
		code string
		want string // 期望命中的分支名
	}{
		// ---- 无参查询 ----
		{" return getGold () ", "getGold"},
		{" return getLevel() ", "getLevel"},
		{" return getName () ", "getName"},
		{" return getGender() ", "getGender"},
		{" return getGuildName() ", "getGuildName"},
		{" return isGuildLeader() ", "isGuildLeader"},
		{" return getJobList() ", "getJobList"},
		{`
		return getRedName()
		`, "getRedName"},

		// ---- 带参 ----
		{" local count = ... return removeGold(count) ", "removeGold"},
		{" local n = ... return hasMagic(n) ", "hasMagic"},
		{" local item, count = ... return addItem(item, count) ", "addItem"},
		{" local item, seq, count = ... return removeItem(item, seq, count) ", "removeItem"},
		{" local item, seq, count = ... return hasItem(item, seq, count) ", "hasItem"},
		{" local exp = ... return addExp(exp) ", "addExp"},

		// ---- 公会（P2 关键路径，没有任何 CM_ 协议可替代）----
		{" return createGuild(...) ", "createGuild"},
		{" return declareGuildWar(...) ", "declareGuildWar"},

		// ---- 装备 ----
		{" return addBoundItem(...) ", "addBoundItem"},
		{" return removeWearItem(...) ", "removeWearItem"},
	}

	for _, tc := range cases {
		norm := normalizeRemoteCode(tc.code)
		matched := ""
		for _, p := range remoteCallPatterns {
			if p.re.MatchString(norm) {
				matched = p.name
				break
			}
		}
		if matched == "" {
			t.Errorf("代码串认不出来（脚本这项功能会失效）:\n  原文: %q\n  规整: %q", tc.code, norm)
			continue
		}
		if matched != tc.want {
			t.Errorf("代码串 %q 错误命中 %s，期望 %s", tc.code, matched, tc.want)
		}
	}
}

// TestRemoteCallUnknownCodeReportsError 未识别的代码串必须报错而不是返回 nil。
//
// ★ 静默返回 nil 的后果：脚本的 `if uidRemoteCall(...) then` 走 else，
// 玩家看到的是「操作失败」而不是「引擎不支持」—— 完全无法排查。
func TestRemoteCallUnknownCodeReportsError(t *testing.T) {
	h := &npcHostAdapter{}
	_, err := h.executeRemoteCall(playerUIDOf(1), "return somethingUnknown()", nil)
	if err == nil {
		t.Fatal("未识别的代码串必须返回错误，不能静默返回 nil")
	}
}

// TestRemoteCallRejectsNPC 目标不是玩家时必须报错。
func TestRemoteCallRejectsNPC(t *testing.T) {
	h := &npcHostAdapter{}
	// NPC UID 的高位是 NPC 类型，不是玩家
	npcUID := uint64(3)<<59 | 12345
	if _, err := h.executeRemoteCall(npcUID, "return getGold()", nil); err == nil {
		t.Fatal("对 NPC 的 remoteCall 必须返回错误（当前不支持）")
	}
}

// TestNormalizeRemoteCodeRemovesAllWhitespace 验证规整会删掉**所有**空白。
//
// ★ 这是「getGold () 被漏掉」那类 bug 的根因：
//
//	早期实现只把连续空白压成一个空格，规整结果是 "return getGold ()"，
//	而正则写的是 `^return getGold\(\)$` —— 名字与括号之间的空格还在，
//	匹配失败。整表只有 getGold / getName / getGuildName 这几条受影响，
//	表现是「NPC 报『请确认一下自己所带的金币』」——
//	看起来像玩家真的没钱，实际是代码串没被识别。
func TestNormalizeRemoteCodeRemovesAllWhitespace(t *testing.T) {
	cases := map[string]string{
		" return getGold () ":                       "returngetGold()",
		"return\tgetGold\n(\n)":                     "returngetGold()",
		" local item, count = ...\nreturn addItem(item, count) ": "localitem,count=...returnaddItem(item,count)",
	}
	for input, want := range cases {
		if got := normalizeRemoteCode(input); got != want {
			t.Errorf("normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestRemoteCallDeclareWarEndToEnd 走一次真实的宣战 remoteCall。
//
// 这是 P2 的最终验证：NPC 脚本说「return declareGuildWar(...)」，
// 经过 uidRemoteCall → executeRemoteCall → GuildWarManager.DeclareWar，
// 真的建起一场战争并扣掉金币。
func TestRemoteCallDeclareWarEndToEnd(t *testing.T) {
	f := newP2Fixture(t)
	leaderDBID := f.mkChar(t, "rc000001", "会长RC")
	enemyDBID := f.mkChar(t, "rc000002", "会长RD")
	if _, code, err := f.Guilds.Create(f.ctx, leaderDBID, "RC会"); err != nil || code != storage.GuildErrNone {
		t.Fatalf("create guild A: %v code=%d", err, code)
	}
	if _, code, err := f.Guilds.Create(f.ctx, enemyDBID, "RD会"); err != nil || code != storage.GuildErrNone {
		t.Fatalf("create guild B: %v code=%d", err, code)
	}
	_ = f.Golds.Set(f.ctx, leaderDBID, 50000)

	leader := &Player{
		ChannelID: 51, DBID: leaderDBID, UID: playerUIDOf(leaderDBID),
		Name: "会长RC", Gold: 50000, Job: 1,
	}
	players := NewPlayerManager()
	players.Add(leader)
	broadcast := &captureBroadcaster{}

	wars := NewGuildWarManager(players, broadcast, f.Guilds, f.Golds)
	h := &npcHostAdapter{
		Players: players, Broadcast: broadcast, Logger: discardLogger(),
		Items: f.Items, Guilds: f.Guilds, GuildWars: wars, Golds: f.Golds,
	}

	// 完全照 NPC 脚本的写法（player.lua:66 的模板）
	results, err := h.RemoteCall(leader.UID, " return declareGuildWar(...) ",
		[]script.RemoteArg{script.RemoteString("RD会"), script.RemoteNumber(30000)})
	if err != nil {
		t.Fatalf("remoteCall: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("期望返回 1 个值（错误码），实际 %d", len(results))
	}
	if code := results[0].AsNumber(); code != 0 {
		t.Fatalf("宣战应该成功（返回 GUILDERR_NONE=0），实际 %v", code)
	}

	// 金币被扣
	if leader.Gold != 20000 {
		t.Fatalf("宣战后金币期望 20000，实际 %d", leader.Gold)
	}
	// 战争真的建起来了
	if !wars.AtWar(f.ctx, leaderDBID, enemyDBID) {
		t.Fatal("宣战后 AtWar 必须为 true")
	}
	// 客户端收到战列表
	if !broadcast.has(protocol.SM_GUILDWARLIST) {
		t.Fatal("宣战后必须推 SM_GUILDWARLIST")
	}
}

// TestRemoteCallIsGuildLeader 验证会长判定。
func TestRemoteCallIsGuildLeader(t *testing.T) {
	f := newP2Fixture(t)
	leaderDBID := f.mkChar(t, "rc000003", "会长RE")
	memberDBID := f.mkChar(t, "rc000004", "会员RF")
	if _, code, err := f.Guilds.Create(f.ctx, leaderDBID, "RE会"); err != nil || code != storage.GuildErrNone {
		t.Fatalf("create guild: %v code=%d", err, code)
	}
	if code, err := f.Guilds.Invite(f.ctx, leaderDBID, memberDBID); err != nil || code != storage.GuildErrNone {
		t.Fatalf("invite: %v code=%d", err, code)
	}
	if code, err := f.Guilds.AcceptInvite(f.ctx, memberDBID, mustGuildID(t, f, leaderDBID)); err != nil || code != storage.GuildErrNone {
		t.Fatalf("accept: %v code=%d", err, code)
	}

	players := NewPlayerManager()
	leader := &Player{ChannelID: 52, DBID: leaderDBID, UID: playerUIDOf(leaderDBID), Name: "会长RE"}
	member := &Player{ChannelID: 53, DBID: memberDBID, UID: playerUIDOf(memberDBID), Name: "会员RF"}
	players.Add(leader)
	players.Add(member)
	h := &npcHostAdapter{
		Players: players, Broadcast: &captureBroadcaster{}, Logger: discardLogger(),
		Guilds: f.Guilds,
	}

	// 会长
	res, err := h.RemoteCall(leader.UID, " return isGuildLeader() ", nil)
	if err != nil {
		t.Fatalf("leader remoteCall: %v", err)
	}
	if len(res) != 1 || res[0].Kind != script.RemoteArgBool || !res[0].Bool {
		t.Fatalf("会长应该返回 true，实际 %+v", res)
	}

	// 普通会员
	res, err = h.RemoteCall(member.UID, " return isGuildLeader() ", nil)
	if err != nil {
		t.Fatalf("member remoteCall: %v", err)
	}
	if len(res) != 1 || res[0].Kind != script.RemoteArgBool || res[0].Bool {
		t.Fatalf("普通会员应该返回 false，实际 %+v", res)
	}
}

// TestRemoteCallGetGuildName 未入会时返回 nil。
func TestRemoteCallGetGuildName(t *testing.T) {
	f := newP2Fixture(t)
	dbid := f.mkChar(t, "rc000005", "散人RG")
	players := NewPlayerManager()
	p := &Player{ChannelID: 54, DBID: dbid, UID: playerUIDOf(dbid), Name: "散人RG"}
	players.Add(p)
	h := &npcHostAdapter{
		Players: players, Broadcast: &captureBroadcaster{}, Logger: discardLogger(),
		Guilds: f.Guilds,
	}
	res, err := h.RemoteCall(p.UID, " return getGuildName() ", nil)
	if err != nil {
		t.Fatalf("remoteCall: %v", err)
	}
	if len(res) != 1 || res[0].Kind != script.RemoteArgNil {
		t.Fatalf("未入会应该返回 nil，实际 %+v", res)
	}
}
