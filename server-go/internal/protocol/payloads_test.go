package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
)

var fixedClientPayloads = map[CMType]any{
	CM_PING:                       CMPing{},
	CM_LOGIN:                      CMLogin{},
	CM_CREATECHAR:                 CMCreateChar{},
	CM_DELETECHAR:                 CMDeleteChar{},
	CM_ACTION:                     CMAction{},
	CM_SETMAGICKEY:                CMSetMagicKey{},
	CM_SETRUNTIMECONFIG:           CMSetRuntimeConfig{},
	CM_QUERYCORECORD:              CMQueryCORecord{},
	CM_QUERYNPCNAME:               CMQueryNPCName{},
	CM_REQUESTADDHEALTH:           CMRequestAddHealth{},
	CM_REQUESTADDEXP:              CMRequestAddExp{},
	CM_REQUESTRETRIEVESECUREDITEM: CMRequestRetrieveSecuredItem{},
	CM_REQUESTSPACEMOVE:           CMRequestSpaceMove{},
	CM_REQUESTMAGICDAMAGE:         CMRequestMagicDamage{},
	CM_PICKUP:                     CMPickUp{},
	CM_QUERYBASEMAPUID:            CMQueryBaseMapUID{},
	CM_QUERYUIDBUFF:               CMQueryUIDBuff{},
	CM_QUERYPLAYERNAME:            CMQueryPlayerName{},
	CM_QUERYPLAYERWLDESP:          CMQueryPlayerWLDesp{},
	CM_QUERYCHATPEERLIST:          CMQueryChatPeerList{},
	CM_QUERYCHATMESSAGE:           CMQueryChatMessage{},
	CM_PLAYERSAY:                  CMPlayerSay{},
	CM_PLAYERBROADCAST:            CMPlayerBroadcast{},
	CM_CREATECHATGROUP:            CMCreateChatGroup{},
	CM_CREATEACCOUNT:              CMCreateAccount{},
	CM_CHANGEPASSWORD:             CMChangePassword{},
	CM_NPCEVENT:                   CMNPCEvent{},
	CM_QUERYSELLITEMLIST:          CMQuerySellItemList{},
	CM_DROPITEM:                   CMDropItem{},
	CM_CONSUMEITEM:                CMConsumeItem{},
	CM_MAKEITEM:                   CMMakeItem{},
	CM_BUY:                        CMBuy{},
	CM_ADDFRIEND:                  CMAddFriend{},
	CM_ACCEPTADDFRIEND:            CMAcceptAddFriend{},
	CM_REJECTADDFRIEND:            CMRejectAddFriend{},
	CM_BLOCKPLAYER:                CMBlockPlayer{},
	CM_REQUESTEQUIPWEAR:           CMRequestEquipWear{},
	CM_REQUESTGRABWEAR:            CMRequestGrabWear{},
	CM_REQUESTEQUIPBELT:           CMRequestEquipBelt{},
	CM_REQUESTGRABBELT:            CMRequestGrabBelt{},
	CM_REQUESTCONSUMEBELT:         CMRequestConsumeBelt{},
	CM_REQUESTJOINTEAM:            CMRequestJoinTeam{},
	CM_REQUESTLEAVETEAM:           CMRequestLeaveTeam{},
	CM_QUERYTEAMINFO:              CMQueryTeamInfo{},
	CM_TRANSFERTEAMLEADER:         CMTransferTeamLeader{},
	CM_SETFOLLOWTEAMLEADER:        CMSetFollowTeamLeader{},
	CM_REQUESTDIRECTTRADE:         CMRequestDirectTrade{},
	CM_RESPONDDIRECTTRADE:         CMRespondDirectTrade{},
	CM_COMMITDIRECTTRADE:          CMCommitDirectTrade{},
	CM_CANCELDIRECTTRADE:          CMCancelDirectTrade{},
	CM_REQUESTLATESTCHATMESSAGE:   CMRequestLatestChatMessage{},
	CM_QUERYRANKING:               CMQueryRanking{},
	CM_CLAIMDELIVERY:              CMClaimDelivery{},
	CM_QUERYAUCTIONITEMLIST:       CMQueryAuctionItemList{},
	CM_REGISTERAUCTIONITEM:        CMRegisterAuctionItem{},
	CM_BUYAUCTIONITEM:             CMBuyAuctionItem{},
	CM_UNREGISTERAUCTIONITEM:      CMUnregisterAuctionItem{},
	CM_ADDSTALLITEM:               CMAddStallItem{},
	CM_REMOVESTALLITEM:            CMRemoveStallItem{},
	CM_STARTSTALL:                 CMStartStall{},
	CM_QUERYSTALL:                 CMQueryStall{},
	CM_BUYSTALLITEM:               CMBuyStallItem{},
	CM_QUERYPLAYERGUILD:           CMQueryPlayerGuild{},
	CM_INVITEGUILDMEMBER:          CMInviteGuildMember{},
	CM_RESPONDGUILDINVITE:         CMRespondGuildInvite{},
	CM_KICKGUILDMEMBER:            CMKickGuildMember{},
	CM_TRANSFERGUILDLEADER:        CMTransferGuildLeader{},
	CM_SETGUILDNOTICE:             CMSetGuildNotice{},
	CM_SETGUILDMEMBERTITLE:        CMSetGuildMemberTitle{},
	CM_GUILDCHAT:                  CMGuildChat{},
}

