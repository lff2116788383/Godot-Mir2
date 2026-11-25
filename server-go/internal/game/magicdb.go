package game

import (
	"encoding/json"
	"fmt"
	"os"
)

// ==================== 魔法表（common/src/magicrecord.hpp）====================
//
// 数据来自 `tools/extract/extract_magics.py` 从 magicrecord.inc 提取的
// content/magics.json，magicID = .inc 顶层数组下标（第 0 项是空占位）。
//
// ★ magicID 必须与客户端 data/magicrecord.json 完全一致
//   （已验证 116 条逐条 name / mp / coolDown 对上）。
//   客户端发 CM_ACTION(ACTION_SPELL) 时，extParam.magicID 就是这个 ID；
//   服务端查不到记录就只能把整个施法动作丢掉，表现为「按下技能没反应」。
//
// ★ 不是所有记录都是玩家技能：
//   - 带 req.job 的 63 条才是真技能（战士 13 / 法师 27 / 道士 23）
//   - 其余是「魔法特效_*」这类给 gfxList 引用的共享特效，
//     以及怪物的专属魔法（如「诺玛法老_火球术」）
//   learnable 字段把这条派生的判断留在数据里，服务端据此拒绝
//   「学了特效条目」这种脏数据。

// MagicElem 是元素类型（magicrecord.hpp:38 magicElemID）。
type MagicElem string

const (
	ElemNone    MagicElem = ""
	ElemFire    MagicElem = "fire"    // 火
	ElemIce     MagicElem = "ice"     // 冰
	ElemLight   MagicElem = "light"   // 雷
	ElemWind    MagicElem = "wind"    // 风
	ElemHoly    MagicElem = "holy"    // 神圣
	ElemDark    MagicElem = "dark"    // 暗黑
	ElemPhantom MagicElem = "phantom" // 幻影
)

// CastRangeType 是 DCCastRangeType（magicrecord.hpp:256-261）。
const (
	CastRangeNone    = 0
	CastRangeDir     = 1 // CRT_DIR：朝向前方 distance 格
	CastRangeLong    = 2 // CRT_LONG：无距离限制（默认值）
	CastRangeLimited = 3 // CRT_LIMITED：distance 格内任意目标
)

// MagicReq 是学习/升级的等级与职业要求。
type MagicReq struct {
	Level [3]int `json:"level"`
	Train [3]int `json:"train"`
	Job   string `json:"job"`
	Prior string `json:"prior"`
}

// MagicCastRange 是施法距离约束。
type MagicCastRange struct {
	Type     string `json:"type"`
	TypeID   int    `json:"typeID"`
	Distance int    `json:"distance"`
}

// MagicRecord 是一条魔法定义。
type MagicRecord struct {
	MagicID     uint32         `json:"magicID"`
	Name        string         `json:"name"`
	Elem        string         `json:"elem"`
	ElemID      MagicElem      `json:"elemID"`
	Req         MagicReq       `json:"req"`
	CastRange   MagicCastRange `json:"castRange"`
	CastDelay   int            `json:"castDelay"`
	CoolDown    int            `json:"coolDown"`
	CheckGround int            `json:"checkGround"`
	MP          int            `json:"mp"`
	MPInc       int            `json:"mpInc"`
	Power       [2]int         `json:"power"`
	AddPower    [2]int         `json:"addPower"`
	SeffBase    *int           `json:"seffBase"`
	Learnable   bool           `json:"learnable"`
}

// MagicDB 是只读魔法表。
type MagicDB struct {
	records map[uint32]*MagicRecord
	byName  map[string]uint32
}

// LoadMagicDB 从 JSON 载入魔法表。
func LoadMagicDB(path string) (*MagicDB, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read magic db %s: %w", path, err)
	}
	var file struct {
		Records map[string]MagicRecord `json:"records"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse magic db %s: %w", path, err)
	}

	db := &MagicDB{
		records: make(map[uint32]*MagicRecord, len(file.Records)),
		byName:  make(map[string]uint32, len(file.Records)),
	}
	for key, rec := range file.Records {
		id := rec.MagicID
		if id == 0 {
			// 空占位记录没有 name，跳过；只有 name 非空才可能是有效记录
			if rec.Name == "" {
				continue
			}
			var parsed uint32
			if _, err := fmt.Sscanf(key, "%d", &parsed); err == nil {
				id = parsed
			}
		}
		if id == 0 {
			continue
		}
		rec.MagicID = id
		stored := rec
		db.records[id] = &stored
		if rec.Name != "" {
			db.byName[rec.Name] = id
		}
	}
	if len(db.records) == 0 {
		return nil, fmt.Errorf("magic db is empty")
	}
	return db, nil
}

// Get 按 magicID 取魔法记录。
func (db *MagicDB) Get(magicID uint32) (MagicRecord, bool) {
	if db == nil {
		return MagicRecord{}, false
	}
	rec, ok := db.records[magicID]
	if !ok {
		return MagicRecord{}, false
	}
	return *rec, true
}

// FindByName 按名字查 magicID。
func (db *MagicDB) FindByName(name string) (uint32, bool) {
	if db == nil {
		return 0, false
	}
	id, ok := db.byName[name]
	return id, ok
}

// Count 返回记录总数（含不可学的特效条目）。
func (db *MagicDB) Count() int {
	if db == nil {
		return 0
	}
	return len(db.records)
}

// LearnableCount 返回玩家可学技能数。
func (db *MagicDB) LearnableCount() int {
	if db == nil {
		return 0
	}
	count := 0
	for _, rec := range db.records {
		if rec.Learnable {
			count++
		}
	}
	return count
}

// MPCost 返回某技能在指定修炼等级下的 MP 消耗。
//
// 原版公式（magicrecord.hpp 的 mp/mpInc 语义）：
//
//	cost = mp + mpInc * trainLevel
//
// trainLevel 是技能修炼级数 0..3（对应 req.train 的三个门槛）。
// 玩家只学了没练过时是 0 级，只付基础 mp。
func (r MagicRecord) MPCost(trainLevel int) int {
	if trainLevel < 0 {
		trainLevel = 0
	}
	if trainLevel > 3 {
		trainLevel = 3
	}
	cost := r.MP + r.MPInc*trainLevel
	if cost < 0 {
		return 0
	}
	return cost
}
