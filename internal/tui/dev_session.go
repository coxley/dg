package tui

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/coxley/dg/layout"
	"github.com/google/uuid"
)

const devSessionVersion = 1

// DevSession contains semantic editor state that survives development process
// replacement.
type DevSession struct {
	Version     uint32            `json:"version"`
	Document    document.Document `json:"document"`
	EntryID     uuid.UUID         `json:"entry_id,omitempty"`
	NeedsSave   bool              `json:"needs_save,omitempty"`
	Cursor      layout.Point      `json:"cursor"`
	Viewport    layout.Point      `json:"viewport"`
	Selection   devSelection      `json:"selection"`
	Active      *layout.Hit       `json:"active,omitempty"`
	Tool        activeTool        `json:"tool"`
	NodeStyle   layout.NodeStyle  `json:"node_style"`
	EdgeStyle   layout.EdgeStyle  `json:"edge_style"`
	Sidebar     devSidebar        `json:"sidebar"`
	Help        devHelp           `json:"help"`
	Preferences *devPreferences   `json:"preferences,omitempty"`
}

type devSelection struct {
	Nodes  []uint32 `json:"nodes,omitempty"`
	Groups []uint32 `json:"groups,omitempty"`
	Edges  []uint32 `json:"edges,omitempty"`
}

type devSidebar struct {
	Open            bool           `json:"open,omitempty"`
	Focused         bool           `json:"focused,omitempty"`
	DraftsCollapsed bool           `json:"drafts_collapsed,omitempty"`
	FocusedID       chrome.FocusID `json:"focused_id,omitempty"`
	ScrollY         int            `json:"scroll_y,omitempty"`
	Width           int            `json:"width,omitempty"`
	Collapsed       []string       `json:"collapsed,omitempty"`
}

type devHelp struct {
	Visible    bool        `json:"visible,omitempty"`
	Requested  chrome.Rect `json:"requested"`
	Positioned bool        `json:"positioned,omitempty"`
	ScrollY    int         `json:"scroll_y,omitempty"`
}

type devPreferences struct {
	Baseline  preferenceDialogValue `json:"baseline"`
	Draft     preferenceDialogValue `json:"draft"`
	FocusedID chrome.ID             `json:"focused_id,omitempty"`
}

// ConsumeDevSession decodes and removes the handoff at path.
func ConsumeDevSession(path string) (DevSession, bool, error) {
	if path == "" {
		return DevSession{}, false, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DevSession{}, false, nil
	}
	if err != nil {
		return DevSession{}, false, fmt.Errorf("read development session: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return DevSession{}, false, fmt.Errorf("remove development session: %w", err)
	}
	session, err := decodeDevSession(data)
	if err != nil {
		return DevSession{}, false, err
	}
	return session, true, nil
}

func (m *Model) writeDevSession(path string) error {
	session := m.captureDevSession()
	if err := m.prepareDevReload(session.Preferences != nil); err != nil {
		return err
	}
	m.document.Update(m.geo)
	session.Document = m.document
	if err := m.history.Save(session.Document); err != nil {
		return fmt.Errorf("save development history: %w", err)
	}
	data, err := encodeDevSession(session)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write development session: %w", err)
	}
	return nil
}

func (m *Model) captureDevSession() DevSession {
	session := DevSession{
		Version:   devSessionVersion,
		NeedsSave: m.dirty != m.saved,
		Cursor:    m.cursor,
		Viewport:  m.viewport,
		Tool:      m.interaction.tool,
		NodeStyle: m.nodeStyle,
		EdgeStyle: m.edgeStyle,
		Sidebar: devSidebar{
			Open:            m.sidebar.open,
			Focused:         m.sidebar.focused,
			DraftsCollapsed: m.sidebar.draftsCollapsed,
			ScrollY:         m.sidebar.viewport.Plan().Offset.Y,
			Width:           m.sidebar.desired,
			Collapsed:       collapsedSections(m.sidebar.collapsed),
		},
		Help: devHelp{
			Visible:    m.helpInspector.visible,
			Requested:  m.helpInspector.requested,
			Positioned: m.helpInspector.positioned,
			ScrollY:    m.helpInspector.viewport.Plan().Offset.Y,
		},
	}
	if m.entry != nil {
		session.EntryID = m.entry.ID
	}
	_, session.Sidebar.FocusedID = m.sidebar.focus.Current()
	for nodeID := range m.geo.Selection().DirectNodes() {
		session.Selection.Nodes = append(session.Selection.Nodes, nodeID)
	}
	for groupID := range m.geo.Selection().Groups() {
		session.Selection.Groups = append(session.Selection.Groups, groupID)
	}
	for edgeID := range m.geo.Selection().Edges() {
		session.Selection.Edges = append(session.Selection.Edges, edgeID)
	}
	if hit, ok := m.activeHit(); ok {
		session.Active = &hit
	}
	if m.dialogs.ActiveID() == surfacePreferences {
		session.Preferences = &devPreferences{
			Baseline:  m.preferences.baseline,
			Draft:     m.preferences.draft,
			FocusedID: m.dialogs.preferences.model.FocusID(),
		}
	}
	return session
}

