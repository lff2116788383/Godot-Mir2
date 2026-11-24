package protocol

import "fmt"

// ChatPeerType mirrors the legacy CPR_* enum.
type ChatPeerType int32

const (
	CPR_NONE ChatPeerType = iota
	CPR_SPECIAL
	CPR_PLAYER
	CPR_GROUP
	CPR_END
)

func readPortableCount(reader *PortableReader, max uint64, what string) int {
	count := reader.SizeTag()
	if reader.Err() != nil {
		return 0
	}
	if count > max {
		reader.fail(fmt.Errorf("%s count %d exceeds limit %d", what, count, max))
		return 0
	}
	return int(count)
}

// SDSendPackage is a packet queued for delivery to a client connection.
type SDSendPackage struct {
	Type uint8
	Buf  []byte
}

func (v SDSendPackage) MarshalPortable(writer *PortableWriter) error {
	writer.Uint8(v.Type)
	writer.Bytes(v.Buf)
	return writer.Err()
}

func (v *SDSendPackage) UnmarshalPortable(reader *PortableReader) error {
	v.Type = reader.Uint8()
	v.Buf = append(v.Buf[:0], reader.Bytes()...)
	return reader.Err()
}

// SDObserverNetPackage forwards one packet to players observing a map grid.
type SDObserverNetPackage struct {
	UID     uint64
	X       int32
	Y       int32
	Package SDSendPackage
}

func (v SDObserverNetPackage) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.Int32(v.X)
	writer.Int32(v.Y)
	if err := v.Package.MarshalPortable(writer); err != nil {
		return err
	}
	return writer.Err()
}

func (v *SDObserverNetPackage) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.X = reader.Int32()
	v.Y = reader.Int32()
	if err := v.Package.UnmarshalPortable(reader); err != nil {
		return err
	}
	return reader.Err()
}

type SDInitQuest struct {
	QuestID        uint32
	FullScriptName string
}

func (v SDInitQuest) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.QuestID)
	writer.String(v.FullScriptName)
	return writer.Err()
}

func (v *SDInitQuest) UnmarshalPortable(reader *PortableReader) error {
	v.QuestID = reader.Uint32()
	v.FullScriptName = reader.String()
	return reader.Err()
}

type SDInitGuard struct {
	MonsterID uint32
	MapUID    uint64
	X         int32
	Y         int32
	StrictLoc bool
	Direction int32
}

func (v SDInitGuard) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.MonsterID)
	writer.Uint64(v.MapUID)
	writer.Int32(v.X)
	writer.Int32(v.Y)
	writer.Bool(v.StrictLoc)
	writer.Int32(v.Direction)
	return writer.Err()
}

func (v *SDInitGuard) UnmarshalPortable(reader *PortableReader) error {
	v.MonsterID = reader.Uint32()
	v.MapUID = reader.Uint64()
	v.X = reader.Int32()
	v.Y = reader.Int32()
	v.StrictLoc = reader.Bool()
	v.Direction = reader.Int32()
	return reader.Err()
}

type SDInitPlayer struct {
	DBID      uint32
	ChannID   uint32
	Name      string
	MapUID    uint64
	X         int32
	Y         int32
	StrictLoc bool
	HP        int32
	MP        int32
	Exp       uint64
	Gold      uint64
	Gender    bool
	Job       int32
	Hair      int32
	HairColor int32
	PKPoint   int32
	ViewRange int32
}

func (v SDInitPlayer) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.DBID)
	writer.Uint32(v.ChannID)
	writer.String(v.Name)
	writer.Uint64(v.MapUID)
	writer.Int32(v.X)
	writer.Int32(v.Y)
	writer.Bool(v.StrictLoc)
	writer.Int32(v.HP)
	writer.Int32(v.MP)
	writer.Uint64(v.Exp)
	writer.Uint64(v.Gold)
	writer.Bool(v.Gender)
	writer.Int32(v.Job)
	writer.Int32(v.Hair)
	writer.Int32(v.HairColor)
	writer.Int32(v.PKPoint)
	writer.Int32(v.ViewRange)
	return writer.Err()
}

