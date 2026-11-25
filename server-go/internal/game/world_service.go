package game

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 玩家进入世界（servicecorenet.cpp:101 之后的流程）====================
//
// 原版 CM_ONLINE 成功后：
//
//	1. 从 tbl_char 读出位置（map/mapx/mapy）
//	2. 构造 SDInitPlayer
//	3. 向地图 actor 发 AM_ADDTOMAP，等待进场
//	4. 地图把它加进网格 + 建立 AOI 观察圈
//	5. 回 SM_ONLINEOK（含 mapUID 与 ActionNode）
//	6. 后续地图通过 AM_OBSERVE 把视野里的其他 CO 推给它
//
// 这里同样是这个顺序，只是 AM_* 换成直接调用 world。
// 关键点：**AOI 事件必须真的转成网络消息发出去**，否则玩家进去看不到任何人。

// WorldService 负责「玩家 ↔ 地图」的桥接。
type WorldService struct {
	Players   *PlayerManager
	Worlds    *world.World
	Broadcast Broadcaster
	Chars     *storage.CharacterRepository
	Logger    *slog.Logger

	// Items 物品表，用于算玩家战斗属性（CombatStatsFor）。
	// 为 nil 时属性退化成「只有等级基础值」，装备不生效。
	Items *ItemDB
	// Magics 魔法表，用于施法校验与伤害结算。为 nil 时所有技能被拒。
	Magics *MagicDB
	// NPCs NPC 脚本管理器。进场时用它加载该地图的 NPC。
	NPCs *NPCManager
	// Durability 装备磨损/修理/金币入账（NPC 修理与拾取金币要用）。
	Durability *DurabilityHandler
	// ItemRepo 角色物品落库（进场时加载背包/装备/腰带）。
	//
	// ★ 之前没有这个字段，`Enter` 只建了个空 Wear map ——
	//	重登后装备和背包全部消失，而且不报错（客户端看到的就是空背包）。
	ItemRepo *storage.ItemRepository
	// MapScripts 地图脚本管理器（刷怪/守卫/安全区）。
	MapScripts *MapScriptManager
	// Ticker 世界时钟驱动器。地图建好后要 Track 它，
	// 否则这张图的 NowMS 永远不动（技能冷却永远不结束）。
	Ticker *MapTicker

	// mapCache mapID -> *world.MapData，避免每次进场都读文件
	mapCache map[uint32]*world.MapData
	// assetsRoot 地图素材目录
	assetsRoot string
}

// NewWorldService 创建世界服务。
func NewWorldService(players *PlayerManager, worlds *world.World, broadcast Broadcaster,
	chars *storage.CharacterRepository, assetsRoot string) *WorldService {
	return &WorldService{
		Players:    players,
		Worlds:     worlds,
		Broadcast:  broadcast,
		Chars:      chars,
		Logger:     slog.Default(),
		mapCache:   make(map[uint32]*world.MapData),
		assetsRoot: assetsRoot,
	}
}

