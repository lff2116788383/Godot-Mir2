package common

import (
	"encoding/binary"
	"fmt"
)

const (
	// DirNone is DIR_NONE from protocoldef.hpp.
	DirNone = 0

	// DefaultSpeed is SYS_DEFSPEED from sysconst.hpp.
	DefaultSpeed = 100

	// ActionNodeWireSize is sizeof(ActionNode) with the project's packed x64 ABI.
	ActionNodeWireSize = 28
)

// ActionType values are kept identical to the legacy ActionType enum.
type ActionType int

const (
	ActionNone ActionType = iota
	ActionSpawn
	ActionStand
	ActionMove
	ActionJump
	ActionPushMove
	ActionSpaceMove
	ActionAttack
	ActionPickUp
	ActionSpell
	ActionTransf
	ActionHitted
	ActionSpinKick
	ActionDie
	ActionMine
)

func (a ActionType) Valid() bool {
	return a >= ActionSpawn && a <= ActionMine
}

func (a ActionType) String() string {
	switch a {
	case ActionNone:
		return "ACTION_NONE"
	case ActionSpawn:
		return "ACTION_SPAWN"
	case ActionStand:
		return "ACTION_STAND"
	case ActionMove:
		return "ACTION_MOVE"
	case ActionJump:
		return "ACTION_JUMP"
	case ActionPushMove:
		return "ACTION_PUSHMOVE"
	case ActionSpaceMove:
		return "ACTION_SPACEMOVE"
	case ActionAttack:
		return "ACTION_ATTACK"
	case ActionPickUp:
		return "ACTION_PICKUP"
	case ActionSpell:
		return "ACTION_SPELL"
	case ActionTransf:
		return "ACTION_TRANSF"
	case ActionHitted:
		return "ACTION_HITTED"
	case ActionSpinKick:
		return "ACTION_SPINKICK"
	case ActionDie:
		return "ACTION_DIE"
	case ActionMine:
		return "ACTION_MINE"
	default:
		return "ACTION_UNKNOWN"
	}
}

// ActionNode mirrors the packed legacy ActionNode.
//
// The first four wire bytes realize the C++ bitfields:
// type:5 at bit 0, speed:9 at bit 5, direction:5 at bit 16.
// AimUID also serves as FromUID for ACTION_HITTED, matching the C++ union.
type ActionNode struct {
	Type      ActionType
	Speed     uint16
	Direction uint8
	X         int16
	Y         int16
	AimX      int16
	AimY      int16
	AimUID    uint64
	ExtParam  [8]byte
}

// AppendWire appends the packed 28-byte representation.
func (n ActionNode) AppendWire(dst []byte) []byte {
	start := len(dst)
	dst = append(dst, make([]byte, ActionNodeWireSize)...)

	flags := uint32(n.Type)&0x1f |
		(uint32(n.Speed)&0x1ff)<<5 |
		(uint32(n.Direction)&0x1f)<<16
	binary.LittleEndian.PutUint32(dst[start:start+4], flags)
	binary.LittleEndian.PutUint16(dst[start+4:start+6], uint16(n.X))
	binary.LittleEndian.PutUint16(dst[start+6:start+8], uint16(n.Y))
	binary.LittleEndian.PutUint16(dst[start+8:start+10], uint16(n.AimX))
	binary.LittleEndian.PutUint16(dst[start+10:start+12], uint16(n.AimY))
	binary.LittleEndian.PutUint64(dst[start+12:start+20], n.AimUID)
	copy(dst[start+20:start+28], n.ExtParam[:])
	return dst
}

// ParseActionNode decodes one packed 28-byte ActionNode.
func ParseActionNode(src []byte) (ActionNode, error) {
	if len(src) != ActionNodeWireSize {
		return ActionNode{}, fmt.Errorf("ActionNode needs %d bytes, got %d", ActionNodeWireSize, len(src))
	}

	flags := binary.LittleEndian.Uint32(src[:4])
	return ActionNode{
		Type:      ActionType(flags & 0x1f),
		Speed:     uint16((flags >> 5) & 0x1ff),
		Direction: uint8((flags >> 16) & 0x1f),
		X:         int16(binary.LittleEndian.Uint16(src[4:6])),
		Y:         int16(binary.LittleEndian.Uint16(src[6:8])),
		AimX:      int16(binary.LittleEndian.Uint16(src[8:10])),
		AimY:      int16(binary.LittleEndian.Uint16(src[10:12])),
		AimUID:    binary.LittleEndian.Uint64(src[12:20]),
		ExtParam:  [8]byte(src[20:28]),
	}, nil
}

// FromUID returns the union member used by ACTION_HITTED.
func (n ActionNode) FromUID() uint64 {
	return n.AimUID
}

// SetSpellExt stores ExtParamSpell.
func (n *ActionNode) SetSpellExt(magicID, modifierID uint32) {
	clear(n.ExtParam[:])
	binary.LittleEndian.PutUint32(n.ExtParam[0:4], magicID)
	binary.LittleEndian.PutUint32(n.ExtParam[4:8], modifierID)
}

// SpellExt returns ExtParamSpell.
func (n ActionNode) SpellExt() (magicID, modifierID uint32) {
	return binary.LittleEndian.Uint32(n.ExtParam[0:4]), binary.LittleEndian.Uint32(n.ExtParam[4:8])
}

// SetAttackExt stores ExtParamAttack.
func (n *ActionNode) SetAttackExt(magicID, modifierID uint32) {
	n.SetSpellExt(magicID, modifierID)
}

// AttackExt returns ExtParamAttack.
func (n ActionNode) AttackExt() (magicID, modifierID uint32) {
	return n.SpellExt()
}

// SetMoveExt stores ExtParamMove.
func (n *ActionNode) SetMoveExt(onHorse bool) {
	clear(n.ExtParam[:])
	if onHorse {
		n.ExtParam[0] = 1
	}
}

// MoveExt returns ExtParamMove.
func (n ActionNode) MoveExt() bool {
	return n.ExtParam[0] != 0
}
