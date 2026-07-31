package layout

import (
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/coxley/dg/ir"
)

var ErrLayerObject = errors.New("object cannot participate in draw order")

// WithDrawOrder sets the initial back-to-front object order.
func WithDrawOrder(order []Hit) Option {
	return func(l *Layout) {
		l.drawOrder = slices.Clone(order)
	}
}

// DrawOrder yields live nodes and edges from back to front.
func (l *Layout) DrawOrder() iter.Seq[Hit] {
	return func(yield func(Hit) bool) {
		if len(l.drawOrder) == 0 {
			for nodeID, node := range l.Nodes {
				if !node.Empty() &&
					!yield(Hit{ID: uint32(nodeID), Kind: HitNode}) {
					return
				}
			}
			for edgeID, edge := range l.Edges {
				if !edge.Empty() &&
					!yield(Hit{ID: uint32(edgeID), Kind: HitEdge}) {
					return
				}
			}
			return
		}
		for _, hit := range l.drawOrder {
			if !yield(hit) {
				return
			}
		}
	}
}

func (l *Layout) hasLayer(hit Hit) bool {
	for _, candidate := range l.drawOrder {
		if candidate == hit {
			return true
		}
	}
	return false
}

// BringForward moves hit one position toward the front.
func (l *Layout) BringForward(hit Hit) error {
	index, err := l.layerIndex(hit)
	if err != nil {
		return err
	}
	if index+1 == len(l.drawOrder) {
		return nil
	}
	return l.setLayerIndex(hit, index+1)
}

// SendBackward moves hit one position toward the back.
func (l *Layout) SendBackward(hit Hit) error {
	index, err := l.layerIndex(hit)
	if err != nil {
		return err
	}
	if index == 0 {
		return nil
	}
	return l.setLayerIndex(hit, index-1)
}

// BringToFront moves hit above every other object.
func (l *Layout) BringToFront(hit Hit) error {
	if _, err := l.layerIndex(hit); err != nil {
		return err
	}
	return l.setLayerIndex(hit, len(l.drawOrder)-1)
}

// SendToBack moves hit below every other object.
func (l *Layout) SendToBack(hit Hit) error {
	if _, err := l.layerIndex(hit); err != nil {
		return err
	}
	return l.setLayerIndex(hit, 0)
}

func (l *Layout) setLayerIndex(hit Hit, target int) error {
	index, err := l.layerIndex(hit)
	if err != nil {
		return err
	}
	if index == target {
		return nil
	}
	if index < target {
		copy(l.drawOrder[index:target], l.drawOrder[index+1:target+1])
	} else {
		copy(l.drawOrder[target+1:index+1], l.drawOrder[target:index])
	}
	l.drawOrder[target] = hit
	if l.recordingChanges() {
		l.recordChange(historyChange{
			Kind:     historySetLayer,
			ID:       hit.ID,
			LayerHit: hit,
			Before:   historyChangeState{Layer: uint32(index)},
			After:    historyChangeState{Layer: uint32(target)},
		})
	}
	return nil
}

func (l *Layout) layerIndex(hit Hit) (int, error) {
	if !l.layerObjectLive(hit) {
		return 0, fmt.Errorf("%w: %+v", ErrLayerObject, hit)
	}
	for i, candidate := range l.drawOrder {
		if candidate == hit {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%w: missing %+v", ErrLayerObject, hit)
}

func (l *Layout) appendLayer(hit Hit) {
	l.drawOrder = append(l.drawOrder, hit)
}

func (l *Layout) removeLayer(hit Hit) (int, bool) {
	for i, candidate := range l.drawOrder {
		if candidate != hit {
			continue
		}
		copy(l.drawOrder[i:], l.drawOrder[i+1:])
		l.drawOrder[len(l.drawOrder)-1] = Hit{}
		l.drawOrder = l.drawOrder[:len(l.drawOrder)-1]
		return i, true
	}
	return 0, false
}

func (l *Layout) insertLayer(hit Hit, index int) {
	index = min(max(index, 0), len(l.drawOrder))
	l.drawOrder = append(l.drawOrder, Hit{})
	copy(l.drawOrder[index+1:], l.drawOrder[index:])
	l.drawOrder[index] = hit
}

func (l *Layout) initializeDrawOrder() error {
	if len(l.drawOrder) == 0 {
		for nodeID := range l.graph.Nodes {
			if l.graph.NodeExists(uint32(nodeID)) {
				l.appendLayer(Hit{ID: uint32(nodeID), Kind: HitNode})
			}
		}
		for edgeID := range l.graph.Edges {
			if l.graph.EdgeExists(uint32(edgeID)) {
				l.appendLayer(Hit{ID: uint32(edgeID), Kind: HitEdge})
			}
		}
		return nil
	}
	return validateDrawOrder(&l.graph, l.drawOrder)
}

func validateDrawOrder(graph *ir.Graph, order []Hit) error {
	want := liveLayerCount(graph)
	if len(order) != want {
		return fmt.Errorf(
			"draw order contains %d objects, want %d",
			len(order),
			want,
		)
	}
	seenNodes := make([]bool, len(graph.Nodes))
	seenEdges := make([]bool, len(graph.Edges))
	for _, hit := range order {
		if !graphLayerObjectLive(graph, hit) {
			return fmt.Errorf("%w: %+v", ErrLayerObject, hit)
		}
		var seen []bool
		switch hit.Kind {
		case HitNode:
			seen = seenNodes
		case HitEdge:
			seen = seenEdges
		default:
			return fmt.Errorf("%w: %+v", ErrLayerObject, hit)
		}
		if seen[hit.ID] {
			return fmt.Errorf("draw order contains duplicate %+v", hit)
		}
		seen[hit.ID] = true
	}
	return nil
}

func (l *Layout) layerObjectLive(hit Hit) bool {
	return graphLayerObjectLive(&l.graph, hit)
}

func graphLayerObjectLive(graph *ir.Graph, hit Hit) bool {
	switch hit.Kind {
	case HitNode:
		return graph.NodeExists(hit.ID)
	case HitEdge:
		return graph.EdgeExists(hit.ID)
	default:
		return false
	}
}

func liveLayerCount(graph *ir.Graph) int {
	count := 0
	for nodeID := range graph.Nodes {
		if graph.NodeExists(uint32(nodeID)) {
			count++
		}
	}
	for edgeID := range graph.Edges {
		if graph.EdgeExists(uint32(edgeID)) {
			count++
		}
	}
	return count
}
