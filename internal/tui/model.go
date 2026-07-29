// Package tui provides an interactive terminal editor for diagram layouts.
package tui

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/internal/settings"
	canvasview "github.com/coxley/dg/internal/tui/canvas"
	"github.com/coxley/dg/internal/tui/chrome"
	clipboardview "github.com/coxley/dg/internal/tui/clipboard"
	modalview "github.com/coxley/dg/internal/tui/modal"
	"github.com/coxley/dg/internal/tui/nav"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
	"github.com/coxley/dg/layout"
)

type mode uint8

type resizeCorner uint8

const (
	modeNavigate mode = iota
	modeMove
	modeEditLabel
	modeConnect
	modeRectangle
)

const (
	resizeEast resizeCorner = 1 << iota
	resizeSouth
)

const (
	finishOperation = "finish the current operation first"
	dragFromSource  = "drag from a source port"
	noticeDuration  = time.Second
)

func (m mode) String() string {
	switch m {
	case modeNavigate:
		return "navigate"
	case modeMove:
		return "move"
	case modeEditLabel:
		return "edit label"
	case modeConnect:
		return "connect"
	case modeRectangle:
		return string(commandRectangle)
	default:
		return "unknown"
	}
}

// Model coordinates terminal interaction with a layout.
type Model struct {
	geo     *layout.Layout
	history *layout.History
	theme   Theme

	cursor      layout.Point
	viewport    layout.Point
	hits        []layout.Hit
	active      int
	target      layout.Hit
	interaction interactionState
	nav         nav.Model
	dialogs     dialogController
	canvas      canvasview.Model
	bindings    *chrome.Resolver
	workspace   chrome.Workspace
	sidebar     sidebarState

	editBuffer       []byte
	editDraft        []byte
	editLines        []layout.LabelLine
	editCaret        int
	editCaretVisible bool

	viewBuffer []byte
	statusText []byte
	styledRuns map[styledRunKey]string

	// Bubble Tea compares consecutive cursor pointers, so each View writes the
	// cursor value that the previous View does not reference.
	viewCursor [2]tea.Cursor
	nextCursor uint8

	width       int
	height      int
	status      string
	statusError string
	path        string

	clipboard *clipboardview.Model

	preferences    preferenceState
	settingsStore  *settings.Store
	preferenceEdit bool
	helpInspector  helpInspector

	nodeStyle layout.NodeStyle
	edgeStyle layout.EdgeStyle

	moveOrigins []layout.Point
	focusNodes  []layout.Hit
}

// Option configures model construction.
type Option func(*modelOptions)

type modelOptions struct {
	settings settings.Snapshot
	store    *settings.Store
}

// WithSettings configures the initial settings snapshot and its durable store.
func WithSettings(snapshot settings.Snapshot, store *settings.Store) Option {
	return func(options *modelOptions) {
		options.settings = snapshot
		options.store = store
	}
}

// New returns a TUI model for geo.
func New(geo *layout.Layout, options ...Option) (*Model, error) {
	return newModel(geo, "", options...)
}

