package game

import (
	"sync"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 玩家运行时（player.hpp 的核心字段）====================
//
// 只覆盖「进图后世界里活着」所必需的部分。原版 player.hpp 有 741 行、
// 含行会/队伍/交易/寄售/脚本事件等，那些属于 B6；这里先保证：
//
//	位置、方向、HP/MP、经验、金币、背包、装备、腰带、已学魔法、buff
//
// 与 world.Entity 的关系：Player 持有 world 侧的 uid，world 只认 uid + 网格。
// 需要坐标/血量时读世界，需要玩家私有时读 Player —— 单一数据源，不复制。

// Player 是一个已进入世界的玩家。
type Player struct {
	// ChannelID 当前绑定的连接。掉线/换线时改这里
	ChannelID uint64
	// DBID 角色库 ID
	DBID uint32
	// UID 玩家 UID（(UID_PLY<<59)|dbid）
	UID uint64
	// MapID 当前地图
	MapID uint32
	// MapUID 当前地图 UID（含 peer/seq，客户端要这个）
	MapUID uint64

	Name      string
	Gender    uint8
	Job       uint8
	Hair      int32
	HairColor int32

	Level     int
	HP, MaxHP int32
	MP, MaxMP int32
	Exp       uint64
	Gold      uint64
	PKPoint   int32
	ViewRange int

	Direction uint8
	X, Y      int

	// 背包与装备（只存 ID 与数量，属性计算按需查表）
	Inventory []Item
	Belt      [6]*Item
	Wear      map[int]*Item // wltype -> item
	Magics    []uint32
	Buffs     []uint32

	// 掉落/拾取节流（原版 m_pickUpLock）
	PickUpLock bool
}

// Item 是背包里的一件物品（精简版，完整 SDItem 在 protocol 层）。
type Item struct {
	ItemID uint32
	SeqID  uint64
	Count  uint32
}

// PlayerManager 管理所有在线玩家。
//
// 并发模型：所有读写都加锁。地图 actor 内部是串行的，但玩家对象的
// 状态会被多个 goroutine 触碰（每个连接一个），所以这里用 RWMutex
// 保护，而不是依赖 actor 串行。
type PlayerManager struct {
	mu sync.RWMutex
	// byUID uid -> player
	byUID map[uint64]*Player
	// byChannel 连接 -> uid，用于断线时快速找到玩家
	byChannel map[uint64]uint64
	// byDBID dbid -> uid
	byDBID map[uint32]uint64
}

// NewPlayerManager 创建玩家管理器。
func NewPlayerManager() *PlayerManager {
	return &PlayerManager{
		byUID:     make(map[uint64]*Player),
		byChannel: make(map[uint64]uint64),
		byDBID:    make(map[uint32]uint64),
	}
}

// Add 把一个玩家登记进管理器。
func (m *PlayerManager) Add(p *Player) {
	if p == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byUID[p.UID] = p
	if p.ChannelID != 0 {
		m.byChannel[p.ChannelID] = p.UID
	}
	m.byDBID[p.DBID] = p.UID
}

// Get 按 UID 取玩家（返回指针，调用方需自行注意并发）。
func (m *PlayerManager) Get(uid uint64) (*Player, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.byUID[uid]
	return p, ok
}

// ByChannel 按连接取玩家。
func (m *PlayerManager) ByChannel(channelID uint64) (*Player, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	uid, ok := m.byChannel[channelID]
	if !ok {
		return nil, false
	}
	p, ok := m.byUID[uid]
	return p, ok
}

// ByDBID 按角色库 ID 取玩家。
func (m *PlayerManager) ByDBID(dbid uint32) (*Player, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	uid, ok := m.byDBID[dbid]
	if !ok {
		return nil, false
	}
	p, ok := m.byUID[uid]
	return p, ok
}

// Remove 移除玩家，返回被移除的对象。
func (m *PlayerManager) Remove(uid uint64) (*Player, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byUID[uid]
	if !ok {
		return nil, false
	}
	delete(m.byUID, uid)
	if p.ChannelID != 0 {
		delete(m.byChannel, p.ChannelID)
	}
	delete(m.byDBID, p.DBID)
	return p, true
}

// Count 返回在线玩家数。
func (m *PlayerManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.byUID)
}

// Snapshot 返回所有在线玩家（浅拷贝列表，元素仍是原指针）。
func (m *PlayerManager) Snapshot() []*Player {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Player, 0, len(m.byUID))
	for _, p := range m.byUID {
		out = append(out, p)
	}
	return out
}

