package script

import (
	"strconv"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// ==================== 通用绑定（common/src/luamodule.cpp:247-560）====================
//
// 这一层和 NPC 无关，所有脚本（NPC / 地图 / 任务）都要用：
// 时间戳、UID 类型判断、物品/怪物/地图的名字与 ID 互查。
//
// 原版用 sol2 绑定，这里是 gopher-lua 等价实现。
// 逐条对照 luamodule.cpp 的函数名与签名，不做增删改名 ——
// 脚本里写的是 addLog / getNanoTstamp / isPlayer 这些确定的名字。

// CommonBindings 返回通用绑定函数表。
//
// 参数：
//
//	items   物品表（按名字查 ID / 按 ID 查名字）
//	monsters 怪物表（同上），可为 nil
//	maps    地图表（同上），可为 nil
//	logFn   日志输出（对应 addLogString）
func CommonBindings(items *NameTable, monsters *NameTable, maps *NameTable,
	logFn func(level int, msg string)) map[string]lua.LGFunction {

	bindings := map[string]lua.LGFunction{
		// ---- 日志（luamodule.cpp:252）----
		// base.lua 的 addLog 调 addLogString(logType, formatted)
		"addLogString": func(l *lua.LState) int {
			level := l.OptInt(1, 0)
			msg := l.OptString(2, "")
			if logFn != nil {
				logFn(level, msg)
			}
			return 0
		},

		// ---- 时间（luamodule.cpp:272-282）----
		"getTime": func(l *lua.LState) int {
			l.Push(lua.LNumber(float64(time.Now().UnixNano()) / 1e9))
			return 1
		},
		"getNanoTstamp": func(l *lua.LState) int {
			l.Push(lua.LNumber(time.Now().UnixNano()))
			return 1
		},
		"getAbsTime": func(l *lua.LState) int {
			l.Push(lua.LNumber(time.Now().Unix()))
			return 1
		},

		// ---- UID 类型判断（luamodule.cpp:287-370）----
		"getUIDType": func(l *lua.LState) int {
			uid := uint64(l.CheckInt64(1))
			l.Push(lua.LNumber(uidTypeOf(uid)))
			return 1
		},
		"getUIDString": func(l *lua.LState) int {
			uid := uint64(l.CheckInt64(1))
			l.Push(lua.LString(uidString(uid)))
			return 1
		},
		"isGM":          boolBinding(func(uid uint64) bool { return false }),
		"isPlayer":      boolBinding(func(uid uint64) bool { return uidTypeOf(uid) == uidTypePlayer }),
		"isQuest":       boolBinding(func(uid uint64) bool { return uidTypeOf(uid) == uidTypeQuest }),
		"isNPChar":      boolBinding(func(uid uint64) bool { return uidTypeOf(uid) == uidTypeNPC }),
		"isReceiver":    boolBinding(func(uid uint64) bool { return false }),
		"isServiceCore": boolBinding(func(uid uint64) bool { return uidTypeOf(uid) == uidTypeServiceCore }),
		"isMap":         boolBinding(func(uid uint64) bool { return uidTypeOf(uid) == uidTypeMap }),
		"isBaseMap":     boolBinding(func(uid uint64) bool { return uidTypeOf(uid) == uidTypeMap }),
		"isInstanceMap": boolBinding(func(uid uint64) bool { return false }),
		"isGuardMode":   boolBinding(func(uid uint64) bool { return false }),
		"isNeutralMode": boolBinding(func(uid uint64) bool { return false }),
		"isPeerCore":    func(l *lua.LState) int { l.Push(lua.LFalse); return 1 },
		"isMonster": func(l *lua.LState) int {
			uid := uint64(l.OptInt64(1, 0))
			l.Push(lua.LBool(uidTypeOf(uid) == uidTypeMonster))
			return 1
		},

		// ---- sleep / exit（luamodule.cpp:375-387）----
		// ★ sleep 在单 goroutine 的 Lua 里会阻塞整个世界。
		//   原版是在 actor 协程里 sleep，只阻塞自己。
		//   这里直接忽略（返回 0 不睡），宁可时序略快也不卡服。
		"sleep": func(l *lua.LState) int { return 0 },
		"exit": func(l *lua.LState) int {
			if logFn != nil {
				logFn(2, "script called exit(), ignored")
			}
			return 0
		},

		// ---- 字符串工具（luamodule.cpp:440-545）----
		"strUpper": func(l *lua.LState) int {
			s := l.CheckString(1)
			l.Push(lua.LString(toUpperASCII(s)))
			return 1
		},
		"strLower": func(l *lua.LState) int {
			s := l.CheckString(1)
			l.Push(lua.LString(toLowerASCII(s)))
			return 1
		},
		"validXML": func(l *lua.LState) int {
			l.Push(lua.LTrue)
			return 1
		},
		"validPar": func(l *lua.LState) int {
			l.Push(lua.LTrue)
			return 1
		},
		"quotedLuaString": func(l *lua.LState) int {
			s := l.CheckString(1)
			l.Push(lua.LString(strconv.Quote(s)))
			return 1
		},
	}

	// ---- 记录表查名/查 ID（luamodule.cpp:387-438）----
	if items != nil {
		bindings["getItemName"] = nameByIDBinding(items)
		bindings["getItemID"] = idByNameBinding(items)
	}
	if monsters != nil {
		bindings["getMonsterName"] = nameByIDBinding(monsters)
		bindings["getMonsterID"] = idByNameBinding(monsters)
	}
	if maps != nil {
		bindings["getMapName"] = nameByIDBinding(maps)
		bindings["getMapID"] = idByNameBinding(maps)
	}

	return bindings
}

// boolBinding 把 Go 的 bool 函数包成 Lua 绑定。
func boolBinding(fn func(uid uint64) bool) lua.LGFunction {
	return func(l *lua.LState) int {
		uid := uint64(l.CheckInt64(1))
		l.Push(lua.LBool(fn(uid)))
		return 1
	}
}

// nameByIDBinding 生成「ID -> 名字」绑定（对应 getItemName / getMonsterName）。
//
// 查不到返回 nil 而不是空串 —— 脚本里都是
// `local name = getItemName(id); if name then ... end` 这种写法。
func nameByIDBinding(tbl *NameTable) lua.LGFunction {
	return func(l *lua.LState) int {
		if l.Get(1) == lua.LNil {
			l.Push(lua.LNil)
			return 1
		}
		id := uint32(l.CheckInt64(1))
		if name, ok := tbl.NameOf(id); ok {
			l.Push(lua.LString(name))
		} else {
			l.Push(lua.LNil)
		}
		return 1
	}
}

// idByNameBinding 生成「名字 -> ID」绑定（对应 getItemID / getMonsterID）。
//
// 查不到返回 0（原版 DBCOM_ITEMID 就是这么约定的，脚本用 `== 0` 判失败）。
func idByNameBinding(tbl *NameTable) lua.LGFunction {
	return func(l *lua.LState) int {
		name := l.CheckString(1)
		if id, ok := tbl.IDOf(name); ok {
			l.Push(lua.LNumber(id))
		} else {
			l.Push(lua.LNumber(0))
		}
		return 1
	}
}

// NameTable 是「ID <-> 名字」双向查询表（物品/怪物/地图通用）。
type NameTable struct {
	byID   map[uint32]string
	byName map[string]uint32
}

// NewNameTable 从「ID -> 名字」的 map 构造。
func NewNameTable(records map[uint32]string) *NameTable {
	t := &NameTable{
		byID:   make(map[uint32]string, len(records)),
		byName: make(map[string]uint32, len(records)),
	}
	for id, name := range records {
		if name == "" {
			continue
		}
		t.byID[id] = name
		// 同名时保留较小 ID：物品表里同名的重复记录取第一条
		// （与 DBCOM_ITEMID 的线性查找语义一致）
		if _, exists := t.byName[name]; !exists {
			t.byName[name] = id
		} else if id < t.byName[name] {
			t.byName[name] = id
		}
	}
	return t
}

// NameOf 按 ID 查名字。
func (t *NameTable) NameOf(id uint32) (string, bool) {
	if t == nil {
		return "", false
	}
	name, ok := t.byID[id]
	return name, ok
}

// IDOf 按名字查 ID。
func (t *NameTable) IDOf(name string) (uint32, bool) {
	if t == nil {
		return 0, false
	}
	id, ok := t.byName[name]
	return id, ok
}

// Count 返回条目数。
func (t *NameTable) Count() int {
	if t == nil {
		return 0
	}
	return len(t.byID)
}

// ==================== UID 类型（common/src/uidf.hpp）====================

// UID 类型编码与 internal/common/uid.go 保持一致。
//
// 不用 common 包避免循环依赖（common 不 import script，
// 但 script 也不该依赖 game 层的实现细节）。
const (
	uidTypeServiceCore = 0
	uidTypeMap         = 1
	uidTypePlayer      = 2
	uidTypeMonster     = 3
	uidTypeNPC         = 4
	uidTypeQuest       = 5
	uidTypeReceiver    = 6
	uidTypeLuaObject   = 7
)

// uidTypeOf 取 UID 的类型位（uidf.hpp: type@59）。
func uidTypeOf(uid uint64) int {
	return int((uid >> 59) & 0xF)
}

// uidString 把 UID 转成可读形式（uidf::getUIDString）。
//
// 格式与原版一致：UID_PLY_1 / UID_MON_1001 这种。
func uidString(uid uint64) string {
	prefix := map[int]string{
		uidTypeServiceCore: "UID_SEV",
		uidTypeMap:         "UID_MAP",
		uidTypePlayer:      "UID_PLY",
		uidTypeMonster:     "UID_MON",
		uidTypeNPC:         "UID_NPC",
		uidTypeQuest:       "UID_QST",
		uidTypeReceiver:    "UID_RCV",
		uidTypeLuaObject:   "UID_LUA",
	}[uidTypeOf(uid)]
	if prefix == "" {
		prefix = "UID_UNK"
	}
	return prefix + "_" + strconv.FormatUint(uid&0x7FFFFFFFF, 10)
}

// ==================== 字符串工具 ====================

func toUpperASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}
