package common

import "testing"

// TestGetBaseMapUIDMatchesClient 锁定 mapUID 的编码布局。
//
// 位布局（两边必须一致，客户端 scripts/core/uid.gd:35-37）：
//
//	type @59, id @35, peer @31, seq @0
//
// ★ 这里有个容易误解的点：客户端注释里写 `(2<<59)|(1<<35)|(1<<31)`
// 里的 `1<<31` 是 **peer 段**（peerIndex=1），不是 seq。
// 原版 getBaseMapUID 的 seq=1 落在**第 0 位**（seqBitOff=0）。
func TestGetBaseMapUIDMatchesClient(t *testing.T) {
	got, err := GetBaseMapUID(1, 0)
	if err != nil {
		t.Fatalf("GetBaseMapUID: %v", err)
	}
	// type=UID_MAP(2) @59, id=1 @35, peer=0 @31, seq=1 @0
	want := uint64(2)<<59 | uint64(1)<<35 | uint64(1)
	if got != want {
		t.Fatalf("base map uid = %#x, want %#x", got, want)
	}
	if got != 0x1000000800000001 {
		t.Fatalf("base map uid = %#x, want 0x1000000800000001", got)
	}

	// 反解必须回到原 mapID
	mapID, err := GetMapID(got)
	if err != nil {
		t.Fatalf("GetMapID: %v", err)
	}
	if mapID != 1 {
		t.Fatalf("mapID = %d, want 1", mapID)
	}
	if UIDSeq(got) != 1 {
		t.Fatalf("base seq = %d, want 1", UIDSeq(got))
	}
	if UIDPeerIndex(got) != 0 {
		t.Fatalf("base peer = %d, want 0", UIDPeerIndex(got))
	}

	// 客户端 main.gd:73 用的是 build_map_uid(1,1)，即 peer=1、seq=0
	// 两者区别仅在 peer/seq 段，type 与 id 完全相同 —— 服务端回包要和客户端一致，
	// 所以 peer 必须由调用方按实际网关序号传入。
	clientStyle := uint64(2)<<59 | uint64(1)<<35 | uint64(1)<<31
	if GetUIDType(clientStyle) != UIDMap {
		t.Fatal("client-style map uid should still be MAP type")
	}
	if UIDID(clientStyle) != 1 {
		t.Fatalf("client-style id = %d, want 1", UIDID(clientStyle))
	}
	if UIDPeerIndex(clientStyle) != 1 {
		t.Fatalf("client-style peer = %d, want 1", UIDPeerIndex(clientStyle))
	}
	// 客户端式的 seq 是 0
	if UIDSeq(clientStyle) != 0 {
		t.Fatalf("client-style seq = %d, want 0", UIDSeq(clientStyle))
	}

	// base 与普通 map uid 的 seq 段不同
	normal, err := BuildMapUID(1, 0)
	if err != nil {
		t.Fatalf("BuildMapUID: %v", err)
	}
	if UIDSeq(normal) != 2 {
		t.Fatalf("BuildMapUID seq = %d, want 2", UIDSeq(normal))
	}
	if normal == got {
		t.Fatal("base map uid and normal map uid must differ")
	}
}

func TestPlayerUIDRoundTrip(t *testing.T) {
	const dbid = 12345
	uid := GetPlayerUID(dbid)
	if !IsPlayerUID(uid) {
		t.Fatalf("uid %#x should be a player uid", uid)
	}
	back, err := GetPlayerDBID(uid)
	if err != nil {
		t.Fatalf("GetPlayerDBID: %v", err)
	}
	if back != dbid {
		t.Fatalf("dbid round trip = %d, want %d", back, dbid)
	}
	// 玩家 UID 只用 type 与 id 两段
	if GetUIDType(uid) != UIDPly {
		t.Fatalf("type = %v, want PLY", GetUIDType(uid))
	}
	// 玩家 UID 的 dbid 在**最低位**，不按 id 段偏移（原版特例）
	if uid != uint64(UIDPly)<<59|uint64(dbid) {
		t.Fatalf("player uid composition wrong: %#x", uid)
	}
	if uid != 0x2800000000003039 {
		t.Fatalf("player uid = %#x, want 0x2800000000003039", uid)
	}
}

