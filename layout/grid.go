package layout

import (
	"cmp"
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

// NoPortID marks an endpoint that is not attached to a graph port.
const NoPortID uint32 = math.MaxUint32

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
	Owners []Hit
}

// RasterEdge describes transient edge geometry without adding it to a graph.
// PortA and PortB control common-endpoint composition; NoPortID leaves an end
// unattached.
type RasterEdge struct {
	Points       []Point
	PortA, PortB uint32
}

// RasterCell contains the connections drawn at one point.
type RasterCell struct {
	Point       Point
	Connections Connections
}

func NewGrid(bounds Rect) (Grid, error) {
	return newGrid(nil, nil, bounds)
}

func newGrid(cells []Connections, owners []Hit, bounds Rect) (Grid, error) {
	if bounds.Empty() {
		return Grid{}, fmt.Errorf("invalid grid size %+v", bounds.Size)
	}
	area := uint64(bounds.Size.Width) * uint64(bounds.Size.Height)
	if area > uint64(math.MaxInt) {
		return Grid{}, fmt.Errorf("grid area %d exceeds supported size", area)
	}
	cells = slices.Grow(cells[:0], int(area))[:int(area)]
	owners = slices.Grow(owners[:0], int(area))[:int(area)]
	clear(cells)
	clear(owners)
	return Grid{
		Bounds: bounds,
		Cells:  cells,
		Owners: owners,
	}, nil
}

func (g *Grid) At(p Point) (Connections, bool) {
	index, ok := g.Index(p)
	if !ok {
		return 0, false
	}
	return g.Cells[index], true
}

// OwnerAt returns the topmost object occupying p.
func (g *Grid) OwnerAt(p Point) (Hit, bool) {
	index, ok := g.Index(p)
	if !ok || g.Owners[index] == (Hit{}) {
		return Hit{}, false
	}
	return g.Owners[index], true
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
	return rasterizePath(points, func(point Point, connections Connections) error {
		index, ok := g.Index(point)
		if !ok {
			return fmt.Errorf("point %+v outside grid", point)
		}
		g.Cells[index] |= connections
		return nil
	})
}

