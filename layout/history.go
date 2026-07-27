package layout

import (
	"errors"
	"fmt"
	"slices"

	"github.com/coxley/dg/ir"
)

const defaultHistoryLimit = 256

var (
	ErrHistoryAttached   = errors.New("history already attached")
	ErrTransactionClosed = errors.New("history transaction closed")
)

type historyKind uint8

const (
	historyCreateNode historyKind = iota + 1
	historyDeleteNode
	historySetLabel
	historyPlaceNode
	historyCreateEdge
	historyDeleteEdge
	historyReconnectEdge
	historySetNodeSize
	historySetLayer
)

type historyPort struct {
	ID   uint32
	Port ir.Port
}

type historyEdge struct {
	ID   uint32
	Edge ir.Edge
}

type historyLayer struct {
	Hit   Hit
	Index uint32
}

type historyNode struct {
	ID     uint32
	Label  string
	Origin Point
	Size   Size
	Ports  []historyPort
	Edges  []historyEdge
	Layers []historyLayer
}

type historyChange struct {
	kind historyKind
	id   uint32

	beforePoint Point
	afterPoint  Point
	beforeSize  Size
	afterSize   Size
	beforeLabel string
	afterLabel  string
	beforeEdge  ir.Edge
	afterEdge   ir.Edge
	layerHit    Hit
	beforeLayer uint32
	afterLayer  uint32
	node        historyNode
}

type historyEntry struct {
	changes []historyChange
}

// History records successful Layout mutations as bounded undo transactions.
type History struct {
	layout *Layout
	limit  int

	entries []historyEntry
	cursor  int
	active  historyEntry

	generation uint64
	activeGen  uint64
	replaying  bool

	historyCacheFields
}

// HistoryOption configures History.
type HistoryOption func(*History)

// NewHistory returns an unattached history retaining 256 interactions.
func NewHistory(options ...HistoryOption) (*History, error) {
	h := &History{
		limit: defaultHistoryLimit,
		historyCacheFields: historyCacheFields{
			cacheDelay: defaultCacheDelay,
		},
	}
	for _, option := range options {
		option(h)
	}
	if h.limit < 1 {
		return nil, errors.New("history limit must be positive")
	}
	if h.cacheDelay < 0 {
		return nil, errors.New("history cache delay must not be negative")
	}
	if h.customCacheDir && h.customStore {
		return nil, errors.New("history cache directory and store are mutually exclusive")
	}
	if h.customCacheDir && h.cacheDir == "" {
		return nil, errors.New("history cache directory must not be empty")
	}
	if h.customStore && h.cacheStore == nil {
		return nil, errors.New("history cache store must not be nil")
	}
	return h, nil
}

// WithHistoryLimit limits the number of committed interactions retained.
func WithHistoryLimit(limit int) HistoryOption {
	return func(h *History) {
		h.limit = limit
	}
}

// Transaction identifies one open interaction.
type Transaction struct {
	history    *History
	generation uint64
}

// Begin starts a transaction and commits any interrupted transaction.
func (h *History) Begin() Transaction {
	h.Interrupt()
	h.generation++
	h.activeGen = h.generation
	h.active = historyEntry{}
	return Transaction{history: h, generation: h.activeGen}
}

// Commit closes t and retains its changes as one undo interaction.
func (t Transaction) Commit() error {
	if !t.active() {
		return ErrTransactionClosed
	}
	t.history.commitActive()
	return nil
}

// Cancel restores the state from before t and records no interaction.
func (t Transaction) Cancel() error {
	if !t.active() {
		return ErrTransactionClosed
	}
	h := t.history
	if len(h.active.changes) == 0 {
		h.closeActive()
		return nil
	}
	entry := h.active
	if err := h.apply(entry, false); err != nil {
		return err
	}
	h.closeActive()
	return nil
}

// Close commits t when it remains active and otherwise does nothing.
func (t Transaction) Close() {
	if t.active() {
		t.history.commitActive()
	}
}

func (t Transaction) active() bool {
	return t.history != nil &&
		t.history.activeGen != 0 &&
		t.history.activeGen == t.generation
}

// Interrupt commits the latest applied state of an open transaction.
func (h *History) Interrupt() {
	if h != nil && h.activeGen != 0 {
		h.commitActive()
	}
}

// Undo commits an open transaction and restores the previous interaction.
func (h *History) Undo() (bool, error) {
	h.Interrupt()
	if h == nil || h.cursor == 0 {
		return false, nil
	}
	if err := h.apply(h.entries[h.cursor-1], false); err != nil {
		return false, err
	}
	h.cursor--
	return true, nil
}

