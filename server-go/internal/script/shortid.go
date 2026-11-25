package script

import (
	"sync"
)

// ==================== 短 ID 映射（解决 Lua 5.1 浮点精度）====================
//
// ★★ 为什么必须有这一层 ★★
//
// gopher-lua 是 Lua 5.1，它的 number 就是 float64：
//
//	精确整数上限 2^53 = 9007199254740992
//
// 而本项目的所有 UID 都超过这个值：
//
//	玩家 PLY_1 = 2882303761517117441
//	任务 QST_1 = 4035225266123964417
//	地图 MAP_1 = 1152921538966585345
//
// 实测在 Lua 里写 1152921538966585345 会变成 1152921538966585344（差 1）。
// 差 1 的后果是 uidRemoteCall 找到「另一个不存在的 actor」，
// 报错 "target not found" —— 而这个报错完全不提精度问题，
// 会让人误以为「actor 没注册」，排查方向整个错。
//
// ★ 解决方式：Lua 只看「短 ID」
//
//	短 ID 从 1 开始递增，永远不会超过 2^53，
//	Go 边界负责 短ID ↔ 真实UID 的双向转换。
//
//	原版任务脚本里 UID 只是一个不透明的句柄：
//	  local npcUID = getNPCharUID('比奇县_0', '客栈店员_1')
//	  uidRemoteCall(npcUID, ...)
//	  它从不解析 UID 的位结构，也不用它做算术。
//	所以把 64 位换成小整数，脚本**一行都不用改**。
//
// ★ 为什么不换成字符串（另一个可行方案）
//
//	脚本里到处是 assertType(uid, 'integer')，
//	字符串会全线触发断言失败。而小整数仍是 integer，语义不变。

// ShortIDMap 维护 短ID ↔ 真实UID 的双向映射。
//
// ★ 短 ID 必须**全局唯一且稳定**：
//
//	同一个真实 UID 无论问多少次都要拿到同一个短 ID，
//	否则任务存的 handler 关联会对不上（存的是短 ID A，后来变成 B）。
type ShortIDMap struct {
	mu sync.RWMutex
	// toShort 真实 UID -> 短 ID
	toShort map[uint64]int64
	// toReal 短 ID -> 真实 UID
	toReal map[int64]uint64
	// next 下一个可用短 ID
	next int64
}

// 短 ID 起始值。
//
// ★ 从 1 开始而不是 0：0 在 Lua 里常被当成「无效/空」，
//	脚本里有 `assert(uid > 0)` 这类检查。
const shortIDStart int64 = 1

// 短 ID 上限。
//
// ★ 取 2^40 是保守值：远小于 2^53（float64 精确上限），
//	即使脚本里做几次乘加也不会溢出精度。
const shortIDMax int64 = 1 << 40

// NewShortIDMap 创建映射表。
func NewShortIDMap() *ShortIDMap {
	return &ShortIDMap{
		toShort: make(map[uint64]int64),
		toReal:  make(map[int64]uint64),
		next:    shortIDStart,
	}
}

// Short 把真实 UID 转成短 ID。
//
// 同一个 UID 反复调用返回同一个短 ID（幂等）。
// UID 为 0 时返回 0（表示「无」）。
func (m *ShortIDMap) Short(realUID uint64) int64 {
	if realUID == 0 {
		return 0
	}
	m.mu.RLock()
	if short, ok := m.toShort[realUID]; ok {
		m.mu.RUnlock()
		return short
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	// 双重检查（并发下可能已被别的 goroutine 分配）
	if short, ok := m.toShort[realUID]; ok {
		return short
	}
	short := m.next
	m.next++
	if short > shortIDMax {
		// 理论上不可能（40 位能放一万亿个），真要到了就环绕
		short = shortIDStart
	}
	m.toShort[realUID] = short
	m.toReal[short] = realUID
	return short
}

// Real 把短 ID 还原成真实 UID。
//
// 短 ID 为 0 或未知时返回 0。
func (m *ShortIDMap) Real(short int64) uint64 {
	if short <= 0 {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.toReal[short]
}

// Forget 注销一个真实 UID 的映射（目标销毁时调用）。
//
// ★ 不注销的后果：短 ID 表无限增长。
//
//	任务/地图/UUID 都是启动期分配的，量不大；
//	但 NPC 是按图懒加载的，玩家不断进新图会让表持续膨胀。
func (m *ShortIDMap) Forget(realUID uint64) {
	if realUID == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if short, ok := m.toShort[realUID]; ok {
		delete(m.toShort, realUID)
		delete(m.toReal, short)
	}
}

// Count 返回已分配的映射数（诊断用）。
func (m *ShortIDMap) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.toShort)
}