func collapsedSections(collapsed map[string]bool) []string {
	sections := make([]string, 0, len(collapsed))
	for section, value := range collapsed {
		if value {
			sections = append(sections, section)
		}
	}
	slices.Sort(sections)
	return sections
}

func (m *Model) prepareDevReload(preferencesOpen bool) error {
	m.clipboard.CancelPending()
	if preferencesOpen {
		m.cancelPreferences()
		return nil
	}
	if m.arrangeOpen {
		m.cancelArrange()
	}
	if m.interaction.session.kind == sessionLabelEdit {
		m.commitLabelEdit()
	} else if err := m.cancelTransaction(); err != nil {
		return fmt.Errorf("cancel development reload interaction: %w", err)
	}
	m.interaction.controlDrag = controlDrag{}
	m.interaction.session = interactionSession{}
	m.clearConnection()
	m.clearBendDrag()
	m.cancelDuplicateDrag()
	m.interaction.resetGesture()
	m.dialogs.CloseWithoutMessage()
	if err := m.render(); err != nil {
		return err
	}
	m.refreshHits()
	return nil
}

func (m *Model) restoreDevSession(session DevSession) {
	m.cursor = session.Cursor
	m.viewport = session.Viewport
	m.nodeStyle = session.NodeStyle
	m.edgeStyle = session.EdgeStyle
	m.dirty = 1
	m.saved = 1
	if session.NeedsSave {
		m.saved = 0
	}
	m.geo.Selection().Clear()
	for _, nodeID := range session.Selection.Nodes {
		m.geo.Selection().Toggle(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	}
	for _, groupID := range session.Selection.Groups {
		m.geo.Selection().Toggle(layout.Hit{ID: groupID, Kind: layout.HitGroup})
	}
	for _, edgeID := range session.Selection.Edges {
		m.geo.Selection().Toggle(layout.Hit{ID: edgeID, Kind: layout.HitEdge})
	}
	m.refreshSelectionHighlight()

	clear(m.sidebar.collapsed)
	for _, section := range session.Sidebar.Collapsed {
		m.sidebar.collapsed[section] = true
	}
	m.sidebar.draftsCollapsed = session.Sidebar.DraftsCollapsed
	m.sidebar.desired = session.Sidebar.Width
	m.rebuildSidebarCatalog()
	switch {
	case !session.Sidebar.Open:
		m.sidebar.hide()
	case session.Sidebar.Focused:
		m.sidebar.show()
		m.sidebar.focusTarget(session.Sidebar.FocusedID)
	default:
		m.sidebar.openInitially()
	}

	m.helpInspector.visible = session.Help.Visible
	m.helpInspector.requested = session.Help.Requested
	m.helpInspector.positioned = session.Help.Positioned
	m.syncWorkspace()
	m.sidebar.viewport.Scroll(0, session.Sidebar.ScrollY)
	m.helpInspector.viewport.Scroll(0, session.Help.ScrollY)

	if session.Preferences != nil {
		preferences := session.Preferences
		m.preferences.baseline = preferences.Baseline
		m.preferences.draft = preferences.Baseline
		m.openPreferences()
		m.dialogs.preferences.Reset(preferences.Draft)
		m.previewPreferences(preferences.Draft)
		m.dialogs.preferences.model.Focus(preferences.FocusedID)
	}
	m.interaction.tool = session.Tool
	m.refreshHits()
	if session.Active != nil {
		for i, hit := range m.hits {
			if hit == *session.Active {
				m.active = i
				m.target = hit
				break
			}
		}
	}
}

func encodeDevSession(session DevSession) ([]byte, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if err := json.NewEncoder(writer).Encode(session); err != nil {
		return nil, errors.Join(fmt.Errorf("encode development session: %w", err), writer.Close())
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("compress development session: %w", err)
	}
	return compressed.Bytes(), nil
}

func decodeDevSession(data []byte) (DevSession, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return DevSession{}, fmt.Errorf("open development session: %w", err)
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var session DevSession
	decodeErr := decoder.Decode(&session)
	var trailing json.RawMessage
	trailingErr := decoder.Decode(&trailing)
	closeErr := reader.Close()
	if decodeErr != nil {
		return DevSession{}, fmt.Errorf("decode development session: %w", decodeErr)
	}
	if !errors.Is(trailingErr, io.EOF) {
		return DevSession{}, errors.New("development session contains trailing data")
	}
	if closeErr != nil {
		return DevSession{}, fmt.Errorf("close development session: %w", closeErr)
	}
	if session.Version != devSessionVersion {
		return DevSession{}, fmt.Errorf("unsupported development session version %d", session.Version)
	}
	if session.Document.Version != document.CurrentVersion {
		encoded, err := json.Marshal(session.Document)
		if err != nil {
			return DevSession{}, fmt.Errorf("encode development session document: %w", err)
		}
		var migrated document.Document
		if _, err := document.Migrate(encoded, &migrated); err != nil {
			return DevSession{}, fmt.Errorf("migrate development session document: %w", err)
		}
		session.Document = migrated
	}
	return session, nil
}