// Redo reapplies the next reverted interaction.
func (h *History) Redo() (bool, error) {
	h.Interrupt()
	if h == nil || h.cursor == len(h.entries) {
		return false, nil
	}
	if err := h.apply(h.entries[h.cursor], true); err != nil {
		return false, err
	}
	h.cursor++
	return true, nil
}

// CanUndo reports whether Undo can change the layout.
func (h *History) CanUndo() bool {
	return h != nil && (h.activeGen != 0 && len(h.active.changes) != 0 || h.cursor != 0)
}

// CanRedo reports whether Redo can change the layout.
func (h *History) CanRedo() bool {
	return h != nil && h.activeGen == 0 && h.cursor < len(h.entries)
}

// Clear discards every recorded interaction without changing the layout.
func (h *History) Clear() {
	if h == nil {
		return
	}
	h.cancelPendingCache()
	clear(h.entries)
	h.entries = h.entries[:0]
	h.cursor = 0
	h.savedCursor = 0
	h.closeActive()
	if h.layout != nil && h.cachePath != "" {
		h.savedState = h.layout.historyState()
		h.cacheBranchValid = true
		h.scheduleCache()
		return
	}
	h.savedState = layoutHistoryState{}
	h.cacheBranchValid = false
}

func (h *History) attach(l *Layout) error {
	if h == nil {
		return nil
	}
	if h.layout != nil && h.layout != l {
		return ErrHistoryAttached
	}
	h.layout = l
	return nil
}

func (h *History) record(change historyChange) {
	if h == nil || h.replaying {
		return
	}
	if h.activeGen == 0 {
		h.discardRedo()
		h.entries = append(h.entries, historyEntry{
			changes: []historyChange{change},
		})
		h.cursor = len(h.entries)
		h.enforceLimit()
		h.scheduleCache()
		return
	}
	h.discardRedo()
	if h.coalesce(change) {
		return
	}
	h.active.changes = append(h.active.changes, change)
}

func (h *History) coalesce(change historyChange) bool {
	if change.kind != historySetLabel &&
		change.kind != historyPlaceNode &&
		change.kind != historySetNodeSize &&
		change.kind != historySetLayer {
		return false
	}
	if change.kind == historySetLayer {
		return h.coalesceLayer(change)
	}
	for i := len(h.active.changes) - 1; i >= 0; i-- {
		previous := &h.active.changes[i]
		if previous.id == change.id &&
			(previous.kind == historyCreateNode || previous.kind == historyDeleteNode) {
			return false
		}
		if previous.kind != change.kind || previous.id != change.id {
			continue
		}
		if coalesceChange(previous, change) {
			h.active.changes = slices.Delete(h.active.changes, i, i+1)
		}
		return true
	}
	return false
}

func (h *History) coalesceLayer(change historyChange) bool {
	for i := len(h.active.changes) - 1; i >= 0; i-- {
		previous := &h.active.changes[i]
		if previous.kind != historySetLayer {
			if layerCoalescingBarrier(previous.kind) {
				return false
			}
			continue
		}
		if previous.layerHit != change.layerHit {
			return false
		}
		if coalesceChange(previous, change) {
			h.active.changes = slices.Delete(h.active.changes, i, i+1)
		}
		return true
	}
	return false
}

func layerCoalescingBarrier(kind historyKind) bool {
	return kind == historyCreateNode ||
		kind == historyDeleteNode ||
		kind == historyCreateEdge ||
		kind == historyDeleteEdge
}

func coalesceChange(previous *historyChange, change historyChange) bool {
	switch change.kind {
	case historySetLabel:
		previous.afterLabel = change.afterLabel
		return previous.beforeLabel == previous.afterLabel
	case historyPlaceNode:
		previous.afterPoint = change.afterPoint
		return previous.beforePoint == previous.afterPoint
	case historySetNodeSize:
		previous.afterSize = change.afterSize
		return previous.beforeSize == previous.afterSize
	case historySetLayer:
		previous.afterLayer = change.afterLayer
		return previous.beforeLayer == previous.afterLayer
	default:
		return false
	}
}

func (h *History) commitActive() {
	if len(h.active.changes) != 0 {
		h.entries = append(h.entries, h.active)
		h.cursor = len(h.entries)
		h.enforceLimit()
		h.scheduleCache()
	}
	h.closeActive()
}

func (h *History) closeActive() {
	h.active = historyEntry{}
	h.activeGen = 0
}

func (h *History) discardRedo() {
	if h.cursor == len(h.entries) {
		return
	}
	if h.cachePath != "" && h.cursor < h.savedCursor {
		h.cacheBranchValid = false
	}
	clear(h.entries[h.cursor:])
	h.entries = h.entries[:h.cursor]
}

