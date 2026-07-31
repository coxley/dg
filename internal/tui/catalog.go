package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	canvasstore "github.com/coxley/dg/store"
)

func (m *Model) startCatalogWatch() tea.Cmd {
	if m.canvasStore == nil || m.catalogFeed != nil {
		return waitCatalog(m.catalogFeed)
	}
	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // The TUI model owns this process-lifetime context.
	events, err := m.canvasStore.Watch(ctx)
	if err != nil {
		cancel()
		return func() tea.Msg { return canvasstore.CatalogEvent{Err: err, Closed: true} }
	}
	m.cancelWatch = cancel
	m.catalogFeed = events
	return waitCatalog(events)
}

func waitCatalog(events <-chan canvasstore.CatalogEvent) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return canvasstore.CatalogEvent{Closed: true}
		}
		return event
	}
}

func reconcileCatalog(store *canvasstore.Store, previous []canvasstore.Entry) tea.Cmd {
	if store == nil {
		return nil
	}
	return func() tea.Msg { return store.Reconcile(previous) }
}

func (m *Model) updateCatalog(event canvasstore.CatalogEvent) {
	if event.Err != nil {
		m.setError(event.Err.Error())
	}
	if event.Entries != nil {
		m.catalog = event.Entries
		m.rebuildSidebarCatalog()
	}
	if m.entry == nil {
		return
	}
	for _, change := range event.Changes {
		if !sameCanvas(changeEntry(change), *m.entry) {
			continue
		}
		if change.External {
			m.handleExternalChange(change)
			continue
		}
		switch change.Kind {
		case canvasstore.ChangeAdded, canvasstore.ChangeModified:
			m.setActiveEntry(change.Entry)
		case canvasstore.ChangeDeleted:
		}
	}
}

func changeEntry(change canvasstore.CatalogChange) canvasstore.Entry {
	if change.Kind == canvasstore.ChangeDeleted {
		return change.Previous
	}
	return change.Entry
}

func sameCanvas(a, b canvasstore.Entry) bool {
	if a.Draft || b.Draft {
		return a.Draft == b.Draft && a.ID == b.ID
	}
	return a.Section == b.Section && a.Name == b.Name
}
