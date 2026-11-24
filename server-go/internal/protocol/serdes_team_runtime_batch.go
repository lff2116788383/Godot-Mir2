package protocol

import "sort"

// SDRuntimeConfig is the serialized runtime configuration map.
type SDRuntimeConfig struct {
	Config map[string]string
}

func (v SDRuntimeConfig) MarshalPortable(writer *PortableWriter) error {
	keys := make([]string, 0, len(v.Config))
	for key := range v.Config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	writer.SizeTag(uint64(len(keys)))
	for _, key := range keys {
		writer.String(key)
		writer.String(v.Config[key])
	}
	return writer.Err()
}

func (v *SDRuntimeConfig) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "runtime config")
	if reader.Err() != nil {
		return reader.Err()
	}
	if count == 0 {
		v.Config = nil
	} else {
		v.Config = make(map[string]string, count)
	}
	for i := 0; i < count; i++ {
		key := reader.String()
		v.Config[key] = reader.String()
	}
	return reader.Err()
}

func (v SDRuntimeConfig) GetConfig(key string) (string, bool) {
	value, ok := v.Config[key]
	return value, ok
}

// SetConfig stores a non-empty value and treats an empty value as a reset.
func (v *SDRuntimeConfig) SetConfig(key, value string) bool {
	if value == "" {
		if _, ok := v.Config[key]; !ok {
			return false
		}
		delete(v.Config, key)
		return true
	}
	if v.Config == nil {
		v.Config = make(map[string]string)
	}
	if old, ok := v.Config[key]; ok && old == value {
		return false
	}
	v.Config[key] = value
	return true
}

type SDPlayerConfig struct {
	MagicKeyList  SDMagicKeyList
	RuntimeConfig SDRuntimeConfig
}

func (v SDPlayerConfig) MarshalPortable(writer *PortableWriter) error {
	if err := v.MagicKeyList.MarshalPortable(writer); err != nil {
		return err
	}
	return v.RuntimeConfig.MarshalPortable(writer)
}

func (v *SDPlayerConfig) UnmarshalPortable(reader *PortableReader) error {
	if err := v.MagicKeyList.UnmarshalPortable(reader); err != nil {
		return err
	}
	return v.RuntimeConfig.UnmarshalPortable(reader)
}

type SDBuffedAbility struct {
	DC        [2]SDTaggedValMap
	MC        [2]SDTaggedValMap
	SC        [2]SDTaggedValMap
	AC        [2]SDTaggedValMap
	MAC       [2]SDTaggedValMap
	DCHit     SDTaggedValMap
	MCHit     SDTaggedValMap
	DCDodge   SDTaggedValMap
	MCDodge   SDTaggedValMap
	Speed     SDTaggedValMap
	LuckCurse SDTaggedValMap
}

