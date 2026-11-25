package script

import (
	"fmt"
	"sync"

	lua "github.com/yuin/gopher-lua"

	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
)

// ==================== 格子触发器（server/src/servermap.cpp:924-960）====================
//
// 【依据】servermap.cpp `allocateGridTriggerId` / `removeGridTriggerId`
//         + servermap.lua `addGridTrigger` / `_RSVD_NAME_runGridTrigger`
//
// ★ 语义（原版注释说得很清楚）：
//
//	地图 record 的 switchList 里的格子，正常会自动把玩家传到下一张图。
//	一旦某个格子上装了触发器，**自动传送就停下**，由脚本决定：
//
//	    local id = addGridTrigger(226, 177, function(uid, x, y)
//	        if server.player.hasItem(uid, '牢房钥匙', 1) then
//	            uidGridMapSwitch(uid, x, y)   -- 放行
//	            return
//	        end
//	        server.player.postString(uid, '我没有钥匙，无法进入……')
//	    end)
//
//	handler 返回精确的 true 时，SYS_EPUID 触发器自我退休。
//
// ★ 用到它的任务（实测 2 个）：
//
//	困魔咒任务      踩到某格把玩家挡回去
//	群体治愈术任务  类似的入口限制
//
// ★ 之前是「返回递增句柄、不真触发」的降级：
//
//	任务能加载、能对话，但走到那片区域时**什么都不发生** ——
//	玩家可以直接穿过本该被挡住的门口。
//	这种「静默失效」比直接报错更难发现，所以这里补上真实现。

// gridTriggerStore 一张地图的触发器表。
//
// ★ 按格索引是必要的：原版 dispatchGridSwitch 是 O(1) 查
//	`getGrid(x,y).triggerList`。若每次都遍历全部矩形，
//	玩家每走一步就要扫几十个矩形，卡顿会非常明显。
type gridTriggerStore struct {
	mu sync.Mutex
	// nextID 单调递增 ID（原版 m_gridTriggerID，从 1 开始）
	nextID int64
	// rects id -> 矩形列表（用于删除时反查）
	rects map[int64][][4]int
	// byGrid "x,y" -> id 列表（原版 getGrid().triggerList）
	byGrid map[[2]int][]int64
}

func newGridTriggerStore() *gridTriggerStore {
	return &gridTriggerStore{
		rects:  make(map[int64][][4]int),
		byGrid: make(map[[2]int][]int64),
	}
}

// allocate 分配一个触发器 ID 并把它挂到全部矩形覆盖的格子上。
func (g *gridTriggerStore) allocate(rectList [][4]int) int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.nextID++
	id := g.nextID
	g.rects[id] = rectList
	for _, r := range rectList {
		x, y, w, h := r[0], r[1], r[2], r[3]
		for nw := 0; nw < w; nw++ {
			for nh := 0; nh < h; nh++ {
				key := [2]int{x + nw, y + nh}
				g.byGrid[key] = append(g.byGrid[key], id)
			}
		}
	}
	return id
}

// remove 删除一个触发器（原版 removeGridTriggerId）。
func (g *gridTriggerStore) remove(id int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	rectList, ok := g.rects[id]
	if !ok {
		return
	}
	for _, r := range rectList {
		x, y, w, h := r[0], r[1], r[2], r[3]
		for nw := 0; nw < w; nw++ {
			for nh := 0; nh < h; nh++ {
				key := [2]int{x + nw, y + nh}
				list := g.byGrid[key]
				for i, v := range list {
					if v == id {
						g.byGrid[key] = append(list[:i], list[i+1:]...)
						break
					}
				}
				if len(g.byGrid[key]) == 0 {
					delete(g.byGrid, key)
				}
			}
		}
	}
	delete(g.rects, id)
}

// at 返回某格上的触发器 ID 列表（原版 getGridTriggerIDList）。
func (g *gridTriggerStore) at(x, y int) []int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	list := g.byGrid[[2]int{x, y}]
	out := make([]int64, len(list))
	copy(out, list)
	return out
}