var fixedServerPayloads = map[SMType]any{
	SM_PING:                   SMPing{},
	SM_UID:                    SMUID{},
	SM_LOGINERROR:             SMLoginError{},
	SM_CREATEACCOUNTERROR:     SMCreateAccountError{},
	SM_CHANGEPASSWORDERROR:    SMChangePasswordError{},
	SM_QUERYCHAROK:            SMQueryCharOK{},
	SM_QUERYCHARERROR:         SMQueryCharError{},
	SM_CREATECHARERROR:        SMCreateCharError{},
	SM_DELETECHARERROR:        SMDeleteCharError{},
	SM_ONLINEOK:               SMOnlineOK{},
	SM_ONLINEERROR:            SMOnlineError{},
	SM_ACTION:                 SMAction{},
	SM_CORECORD:               SMCORecord{},
	SM_NOTIFYDEAD:             SMNotifyDead{},
	SM_DEADFADEOUT:            SMDeadFadeOut{},
	SM_EXP:                    SMExp{},
	SM_BUFF:                   SMBuff{},
	SM_MISS:                   SMMiss{},
	SM_CASTMAGIC:              SMCastMagic{},
	SM_OFFLINE:                SMOffline{},
	SM_CODISAPPEAR:            SMCODisappear{},
	SM_REVIVE:                 SMRevive{},
	SM_VIEWRANGE:              SMViewRange{},
	SM_PICKUPERROR:            SMPickUpError{},
	SM_REMOVEGROUNDITEM:       SMRemoveGroundItem{},
	SM_NPCERROR:               SMNPCError{},
	SM_GOLD:                   SMGold{},
	SM_INVOPCOST:              SMInvOpCost{},
	SM_STRIKEGRID:             SMStrikeGrid{},
	SM_PLAYERTEAMLEADER:       SMPlayerTeamLeader{},
	SM_BUILDVERSION:           SMBuildVersion{},
	SM_REMOVEITEM:             SMRemoveItem{},
	SM_REMOVESECUREDITEM:      SMRemoveSecuredItem{},
	SM_BUYSUCCEED:             SMBuySucceed{},
	SM_BUYERROR:               SMBuyError{},
	SM_EQUIPWEARERROR:         SMEquipWearError{},
	SM_GRABWEARERROR:          SMGrabWearError{},
	SM_EQUIPBELTERROR:         SMEquipBeltError{},
	SM_GRABBELTERROR:          SMGrabBeltError{},
	SM_WEARITEMDURATION:       SMWearItemDuration{},
	SM_TEAMERROR:              SMTeamError{},
	SM_REQUESTDIRECTTRADE:     SMRequestDirectTrade{},
	SM_STARTDIRECTTRADE:       SMStartDirectTrade{},
	SM_COMPLETEDIRECTTRADE:    SMCompleteDirectTrade{},
	SM_CLOSEDIRECTTRADE:       SMCloseDirectTrade{},
	SM_DIRECTTRADEERROR:       SMDirectTradeError{},
	SM_STALLERROR:             SMStallError{},
	SM_PLAYERSTALL:            SMPlayerStall{},
	SM_PLAYERSAY:              SMPlayerSay{},
	SM_PLAYERBROADCAST:        SMPlayerBroadcast{},
	SM_CLAIMDELIVERYERROR:     SMClaimDeliveryError{},
	SM_AUCTIONREGISTERERROR:   SMAuctionRegisterError{},
	SM_AUCTIONBUYERROR:        SMAuctionBuyError{},
	SM_AUCTIONUNREGISTERERROR: SMAuctionUnregisterError{},
	SM_GUILDERROR:             SMGuildError{},
	SM_GUILDCHAT:              SMGuildChat{},
}