func (v SDBuffedAbility) MarshalPortable(writer *PortableWriter) error {
	values := []SDTaggedValMap{
		v.DC[0], v.DC[1], v.MC[0], v.MC[1], v.SC[0], v.SC[1],
		v.AC[0], v.AC[1], v.MAC[0], v.MAC[1],
		v.DCHit, v.MCHit, v.DCDodge, v.MCDodge, v.Speed, v.LuckCurse,
	}
	for _, value := range values {
		if err := value.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDBuffedAbility) UnmarshalPortable(reader *PortableReader) error {
	values := []*SDTaggedValMap{
		&v.DC[0], &v.DC[1], &v.MC[0], &v.MC[1], &v.SC[0], &v.SC[1],
		&v.AC[0], &v.AC[1], &v.MAC[0], &v.MAC[1],
		&v.DCHit, &v.MCHit, &v.DCDodge, &v.MCDodge, &v.Speed, &v.LuckCurse,
	}
	for _, value := range values {
		if err := value.UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

type SDHealth struct {
	UID       uint64
	HP        int32
	MP        int32
	MaxHP     int32
	MaxMP     int32
	HPRecover int32
	MPRecover int32

	BuffedMaxHP     SDTaggedValMap
	BuffedMaxMP     SDTaggedValMap
	BuffedHPRecover SDTaggedValMap
	BuffedMPRecover SDTaggedValMap
}

func (v SDHealth) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.Int32(v.HP)
	writer.Int32(v.MP)
	writer.Int32(v.MaxHP)
	writer.Int32(v.MaxMP)
	writer.Int32(v.HPRecover)
	writer.Int32(v.MPRecover)
	for _, value := range []SDTaggedValMap{v.BuffedMaxHP, v.BuffedMaxMP, v.BuffedHPRecover, v.BuffedMPRecover} {
		if err := value.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDHealth) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.HP = reader.Int32()
	v.MP = reader.Int32()
	v.MaxHP = reader.Int32()
	v.MaxMP = reader.Int32()
	v.HPRecover = reader.Int32()
	v.MPRecover = reader.Int32()
	for _, value := range []*SDTaggedValMap{&v.BuffedMaxHP, &v.BuffedMaxMP, &v.BuffedHPRecover, &v.BuffedMPRecover} {
		if err := value.UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

func (v SDHealth) Dead() bool { return v.HP <= 0 }

func (v SDHealth) GetMaxHP() int32 {
	value := v.MaxHP + v.BuffedMaxHP.Sum()
	if value < 0 {
		return 0
	}
	return value
}

func (v SDHealth) GetMaxMP() int32 {
	value := v.MaxMP + v.BuffedMaxMP.Sum()
	if value < 0 {
		return 0
	}
	return value
}

func (v SDHealth) GetHPRecover() int32 {
	return v.HPRecover + v.BuffedHPRecover.Sum()
}

func (v SDHealth) GetMPRecover() int32 {
	return v.MPRecover + v.BuffedMPRecover.Sum()
}

func clampHealth(value, maxValue int32) int32 {
	if value < 0 {
		return 0
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func (v *SDHealth) UpdateHealth(addHP, addMP, addMaxHP, addMaxMP int32) bool {
	oldHP, oldMP, oldMaxHP, oldMaxMP := v.HP, v.MP, v.MaxHP, v.MaxMP
	v.MaxHP += addMaxHP
	if v.MaxHP < 0 {
		v.MaxHP = 0
	}
	v.MaxMP += addMaxMP
	if v.MaxMP < 0 {
		v.MaxMP = 0
	}
	v.HP = clampHealth(v.HP+addHP, v.GetMaxHP())
	v.MP = clampHealth(v.MP+addMP, v.GetMaxMP())
	return v.HP != oldHP || v.MP != oldMP || v.MaxHP != oldMaxHP || v.MaxMP != oldMaxMP
}

type SDBuffIDList struct {
	UID    uint64
	IDList []uint32
}

func (v SDBuffIDList) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.SizeTag(uint64(len(v.IDList)))
	for _, id := range v.IDList {
		writer.Uint32(id)
	}
	return writer.Err()
}

func (v *SDBuffIDList) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	count := readPortableCount(reader, 1<<20, "buff id")
	if reader.Err() != nil {
		return reader.Err()
	}
	if count == 0 {
		v.IDList = nil
	} else {
		v.IDList = make([]uint32, count)
	}
	for i := range v.IDList {
		v.IDList[i] = reader.Uint32()
	}
	return reader.Err()
}

type SDRegisterQuest struct {
	Name string
}

func (v SDRegisterQuest) MarshalPortable(writer *PortableWriter) error {
	writer.String(v.Name)
	return writer.Err()
}

func (v *SDRegisterQuest) UnmarshalPortable(reader *PortableReader) error {
	v.Name = reader.String()
	return reader.Err()
}

type SDQueryQuestUID struct {
	Name string
}

func (v SDQueryQuestUID) MarshalPortable(writer *PortableWriter) error {
	writer.String(v.Name)
	return writer.Err()
}

func (v *SDQueryQuestUID) UnmarshalPortable(reader *PortableReader) error {
	v.Name = reader.String()
	return reader.Err()
}

type SDTeamJobReq struct {
	MinLevel    int32
	MaxLevel    int32
	MinCapacity int32
	MagicIDList []uint32
}

func (v SDTeamJobReq) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.MinLevel)
	writer.Int32(v.MaxLevel)
	writer.Int32(v.MinCapacity)
	ids := append([]uint32(nil), v.MagicIDList...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	writer.SizeTag(uint64(len(ids)))
	for _, id := range ids {
		writer.Uint32(id)
	}
	return writer.Err()
}

func (v *SDTeamJobReq) UnmarshalPortable(reader *PortableReader) error {
	v.MinLevel = reader.Int32()
	v.MaxLevel = reader.Int32()
	v.MinCapacity = reader.Int32()
	count := readPortableCount(reader, 1<<20, "team magic requirement")
	if reader.Err() != nil {
		return reader.Err()
	}
	if count == 0 {
		v.MagicIDList = nil
	} else {
		v.MagicIDList = make([]uint32, count)
	}
	for i := range v.MagicIDList {
		v.MagicIDList[i] = reader.Uint32()
	}
	sort.Slice(v.MagicIDList, func(i, j int) bool { return v.MagicIDList[i] < v.MagicIDList[j] })
	return reader.Err()
}

func (v SDTeamJobReq) Empty() bool {
	return v.MinLevel < 0 && v.MaxLevel < 0 && v.MinCapacity <= 0 && len(v.MagicIDList) == 0
}

func (v SDTeamJobReq) Valid() bool {
	const maxLevel int32 = 1024
	if v.MinLevel >= 0 && v.MinLevel > maxLevel {
		return false
	}
	if v.MaxLevel >= 0 && v.MaxLevel > maxLevel {
		return false
	}
	if v.MinLevel >= 0 && v.MaxLevel >= 0 && v.MinLevel > v.MaxLevel {
		return false
	}
	return v.MinCapacity >= 0
}

type OptionalTeamJobReq struct {
	Present bool
	Value   SDTeamJobReq
}

func (v OptionalTeamJobReq) MarshalPortable(writer *PortableWriter) error {
	writer.Bool(!v.Present)
	if v.Present {
		return v.Value.MarshalPortable(writer)
	}
	return writer.Err()
}

func (v *OptionalTeamJobReq) UnmarshalPortable(reader *PortableReader) error {
	v.Present = !reader.Bool()
	if v.Present {
		return v.Value.UnmarshalPortable(reader)
	}
	v.Value = SDTeamJobReq{}
	return reader.Err()
}

type SDTeamReq struct {
	MaxMember  int32
	AutoAccept bool
	JobReqList [3]OptionalTeamJobReq
}

func (v SDTeamReq) MarshalPortable(writer *PortableWriter) error {
	writer.Int32(v.MaxMember)
	writer.Bool(v.AutoAccept)
	for _, req := range v.JobReqList {
		if err := req.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return writer.Err()
}

func (v *SDTeamReq) UnmarshalPortable(reader *PortableReader) error {
	v.MaxMember = reader.Int32()
	v.AutoAccept = reader.Bool()
	for i := range v.JobReqList {
		if err := v.JobReqList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return reader.Err()
}

func (v SDTeamReq) MemberLimit() int32 {
	if v.MaxMember > 0 {
		return v.MaxMember
	}
	return 100
}

type SDTeamPlayer struct {
	UID   uint64
	Level uint32
	Job   int32
	Name  string
}

func (v SDTeamPlayer) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.UID)
	writer.Uint32(v.Level)
	writer.Int32(v.Job)
	writer.String(v.Name)
	return writer.Err()
}

func (v *SDTeamPlayer) UnmarshalPortable(reader *PortableReader) error {
	v.UID = reader.Uint64()
	v.Level = reader.Uint32()
	v.Job = reader.Int32()
	v.Name = reader.String()
	return reader.Err()
}

type SDTeamCandidate struct {
	Player SDTeamPlayer
}

func (v SDTeamCandidate) MarshalPortable(writer *PortableWriter) error {
	return v.Player.MarshalPortable(writer)
}

func (v *SDTeamCandidate) UnmarshalPortable(reader *PortableReader) error {
	return v.Player.UnmarshalPortable(reader)
}

type SDRequestJoinTeam struct {
	Player      SDTeamPlayer
	Job         int32
	Stat        int32
	MagicIDList []uint32
}

func (v SDRequestJoinTeam) MarshalPortable(writer *PortableWriter) error {
	if err := v.Player.MarshalPortable(writer); err != nil {
		return err
	}
	writer.Int32(v.Job)
	writer.Int32(v.Stat)
	writer.SizeTag(uint64(len(v.MagicIDList)))
	for _, id := range v.MagicIDList {
		writer.Uint32(id)
	}
	return writer.Err()
}

func (v *SDRequestJoinTeam) UnmarshalPortable(reader *PortableReader) error {
	if err := v.Player.UnmarshalPortable(reader); err != nil {
		return err
	}
	v.Job = reader.Int32()
	v.Stat = reader.Int32()
	count := readPortableCount(reader, 1<<20, "team request magic")
	if reader.Err() != nil {
		return reader.Err()
	}
	if count == 0 {
		v.MagicIDList = nil
	} else {
		v.MagicIDList = make([]uint32, count)
	}
	for i := range v.MagicIDList {
		v.MagicIDList[i] = reader.Uint32()
	}
	return reader.Err()
}

type SDTeamMemberList struct {
	TeamLeader uint64
	MemberList []SDTeamPlayer
	Req        SDTeamReq
}

func (v SDTeamMemberList) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.TeamLeader)
	writer.SizeTag(uint64(len(v.MemberList)))
	for _, member := range v.MemberList {
		if err := member.MarshalPortable(writer); err != nil {
			return err
		}
	}
	return v.Req.MarshalPortable(writer)
}

func (v *SDTeamMemberList) UnmarshalPortable(reader *PortableReader) error {
	v.TeamLeader = reader.Uint64()
	count := readPortableCount(reader, 1<<20, "team member")
	if reader.Err() != nil {
		return reader.Err()
	}
	if count == 0 {
		v.MemberList = nil
	} else {
		v.MemberList = make([]SDTeamPlayer, count)
	}
	for i := range v.MemberList {
		if err := v.MemberList[i].UnmarshalPortable(reader); err != nil {
			return err
		}
	}
	return v.Req.UnmarshalPortable(reader)
}

func (v SDTeamMemberList) HasMember(uid uint64) bool {
	if v.TeamLeader == 0 {
		return false
	}
	for _, member := range v.MemberList {
		if member.UID == uid {
			return true
		}
	}
	return false
}

func (v SDTeamMemberList) GetUIDList() []uint64 {
	if v.TeamLeader == 0 {
		return nil
	}
	uids := make([]uint64, len(v.MemberList))
	for i, member := range v.MemberList {
		uids[i] = member.UID
	}
	return uids
}

func (v *SDTeamMemberList) Clear() { *v = SDTeamMemberList{} }

type SDTransferTeamLeader struct {
	MemberList []uint64
	Req        SDTeamReq
}

func (v SDTransferTeamLeader) MarshalPortable(writer *PortableWriter) error {
	writer.SizeTag(uint64(len(v.MemberList)))
	for _, uid := range v.MemberList {
		writer.Uint64(uid)
	}
	return v.Req.MarshalPortable(writer)
}

func (v *SDTransferTeamLeader) UnmarshalPortable(reader *PortableReader) error {
	count := readPortableCount(reader, 1<<20, "transfer leader member")
	if reader.Err() != nil {
		return reader.Err()
	}
	if count == 0 {
		v.MemberList = nil
	} else {
		v.MemberList = make([]uint64, count)
	}
	for i := range v.MemberList {
		v.MemberList[i] = reader.Uint64()
	}
	return v.Req.UnmarshalPortable(reader)
}

type SDTeamInfo struct {
	Leader      uint64
	Name        string
	Job         int32
	Level       int32
	MemberCount int32
	Req         SDTeamReq
}

func (v SDTeamInfo) MarshalPortable(writer *PortableWriter) error {
	writer.Uint64(v.Leader)
	writer.String(v.Name)
	writer.Int32(v.Job)
	writer.Int32(v.Level)
	writer.Int32(v.MemberCount)
	return v.Req.MarshalPortable(writer)
}

func (v *SDTeamInfo) UnmarshalPortable(reader *PortableReader) error {
	v.Leader = reader.Uint64()
	v.Name = reader.String()
	v.Job = reader.Int32()
	v.Level = reader.Int32()
	v.MemberCount = reader.Int32()
	return v.Req.UnmarshalPortable(reader)
}