// empty 是否一个触发器都没有（用来跳过整个 runGridTrigger）。
func (g *gridTriggerStore) empty() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.rects) == 0
}

// count 触发器总数（诊断用）。
func (g *gridTriggerStore) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.rects)
}

// ==================== 绑定 ====================

// installGridTriggers 把格子触发器相关绑定装进地图 Lua 状态。
//
// ★ 装上 servevermap.lua 之后这些才是「真」的：
//	之前 _RSVD_NAME_allocateGridTriggerId 返回恒 0，
//	于是所有触发器都写进 _RSVD_NAME_gridTriggers[0] 互相覆盖。
func (s *MapScript) installGridTriggers() {
	if s.gridTriggers == nil {
		s.gridTriggers = newGridTriggerStore()
	}
	g := s.gridTriggers
	L := s.Runtime.L

	// _RSVD_NAME_allocateGridTriggerId(rectList) -> id
	L.SetGlobal("_RSVD_NAME_allocateGridTriggerId", L.NewFunction(func(l *lua.LState) int {
		rectTbl, _ := l.Get(1).(*lua.LTable)
		l.Push(lua.LNumber(g.allocate(s.flattenRectsLua(rectTbl))))
		return 1
	}))

	// _RSVD_NAME_removeGridTriggerId(id)
	L.SetGlobal("_RSVD_NAME_removeGridTriggerId", L.NewFunction(func(l *lua.LState) int {
		g.remove(int64(l.CheckInt64(1)))
		return 0
	}))

	// _RSVD_NAME_getGridTriggerIDList(x, y) -> id 列表
	L.SetGlobal("_RSVD_NAME_getGridTriggerIDList", L.NewFunction(func(l *lua.LState) int {
		x := l.CheckInt(1)
		y := l.CheckInt(2)
		tbl := l.NewTable()
		for _, id := range g.at(x, y) {
			tbl.Append(lua.LNumber(id))
		}
		l.Push(tbl)
		return 1
	}))
}

// FireGridTrigger 在某格上触发格子触发器（对应原版 dispatchGridSwitch）。
//
// 返回 true 表示「该格有触发器，已运行，地图不应自动传送玩家」。
//
// ★ 这是服务端玩家移动逻辑的接入点：
//
//	玩家踩到有触发器的格 → 调这里 → servermap.lua 的
//	_RSVD_NAME_runGridTrigger 决定放行/挡住。
//
//	不接这一步的话，2 个任务的入口限制完全失效。
func (s *MapScript) FireGridTrigger(playerShortUID int64, x, y int) bool {
	if s == nil || s.Runtime == nil || s.Runtime.L == nil {
		return false
	}
	if s.gridTriggers == nil || s.gridTriggers.empty() {
		return false
	}
	if len(s.gridTriggers.at(x, y)) == 0 {
		return false
	}

	fn, ok := s.Runtime.L.GetGlobal("_RSVD_NAME_runGridTrigger").(*lua.LFunction)
	if !ok {
		return false
	}

	// ★ 必须在协程里跑：
	//	runGridTrigger 内部会 runMapThread / uidRemoteCall，
	//	都可能 yield。
	co, _ := s.Runtime.L.NewThread()
	if _, err, _ := s.Runtime.L.Resume(co, fn,
		lua.LNumber(playerShortUID), lua.LNumber(x), lua.LNumber(y)); err != nil {
		s.Host.Log(1, fmt.Sprintf("run grid trigger at (%d,%d): %v", x, y, err))
	}
	return true
}