// AssetsRoot 解析地图素材根目录。
//
// 优先环境变量 MIR2X_ASSETS，其次几个约定位置。
// 找不到时返回空串，调用方应准备一块兜底平地（否则玩家进不去）。
func ResolveAssetsRoot() string {
	if root := os.Getenv("MIR2X_ASSETS"); root != "" {
		return root
	}
	candidates := []string{
		"E:/Desktop/game/mir2x_assets-main",
		"../mir2x_assets-main",
		"../../mir2x_assets-main",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}

// LoadMap 载入一张地图（带缓存），并在 world 里建好对应地图 actor。
//
// 载不到真实素材时退化为一块 200x200 全可走平地 —— 这样至少能进游戏、
// 能移动、能看到彼此，而不是让 B5 卡在「缺素材」上。
func (s *WorldService) LoadMap(ctx context.Context, mapID uint32) error {
	if _, ok := s.mapCache[mapID]; ok {
		return nil
	}

	var data *world.MapData
	if s.assetsRoot != "" {
		path := filepath.Join(s.assetsRoot, "map", fmt.Sprintf("%d.bin", mapID))
		if _, err := os.Stat(path); err == nil {
			loaded, err := world.LoadMapFile(path)
			if err != nil {
				s.Logger.Warn("load map failed, falling back to flat ground",
					"map", mapID, "path", path, "error", err)
			} else {
				data = loaded
			}
		}
	}
	if data == nil {
		flat, err := world.NewMapData(200, 200)
		if err != nil {
			return fmt.Errorf("create fallback map: %w", err)
		}
		for y := 0; y < 200; y++ {
			for x := 0; x < 200; x++ {
				_ = flat.SetLand(x, y, world.Land{CanWalk: true})
			}
		}
		data = flat
		s.Logger.Warn("using fallback flat map", "map", mapID, "size", "200x200")
	}

	s.mapCache[mapID] = data
	if err := s.Worlds.CreateMap(mapID, data); err != nil {
		// 地图已存在（重启/重连）不算错误
		if s.Worlds.MapCount() > 0 {
			s.trackMap(mapID)
			return nil
		}
		return fmt.Errorf("create map actor: %w", err)
	}
	s.trackMap(mapID)
	s.Logger.Info("map ready", "map", mapID, "size", fmt.Sprintf("%dx%d", data.Width, data.Height))

	// 加载这张地图的 NPC（原版 servermap.cpp:1701 loadNPChar）
	if err := s.loadMapNPCs(ctx, mapID); err != nil {
		// NPC 加载失败不影响地图可用性，只记日志
		s.Logger.Warn("load map npcs failed", "map", mapID, "error", err)
	}
	// 加载地图脚本（刷怪/守卫/安全区，原版 servermap.cpp:577）
	if err := s.loadMapScript(ctx, mapID); err != nil {
		s.Logger.Warn("load map script failed", "map", mapID, "error", err)
	}
	return nil
}

// loadMapScript 加载本图的地图脚本。
//
// 顺序必须在 NPC 之后：地图脚本的 addGuard 会在安全区上放守卫，
// 安全区要先由脚本自己 addSafeZone 建立（脚本内顺序），
// 但 NPC 与人无关，先加载不影响。
func (s *WorldService) loadMapScript(ctx context.Context, mapID uint32) error {
	if s.MapScripts == nil {
		return nil
	}
	mapName := world.MapNameOf(mapID)
	if mapName == "" {
		return nil
	}
	_, err := s.MapScripts.LoadMap(ctx, mapID, mapName, s.Worlds, s.Players)
	return err
}

// loadMapNPCs 为本图加载 NPC 脚本并在世界层生成实体。
//
// ★ NPC 实体必须进世界层，否则：
//
//  1. 玩家看不到 NPC（AOI 不会下发不存在的东西）
//  2. 客户端点不到（点击靠 SM_CORECORD 里的 UID）
//  3. 距离校验无从谈起（要读 NPC 的坐标）
//
// 原版 NPC 是独立 Actor 走 AM_ADDCO 注册进来的，
// 这里直接 AddEntity，语义等价（都在同一张图上、都参与 AOI）。
func (s *WorldService) loadMapNPCs(ctx context.Context, mapID uint32) error {
	if s.NPCs == nil {
		return nil
	}
	mapName := world.MapNameOf(mapID)
	if mapName == "" {
		return nil
	}

	loaded, err := s.NPCs.LoadMap(ctx, mapID, mapName, func(npcUID uint64) *npcHostAdapter {
		return &npcHostAdapter{
			NPCUID:    npcUID,
			MapID:     mapID,
			Players:   s.Players,
			Worlds:    s.Worlds,
			Broadcast: s.Broadcast,
			Items:     s.Items,
			Logger:    s.Logger,
		}
	})
	if err != nil {
		return err
	}

	spawned := 0
	for _, inst := range loaded {
		entity := world.Entity{
			UID:       inst.NPCUID,
			Type:      world.EntityNPC,
			Name:      inst.Def.Name,
			X:         inst.Def.X,
			Y:         inst.Def.Y,
			Direction: world.Direction(inst.Def.GfxDir + 1), // gfxDir 0..7 -> DIR 1..8
			HP:        1, MaxHP: 1,
			// NPC 不参与战斗，属性留空；HP=1 是为了让 Dead() 的
			// `HP <= 0` 判断不把 NPC 当成尸体
			ViewRange: 0,
		}
		if _, err := s.Worlds.AddEntity(ctx, mapID, entity); err != nil {
			s.Logger.Debug("spawn npc failed", "npc", inst.Def.Name, "error", err)
			continue
		}
		spawned++
	}
	s.Logger.Info("map npc spawned", "map", mapName, "loaded", len(loaded), "spawned", spawned)
	return nil
}

// trackMap 让 ticker 开始驱动这张地图。
//
// ★ 必须在 CreateMap 成功后立刻做：
//
//	NowMS 只在 World.Advance 被调用时推进，没人调它就恒为 0。
//	而技能冷却判断是 `nowMS - last < coolDown`，两个 0 相减
//	得到 0，永远小于任何正数冷却 —— 表现是「第一次放技能后
//	这个技能再也放不出来」。
func (s *WorldService) trackMap(mapID uint32) {
	if s.Ticker != nil {
		s.Ticker.Track(mapID)
	}
}

// Enter 把玩家放进世界。
//
// 返回的 Player 已经登记进 PlayerManager，且地图里已有对应实体。
func (s *WorldService) Enter(ctx context.Context, ch storage.Character, channelID uint64) (*Player, error) {
	mapID := ch.Map
	if mapID == 0 {
		mapID = DefaultSpawnMap
	}
	if err := s.LoadMap(ctx, mapID); err != nil {
		return nil, err
	}

	mapUID, err := common.GetBaseMapUID(mapID, 0)
	if err != nil {
		return nil, fmt.Errorf("build map uid: %w", err)
	}
	playerUID := common.GetPlayerUID(ch.DBID)

	level := DisplayLevel(ch.Exp)
	hp, maxHP := ch.HP, ch.HP
	if hp <= 0 {
		// 老角色血量为 0（没练过）：给一组初始值，否则进门就死
		maxHP = 30 + int32(level)*5
		hp = maxHP
	}
	mp, maxMP := ch.MP, 30+int32(level)*3
	if mp <= 0 {
		mp = maxMP
	}
	viewRange := int(ch.ViewRange)
	if viewRange <= 0 {
		viewRange = 10 // 原版默认视野
	}

	p := &Player{
		ChannelID: channelID,
		DBID:      ch.DBID,
		UID:       playerUID,
		MapID:     mapID,
		MapUID:    mapUID,
		Name:      ch.Name,
		Gender:    ch.Gender,
		Job:       ch.Job,
		Hair:      ch.Hair,
		HairColor: ch.HairColor,
		Level:     level,
		HP:        hp,
		MaxHP:     maxHP,
		MP:        mp,
		MaxMP:     maxMP,
		Exp:       ch.Exp,
		Gold:      ch.Gold,
		PKPoint:   ch.PKPoint,
		ViewRange: viewRange,
		Direction: common.DirectionDown,
		X:         int(ch.MapX),
		Y:         int(ch.MapY),
		Wear:      make(map[int]*Item),
		// ★ 这三个 map 必须在进场时初始化，不能留给首次使用时懒建。
		//   施法路径会直接写 p.MagicCooldown[magicID] = nowMS，
		//   nil map 写入是 panic（不是静默失败），一次技能就把服务端打挂。
		MagicTrain:    make(map[uint32]int),
		MagicCooldown: make(map[uint32]int64),
		MagicKeys:     make(map[uint32]uint8),
	}

	// 加载背包 / 装备 / 腰带
	//
	// ★ 顺序必须在「算属性」之前：
	//
	//	WorldEntity 里的 Stats 是 CombatStatsFor 算的，那个函数读 p.Wear。
	//	先算属性再加载装备，玩家进图时是「裸装属性」，
	//	要等下次换装才恢复 —— 表现是「重登后攻击力变低了」。
	s.loadPlayerItems(ctx, p)

	// 进地图
	events, err := s.Worlds.AddEntity(ctx, mapID, p.WorldEntity(mapUID, s.Items))
	if err != nil {
		return nil, fmt.Errorf("add player to map: %w", err)
	}
	s.Players.Add(p)

	// 落地坐标可能被占地拒绝；读回真实位置
	if e, ok, err := s.Worlds.Entity(ctx, mapID, playerUID); err == nil && ok {
		p.SyncFromWorld(e)
	}

	// ★ 关键：把 AOI 事件转成网络消息，否则玩家进去看不到任何人
	s.dispatchObserverEvents(ctx, p, events)

	s.Logger.Info("player entered world",
		"name", p.Name, "uid", common.GetUIDString(p.UID),
		"map", mapID, "at", fmt.Sprintf("(%d,%d)", p.X, p.Y),
		"aoi_events", len(events))
	return p, nil
}

// dispatchObserverEvents 把 AOI 事件翻译成 SM 消息。
//
// 这层是 B4 留下的「最后一公里」：world 层已经算出了该通知谁，
// 但之前没有任何代码把它变成网络包，所以进图后世界是空的。
//
// 事件语义（internal/world/aoi.go）：
//
//	COAppear      observer 应该看到 target 出现了 -> 推 SM_CORECORD
//	CODisappear   observer 应该把 target 移出视野 -> 推 SM_CODISAPPEAR
//	ObserverAdd   观察者自己开始观察某格（不需直接发消息，实体靠 COAppear 通知）
//	ObserverRemove 观察者不再观察某格（同上）
func (s *WorldService) dispatchObserverEvents(ctx context.Context, self *Player, events []world.ObserverEvent) {
	for _, ev := range events {
		switch ev.Kind {
		case world.COAppear:
			s.sendCOAppear(ctx, ev)
		case world.CODisappear:
			s.sendCODisappear(ctx, ev)
		default:
			// ObserverAdd / ObserverRemove 是观察圈自身变化，
			// 实体出现/消失由 COAppear / CODisappear 承载，这里无需处理
		}
	}
}

// sendCOAppear 把「某个实体出现在你的视野里」发给观察者。
//
// 原版是 AM_OBSERVE：观察者收到后向对方索取 CORecord，
// 拿到坐标/名字/外观后创建本地实体。这里直接推 SM_CORECORD（省一次往返）。
func (s *WorldService) sendCOAppear(ctx context.Context, ev world.ObserverEvent) {
	// 观察者必须是玩家才有连接
	observer, ok := s.Players.Get(ev.Observer)
	if !ok {
		return
	}
	record, err := s.BuildCORecord(ctx, observer.MapID, ev.Target)
	if err != nil {
		return
	}
	record.MapUID = observer.MapUID
	payload, err := protocol.AppendWirePayload(nil, record)
	if err != nil {
		return
	}
	if s.Broadcast != nil {
		_ = s.Broadcast.SendTo(observer.ChannelID, protocol.SM_CORECORD, payload, 0)
	}
}

// sendCODisappear 把「某个实体离开你的视野」发给观察者。
func (s *WorldService) sendCODisappear(ctx context.Context, ev world.ObserverEvent) {
	observer, ok := s.Players.Get(ev.Observer)
	if !ok {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMCODisappear{
		UID:    ev.Target,
		MapUID: observer.MapUID,
	})
	if err != nil {
		return
	}
	if s.Broadcast != nil {
		_ = s.Broadcast.SendTo(observer.ChannelID, protocol.SM_CODISAPPEAR, payload, 0)
	}
}

// BuildCORecord 构造某个实体的 SM_CORECORD。
func (s *WorldService) BuildCORecord(ctx context.Context, mapID uint32, uid uint64) (protocol.SMCORecord, error) {
	e, ok, err := s.Worlds.Entity(ctx, mapID, uid)
	if err != nil {
		return protocol.SMCORecord{}, err
	}
	if !ok {
		return protocol.SMCORecord{}, fmt.Errorf("entity %s not found", common.GetUIDString(uid))
	}

	rec := protocol.SMCORecord{
		UID:    e.UID,
		MapUID: 0, // 同图，调用方补
		Action: common.ActionNode{
			Type:      common.ActionSpawn,
			Direction: uint8(e.Direction),
			X:         int16(e.X),
			Y:         int16(e.Y),
			AimX:      int16(e.X),
			AimY:      int16(e.Y),
		},
	}
	// Union 5 字节按类型解释：玩家放 dbid+level，怪物放 lookID
	switch e.Type {
	case world.EntityPlayer:
		// Player.Union 的布局见 client/src/co_record.gd：
		// 玩家：dbid(4) + level(1)，Level 在偏移 4
		if p, exists := s.Players.Get(uid); exists {
			putUint32(rec.Union[:], p.DBID)
			rec.Union[4] = uint8(p.Level)
		}
	case world.EntityMonster:
		// 怪物：lookID 在低 2 字节（客户端据此找贴图）
		putUint16(rec.Union[:], uint16(common.UIDID(uid)&0xFFFF))
	}
	return rec, nil
}

func putUint32(dst []byte, v uint32) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
}