func newModel(geo *layout.Layout, path string, options ...Option) (*Model, error) {
	if geo == nil {
		return nil, errors.New("nil layout")
	}
	var configured modelOptions
	for _, option := range options {
		option(&configured)
	}
	m := &Model{
		geo:           geo,
		history:       geo.History(),
		path:          path,
		theme:         DefaultTheme(true),
		helpInspector: newHelpInspector(),
		styledRuns:    make(map[styledRunKey]string),
		settingsStore: configured.store,
	}
	resolver, err := chrome.NewResolver(applicationBindings)
	if err != nil {
		return nil, fmt.Errorf("configure bindings: %w", err)
	}
	m.bindings = resolver
	m.workspace.SetFooter(1)
	m.applySettingsSnapshot(configured.settings)
	m.sidebar = newSidebar(sidebarDeclaration{
		Header: "SIDEBAR",
		Items: []sidebarItem{
			{ID: "overview", Label: "Overview"},
			{ID: "selection", Label: "Selection"},
			{ID: "document", Label: "Document"},
		},
		Footer: "Esc canvas",
	}, m.theme.sidebarStyles())
	m.syncSidebarShortcut()
	m.clipboard = clipboardview.New(m.theme.formStyles())
	m.dialogs = newDialogController(m.theme, m.clipboard, m.preferenceValue())
	m.nav = nav.New(m.theme.Nav, []nav.Item{
		{ID: "cursor", Tool: nav.Cursor, Label: " Cursor "},
		{ID: "rectangle", Tool: nav.Rectangle, Label: " Rectangle "},
		{ID: "line", Tool: nav.Line, Label: " Line "},
	})
	m.canvas = canvasview.New(m.theme.Canvas)
	m.viewCursor[0] = *tea.NewCursor(0, 0)
	m.viewCursor[1] = m.viewCursor[0]
	for i := range geo.Nodes {
		if !geo.Nodes[i].Empty() {
			m.cursor = geo.Nodes[i].LabelPoint
			break
		}
	}
	for hit := range geo.DrawOrder() {
		switch hit.Kind {
		case layout.HitNode:
			m.nodeStyle, _ = geo.NodeStyle(hit.ID)
		case layout.HitEdge:
			m.edgeStyle, _ = geo.EdgeStyle(hit.ID)
		case layout.HitPort:
		}
	}
	if err := m.rebuild(); err != nil {
		return nil, err
	}
	m.refreshHits()
	m.syncWorkspace()
	return m, nil
}

// Run starts an interactive terminal editor for geo and saves it to path.
func Run(geo *layout.Layout, path string, options ...Option) error {
	model, err := newModel(geo, path, options...)
	if err != nil {
		return err
	}
	defer model.interruptInteraction()
	_, err = tea.NewProgram(model).Run()
	return err
}

func (*Model) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { return tea.RequestBackgroundColor() })
}

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	syncWorkspace := m.workspaceNeedsSync()
	defer func() {
		m.syncWorkspaceAfterUpdate(syncWorkspace)
	}()
	if command, handled := m.updatePresentation(message); handled {
		return m, command
	}
	switch message := message.(type) {
	case tea.BackgroundColorMsg:
		syncWorkspace = true
		preferences := m.preferenceValue()
		m.applyTheme(themeForTints(
			message.IsDark(),
			preferences.DarkTint,
			preferences.LightTint,
		))
	case tea.KeyboardEnhancementsMsg:
		syncWorkspace = true
		m.bindings.SetSuperAvailable(message.SupportsKeyDisambiguation())
	case tea.ClipboardMsg:
		return m, m.updateClipboard(message)
	case tea.WindowSizeMsg:
		syncWorkspace = true
		m.width = max(message.Width, 0)
		m.height = max(message.Height, 0)
		m.workspace.SetTerminal(chrome.Size{Width: m.width, Height: m.height})
		m.nav.SetWidth(m.width)
		m.ensureCursorVisible()
		return m, m.retargetSidebar()
	case tea.KeyPressMsg:
		return m, m.updateKey(message)
	case tea.PasteMsg:
		return m, m.updatePaste(message)
	case tea.MouseClickMsg:
		m.clipboard.CancelPending()
		return m, m.updateSurfaceMouseClick(message)
	case tea.MouseReleaseMsg:
		m.clipboard.CancelPending()
		m.updateSurfaceMouseRelease(message)
	case tea.MouseMotionMsg:
		return m, m.updateSurfaceMouseMotion(message)
	case tea.MouseWheelMsg:
		m.clipboard.CancelPending()
		return m, m.updateMouseWheelMessage(message)
	case tea.BlurMsg:
		m.interruptInteraction()
	case chrome.FormActivateMsg:
		return m, m.updateDialog(message)
	case chrome.FormSubmitMsg:
		return m, m.updateDialog(message)
	case chrome.FormFlashExpiredMsg:
		return m, m.updateDialog(message)
	case preferencesview.UpdateMsg:
		return m, m.updateDialog(message)
	case clipboardview.UpdateMsg:
		if m.dialogs.ActiveID() == surfaceExport {
			return m, m.updateDialog(message)
		}
		return m, m.updateClipboard(message)
	case noticeExpiredMsg:
		if m.dialogs.ActiveID() == surfaceNotice &&
			m.dialogs.notice.Generation() == message.id {
			return m, m.dismissDialog()
		}
	case sidebarMotionMsg:
		syncWorkspace = true
		return m, m.updateSidebarMotion(message)
	}
	return m, nil
}

