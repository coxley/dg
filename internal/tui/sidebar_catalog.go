package tui

import (
	"errors"
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
	items := append(canvasItems, sidebarItem{
		ID: sidebarDraftsDivider, Label: sidebarDividerLabel, Kind: sidebarItemDivider,
	})
	marker := "▾ "
	if m.sidebar.draftsCollapsed {
		marker = "▸ "
	}
	items = append(items, sidebarItem{
		ID: sidebarDraftsSection, Label: marker + "[Drafts]",
		Kind: sidebarItemSection, Drafts: true,
	})
	if !m.sidebar.draftsCollapsed {
		items = append(items, draftItems...)
	}
	m.sidebar.setContent(items, labels)
	m.sidebar.setActive(m.entry)
}

func (m *Model) canvasSidebarItems() ([]sidebarItem, []string) {
	var root []canvasstore.Entry
	sections := make(map[string][]canvasstore.Entry)
	var labels []string
	for _, entry := range m.catalog {
		if entry.Draft {
			continue
		}
		label := canvasEntryLabel(entry)
		measured := label
		if entry.Section != "" {
			measured = sidebarNestedIndent + measured
		}
		labels = append(labels, measured)
		if entry.Section == "" {
			root = append(root, entry)
		} else {
			sections[entry.Section] = append(sections[entry.Section], entry)
			labels = append(labels, "["+entry.Section+"]")
		}
	}
	items := make([]sidebarItem, 0, len(root)+len(sections))
	for _, entry := range root {
		items = append(items, canvasSidebarItem(entry))
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
			items = append(items, canvasSidebarItem(entry))
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
		label := draftCanvasName(entry)
		labels = append(labels, sidebarNestedIndent+label)
		items = append(items, sidebarItem{
			ID: chrome.FocusID("draft:" + entry.ID.String()), Label: label,
			Kind: sidebarItemRecord, Entry: entry, Drafts: true,
		})
	}
	if len(drafts) != 0 {
		const label = "[Clear Drafts]"
		labels = append(labels, sidebarNestedIndent+label)
		items = append(items, sidebarItem{
			ID: "drafts:clear", Label: label,
			Kind: sidebarItemClearDrafts, Drafts: true,
		})
	}
	return items, labels
}

func draftCanvasName(entry canvasstore.Entry) string {
	return entry.Modified.Local().Format("Jan 2, 15:04")
}

func canvasSidebarItem(entry canvasstore.Entry) sidebarItem {
	return sidebarItem{
		ID:    chrome.FocusID("canvas:" + entry.Section + "/" + entry.Name),
		Label: canvasEntryLabel(entry),
		Kind:  sidebarItemRecord,
		Entry: entry,
	}
}

func canvasEntryLabel(entry canvasstore.Entry) string {
	label := entry.Name
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
	if delta == 0 || len(sidebarTabs) < 2 {
		return nil
	}
	index := slices.IndexFunc(sidebarTabs[:], func(tab sidebarTab) bool {
		return tab.ID == m.sidebar.activeTab
	})
	if index < 0 {
		index = 0
	}
	index = (index + len(sidebarTabs) + delta%len(sidebarTabs)) % len(sidebarTabs)
	return m.selectSidebarTab(sidebarTabs[index].ID)
}

func (m *Model) selectSidebarTab(id chrome.FocusID) tea.Cmd {
	m.sidebar.activeTab = id
	m.rebuildSidebarCatalog()
	if m.sidebar.focused {
		m.sidebar.focusTab(id)
	}
	return m.retargetSidebar()
}

func (m *Model) activateSidebar() tea.Cmd {
	if tab, ok := m.sidebar.focusedTab(); ok {
		return m.selectSidebarTab(tab.ID)
	}
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
		if item.Drafts {
			m.sidebar.draftsCollapsed = !m.sidebar.draftsCollapsed
		} else {
			m.sidebar.collapsed[item.Section] = !m.sidebar.collapsed[item.Section]
		}
		m.rebuildSidebarCatalog()
		return m.retargetSidebar()
	case sidebarItemClearDrafts:
		m.openClearDraftsConfirmation()
	case sidebarItemDivider:
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

func (m *Model) moveCanvasToSection(entry canvasstore.Entry, section string) tea.Cmd {
	if entry.Draft {
		return m.nameDraftInSection(entry, section)
	}
	if entry.Section == section {
		return nil
	}
	active := m.entry != nil && sameCanvas(*m.entry, entry)
	if active {
		if err := m.flushActive(); err != nil {
			m.setError(fmt.Sprintf("save canvas before moving: %v", err))
			return nil
		}
		entry = *m.entry
	}
	moved, err := m.canvasStore.Move(entry, section, entry.Name)
	if err != nil {
		if errors.Is(err, canvasstore.ErrEntryExists) {
			m.setError("a canvas with that name already exists in the destination")
			return nil
		}
		m.setError(fmt.Sprintf("move canvas: %v", err))
		return nil
	}
	if active {
		m.setActiveEntry(moved)
	}
	m.sidebar.collapsed[section] = false
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
	m.sidebar.focusTarget(canvasSidebarItem(moved).ID)
	m.sidebar.focus.Reveal(m.sidebar.viewport)
	m.sidebar.render()
	m.status = "moved " + moved.Name
	if section == "" {
		m.status += " to Canvases"
	} else {
		m.status += " to " + section
	}
	m.statusError = ""
	return m.retargetSidebar()
}

func (m *Model) nameDraftInSection(entry canvasstore.Entry, section string) tea.Cmd {
	name := draftCanvasName(entry)
	active := m.entry != nil && sameCanvas(*m.entry, entry)
	if active {
		if err := m.flushActive(); err != nil {
			m.setError(fmt.Sprintf("save draft before naming: %v", err))
			return nil
		}
		entry = *m.entry
	}
	named, err := m.nameDraftUniquely(entry, section, name)
	if err != nil {
		m.setError(fmt.Sprintf("name draft: %v", err))
		return nil
	}
	if active {
		m.setActiveEntry(named)
	}
	m.sidebar.collapsed[section] = false
	m.updateCatalog(m.canvasStore.Reconcile(m.catalog))
	m.sidebar.focusTarget(canvasSidebarItem(named).ID)
	m.sidebar.focus.Reveal(m.sidebar.viewport)
	m.sidebar.render()
	m.status = "saved " + named.Name
	if section != "" {
		m.status += " to " + section
	}
	m.statusError = ""
	return m.retargetSidebar()
}

func (m *Model) nameDraftUniquely(
	entry canvasstore.Entry,
	section, base string,
) (canvasstore.Entry, error) {
	for suffix := 1; ; suffix++ {
		name := base
		if suffix > 1 {
			name = fmt.Sprintf("%s (%d)", base, suffix)
		}
		named, err := m.canvasStore.Name(entry, section, name)
		if !errors.Is(err, canvasstore.ErrEntryExists) {
			return named, err
		}
	}
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
