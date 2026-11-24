package protocol

import "github.com/lff2116788383/godot-mir2/server-go/internal/common"

type SMPing struct {
	Tick uint32
}

type SMUID struct {
	UID uint64
}

type SMLoginError struct {
	Error uint8
}

type SMCreateAccountError struct {
	Error uint32
}

type SMChangePasswordError struct {
	Error uint32
}

type SMQueryCharOK struct {
	Name   common.StaticBuffer[common.Cap64]
	Gender uint8
	Job    uint8
	Exp    uint64
}

type SMQueryCharError struct {
	Error uint8
}

type SMCreateCharError struct {
	Error uint8
}

type SMDeleteCharError struct {
	Error uint8
}

// SMOnlineOK mirrors the packed online payload with its one-byte bitfield.
type SMOnlineOK struct {
	UID    uint64
	Name   common.StaticBuffer[common.Cap128]
	Gender bool
	Job    uint8
	MapUID uint64
	Action common.ActionNode
}

type SMOnlineError struct {
	Error uint8
}

type SMAction struct {
	UID    uint64
	MapUID uint64
	Action common.ActionNode
}

// SMCORecord keeps the untagged C++ union as raw bytes. The correct union
// interpretation depends on surrounding world state, not on the packet itself.
type SMCORecord struct {
	UID    uint64
	MapUID uint64
	Action common.ActionNode
	Union  [5]byte
}

type SMNotifyDead struct {
	UID uint64
}

type SMDeadFadeOut struct {
	UID    uint64
	MapUID uint64
	X      uint32
	Y      uint32
}

type SMExp struct {
	Exp uint64
}

type SMBuff struct {
	UID   uint64
	Type  uint32
	State uint32
}

type SMMiss struct {
	UID uint64
}

type SMCastMagic struct {
	UID        uint64
	MapUID     uint64
	Magic      uint8
	MagicParam uint8
	Speed      uint8
	Direction  uint8
	X          uint16
	Y          uint16
	AimX       uint16
	AimY       uint16
	AimUID     uint64
}

type SMOffline struct {
	UID    uint64
	MapUID uint64
}

type SMCODisappear struct {
	UID    uint64
	MapUID uint64
}

type SMRevive struct {
	MapUID    uint64
	X         uint32
	Y         uint32
	Direction uint16
}

type SMViewRange struct {
	ViewRange uint8
}

type SMPickUpError struct {
	FailedItemID uint32
}

type SMRemoveGroundItem struct {
	X    uint16
	Y    uint16
	ID   uint32
	DBID uint32
}

type SMGold struct {
	Gold uint64
}

type SMInvOpCost struct {
	InvOp  uint32
	ItemID uint32
	SeqID  uint32
	Cost   uint32
}

type SMStrikeGrid struct {
	X uint32
	Y uint32
}

type SMBuildVersion struct {
	Version common.StaticBuffer[common.Cap128]
}

type SMRemoveItem struct {
	ItemID uint32
	SeqID  uint32
	Count  uint16
}

type SMRemoveSecuredItem struct {
	ItemID uint32
	SeqID  uint32
}

type SMBuySucceed struct {
	NPCUID uint64
	ItemID uint32
	SeqID  uint32
}

type SMBuyError struct {
	NPCUID uint64
	ItemID uint32
	SeqID  uint32
	Error  uint16
}

type SMEquipWearError struct {
	ItemID uint32
	SeqID  uint32
	Error  uint16
}

type SMGrabWearError struct {
	Error uint16
}

type SMEquipBeltError struct {
	ItemID uint32
	SeqID  uint32
	Error  uint16
}

type SMGrabBeltError struct {
	Error uint16
}

type SMWearItemDuration struct {
	WLType   uint32
	Duration [2]uint32
}

type SMPlayerTeamLeader struct {
	UID        uint64
	TeamLeader uint8
}

type SMTeamMemberLeft struct {
	UID uint64
}

type SMTeamError struct {
	Error uint8
}

type SMRequestDirectTrade struct {
	UID  uint64
	Name common.StaticBuffer[common.Cap64]
}

type SMStartDirectTrade struct {
	UID  uint64
	Name common.StaticBuffer[common.Cap64]
}

type SMCompleteDirectTrade struct {
	UID uint64
}

type SMCloseDirectTrade struct {
	UID uint64
}

type SMDirectTradeError struct {
	Error uint8
}

type SMStallError struct {
	Error uint8
}

type SMPlayerStall struct {
	UID     uint64
	Selling uint8
}

type SMNPCError struct {
	Error uint8
}

type SMPlayerSay struct {
	UID     uint64
	Content [128]byte
}

type SMPlayerBroadcast struct {
	UID     uint64
	Content [128]byte
}

type SMClaimDeliveryError struct {
	Error uint8
}

type SMAuctionRegisterError struct {
	Error uint8
}

type SMAuctionBuyError struct {
	Error uint8
}

type SMAuctionUnregisterError struct {
	Error uint8
}

type SMGuildError struct {
	Error uint8
}

type SMGuildChat struct {
	UID     uint64
	Name    common.StaticBuffer[common.Cap64]
	Content [128]byte
}