func (v *SDInitPlayer) UnmarshalPortable(reader *PortableReader) error {
	v.DBID = reader.Uint32()
	v.ChannID = reader.Uint32()
	v.Name = reader.String()
	v.MapUID = reader.Uint64()
	v.X = reader.Int32()
	v.Y = reader.Int32()
	v.StrictLoc = reader.Bool()
	v.HP = reader.Int32()
	v.MP = reader.Int32()
	v.Exp = reader.Uint64()
	v.Gold = reader.Uint64()
	v.Gender = reader.Bool()
	v.Job = reader.Int32()
	v.Hair = reader.Int32()
	v.HairColor = reader.Int32()
	v.PKPoint = reader.Int32()
	v.ViewRange = reader.Int32()
	return reader.Err()
}

type SDInitNPChar struct {
	LookID         uint16
	NPCName        string
	FullScriptName string
	MapUID         uint64
	X              int32
	Y              int32
	StrictLoc      bool
	GfxDir         int32
}

func (v SDInitNPChar) MarshalPortable(writer *PortableWriter) error {
	writer.Uint16(v.LookID)
	writer.String(v.NPCName)
	writer.String(v.FullScriptName)
	writer.Uint64(v.MapUID)
	writer.Int32(v.X)
	writer.Int32(v.Y)
	writer.Bool(v.StrictLoc)
	writer.Int32(v.GfxDir)
	return writer.Err()
}

func (v *SDInitNPChar) UnmarshalPortable(reader *PortableReader) error {
	v.LookID = reader.Uint16()
	v.NPCName = reader.String()
	v.FullScriptName = reader.String()
	v.MapUID = reader.Uint64()
	v.X = reader.Int32()
	v.Y = reader.Int32()
	v.StrictLoc = reader.Bool()
	v.GfxDir = reader.Int32()
	return reader.Err()
}

type SDInitMonster struct {
	MonsterID uint32
	MapUID    uint64
	X         int32
	Y         int32
	StrictLoc bool
	Direction int32
	MasterUID uint64
}

func (v SDInitMonster) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.MonsterID)
	writer.Uint64(v.MapUID)
	writer.Int32(v.X)
	writer.Int32(v.Y)
	writer.Bool(v.StrictLoc)
	writer.Int32(v.Direction)
	writer.Uint64(v.MasterUID)
	return writer.Err()
}

func (v *SDInitMonster) UnmarshalPortable(reader *PortableReader) error {
	v.MonsterID = reader.Uint32()
	v.MapUID = reader.Uint64()
	v.X = reader.Int32()
	v.Y = reader.Int32()
	v.StrictLoc = reader.Bool()
	v.Direction = reader.Int32()
	v.MasterUID = reader.Uint64()
	return reader.Err()
}

type SDNPCXMLLayout struct {
	NPCUID    uint64
	EventPath string
	XMLLayout string
}

func (v SDNPCXMLLayout) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.NPCUID)
	writer.String(v.EventPath)
	writer.String(v.XMLLayout)
	return writer.Err()
}

func (v *SDNPCXMLLayout) UnmarshalPortable(reader *PortableReader) error {
	v.NPCUID = reader.Uint64()
	v.EventPath = reader.String()
	v.XMLLayout = reader.String()
	return reader.Err()
}

type SDChatGroupMember struct {
	DBID     uint32
	Priority uint16
}

func (v SDChatGroupMember) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.DBID)
	writer.Uint16(v.Priority)
	return writer.Err()
}

func (v *SDChatGroupMember) UnmarshalPortable(reader *PortableReader) error {
	v.DBID = reader.Uint32()
	v.Priority = reader.Uint16()
	return reader.Err()
}

type SDChatPeerPlayerVar struct {
	Gender bool
	Job    int32
}

