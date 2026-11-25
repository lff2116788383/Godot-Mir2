package game

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// BuffDB 是 buff 静态配置表（来自 buffrecord.inc，21 条）。
//
// 【为什么需要它】
//
//	之前 buff 只有「时长」，没有任何实际效果 ——
//	玩家中了「施毒术」（HP -20%）血条不变，
//	上了「治愈术」（HP +20%）上限也不变。
//	buff 图标会出现在头顶，但完全不影响战斗数值。
type BuffDB struct {
	mu      sync.RWMutex
	records map[uint32]world.BuffRecord
}

// BuffDef 是一条 buff 的 JSON 结构（与 extract_buffs.py 的产出对应）。
type BuffDef struct {
	BuffID      uint32             `json:"buffID"`
	Name        string             `json:"name"`
	Favor       int                `json:"favor"`
	Dispellable bool               `json:"dispellable"`
	StackCount  int                `json:"stackCount"`
	Attrs       []BuffAttrModifier `json:"attrs"`
}

// BuffAttrModifier 是一条属性修正。
type BuffAttrModifier struct {
	Attr       string `json:"attr"`
	Value      int    `json:"value"`
	Percentage int    `json:"percentage"`
}

// LoadBuffDB 从 JSON 加载 buff 表。
func LoadBuffDB(path string) (*BuffDB, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read buff db %s: %w", path, err)
	}
	var file struct {
		Records map[string]BuffDef `json:"records"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse buff db %s: %w", path, err)
	}

	db := &BuffDB{records: make(map[uint32]world.BuffRecord, len(file.Records))}
	for key, def := range file.Records {
		var id uint32
		if _, err := fmt.Sscanf(key, "%d", &id); err != nil {
			continue
		}
		acts := make([]world.BuffAttributeModifier, 0, len(def.Attrs))
		for _, a := range def.Attrs {
			acts = append(acts, world.BuffAttributeModifier{
				Attr: a.Attr, Value: a.Value, Percentage: a.Percentage,
			})
		}
		db.records[id] = world.BuffRecord{
			Name:        def.Name,
			Favor:       def.Favor,
			Dispellable: def.Dispellable,
			StackCount:  def.StackCount,
			Acts:        acts,
		}
	}
	return db, nil
}

// Get 按 buffID 取配置。
func (db *BuffDB) Get(buffID uint32) (world.BuffRecord, bool) {
	if db == nil {
		return world.BuffRecord{}, false
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	rec, ok := db.records[buffID]
	return rec, ok
}

// Count 返回 buff 条数。
func (db *BuffDB) Count() int {
	if db == nil {
		return 0
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.records)
}

// BindBuffResolver 把 buff 表接到 world 的查表钩子上。
//
// ★ 一次性绑定，不在每个 buff 加的时候设：
//
//	world.buffResolver 是包级变量，每帧重设就是数据竞争，
//	和元素抗性解析器（BindElementResolver）是同一类问题。
func BindBuffResolver(db *BuffDB) {
	if db == nil {
		return
	}
	world.SetBuffResolver(func(buffID uint32) (world.BuffRecord, bool) {
		return db.Get(buffID)
	})
}
