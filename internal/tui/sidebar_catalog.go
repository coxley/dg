package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	canvasstore "github.com/coxley/dg/store"
)

func (m *Model) rebuildSidebarCatalog() {
	if m.canvasStore == nil {
		return
	}
	canvasItems, canvasLabels := m.canvasSidebarItems()
	draftItems, draftLabels := m.draftSidebarItems()
	labels := append(canvasLabels, draftLabels...)
	if m.sidebar.drafts {
		m.sidebar.setContent("Canvases  [Drafts]", draftItems, labels)
		return
	}
	m.sidebar.setContent("[Canvases]  Drafts", canvasItems, labels)
}

func (m *Model) canvasSidebarItems() ([]sidebarItem, []string) {
	var root []canvasstore.Entry
	sections := make(map[string][]canvasstore.Entry)
	var labels []string
	for _, entry := range m.catalog {
		if entry.Draft {
			continue
		}
		label := canvasEntryLabel(entry, false)
		labels = append(labels, label)
		if entry.Section == "" {
			root = append(root, entry)
		} else {
			sections[entry.Section] = append(sections[entry.Section], entry)
			labels = append(labels, "["+entry.Section+"]")
		}
	}
	items := make([]sidebarItem, 0, len(root)+len(sections))
	for _, entry := range root {
		items = append(items, canvasSidebarItem(entry, false))
	}
	names := make([]string, 0, len(sections))
	for section := range sections {
		names = append(names, section)
	}
	slices.Sort(names)
	for _, section := range names {
		collapsed := m.sidebar.collapsed[section]
		marker := "▾ "
		if collapsed {
			marker = "▸ "
		}
		items = append(items, sidebarItem{
			ID:      chrome.FocusID("section:" + section),
			Label:   marker + "[" + section + "]",
			Kind:    sidebarItemSection,
			Section: section,
		})
		if collapsed {
			continue
		}
		for _, entry := range sections[section] {
			items = append(items, canvasSidebarItem(entry, true))
		}
	}
	return items, labels
}

func (m *Model) draftSidebarItems() ([]sidebarItem, []string) {
	var drafts []canvasstore.Entry
	for _, entry := range m.catalog {
		if entry.Draft {
			drafts = append(drafts, entry)
		}
	}
	slices.SortFunc(drafts, func(a, b canvasstore.Entry) int {
		return b.Modified.Compare(a.Modified)
	})
	items := make([]sidebarItem, 0, len(drafts)+1)
	labels := make([]string, 0, len(drafts)+1)
	for _, entry := range drafts {
		label := entry.Modified.Local().Format("Jan 2, 15:04")
		labels = append(labels, label)
		items = append(items, sidebarItem{
			ID:    chrome.FocusID("draft:" + entry.ID.String()),
			Label: label,
			Kind:  sidebarItemRecord,
			Entry: entry,
		})
	}
	if len(drafts) != 0 {
		const label = "Clear Drafts..."
		labels = append(labels, label)
		items = append(items, sidebarItem{
			ID:    "drafts:clear",
			Label: label,
			Kind:  sidebarItemClearDrafts,
		})
	}
	return items, labels
}

func canvasSidebarItem(entry canvasstore.Entry, indented bool) sidebarItem {
	return sidebarItem{
		ID:    chrome.FocusID("canvas:" + entry.Section + "/" + entry.Name),
		Label: canvasEntryLabel(entry, indented),
		Kind:  sidebarItemRecord,
		Entry: entry,
	}
}

func canvasEntryLabel(entry canvasstore.Entry, indented bool) string {
	label := entry.Name
	if indented {
		label = "  " + label
	}
	if backupName(entry.Name) {
		label += " [backup]"
	}
	return label
}

func backupName(name string) bool {
	index := strings.LastIndex(name, ".bak")
	if index < 0 || index+4 == len(name) {
		return index >= 0 && index+4 == len(name)
	}
	for _, char := range name[index+4:] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (m *Model) switchSidebarTab(delta int) tea.Cmd {
	if delta == 0 {
		return nil
	}
	m.sidebar.drafts = !m.sidebar.drafts
	m.rebuildSidebarCatalog()
	return m.retargetSidebar()
}

func (m *Model) activateSidebar() tea.Cmd {
	item, ok := m.sidebar.focusedItem()
	if !ok {
		return nil
	}
	switch item.Kind {
	case sidebarItemRecord:
		if err := m.switchCanvas(item.Entry); err != nil {
			m.setError(err.Error())
		}
	case sidebarItemSection:
		m.sidebar.collapsed[item.Section] = !m.sidebar.collapsed[item.Section]
		m.rebuildSidebarCatalog()
		return m.retargetSidebar()
	case sidebarItemClearDrafts:
		m.openClearDraftsConfirmation()
	}
	return nil
}

func (m *Model) deleteFocusedCanvas() {
	item, ok := m.sidebar.focusedItem()
	if !ok || item.Kind != sidebarItemRecord {
		return
	}
	entry := item.Entry
	active := m.entry != nil && sameCanvas(*m.entry, entry)
	if !entry.Draft {
		m.demoteCanvas(entry, active)
		return
	}
	if active {
		m.setError("cannot delete the active draft")
		return
	}
	if err := m.canvasStore.Delete(entry); err != nil {
		m.setError(fmt.Sprintf("delete draft: %v", err))
		return
	}
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
}

func (m *Model) demoteCanvas(entry canvasstore.Entry, active bool) {
	title := canvasTitle(entry)
	if active {
		if err := m.flushActive(); err != nil {
			m.setError(fmt.Sprintf("save canvas before deletion: %v", err))
			return
		}
		entry = *m.entry
	}
	draft, err := m.canvasStore.Demote(entry)
	if err != nil {
		m.setError(fmt.Sprintf("move canvas to drafts: %v", err))
		return
	}
	if active {
		m.setActiveEntry(draft)
	}
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
	m.status = fmt.Sprintf("moved %s to Drafts", title)
	m.statusError = ""
}

func (s *sidebarState) focusedItem() (sidebarItem, bool) {
	_, focused := s.focus.Current()
	for _, item := range s.declaration.Items {
		if item.ID == focused {
			return item, true
		}
	}
	return sidebarItem{}, false
}