// RasterizeEdgeInto writes row-major cells for edge over base into dst. It
// preserves committed connectivity at shared endpoints and replaces it at
// crossings.
func RasterizeEdgeInto(
	dst []RasterCell,
	base *Grid,
	l *Layout,
	edge RasterEdge,
) ([]RasterCell, error) {
	if base == nil {
		return nil, errors.New("nil base grid")
	}
	if l == nil {
		return nil, errors.New("nil layout")
	}
	dst = dst[:0]
	err := rasterizePath(edge.Points, func(point Point, connections Connections) error {
		if len(dst) != 0 && dst[len(dst)-1].Point == point {
			dst[len(dst)-1].Connections |= connections
			return nil
		}
		dst = append(dst, RasterCell{
			Point:       point,
			Connections: connections,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(dst, func(a, b RasterCell) int {
		if order := cmp.Compare(a.Point.Y, b.Point.Y); order != 0 {
			return order
		}
		return cmp.Compare(a.Point.X, b.Point.X)
	})
	compacted := dst[:0]
	for _, cell := range dst {
		if len(compacted) != 0 &&
			compacted[len(compacted)-1].Point == cell.Point {
			compacted[len(compacted)-1].Connections |= cell.Connections
			continue
		}
		compacted = append(compacted, cell)
	}
	dst = compacted
	for i := range dst {
		baseConnections, ok := base.At(dst[i].Point)
		if !ok {
			continue
		}
		previous, _ := base.OwnerAt(dst[i].Point)
		if l.rasterEdgePreservesCell(edge, dst[i].Point, previous) {
			dst[i].Connections |= baseConnections
		}
	}
	return dst, nil
}

// Rasterize converts layout geometry into directional cell occupancy.
func Rasterize(l *Layout) (Grid, error) {
	return RasterizeInto(nil, l)
}

// RasterizeInto converts layout geometry into directional cell occupancy,
// reusing cells when it has sufficient capacity.
func RasterizeInto(cells []Connections, l *Layout) (Grid, error) {
	return rasterizeInto(cells, nil, l, math.MaxUint32)
}

// RasterizeOwnedInto converts layout geometry into directional cell occupancy
// and ownership while reusing both aligned slices.
func RasterizeOwnedInto(
	cells []Connections,
	owners []Hit,
	l *Layout,
) (Grid, error) {
	return rasterizeInto(cells, owners, l, math.MaxUint32)
}

// RasterizeWithoutEdgeInto converts layout geometry into directional cell
// occupancy while omitting edgeID.
func RasterizeWithoutEdgeInto(
	cells []Connections,
	l *Layout,
	edgeID uint32,
) (Grid, error) {
	return rasterizeInto(cells, nil, l, edgeID)
}

// RasterizeWithoutEdgeOwnedInto rasterizes ownership while omitting edgeID.
func RasterizeWithoutEdgeOwnedInto(
	cells []Connections,
	owners []Hit,
	l *Layout,
	edgeID uint32,
) (Grid, error) {
	return rasterizeInto(cells, owners, l, edgeID)
}

func rasterizeInto(
	cells []Connections,
	owners []Hit,
	l *Layout,
	excludedEdge uint32,
) (Grid, error) {
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
	grid, err := newGrid(cells, owners, bounds)
	if err != nil {
		return Grid{}, err
	}
	for hit := range l.DrawOrder() {
		switch hit.Kind {
		case HitNode:
			if l.Nodes[hit.ID].Empty() {
				continue
			}
			grid.claimRect(l.Nodes[hit.ID].Rect, hit)
			if err := grid.AddRect(l.Nodes[hit.ID].Rect); err != nil {
				return Grid{}, fmt.Errorf("rasterize node %d: %w", hit.ID, err)
			}
		case HitEdge:
			if hit.ID == excludedEdge || l.Edges[hit.ID].Empty() {
				continue
			}
			if err := grid.claimEdge(l, hit.ID); err != nil {
				return Grid{}, fmt.Errorf("claim edge %d: %w", hit.ID, err)
			}
			if err := grid.AddPath(l.Edges[hit.ID].Points); err != nil {
				return Grid{}, fmt.Errorf("rasterize edge %d: %w", hit.ID, err)
			}
		case HitPort:
			continue
		}
	}
	return grid, nil
}

func (g *Grid) claimRect(rect Rect, owner Hit) {
	limit := rect.Max()
	for y := rect.Min.Y; y < limit.Y; y++ {
		for x := rect.Min.X; x < limit.X; x++ {
			index, _ := g.Index(Point{X: x, Y: y})
			g.Cells[index] = 0
			g.Owners[index] = owner
		}
	}
}

func (g *Grid) claimEdge(l *Layout, edgeID uint32) error {
	points := l.Edges[edgeID].Points
	owner := Hit{ID: edgeID, Kind: HitEdge}
	edge := RasterEdge{
		Points: points,
		PortA:  NoPortID,
		PortB:  NoPortID,
	}
	if l.graph.EdgeExists(edgeID) {
		graphEdge := l.graph.Edges[edgeID]
		edge.PortA = graphEdge.PortA
		edge.PortB = graphEdge.PortB
	}
	claim := func(point Point) {
		if l.edgeEndpointAt(edgeID, point) {
			return
		}
		index, _ := g.Index(point)
		previous := g.Owners[index]
		if !l.rasterEdgePreservesCell(edge, point, previous) {
			g.Cells[index] = 0
		}
		g.Owners[index] = owner
	}
	claim(points[0])
	for i := 1; i < len(points); i++ {
		if err := walkSegment(points[i-1], points[i], claim); err != nil {
			return fmt.Errorf("segment %d: %w", i-1, err)
		}
	}
	return nil
}

func (l *Layout) rasterEdgePreservesCell(
	edge RasterEdge,
	point Point,
	previous Hit,
) bool {
	if len(edge.Points) != 0 {
		if point == edge.Points[0] && edge.PortA != NoPortID {
			return true
		}
		if point == edge.Points[len(edge.Points)-1] &&
			edge.PortB != NoPortID {
			return true
		}
	}
	if previous.Kind != HitEdge || !l.graph.EdgeExists(previous.ID) {
		return false
	}
	previousEdge := l.graph.Edges[previous.ID]
	return previousEdge.HasPort(edge.PortA) || previousEdge.HasPort(edge.PortB)
}

func walkSegment(a, b Point, visit func(Point)) error {
	switch {
	case a.X == b.X:
		for a != b {
			if b.Y < a.Y {
				a.Y--
			} else {
				a.Y++
			}
			visit(a)
		}
	case a.Y == b.Y:
		for a != b {
			if b.X < a.X {
				a.X--
			} else {
				a.X++
			}
			visit(a)
		}
	default:
		return fmt.Errorf("non-orthogonal points %+v and %+v", a, b)
	}
	return nil
}

func rasterizePath(
	points []Point,
	visit func(Point, Connections) error,
) error {
	if len(points) < 2 {
		return errors.New("path needs at least two points")
	}
	for i := 1; i < len(points); i++ {
		current := points[i-1]
		finish := points[i]
		if current.X != finish.X && current.Y != finish.Y {
			return fmt.Errorf(
				"segment %d: non-orthogonal points %+v and %+v",
				i-1,
				current,
				finish,
			)
		}
		for current != finish {
			next := current
			switch {
			case finish.X < current.X:
				next.X--
			case finish.X > current.X:
				next.X++
			case finish.Y < current.Y:
				next.Y--
			default:
				next.Y++
			}
			dir, _ := directionBetween(current, next)
			from, to := directionConnections(dir)
			if err := visit(current, from); err != nil {
				return fmt.Errorf("segment %d: %w", i-1, err)
			}
			if err := visit(next, to); err != nil {
				return fmt.Errorf("segment %d: %w", i-1, err)
			}
			current = next
		}
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
