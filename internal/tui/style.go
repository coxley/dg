package tui

import (
	"errors"

	"github.com/coxley/dg/layout"
)

func (m *Model) cycleBorder() {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	targets, ok := m.styleTargets(layout.HitNode)
	if !ok {
		m.status = "select a node to change its border"
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
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	if err := m.render(); err != nil {
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.status = err.Error()
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
		m.status = finishOperation
		return
	}
	targets, ok := m.styleTargets(layout.HitNode)
	if !ok {
		m.status = "select a node to align its label"
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
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.status = err.Error()
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

func (m *Model) cycleEdgeArrow(portA bool) {
	if m.mode != modeNavigate {
		m.status = finishOperation
		return
	}
	targets, ok := m.styleTargets(layout.HitEdge)
	if !ok {
		m.status = "select an edge to change its arrows"
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
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	if err := m.rebuild(); err != nil {
		m.status = errors.Join(err, m.cancelTransaction()).Error()
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.status = err.Error()
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