// ==================== 与世界层的桥接 ====================

// WorldEntity 把 Player 转成 world 侧实体。
//
// 等级 -> 属性映射先给一组保守值，等 magicrecord/monsterrecord 的属性表
// 接进来（B5 后半）再替换成真实公式。
func (p *Player) WorldEntity(mapUID uint64) world.Entity {
	return world.Entity{
		UID:       p.UID,
		Type:      world.EntityPlayer,
		Name:      p.Name,
		X:         p.X,
		Y:         p.Y,
		Direction: world.Direction(p.Direction),
		HP:        p.HP,
		MaxHP:     p.MaxHP,
		MP:        p.MP,
		MaxMP:     p.MaxMP,
		ViewRange: p.ViewRange,
		Stats:     PlayerCombatStats(p.Job, p.Level),
	}
}

// PlayerCombatStats 按职业与等级给出战斗属性。
//
// 原版从 hero.cpp 的 _getAttackPower/_getMagicPower 与 tbl_char 的装备
// 综合计算，公式依赖 magicrecord/物品表。这里先给「能打但不荒谬」的
// 基础值，等属性表接入后替换（会在 REPLICA_AUDIT 里标注为临时代码）。
//
// 不这么做的话，玩家进图后 DC=0，连怪都打不动，B5 无法端到端验证。
func PlayerCombatStats(job uint8, level int) world.CombatStats {
	if level < 1 {
		level = 1
	}
	// 战士偏物理、法道偏魔法，与 jobf.hpp 的职业定位一致
	switch job {
	case 1: // 战士
		return world.CombatStats{
			DC:    world.AbilityBounds{Min: level/3 + 1, Max: level/2 + 3},
			DCHit: 8 + level*2,
			AC:    world.AbilityBounds{Min: level / 4, Max: level/2 + 1},
			MAC:   world.AbilityBounds{Min: level / 6, Max: level/3 + 1},
		}
	case 2: // 法师
		return world.CombatStats{
			DC:    world.AbilityBounds{Min: level/6 + 1, Max: level/4 + 2},
			DCHit: 6 + level,
			MC:    world.AbilityBounds{Min: level/2 + 1, Max: level + 2},
			MCHit: 8 + level*2,
			AC:    world.AbilityBounds{Min: level / 6, Max: level/3 + 1},
			MAC:   world.AbilityBounds{Min: level / 4, Max: level/2 + 1},
		}
	case 3: // 道士
		return world.CombatStats{
			DC:    world.AbilityBounds{Min: level/5 + 1, Max: level/3 + 2},
			DCHit: 7 + level,
			MC:    world.AbilityBounds{Min: level/3 + 1, Max: level/2 + 2},
			MCHit: 7 + level,
			AC:    world.AbilityBounds{Min: level / 5, Max: level/3 + 1},
			MAC:   world.AbilityBounds{Min: level / 4, Max: level/2 + 1},
		}
	default:
		return world.CombatStats{
			DC:    world.AbilityBounds{Min: 1, Max: 3},
			DCHit: 8,
			AC:    world.AbilityBounds{Min: 0, Max: 1},
			MAC:   world.AbilityBounds{Min: 0, Max: 1},
		}
	}
}

// SyncFromWorld 把 world 侧的坐标/血量同步回 Player。
//
// 为什么要这一步：移动与战斗都在 world 层执行（那里有网格与占用），
// 执行完要把结果写回 Player，否则玩家对象上的坐标就过期了。
func (p *Player) SyncFromWorld(e world.Entity) {
	p.X, p.Y = e.X, e.Y
	p.Direction = uint8(e.Direction)
	p.HP, p.MaxHP = e.HP, e.MaxHP
	p.MP, p.MaxMP = e.MP, e.MaxMP
}

// DisplayLevel 按经验算等级（SYS_LEVEL）。
//
// 原版 sysconst.hpp:SYS_LEVEL 是对 SYS_EXPLEVELARRAY 的查表，
// 这里先用等比近似，等经验表接入再替换。
func DisplayLevel(exp uint64) int {
	level := 1
	need := uint64(100)
	for level < 100 && exp >= need {
		exp -= need
		level++
		need = uint64(float64(need) * 1.25)
	}
	return level
}

// UIDOf 返回玩家的 world UID（便捷函数）。
func (p *Player) UIDOf() uint64 {
	if p.UID != 0 {
		return p.UID
	}
	return common.GetPlayerUID(p.DBID)
}
