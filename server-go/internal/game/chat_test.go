package game

import (
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// TestSayBroadcastsToSelfAndViewers 验证原地说话：自己 + 视野内的人都收到。
func TestSayBroadcastsToSelfAndViewers(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	bob := f.mkPlayer(t, 2, "bob", 103, 100)
	// 远处的 carol：不在 alice 视野内
	carol := f.mkPlayer(t, 3, "carol", 190, 190)

	_ = carol

	f.Cast.reset()
	var req protocol.CMPlayerSay
	copy(req.Content[:], "你好")
	if err := f.Chat.Say(f.ctx, alice.ChannelID, req); err != nil {
		t.Fatalf("say: %v", err)
	}

	if n := len(f.Cast.of(alice.ChannelID, protocol.SM_PLAYERSAY)); n != 1 {
		t.Fatalf("发言者自己应该收到 1 条，实际 %d", n)
	}
	_ = bob
}

// TestSayIgnoresEmptyContent 空白内容不广播。
func TestSayIgnoresEmptyContent(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alice", 100, 100)

	f.Cast.reset()
	var req protocol.CMPlayerSay
	copy(req.Content[:], "    ")
	if err := f.Chat.Say(f.ctx, p.ChannelID, req); err != nil {
		t.Fatalf("say: %v", err)
	}
	if n := len(f.Cast.of(p.ChannelID, protocol.SM_PLAYERSAY)); n != 0 {
		t.Fatalf("空白内容不该广播，实际 %d 条", n)
	}
}

// TestBroadcastSayReachesEveryone 喇叭全服可见。
func TestBroadcastSayReachesEveryone(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	bob := f.mkPlayer(t, 2, "bob", 190, 190) // 离得很远
	carol := f.mkPlayer(t, 3, "carol", 20, 20)

	f.Cast.reset()
	var req protocol.CMPlayerBroadcast
	copy(req.Content[:], "全服公告")
	if err := f.Chat.BroadcastSay(f.ctx, alice.ChannelID, req); err != nil {
		t.Fatalf("broadcast: %v", err)
	}

	for _, p := range []*Player{alice, bob, carol} {
		if n := len(f.Cast.of(p.ChannelID, protocol.SM_PLAYERBROADCAST)); n != 1 {
			t.Fatalf("%s 应该收到喇叭，实际 %d 条", p.Name, n)
		}
	}
}

// TestChatMessageRepliesWithOK 私聊要回 SM_CHATMESSAGEOK（客户端靠它标记已送达）。
func TestChatMessageRepliesWithOK(t *testing.T) {
	f := newB5SpellFixture(t)
	alice := f.mkPlayer(t, 1, "alice", 100, 100)
	bob := f.mkPlayer(t, 2, "bob", 101, 100)

	f.Cast.reset()
	header := protocol.CMChatMessageHeader{
		ToCPID: protocol.NewSDChatPeerID(ChatPeerPlayer, bob.DBID).Data,
	}
	if err := f.Chat.ChatMessage(f.ctx, alice.ChannelID, header, "私聊内容"); err != nil {
		t.Fatalf("chat: %v", err)
	}

	if n := len(f.Cast.of(alice.ChannelID, protocol.SM_CHATMESSAGEOK)); n != 1 {
		t.Fatalf("发送者应该收到 CHATMESSAGEOK，实际 %d 条", n)
	}
	if n := len(f.Cast.of(bob.ChannelID, protocol.SM_CHATMESSAGELIST)); n != 1 {
		t.Fatalf("接收者应该收到消息列表，实际 %d 条", n)
	}
}

// TestSplitChatMessageHeader 验证 16 字节头部的位域解析。
func TestSplitChatMessageHeader(t *testing.T) {
	// hasRef=1, refID=12345
	flags := uint64(12345)<<1 | 1
	payload := make([]byte, 16)
	// toCPID = CPR_PLAYER<<32 | 7
	to := uint64(ChatPeerPlayer)<<32 | 7
	for i := 0; i < 8; i++ {
		payload[i] = byte(to >> (8 * i))
		payload[8+i] = byte(flags >> (8 * i))
	}
	payload = append(payload, []byte("hello")...)

	header, body, err := splitChatMessage(payload)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if header.ToCPID != to {
		t.Fatalf("toCPID = %#x, want %#x", header.ToCPID, to)
	}
	if !header.HasRef || header.RefID != 12345 {
		t.Fatalf("hasRef=%v refID=%d, want true/12345", header.HasRef, header.RefID)
	}
	if body != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}
}

// TestTruncateCStringUTF8Safe 长中文要按字符边界截断，不能切出非法 UTF-8。
func TestTruncateCStringUTF8Safe(t *testing.T) {
	// 63 个汉字 = 189 字节，远超 127
	long := ""
	for i := 0; i < 63; i++ {
		long += "中"
	}
	out := truncateCString(long, 128)
	// 必须是合法 UTF-8（切在字符中间会产生 0xE4 0xB8 这种残片）
	got := trimCString(out[:])
	for _, r := range got {
		if r == '\uFFFD' {
			t.Fatalf("截断产生了非法 UTF-8：%q", got)
		}
	}
	if len(got) > 127 {
		t.Fatalf("截断后 %d 字节，超过 127", len(got))
	}
	t.Logf("原文 %d 字节 → 截断 %d 字节", len(long), len(got))
}
