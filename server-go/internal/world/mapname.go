package world

import "sync"

// ==================== 地图 ID <-> 名字 ====================
//
// NPC 脚本文件名用地图**名字**（道馆_1.阿天_1.GLOC_396_116_0.LOOK_18.lua），
// 而服务端按 mapID 索引地图。两边要能互查。
//
// 数据来自 maprecord.inc（603 条，第 0 项是空占位）。

var (
	mapNameMu   sync.RWMutex
	mapIDToName = map[uint32]string{}
	nameToMapID = map[string]uint32{}
)

// SetMapNames 注入地图名表（启动时调一次）。
func SetMapNames(names map[uint32]string) {
	mapNameMu.Lock()
	defer mapNameMu.Unlock()
	mapIDToName = make(map[uint32]string, len(names))
	nameToMapID = make(map[string]uint32, len(names))
	for id, name := range names {
		if name == "" {
			continue
		}
		mapIDToName[id] = name
		if _, exists := nameToMapID[name]; !exists {
			nameToMapID[name] = id
		}
	}
}

// MapNameOf 按 mapID 取地图名（找不到返回空串）。
func MapNameOf(mapID uint32) string {
	mapNameMu.RLock()
	defer mapNameMu.RUnlock()
	return mapIDToName[mapID]
}

// MapIDOf 按名字取 mapID（找不到返回 0）。
func MapIDOf(name string) uint32 {
	mapNameMu.RLock()
	defer mapNameMu.RUnlock()
	return nameToMapID[name]
}

// MapCount 返回已登记的地图数。
func MapCount() int {
	mapNameMu.RLock()
	defer mapNameMu.RUnlock()
	return len(mapIDToName)
}
