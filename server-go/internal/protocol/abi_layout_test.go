package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
)

type cppLayoutFixture struct {
	StructSizes  map[string]int    `json:"struct_sizes"`
	HelperSizes  map[string]int    `json:"helper_sizes"`
	ActionOffset map[string]int    `json:"action_offsets"`
	Bitfields    map[string]string `json:"bitfield_probes"`
}

func loadCPPLayout(t *testing.T) cppLayoutFixture {
	t.Helper()
	path := filepath.Join("testdata", "cpp-layout.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var layout cppLayoutFixture
	if err := json.Unmarshal(data, &layout); err != nil {
		t.Fatal(err)
	}
	return layout
}

func TestCPPLayoutFixture(t *testing.T) {
	t.Parallel()

	layout := loadCPPLayout(t)
	if got := layout.StructSizes["ActionNode"]; got != common.ActionNodeWireSize {
		t.Fatalf("C++ ActionNode size = %d, Go = %d", got, common.ActionNodeWireSize)
	}
	if layout.ActionOffset["x"] != 4 || layout.ActionOffset["aimUID"] != 12 || layout.ActionOffset["extParam"] != 20 {
		t.Fatalf("unexpected ActionNode offsets: %+v", layout.ActionOffset)
	}
	if layout.HelperSizes["StaticBuffer64"] != 68 || layout.HelperSizes["StaticBuffer128"] != 132 {
		t.Fatalf("unexpected static helper sizes: %+v", layout.HelperSizes)
	}
	if layout.Bitfields["CMChatMessageHeader.hasRef"] != "00000000000000000100000000000000" {
		t.Fatalf("unexpected chat header bitfield layout: %s", layout.Bitfields["CMChatMessageHeader.hasRef"])
	}
}

func TestCPPLayoutMatchesMessageTable(t *testing.T) {
	t.Parallel()

	layout := loadCPPLayout(t)
	cases := []struct {
		cppType string
		attr    MessageAttribute
	}{
		{"CMPing", mustClient(t, CM_PING)},
		{"CMLogin", mustClient(t, CM_LOGIN)},
		{"CMAction", mustClient(t, CM_ACTION)},
		{"CMRequestLatestChatMessage", mustClient(t, CM_REQUESTLATESTCHATMESSAGE)},
		{"SMAction", mustServer(t, SM_ACTION)},
		{"SMOnlineOK", mustServer(t, SM_ONLINEOK)},
		{"SMCORecord", mustServer(t, SM_CORECORD)},
		{"SMGuildChat", mustServer(t, SM_GUILDCHAT)},
	}
	for _, tc := range cases {
		if got := layout.StructSizes[tc.cppType]; got != tc.attr.DataLen {
			t.Fatalf("%s: C++ size %d, message table %d", tc.cppType, got, tc.attr.DataLen)
		}
	}
}