// ==================== 安全区（servermap.cpp:1331-1360）====================
//
// ★ 为什么必须实现（而不是留着空桩）：
//
//	servermap.lua（第 411 行起）**重新定义了** addSafeZone：
//
//	    function addSafeZone(name, ...)
//	        ...
//	        _RSVD_NAME_safeZones[name] = _RSVD_NAME_allocateSafeZoneId(...)
//	    end
//
//	也就是说加载 servermap.lua 之后，addSafeZone 会去调
//	_RSVD_NAME_allocateSafeZoneId —— 那个函数如果没绑定，
//	**每一个地图脚本的第 1 行就崩**（比奇县_0.lua:1 → servermap.lua:415）。
//
//	所以「装 servermap.lua 以支持格子触发器」这件事，
//	和「补 3 个安全区 ID 绑定」是必须一起做的。

// safeZone 是一个安全区（矩形集合 + 圆集合）。
type safeZone struct {
	rectList   [][4]int // x, y, w, h
	circleList [][3]int // x, y, r
}

// safeZoneStore 一张地图的安全区表。
type safeZoneStore struct {
	mu      sync.Mutex
	nextID  int64
	zones   map[int64]safeZone
	// byGrid 格 -> 该格是否在任何安全区里（isSafeGrid 用）
	//
	// ★ 用 map 而不是每次遍历全部区域：
	//	玩家每走一步都要查 isSafeGrid，遍历是 O(区域数)。
	grids map[[2]int]int
}

func newSafeZoneStore() *safeZoneStore {
	return &safeZoneStore{
		zones: make(map[int64]safeZone),
		grids: make(map[[2]int]int),
	}
}

func (s *safeZoneStore) allocate(z safeZone) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := s.nextID
	s.zones[id] = z
	s.reindexLocked()
	return id
}

func (s *safeZoneStore) update(id int64, z safeZone) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.zones[id]; !ok {
		return false
	}
	s.zones[id] = z
	s.reindexLocked()
	return true
}

func (s *safeZoneStore) remove(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.zones, id)
	s.reindexLocked()
}

// isSafe 判断某格是否在任一安全区里（isSafeGrid）。
func (s *safeZoneStore) isSafe(x, y int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grids[[2]int{x, y}] > 0
}

func (s *safeZoneStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.zones)
}

// reindexLocked 重建格索引（区域数量很少，全量重建足够）。
func (s *safeZoneStore) reindexLocked() {
	s.grids = make(map[[2]int]int, len(s.grids))
	for _, z := range s.zones {
		for _, r := range z.rectList {
			x, y, w, h := r[0], r[1], r[2], r[3]
			for nw := 0; nw < w; nw++ {
				for nh := 0; nh < h; nh++ {
					s.grids[[2]int{x + nw, y + nh}]++
				}
			}
		}
		for _, c := range z.circleList {
			cx, cy, r := c[0], c[1], c[2]
			// 圆拆成格：外接正方形内筛距离
			for dx := -r; dx <= r; dx++ {
				for dy := -r; dy <= r; dy++ {
					if dx*dx+dy*dy <= r*r {
						s.grids[[2]int{cx + dx, cy + dy}]++
					}
				}
			}
		}
	}
}

