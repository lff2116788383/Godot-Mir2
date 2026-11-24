package protocol

import "github.com/lff2116788383/godot-mir2/server-go/internal/common"

// CMPing is the fixed payload of CM_PING.
type CMPing struct {
	Tick uint32
}

// CMLogin is the fixed payload of CM_LOGIN.
type CMLogin struct {
	ID       common.StaticBuffer[common.Cap64]
	Password common.StaticBuffer[common.Cap64]
}

type CMSetMagicKey struct {
	MagicID uint32
	Key     uint8
}

type CMCreateChar struct {
	Name   common.StaticBuffer[common.Cap64]
	Job    uint8
	Gender uint8
}

type CMDeleteChar struct {
	Password common.StaticBuffer[common.Cap64]
}

type CMAction struct {
	UID    uint64
	MapUID uint64
	Action common.ActionNode
}

type CMQueryCORecord struct {
	AimUID uint64
}

type CMRequestAddHealth struct {
	AddHP int32
	AddMP int32
}

type CMRequestAddExp struct {
	AddExp uint64
}

type CMRequestRetrieveSecuredItem struct {
	ItemID uint32
	SeqID  uint32
}

type CMRequestSpaceMove struct {
	MapUID uint64
	X      uint16
	Y      uint16
}

type CMRequestMagicDamage struct {
	MagicID uint32
	AimUID  uint64
}

type CMPickUp struct {
	X      uint16
	Y      uint16
	MapUID uint64
}

type CMQueryBaseMapUID struct {
	MapID uint32
}

type CMQueryUIDBuff struct {
	UID uint64
}

type CMQueryPlayerName struct {
	UID uint64
}

type CMQueryNPCName struct {
	UID uint64
}

type CMQueryPlayerWLDesp struct {
	UID uint64
}

type CMQueryChatPeerList struct {
	Input common.StaticBuffer[common.Cap128]
}

// CMChatMessageHeader mirrors the fixed legacy chat message header.
type CMChatMessageHeader struct {
	ToCPID uint64
	HasRef bool
	RefID  uint64
}

type CMPlayerSay struct {
	Content [128]byte
}

type CMPlayerBroadcast struct {
	Content [128]byte
}

type CMQueryChatMessage struct {
	MsgID uint64
}

type CMCreateChatGroup struct {
	Name common.StaticBuffer[common.Cap128]
	List common.StaticVector[uint32, common.Cap512]
}

type CMCreateAccount struct {
	ID       common.StaticBuffer[common.Cap64]
	Password common.StaticBuffer[common.Cap64]
}

type CMChangePassword struct {
	ID          common.StaticBuffer[common.Cap64]
	Password    common.StaticBuffer[common.Cap64]
	PasswordNew common.StaticBuffer[common.Cap64]
}

type CMSetRuntimeConfig struct {
	Type     uint16
	KeyBuf   common.StaticBuffer[common.Cap64]
	ValueBuf common.StaticBuffer[common.Cap256]
}

type CMNPCEvent struct {
	UID       uint64
	Path      [100]byte
	Event     [100]byte
	Value     [200]byte
	ValueSize int16
}

type CMQuerySellItemList struct {
	NPCUID uint64
	ItemID uint32
}

type CMDropItem struct {
	ItemID uint32
	SeqID  uint32
	Count  uint16
}

type CMConsumeItem struct {
	ItemID uint32
	SeqID  uint32
	Count  uint16
}

type CMMakeItem struct {
	ItemID uint32
	Count  uint16
}

type CMBuy struct {
	NPCUID uint64
	ItemID uint32
	SeqID  uint32
	Count  uint64
}

type CMAddFriend struct {
	CPID uint64
}

type CMAcceptAddFriend struct {
	CPID uint64
}

type CMRejectAddFriend struct {
	CPID uint64
}

type CMBlockPlayer struct {
	CPID uint64
}

type CMRequestEquipWear struct {
	ItemID uint32
	SeqID  uint32
	WLType uint16
}

type CMRequestGrabWear struct {
	WLType uint16
}

type CMRequestEquipBelt struct {
	ItemID uint32
	SeqID  uint32
	Slot   uint16
}

type CMRequestGrabBelt struct {
	Slot uint16
}

type CMRequestConsumeBelt struct {
	Slot uint16
}

type CMRequestJoinTeam struct {
	UID uint64
}

type CMRequestLeaveTeam struct {
	UID uint64
}

type CMQueryTeamInfo struct {
	UID uint64
}

type CMTransferTeamLeader struct {
	UID uint64
}

type CMSetFollowTeamLeader struct {
	Follow uint8
}

type CMRequestDirectTrade struct {
	UID uint64
}

type CMRespondDirectTrade struct {
	UID    uint64
	Accept uint8
}

type CMCommitDirectTrade struct {
	UID uint64
}

type CMCancelDirectTrade struct {
	UID uint64
}

// CMRequestLatestChatMessage mirrors the packed static vector plus bitfield tail.
type CMRequestLatestChatMessage struct {
	CPIDList    common.StaticVector[uint64, common.Cap128]
	LimitCount  uint32
	IncludeSend bool
	IncludeRecv bool
}

type CMQueryRanking struct {
	Type uint8
}

type CMClaimDelivery struct {
	Record common.StaticBuffer[common.Cap32]
}

type CMQueryAuctionItemList struct {
	Category uint8
}

type CMRegisterAuctionItem struct {
	ItemID uint32
	SeqID  uint32
	Price  uint64
	Note   common.StaticBuffer[common.Cap128]
}

type CMBuyAuctionItem struct {
	AuctionID uint64
}

type CMUnregisterAuctionItem struct {
	AuctionID uint64
}

type CMAddStallItem struct {
	ItemID uint32
	SeqID  uint32
	Price  uint64
}

type CMRemoveStallItem struct {
	ItemID uint32
	SeqID  uint32
}

type CMStartStall struct {
	Name common.StaticBuffer[common.Cap48]
}

type CMQueryStall struct {
	DBID uint32
}

type CMBuyStallItem struct {
	SellerDBID uint32
	ItemID     uint32
	SeqID      uint32
	Count      uint64
	Price      uint64
}

type CMQueryPlayerGuild struct {
	DBID uint32
}

type CMInviteGuildMember struct {
	Name common.StaticBuffer[common.Cap64]
}

type CMRespondGuildInvite struct {
	GuildID uint32
	Accept  uint8
}

type CMKickGuildMember struct {
	DBID uint32
}

type CMTransferGuildLeader struct {
	DBID uint32
}

type CMSetGuildNotice struct {
	Notice common.StaticBuffer[common.Cap1024]
}

type CMSetGuildMemberTitle struct {
	DBID  uint32
	Title common.StaticBuffer[common.Cap32]
}

type CMGuildChat struct {
	Content [128]byte
}
