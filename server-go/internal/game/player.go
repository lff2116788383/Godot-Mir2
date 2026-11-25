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

	// MagicTrain 各技能的修炼等级（0..3，对应 magicrecord req.train 三档）。
	//
	// 影响两处：MP 消耗（mp + mpInc*trainLevel）与技能伤害
	// （power + addPower*trainLevel）。没练过的技能是 0 级。
	MagicTrain map[uint32]int
	// MagicCooldown 各技能上次施放的世界时钟（毫秒）。
	//
	// ★ 必须用世界时钟而不是 time.Now()：
	//
	//	地图 actor 串行执行，同一 tick 内 NowMS 一致；
	//	用系统时间会让「同一 tick 内连点两次」这种边界行为不确定。
	MagicCooldown map[uint32]int64
	// MagicKeys 技能快捷键绑定（magicID -> 按键），1-6 留给腰带
	MagicKeys map[uint32]uint8

	// 掉落/拾取节流（原版 m_pickUpLock）
	PickUpLock bool
}

// MagicTrainLevel 返回某技能的修炼等级（未学/未练都返回 0）。
func (p *Player) MagicTrainLevel(magicID uint32) int {
	if p.MagicTrain == nil {
		return 0
	}
	return p.MagicTrain[magicID]
}

// Item 是背包里的一件物品（精简版，完整 SDItem 在 protocol 层）。
type Item struct {
	ItemID uint32
	SeqID  uint64
	Count  uint32

	// Durability 当前持久（SDItem.duration[0]）
	//
	// ★ 三态语义，必须严格区分（sditem.hpp:205-212）：
	//
	//	MaxDurability == 0                  物品没有持久概念（药品/材料）
	//	MaxDurability >  0 && Durability > 0  正常使用中
	//	MaxDurability >  0 && Durability == 0 **已损坏，不提供任何属性**
	//
	// 第三条最容易漏：不加判断的话，损坏的武器照样能砍出满攻击。
	Durability uint32
	// MaxDurability 最大持久（SDItem.duration[1]）
	//
	// 这是**每件实例**的值，不是物品记录的值 ——
	// 物品记录的 equip.duration 只是「新物品的初始最大持久」。
	// 普通修理会永久降低这个上限（每次损失 (max-curr)/10）。
	MaxDurability uint32
}

// Broken 判断物品是否已损坏（有持久概念但当前为 0）。
func (i Item) Broken() bool {
	return i.MaxDurability > 0 && i.Durability == 0
}

// HasDurability 判断物品是否有持久概念。
func (i Item) HasDurability() bool {
	return i.MaxDurability > 0
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
// Stats 用 CombatStatsFor(items)：等级 + 职业 + 装备 + 已学技能。
// items 为 nil 时退化成只算等级部分（测试环境常见）。
func (p *Player) WorldEntity(mapUID uint64, items *ItemDB) world.Entity {
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
		Stats:     p.CombatStatsFor(items),
	}
}

// PlayerCombatStats 按职业与等级给出战斗属性。
//
// 【依据】common/src/combatnode.cpp getCombatNode：真实公式是
//
//	等级基础：ac += level/2 .. level/2+level%2，mac 同上
//	职业加成：战士 dc += 半级，道士 sc += 半级，法师 mc += 半级
//	学了「基本剑术」：dcHit += 2
//	再累加所有已穿装备的 equip.dc/ac/mc/sc/dcHit/mcHit/...
//
// ★ 这里只算「等级 + 职业 + 技能」的部分，装备部分由 CombatStatsFor
//
//	单独累加。拆开的理由：装备会变（穿脱），等级也会变，但两者的
//	重算时机不同 —— 合成一个函数会让每次穿装备都重跑等级公式。
//
// 初版曾按「战士 DC=level/2+3」这类拍脑袋公式写，结果法师的 MC
// 与道士的 SC 完全不存在，任何法术伤害都算成 0。
func PlayerCombatStats(job uint8, level int) world.CombatStats {
	if level < 1 {
		level = 1
	}
	// level/2 与 level/2+level%2 —— 原版就是这么写的，
	// 奇数级多 1 点上限，不能写成 level/2*2。
	half := level / 2
	halfUp := half + level%2

	stats := world.CombatStats{
		AC:  world.AbilityBounds{Min: half, Max: halfUp},
		MAC: world.AbilityBounds{Min: half, Max: halfUp},
	}
	// jobf.hpp：JOB_WARRIOR=1 / JOB_WIZARD=2 / JOB_TAOIST=3（按位）
	switch job {
	case 1: // 战士：只有 DC
		stats.DC = world.AbilityBounds{Min: half, Max: halfUp}
	case 2: // 法师：只有 MC
		stats.MC = world.AbilityBounds{Min: half, Max: halfUp}
	case 3: // 道士：只有 SC（道术）
		stats.SC = world.AbilityBounds{Min: half, Max: halfUp}
	}
	return stats
}

// CombatStatsFor 算出玩家的完整战斗属性（等级 + 职业 + 装备 + 已学技能）。
//
// items 可为 nil（物品表没载入时退化，只算等级部分）。
// 这是「玩家当前真实战力」的唯一入口 —— world 层的 SetStats
// 与所有伤害结算都应该用它，不要各算一份。
func (p *Player) CombatStatsFor(items *ItemDB) world.CombatStats {
	stats := PlayerCombatStats(p.Job, p.Level)

	for _, it := range p.Wear {
		if it == nil || items == nil {
			continue
		}
		// ★ 已损坏的装备不提供任何属性（sditem.hpp:211）。
		//
		//	不判这一条的话，武器持久打光了照样能砍出满攻击 ——
		//	玩家就没有任何理由去修理，整个经济系统的支出端断掉。
		if it.Broken() {
			continue
		}
		rec, ok := items.Get(it.ItemID)
		if !ok || rec.Equip == nil {
			continue
		}
		eq := rec.Equip
		stats.DC.Min += eq.DC[0]
		stats.DC.Max += eq.DC[1]
		stats.MC.Min += eq.MC[0]
		stats.MC.Max += eq.MC[1]
		stats.SC.Min += eq.SC[0]
		stats.SC.Max += eq.SC[1]
		stats.AC.Min += eq.AC[0]
		stats.AC.Max += eq.AC[1]
		stats.MAC.Min += eq.MAC[0]
		stats.MAC.Max += eq.MAC[1]
		stats.DCHit += eq.DCHit
		stats.MCHit += eq.MCHit
		stats.DCDodge += eq.DCDodge
		stats.MCDodge += eq.MCDodge
	}

	// 基本剑术（学了就 +2 命中）
	if items != nil {
		if basicID, ok := items.FindByName("基本剑术"); ok && p.HasMagic(basicID) {
			stats.DCHit += 2
		}
	}
	return stats
}

// HasMagic 判断玩家是否已学某个 magicID。
func (p *Player) HasMagic(magicID uint32) bool {
	for _, id := range p.Magics {
		if id == magicID {
			return true
		}
	}
	return false
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
