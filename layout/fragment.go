package layout

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

const fragmentVersion = 1

// Fragment owns a portable copy of selected layout objects.
type Fragment struct {
	state fragmentState
}

type fragmentState struct {
	Version uint8              `json:"version"`
	Origin  Point              `json:"origin"`
	Size    Size               `json:"size"`
	Layout  layoutHistoryState `json:"layout"`
}

// Origin returns the source-space origin of the fragment bounds.
func (f Fragment) Origin() Point {
	return f.state.Origin
}

// Bounds returns the source-space bounds containing the fragment geometry.
func (f Fragment) Bounds() Rect {
	return Rect{Min: f.state.Origin, Size: f.state.Size}
}

// CopySelection copies selected nodes and edges wholly contained by those
// nodes into an independent fragment.
func (l *Layout) CopySelection() (Fragment, error) {
	selectedNodes := make([]uint32, 0, l.selection.nodeCount)
	for nodeID := range l.selection.Nodes() {
		selectedNodes = append(selectedNodes, nodeID)
	}
	if len(selectedNodes) == 0 {
		return Fragment{}, errors.New("selection contains no nodes")
	}

	bounds, err := l.selectionBounds(selectedNodes)
	if err != nil {
		return Fragment{}, err
	}
	copy, err := New(WithRouter(l.router))
	if err != nil {
		return Fragment{}, err
	}
	copy.padding = l.padding
	if err := copy.copyNodesFrom(
		l,
		selectedNodes,
		-int64(bounds.Min.X),
		-int64(bounds.Min.Y),
		false,
	); err != nil {
		return Fragment{}, fmt.Errorf("copy selection: %w", err)
	}
	return Fragment{state: fragmentState{
		Version: fragmentVersion,
		Origin:  bounds.Min,
		Size:    bounds.Size,
		Layout:  copy.historyState(),
	}}, nil
}

// Paste copies a fragment into the layout with its bounds rooted at origin.
// Paste replaces the selection with the newly created objects.
func (l *Layout) Paste(fragment Fragment, origin Point) error {
	state := fragment.state
	if err := validateFragmentState(&state); err != nil {
		return err
	}
	source, err := New()
	if err != nil {
		return err
	}
	if err := source.restoreHistoryState(state.Layout); err != nil {
		return fmt.Errorf("restore fragment: %w", err)
	}
	selectedNodes := make([]uint32, 0, len(source.graph.Nodes))
	for nodeID := range source.graph.Nodes {
		if source.graph.NodeExists(uint32(nodeID)) {
			selectedNodes = append(selectedNodes, uint32(nodeID))
		}
	}
	if err := validateCopyOffset(source, selectedNodes, int64(origin.X), int64(origin.Y)); err != nil {
		return err
	}
	return l.copyNodesFrom(
		source,
		selectedNodes,
		int64(origin.X),
		int64(origin.Y),
		false,
	)
}

// MarshalJSON encodes the fragment's portable state.
func (f Fragment) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.state)
}

// UnmarshalJSON decodes and validates portable fragment state.
func (f *Fragment) UnmarshalJSON(data []byte) error {
	var state fragmentState
	if err := decodeHistoryJSON(data, &state); err != nil {
		return fmt.Errorf("decode layout fragment: %w", err)
	}
	if err := validateFragmentState(&state); err != nil {
		return fmt.Errorf("decode layout fragment: %w", err)
	}
	f.state = state
	return nil
}

func validateFragmentState(state *fragmentState) error {
	if state.Version != fragmentVersion {
		return fmt.Errorf("unsupported fragment version: %d", state.Version)
	}
	if state.Size.Empty() {
		return errors.New("empty fragment bounds")
	}
	if err := validateLayoutHistoryState(&state.Layout); err != nil {
		return fmt.Errorf("invalid fragment layout: %w", err)
	}
	for nodeID := range state.Layout.Graph.Nodes {
		if state.Layout.Graph.NodeExists(uint32(nodeID)) {
			return nil
		}
	}
	return errors.New("fragment contains no nodes")
}

func (l *Layout) selectionBounds(selectedNodes []uint32) (Rect, error) {
	selected := make([]bool, len(l.graph.Nodes))
	minX, minY := uint32(math.MaxUint32), uint32(math.MaxUint32)
	var maxX, maxY uint32
	for _, nodeID := range selectedNodes {
		selected[nodeID] = true
		rect := l.Nodes[nodeID].Rect
		minX, minY = min(minX, rect.Min.X), min(minY, rect.Min.Y)
		maxPoint := rect.Max()
		maxX, maxY = max(maxX, maxPoint.X), max(maxY, maxPoint.Y)
	}
	for edgeID, edge := range l.graph.Edges {
		if !l.graph.EdgeExists(uint32(edgeID)) {
			continue
		}
		nodeA := l.graph.Ports[edge.PortA].Node
		nodeB := l.graph.Ports[edge.PortB].Node
		if !selected[nodeA] || !selected[nodeB] {
			continue
		}
		for _, point := range l.Edges[edgeID].Points {
			if err := includeFragmentPoint(point, &minX, &minY, &maxX, &maxY); err != nil {
				return Rect{}, err
			}
		}
		for _, bend := range l.edgeBends[edgeID] {
			if err := includeFragmentPoint(bend.Point, &minX, &minY, &maxX, &maxY); err != nil {
				return Rect{}, err
			}
		}
	}
	return Rect{
		Min: NewPoint(minX, minY),
		Size: Size{
			Width:  maxX - minX,
			Height: maxY - minY,
		},
	}, nil
}

func includeFragmentPoint(point Point, minX, minY, maxX, maxY *uint32) error {
	if point.X == math.MaxUint32 || point.Y == math.MaxUint32 {
		return errors.New("fragment geometry outside representable bounds")
	}
	*minX, *minY = min(*minX, point.X), min(*minY, point.Y)
	*maxX, *maxY = max(*maxX, point.X+1), max(*maxY, point.Y+1)
	return nil
}

func validateCopyOffset(
	source *Layout,
	selectedNodes []uint32,
	dx, dy int64,
) error {
	for _, nodeID := range selectedNodes {
		if _, ok := offsetPoint(source.origins[nodeID], dx, dy); !ok {
			return errors.New("paste placement outside coordinate space")
		}
		limit := source.Nodes[nodeID].Rect.Max()
		x, y := int64(limit.X)+dx, int64(limit.Y)+dy
		if x < 0 || y < 0 || x > math.MaxUint32 || y > math.MaxUint32 {
			return errors.New("paste geometry outside coordinate space")
		}
	}
	for edgeID := range source.graph.Edges {
		if !source.graph.EdgeExists(uint32(edgeID)) {
			continue
		}
		if _, ok := offsetPinnedBends(source.edgeBends[edgeID], dx, dy); !ok {
			return errors.New("paste bend outside coordinate space")
		}
	}
	return nil
}
