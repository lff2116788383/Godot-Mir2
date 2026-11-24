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
type MapData struct {
	Width  int
	Height int
	Cells  []Land
}

func NewMapData(width, height int) (*MapData, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid map size %dx%d", width, height)
	}
	return &MapData{Width: width, Height: height, Cells: make([]Land, width*height)}, nil
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
	clone := &MapData{Width: m.Width, Height: m.Height, Cells: append([]Land(nil), m.Cells...)}
	return clone
}
