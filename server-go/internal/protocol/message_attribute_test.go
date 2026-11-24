package protocol

import "testing"

func TestGeneratedMessageAttributes(t *testing.T) {
	t.Parallel()

	checks := []struct {
		name string
		attr MessageAttribute
		ok   bool
		kind MessageKind
		size int
	}{
		{"CM_PING", mustClient(t, CM_PING), true, MessageFixedRaw, 4},
		{"CM_LOGIN", mustClient(t, CM_LOGIN), true, MessageFixedCompressed, 136},
		{"CM_CHATMESSAGE", mustClient(t, CM_CHATMESSAGE), true, MessageVariable, 0},
		{"CM_CREATECHATGROUP", mustClient(t, CM_CREATECHATGROUP), true, MessageFixedCompressed, 2184},
		{"SM_ERROR", mustServer(t, SM_ERROR), true, MessageEmpty, 0},
		{"SM_ONLINEOK", mustServer(t, SM_ONLINEOK), true, MessageFixedCompressed, 177},
		{"SM_GUILDCHAT", mustServer(t, SM_GUILDCHAT), true, MessageFixedCompressed, 204},
		{"SM_HEALTH", mustServer(t, SM_HEALTH), true, MessageVariable, 0},
	}
	for _, check := range checks {
		if check.attr.Name != check.name {
			t.Fatalf("%s name = %q", check.name, check.attr.Name)
		}
		if check.attr.Kind != check.kind || check.attr.DataLen != check.size {
			t.Fatalf("%s = kind %d size %d, want kind %d size %d", check.name, check.attr.Kind, check.attr.DataLen, check.kind, check.size)
		}
	}

	if _, ok := ClientMessageAttribute(CM_END); ok {
		t.Fatal("CM_END must not be registered as a packet")
	}
	if _, ok := ServerMessageAttribute(SM_END); ok {
		t.Fatal("SM_END must not be registered as a packet")
	}
}

func mustClient(t *testing.T, id CMType) MessageAttribute {
	t.Helper()
	attr, ok := ClientMessageAttribute(id)
	if !ok {
		t.Fatalf("missing client message %d", id)
	}
	return attr
}

func mustServer(t *testing.T, id SMType) MessageAttribute {
	t.Helper()
	attr, ok := ServerMessageAttribute(id)
	if !ok {
		t.Fatalf("missing server message %d", id)
	}
	return attr
}
