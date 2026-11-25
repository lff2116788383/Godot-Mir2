package game

import (
	"encoding/json"
	"fmt"
	"os"
)

// ==================== 物品表（common/src/itemrecord.hpp）====================
//
// 数据来自 `tools/extract/extract_items.py` 从 itemrecord.inc 提取的
// content/items.json，itemID = .inc 数组下标（第 0 项是空占位，所以从 1 开始）。
//
// ★ itemID 必须与客户端 data/itemrecord.json 完全一致
//   （已验证 1258 条逐条 name/pkgGfxID 对上）。
//   两边错一个 ID，客户端就会显示成别的图标 —— 这类问题很难从日志看出来。
//
// 本批只提取了逻辑需要的字段：type / weight / price / potion / book。
// 装备的战斗属性（dc/ac/mc 等）留给后续批次，因为穿戴校验只看 type。

// ItemPotion 是恢复药水的效果（itemrecord.hpp PotionAttribute）。
type ItemPotion struct {
	HP   int `json:"hp"`
	MP   int `json:"mp"`
	Time int `json:"time"`
}

// ItemBook 是技能书（itemrecord.hpp BookAttribute）。
type ItemBook struct {
	Job   string `json:"job"`
	Level int    `json:"level"`
	// Magic 只有「秘籍」才有，普通技能书是原料，不能直接学
	Magic string `json:"magic"`
}

// ItemExtHealth 是装备对 HP/MP 的三项修正（itemrecord.hpp ExtHealth）。
type ItemExtHealth struct {
	Add     int `json:"add"`
	Steal   int `json:"steal"`
	Recover int `json:"recover"`
}

// ItemEquip 是装备的战斗属性（itemrecord.hpp:20-100 EquipAttribute）。
//
// ★ 这些字段是服务端算玩家属性的唯一来源。
//
//	getCombatNode 把穿的每件装备的 dc/ac/mc/sc 累加，再加等级基础值。
//	如果物品表里没提 equip，玩家穿什么都只有等级属性 —— 表现就是
//	「换了把屠龙刀攻击力一点没变」，而且从日志完全看不出来。
type ItemEquip struct {
	Duration  int            `json:"duration"`
	DC        [2]int         `json:"dc"`
	MC        [2]int         `json:"mc"`
	SC        [2]int         `json:"sc"`
	AC        [2]int         `json:"ac"`
	MAC       [2]int         `json:"mac"`
	DCHit     int            `json:"dcHit"`
	MCHit     int            `json:"mcHit"`
	DCDodge   int            `json:"dcDodge"`
	MCDodge   int            `json:"mcDodge"`
	Speed     int            `json:"speed"`
	Comfort   int            `json:"comfort"`
	LuckCurse int            `json:"luckCurse"`
	HP        *ItemExtHealth `json:"hp,omitempty"`
	MP        *ItemExtHealth `json:"mp,omitempty"`
	ReqLevel  int            `json:"reqLevel"`
}

// ItemRecord 是一条物品定义。
type ItemRecord struct {
	ItemID   uint32      `json:"itemID"`
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Weight   int         `json:"weight"`
	Price    int         `json:"price"`
	PkgGfxID uint32      `json:"pkgGfxID"`
	Shape    int         `json:"shape"`
	Potion   *ItemPotion `json:"potion,omitempty"`
	Book     *ItemBook   `json:"book,omitempty"`
	Equip    *ItemEquip  `json:"equip,omitempty"`
}

// 物品类型常量（itemrecord.hpp 的 type 字符串）。
//
// ★ 原版的 wearable/beltable/isPotion/isBook 全是**按 type 字符串匹配**的，
//
//	不是按数字枚举。所以这里也用字符串常量，避免自己发明一套 enum 再映射回去。
const (
	TypeGold       = "金币"
	TypePotion     = "恢复药水"
	TypeScroll     = "传送卷轴"
	TypeCharm      = "护身符"
	TypePowder     = "药粉"
	TypeBook       = "技能书"
	TypeDope       = "强效药水"
	TypeDress      = "衣服"
	TypeHelmet     = "头盔"
	TypeWeapon     = "武器"
	TypeShoes      = "鞋"
	TypeNecklace   = "项链"
	TypeArmRing    = "手镯"
	TypeRing       = "戒指"
	TypeTorch      = "火把"
	TypeCharmExtra = "魅力"
)

// 装备槽位（protocoldef.hpp WearLookGridType）。
//
//	WLG_DRESS=1, HELMET=2, WEAPON=3, SHOES=4, NECKLACE=5,
//	ARMRING0=6, ARMRING1=7, RING0=8, RING1=9, TORCH=10, CHARM=11
//
// ★ 顺序不能凭记忆写：DRESS/HELMET/WEAPON 在 L 段，SHOES 之后才是 W 段。
const (
	WLGNone     = 0
	WLGDress    = 1
	WLGHelmet   = 2
	WLGWeapon   = 3
	WLGShoes    = 4
	WLGNecklace = 5
	WLGArmRing0 = 6
	WLGArmRing1 = 7
	WLGRing0    = 8
	WLGRing1    = 9
	WLGTorch    = 10
	WLGCharm    = 11
	WLGBegin    = WLGDress
	WLGEnd      = WLGCharm + 1
)

// WLGValid 判断槽位是否合法。
func WLGValid(wltype int) bool {
	return wltype >= WLGBegin && wltype < WLGEnd
}