// flattenRectsLua 把任意形式的 rectList 转成扁平整数切片 [x,y,w,h, x,y,w,h, ...]。
//
// ★★ 为什么走 Lua 而不是在 Go 里解析表结构 ★★
//
//	servermap.lua 的 parseRectList 产出的 rect 是**具名键**形式
//	{x=, y=, w=, h=}，而 quest.lua 拼参数时可能是数组形式 {x,y,w,h}，
//	也可能直接给 (x, y) 两个数。三种形式混在一起，
//	在 Go 里逐字段试探既啰嗦又容易漏（实测就漏了：
//	RawGetString 读出来全是 0，触发器分配成功但任何格子都查不到）。
//
//	交给 Lua 做转换：它天然知道 `r.x or r[1]` 的优先级，
//	而且这段逻辑和 servermap.lua 自己的 parseRectList 完全一致。
//	Go 只负责按 4 个一组切分。
func (s *MapScript) flattenRectsLua(tbl *lua.LTable) [][4]int {
	if tbl == nil || s.Runtime == nil || s.Runtime.L == nil {
		return nil
	}
	L := s.Runtime.L

	fn, ok := L.GetGlobal("__mir2_flatten_rects").(*lua.LFunction)
	if !ok {
		// 没装辅助函数时退回朴素解析（至少不会崩）
		return parseRectListLua(tbl)
	}

	base := L.GetTop()
	L.Push(fn)
	L.Push(tbl)
	if err := L.PCall(1, 1, nil); err != nil {
		L.SetTop(base)
		return parseRectListLua(tbl)
	}
	flat, ok := L.Get(-1).(*lua.LTable)
	L.SetTop(base)
	if !ok {
		return parseRectListLua(tbl)
	}

	n := flat.Len() / 4
	out := make([][4]int, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, [4]int{
			int(lua.LVAsNumber(flat.RawGetInt(i*4 + 1))),
			int(lua.LVAsNumber(flat.RawGetInt(i*4 + 2))),
			int(lua.LVAsNumber(flat.RawGetInt(i*4 + 3))),
			int(lua.LVAsNumber(flat.RawGetInt(i*4 + 4))),
		})
	}
	return out
}

// flattenCirclesLua 把任意形式的 circleList 转成扁平整数切片 [x,y,r, ...]。
func (s *MapScript) flattenCirclesLua(tbl *lua.LTable) [][3]int {
	if tbl == nil || s.Runtime == nil || s.Runtime.L == nil {
		return nil
	}
	L := s.Runtime.L

	fn, ok := L.GetGlobal("__mir2_flatten_circles").(*lua.LFunction)
	if !ok {
		return parseCircleListLua(tbl)
	}
	base := L.GetTop()
	L.Push(fn)
	L.Push(tbl)
	if err := L.PCall(1, 1, nil); err != nil {
		L.SetTop(base)
		return parseCircleListLua(tbl)
	}
	flat, ok := L.Get(-1).(*lua.LTable)
	L.SetTop(base)
	if !ok {
		return parseCircleListLua(tbl)
	}

	n := flat.Len() / 3
	out := make([][3]int, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, [3]int{
			int(lua.LVAsNumber(flat.RawGetInt(i*3 + 1))),
			int(lua.LVAsNumber(flat.RawGetInt(i*3 + 2))),
			int(lua.LVAsNumber(flat.RawGetInt(i*3 + 3))),
		})
	}
	return out
}

// installFlattenHelpers 装两个「把矩形/圆形表摊平成整数数组」的 Lua 辅助函数。
//
// ★ 必须在地图脚本加载**之前**装：addSafeZone / addGridTrigger 会在
//	加载期就调用对应的 allocate 绑定。
func (s *MapScript) installFlattenHelpers() {
	L := s.Runtime.L
	src := `
-- 把 rectList 摊平成 {x,y,w,h, x,y,w,h, ...}
-- 接受三种形式：{x=,y=,w=,h=} / {x,y,w,h} / 单个数对
function __mir2_flatten_rects(rectList)
    local out = {}
    if type(rectList) ~= 'table' then return out end
    for _, r in ipairs(rectList) do
        if type(r) == 'table' then
            local w = r.w or r[3] or 1
            local h = r.h or r[4] or 1
            out[#out + 1] = r.x or r[1] or 0
            out[#out + 1] = r.y or r[2] or 0
            out[#out + 1] = w
            out[#out + 1] = h
        end
    end
    return out
end

-- 把 circleList 摊平成 {x,y,r, x,y,r, ...}
function __mir2_flatten_circles(circleList)
    local out = {}
    if type(circleList) ~= 'table' then return out end
    for _, c in ipairs(circleList) do
        if type(c) == 'table' then
            out[#out + 1] = c.x or c[1] or 0
            out[#out + 1] = c.y or c[2] or 0
            out[#out + 1] = c.r or c[3] or 0
        end
    end
    return out
end
`
	if err := L.DoString(src); err != nil {
		s.Host.Log(1, fmt.Sprintf("install flatten helpers: %v", err))
	}
}

