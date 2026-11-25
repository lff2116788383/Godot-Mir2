package protocol

import "sort"

// SDStartInvOp requests the start of an inventory operation.
type SDStartInvOp struct {
	InvOp     int32
	UID       uint64
	QueryTag  string
	CommitTag string
	TypeList  []string
}

func (v SDStartInvOp) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.InvOp)
	writer.Uint64(v.UID)
	writer.String(v.QueryTag)
	writer.String(v.CommitTag)
	writer.SizeTag(uint64(len(v.TypeList)))
	for _, itemType := range v.TypeList {
		writer.String(itemType)
	}
	return writer.Err()
}

func (v *SDStartInvOp) UnmarshalPortable(reader *PortableReader) error {
	v.InvOp = reader.Int32()
	v.UID = reader.Uint64()
	v.QueryTag = reader.String()
	v.CommitTag = reader.String()
	count := readPortableCount(reader, 1<<20, "inventory type")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.TypeList = make([]string, count)
	for i := range v.TypeList {
		v.TypeList[i] = reader.String()
	}
	return reader.Err()
}

type SDStartInput struct {
	UID       uint64
	Title     string
	CommitTag string
	Security  bool
}

func (v SDStartInput) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.String(v.Title)
	writer.String(v.CommitTag)
	writer.Bool(v.Security)
	return writer.Err()
}

func (v *SDStartInput) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.Title = reader.String()
	v.CommitTag = reader.String()
	v.Security = reader.Bool()
	return reader.Err()
}

type SDNPCSell struct {
	NPCUID   uint64
	ItemList []uint32
}

func (v SDNPCSell) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.NPCUID)
	writer.SizeTag(uint64(len(v.ItemList)))
	for _, itemID := range v.ItemList {
		writer.Uint32(itemID)
	}
	return writer.Err()
}

func (v *SDNPCSell) UnmarshalPortable(reader *PortableReader) error {
	v.NPCUID = reader.Uint64()
	count := readPortableCount(reader, 1<<20, "NPC sell item")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemList = make([]uint32, count)
	for i := range v.ItemList {
		v.ItemList[i] = reader.Uint32()
	}
	return reader.Err()
}

type SDCostItem struct {
	ItemID uint32
	Count  uint64
}

func (v SDCostItem) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.ItemID)
	writer.Uint64(v.Count)
	return writer.Err()
}

func (v *SDCostItem) UnmarshalPortable(reader *PortableReader) error {
	v.ItemID = reader.Uint32()
	v.Count = reader.Uint64()
	return reader.Err()
}

type SDPlayerName struct {
	UID       uint64
	Name      string
	NameColor uint32
}

func (v SDPlayerName) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.String(v.Name)
	writer.Uint32(v.NameColor)
	return writer.Err()
}

func (v *SDPlayerName) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.Name = reader.String()
	v.NameColor = reader.Uint32()
	return reader.Err()
}

type SDNPCName struct {
	UID  uint64
	Name string
}

func (v SDNPCName) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.String(v.Name)
	return writer.Err()
}

func (v *SDNPCName) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.Name = reader.String()
	return reader.Err()
}

type SDPlayerGuild struct {
	DBID      uint32
	GuildID   uint32
	GuildName string
	Title     string
}

func (v SDPlayerGuild) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.DBID)
	writer.Uint32(v.GuildID)
	writer.String(v.GuildName)
	writer.String(v.Title)
	return writer.Err()
}

func (v *SDPlayerGuild) UnmarshalPortable(reader *PortableReader) error {
	v.DBID = reader.Uint32()
	v.GuildID = reader.Uint32()
	v.GuildName = reader.String()
	v.Title = reader.String()
	return reader.Err()
}

type SDGuildWar struct {
	GuildID   uint32
	GuildName string
	EndTime   uint64
}

func (v SDGuildWar) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.GuildID)
	writer.String(v.GuildName)
	writer.Uint64(v.EndTime)
	return writer.Err()
}

func (v *SDGuildWar) UnmarshalPortable(reader *PortableReader) error {
	v.GuildID = reader.Uint32()
	v.GuildName = reader.String()
	v.EndTime = reader.Uint64()
	return reader.Err()
}

type SDGuildWarList struct {
	GuildID uint32
	WarList []SDGuildWar
}

