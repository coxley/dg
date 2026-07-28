package layout

import (
	"errors"
	"fmt"

	"github.com/coxley/dg/ir"
)

// BorderStyle controls a node's visible boundary.
type BorderStyle uint8

const (
	BorderSolid BorderStyle = iota
	BorderRounded
	BorderNone
	borderStyleCount
)

// Valid reports whether style is supported.
func (s BorderStyle) Valid() bool {
	return s < borderStyleCount
}

// Next returns the next border style, wrapping to BorderSolid.
func (s BorderStyle) Next() BorderStyle {
	return (s + 1) % borderStyleCount
}

// HorizontalAlign controls label placement across a node's inner bounds.
type HorizontalAlign uint8

const (
	AlignLeft HorizontalAlign = iota
	AlignCenter
	AlignRight
	horizontalAlignCount
)

// Valid reports whether alignment is supported.
func (a HorizontalAlign) Valid() bool {
	return a < horizontalAlignCount
}

// Next returns the next horizontal alignment.
func (a HorizontalAlign) Next() HorizontalAlign {
	return (a + 1) % horizontalAlignCount
}

// VerticalAlign controls label placement down a node's inner bounds.
type VerticalAlign uint8

const (
	AlignTop VerticalAlign = iota
	AlignMiddle
	AlignBottom
	verticalAlignCount
)

// Valid reports whether alignment is supported.
func (a VerticalAlign) Valid() bool {
	return a < verticalAlignCount
}

// Next returns the next vertical alignment.
func (a VerticalAlign) Next() VerticalAlign {
	return (a + 1) % verticalAlignCount
}

// ArrowStyle controls an edge endpoint marker.
type ArrowStyle uint8

const (
	ArrowNone ArrowStyle = iota
	ArrowOpen
	ArrowFilled
	arrowStyleCount
)

// Valid reports whether style is supported.
func (s ArrowStyle) Valid() bool {
	return s < arrowStyleCount
}

// Next cycles through none, filled, and outline arrows.
func (s ArrowStyle) Next() ArrowStyle {
	switch s {
	case ArrowNone:
		return ArrowFilled
	case ArrowFilled:
		return ArrowOpen
	default:
		return ArrowNone
	}
}

// NodeStyle controls node rendering.
type NodeStyle struct {
	Border     BorderStyle
	Horizontal HorizontalAlign
	Vertical   VerticalAlign
}

// Valid reports whether every node style dimension is supported.
func (s NodeStyle) Valid() bool {
	return s.Border.Valid() && s.Horizontal.Valid() && s.Vertical.Valid()
}

// EdgeStyle controls endpoint rendering by graph port order.
type EdgeStyle struct {
	PortAArrow ArrowStyle
	PortBArrow ArrowStyle
}

// Valid reports whether every edge style dimension is supported.
func (s EdgeStyle) Valid() bool {
	return s.PortAArrow.Valid() && s.PortBArrow.Valid()
}

// NodeStyle returns nodeID's style.
func (l *Layout) NodeStyle(nodeID uint32) (NodeStyle, bool) {
	if !l.graph.NodeExists(nodeID) ||
		uint64(nodeID) >= uint64(len(l.nodeStyles)) {
		return NodeStyle{}, false
	}
	return l.nodeStyles[nodeID], true
}

// SetNodeStyle changes nodeID's style.
func (l *Layout) SetNodeStyle(nodeID uint32, style NodeStyle) error {
	if !l.graph.NodeExists(nodeID) {
		return fmt.Errorf("%w: %d", ir.ErrNodeNotFound, nodeID)
	}
	if !style.Valid() {
		return errors.New("invalid node style")
	}
	previous := l.nodeStyles[nodeID]
	if previous == style {
		return nil
	}
	l.nodeStyles[nodeID] = style
	if l.history != nil {
		l.history.record(historyChange{
			kind:            historySetNodeStyle,
			id:              nodeID,
			beforeNodeStyle: previous,
			afterNodeStyle:  style,
		})
	}
	return nil
}

// EdgeStyle returns edgeID's style.
func (l *Layout) EdgeStyle(edgeID uint32) (EdgeStyle, bool) {
	if !l.graph.EdgeExists(edgeID) ||
		uint64(edgeID) >= uint64(len(l.edgeStyles)) {
		return EdgeStyle{}, false
	}
	return l.edgeStyles[edgeID], true
}

// SetEdgeStyle changes edgeID's style.
func (l *Layout) SetEdgeStyle(edgeID uint32, style EdgeStyle) error {
	if !l.graph.EdgeExists(edgeID) {
		return fmt.Errorf("%w: %d", ir.ErrEdgeNotFound, edgeID)
	}
	if !style.Valid() {
		return errors.New("invalid edge arrow style")
	}
	previous := l.edgeStyles[edgeID]
	if previous == style {
		return nil
	}
	l.edgeStyles[edgeID] = style
	if l.history != nil {
		l.history.record(historyChange{
			kind:            historySetEdgeStyle,
			id:              edgeID,
			beforeEdgeStyle: previous,
			afterEdgeStyle:  style,
		})
	}
	return nil
}