func (m *Model) applyTheme(theme Theme) {
	m.theme = theme
	m.nav.SetStyles(theme.Nav)
	m.dialogs.SetStyles(theme)
	m.canvas.SetStyles(theme.Canvas)
	m.sidebar.setStyles(theme.sidebarStyles())
	clear(m.styledRuns)
}

func (m *Model) syncWorkspaceAfterUpdate(sync bool) {
	if sync || m.workspaceNeedsSync() {
		m.syncWorkspace()
	}
}

func (m *Model) workspaceNeedsSync() bool {
	return m.helpInspector.visible ||
		m.dialogs.ActiveID() != surfaceNone ||
		m.sidebar.open ||
		m.workspace.SurfaceMoving(surfaceSidebar) ||
		m.workspace.SurfacePosition(surfaceSidebar) != 0
}

func (m *Model) updatePresentation(message tea.Msg) (tea.Cmd, bool) {
	switch message := message.(type) {
	case nav.ActivateMsg:
		m.nav, _ = m.nav.Update(message)
		switch message.Tool {
		case nav.Cursor:
			m.cancelMode()
		case nav.Rectangle:
			m.activateTool(modeRectangle)
		case nav.Line:
			m.activateTool(modeConnect)
		}
		return nil, true
	case modalview.CloseMsg:
		return m.dismissDialog(), true
	case clipboardview.OpenExportMsg:
		m.dialogs.OpenExport()
		m.status = ""
		return nil, true
	case clipboardview.CloseExportMsg:
		if m.dialogs.ActiveID() == surfaceExport {
			m.dialogs.CloseWithoutMessage()
		}
		return nil, true
	case clipboardview.CopiedMsg:
		return m.showNotice("Copied to clipboard", surfaceNone), true
	case clipboardview.ErrorMsg:
		m.setError("copy selection: " + message.Err.Error())
		return nil, true
	default:
		return nil, false
	}
}

func (m *Model) updatePaste(message tea.PasteMsg) tea.Cmd {
	if m.dialogs.ActiveID() != surfaceNone {
		return m.updateDialog(message)
	}
	if m.interaction.session.kind == sessionLabelEdit {
		m.insertLabelText(message.Content)
	}
	return nil
}

func (m *Model) updateMouseWheelMessage(message tea.MouseWheelMsg) tea.Cmd {
	return m.updateSurfaceMouseWheel(message)
}

func (m *Model) updateKey(message tea.KeyPressMsg) tea.Cmd {
	copyKey := m.bindings.MatchesKey(message, commandCopy)
	if m.dialogs.DismissAnyKey() {
		returnTo := m.dialogs.notice.ReturnTo()
		command := m.dismissDialog()
		if !copyKey || returnTo != surfaceNone {
			return command
		}
		if command != nil {
			return tea.Sequence(command, m.copySelection())
		}
	}
	if !copyKey && !isModifierKey(message) {
		m.clipboard.CancelPending()
	}
	m.interaction.click.valid = false
	if command, ok := m.bindings.ResolveKey(
		message,
		m.activeBindingScopes(),
		m.textEntryActive(),
	); ok {
		return m.updateSemanticCommand(command)
	}
	switch {
	case m.dialogs.ActiveID() != surfaceNone:
		return m.updateDialog(message)
	case m.sidebar.focused:
	case m.interaction.session.kind == sessionLabelEdit:
		m.updateLabel(message)
	}
	return nil
}

type noticeExpiredMsg struct {
	id uint64
}

func (m *Model) showNotice(text string, returnTo chrome.SurfaceID) tea.Cmd {
	id := m.dialogs.OpenNotice(text, returnTo)
	return tea.Tick(noticeDuration, func(time.Time) tea.Msg {
		return noticeExpiredMsg{id: id}
	})
}

func (m *Model) setError(text string) {
	m.status = text
	m.statusError = text
}