func (v SDGuildWarList) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.GuildID)
	writer.SizeTag(uint64(len(v.WarList)))
	for _, war := range v.WarList {
		if err := war.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDGuildWarList) UnmarshalPortable(reader *PortableReader) error {
	v.GuildID = reader.Uint32()
	count := readPortableCount(reader, 1<<20, "guild war")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.WarList = make([]SDGuildWar, count)
	for i := range v.WarList {
		if err := v.WarList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDGuildMember struct {
	DBID   uint32
	Name   string
	Rank   int32
	Title  string
	Online bool
}

func (v SDGuildMember) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.DBID)
	writer.String(v.Name)
	writer.Int32(v.Rank)
	writer.String(v.Title)
	writer.Bool(v.Online)
	return writer.Err()
}

func (v *SDGuildMember) UnmarshalPortable(reader *PortableReader) error {
	v.DBID = reader.Uint32()
	v.Name = reader.String()
	v.Rank = reader.Int32()
	v.Title = reader.String()
	v.Online = reader.Bool()
	return reader.Err()
}

type SDGuildInfo struct {
	GuildID    uint32
	Name       string
	Notice     string
	MemberList []SDGuildMember
}

func (v SDGuildInfo) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.GuildID)
	writer.String(v.Name)
	writer.String(v.Notice)
	writer.SizeTag(uint64(len(v.MemberList)))
	for _, member := range v.MemberList {
		if err := member.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDGuildInfo) UnmarshalPortable(reader *PortableReader) error {
	v.GuildID = reader.Uint32()
	v.Name = reader.String()
	v.Notice = reader.String()
	count := readPortableCount(reader, 1<<20, "guild member")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.MemberList = make([]SDGuildMember, count)
	for i := range v.MemberList {
		if err := v.MemberList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDGridItemIDList struct {
	X          int32
	Y          int32
	ItemIDList []uint32
}

func (v SDGridItemIDList) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.X)
	writer.Int32(v.Y)
	writer.SizeTag(uint64(len(v.ItemIDList)))
	for _, itemID := range v.ItemIDList {
		writer.Uint32(itemID)
	}
	return writer.Err()
}

func (v *SDGridItemIDList) UnmarshalPortable(reader *PortableReader) error {
	v.X = reader.Int32()
	v.Y = reader.Int32()
	count := readPortableCount(reader, 1<<20, "grid item id")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.ItemIDList = make([]uint32, count)
	for i := range v.ItemIDList {
		v.ItemIDList[i] = reader.Uint32()
	}
	return reader.Err()
}

type SDGroundItemIDList struct {
	MapUID         uint64
	GridItemIDList []SDGridItemIDList
}

func (v SDGroundItemIDList) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.MapUID)
	writer.SizeTag(uint64(len(v.GridItemIDList)))
	for _, grid := range v.GridItemIDList {
		if err := grid.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDGroundItemIDList) UnmarshalPortable(reader *PortableReader) error {
	v.MapUID = reader.Uint64()
	count := readPortableCount(reader, 1<<20, "ground item grid")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.GridItemIDList = make([]SDGridItemIDList, count)
	for i := range v.GridItemIDList {
		if err := v.GridItemIDList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDGridFireWall struct {
	X     int32
	Y     int32
	Count int32
}

func (v SDGridFireWall) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.X)
	writer.Int32(v.Y)
	writer.Int32(v.Count)
	return writer.Err()
}

func (v *SDGridFireWall) UnmarshalPortable(reader *PortableReader) error {
	v.X = reader.Int32()
	v.Y = reader.Int32()
	v.Count = reader.Int32()
	return reader.Err()
}

type SDGroundFireWallList struct {
	MapUID       uint64
	FireWallList []SDGridFireWall
}

func (v SDGroundFireWallList) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.MapUID)
	writer.SizeTag(uint64(len(v.FireWallList)))
	for _, wall := range v.FireWallList {
		if err := wall.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDGroundFireWallList) UnmarshalPortable(reader *PortableReader) error {
	v.MapUID = reader.Uint64()
	count := readPortableCount(reader, 1<<20, "ground firewall")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.FireWallList = make([]SDGridFireWall, count)
	for i := range v.FireWallList {
		if err := v.FireWallList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDSafeZone struct {
	RectList   [][4]int32
	CircleList [][3]int32
}

func (v SDSafeZone) MarshalPortable(writer *PortableWriter) error {
	writer.SizeTag(uint64(len(v.RectList)))
	for _, rect := range v.RectList {
		for _, value := range rect {
			writer.Int32(value)
		}
	}
	writer.SizeTag(uint64(len(v.CircleList)))
	for _, circle := range v.CircleList {
		for _, value := range circle {
			writer.Int32(value)
		}
	}
	return writer.Err()
}

func (v *SDSafeZone) UnmarshalPortable(reader *PortableReader) error {
	rectCount := readPortableCount(reader, 1<<20, "safe-zone rectangle")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.RectList = make([][4]int32, rectCount)
	for i := range v.RectList {
		for j := range v.RectList[i] {
			v.RectList[i][j] = reader.Int32()
		}
	}
	circleCount := readPortableCount(reader, 1<<20, "safe-zone circle")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.CircleList = make([][3]int32, circleCount)
	for i := range v.CircleList {
		for j := range v.CircleList[i] {
			v.CircleList[i][j] = reader.Int32()
		}
	}
	return reader.Err()
}

func (v SDSafeZone) Empty() bool {
	return len(v.RectList) == 0 && len(v.CircleList) == 0
}

type SDSafeZoneList struct {
	MapUID uint64
	Zone   SDSafeZone
}

func (v SDSafeZoneList) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.MapUID)
	if err := v.Zone.MarshalPortable(writer); err != nil {
		return err
	}
	return writer.Err()
}

