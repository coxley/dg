package tui

import (
	"errors"
	"fmt"

	"github.com/coxley/dg/layout"
	canvasstore "github.com/coxley/dg/store"
)

var errExternalConflictChanged = errors.New("external conflict changed")

type externalConflict struct {
	entry   canvasstore.Entry
	deleted bool
}

type externalChoiceMsg bool

func (m *Model) handleExternalChange(change canvasstore.CatalogChange) {
	entry := change.Previous
	if change.Kind != canvasstore.ChangeDeleted {
		entry = change.Entry
	}
	if change.Kind == canvasstore.ChangeDeleted {
		current, err := m.canvasStore.LoadCurrentInto(change.Previous, &m.externalDoc)
		switch {
		case errors.Is(err, canvasstore.ErrEntryNotFound):
			m.openExternalConflict(externalConflict{entry: change.Previous, deleted: true})
			return
		case err != nil:
			m.openExternalConflict(externalConflict{entry: change.Previous})
			return
		default:
			entry = current
		}
	} else {
		current, err := m.canvasStore.LoadCurrentInto(entry, &m.externalDoc)
		if err != nil {
			m.setError(fmt.Sprintf("inspect external canvas: %v", err))
			return
		}
		entry = current
	}
	if m.entry != nil && entry.ID != m.entry.ID {
		if err := m.adoptExternalReplacement(entry); err != nil {
			m.setError(fmt.Sprintf("adopt external canvas: %v", err))
		}
		return
	}
	m.openExternalConflict(externalConflict{entry: entry})
}

func (m *Model) openExternalConflict(conflict externalConflict) {
	m.interruptInteraction()
	m.external = &conflict
	title := canvasTitle(conflict.entry)
	if conflict.deleted {
		m.dialogs.OpenChoice(
			"External deletion",
			title+" was externally deleted; restore it?",
			"Restore",
			externalChoiceMsg(true),
			"Keep as Draft",
			externalChoiceMsg(false),
		)
		return
	}
	m.dialogs.OpenChoice(
		"External modification",
		title+" has been externally modified; load it?",
		"Load",
		externalChoiceMsg(true),
		"Keep Mine",
		externalChoiceMsg(false),
	)
}

func (m *Model) resolveExternal(load bool) {
	if m.external == nil || m.entry == nil {
		m.dialogs.CloseWithoutMessage()
		return
	}
	conflict := *m.external
	var err error
	if conflict.deleted {
		if load {
			err = m.restoreDeletedCanvas(conflict.entry)
		} else {
			err = m.preserveDeletedCanvas()
		}
	} else if load {
		err = m.loadExternalCanvas(conflict.entry)
	} else {
		err = m.keepLocalCanvas(conflict.entry)
	}
	if err != nil {
		if errors.Is(err, errExternalConflictChanged) {
			return
		}
		m.setError(err.Error())
		return
	}
	m.external = nil
	m.dialogs.CloseWithoutMessage()
}

func (m *Model) loadExternalCanvas(entry canvasstore.Entry) error {
	current, err := m.canvasStore.LoadCurrentInto(entry, &m.externalDoc)
	if errors.Is(err, canvasstore.ErrEntryNotFound) {
		m.openExternalConflict(externalConflict{entry: entry, deleted: true})
		return errExternalConflictChanged
	}
	if err != nil {
		return fmt.Errorf("load external canvas: %w", err)
	}
	if current.ID != m.document.ID {
		return m.adoptExternalReplacement(current)
	}
	m.document.Update(m.geo)
	if err := m.replaceActiveDocument(current, true); err != nil {
		return fmt.Errorf("reload external canvas: %w", err)
	}
	m.status = "loaded external changes"
	m.statusError = ""
	return nil
}

func (m *Model) keepLocalCanvas(entry canvasstore.Entry) error {
	m.document.Update(m.geo)
	backup, restored, err := m.canvasStore.BackupAndRestore(entry, m.document)
	if errors.Is(err, canvasstore.ErrEntryNotFound) {
		m.openExternalConflict(externalConflict{entry: entry, deleted: true})
		return errExternalConflictChanged
	}
	if err != nil {
		return fmt.Errorf("keep local canvas: %w", err)
	}
	m.setActiveEntry(restored)
	if err := m.history.Save(m.document); err != nil {
		m.setError(fmt.Sprintf("save local history: %v", err))
	} else {
		m.statusError = ""
	}
	m.saved = m.dirty
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
	m.status = "kept local; saved " + canvasTitle(backup)
	return nil
}

func (m *Model) restoreDeletedCanvas(entry canvasstore.Entry) error {
	m.document.Update(m.geo)
	restored, err := m.canvasStore.RestoreDeleted(entry, m.document)
	if err != nil {
		return fmt.Errorf("restore deleted canvas: %w", err)
	}
	m.setActiveEntry(restored)
	if err := m.history.Save(m.document); err != nil {
		m.setError(fmt.Sprintf("save restored history: %v", err))
	} else {
		m.statusError = ""
	}
	m.saved = m.dirty
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
	m.status = "restored " + canvasTitle(restored)
	return nil
}

func (m *Model) preserveDeletedCanvas() error {
	m.document.Update(m.geo)
	draft, err := m.canvasStore.PreserveDraft(m.document)
	if err != nil {
		return fmt.Errorf("preserve deleted canvas: %w", err)
	}
	m.setActiveEntry(draft)
	if err := m.history.Save(m.document); err != nil {
		m.setError(fmt.Sprintf("save draft history: %v", err))
	} else {
		m.statusError = ""
	}
	m.saved = m.dirty
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
	m.status = "preserved as draft"
	return nil
}

func (m *Model) adoptExternalReplacement(entry canvasstore.Entry) error {
	m.interruptInteraction()
	m.document.Update(m.geo)
	if _, err := m.canvasStore.PreserveDraft(m.document); err != nil {
		return fmt.Errorf("preserve previous canvas: %w", err)
	}
	historyErr := m.history.Save(m.document)
	if err := m.replaceActiveDocument(entry, false); err != nil {
		return err
	}
	m.external = nil
	m.dialogs.CloseWithoutMessage()
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
	m.status = "opened replacement " + canvasTitle(entry)
	if historyErr != nil {
		m.status += fmt.Sprintf(" (previous history: %v)", historyErr)
		m.statusError = m.status
	} else {
		m.statusError = ""
	}
	return nil
}

func (m *Model) replaceActiveDocument(entry canvasstore.Entry, reload bool) error {
	previous := m.document
	m.document = m.externalDoc
	replace := func() error { return m.document.ConvertInto(m.geo) }
	var err error
	if reload {
		err = m.history.Reload(replace)
	} else {
		err = m.history.Reset(replace)
	}
	if err != nil {
		m.externalDoc = m.document
		m.document = previous
		return err
	}
	m.externalDoc = previous
	m.setActiveEntry(entry)
	m.dirty++
	m.saved = m.dirty
	m.viewport = layout.NewPoint(0, 0)
	m.clearSelection()
	if err := m.rebuild(); err != nil {
		return err
	}
	if reload {
		if err := m.history.Save(m.document); err != nil {
			m.setError(fmt.Sprintf("save external history: %v", err))
		}
		return nil
	}
	if _, err := m.history.Restore(m.document); err != nil {
		m.setError(fmt.Sprintf("restore replacement history: %v", err))
	}
	return nil
}