func (m *Model) activateTool(next mode) {
	if m.interaction.mode() == next {
		return
	}
	m.cancelMode()
	switch next {
	case modeRectangle:
		m.beginRectangle()
	case modeConnect:
		m.beginConnection()
	default:
		return
	}
}

func (m *Model) reorderLayer(all, backward bool) {
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	hit, ok := m.selectedLayer()
	if !ok {
		hit, ok = m.activeHit()
	}
	if !ok || hit.Kind == layout.HitPort {
		m.setError("select a node or edge to reorder")
		return
	}
	var err error
	switch {
	case all && backward:
		err = m.geo.SendToBack(hit)
	case all:
		err = m.geo.BringToFront(hit)
	case backward:
		err = m.geo.SendBackward(hit)
	default:
		err = m.geo.BringForward(hit)
	}
	if err != nil {
		m.setError(err.Error())
		return
	}
	if err := m.render(); err != nil {
		m.setError(err.Error())
		return
	}
	m.selectOnly(hit)
	m.target = hit
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) selectedLayer() (layout.Hit, bool) {
	var selected layout.Hit
	ok := false
	for hit := range m.geo.DrawOrder() {
		if m.geo.Selection().Contains(hit) {
			selected, ok = hit, true
		}
	}
	return selected, ok
}

func (m *Model) move(dx, dy int) {
	if m.interaction.session.kind == sessionKeyboardMove {
		m.moveNode(dx, dy)
		return
	}
	if m.interaction.idle() {
		nodes, edges := m.selectedCounts()
		if nodes > 0 && nodes+edges > 1 {
			m.shiftSelection(dx, dy)
			return
		}
		if hit, ok := m.focusedNode(); ok {
			m.shiftFocusedNode(hit, dx, dy)
			return
		}
	}
	point, ok := movePoint(m.cursor, dx, dy)
	if !ok {
		return
	}
	m.cursor = point
	m.refreshHits()
	if m.interaction.mode() == modeConnect {
		m.refreshConnectionPreview()
	}
	m.ensureCursorVisible()
	m.status = ""
}

func (m *Model) moveNode(dx, dy int) {
	if m.target.Kind != layout.HitNode || !m.geo.NodeExists(m.target.ID) {
		m.interaction.session = interactionSession{}
		m.setError("selected node no longer exists")
		return
	}
	cursor, ok := movePoint(m.cursor, dx, dy)
	if !ok {
		return
	}
	if _, err := m.moveSelectedNodes(int64(dx), int64(dy), cursor); err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) dragNode(nodeID uint32, cursor layout.Point) {
	if !m.geo.Selection().Contains(
		layout.Hit{ID: nodeID, Kind: layout.HitNode},
	) {
		m.selectOnly(layout.Hit{ID: nodeID, Kind: layout.HitNode})
	}
	previous := m.geo.Nodes[nodeID].Rect.Min
	offset := m.interaction.gesture.offset
	dx := int64(cursor.X) - int64(offset.X) - int64(previous.X)
	dy := int64(cursor.Y) - int64(offset.Y) - int64(previous.Y)
	rebase, err := m.rebaseSelectionMove(dx, dy, cursor)
	if err != nil {
		m.setError(err.Error())
		return
	}
	cursor = cursor.Add(rebase.X, rebase.Y)
	if _, err := m.moveSelectedNodes(dx, dy, cursor); err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) rebaseSelectionMove(
	dx, dy int64,
	cursor layout.Point,
) (layout.Point, error) {
	var shift layout.Point
	include := func(point layout.Point) {
		shift.X = max(shift.X, rebaseCoordinate(point.X, dx))
		shift.Y = max(shift.Y, rebaseCoordinate(point.Y, dy))
	}
	for nodeID := range m.geo.Selection().Nodes() {
		include(m.geo.Nodes[nodeID].Rect.Min)
	}
	for edgeID, edge := range m.geo.Edges {
		id := uint32(edgeID)
		if !m.geo.EdgeExists(id) {
			continue
		}
		nodeA, nodeB, err := m.geo.EdgeNodes(id)
		if err != nil ||
			!m.geo.Selection().Contains(layout.Hit{ID: nodeA, Kind: layout.HitNode}) ||
			!m.geo.Selection().Contains(layout.Hit{ID: nodeB, Kind: layout.HitNode}) {
			continue
		}
		for _, point := range edge.Points {
			include(point)
		}
	}
	if shift == (layout.Point{}) {
		return shift, nil
	}
	if cursor.X > math.MaxUint32-shift.X ||
		cursor.Y > math.MaxUint32-shift.Y ||
		m.viewport.X > math.MaxUint32-shift.X ||
		m.viewport.Y > math.MaxUint32-shift.Y {
		return layout.Point{}, errors.New("viewport outside coordinate space")
	}
	if err := m.geo.Translate(shift.X, shift.Y); err != nil {
		return layout.Point{}, err
	}
	m.viewport = m.viewport.Add(shift.X, shift.Y)
	return shift, nil
}

