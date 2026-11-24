package common

import "fmt"

// ==================== UID 编码（common/src/uidf.cpp）====================
//
// 位布局（uidf.hpp 注释，仅用低 63 位，最高位留给 Lua 的有符号整数）：
//
//	63      : 未使用
//	62 - 59 : type    （4 位，最多 16 种）
//	58 - 55 : peer    （4 位）
//	54 - 31 : id      （24 位）
//	30 - 0  : seq     （31 位）
//
// 客户端复刻时踩过坑：mapUID 不能传裸 mapID。
// 必须 getBaseMapUID(mapID) = type=UID_MAP, seq=1 的完整编码。
// 例如 0.bin 的 mapUID 是 (2<<59)|(1<<35)|(1<<31)。

const (
	uidTypeBits   = 4
	peerIndexBits = 4
	idBits        = 24
	reservedBits  = 1
	seqBits       = 64 - reservedBits - uidTypeBits - idBits - peerIndexBits // 31

	uidTypeBitOff   = 64 - reservedBits - uidTypeBits                          // 59
	idBitOff        = 64 - reservedBits - uidTypeBits - idBits                 // 35
	peerIndexBitOff = 64 - reservedBits - uidTypeBits - idBits - peerIndexBits // 31
	seqBitOff       = 0

	uidTypeBitMask   = uint64((1<<uidTypeBits)-1) << uidTypeBitOff
	idBitMask        = uint64((1<<idBits)-1) << idBitOff
	peerIndexBitMask = uint64((1<<peerIndexBits)-1) << peerIndexBitOff
	seqBitMask       = uint64((1<<seqBits)-1) << seqBitOff
)

// UIDType 对应 uidf.hpp 的 UIDType 枚举。
type UIDType uint8

const (
	UIDNone UIDType = 0
	UIDCor  UIDType = 1 // 核心（service core / peer core）
	UIDMap  UIDType = 2
	UIDNPC  UIDType = 3
	UIDMon  UIDType = 4
	UIDPly  UIDType = 5
	UIDRcv  UIDType = 6
	UIDQst  UIDType = 7
	UIDSlo  UIDType = 8 // server lua object
	UIDEnd  UIDType = 9
)

// String 返回类型短名（与原版 getUIDTypeCStr 一致）。
func (t UIDType) String() string {
	switch t {
	case UIDCor:
		return "COR"
	case UIDMap:
		return "MAP"
	case UIDNPC:
		return "NPC"
	case UIDMon:
		return "MON"
	case UIDPly:
		return "PLY"
	case UIDRcv:
		return "RCV"
	case UIDQst:
		return "QST"
	case UIDSlo:
		return "SLO"
	default:
		return "ERR"
	}
}

// GetUIDType 提取 UID 的类型字段。
func GetUIDType(uid uint64) UIDType {
	return UIDType((uid & uidTypeBitMask) >> uidTypeBitOff)
}

// buildUID 按原版 _build_UID_helper 的位运算拼装 UID。
//
// 参数校验与原版 fflassert 对应：id 必须 >0（allowZeroID 除外的场景不使用）、
// id 必须塞得进 24 位、peerIndex 必须塞得进 4 位、seq 必须塞得进 31 位。
func buildUID(uidType UIDType, id, peerIndex uint32, seq uint32) (uint64, error) {
	if id == 0 {
		return 0, fmt.Errorf("uid id must be non-zero")
	}
	if id >= (1 << idBits) {
		return 0, fmt.Errorf("uid id %d exceeds %d bits", id, idBits)
	}
	if peerIndex >= (1 << peerIndexBits) {
		return 0, fmt.Errorf("uid peerIndex %d exceeds %d bits", peerIndex, peerIndexBits)
	}
	if seq >= (1 << seqBits) {
		return 0, fmt.Errorf("uid seq %d exceeds %d bits", seq, seqBits)
	}
	return uint64(uidType)<<uidTypeBitOff |
		uint64(peerIndex)<<peerIndexBitOff |
		uint64(id)<<idBitOff |
		uint64(seq)<<seqBitOff, nil
}

// BuildMapUID 构造一个"带序号的"地图 UID（原版 buildMapUID，seq 起点 2）。
//
// 一张地图可以有多份实例（副本/镜像），seq 区分它们。
func BuildMapUID(mapID uint32, peerIndex uint32) (uint64, error) {
	return buildUID(UIDMap, mapID, peerIndex, 2)
}

// GetBaseMapUID 构造某个 mapID 的"基础"地图 UID（原版 getBaseMapUID，seq=1）。
//
// ★ 这是客户端连服务器时最常用的那个编码。
// 传裸 mapID 会让客户端认不出地图，这是客户端复刻时踩过的坑。
func GetBaseMapUID(mapID uint32, peerIndex uint32) (uint64, error) {
	return buildUID(UIDMap, mapID, peerIndex, 1)
}

// GetPlayerUID 构造玩家 UID（原版 getPlayerUID，uidf.cpp:182）。
//
// ★ 注意：玩家 UID **不按 id 段偏移**，dbid 直接放在最低位：
//
//	return (UID_PLY << uidTypeBitOff) | dbid
//
// 原版 getPlayerDBID 就是 to_u32(uid)，直接截低 32 位。
// 这里曾误按 idBitOff 位移，导致反解恒为 0 —— 玩家 UID 是特例，不是 buildUID。
func GetPlayerUID(dbid uint32) uint64 {
	return uint64(UIDPly)<<uidTypeBitOff | uint64(dbid)
}