func (h *History) enforceLimit() {
	excess := len(h.entries) - h.limit
	if excess <= 0 {
		return
	}
	if h.cachePath != "" && excess > h.savedCursor {
		h.cacheBranchValid = false
	}
	copy(h.entries, h.entries[excess:])
	clear(h.entries[len(h.entries)-excess:])
	h.entries = h.entries[:len(h.entries)-excess]
	h.cursor = max(0, h.cursor-excess)
	h.savedCursor = max(0, h.savedCursor-excess)
}

func (h *History) apply(entry historyEntry, forward bool) error {
	if h.layout == nil {
		return errors.New("history is not attached")
	}
	rollback := h.layout.historyState()
	h.replaying = true
	defer func() {
		h.replaying = false
	}()

	var err error
	if forward {
		for i := range entry.changes {
			if err = h.applyChange(entry.changes[i], true); err != nil {
				break
			}
		}
	} else {
		for i := len(entry.changes) - 1; i >= 0; i-- {
			if err = h.applyChange(entry.changes[i], false); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = h.layout.Build()
	}
	if err == nil {
		return nil
	}
	if restoreErr := h.layout.restoreHistoryState(rollback); restoreErr != nil {
		return errors.Join(err, fmt.Errorf("restore history rollback: %w", restoreErr))
	}
	return err
}

func (h *History) restoreLayoutState(state layoutHistoryState) error {
	h.replaying = true
	defer func() {
		h.replaying = false
	}()
	return h.layout.restoreHistoryState(state)
}

func (h *History) applyChange(change historyChange, forward bool) error {
	switch change.kind {
	case historyCreateNode:
		if forward {
			return h.layout.restoreHistoryNode(change.node)
		}
		return h.layout.DeleteNode(change.id)
	case historyDeleteNode:
		if forward {
			return h.layout.DeleteNode(change.id)
		}
		return h.layout.restoreHistoryNode(change.node)
	case historySetLabel:
		if forward {
			return h.layout.SetNodeLabel(change.id, change.afterLabel)
		}
		return h.layout.SetNodeLabel(change.id, change.beforeLabel)
	case historyPlaceNode:
		if forward {
			return h.layout.PlaceNode(change.id, change.afterPoint)
		}
		return h.layout.PlaceNode(change.id, change.beforePoint)
	case historySetNodeSize:
		size := change.beforeSize
		if forward {
			size = change.afterSize
		}
		return h.layout.setNodeSize(change.id, size)
	case historyCreateEdge:
		if forward {
			return h.layout.restoreHistoryEdge(
				change.id,
				change.afterEdge,
				int(change.afterLayer),
			)
		}
		return h.layout.DeleteEdge(change.id)
	case historyDeleteEdge:
		if forward {
			return h.layout.DeleteEdge(change.id)
		}
		return h.layout.restoreHistoryEdge(
			change.id,
			change.beforeEdge,
			int(change.beforeLayer),
		)
	case historyReconnectEdge:
		edge := change.beforeEdge
		if forward {
			edge = change.afterEdge
		}
		return h.layout.restoreHistoryEdge(change.id, edge, -1)
	case historySetLayer:
		index := change.beforeLayer
		if forward {
			index = change.afterLayer
		}
		return h.layout.setLayerIndex(change.layerHit, int(index))
	default:
		return fmt.Errorf("unknown history change %d", change.kind)
	}
}

type layoutHistoryState struct {
	graph   ir.Graph
	origins []Point
	sizes   []Size
	order   []Hit
	padding Padding
	router  Router
}

func (l *Layout) historyState() layoutHistoryState {
	return layoutHistoryState{
		graph:   l.graph.Clone(),
		origins: slices.Clone(l.origins),
		sizes:   slices.Clone(l.explicitSizes),
		order:   slices.Clone(l.drawOrder),
		padding: l.padding,
		router:  l.router,
	}
}

func (l *Layout) restoreHistoryState(state layoutHistoryState) error {
	l.graph = state.graph.Clone()
	l.padding = state.padding
	l.router = state.router
	l.drawOrder = slices.Clone(state.order)
	if err := l.initializeGeometry(); err != nil {
		return err
	}
	l.explicitSizes = slices.Clone(state.sizes)
	for nodeID := range l.graph.Nodes {
		if !l.graph.NodeExists(uint32(nodeID)) {
			continue
		}
		node, err := l.prepareNode(
			uint32(nodeID),
			l.graph.Nodes[nodeID].Label,
			state.origins[nodeID],
		)
		if err != nil {
			return err
		}
		l.origins[nodeID] = state.origins[nodeID]
		l.Nodes[nodeID] = node
		l.commitNodePorts(uint32(nodeID))
	}
	return l.Build()
}

func (l *Layout) historyNode(nodeID uint32) historyNode {
	source := l.graph.Nodes[nodeID]
	node := historyNode{
		ID:     nodeID,
		Label:  source.Label,
		Origin: l.origins[nodeID],
		Size:   l.explicitSizes[nodeID],
		Ports:  make([]historyPort, 0, len(source.Ports)),
	}
	for _, portID := range source.Ports {
		node.Ports = append(node.Ports, historyPort{
			ID:   portID,
			Port: l.graph.Ports[portID],
		})
	}
	for edgeID := range l.graph.Edges {
		if l.graph.EdgeExists(uint32(edgeID)) &&
			l.graph.EdgeIncidentTo(uint32(edgeID), nodeID) {
			node.Edges = append(node.Edges, historyEdge{
				ID:   uint32(edgeID),
				Edge: l.graph.Edges[edgeID],
			})
		}
	}
	for index, hit := range l.drawOrder {
		if hit == (Hit{ID: nodeID, Kind: HitNode}) {
			node.Layers = append(node.Layers, historyLayer{
				Hit:   hit,
				Index: uint32(index),
			})
			continue
		}
		if hit.Kind == HitEdge &&
			l.graph.EdgeExists(hit.ID) &&
			l.graph.EdgeIncidentTo(hit.ID, nodeID) {
			node.Layers = append(node.Layers, historyLayer{
				Hit:   hit,
				Index: uint32(index),
			})
		}
	}
	return node
}

func (l *Layout) restoreHistoryNode(node historyNode) error {
	l.graph.Nodes = growTo(l.graph.Nodes, int(node.ID)+1)
	l.graph.Ports = growTo(l.graph.Ports, maxHistoryPort(node)+1)
	l.graph.Edges = growTo(l.graph.Edges, maxHistoryEdge(node)+1)

	ports := slices.Grow(l.graph.Nodes[node.ID].Ports[:0], len(node.Ports))
	for _, port := range node.Ports {
		l.graph.Ports[port.ID] = port.Port
		ports = append(ports, port.ID)
	}
	l.graph.Nodes[node.ID] = ir.Node{Label: node.Label, Ports: ports}
	for _, edge := range node.Edges {
		l.graph.Edges[edge.ID] = edge.Edge
	}
	l.graph = l.graph.Clone()

	l.origins = growTo(l.origins, len(l.graph.Nodes))
	l.explicitSizes = growTo(l.explicitSizes, len(l.graph.Nodes))
	l.Nodes = growTo(l.Nodes, len(l.graph.Nodes))
	l.Ports = growTo(l.Ports, len(l.graph.Ports))
	l.portUsable = growTo(l.portUsable, len(l.graph.Ports))
	l.Edges = growTo(l.Edges, len(l.graph.Edges))
	l.explicitSizes[node.ID] = node.Size
	resolved, err := l.prepareNode(node.ID, node.Label, node.Origin)
	if err != nil {
		return err
	}
	l.origins[node.ID] = node.Origin
	l.Nodes[node.ID] = resolved
	l.commitNodePorts(node.ID)
	for _, edge := range node.Edges {
		l.Edges[edge.ID] = Edge{}
	}
	if len(node.Layers) == 0 {
		l.appendLayer(Hit{ID: node.ID, Kind: HitNode})
		for _, edge := range node.Edges {
			l.appendLayer(Hit{ID: edge.ID, Kind: HitEdge})
		}
	} else {
		for _, layer := range node.Layers {
			l.insertLayer(layer.Hit, int(layer.Index))
		}
	}
	return nil
}

func (l *Layout) restoreHistoryEdge(edgeID uint32, edge ir.Edge, layer int) error {
	if !l.graph.PortExists(edge.PortA) || !l.graph.PortExists(edge.PortB) {
		return errors.New("restore edge with deleted port")
	}
	l.graph.Edges = growTo(l.graph.Edges, int(edgeID)+1)
	l.graph.Edges[edgeID] = edge
	l.graph = l.graph.Clone()
	l.Edges = growTo(l.Edges, len(l.graph.Edges))
	l.Edges[edgeID] = Edge{}
	hit := Hit{ID: edgeID, Kind: HitEdge}
	if !l.hasLayer(hit) {
		if layer < 0 {
			l.appendLayer(hit)
		} else {
			l.insertLayer(hit, layer)
		}
	}
	return nil
}

func maxHistoryPort(node historyNode) int {
	maxID := -1
	for _, port := range node.Ports {
		maxID = max(maxID, int(port.ID))
	}
	return maxID
}

func maxHistoryEdge(node historyNode) int {
	maxID := -1
	for _, edge := range node.Edges {
		maxID = max(maxID, int(edge.ID))
	}
	return maxID
}