func rebaseCoordinate(value uint32, delta int64) uint32 {
	if delta >= 0 || -delta <= int64(value) {
		return 0
	}
	return uint32(-delta - int64(value))
}

func (m *Model) moveSelectedNodes(
	dx, dy int64,
	cursor layout.Point,
) (bool, error) {
	if missing := len(m.geo.Nodes) - len(m.moveOrigins); missing > 0 {
		m.moveOrigins = slices.Grow(m.moveOrigins, missing)[:len(m.geo.Nodes)]
	}
	for nodeID := range m.geo.Selection().Nodes() {
		origin := m.geo.Nodes[nodeID].Rect.Min
		_, ok := moveCoordinate64(origin.X, dx)
		if !ok {
			return false, nil
		}
		_, ok = moveCoordinate64(origin.Y, dy)
		if !ok {
			return false, nil
		}
		m.moveOrigins[nodeID] = origin
	}
	if err := m.geo.MoveSelection(dx, dy); err != nil {
		return false, errors.Join(err, m.restoreMovedNodes())
	}
	if m.interaction.movingRigidly() {
		if err := m.render(); err != nil {
			return false, errors.Join(err, m.restoreMovedNodes())
		}
		m.interaction.render.moveHighlight = appendSelectionHighlight(
			m.interaction.render.moveHighlight,
			m.geo,
			m.canvas.Frame(canvasview.BaseFrame),
		)
	} else if err := m.rebuildSelection(); err != nil {
		return false, errors.Join(err, m.restoreMovedNodes())
	}
	m.cursor = cursor
	m.refreshHits()
	m.selectTarget()
	m.ensureCursorVisible()
	m.status = ""
	return true, nil
}

func (m *Model) restoreMovedNodes() error {
	var restoreErr error
	for nodeID := range m.geo.Selection().Nodes() {
		restoreErr = errors.Join(
			restoreErr,
			m.geo.PlaceNode(nodeID, m.moveOrigins[nodeID]),
		)
	}
	return errors.Join(restoreErr, m.rebuild())
}

func (m *Model) beginMove() {
	if m.interaction.session.kind == sessionKeyboardMove {
		m.finishMove()
		return
	}
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	if !m.hasSelectedNodes() {
		hit, ok := m.activeHit()
		if !ok || hit.Kind != layout.HitNode {
			m.setError("select a node to move")
			return
		}
		m.selectOnly(hit)
	}
	target, ok := m.firstSelectedNode()
	if !ok {
		m.setError("select a node to move")
		return
	}
	m.target = target
	m.beginTransaction(transactionKeyboardMove)
	m.interaction.session = interactionSession{
		kind:  sessionKeyboardMove,
		rigid: m.geo.SelectionMovesRigidly(),
	}
	m.status = ""
}

func (m *Model) beginLabelEdit() {
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	hit, ok := m.activeHit()
	if !ok || hit.Kind != layout.HitNode {
		m.setError("select a node to edit")
		return
	}
	m.beginTransaction(transactionLabelEdit)
	m.startLabelEdit(hit)
}

func (m *Model) newNode() {
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	m.beginTransaction(transactionLabelEdit)
	nodeID, err := m.geo.NewNodeAt("", m.cursor)
	if err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.geo.SetNodeStyle(nodeID, m.nodeStyle); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	if err := m.rebuild(); err != nil {
		m.setError(errors.Join(err, m.cancelTransaction()).Error())
		return
	}
	m.startLabelEdit(layout.Hit{ID: nodeID, Kind: layout.HitNode})
}

