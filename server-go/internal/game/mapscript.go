package game

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 地图脚本运行时（server/src/servermap.cpp:88-560）====================
//
// 地图脚本决定「这张图上有什么」：
//
//	addSafeZone(SYS_SAFEZONE, {{x=423,y=102,r=30}})   安全区
//	addGuard('大刀卫士', 371, 160, DIR_DOWNLEFT)      守卫
//	local addMonCo = addmon.monGener({...})           刷怪配置
//	coroutine.resume(addMonCo)                        启动刷怪协程
//
// ★ 最后两行是整套机制的核心：
//
//	monGener() 返回一个协程，里面是 `while true do ... end`，
//	每轮检查「到刷怪时间了吗」，然后 addMonster() 刷一批。
//	原版由 actor 定时器周期性 resume 它。
//
//	Go 侧由 MapTicker 每 100ms 调 script.Tick()，
//	Script.Tick 内部按 1 秒粒度 resume 刷怪协程。

// MapScriptManager 管理所有地图的脚本实例。
type MapScriptManager struct {
	mu    sync.RWMutex
	byMap map[uint32]*mapScriptEntry

	FrameworkDir string
	ScriptRoot   string
	Monsters     *world.MonsterDB
	Logger       *slog.Logger
}

// mapScriptEntry 是一张地图的脚本 + 宿主。
type mapScriptEntry struct {
	MapID  uint32
	Script *script.MapScript
	Host   *mapScriptHost
}

// NewMapScriptManager 创建管理器。
func NewMapScriptManager(monsters *world.MonsterDB, logger *slog.Logger) *MapScriptManager {
	return &MapScriptManager{
		byMap:        make(map[uint32]*mapScriptEntry),
		FrameworkDir: script.ResolveFrameworkDir(),
		ScriptRoot:   script.ResolveScriptRoot(),
		Monsters:     monsters,
		Logger:       logger,
	}
}

// LoadMap 加载一张地图的脚本。
func (m *MapScriptManager) LoadMap(ctx context.Context, mapID uint32, mapName string,
	worlds *world.World, players *PlayerManager) (*script.MapScript, error) {

	host := &mapScriptHost{
		mapID:    mapID,
		mapName:  mapName,
		Worlds:   worlds,
		Players:  players,
		Monsters: m.Monsters,
		Logger:   m.Logger,
		ctx:      ctx,
	}

	// 记录表（脚本里 getItemID / getMonsterID 要用）
	itemTbl := m.itemNameTable(players)
	monsterTbl := script.NewNameTable(m.Monsters.NameTable())
	mapTbl := script.NewNameTable(nil)

	commonBindings := script.CommonBindings(itemTbl, monsterTbl, mapTbl,
		func(level int, msg string) {
			if m.Logger != nil {
				m.Logger.Debug("map script", "map", mapName, "msg", msg)
			}
		})

	ms, err := script.LoadMapScript(script.Options{
		FrameworkDir: m.FrameworkDir,
		ScriptRoot:   m.ScriptRoot,
		Consts:       script.DefaultConsts(),
		Bindings:     commonBindings,
	}, mapName, host)
	if err != nil {
		return nil, err
	}

	// 启动 main() 协程（刷怪/守卫从这一刻开始运转）
	ms.Start(0)

	m.mu.Lock()
	m.byMap[mapID] = &mapScriptEntry{MapID: mapID, Script: ms, Host: host}
	m.mu.Unlock()

	if m.Logger != nil {
		m.Logger.Info("map script loaded", "map", mapName,
			"spawners", ms.SpawnerCount(),
			"guards", host.GuardCount(),
			"monsters", host.MonsterCount(),
			"safeZones", host.SafeZoneCount())
	}
	return ms, nil
}

// Tick 推进所有地图的脚本（由 MapTicker 调用）。
func (m *MapScriptManager) Tick(nowMS int64) {
	m.mu.RLock()
	entries := make([]*mapScriptEntry, 0, len(m.byMap))
	for _, e := range m.byMap {
		entries = append(entries, e)
	}
	m.mu.RUnlock()

	for _, e := range entries {
		e.Script.Tick(nowMS)
	}
}

