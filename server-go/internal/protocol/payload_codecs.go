package protocol

import (
	"encoding/binary"
	"fmt"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
)

const (
	CMChatMessageHeaderWireSize        = 16
	CMRequestLatestChatMessageWireSize = 1036
	SMOnlineOKWireSize                 = 177
	SMCORecordWireSize                 = 49
)

// AppendWire encodes the ABI-sensitive chat message header.
func (h CMChatMessageHeader) AppendWire(dst []byte) ([]byte, error) {
	start := len(dst)
	dst = append(dst, make([]byte, CMChatMessageHeaderWireSize)...)
	binary.LittleEndian.PutUint64(dst[start:start+8], h.ToCPID)

	var flags uint64
	if h.HasRef {
		flags |= 1
	}
	flags |= (h.RefID & 0x7fffffffffffffff) << 1
	binary.LittleEndian.PutUint64(dst[start+8:start+16], flags)
	return dst, nil
}

// ReadWire decodes the ABI-sensitive chat message header.
func (h *CMChatMessageHeader) ReadWire(src []byte) (int, error) {
	if len(src) < CMChatMessageHeaderWireSize {
		return 0, fmt.Errorf("CMChatMessageHeader needs %d bytes, got %d", CMChatMessageHeaderWireSize, len(src))
	}
	h.ToCPID = binary.LittleEndian.Uint64(src[:8])
	flags := binary.LittleEndian.Uint64(src[8:16])
	h.HasRef = flags&1 != 0
	h.RefID = (flags >> 1) & 0x7fffffffffffffff
	return CMChatMessageHeaderWireSize, nil
}

// AppendWire encodes the packed static vector and bitfield tail.
func (m CMRequestLatestChatMessage) AppendWire(dst []byte) ([]byte, error) {
	if m.LimitCount >= 1<<30 {
		return nil, fmt.Errorf("limit count %d exceeds 30-bit field", m.LimitCount)
	}
	var err error
	dst, err = m.CPIDList.AppendWire(dst)
	if err != nil {
		return nil, err
	}
	var flags uint32
	flags |= m.LimitCount & ((1 << 30) - 1)
	if m.IncludeSend {
		flags |= 1 << 30
	}
	if m.IncludeRecv {
		flags |= 1 << 31
	}
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], flags)
	return append(dst, raw[:]...), nil
}

// ReadWire decodes the packed static vector and bitfield tail.
func (m *CMRequestLatestChatMessage) ReadWire(src []byte) (int, error) {
	if len(src) < CMRequestLatestChatMessageWireSize {
		return 0, fmt.Errorf("CMRequestLatestChatMessage needs %d bytes, got %d", CMRequestLatestChatMessageWireSize, len(src))
	}
	n, err := m.CPIDList.ReadWire(src)
	if err != nil {
		return 0, err
	}
	flags := binary.LittleEndian.Uint32(src[n : n+4])
	m.LimitCount = flags & ((1 << 30) - 1)
	m.IncludeSend = flags&(1<<30) != 0
	m.IncludeRecv = flags&(1<<31) != 0
	return n + 4, nil
}

// AppendWire encodes the online payload using the observed MS bitfield layout.
func (m SMOnlineOK) AppendWire(dst []byte) ([]byte, error) {
	start := len(dst)
	dst = append(dst, make([]byte, SMOnlineOKWireSize)...)
	binary.LittleEndian.PutUint64(dst[start:start+8], m.UID)

	nameDst, err := m.Name.AppendWire(nil)
	if err != nil {
		return nil, err
	}
	if len(nameDst) != 132 {
		return nil, fmt.Errorf("SMOnlineOK name encoded to %d bytes, want 132", len(nameDst))
	}
	copy(dst[start+8:start+140], nameDst)
	var flags uint8
	if m.Gender {
		flags |= 1
	}
	flags |= (m.Job & 0x7) << 1
	dst[start+140] = flags
	binary.LittleEndian.PutUint64(dst[start+141:start+149], m.MapUID)

	action, err := m.Action.AppendWire(nil)
	if err != nil {
		return nil, err
	}
	copy(dst[start+149:start+177], action)
	return dst, nil
}

