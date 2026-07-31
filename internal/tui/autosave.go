package tui

import (
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/layout"
	canvasstore "github.com/coxley/dg/store"
)

const autosaveDelay = 500 * time.Millisecond

type autosaveMsg uint64

func (m *Model) markDocumentDirty() {
	m.dirty++
}

func autosaveAfter(generation uint64) tea.Cmd {
	return tea.Tick(autosaveDelay, func(time.Time) tea.Msg {
		return autosaveMsg(generation)
	})
}

func (m *Model) updatePersistence(message tea.Msg) (tea.Cmd, bool) {
	switch message := message.(type) {
	case autosaveMsg:
		if uint64(message) == m.dirty && m.dirty != m.saved && m.canvasStore != nil {
			if err := m.persistActive(); err != nil {
				m.setError(err.Error())
			}
		}
		return nil, true
	case canvasstore.CatalogEvent:
		m.updateCatalog(message)
		if message.Closed {
			return nil, true
		}
		return waitCatalog(m.catalogFeed), true
	case flushRequestMsg:
		return m.handleFlushRequest(message), true
	default:
		return nil, false
	}
}

func (m *Model) persistActive() error {
	if m.canvasStore == nil || m.entry == nil {
		return errors.New("no active canvas store entry")
	}
	m.document.Update(m.geo)
	entry, err := m.canvasStore.Save(*m.entry, m.document)
	if err != nil {
		return fmt.Errorf("autosave canvas: %w", err)
	}
	*m.entry = entry
	for i := range m.catalog {
		if sameCanvas(m.catalog[i], entry) {
			m.catalog[i] = entry
			break
		}
	}
	if err := m.history.Save(m.document); err != nil {
		return fmt.Errorf("save canvas history: %w", err)
	}
	m.saved = m.dirty
	m.status = "autosaved"
	m.statusError = ""
	return nil
}

func (m *Model) flushActive() error {
	m.interruptInteraction()
	var errs []error
	if m.canvasStore != nil && m.entry != nil && m.dirty != m.saved {
		errs = append(errs, m.persistActive())
	}
	if m.history != nil && m.history.Dirty() {
		errs = append(errs, m.history.Flush())
	}
	return errors.Join(errs...)
}

func (m *Model) switchCanvas(entry canvasstore.Entry) error {
	if m.canvasStore == nil {
		return errors.New("no canvas store configured")
	}
	if m.entry != nil && sameCanvas(*m.entry, entry) {
		return nil
	}
	if err := m.flushActive(); err != nil {
		return err
	}
	previousID := m.document.ID
	if err := m.canvasStore.LoadInto(entry, &m.document); err != nil {
		m.document.ID = previousID
		m.document.Update(m.geo)
		return err
	}
	if err := m.history.Reset(func() error {
		return m.document.ConvertInto(m.geo)
	}); err != nil {
		m.document.ID = previousID
		m.document.Update(m.geo)
		return err
	}
	_, cacheErr := m.history.Restore(m.document)
	active := entry
	m.entry = &active
	m.dirty++
	m.saved = m.dirty
	m.viewport = layout.NewPoint(0, 0)
	m.clearSelection()
	if err := m.rebuild(); err != nil {
		return err
	}
	m.status = canvasTitle(entry)
	if cacheErr != nil {
		m.status += fmt.Sprintf(" (undo history: %v)", cacheErr)
		m.statusError = m.status
	} else {
		m.statusError = ""
	}
	return nil
}

func canvasTitle(entry canvasstore.Entry) string {
	if entry.Draft {
		return "draft"
	}
	if entry.Section == "" {
		return entry.Name
	}
	return entry.Section + "/" + entry.Name
}