func putUint16(dst []byte, v uint16) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
}

// Snapshot 返回 uid 视野内可见的实体列表（进场时一次性下发）。
func (s *WorldService) Snapshot(ctx context.Context, mapID uint32, uid uint64) ([]protocol.SMCORecord, error) {
	visible, err := s.Worlds.VisibleEntities(ctx, mapID, uid)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.SMCORecord, 0, len(visible))
	for _, other := range visible {
		rec, err := s.BuildCORecord(ctx, mapID, other)
		if err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// SendSnapshot 把某个玩家视野内的所有实体推给他。
//
// 进场时调用：AOI 事件只覆盖「新进入视野」的增量，
// 而玩家刚进场时周围已经站着的实体不会产生增量事件，需要主动推一次。
func (s *WorldService) SendSnapshot(ctx context.Context, mapID uint32, uid uint64) {
	player, ok := s.Players.Get(uid)
	if !ok {
		return
	}
	records, err := s.Snapshot(ctx, mapID, uid)
	if err != nil || len(records) == 0 {
		return
	}
	for _, rec := range records {
		rec.MapUID = player.MapUID
		payload, err := protocol.AppendWirePayload(nil, rec)
		if err != nil {
			continue
		}
		if s.Broadcast != nil {
			_ = s.Broadcast.SendTo(player.ChannelID, protocol.SM_CORECORD, payload, 0)
		}
	}
	s.Logger.Info("snapshot sent", "name", player.Name, "entities", len(records))
}

// Leave 让玩家离开世界。
//
// 对应原版 AM_OFFLINE / AM_CLOSEMAP：
//  1. 从地图移除实体（产生 CODisappear 事件）
//  2. 把消失事件发给仍能看到他的玩家
//  3. 落库位置（下次登录从这里开始）
//  4. 从 PlayerManager 移除
func (s *WorldService) Leave(ctx context.Context, channelID uint64) {
	player, ok := s.Players.ByChannel(channelID)
	if !ok {
		return
	}

	// 落库：位置与血量。失败不阻断离开流程。
	if s.Chars != nil {
		if err := s.Chars.UpdatePosition(ctx, player.DBID, player.MapID, int32(player.X), int32(player.Y)); err != nil {
			s.Logger.Warn("save position failed", "dbid", player.DBID, "error", err)
		}
		if err := s.Chars.UpdateHealth(ctx, player.DBID, player.HP, player.MP); err != nil {
			s.Logger.Warn("save health failed", "dbid", player.DBID, "error", err)
		}
		if err := s.Chars.UpdateExp(ctx, player.DBID, player.Exp); err != nil {
			s.Logger.Warn("save exp failed", "dbid", player.DBID, "error", err)
		}
	}

	_, removed, events, err := s.Worlds.RemoveEntity(ctx, player.MapID, player.UID)
	if err != nil || !removed {
		s.Logger.Warn("remove player from map failed", "uid", common.GetUIDString(player.UID), "error", err)
	}

	// 广播消失（放在移除 PlayerManager 之前，因为 sendCODisappear 要查观察者）
	s.dispatchObserverEvents(ctx, player, events)

	s.Players.Remove(player.UID)
	s.Logger.Info("player left world", "name", player.Name, "uid", common.GetUIDString(player.UID))
}

// loadPlayerItems 从库里读回角色的背包 / 装备 / 腰带。
//
// 每件物品都带持久值（SDItem.Duration[2]），
// 损坏的装备（Duration[0]==0）加载后依然损坏 —— 不会「重登修复装备」。
func (s *WorldService) loadPlayerItems(ctx context.Context, p *Player) {
	if s.ItemRepo == nil || p == nil {
		return
	}

	if items, err := s.ItemRepo.LoadInventory(ctx, p.DBID); err == nil {
		p.Inventory = make([]Item, 0, len(items))
		for _, it := range items {
			p.Inventory = append(p.Inventory, fromSDItem(it))
		}
	} else if s.Logger != nil {
		s.Logger.Warn("load inventory failed", "dbid", p.DBID, "error", err)
	}

	if wear, err := s.ItemRepo.LoadWear(ctx, p.DBID); err == nil {
		for slot, it := range wear.List {
			if it.ItemID == 0 {
				continue
			}
			item := fromSDItem(it)
			p.Wear[int(slot)] = &item
		}
	} else if s.Logger != nil {
		s.Logger.Warn("load wear failed", "dbid", p.DBID, "error", err)
	}

	if belt, err := s.ItemRepo.LoadBelt(ctx, p.DBID); err == nil {
		for slot, it := range belt.List {
			if it.ItemID == 0 || slot < 0 || slot >= len(p.Belt) {
				continue
			}
			item := fromSDItem(it)
			p.Belt[slot] = &item
		}
	} else if s.Logger != nil {
		s.Logger.Warn("load belt failed", "dbid", p.DBID, "error", err)
	}

	if s.Logger != nil {
		s.Logger.Info("player items loaded", "dbid", p.DBID,
			"inventory", len(p.Inventory), "wear", len(p.Wear))
	}
}