// ReadWire decodes the online payload.
func (m *SMOnlineOK) ReadWire(src []byte) (int, error) {
	if len(src) < SMOnlineOKWireSize {
		return 0, fmt.Errorf("SMOnlineOK needs %d bytes, got %d", SMOnlineOKWireSize, len(src))
	}
	m.UID = binary.LittleEndian.Uint64(src[:8])
	n, err := m.Name.ReadWire(src[8:])
	if err != nil {
		return 0, err
	}
	flags := src[8+n]
	m.Gender = flags&1 != 0
	m.Job = (flags >> 1) & 0x7
	m.MapUID = binary.LittleEndian.Uint64(src[141:149])

	action, err := common.ParseActionNode(src[149:177])
	if err != nil {
		return 0, err
	}
	m.Action = action
	return SMOnlineOKWireSize, nil
}

// MonsterID decodes the monster arm of the SMCORecord union.
func (r SMCORecord) MonsterID() uint32 {
	return binary.LittleEndian.Uint32(r.Union[:4])
}

// SetMonsterID writes the monster arm of the SMCORecord union.
func (r *SMCORecord) SetMonsterID(monsterID uint32) {
	clear(r.Union[:])
	binary.LittleEndian.PutUint32(r.Union[:4], monsterID)
}

// NPCID decodes the NPC arm of the SMCORecord union.
func (r SMCORecord) NPCID() uint32 {
	return binary.LittleEndian.Uint32(r.Union[:4])
}

// SetNPCID writes the NPC arm of the SMCORecord union.
func (r *SMCORecord) SetNPCID(npcID uint32) {
	clear(r.Union[:])
	binary.LittleEndian.PutUint32(r.Union[:4], npcID)
}

// PlayerGender decodes the player bitfield arm.
func (r SMCORecord) PlayerGender() bool {
	return r.Union[0]&1 != 0
}

// PlayerJob decodes the player bitfield arm.
func (r SMCORecord) PlayerJob() uint8 {
	return (r.Union[0] >> 1) & 0x7
}

// PlayerTeamLeader decodes the player bitfield arm.
func (r SMCORecord) PlayerTeamLeader() bool {
	return r.Union[0]&(1<<4) != 0
}

// PlayerStall decodes the player bitfield arm.
func (r SMCORecord) PlayerStall() bool {
	return r.Union[0]&(1<<5) != 0
}

// PlayerLevel decodes the player level following the bitfield byte.
func (r SMCORecord) PlayerLevel() uint32 {
	return binary.LittleEndian.Uint32(r.Union[1:5])
}

// SetPlayer writes the player arm of the SMCORecord union.
func (r *SMCORecord) SetPlayer(gender bool, job uint8, teamLeader, stall bool, level uint32) {
	clear(r.Union[:])
	var flags uint8
	if gender {
		flags |= 1
	}
	flags |= (job & 0x7) << 1
	if teamLeader {
		flags |= 1 << 4
	}
	if stall {
		flags |= 1 << 5
	}
	r.Union[0] = flags
	binary.LittleEndian.PutUint32(r.Union[1:5], level)
}

// AppendWire encodes the fixed record and the five raw union bytes.
func (r SMCORecord) AppendWire(dst []byte) ([]byte, error) {
	start := len(dst)
	dst = append(dst, make([]byte, SMCORecordWireSize)...)
	binary.LittleEndian.PutUint64(dst[start:start+8], r.UID)
	binary.LittleEndian.PutUint64(dst[start+8:start+16], r.MapUID)
	action, err := r.Action.AppendWire(nil)
	if err != nil {
		return nil, err
	}
	copy(dst[start+16:start+44], action)
	copy(dst[start+44:start+49], r.Union[:])
	return dst, nil
}

// ReadWire decodes the fixed record and the five raw union bytes.
func (r *SMCORecord) ReadWire(src []byte) (int, error) {
	if len(src) < SMCORecordWireSize {
		return 0, fmt.Errorf("SMCORecord needs %d bytes, got %d", SMCORecordWireSize, len(src))
	}
	r.UID = binary.LittleEndian.Uint64(src[:8])
	r.MapUID = binary.LittleEndian.Uint64(src[8:16])
	action, err := common.ParseActionNode(src[16:44])
	if err != nil {
		return 0, err
	}
	r.Action = action
	copy(r.Union[:], src[44:49])
	return SMCORecordWireSize, nil
}
