package world

// ==================== AOI 观察者圈（servermap.cpp）====================
//
// 原版 AOI 的运作方式：
//
//	每个格维护一个 observerList：当前视野覆盖该格的实体 uid。
//	实体移动时只更新「离开的格」和「进入的格」（updateGridObserver）。
//	当某格新进入某实体的视野时，把该格上的所有 CO 通知给这个新观察者
//	（notifyGridObserved → AM_OBSERVE），让它知道「你现在能看到这些人」。
//	当实体离开某格时，把 CO 消失通知给该格的观察者（notifyCODisappear）。
//	动作广播时，只发给「当前能看到该格」的观察者集合，无需再扫圆。
//
// ★ 半径语义（极易写错）：
//
//	doCircle(cx, cy, r) 用 LDistance2 <= (r-1)² 筛选，
//	即**实际半径是 r-1**，而遍历窗口是 (2r-1)²。
//	所以 viewRange=10 时覆盖的是 19×19 窗口里欧氏距离 ≤9 的格。
//	LDistance2 是**欧氏距离平方**，不是切比雪夫距离 —— 圆形不是方形。

// ViewRange 是实体视野半径（格）。0 表示不观察任何格。
const MaxViewRange = 24

// DoCircle 遍历以 (cx,cy) 为中心、半径 r 的圆形区域内的合法格。
//
// 完全照抄 servermap.hpp:doCircle 的三重条件：
//  1. 窗口 2r-1 从 (cx-r+1, cy-r+1) 开始
//  2. 坐标在 map 边界内
//  3. LDistance2(x,y,cx,cy) <= (r-1)²
//
// fn 返回 true 时提前终止并返回 true。
func DoCircle(md *MapData, cx, cy, r int, fn func(x, y int) bool) bool {
	w := 2*r - 1
	h := 2*r - 1
	x0 := cx - r + 1
	y0 := cy - r + 1
	if w <= 0 || h <= 0 {
		return false
	}
	limit := (r - 1) * (r - 1)
	for x := x0; x < x0+w; x++ {
		for y := y0; y < y0+h; y++ {
			if md != nil && !md.InBounds(Point{X: x, Y: y}) {
				continue
			}
			if LDistance2(x, y, cx, cy) <= limit {
				if fn(x, y) {
					return true
				}
			}
		}
	}
	return false
}

// LDistance2 是欧氏距离平方（mathf.hpp:LDistance2）。
func LDistance2(x, y, x1, y1 int) int {
	dx := x - x1
	dy := y - y1
	return dx*dx + dy*dy
}

// InCircle 判断 (x,y) 是否在以 (cx,cy) 为心、半径 r 的观察圆内。
//
// 注意用 (r-1)² 作为上界，与 DoCircle 一致。
func InCircle(x, y, cx, cy, r int) bool {
	if r <= 1 {
		// 半径为 0/1 时圆只覆盖中心格：r=1 时 (1-1)²=0，只有距离 0 的格
		return x == cx && y == cy && r > 0
	}
	return LDistance2(x, y, cx, cy) <= (r-1)*(r-1)
}

// observerEventKind 是 AOI 变更事件类型。
type observerEventKind uint8

const (
	// ObserverAdd：此格新进入观察者视野，观察者应看到该格上的实体
	ObserverAdd observerEventKind = iota + 1
	// ObserverRemove：此格离开观察者视野，观察者应移除该格上的实体
	ObserverRemove
	// COAppear：有实体进入某格，该格的观察者应新增这个实体
	COAppear
	// CODisappear：有实体离开某格，该格的观察者应移除这个实体
	CODisappear
)

// ObserverEvent 是一条 AOI 变更，交给上层转成网络消息。
//
// 与原版 AM_OBSERVE / AM_CODISAPPEAR 的对应关系：
//   - ObserverAdd     → 观察者收到目标格上实体的 AM_OBSERVE
//   - ObserverRemove  → 观察者不再感知该格（原版不发消息，靠 CODisappear 收敛）
//   - COAppear        → 该格观察者收到该实体的 AM_OBSERVE
//   - CODisappear     → 该格观察者收到该实体的 AM_CODISAPPEAR
type ObserverEvent struct {
	Kind observerEventKind
	// Observer 事件的主体观察者（ObserverAdd/Remove 时有效）
	Observer uint64
	// Target 事件涉及的实体（COAppear/CODisappear 时有效）
	Target uint64
	X      int
	Y      int
}

// observersOf 返回某格的观察者列表（内部用，不做拷贝）。
func (m *MapState) observersOf(x, y int) []uint64 {
	cell, err := m.cell(x, y)
	if err != nil {
		return nil
	}
	return cell.Observers
}

func (m *MapState) observerIndex(x, y int, uid uint64) int {
	cell, err := m.cell(x, y)
	if err != nil {
		return -1
	}
	for i, v := range cell.Observers {
		if v == uid {
			return i
		}
	}
	return -1
}

