package world

import (
	"encoding/json"
	"fmt"
	"os"
)

// ==================== 怪物表（common/src/monsterrecord.hpp）====================
//
// 数据来自 `tools/extract/extract_monsters.py` 从 monsterrecord.inc 提取的
// content/monsters.json，monsterID = .inc 顶层数组下标（第 0 项是空占位）。
//
// ★ 与既有 world.MonsterRecord 的关系：
//
//	world/monster.go 里的 MonsterRecord 是「AI 与掉落需要的部分」，
//	结构精简过。这里的 MonsterDef 是**完整记录**（含 hp/mp/dc/ac/元素抗性），
//	刷怪时由 Def.ToRecord() 转成前者供 AI 用，同时用完整字段初始化实体。
//
//	不合并成一个类型的原因：MonsterRecord 被 AI 层广泛引用，
//	加一堆用不到的字段会让 AI 代码噪音变大；而刷怪需要完整数值。
//	两者职责不同，边界清楚比"少一个类型"重要。

// MonsterDef 是一条完整的怪物定义。
type MonsterDef struct {
	MonsterID uint32 `json:"monsterID"`
	Name      string `json:"name"`
	LookID    int    `json:"lookID"`
	HP        int32  `json:"hp"`
	MP        int32  `json:"mp"`
	Exp       uint64 `json:"exp"`

	DC  [2]int `json:"dc"`
	MC  [2]int `json:"mc"`
	AC  [2]int `json:"ac"`
	MAC [2]int `json:"mac"`

	DCHit   int `json:"dcHit"`
	MCHit   int `json:"mcHit"`
	DCDodge int `json:"dcDodge"`
	MCDodge int `json:"mcDodge"`

	View       int `json:"view"`
	WalkWait   int `json:"walkWait"`
	WalkStep   int `json:"walkStep"`
	AttackWait int `json:"attackWait"`

	ACElem map[string]int  `json:"acElem,omitempty"`
	Drops  [][]MonsterDrop `json:"dropItemList"`
}

// MonsterDrop 是一条掉落配置（monsterrecord.hpp:104-109）。
type MonsterDrop struct {
	Item      string `json:"item"`
	ProbRecip int    `json:"probRecip"`
	Count     int    `json:"count"`
	Repeat    int    `json:"repeat"`
}

// MonsterDB 是只读怪物表。
type MonsterDB struct {
	records map[uint32]*MonsterDef
	byName  map[string]uint32
}