// Close 释放所有脚本。
func (m *MapScriptManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.byMap {
		e.Script.Close()
	}
	m.byMap = make(map[uint32]*mapScriptEntry)
}

// itemNameTable 构造物品名字表。
func (m *MapScriptManager) itemNameTable(players *PlayerManager) *script.NameTable {
	// 物品表挂在 WorldService 上，这里通过 playerManager 拿不到，
	// 由调用方在装配阶段注入 —— 当前返回空表，
	// 影响仅是地图脚本里 getItemID(' xxx') 返回 0（刷怪不用物品表）。
	return script.NewNameTable(nil)
}

// ==================== 宿主实现 ====================

// mapScriptHost 把世界层能力暴露给地图 Lua 脚本。
type mapScriptHost struct {
	mapID   uint32
	mapName string

	Worlds   *world.World
	Players  *PlayerManager
	Monsters *world.MonsterDB
	Logger   *slog.Logger

	ctx context.Context

	mu        sync.Mutex
	guards    []uint64
	safeZones int
	// spawnedMonsters 累计刷出的怪物数（诊断用）
	spawnedMonsters int
	// nextMonsterSeq 刷怪序号（保证 UID 唯一）
	nextMonsterSeq uint32
}

func (h *mapScriptHost) MapID() uint32   { return h.mapID }
func (h *mapScriptHost) MapName() string { return h.mapName }

func (h *mapScriptHost) MapSize() (int, int) {
	w, hh, err := h.Worlds.MapSize(h.ctx, h.mapID)
	if err != nil {
		return 0, 0
	}
	return w, hh
}

// CountThrough 批量统计矩形区域内的可走格数。
//
// 一次跨 actor 调用算完整块：把 CountThrough 的循环放在 world 层做，
// 而不是在 Lua 里逐格调 CanThrough（那要 41 万次跨边界，30 秒）。
func (h *mapScriptHost) CountThrough(x, y, w, hh int) int {
	return h.Worlds.CountWalkable(h.ctx, h.mapID, x, y, w, hh)
}

func (h *mapScriptHost) CanThrough(x, y int) bool {
	// 用「无实体占用时的可站立判定」——刷怪只需要地图地形可走，
	// 不需要考虑当前有没有怪站在那（原版 groundValid 也是这个语义）
	return h.Worlds.CanWalk(h.ctx, h.mapID, x, y)
}

// AddMonster 刷一只怪。
//
// ★ 这是全链路最热的调用（753 处 addMonCo 最终都到这里）。
//
//	x/y 为 -1 表示「随机位置」，与原版 addMonster 的默认行为一致。
//	strictLoc=true 表示必须落在这个精确格（失败就不刷）。
func (h *mapScriptHost) AddMonster(name string, x, y int, strictLoc bool) uint64 {
	def, ok := h.lookupMonster(name)
	if !ok {
		return 0
	}

	// 随机位置：整图随机找一个可走格
	if x < 0 || y < 0 {
		w, hh := h.MapSize()
		found := false
		for i := 0; i < 200 && !found; i++ {
			rx, ry := randInt(w), randInt(hh)
			if h.CanThrough(rx, ry) {
				x, y = rx, ry
				found = true
			}
		}
		if !found {
			return 0
		}
	}

	if h.CanThrough(x, y) == false && strictLoc {
		return 0
	}

	uid := h.newMonsterUID(def.LookID)
	entity := world.Entity{
		UID:       uid,
		Type:      world.EntityMonster,
		Name:      def.Name,
		X:         x,
		Y:         y,
		Direction: world.DirDown,
		HP:        def.HP,
		MaxHP:     def.HP,
		MP:        def.MP,
		MaxMP:     def.MP,
		Speed:     600,
		ViewRange: def.View,
		Stats:     def.CombatStatsFor(),
		// BaseStats 是 buff 到期时重算的基线，不给的话回退会归零
		BaseStats: def.CombatStatsFor(),
	}
	rec := def.ToRecord()
	if _, err := h.Worlds.SpawnMonster(h.ctx, h.mapID, entity, rec); err != nil {
		return 0
	}
	h.mu.Lock()
	h.spawnedMonsters++
	h.mu.Unlock()
	return uid
}