// UpdateGridObserver 让 uid 开始/停止观察格 (x,y)。
//
// 对照 servermap.cpp:updateGridObserver：
//   - add 且尚未观察：加入列表，并触发「该格上的实体对我可见」
//   - remove 且正在观察：移出列表（原版用 swap+pop 做 O(1) 删除，
//     这里顺序无关，同样用 swap+pop 保持一致的迭代代价）
func (m *MapState) UpdateGridObserver(uid uint64, x, y int, add bool) []ObserverEvent {
	cell, err := m.cell(x, y)
	if err != nil {
		return nil
	}
	idx := -1
	for i, v := range cell.Observers {
		if v == uid {
			idx = i
			break
		}
	}

	if add {
		if idx >= 0 {
			return nil // 已在观察，不重复通知
		}
		cell.Observers = append(cell.Observers, uid)
		// notifyGridObserved：该格上的实体向新观察者报到
		var events []ObserverEvent
		for occupant := range cell.Occupants {
			if occupant == uid {
				continue
			}
			events = append(events, ObserverEvent{
				Kind: COAppear, Observer: uid, Target: occupant, X: x, Y: y,
			})
		}
		return events
	}

	if idx < 0 {
		return nil // 本来就没观察，无需处理
	}
	// swap + pop，与原版一致
	last := len(cell.Observers) - 1
	cell.Observers[idx] = cell.Observers[last]
	cell.Observers = cell.Observers[:last]
	return []ObserverEvent{{Kind: ObserverRemove, Observer: uid, X: x, Y: y}}
}

// UpdateGridObserverCircle 让 uid 开始/停止观察以 (x,y) 为心的整个圆。
func (m *MapState) UpdateGridObserverCircle(uid uint64, rangeVal, x, y int, add bool) []ObserverEvent {
	if rangeVal <= 0 {
		return nil
	}
	var events []ObserverEvent
	DoCircle(m.Data, x, y, rangeVal, func(gx, gy int) bool {
		events = append(events, m.UpdateGridObserver(uid, gx, gy, add)...)
		return false
	})
	return events
}

// MoveGridObserverCircle 在实体从 (srcX,srcY) 移到 (dstX,dstY) 时增量更新观察圈。
//
// 只处理「离开旧圆」和「进入新圆」的格，交集不动 —— 原版注释明确写了这点：
//
//	only update grids that leave or enter the observer circle
//
// 原版另有 m_addDeleteGridList 预计算缓存（按 dir/step/range 查表），
// 这里是直接比较两个圆。结果完全等价，只是多一次扫描；
// 视野 ≤24、每次移动扫 (2r-1)² ≤ 2209 格，可接受。
// 若后续压测成为瓶颈，再按原版加缓存（键为 dir,step,range）。
func (m *MapState) MoveGridObserverCircle(uid uint64, rangeVal, srcX, srcY, dstX, dstY int) []ObserverEvent {
	if rangeVal <= 0 || (srcX == dstX && srcY == dstY) {
		return nil
	}
	var events []ObserverEvent

	// 旧圆里、已不在新圆内的格：取消观察
	DoCircle(m.Data, srcX, srcY, rangeVal, func(gx, gy int) bool {
		if !InCircle(gx, gy, dstX, dstY, rangeVal) {
			events = append(events, m.UpdateGridObserver(uid, gx, gy, false)...)
		}
		return false
	})

	// 新圆里、不在旧圆内的格：开始观察
	DoCircle(m.Data, dstX, dstY, rangeVal, func(gx, gy int) bool {
		if !InCircle(gx, gy, srcX, srcY, rangeVal) {
			events = append(events, m.UpdateGridObserver(uid, gx, gy, true)...)
		}
		return false
	})

	return events
}

// ResizeGridObserverCircle 在视野范围变化时调整观察圈，中心不变。
//
// 原版 servermap.cpp:resizeGridObserverCircle：只处理两个圆之间的环带，
// 已观察的格不重复通知。视野变小时移除环带，变大时加入环带。
func (m *MapState) ResizeGridObserverCircle(uid uint64, x, y, oldRange, newRange int) []ObserverEvent {
	if oldRange < 0 {
		oldRange = 0
	}
	if newRange < 0 {
		newRange = 0
	}
	if oldRange == newRange {
		return nil
	}
	var events []ObserverEvent

	if oldRange > newRange {
		// 缩小：旧圆里、不在新圆的格取消观察
		if newRange <= 0 {
			DoCircle(m.Data, x, y, oldRange, func(gx, gy int) bool {
				events = append(events, m.UpdateGridObserver(uid, gx, gy, false)...)
				return false
			})
			return events
		}
		DoCircle(m.Data, x, y, oldRange, func(gx, gy int) bool {
			if !InCircle(gx, gy, x, y, newRange) {
				events = append(events, m.UpdateGridObserver(uid, gx, gy, false)...)
			}
			return false
		})
		return events
	}

	// 扩大：新圆里、不在旧圆的格开始观察
	DoCircle(m.Data, x, y, newRange, func(gx, gy int) bool {
		if oldRange <= 0 || !InCircle(gx, gy, x, y, oldRange) {
			events = append(events, m.UpdateGridObserver(uid, gx, gy, true)...)
		}
		return false
	})
	return events
}