func TestMonsterAndNPCUID(t *testing.T) {
	muid, err := BuildMonsterUID(100, 3, 7)
	if err != nil {
		t.Fatalf("BuildMonsterUID: %v", err)
	}
	if !IsMonsterUID(muid) {
		t.Fatal("should be monster uid")
	}
	mid, err := GetMonsterID(muid)
	if err != nil || mid != 100 {
		t.Fatalf("monsterID = %d (%v), want 100", mid, err)
	}
	if UIDPeerIndex(muid) != 3 {
		t.Fatalf("peer = %d, want 3", UIDPeerIndex(muid))
	}
	if UIDSeq(muid) != 7 {
		t.Fatalf("seq = %d, want 7", UIDSeq(muid))
	}

	// seq 缺省补 1
	muid2, _ := BuildMonsterUID(5, 0, 0)
	if UIDSeq(muid2) != 1 {
		t.Fatalf("default seq = %d, want 1", UIDSeq(muid2))
	}

	nuid, err := BuildNPCUID(42, 1, 0)
	if err != nil {
		t.Fatalf("BuildNPCUID: %v", err)
	}
	if !IsNPCUID(nuid) {
		t.Fatal("should be npc uid")
	}
	nid, err := GetNPCID(nuid)
	if err != nil || nid != 42 {
		t.Fatalf("npcID = %d (%v), want 42", nid, err)
	}
}

func TestBuildUIDValidation(t *testing.T) {
	// id = 0 拒绝
	if _, err := BuildMapUID(0, 0); err == nil {
		t.Fatal("id 0 should be rejected")
	}
	// id 溢出 24 位
	if _, err := BuildMapUID(1<<24, 0); err == nil {
		t.Fatal("id exceeding 24 bits should be rejected")
	}
	// peerIndex 溢出 4 位
	if _, err := BuildMapUID(1, 16); err == nil {
		t.Fatal("peerIndex 16 should be rejected")
	}
	// 边界值可用
	if _, err := BuildMapUID((1<<24)-1, 15); err != nil {
		t.Fatalf("boundary values should be accepted: %v", err)
	}
}

func TestUIDTypeStringAndString(t *testing.T) {
	cases := map[UIDType]string{
		UIDCor: "COR", UIDMap: "MAP", UIDNPC: "NPC", UIDMon: "MON",
		UIDPly: "PLY", UIDRcv: "RCV", UIDQst: "QST", UIDSlo: "SLO",
	}
	for typ, want := range cases {
		if got := typ.String(); got != want {
			t.Fatalf("%d.String() = %q, want %q", typ, got, want)
		}
	}

	if GetUIDString(0) != "ZERO" {
		t.Fatalf("zero uid string = %q", GetUIDString(0))
	}
	uid := GetPlayerUID(7)
	if s := GetUIDString(uid); s != "PLY_7" {
		t.Fatalf("player uid string = %q, want PLY_7", s)
	}
	muid, _ := BuildMonsterUID(9, 2, 3)
	if s := GetUIDString(muid); s != "MON_9_2_3" {
		t.Fatalf("monster uid string = %q, want MON_9_2_3", s)
	}
}

func TestCoreAndQuestUID(t *testing.T) {
	svc := GetServiceCoreUID()
	if GetUIDType(svc) != UIDCor {
		t.Fatal("service core should be COR type")
	}
	if UIDPeerIndex(svc) != 0 {
		t.Fatalf("service core peer = %d, want 0", UIDPeerIndex(svc))
	}
	if GetPeerCoreUID(3) == svc {
		t.Fatal("peer 3 should differ from service core")
	}

	q := GetQuestUID(88)
	// 原版 getQuestUID 是 (UID_QST << uidTypeBitOff) + questID，
	// questID 同样在**最低位**，不走 id 段（uidf.cpp:170）
	if GetUIDType(q) != UIDQst {
		t.Fatalf("quest uid type = %v, want QST", GetUIDType(q))
	}
	if uint32(q) != 88 {
		t.Fatalf("quest uid low 32 = %d, want 88", uint32(q))
	}
	if q != 0x3800000000000058 {
		t.Fatalf("quest uid = %#x, want 0x3800000000000058", q)
	}
	slo := GetServerLuaObjectUID(12)
	// 同样是最低位承载 index
	if GetUIDType(slo) != UIDSlo || uint32(slo) != 12 {
		t.Fatalf("lua obj uid wrong: %#x", slo)
	}
}

func TestTypeMismatchErrors(t *testing.T) {
	puid := GetPlayerUID(1)
	if _, err := GetMapID(puid); err == nil {
		t.Fatal("GetMapID should reject a player uid")
	}
	if _, err := GetMonsterID(puid); err == nil {
		t.Fatal("GetMonsterID should reject a player uid")
	}
	if _, err := GetNPCID(puid); err == nil {
		t.Fatal("GetNPCID should reject a player uid")
	}
	if _, err := GetPlayerDBID(GetQuestUID(1)); err == nil {
		t.Fatal("GetPlayerDBID should reject a quest uid")
	}
}