// LoadMonsterDB 从 JSON 载入怪物表。
func LoadMonsterDB(path string) (*MonsterDB, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read monster db %s: %w", path, err)
	}
	var file struct {
		Records map[string]MonsterDef `json:"records"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse monster db %s: %w", path, err)
	}

	db := &MonsterDB{
		records: make(map[uint32]*MonsterDef, len(file.Records)),
		byName:  make(map[string]uint32, len(file.Records)),
	}
	for key, rec := range file.Records {
		id := rec.MonsterID
		if id == 0 {
			if rec.Name == "" {
				continue
			}
			var parsed uint32
			if _, err := fmt.Sscanf(key, "%d", &parsed); err == nil {
				id = parsed
			}
		}
		if id == 0 || rec.Name == "" {
			continue
		}
		rec.MonsterID = id
		stored := rec
		db.records[id] = &stored
		// 同名只保留最小 ID（与 DBCOM_MONSTERID 的线性查找语义一致）
		if existing, exists := db.byName[rec.Name]; !exists || id < existing {
			db.byName[rec.Name] = id
		}
	}
	if len(db.records) == 0 {
		return nil, fmt.Errorf("monster db is empty")
	}
	return db, nil
}

// Get 按 monsterID 取记录。
func (db *MonsterDB) Get(monsterID uint32) (MonsterDef, bool) {
	if db == nil {
		return MonsterDef{}, false
	}
	rec, ok := db.records[monsterID]
	if !ok {
		return MonsterDef{}, false
	}
	return *rec, true
}

// FindByName 按名字查 monsterID。
func (db *MonsterDB) FindByName(name string) (uint32, bool) {
	if db == nil {
		return 0, false
	}
	id, ok := db.byName[name]
	return id, ok
}

// Count 返回怪物总数。
func (db *MonsterDB) Count() int {
	if db == nil {
		return 0
	}
	return len(db.records)
}

// NameTable 返回「ID -> 名字」映射（供 Lua 的 getMonsterName / getMonsterID 用）。
func (db *MonsterDB) NameTable() map[uint32]string {
	out := make(map[uint32]string, db.Count())
	for id, rec := range db.records {
		out[id] = rec.Name
	}
	return out
}

// ToRecord 转成 AI/掉落层用的精简记录。
//
// 掉落表要逐字段搬过去：RollDrop 用的是 MonsterRecord.DropItems，
// 少了这一步「打死怪掉一地东西」就变成「打死怪什么都没有」。
func (d MonsterDef) ToRecord() MonsterRecord {
	groups := make([][]DropItem, 0, len(d.Drops))
	for _, group := range d.Drops {
		items := make([]DropItem, 0, len(group))
		for _, drop := range group {
			items = append(items, DropItem{
				Item:      drop.Item,
				ProbRecip: drop.ProbRecip,
				Count:     drop.Count,
				Repeat:    drop.Repeat,
			})
		}
		if len(items) > 0 {
			groups = append(groups, items)
		}
	}

	return MonsterRecord{
		Name:       d.Name,
		LookID:     d.LookID,
		HP:         d.HP,
		MP:         d.MP,
		Exp:        d.Exp,
		DC:         AbilityBounds{Min: d.DC[0], Max: d.DC[1]},
		MC:         AbilityBounds{Min: d.MC[0], Max: d.MC[1]},
		AC:         AbilityBounds{Min: d.AC[0], Max: d.AC[1]},
		MAC:        AbilityBounds{Min: d.MAC[0], Max: d.MAC[1]},
		DCHit:      d.DCHit,
		MCHit:      d.MCHit,
		DCDodge:    d.DCDodge,
		MCDodge:    d.MCDodge,
		View:       d.View,
		WalkWait:   d.WalkWait,
		WalkStep:   d.WalkStep,
		AttackWait: d.AttackWait,
		DropItems:  groups,
		Elem:       d.elementResist(),
	}
}

// elementResist 把 acElem 映射转成 ElementResist。
func (d MonsterDef) elementResist() ElementResist {
	get := func(key string) int {
		if d.ACElem == nil {
			return 0
		}
		return d.ACElem[key]
	}
	return ElementResist{
		Fire:    get("fire"),
		Ice:     get("ice"),
		Light:   get("light"),
		Wind:    get("wind"),
		Holy:    get("holy"),
		Dark:    get("dark"),
		Phantom: get("phantom"),
	}
}

// CombatStatsFor 用完整数值构造受击结算属性。
//
// 与 CombatStatsOf(rec MonsterRecord) 的区别：
//
//	那个只填 AI 需要的字段，这个连元素抗性一起填。
//	不打元素伤害时两者等价，但法师的雷电术/火球术全靠它算减伤。
func (d MonsterDef) CombatStatsFor() CombatStats {
	return CombatStats{
		DC:      AbilityBounds{Min: d.DC[0], Max: d.DC[1]},
		MC:      AbilityBounds{Min: d.MC[0], Max: d.MC[1]},
		SC:      AbilityBounds{},
		DCHit:   d.DCHit,
		MCHit:   d.MCHit,
		DCDodge: d.DCDodge,
		MCDodge: d.MCDodge,
		AC:      AbilityBounds{Min: d.AC[0], Max: d.AC[1]},
		MAC:     AbilityBounds{Min: d.MAC[0], Max: d.MAC[1]},
		Elem:    d.elementResist(),
	}
}
