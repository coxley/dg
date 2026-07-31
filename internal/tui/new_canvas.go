package tui

import (
	"errors"
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
	var options []layout.Option
	if m.preferences.applyToFuture {
		options = append(options, layout.WithRouter(m.preferences.baseline.Router))
	}
	blank, err := layout.New(options...)
	if err != nil {
		return fmt.Errorf("create layout: %w", err)
	}
	entry, err := m.canvasStore.CreateDraft(document.New(blank))
	if err != nil {
		return fmt.Errorf("create draft: %w", err)
	}
	if err := m.switchCanvas(entry); err != nil {
		deleteErr := m.canvasStore.Delete(entry)
		return errors.Join(err, deleteErr)
	}
	m.cursor = layout.Point{}
	m.target = layout.Hit{}
	m.active = 0
	m.hits = m.hits[:0]
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
	m.status = "new draft"
	m.statusError = ""
	return nil
}