// GetPlayerDBID 从玩家 UID 反解 dbid（原版 uidf.cpp:187 的 to_u32(uid)）。
func GetPlayerDBID(uid uint64) (uint32, error) {
	if GetUIDType(uid) != UIDPly {
		return 0, fmt.Errorf("uid %s is not a player uid", GetUIDString(uid))
	}
	return uint32(uid), nil
}

// BuildMonsterUID 构造怪物 UID（原版 buildMonsterUID，seq 起点 1）。
func BuildMonsterUID(monsterID uint32, peerIndex uint32, seq uint32) (uint64, error) {
	if seq == 0 {
		seq = 1 // 原版 startOff=1
	}
	return buildUID(UIDMon, monsterID, peerIndex, seq)
}

// BuildNPCUID 构造 NPC UID（原版 buildNPCUID，seq 起点 1）。
func BuildNPCUID(npcID uint32, peerIndex uint32, seq uint32) (uint64, error) {
	if seq == 0 {
		seq = 1
	}
	return buildUID(UIDNPC, npcID, peerIndex, seq)
}

// GetPeerCoreUID 构造 peer core UID（原版 getPeerCoreUID）。
func GetPeerCoreUID(peerIndex uint32) uint64 {
	return uint64(UIDCor)<<uidTypeBitOff | uint64(peerIndex)
}

// GetServiceCoreUID 返回 service core 的 UID（peerIndex 0）。
func GetServiceCoreUID() uint64 { return GetPeerCoreUID(0) }

// GetQuestUID 构造任务 UID（原版 getQuestUID）。
func GetQuestUID(questID uint32) uint64 {
	return uint64(UIDQst)<<uidTypeBitOff | uint64(questID)
}

// GetServerLuaObjectUID 构造服务端 Lua 对象 UID（原版 getServerLuaObjectUID）。
func GetServerLuaObjectUID(luaObjIndex uint32) uint64 {
	return uint64(UIDSlo)<<uidTypeBitOff | uint64(luaObjIndex)
}

// UIDID 提取 id 段（原版 getMapID/getMonsterID 等的共同实现）。
func UIDID(uid uint64) uint32 {
	return uint32((uid & idBitMask) >> idBitOff)
}

// UIDPeerIndex 提取 peer 段。
//
// 原版只对 UID_MAP/NPC/MON 返回该字段，其他类型返回 0。
func UIDPeerIndex(uid uint64) uint32 {
	switch GetUIDType(uid) {
	case UIDMap, UIDNPC, UIDMon:
		return uint32((uid & peerIndexBitMask) >> peerIndexBitOff)
	case UIDCor:
		return uint32((uid & seqBitMask) >> seqBitOff) // 原版用 seq 段承载 peer
	default:
		return 0
	}
}

// UIDSeq 提取 seq 段。
func UIDSeq(uid uint64) uint32 {
	return uint32((uid & seqBitMask) >> seqBitOff)
}

// GetMapID 从地图 UID 反解 mapID。
func GetMapID(uid uint64) (uint32, error) {
	if GetUIDType(uid) != UIDMap {
		return 0, fmt.Errorf("uid %s is not a map uid", GetUIDString(uid))
	}
	return UIDID(uid), nil
}

// GetMonsterID 从怪物 UID 反解 monsterID。
func GetMonsterID(uid uint64) (uint32, error) {
	if GetUIDType(uid) != UIDMon {
		return 0, fmt.Errorf("uid %s is not a monster uid", GetUIDString(uid))
	}
	return UIDID(uid), nil
}

// GetNPCID 从 NPC UID 反解 npcID。
func GetNPCID(uid uint64) (uint32, error) {
	if GetUIDType(uid) != UIDNPC {
		return 0, fmt.Errorf("uid %s is not a npc uid", GetUIDString(uid))
	}
	return UIDID(uid), nil
}

// GetUIDString 返回可读的 UID 描述（与原版 getUIDString 对齐，便于日志排查）。
func GetUIDString(uid uint64) string {
	if uid == 0 {
		return "ZERO"
	}
	switch GetUIDType(uid) {
	case UIDPly:
		return fmt.Sprintf("PLY_%d", uint32(uid)) // dbid 在最低位，不是 id 段
	case UIDMon:
		return fmt.Sprintf("MON_%d_%d_%d", UIDID(uid), UIDPeerIndex(uid), UIDSeq(uid))
	case UIDNPC:
		return fmt.Sprintf("NPC_%d_%d_%d", UIDID(uid), UIDPeerIndex(uid), UIDSeq(uid))
	case UIDMap:
		return fmt.Sprintf("MAP_%d_%d_%d", UIDID(uid), UIDPeerIndex(uid), UIDSeq(uid))
	case UIDCor:
		return fmt.Sprintf("CORE_%d", UIDPeerIndex(uid))
	case UIDRcv:
		return fmt.Sprintf("RCV_%d", UIDSeq(uid))
	case UIDQst:
		return fmt.Sprintf("QST_%d", UIDID(uid))
	default:
		return fmt.Sprintf("ERR_%d", uid)
	}
}

// IsPlayerUID / IsMonsterUID / IsNPCUID / IsMapUID 是便捷判定。
func IsPlayerUID(uid uint64) bool  { return GetUIDType(uid) == UIDPly }
func IsMonsterUID(uid uint64) bool { return GetUIDType(uid) == UIDMon }
func IsNPCUID(uid uint64) bool     { return GetUIDType(uid) == UIDNPC }
func IsMapUID(uid uint64) bool     { return GetUIDType(uid) == UIDMap }
