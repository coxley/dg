package tui

import (
	"errors"

	"github.com/coxley/dg/layout"
)

func (m *Model) cycleBorder() {
	if m.mode != modeNavigate {
		m.setError(finishOperation)
		return
	}
	targets, ok := m.styleTargets(layout.HitNode)
	if !ok {
		m.setError("select a node to change its border")
		return
	}
	inherited, _ := m.geo.NodeStyle(targets.primary.ID)
	inherited.Border = inherited.Border.Next()

	m.beginTransaction()
	apply := func(nodeID uint32) error {
		style, _ := m.geo.NodeStyle(nodeID)
		style.Border = style.Border.Next()
		return m.geo.SetNodeStyle(nodeID, style)
	}
	var err error
	if targets.selection {
		for nodeID := range m.geo.Selection().Nodes() {
			if err = apply(nodeID); err != nil {
				break
			}
		}
	} else {
		err = apply(targets.primary.ID)
	}
	if err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.render(); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	m.nodeStyle = inherited
	if !targets.selection {
		m.target = targets.primary
		m.selectOnly(targets.primary)
	}
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) cycleTextAlignment(vertical bool) {
	if m.mode != modeNavigate {
		m.setError(finishOperation)
		return
	}
	targets, ok := m.styleTargets(layout.HitNode)
	if !ok {
		m.setError("select a node to align its label")
		return
	}
	inherited, _ := m.geo.NodeStyle(targets.primary.ID)
	if vertical {
		inherited.Vertical = inherited.Vertical.Next()
	} else {
		inherited.Horizontal = inherited.Horizontal.Next()
	}

	m.beginTransaction()
	apply := func(nodeID uint32) error {
		style, _ := m.geo.NodeStyle(nodeID)
		if vertical {
			style.Vertical = style.Vertical.Next()
		} else {
			style.Horizontal = style.Horizontal.Next()
		}
		return m.geo.SetNodeStyle(nodeID, style)
	}
	var err error
	if targets.selection {
		for nodeID := range m.geo.Selection().Nodes() {
			if err = apply(nodeID); err != nil {
				break
			}
		}
	} else {
		err = apply(targets.primary.ID)
	}
	if err == nil {
		err = m.render()
	}
	if err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	m.nodeStyle = inherited
	if !targets.selection {
		m.target = targets.primary
		m.selectOnly(targets.primary)
	}
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) toggleStroke() {
	if m.mode != modeNavigate {
		m.setError(finishOperation)
		return
	}
	selection := m.geo.Selection()
	hit, hasHit := m.activeHit()
	if selection.Empty() && (!hasHit ||
		hit.Kind != layout.HitNode && hit.Kind != layout.HitEdge) {
		m.setError("select a node or edge to change its stroke")
		return
	}

	m.beginTransaction()
	var err error
	if selection.Empty() {
		err = m.toggleHitStroke(hit)
	} else {
		err = m.toggleSelectionStroke()
	}
	if err == nil {
		err = m.render()
	}
	if err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	if selection.Empty() {
		m.target = hit
		m.selectOnly(hit)
	}
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) toggleHitStroke(hit layout.Hit) error {
	switch hit.Kind {
	case layout.HitNode:
		style, _ := m.geo.NodeStyle(hit.ID)
		style.Stroke = style.Stroke.Toggle()
		m.nodeStyle = style
		return m.geo.SetNodeStyle(hit.ID, style)
	case layout.HitEdge:
		style, _ := m.geo.EdgeStyle(hit.ID)
		style.Stroke = style.Stroke.Toggle()
		m.edgeStyle = style
		return m.geo.SetEdgeStyle(hit.ID, style)
	case layout.HitPort:
		return nil
	}
	return nil
}

func (m *Model) toggleSelectionStroke() error {
	for nodeID := range m.geo.Selection().Nodes() {
		if err := m.toggleHitStroke(layout.Hit{
			ID:   nodeID,
			Kind: layout.HitNode,
		}); err != nil {
			return err
		}
	}
	for edgeID := range m.geo.Selection().Edges() {
		if err := m.toggleHitStroke(layout.Hit{
			ID:   edgeID,
			Kind: layout.HitEdge,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Model) cycleEdgeArrow(portA bool) {
	if m.mode != modeNavigate {
		m.setError(finishOperation)
		return
	}
	targets, ok := m.styleTargets(layout.HitEdge)
	if !ok {
		m.setError("select an edge to change its arrows")
		return
	}
	inherited, _ := m.geo.EdgeStyle(targets.primary.ID)
	if portA {
		inherited.PortAArrow = inherited.PortAArrow.Next()
	} else {
		inherited.PortBArrow = inherited.PortBArrow.Next()
	}

	m.beginTransaction()
	apply := func(edgeID uint32) error {
		style, _ := m.geo.EdgeStyle(edgeID)
		if portA {
			style.PortAArrow = style.PortAArrow.Next()
		} else {
			style.PortBArrow = style.PortBArrow.Next()
		}
		return m.geo.SetEdgeStyle(edgeID, style)
	}
	var err error
	if targets.selection {
		for edgeID := range m.geo.Selection().Edges() {
			if err = apply(edgeID); err != nil {
				break
			}
		}
	} else {
		err = apply(targets.primary.ID)
	}
	if err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.rebuild(); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	m.edgeStyle = inherited
	if !targets.selection {
		m.target = targets.primary
		m.selectOnly(targets.primary)
	}
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

type styleTargetSet struct {
	primary   layout.Hit
	selection bool
}

func (m *Model) styleTargets(kind layout.HitKind) (styleTargetSet, bool) {
	var targets styleTargetSet
	for hit := range m.geo.DrawOrder() {
		if hit.Kind == kind && m.geo.Selection().Contains(hit) {
			targets.primary = hit
			targets.selection = true
		}
	}
	if targets.selection {
		return targets, true
	}
	hit, ok := m.activeHit()
	return styleTargetSet{primary: hit}, ok && hit.Kind == kind
}