// VisibleObservers 返回当前能看到格 (x,y) 的观察者 uid 列表。
//
// 对应原版 forwardObserverAction 的核心：
//
//	for observerUID := range getGrid(x,y).observerList { post(observerUID, AM_ACTION) }
//
// 动作广播时用它确定收件人，不必再扫圆。
func (m *MapState) VisibleObservers(x, y int) []uint64 {
	cell, err := m.cell(x, y)
	if err != nil {
		return nil
	}
	return cell.Observers
}

// ForwardAction 把一条动作事件投递给「源格 + 目标格」的观察者并集。
//
// 对照 servermap.cpp:forwardObserverAction(amA, srcX, srcY, dstX, dstY)：
//
//	forward a move/jump/spacemove to the union of observers of the source and destination grids
//	src-only observers lose sight of the actor and remove it, dst observers gain or keep sight of it
//
// 去重规则：同一 uid 只投递一次，且不投递给动作发起者自己。
func (m *MapState) ForwardAction(selfUID uint64, srcX, srcY, dstX, dstY int) []uint64 {
	seen := make(map[uint64]struct{}, 16)
	var out []uint64
	addGrid := func(x, y int) {
		if !m.Data.InBounds(Point{X: x, Y: y}) {
			return
		}
		for _, observer := range m.VisibleObservers(x, y) {
			if observer == selfUID {
				continue
			}
			if _, dup := seen[observer]; dup {
				continue
			}
			seen[observer] = struct{}{}
			out = append(out, observer)
		}
	}
	addGrid(srcX, srcY)
	addGrid(dstX, dstY)
	return out
}

// NotifyCOAppear 在实体进入某格后，通知该格的所有观察者「它出现了」。
//
// 对照 servermap.cpp：实体 AddEntity 后对所在格做 notify。原版是通过
// 观察者加入时反向通知（notifyGridObserved）；这里补充正向通知，
// 因为地图上已有的观察者也需要知道新来的实体。
func (m *MapState) NotifyCOAppear(uid uint64, x, y int) []ObserverEvent {
	cell, err := m.cell(x, y)
	if err != nil {
		return nil
	}
	var events []ObserverEvent
	for _, observer := range cell.Observers {
		if observer == uid {
			continue
		}
		events = append(events, ObserverEvent{
			Kind: COAppear, Observer: observer, Target: uid, X: x, Y: y,
		})
	}
	return events
}

// NotifyCODisappear 在实体离开某格/消失后，通知该格的所有观察者。
//
// 对照 servermap.cpp:notifyCODisappear。
func (m *MapState) NotifyCODisappear(uid uint64, x, y int) []ObserverEvent {
	cell, err := m.cell(x, y)
	if err != nil {
		return nil
	}
	var events []ObserverEvent
	for _, observer := range cell.Observers {
		if observer == uid {
			continue
		}
		events = append(events, ObserverEvent{
			Kind: CODisappear, Observer: observer, Target: uid, X: x, Y: y,
		})
	}
	return events
}

// QueryObserved 判断某格上是否至少有一个玩家在观察。
//
// 对照 servermap.cpp:on_AM_QUERYOBSERVED —— 用于判断「这个格是否有人在看」，
// 决定要不要广播比如怪物动作这类开销大的消息。
func (m *MapState) QueryObserved(x, y int) bool {
	if !m.Data.InBounds(Point{X: x, Y: y}) {
		return false
	}
	for _, observer := range m.VisibleObservers(x, y) {
		if entity, ok := m.Entities[observer]; ok && entity.Type == EntityPlayer {
			return true
		}
	}
	return false
}

// VisibleEntities 返回 uid 视野内所有其他实体的 UID（不含自己）。
//
// 实现方式：遍历自身观察圈覆盖的格，把这些格上的 Occupants 收集起来去重。
// 与 VisibleObservers 正好相反 —— 那个是「谁能看到这格」，这个是「我能看到谁」。
func (m *MapState) VisibleEntities(uid uint64) []uint64 {
	self, ok := m.Entities[uid]
	if !ok {
		return nil
	}
	if self.ViewRange <= 0 {
		// 视野 0 只能看到同格
		cell, err := m.cell(self.X, self.Y)
		if err != nil {
			return nil
		}
		out := make([]uint64, 0, len(cell.Occupants))
		for other := range cell.Occupants {
			if other != uid {
				out = append(out, other)
			}
		}
		return out
	}

	seen := make(map[uint64]struct{}, 32)
	var out []uint64
	DoCircle(m.Data, self.X, self.Y, self.ViewRange, func(x, y int) bool {
		cell, err := m.cell(x, y)
		if err != nil {
			return false
		}
		for other := range cell.Occupants {
			if other == uid {
				continue
			}
			if _, dup := seen[other]; dup {
				continue
			}
			seen[other] = struct{}{}
			out = append(out, other)
		}
		return false
	})
	return out
}
