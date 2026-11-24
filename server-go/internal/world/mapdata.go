package world

import "fmt"

// Land mirrors Mir2xMapData::LAND.
type Land struct {
	Type    uint8
	CanFly  bool
	CanWalk bool
}

func (l Land) CanThrough() bool { return l.CanWalk || l.CanFly }

// MapData is the static walkability layer of one map.
//
// 除可行走信息外，还保留 LIGHT / OBJECT / TILE，用于：
//   - 服务端安全区与视野计算（walkable）
//   - 客户端进场时的地图下发（仅需宽高与物件统计，贴图在客户端本地）
type MapData struct {
	Width  int
	Height int
	Cells  []Land

	// Lights 每格光照（mir2xmapdata.hpp LIGHT）
	Lights []MapLight
	// Objs 每格 2 个物件，扁平存：cellIndex*2+slot
	Objs []MapObject
	// Tiles 每 2×2 格共用一个块贴图，索引 = (x/2)+(y/2)*(Width/2)
	Tiles []MapTile
}

func NewMapData(width, height int) (*MapData, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid map size %dx%d", width, height)
	}
	cells := width * height
	return &MapData{
		Width:  width,
		Height: height,
		Cells:  make([]Land, cells),
		Lights: make([]MapLight, cells),
		Objs:   make([]MapObject, cells*2),
		Tiles:  make([]MapTile, (width/2)*(height/2)),
	}, nil
}

func (m *MapData) InBounds(p Point) bool {
	return m != nil && p.X >= 0 && p.Y >= 0 && p.X < m.Width && p.Y < m.Height
}

func (m *MapData) index(x, y int) int { return x + y*m.Width }

func (m *MapData) LandAt(x, y int) (Land, bool) {
	p := Point{X: x, Y: y}
	if !m.InBounds(p) {
		return Land{}, false
	}
	return m.Cells[m.index(x, y)], true
}

func (m *MapData) SetLand(x, y int, land Land) error {
	p := Point{X: x, Y: y}
	if !m.InBounds(p) {
		return fmt.Errorf("map coordinate out of bounds %d,%d", x, y)
	}
	m.Cells[m.index(x, y)] = land
	return nil
}

func (m *MapData) CanWalk(x, y int) bool {
	land, ok := m.LandAt(x, y)
	return ok && land.CanWalk
}

func (m *MapData) CanFly(x, y int) bool {
	land, ok := m.LandAt(x, y)
	return ok && land.CanFly
}

func (m *MapData) Clone() *MapData {
	if m == nil {
		return nil
	}
	return &MapData{
		Width:  m.Width,
		Height: m.Height,
		Cells:  append([]Land(nil), m.Cells...),
		Lights: append([]MapLight(nil), m.Lights...),
		Objs:   append([]MapObject(nil), m.Objs...),
		Tiles:  append([]MapTile(nil), m.Tiles...),
	}
}

// LightAt 取某格光照。
func (m *MapData) LightAt(x, y int) (MapLight, bool) {
	if !m.InBounds(Point{X: x, Y: y}) || len(m.Lights) == 0 {
		return MapLight{}, false
	}
	return m.Lights[m.index(x, y)], true
}

// ObjectAt 取某格的第 slot 个物件（slot ∈ {0,1}）。
func (m *MapData) ObjectAt(x, y, slot int) (MapObject, bool) {
	if !m.InBounds(Point{X: x, Y: y}) || slot < 0 || slot > 1 || len(m.Objs) == 0 {
		return MapObject{}, false
	}
	return m.Objs[m.index(x, y)*2+slot], true
}

// TileAt 取某格所属块贴图。
func (m *MapData) TileAt(x, y int) (MapTile, bool) {
	if !m.InBounds(Point{X: x, Y: y}) || len(m.Tiles) == 0 {
		return MapTile{}, false
	}
	blockIndex := x/2 + (y/2)*(m.Width/2)
	if blockIndex < 0 || blockIndex >= len(m.Tiles) {
		return MapTile{}, false
	}
	return m.Tiles[blockIndex], true
}

// ObjectCount 统计某深度的有效物件数（自检/诊断用）。
func (m *MapData) ObjectCount(depth uint8) int {
	n := 0
	for _, o := range m.Objs {
		if o.Valid && o.Depth == depth {
			n++
		}
	}
	return n
}

// LightCount 统计有光照的格子数。
func (m *MapData) LightCount() int {
	n := 0
	for _, l := range m.Lights {
		if l.Valid {
			n++
		}
	}
	return n
}

// WalkableCount 统计可行走格数。
func (m *MapData) WalkableCount() int {
	n := 0
	for _, c := range m.Cells {
		if c.CanThrough() {
			n++
		}
	}
	return n
}