func (v *SDSafeZoneList) UnmarshalPortable(reader *PortableReader) error {
	v.MapUID = reader.Uint64()
	if err := v.Zone.UnmarshalPortable(reader); err != nil {
		return err
	}
	return reader.Err()
}

type SDLearnedMagic struct {
	MagicID uint32
	Exp     int32
}

func (v SDLearnedMagic) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.MagicID)
	writer.Int32(v.Exp)
	return writer.Err()
}

func (v *SDLearnedMagic) UnmarshalPortable(reader *PortableReader) error {
	v.MagicID = reader.Uint32()
	v.Exp = reader.Int32()
	return reader.Err()
}

// Level is the current legacy placeholder implementation.
func (SDLearnedMagic) Level() int { return 1 }

type SDLearnedMagicList struct {
	MagicList []SDLearnedMagic
}

func (v SDLearnedMagicList) MarshalPortable(writer *PortableWriter) error {
	writer.SizeTag(uint64(len(v.MagicList)))
	for _, magic := range v.MagicList {
		if err := magic.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDLearnedMagicList) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "learned magic")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.MagicList = make([]SDLearnedMagic, count)
	for i := range v.MagicList {
		if err := v.MagicList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

func (v SDLearnedMagicList) Has(magicID uint32) bool {
	for _, magic := range v.MagicList {
		if magic.MagicID == magicID {
			return true
		}
	}
	return false
}

type SDMagicKeyList struct {
	KeyList map[uint32]uint8
}

func (v SDMagicKeyList) MarshalPortable(writer *PortableWriter) error {
	keys := make([]uint32, 0, len(v.KeyList))
	for key := range v.KeyList {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	writer.SizeTag(uint64(len(keys)))
	for _, key := range keys {
		writer.Uint32(key)
		writer.Uint8(v.KeyList[key])
	}
	return writer.Err()
}

func (v *SDMagicKeyList) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "magic key")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.KeyList = make(map[uint32]uint8, count)
	for i := 0; i < count; i++ {
		key := reader.Uint32()
		v.KeyList[key] = reader.Uint8()
	}
	return reader.Err()
}

func (SDMagicKeyList) ValidKey(key uint8) bool {
	return (key >= 'a' && key <= 'z') || key == '0' || (key >= '7' && key <= '9')
}

type SDAddFriendNotif struct {
	Notif int32
}

func (v SDAddFriendNotif) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.Notif)
	return writer.Err()
}

func (v *SDAddFriendNotif) UnmarshalPortable(reader *PortableReader) error {
	v.Notif = reader.Int32()
	return reader.Err()
}

type SDBlockPlayerNotif struct {
	Notif int32
}

func (v SDBlockPlayerNotif) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.Notif)
	return writer.Err()
}

func (v *SDBlockPlayerNotif) UnmarshalPortable(reader *PortableReader) error {
	v.Notif = reader.Int32()
	return reader.Err()
}

type SDChatMessageDBSeq struct {
	ID        uint64
	Timestamp uint64
}

func (v SDChatMessageDBSeq) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.ID)
	writer.Uint64(v.Timestamp)
	return writer.Err()
}

func (v *SDChatMessageDBSeq) UnmarshalPortable(reader *PortableReader) error {
	v.ID = reader.Uint64()
	v.Timestamp = reader.Uint64()
	return reader.Err()
}

type SDChatMessage struct {
	HasSeq   bool
	Seq      SDChatMessageDBSeq
	HasRefer bool
	Refer    uint64
	From     SDChatPeerID
	To       SDChatPeerID
	Message  string
}