func TestAllFixedMessagePayloadSizes(t *testing.T) {
	t.Parallel()

	t.Run("client", func(t *testing.T) {
		for id, attr := range clientMessageAttributes {
			if attr.Kind != MessageFixedCompressed && attr.Kind != MessageFixedRaw {
				continue
			}
			payload, ok := fixedClientPayloads[id]
			if !ok {
				t.Fatalf("%s has no Go payload type", attr.Name)
			}
			checkPayloadCodec(t, attr, payload)
		}
	})

	t.Run("server", func(t *testing.T) {
		for id, attr := range serverMessageAttributes {
			if attr.Kind != MessageFixedCompressed && attr.Kind != MessageFixedRaw {
				continue
			}
			payload, ok := fixedServerPayloads[id]
			if !ok {
				t.Fatalf("%s has no Go payload type", attr.Name)
			}
			checkPayloadCodec(t, attr, payload)
		}
	})
}

func checkPayloadCodec(t *testing.T, attr MessageAttribute, payload any) {
	t.Helper()
	encoded, err := AppendWirePayload(nil, payload)
	if err != nil {
		t.Fatalf("%s encode: %v", attr.Name, err)
	}
	if len(encoded) != attr.DataLen {
		t.Fatalf("%s encoded length = %d, C++ size = %d", attr.Name, len(encoded), attr.DataLen)
	}

	target := reflect.New(reflect.TypeOf(payload)).Interface()
	consumed, err := ReadWirePayload(target, encoded)
	if err != nil {
		t.Fatalf("%s decode: %v", attr.Name, err)
	}
	if consumed != len(encoded) {
		t.Fatalf("%s consumed %d of %d bytes", attr.Name, consumed, len(encoded))
	}
	reencoded, err := AppendWirePayload(nil, target)
	if err != nil {
		t.Fatalf("%s re-encode: %v", attr.Name, err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatalf("%s did not round-trip\nfirst:  %x\nsecond: %x", attr.Name, encoded, reencoded)
	}
}

func TestCMChatMessageHeaderCodec(t *testing.T) {
	t.Parallel()

	header := CMChatMessageHeader{ToCPID: 0x1122334455667788, HasRef: true, RefID: 1}
	encoded, err := AppendWirePayload(nil, header)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString("88776655443322110300000000000000")
	if !bytes.Equal(encoded, want) {
		t.Fatalf("encoded = %x, want %x", encoded, want)
	}

	var decoded CMChatMessageHeader
	if _, err := ReadWirePayload(&decoded, encoded); err != nil {
		t.Fatal(err)
	}
	if decoded != header {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestCMRequestLatestChatMessageCodec(t *testing.T) {
	t.Parallel()

	message := CMRequestLatestChatMessage{
		CPIDList:    common.StaticVector[uint64, common.Cap128]{Data: []uint64{1, 2}},
		LimitCount:  0x3fffffff,
		IncludeSend: true,
		IncludeRecv: true,
	}
	encoded, err := AppendWirePayload(nil, message)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != CMRequestLatestChatMessageWireSize {
		t.Fatalf("length = %d", len(encoded))
	}
	if got := binary.LittleEndian.Uint32(encoded[len(encoded)-4:]); got != 0xffffffff {
		t.Fatalf("flags = %08x", got)
	}

	var decoded CMRequestLatestChatMessage
	if _, err := ReadWirePayload(&decoded, encoded); err != nil {
		t.Fatal(err)
	}
	if decoded.LimitCount != message.LimitCount || !decoded.IncludeSend || !decoded.IncludeRecv || !reflect.DeepEqual(decoded.CPIDList.Data, []uint64{1, 2}) {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestSMOnlineOKCodec(t *testing.T) {
	t.Parallel()

	message := SMOnlineOK{
		UID:    123,
		Name:   common.StaticBuffer[common.Cap128]{Data: []byte("alice")},
		Gender: true,
		Job:    7,
		MapUID: 456,
		Action: common.ActionNode{Type: common.ActionStand, Direction: 3, X: 10, Y: 20},
	}
	encoded, err := AppendWirePayload(nil, message)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != SMOnlineOKWireSize {
		t.Fatalf("length = %d", len(encoded))
	}
	if encoded[140] != 0x0f {
		t.Fatalf("bitfield byte = %02x, want 0f", encoded[140])
	}

	var decoded SMOnlineOK
	if _, err := ReadWirePayload(&decoded, encoded); err != nil {
		t.Fatal(err)
	}
	if decoded.UID != message.UID || decoded.Name.String() != "alice" || !decoded.Gender || decoded.Job != 7 || decoded.MapUID != 456 || decoded.Action != message.Action {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestSMCORecordUnion(t *testing.T) {
	t.Parallel()

	var record SMCORecord
	record.SetMonsterID(0x11223344)
	if record.MonsterID() != 0x11223344 {
		t.Fatal("monster union mismatch")
	}

	record.SetPlayer(true, 7, true, true, 55)
	if !record.PlayerGender() || record.PlayerJob() != 7 || !record.PlayerTeamLeader() || !record.PlayerStall() || record.PlayerLevel() != 55 {
		t.Fatalf("player union mismatch: %x", record.Union)
	}
	if record.Union[0] != 0x3f {
		t.Fatalf("player flags = %02x, want 3f", record.Union[0])
	}

	encoded, err := AppendWirePayload(nil, record)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != SMCORecordWireSize {
		t.Fatalf("length = %d", len(encoded))
	}
	var decoded SMCORecord
	if _, err := ReadWirePayload(&decoded, encoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.Union[:], record.Union[:]) {
		t.Fatalf("union = %x", decoded.Union)
	}
}
func TestCMCreateChatGroupStaticVector(t *testing.T) {
	t.Parallel()

	message := CMCreateChatGroup{
		Name: common.StaticBuffer[common.Cap128]{Data: []byte("party")},
		List: common.StaticVector[uint32, common.Cap512]{Data: []uint32{11, 22, 33}},
	}
	encoded, err := AppendWirePayload(nil, message)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 2184 {
		t.Fatalf("length = %d", len(encoded))
	}
	if encoded[132] != 3 || encoded[133] != 0 {
		t.Fatalf("static vector size = %x", encoded[132:134])
	}
	if got := binary.LittleEndian.Uint32(encoded[136:140]); got != 11 {
		t.Fatalf("first list element = %d", got)
	}

	var decoded CMCreateChatGroup
	if _, err := ReadWirePayload(&decoded, encoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Name.String() != "party" || !reflect.DeepEqual(decoded.List.Data, []uint32{11, 22, 33}) {
		t.Fatalf("decoded = %+v", decoded)
	}
}
