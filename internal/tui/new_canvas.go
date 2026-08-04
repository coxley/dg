package tui

import (
	"fmt"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/layout"
)

func (m *Model) newCanvas() {
	if m.canvasStore == nil {
		m.setError("no canvas store configured")
		return
	}
	if m.dialogs.ActiveID() != surfaceNone || !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	if err := m.createCanvas(); err != nil {
		m.setError(fmt.Sprintf("new canvas: %v", err))
	}
}

func (m *Model) createCanvas() error {
	if err := m.flushActive(); err != nil {
		return err
	}
	if err := m.history.Reset(func() error {
		return m.geo.Replace(
			func(*layout.Layout) error { return nil },
			layout.WithRouter(m.preferences.defaultRouter),
		)
	}); err != nil {
		return err
	}
	m.document = document.New(m.geo)
	m.entry = nil
	m.syncWindowTitle()
	m.saved = m.dirty
	m.viewport = layout.NewPoint(0, 0)
	m.clearSelection()
	if err := m.rebuild(); err != nil {
		return err
	}
	m.cursor = layout.Point{}
	m.target = layout.Hit{}
	m.active = 0
	m.hits = m.hits[:0]
	m.status = "new draft"
	m.statusError = ""
	return nil
}