// Wearable 判断某物品能否穿到指定槽位（itemrecord.hpp:169-192）。
//
// 逐槽照抄原版：手镯/戒指各有两个槽位（0/1）但类型相同；
// 护身符槽接受「魅力|护身符|药粉」三类（原版是子串查找）。
func (r ItemRecord) Wearable(wltype int) bool {
	switch wltype {
	case WLGDress:
		return r.Type == TypeDress
	case WLGHelmet:
		return r.Type == TypeHelmet
	case WLGWeapon:
		return r.Type == TypeWeapon
	case WLGShoes:
		return r.Type == TypeShoes
	case WLGNecklace:
		return r.Type == TypeNecklace
	case WLGArmRing0, WLGArmRing1:
		return r.Type == TypeArmRing
	case WLGRing0, WLGRing1:
		return r.Type == TypeRing
	case WLGTorch:
		return r.Type == TypeTorch
	case WLGCharm:
		// 原版：u8"魅力|护身符|药粉".find(type) != npos，即三类之一
		return r.Type == TypeCharmExtra || r.Type == TypeCharm || r.Type == TypePowder
	default:
		return false
	}
}

// Packable 判断物品能否堆叠（itemrecord.hpp:159-166）。
//
// ★ 只有这四类可堆叠，且单格上限 SYS_INVGRIDMAXHOLD = 99。
//
//	不可堆叠的物品（武器/衣服/戒指…）即使 itemID 相同也**永远各占一格**，
//	必须靠 seqID 区分 —— 这是背包去重的基础。
func (r ItemRecord) Packable() bool {
	return r.Type == TypePotion ||
		r.Type == TypeScroll ||
		r.Type == TypeCharm ||
		r.Type == TypePowder
}

// Beltable 判断能否放腰带（itemrecord.hpp:161-167）。
//
// ★ 只有「恢复药水」和「传送卷轴」两类。强效药水、药粉、护身符都**不能**上腰带。
func (r ItemRecord) Beltable() bool {
	return r.Type == TypePotion || r.Type == TypeScroll
}

// IsPotion 判断是否是可直接喝的恢复药水。
func (r ItemRecord) IsPotion() bool { return r.Type == TypePotion }

// IsBook 判断是否是技能书。
func (r ItemRecord) IsBook() bool { return r.Type == TypeBook }

// IsMagicBook 判断是否是「秘籍」（能真正学会魔法的书）。
//
// 普通技能书是原料：怪物掉的/商店买的都没有 magic 字段，
// 必须由「武功教头」抄写成秘籍才能学。
func (r ItemRecord) IsMagicBook() bool {
	return r.IsBook() && r.Book != nil && r.Book.Magic != ""
}

// IsGold 判断是否是金币物品。
func (r ItemRecord) IsGold() bool { return r.Type == TypeGold }

// ItemDB 是物品表。
type ItemDB struct {
	records map[uint32]*ItemRecord
	// goldID 「金币」物品的 itemID 缓存（懒解析，见 GoldID）
	goldID uint32
}

// LoadItemDB 从 JSON 文件加载物品表。
func LoadItemDB(path string) (*ItemDB, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read items: %w", err)
	}
	var payload struct {
		Records map[string]*ItemRecord `json:"records"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse items: %w", err)
	}

	db := &ItemDB{records: make(map[uint32]*ItemRecord, len(payload.Records))}
	for key, rec := range payload.Records {
		if rec == nil {
			continue
		}
		// 以 JSON 里的 itemID 为准；缺省时用 map key 解析
		id := rec.ItemID
		if id == 0 {
			var parsed uint32
			if _, err := fmt.Sscanf(key, "%d", &parsed); err == nil {
				id = parsed
			}
		}
		if id == 0 {
			continue
		}
		rec.ItemID = id
		db.records[id] = rec
	}
	if len(db.records) == 0 {
		return nil, fmt.Errorf("item db is empty")
	}
	return db, nil
}

// Get 按 itemID 取物品记录。
func (db *ItemDB) Get(itemID uint32) (ItemRecord, bool) {
	if db == nil {
		return ItemRecord{}, false
	}
	rec, ok := db.records[itemID]
	if !ok {
		return ItemRecord{}, false
	}
	return *rec, true
}

// Count 返回物品总数。
func (db *ItemDB) Count() int {
	if db == nil {
		return 0
	}
	return len(db.records)
}

// FindByName 按名字查 itemID（GM 命令/测试用）。
func (db *ItemDB) FindByName(name string) (uint32, bool) {
	if db == nil {
		return 0, false
	}
	for id, rec := range db.records {
		if rec.Name == name {
			return id, true
		}
	}
	return 0, false
}

// GoldID 返回「金币」这一物品的 itemID（找不到时返回 0）。
//
// ★ 金币不是一条记录，而是 5 条面额：金币（小/中/大/特/超），
//
//	itemID 1..5。这里必须返回**面额最小的那条（itemID 最小）**，
//	不能随便取一条。
//
// 为什么不能"随便一条"：
//
//	GoldID 决定 costList 里金币用哪个 itemID 表示。
//	若用金币（超）(id=5) 当"计价单位"，玩家付 80 金币时会写成
//	"5 号物品 x80"，而 addGold 又按另一条面额找，两边对不上 ——
//	表现是"扣了钱但金币数不变"，或者反过来。
//
// ★ 必须按 itemID 升序取，因为 Go 的 map 遍历顺序是随机的。
//
//	初版写成 `for id := range db.records { if rec.Type == 金币 { return id } }`，
//	每次进程启动返回的 ID 都不一样（实测同一份数据两次启动分别是 1 和 4）。
//	这是最难查的一类 bug：单进程内自洽，重启后行为变化。
//
// 缓存到 db 上：每次购买都线性扫 1258 条太浪费。
func (db *ItemDB) GoldID() uint32 {
	if db == nil {
		return 0
	}
	if db.goldID != 0 {
		return db.goldID
	}
	var best uint32
	for id, rec := range db.records {
		if rec.Type != TypeGold {
			continue
		}
		if best == 0 || id < best {
			best = id
		}
	}
	db.goldID = best
	return best
}
