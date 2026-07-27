package layout

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// Connections records which neighboring cells connect to the center of a cell.
type Connections uint8

const (
	North Connections = 1 << iota
	East
	South
	West
)

// ContainsAll reports whether every connection in mask is present.
func (c Connections) ContainsAll(mask Connections) bool {
	return c&mask == mask
}

// ContainsAny reports whether at least one connection in mask is present.
func (c Connections) ContainsAny(mask Connections) bool {
	return c&mask != 0
}

// Grid stores cell connectivity in row-major order.
type Grid struct {
	Bounds Rect
	Cells  []Connections
}

func NewGrid(bounds Rect) (Grid, error) {
	return newGrid(nil, bounds)
}

func newGrid(cells []Connections, bounds Rect) (Grid, error) {
	if bounds.Empty() {
		return Grid{}, fmt.Errorf("invalid grid size %+v", bounds.Size)
	}
	area := uint64(bounds.Size.Width) * uint64(bounds.Size.Height)
	if area > uint64(math.MaxInt) {
		return Grid{}, fmt.Errorf("grid area %d exceeds supported size", area)
	}
	cells = slices.Grow(cells[:0], int(area))[:int(area)]
	clear(cells)
	return Grid{
		Bounds: bounds,
		Cells:  cells,
	}, nil
}

func (g *Grid) At(p Point) (Connections, bool) {
	index, ok := g.Index(p)
	if !ok {
		return 0, false
	}
	return g.Cells[index], true
}

// AddRect adds the perimeter connections of rect.
func (g *Grid) AddRect(rect Rect) error {
	if rect.Size.Width < 2 || rect.Size.Height < 2 {
		return fmt.Errorf("rectangle too small: %+v", rect.Size)
	}
	max := rect.Max()
	points := []Point{
		rect.Min,
		{X: max.X - 1, Y: rect.Min.Y},
		{X: max.X - 1, Y: max.Y - 1},
		{X: rect.Min.X, Y: max.Y - 1},
		rect.Min,
	}
	return g.AddPath(points)
}

// AddPath adds every cell crossed by an orthogonal polyline.
func (g *Grid) AddPath(points []Point) error {
	if len(points) < 2 {
		return errors.New("path needs at least two points")
	}
	for i := 1; i < len(points); i++ {
		if err := g.addSegment(points[i-1], points[i]); err != nil {
			return fmt.Errorf("segment %d: %w", i-1, err)
		}
	}
	return nil
}

// Rasterize converts layout geometry into directional cell occupancy.
func Rasterize(l *Layout) (Grid, error) {
	return RasterizeInto(nil, l)
}

// RasterizeInto converts layout geometry into directional cell occupancy,
// reusing cells when it has sufficient capacity.
func RasterizeInto(cells []Connections, l *Layout) (Grid, error) {
	if l == nil {
		return Grid{}, errors.New("nil layout")
	}
	bounds, ok, err := geometryBounds(l)
	if err != nil {
		return Grid{}, fmt.Errorf("geometry bounds: %w", err)
	}
	if !ok {
		return Grid{}, errors.New("layout has no geometry")
	}
	grid, err := newGrid(cells, bounds)
	if err != nil {
		return Grid{}, err
	}
	for i := range l.Nodes {
		if l.Nodes[i].Empty() {
			continue
		}
		if err := grid.AddRect(l.Nodes[i].Rect); err != nil {
			return Grid{}, fmt.Errorf("rasterize node %d: %w", i, err)
		}
	}
	for i := range l.Edges {
		if l.Edges[i].Empty() {
			continue
		}
		if err := grid.AddPath(l.Edges[i].Points); err != nil {
			return Grid{}, fmt.Errorf("rasterize edge %d: %w", i, err)
		}
	}
	return grid, nil
}

func (g *Grid) addSegment(a, b Point) error {
	switch {
	case a.X == b.X:
		for a != b {
			next := a
			if b.Y < a.Y {
				next.Y--
			} else {
				next.Y++
			}
			if err := g.connect(a, next); err != nil {
				return err
			}
			a = next
		}
	case a.Y == b.Y:
		for a != b {
			next := a
			if b.X < a.X {
				next.X--
			} else {
				next.X++
			}
			if err := g.connect(a, next); err != nil {
				return err
			}
			a = next
		}
	default:
		return fmt.Errorf("non-orthogonal points %+v and %+v", a, b)
	}
	return nil
}

func (g *Grid) connect(a, b Point) error {
	aIndex, ok := g.Index(a)
	if !ok {
		return fmt.Errorf("point %+v outside grid", a)
	}
	bIndex, ok := g.Index(b)
	if !ok {
		return fmt.Errorf("point %+v outside grid", b)
	}

	dir, ok := directionBetween(a, b)
	if !ok {
		return fmt.Errorf("points %+v and %+v are not adjacent", a, b)
	}
	switch dir {
	case north:
		g.Cells[aIndex] |= North
		g.Cells[bIndex] |= South
	case east:
		g.Cells[aIndex] |= East
		g.Cells[bIndex] |= West
	case south:
		g.Cells[aIndex] |= South
		g.Cells[bIndex] |= North
	case west:
		g.Cells[aIndex] |= West
		g.Cells[bIndex] |= East
	}
	return nil
}

// Index returns the row-major cell index for p.
func (g *Grid) Index(p Point) (int, bool) {
	if !g.Bounds.Contains(p) {
		return 0, false
	}
	x := p.X - g.Bounds.Min.X
	y := p.Y - g.Bounds.Min.Y
	return int(y)*int(g.Bounds.Size.Width) + int(x), true
}

func geometryBounds(l *Layout) (Rect, bool, error) {
	if len(l.Nodes) == 0 && len(l.Edges) == 0 {
		return Rect{}, false, nil
	}

	first := true
	var minX, minY, maxX, maxY uint32
	addPoint := func(p Point) {
		if first {
			minX, maxX = p.X, p.X
			minY, maxY = p.Y, p.Y
			first = false
			return
		}
		minX, minY = min(minX, p.X), min(minY, p.Y)
		maxX, maxY = max(maxX, p.X), max(maxY, p.Y)
	}
	for i := range l.Nodes {
		if l.Nodes[i].Empty() {
			continue
		}
		addPoint(l.Nodes[i].Rect.Min)
		max := l.Nodes[i].Rect.Max()
		addPoint(Point{X: max.X - 1, Y: max.Y - 1})
	}
	for i := range l.Edges {
		for _, point := range l.Edges[i].Points {
			addPoint(point)
		}
	}
	if first {
		return Rect{}, false, nil
	}
	if maxX == math.MaxUint32 || maxY == math.MaxUint32 {
		return Rect{}, false, errors.New("geometry bounds exceed coordinate space")
	}
	rect, err := NewRect(
		Point{X: minX, Y: minY},
		Point{X: maxX, Y: maxY}.Add(1, 1),
	)
	if err != nil {
		return Rect{}, false, err
	}
	return rect, true, nil
}