func (v SDChatPeerPlayerVar) MarshalPortable(writer *PortableWriter) error {
	writer.Bool(v.Gender)
	writer.Int32(v.Job)
	return writer.Err()
}

func (v *SDChatPeerPlayerVar) UnmarshalPortable(reader *PortableReader) error {
	v.Gender = reader.Bool()
	v.Job = reader.Int32()
	return reader.Err()
}

type SDChatPeerGroupVar struct {
	Creator    uint32
	Createtime uint64
	MemberList []SDChatGroupMember
}

func (v SDChatPeerGroupVar) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.Creator)
	writer.Uint64(v.Createtime)
	writer.SizeTag(uint64(len(v.MemberList)))
	for _, member := range v.MemberList {
		if err := member.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDChatPeerGroupVar) UnmarshalPortable(reader *PortableReader) error {
	v.Creator = reader.Uint32()
	v.Createtime = reader.Uint64()
	count := readPortableCount(reader, 1<<20, "chat group member")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.MemberList = make([]SDChatGroupMember, count)
	for i := range v.MemberList {
		if err := v.MemberList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

// SDChatPeer preserves the optional avatar and tagged descriptor variant.
type SDChatPeer struct {
	ID            uint32
	Name          string
	AvatarPresent bool
	Avatar        uint64
	DespKind      int32
	PlayerVar     SDChatPeerPlayerVar
	GroupVar      SDChatPeerGroupVar
}

const (
	SDChatPeerDespNone int32 = iota
	SDChatPeerDespPlayer
	SDChatPeerDespGroup
)

func (v SDChatPeer) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.ID)
	writer.String(v.Name)

	writer.Bool(!v.AvatarPresent)
	if v.AvatarPresent {
		writer.Uint64(v.Avatar)
	}

	writer.Int32(v.DespKind)
	switch v.DespKind {
	case SDChatPeerDespNone:
	case SDChatPeerDespPlayer:
		if err := v.PlayerVar.MarshalPortable(writer); err != nil {
			return err
		}
	case SDChatPeerDespGroup:
		if err := v.GroupVar.MarshalPortable(writer); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid SDChatPeer descriptor variant %d", v.DespKind)
	}
	return writer.Err()
}

func (v *SDChatPeer) UnmarshalPortable(reader *PortableReader) error {
	v.ID = reader.Uint32()
	v.Name = reader.String()
	v.Avatar = 0
	v.AvatarPresent = !reader.Bool()
	if v.AvatarPresent {
		v.Avatar = reader.Uint64()
	}
	v.PlayerVar = SDChatPeerPlayerVar{}
	v.GroupVar = SDChatPeerGroupVar{}
	v.DespKind = reader.Int32()
	switch v.DespKind {
	case SDChatPeerDespNone:
	case SDChatPeerDespPlayer:
		if err := v.PlayerVar.UnmarshalPortable(reader); err != nil {
			return err
		}
	case SDChatPeerDespGroup:
		if err := v.GroupVar.UnmarshalPortable(reader); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid SDChatPeer descriptor variant %d", v.DespKind)
	}
	return reader.Err()
}

// SDChatPeerID stores the high 32-bit peer type and low 32-bit peer id.
type SDChatPeerID struct {
	Data uint64
}

func NewSDChatPeerID(peerType ChatPeerType, id uint32) SDChatPeerID {
	return SDChatPeerID{Data: uint64(uint32(peerType))<<32 | uint64(id)}
}

func (v SDChatPeerID) Type() ChatPeerType { return ChatPeerType(v.Data >> 32) }
func (v SDChatPeerID) ID() uint32         { return uint32(v.Data) }
func (v SDChatPeerID) Empty() bool        { return v.Data == 0 }

func (v SDChatPeerID) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.Data)
	return writer.Err()
}

func (v *SDChatPeerID) UnmarshalPortable(reader *PortableReader) error {
	v.Data = reader.Uint64()
	return reader.Err()
}