func (m *Model) beginRectangle() {
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	m.clearConnection()
	m.interaction.tool = toolRectangle
	m.status = "drag to create a rectangle"
}

func (m *Model) beginConnection() {
	if !m.interaction.idle() {
		m.setError(finishOperation)
		return
	}
	m.clearConnection()
	m.interaction.tool = toolConnect
	hit, ok := m.activeHit()
	if !ok {
		m.status = dragFromSource
		return
	}
	if err := m.startConnection(hit); err != nil {
		m.setError(err.Error())
		return
	}
	m.status = ""
}

func (m *Model) startConnection(hit layout.Hit) error {
	var connection connectionSession
	switch hit.Kind {
	case layout.HitPort:
		connection.source = hit.ID
	case layout.HitEdge:
		portA, portB, err := m.geo.EdgePorts(hit.ID)
		if err != nil {
			return err
		}
		moving, stationary := portA, portB
		if pointDistance(m.cursor, m.geo.Ports[portB].Anchor) <
			pointDistance(m.cursor, m.geo.Ports[portA].Anchor) {
			moving, stationary = portB, portA
		}
		connection.edge = hit.ID
		connection.oldPort = moving
		connection.source = stationary
		connection.reconnect = true
	default:
		return errors.New(dragFromSource)
	}
	m.interaction.session = interactionSession{
		kind:       sessionConnection,
		connection: connection,
	}
	m.refreshConnectionPreview()
	return nil
}

func (m *Model) completeConnection() {
	if m.interaction.session.kind != sessionConnection {
		m.status = dragFromSource
		return
	}
	hit, ok := m.activeHit()
	if !ok || hit.Kind != layout.HitPort {
		m.status = "select a destination port"
		return
	}
	m.completeConnectionTo(hit.ID)
}