// AddGuard 放一个守卫。
//
// 守卫与普通怪的区别只有 AI 行为（只打红名），
// 当前版本先当普通怪刷出来，行为差异留到 AI 批次。
func (h *mapScriptHost) AddGuard(name string, x, y, direction int) uint64 {
	def, ok := h.lookupMonster(name)
	if !ok {
		return 0
	}
	uid := h.newMonsterUID(def.LookID)
	entity := world.Entity{
		UID:       uid,
		Type:      world.EntityMonster,
		Name:      def.Name,
		X:         x,
		Y:         y,
		Direction: world.Direction(direction),
		HP:        def.HP,
		MaxHP:     def.HP,
		MP:        def.MP,
		MaxMP:     def.MP,
		Speed:     600,
		ViewRange: def.View,
		Stats:     def.CombatStatsFor(),
		// BaseStats 是 buff 到期时重算的基线，不给的话回退会归零
		BaseStats: def.CombatStatsFor(),
	}
	rec := def.ToRecord()
	if _, err := h.Worlds.SpawnMonster(h.ctx, h.mapID, entity, rec); err != nil {
		return 0
	}
	h.mu.Lock()
	h.guards = append(h.guards, uid)
	h.mu.Unlock()
	return uid
}

// AddSafeZone 加一个圆形安全区。
func (h *mapScriptHost) AddSafeZone(x, y, radius int) {
	if err := h.Worlds.AddSafeZoneCircle(h.ctx, h.mapID, x, y, radius); err != nil {
		if h.Logger != nil {
			h.Logger.Debug("add safe zone failed", "map", h.mapName, "error", err)
		}
		return
	}
	h.mu.Lock()
	h.safeZones++
	h.mu.Unlock()
}

func (h *mapScriptHost) Alive(uid uint64) bool {
	e, ok, err := h.Worlds.Entity(h.ctx, h.mapID, uid)
	if err != nil || !ok {
		return false
	}
	return e.HP > 0
}

func (h *mapScriptHost) Log(level int, msg string) {
	if h.Logger == nil {
		return
	}
	switch level {
	case 0:
		h.Logger.Info("map script", "map", h.mapName, "msg", msg)
	case 1:
		h.Logger.Warn("map script", "map", h.mapName, "msg", msg)
	default:
		h.Logger.Error("map script", "map", h.mapName, "msg", msg)
	}
}

func (h *mapScriptHost) GuardCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.guards)
}

func (h *mapScriptHost) MonsterCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.spawnedMonsters
}

func (h *mapScriptHost) SafeZoneCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.safeZones
}

// lookupMonster 按名字或 ID 查怪物定义。
func (h *mapScriptHost) lookupMonster(nameOrID string) (world.MonsterDef, bool) {
	if h.Monsters == nil {
		return world.MonsterDef{}, false
	}
	// 纯数字：当作 monsterID（脚本允许 addMonster(37, x, y)）
	if id, err := parseUint32(nameOrID); err == nil && id > 0 {
		return h.Monsters.Get(id)
	}
	id, ok := h.Monsters.FindByName(nameOrID)
	if !ok {
		return world.MonsterDef{}, false
	}
	return h.Monsters.Get(id)
}

// newMonsterUID 生成怪物 UID。
//
// 用「自增序号」而不是坐标哈希：刷怪位置会变（随机），
// 用坐标会让同一只怪换个位置就换 UID，仇恨/掉落归属全乱。
func (h *mapScriptHost) newMonsterUID(lookID int) uint64 {
	h.mu.Lock()
	h.nextMonsterSeq++
	seq := h.nextMonsterSeq
	h.mu.Unlock()

	id := uint32(lookID)
	if id == 0 {
		id = 1
	}
	if uid, err := common.BuildMonsterUID(id, 0, seq); err == nil {
		return uid
	}
	return uint64(3)<<59 | uint64(id&0xFFFFFF)<<35 | uint64(seq&0x7FFFFFFF)<<4 | 1
}

// parseUint32 解析十进制无符号整数。
func parseUint32(s string) (uint32, error) {
	var v uint32
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

// randInt 是包级随机（刷怪位置不需要密码学强度）。
func randInt(n int) int {
	if n <= 0 {
		return 0
	}
	return int(randUint32() % uint32(n))
}
