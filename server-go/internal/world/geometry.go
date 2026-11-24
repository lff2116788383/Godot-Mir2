package world

// Direction matches the legacy DIR_* wire values.
type Direction int

const (
	DirNone Direction = iota
	DirUp
	DirUpRight
	DirRight
	DirDownRight
	DirDown
	DirDownLeft
	DirLeft
	DirUpLeft
	DirEnd
)

// Point is a map grid coordinate.
type Point struct {
	X int
	Y int
}

func (p Point) Equal(other Point) bool { return p.X == other.X && p.Y == other.Y }

func (d Direction) Valid() bool { return d >= DirUp && d < DirEnd }

var directionOffsets = [...]Point{
	{0, 0},
	{0, -1},
	{1, -1},
	{1, 0},
	{1, 1},
	{0, 1},
	{-1, 1},
	{-1, 0},
	{-1, -1},
}

func (d Direction) Offset(length int) Point {
	if !d.Valid() {
		return Point{}
	}
	offset := directionOffsets[d]
	return Point{X: offset.X * length, Y: offset.Y * length}
}

func (d Direction) Front(p Point, length int) Point {
	offset := d.Offset(length)
	return Point{X: p.X + offset.X, Y: p.Y + offset.Y}
}

func (d Direction) Back(p Point, length int) Point {
	return d.Front(p, -length)
}

func (d Direction) Next(diff int) Direction {
	if !d.Valid() {
		return DirNone
	}
	count := int(DirEnd - DirUp)
	index := ((int(d-DirUp)+diff)%count + count) % count
	return Direction(DirUp) + Direction(index)
}

func (d Direction) Backward() Direction {
	switch d {
	case DirUp:
		return DirDown
	case DirUpRight:
		return DirDownLeft
	case DirRight:
		return DirLeft
	case DirDownRight:
		return DirUpLeft
	case DirDown:
		return DirUp
	case DirDownLeft:
		return DirUpRight
	case DirLeft:
		return DirRight
	case DirUpLeft:
		return DirDownRight
	default:
		return DirNone
	}
}

func DirectionFromDelta(dx, dy int) Direction {
	switch {
	case dx == 0 && dy < 0:
		return DirUp
	case dx > 0 && dy < 0:
		return DirUpRight
	case dx > 0 && dy == 0:
		return DirRight
	case dx > 0 && dy > 0:
		return DirDownRight
	case dx == 0 && dy > 0:
		return DirDown
	case dx < 0 && dy > 0:
		return DirDownLeft
	case dx < 0 && dy == 0:
		return DirLeft
	case dx < 0 && dy < 0:
		return DirUpLeft
	default:
		return DirNone
	}
}

func DirectionBetween(from, to Point) Direction {
	return DirectionFromDelta(sign(to.X-from.X), sign(to.Y-from.Y))
}

func sign(value int) int {
	if value < 0 {
		return -1
	}
	if value > 0 {
		return 1
	}
	return 0
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ChebyshevDistance is the number of 8-direction steps between points.
func ChebyshevDistance(a, b Point) int {
	return max(abs(a.X-b.X), abs(a.Y-b.Y))
}

// HopValid mirrors pathf::hopValid's 1/2/diagonal squared-distance rule.
func HopValid(maxStep int, from, to Point) bool {
	if maxStep < 1 || maxStep > 3 {
		return false
	}
	dx := to.X - from.X
	dy := to.Y - from.Y
	distance2 := dx*dx + dy*dy
	return distance2 == 1 || distance2 == 2 ||
		distance2 == maxStep*maxStep || distance2 == 2*maxStep*maxStep
}