func (v SDChatMessage) MarshalPortable(writer *PortableWriter) error {
	writer.Bool(!v.HasSeq)
	if v.HasSeq {
		if err := v.Seq.MarshalPortable(writer); err != nil {
			return err
		}
	}
	writer.Bool(!v.HasRefer)
	if v.HasRefer {
		writer.Uint64(v.Refer)
	}
	if err := v.From.MarshalPortable(writer); err != nil {
		return err
	}
	if err := v.To.MarshalPortable(writer); err != nil {
		return err
	}
	writer.String(v.Message)
	return writer.Err()
}

func (v *SDChatMessage) UnmarshalPortable(reader *PortableReader) error {
	v.Seq = SDChatMessageDBSeq{}
	v.Refer = 0
	v.HasSeq = !reader.Bool()
	if v.HasSeq {
		if err := v.Seq.UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	v.HasRefer = !reader.Bool()
	if v.HasRefer {
		v.Refer = reader.Uint64()
	}
	if err := v.From.UnmarshalPortable(reader); err != nil {
		return err
	}
	if err := v.To.UnmarshalPortable(reader); err != nil {
		return err
	}
	v.Message = reader.String()
	return reader.Err()
}

type SDChatMessageList []SDChatMessage

func (v SDChatMessageList) MarshalPortable(writer *PortableWriter) error {
	writer.SizeTag(uint64(len(v)))
	for _, message := range v {
		if err := message.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDChatMessageList) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "chat message")
	if reader.Err() != nil {
		return reader.Err()
	}
	*v = make(SDChatMessageList, count)
	for i := range *v {
		if err := (*v)[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDNPCEvent struct {
	X        int32
	Y        int32
	MapUID   uint64
	Path     string
	Event    string
	HasValue bool
	Value    string
}

func (v SDNPCEvent) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.X)
	writer.Int32(v.Y)
	writer.Uint64(v.MapUID)
	writer.String(v.Path)
	writer.String(v.Event)
	writer.Bool(!v.HasValue)
	if v.HasValue {
		writer.String(v.Value)
	}
	return writer.Err()
}

func (v *SDNPCEvent) UnmarshalPortable(reader *PortableReader) error {
	v.X = reader.Int32()
	v.Y = reader.Int32()
	v.MapUID = reader.Uint64()
	v.Path = reader.String()
	v.Event = reader.String()
	v.HasValue = !reader.Bool()
	if v.HasValue {
		v.Value = reader.String()
	} else {
		v.Value = ""
	}
	return reader.Err()
}

type SDTaggedValMap struct {
	ValMap map[int32]int32
}

func (v SDTaggedValMap) MarshalPortable(writer *PortableWriter) error {
	keys := make([]int32, 0, len(v.ValMap))
	for key := range v.ValMap {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	writer.SizeTag(uint64(len(keys)))
	for _, key := range keys {
		writer.Int32(key)
		writer.Int32(v.ValMap[key])
	}
	return writer.Err()
}

func (v *SDTaggedValMap) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "tagged value")
	if reader.Err() != nil {
		return reader.Err()
	}
	if count == 0 {
		v.ValMap = nil
	} else {
		v.ValMap = make(map[int32]int32, count)
	}
	for i := 0; i < count; i++ {
		key := reader.Int32()
		v.ValMap[key] = reader.Int32()
	}
	return reader.Err()
}

func (v *SDTaggedValMap) Add(value int32) int32 {
	if v.ValMap == nil {
		v.ValMap = make(map[int32]int32)
	}
	if len(v.ValMap) == 0 {
		v.ValMap[1] = value
		return 1
	}
	var maxTag int32
	for tag := range v.ValMap {
		if tag > maxTag {
			maxTag = tag
		}
	}
	maxTag++
	v.ValMap[maxTag] = value
	return maxTag
}

func (v SDTaggedValMap) Sum() int32 {
	var sum int32
	for _, value := range v.ValMap {
		sum += value
	}
	return sum
}

type SDPeerConfig struct {
	DropRate float64
	GoldRate float64
}

func (v SDPeerConfig) MarshalPortable(writer *PortableWriter) error {
	writer.Float64(v.DropRate)
	writer.Float64(v.GoldRate)
	return writer.Err()
}

func (v *SDPeerConfig) UnmarshalPortable(reader *PortableReader) error {
	v.DropRate = reader.Float64()
	v.GoldRate = reader.Float64()
	return reader.Err()
}

type SDSysSlavePeerPort struct {
	Port int32
}