func (m *Model) completeConnectionTo(destination uint32) {
	connection := m.interaction.session.connection
	m.beginTransaction(transactionConnection)
	edgeID := connection.edge
	var err error
	if connection.reconnect {
		err = m.geo.ReconnectEdge(edgeID, connection.oldPort, destination)
	} else {
		edgeID, err = m.geo.ConnectPorts(connection.source, destination)
	}
	if err != nil {
		_ = m.cancelTransaction()
		m.setError(err.Error())
		return
	}
	if !connection.reconnect {
		if err := m.geo.SetEdgeStyle(edgeID, m.edgeStyle); err != nil {
			_ = m.cancelTransaction()
			m.setError(err.Error())
			return
		}
	}
	if err := m.rebuild(); err != nil {
		m.setError(errors.Join(
			err,
			m.cancelTransaction(),
			m.render(),
		).Error())
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	m.interaction.tool = toolNavigate
	m.target = layout.Hit{ID: edgeID, Kind: layout.HitEdge}
	m.selectOnly(m.target)
	m.clearConnection()
	m.refreshHits()
	m.selectTarget()
	m.status = ""
}

func (m *Model) deleteActive() {
	session := m.interaction.session.kind
	if !m.interaction.idle() && session != sessionKeyboardMove {
		m.setError(finishOperation)
		return
	}
	if session == sessionKeyboardMove {
		if err := m.commitTransaction(); err != nil {
			m.setError(err.Error())
			return
		}
		m.interaction.session = interactionSession{}
	}
	if !m.hasSelection() {
		hit, ok := m.activeHit()
		if !ok {
			m.setError("nothing selected")
			return
		}
		if hit.Kind == layout.HitPort {
			m.setError("ports cannot be deleted independently")
			return
		}
		m.selectOnly(hit)
	}
	m.beginTransaction(transactionImmediate)
	var err error
	for nodeID := range m.geo.Selection().Nodes() {
		err = m.geo.DeleteNode(nodeID)
		if err != nil {
			break
		}
	}
	if err == nil {
		for edgeID := range m.geo.Selection().Edges() {
			err = m.geo.DeleteEdge(edgeID)
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		_ = m.cancelTransaction()
		m.setError(err.Error())
		return
	}
	if err := m.rebuild(); err != nil {
		m.setError(errors.Join(
			err,
			m.cancelTransaction(),
			m.render(),
		).Error())
		return
	}
	if err := m.commitTransaction(); err != nil {
		m.setError(err.Error())
		return
	}
	m.interaction.tool = toolNavigate
	m.interaction.session = interactionSession{}
	m.target = layout.Hit{}
	m.interaction.resetGesture()
	m.clearSelection()
	m.refreshHits()
	m.status = ""
}

func (m *Model) cancelMode() {
	if m.interaction.session.kind == sessionKeyboardMove {
		m.finishMove()
		return
	}
	if m.interaction.gesture.kind == gestureRectangle {
		m.interaction.resetGesture()
		m.interaction.tool = toolNavigate
		err := errors.Join(
			m.cancelTransaction(),
			m.render(),
		)
		if err != nil {
			m.setError(err.Error())
		} else {
			m.status = ""
		}
		m.refreshHits()
		return
	}
	m.interaction.tool = toolNavigate
	if m.interaction.session.kind == sessionLabelEdit {
		m.commitLabelEdit()
	} else {
		m.interaction.session = interactionSession{}
	}
	m.clearConnection()
	m.cancelDuplicateDrag()
	m.interaction.resetGesture()
	m.status = ""
}

func (m *Model) beginTransaction(owner transactionOwner) {
	if m.history == nil {
		return
	}
	m.interaction.transaction = interactionTransaction{
		value: m.history.Begin(),
		owner: owner,
	}
}

func (m *Model) commitTransaction() error {
	if !m.interaction.transaction.open() {
		return nil
	}
	transaction := m.interaction.transaction.value
	m.interaction.transaction = interactionTransaction{}
	return transaction.Commit()
}

func (m *Model) cancelTransaction() error {
	if !m.interaction.transaction.open() {
		return nil
	}
	transaction := m.interaction.transaction.value
	m.interaction.transaction = interactionTransaction{}
	return transaction.Cancel()
}

func (m *Model) finishMove() {
	var routeErr error
	if m.interaction.movingRigidly() {
		routeErr = m.rebuildSelection()
	}
	err := errors.Join(routeErr, m.commitTransaction())
	m.interaction.session = interactionSession{}
	m.interaction.resetGesture()
	m.interaction.render.moveHighlight = m.interaction.render.moveHighlight[:0]
	if err != nil {
		m.setError(err.Error())
	} else {
		m.status = ""
	}
}

func (m *Model) interruptInteraction() {
	m.clipboard.CancelPending()
	if m.preferenceEdit {
		m.cancelPreferences()
	}
	if m.history != nil {
		m.history.Interrupt()
	}
	m.interaction.transaction = interactionTransaction{}
	if m.interaction.session.kind == sessionLabelEdit {
		m.finishLabelEdit()
	} else {
		m.interaction.session = interactionSession{}
	}
	m.interaction.tool = toolNavigate
	m.clearConnection()
	m.cancelDuplicateDrag()
	m.interaction.resetGesture()
	m.interaction.render.moveHighlight = m.interaction.render.moveHighlight[:0]
}

func (m *Model) undo() {
	if m.history == nil {
		m.setError("undo history is unavailable")
		return
	}
	changed, err := m.history.Undo()
	m.afterHistoryChange(changed, err, "nothing to undo")
}

func (m *Model) redo() {
	if m.history == nil {
		m.setError("undo history is unavailable")
		return
	}
	changed, err := m.history.Redo()
	m.afterHistoryChange(changed, err, "nothing to redo")
}

func (m *Model) afterHistoryChange(changed bool, err error, unchanged string) {
	m.interaction.tool = toolNavigate
	m.interaction.session = interactionSession{}
	m.interaction.resetGesture()
	m.interaction.transaction = interactionTransaction{}
	m.clearConnection()
	m.clearSelection()
	if err != nil {
		m.setError(err.Error())
		return
	}
	if !changed {
		m.status = unchanged
		return
	}
	if err := m.render(); err != nil {
		m.setError(err.Error())
		return
	}
	m.refreshHits()
	m.status = ""
}

func (m *Model) clearConnection() {
	if m.interaction.session.kind == sessionConnection {
		m.interaction.session = interactionSession{}
	}
	if m.interaction.gesture.kind == gestureConnection ||
		m.interaction.gesture.kind == gestureConnectionPending {
		m.interaction.resetGesture()
	}
	m.interaction.render.connectionPreview = m.interaction.render.connectionPreview[:0]
	m.interaction.render.connectionRaster = m.interaction.render.connectionRaster[:0]
}

func (m *Model) rebuild() error {
	if err := m.geo.Build(); err != nil {
		return fmt.Errorf("build layout: %w", err)
	}
	return m.render()
}

func (m *Model) rebuildSelection() error {
	if err := m.geo.BuildSelection(); err != nil {
		return fmt.Errorf("build selection: %w", err)
	}
	return m.render()
}

func (m *Model) render() error {
	if !hasGeometry(m.geo) {
		m.canvas.Clear(canvasview.BaseFrame)
		return nil
	}
	if err := m.canvas.Render(canvasview.BaseFrame, m.geo); err != nil {
		return fmt.Errorf("render layout: %w", err)
	}
	return nil
}

func (m *Model) renderConnectionBase() error {
	err := m.canvas.RenderWithoutEdge(
		canvasview.ConnectionFrame,
		m.geo,
		m.interaction.session.connection.edge,
	)
	if err != nil {
		return fmt.Errorf("render connection base: %w", err)
	}
	return nil
}

func (m *Model) refreshHits() {
	m.hits = slices.AppendSeq(m.hits[:0], m.geo.Hits(m.cursor))
	m.active = 0
}

func (m *Model) cycleHit(delta int) {
	if len(m.hits) == 0 {
		return
	}
	m.active = (m.active + delta + len(m.hits)) % len(m.hits)
	m.clearSelection()
	m.status = ""
}

func (m *Model) activeHit() (layout.Hit, bool) {
	if m.active < 0 || m.active >= len(m.hits) {
		return layout.Hit{}, false
	}
	return m.hits[m.active], true
}

func (m *Model) selectTarget() {
	for i, hit := range m.hits {
		if hit == m.target {
			m.active = i
			return
		}
	}
}

func (m *Model) ensureCursorVisible() {
	host := m.workspace.Geometry().Canvas
	m.viewport.X = visibleOrigin(m.viewport.X, m.cursor.X, host.Width)
	m.viewport.Y = visibleOrigin(m.viewport.Y, m.cursor.Y, host.Height)
}

func hasGeometry(geo *layout.Layout) bool {
	for i := range geo.Nodes {
		if !geo.Nodes[i].Empty() {
			return true
		}
	}
	for i := range geo.Edges {
		if !geo.Edges[i].Empty() {
			return true
		}
	}
	return false
}

func movePoint(point layout.Point, dx, dy int) (layout.Point, bool) {
	x, ok := moveCoordinate(point.X, dx)
	if !ok {
		return point, false
	}
	y, ok := moveCoordinate(point.Y, dy)
	if !ok {
		return point, false
	}
	return layout.NewPoint(x, y), true
}

func moveCoordinate(value uint32, delta int) (uint32, bool) {
	return moveCoordinate64(value, int64(delta))
}

func moveCoordinate64(value uint32, delta int64) (uint32, bool) {
	switch {
	case delta < 0 && int64(value)+delta < 0:
		return value, false
	case delta > 0 && uint64(delta) > uint64(math.MaxUint32-value):
		return value, false
	default:
		return uint32(int64(value) + delta), true
	}
}

func visibleOrigin(origin, cursor uint32, size int) uint32 {
	if size <= 0 || cursor < origin {
		return cursor
	}
	size64 := uint64(size)
	if size64 > uint64(math.MaxUint32)+1 {
		return 0
	}
	if uint64(cursor) < uint64(origin)+size64 {
		return origin
	}
	return uint32(uint64(cursor) - size64 + 1)
}

func pointDistance(a, b layout.Point) uint64 {
	return uint64(max(a.X, b.X)-min(a.X, b.X)) +
		uint64(max(a.Y, b.Y)-min(a.Y, b.Y))
}

var _ tea.Model = (*Model)(nil)