// parseRectListLua 把 Lua 的 rectList 表转成 [][4]int。
//
// ★★ 必须同时支持具名键与数组下标 ★★
//
//	servermap.lua 的 parseRectList（第 87-99 行）产出的是
//	**具名键**形式：{x = .., y = .., w = .., h = ..}。
//	只按 RawGetInt(1..4) 读会全部拿到 0 ——
//	表现是「触发器 ID 分配成功，但任何格子都查不到」。
//
//	同时也接受 {x,y,w,h} 的数组形式，因为 quest.lua 拼参数时
//	两种都可能出现。
func parseRectListLua(tbl *lua.LTable) [][4]int {
	if tbl == nil {
		return nil
	}
	out := make([][4]int, 0, tbl.Len())
	tbl.ForEach(func(_, v lua.LValue) {
		row, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		out = append(out, [4]int{
			rectField(row, "x", 1),
			rectField(row, "y", 2),
			rectFieldDefault(row, "w", 3, 1),
			rectFieldDefault(row, "h", 4, 1),
		})
	})
	return out
}

// rectField 取一个必填字段（先具名键，再数组下标）。
func rectField(tbl *lua.LTable, name string, idx int) int {
	if v := tbl.RawGetString(name); v != lua.LNil {
		return int(lua.LVAsNumber(v))
	}
	return int(lua.LVAsNumber(tbl.RawGetInt(idx)))
}

// rectFieldDefault 取一个可省字段（缺省用数组下标，再缺省用 def）。
func rectFieldDefault(tbl *lua.LTable, name string, idx int, def int) int {
	if v := tbl.RawGetString(name); v != lua.LNil {
		return int(lua.LVAsNumber(v))
	}
	if v := tbl.RawGetInt(idx); v != lua.LNil {
		return int(lua.LVAsNumber(v))
	}
	return def
}

// parseCircleListLua 把 Lua 的 circleList 表转成 [][3]int。
//
// 与 rectList 一样：servermap.lua 产出具名键 {x=, y=, r=}。
func parseCircleListLua(tbl *lua.LTable) [][3]int {
	if tbl == nil {
		return nil
	}
	out := make([][3]int, 0, tbl.Len())
	tbl.ForEach(func(_, v lua.LValue) {
		row, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		out = append(out, [3]int{
			rectField(row, "x", 1),
			rectField(row, "y", 2),
			rectField(row, "r", 3),
		})
	})
	return out
}

// installSafeZones 把安全区相关绑定装进地图 Lua 状态。
func (s *MapScript) installSafeZones() {
	if s.safeZones == nil {
		s.safeZones = newSafeZoneStore()
	}
	z := s.safeZones
	L := s.Runtime.L

	// _RSVD_NAME_allocateSafeZoneId(rectList, circleList) -> id
	L.SetGlobal("_RSVD_NAME_allocateSafeZoneId", L.NewFunction(func(l *lua.LState) int {
		rectTbl, _ := l.Get(1).(*lua.LTable)
		circleTbl, _ := l.Get(2).(*lua.LTable)
		id := z.allocate(safeZone{
			rectList:   s.flattenRectsLua(rectTbl),
			circleList: s.flattenCirclesLua(circleTbl),
		})
		// 同时登记到宿主（世界层要用它判断「这格是不是安全区」）
		l.Push(lua.LNumber(id))
		return 1
	}))

	// _RSVD_NAME_updateSafeZoneId(id, rectList, circleList) -> bool
	L.SetGlobal("_RSVD_NAME_updateSafeZoneId", L.NewFunction(func(l *lua.LState) int {
		id := int64(l.CheckInt64(1))
		rectTbl, _ := l.Get(2).(*lua.LTable)
		circleTbl, _ := l.Get(3).(*lua.LTable)
		ok := z.update(id, safeZone{
			rectList:   s.flattenRectsLua(rectTbl),
			circleList: s.flattenCirclesLua(circleTbl),
		})
		l.Push(lua.LBool(ok))
		return 1
	}))

	// _RSVD_NAME_removeSafeZoneId(id)
	L.SetGlobal("_RSVD_NAME_removeSafeZoneId", L.NewFunction(func(l *lua.LState) int {
		z.remove(int64(l.CheckInt64(1)))
		return 0
	}))

	// isSafeGrid(x, y) -> bool
	L.SetGlobal("isSafeGrid", L.NewFunction(func(l *lua.LState) int {
		l.Push(lua.LBool(z.isSafe(l.CheckInt(1), l.CheckInt(2))))
		return 1
	}))
}