func (v SDSysSlavePeerPort) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.Port)
	return writer.Err()
}

func (v *SDSysSlavePeerPort) UnmarshalPortable(reader *PortableReader) error {
	v.Port = reader.Int32()
	return reader.Err()
}

type SDSysSlavePeerAddress struct {
	Host string
	Port uint32
}

type SDSysSlavePeerList struct {
	List map[uint64]SDSysSlavePeerAddress
}

func (v SDSysSlavePeerList) MarshalPortable(writer *PortableWriter) error {
	keys := make([]uint64, 0, len(v.List))
	for key := range v.List {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	writer.SizeTag(uint64(len(keys)))
	for _, key := range keys {
		writer.Uint64(key)
		address := v.List[key]
		writer.String(address.Host)
		writer.Uint32(address.Port)
	}
	return writer.Err()
}

func (v *SDSysSlavePeerList) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "slave peer")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.List = make(map[uint64]SDSysSlavePeerAddress, count)
	for i := 0; i < count; i++ {
		key := reader.Uint64()
		address := SDSysSlavePeerAddress{Host: reader.String(), Port: reader.Uint32()}
		v.List[key] = address
	}
	return reader.Err()
}

type SDSysPeerIndex struct {
	Index        uint64
	MasterConfig string
}

func (v SDSysPeerIndex) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.Index)
	writer.String(v.MasterConfig)
	return writer.Err()
}

func (v *SDSysPeerIndex) UnmarshalPortable(reader *PortableReader) error {
	v.Index = reader.Uint64()
	v.MasterConfig = reader.String()
	return reader.Err()
}

type SDQuestDespUpdate struct {
	Name    string
	FSM     string
	HasDesp bool
	Desp    string
}

func (v SDQuestDespUpdate) MarshalPortable(writer *PortableWriter) error {
	writer.String(v.Name)
	writer.String(v.FSM)
	writer.Bool(!v.HasDesp)
	if v.HasDesp {
		writer.String(v.Desp)
	}
	return writer.Err()
}

func (v *SDQuestDespUpdate) UnmarshalPortable(reader *PortableReader) error {
	v.Name = reader.String()
	v.FSM = reader.String()
	v.HasDesp = !reader.Bool()
	if v.HasDesp {
		v.Desp = reader.String()
	} else {
		v.Desp = ""
	}
	return reader.Err()
}

type SDRankingEntry struct {
	DBID  uint32
	Level uint32
	Gold  uint64
	Name  string
}

func (v SDRankingEntry) MarshalPortable(writer *PortableWriter) error {
	writer.Uint32(v.DBID)
	writer.Uint32(v.Level)
	writer.Uint64(v.Gold)
	writer.String(v.Name)
	return writer.Err()
}

func (v *SDRankingEntry) UnmarshalPortable(reader *PortableReader) error {
	v.DBID = reader.Uint32()
	v.Level = reader.Uint32()
	v.Gold = reader.Uint64()
	v.Name = reader.String()
	return reader.Err()
}

type SDRankingList struct {
	Type    uint8
	Entries []SDRankingEntry
}

func (v SDRankingList) MarshalPortable(writer *PortableWriter) error {
	writer.Uint8(v.Type)
	writer.SizeTag(uint64(len(v.Entries)))
	for _, entry := range v.Entries {
		if err := entry.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDRankingList) UnmarshalPortable(reader *PortableReader) error {
	v.Type = reader.Uint8()
	count := readPortableCount(reader, 1<<20, "ranking entry")
	if reader.Err() != nil {
		return reader.Err()
	}
	v.Entries = make([]SDRankingEntry, count)
	for i := range v.Entries {
		if err := v.Entries[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDChatPeerList []SDChatPeer

func (v SDChatPeerList) MarshalPortable(writer *PortableWriter) error {
	writer.SizeTag(uint64(len(v)))
	for _, peer := range v {
		if err := peer.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDChatPeerList) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "chat peer")
	if reader.Err() != nil {
		return reader.Err()
	}
	*v = make(SDChatPeerList, count)
	for i := range *v {
		if err := (*v)[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDUIDList []uint64

func (v SDUIDList) MarshalPortable(writer *PortableWriter) error {
	writer.SizeTag(uint64(len(v)))
	for _, uid := range v {
		writer.Uint64(uid)
	}
	return writer.Err()
}

func (v *SDUIDList) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "uid")
	if reader.Err() != nil {
		return reader.Err()
	}
	*v = make(SDUIDList, count)
	for i := range *v {
		(*v)[i] = reader.Uint64()
	}
	return reader.Err()
}