// IsSafeGrid 判断某格是否在地图安全区内（供世界层查询）。
func (s *MapScript) IsSafeGrid(x, y int) bool {
	if s == nil || s.safeZones == nil {
		return false
	}
	return s.safeZones.isSafe(x, y)
}

// SafeZoneCount 返回本图的安全区数量（诊断用）。
func (s *MapScript) SafeZoneCount() int {
	if s == nil || s.safeZones == nil {
		return 0
	}
	return s.safeZones.count()
}

// GridTriggerCount 返回本图装了几个触发器（诊断用）。
func (s *MapScript) GridTriggerCount() int {
	if s == nil || s.gridTriggers == nil {
		return 0
	}
	return s.gridTriggers.count()
}

// ExecRemote 在本图的地图 Lua 状态里执行一段远程代码串。
//
// ★ 用途：任务的 uidRemoteCall 打过来，要在**地图 actor 的上下文里**
//
//	运行 addQuestGridTrigger 之类的函数 —— 那些函数定义在
//	servermap.lua，只有地图状态里才有。
//
// ★ 必须起协程：代码串里可能含 pause / runMapThread，
//	在主状态直接跑会报 "can not yield from outside of a coroutine"。
func (s *MapScript) ExecRemote(code string, args []protocol.LuaVar) RPCResult {
	if s == nil || s.Runtime == nil || s.Runtime.L == nil {
		return RPCResult{Err: &RPCError{Message: "map runtime closed"}}
	}
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	patched, _ := patchIntDiv(code)
	patched, _ = patchToBeClosed(patched)

	L := s.Runtime.L
	fn, err := L.LoadString(patched)
	if err != nil {
		return RPCResult{Err: &RPCError{Message: err.Error()}}
	}

	luaArgs := make([]lua.LValue, 0, len(args))
	for _, arg := range args {
		luaArgs = append(luaArgs, luaVarToLua(L, arg))
	}

	co, _ := L.NewThread()
	st, rerr, rets := L.Resume(co, fn, luaArgs...)
	if rerr != nil {
		return RPCResult{Err: &RPCError{Message: rerr.Error()}}
	}
	if st == lua.ResumeYield {
		// 让出了（代码里 pause 之类）：返回值要等它跑完才有
		//
		// ★ 对 addQuestGridTrigger 这种「同步返回一个 ID」的调用不会走到这里。
		//	真走到了说明脚本在做异步操作，返回 nil 让它继续跑。
		return RPCResult{Values: []protocol.LuaVar{protocol.NewLuaNilVar()}}
	}

	// ★★ 返回值必须从 Resume 的第三个返回值取，不能读协程栈 ★★
	//
	//	gopher-lua 的 Resume 在协程结束后会把返回值搬到**主状态**的栈上，
	//	同时把协程自己的栈清空。读 co.Get(i) 拿到的是协程内部的临时值
	//	（比如 load(code) 产生的中间表），完全不是函数返回值。
	values := make([]protocol.LuaVar, 0, len(rets))
	for _, v := range rets {
		values = append(values, luaToLuaVar(v))
	}
	return RPCResult{Values: values}
}