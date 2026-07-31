package layout

import (
	"errors"
	"fmt"
	"slices"
)

// ErrChangeCallbackAttached reports an attempt to attach a second callback.
var ErrChangeCallbackAttached = errors.New("layout change callback already attached")

// ReplayDirection selects which side of a reversible change Replay applies.
type ReplayDirection uint8

const (
	// ReplayBackward applies changes in reverse order using their before values.
	ReplayBackward ReplayDirection = iota
	// ReplayForward applies changes in recorded order using their after values.
	ReplayForward
)

// Change contains one opaque reversible layout mutation.
type Change struct {
	value historyChange
}

// Snapshot contains an opaque copy of a layout's semantic state.
type Snapshot struct {
	state layoutHistoryState
}

// ChangeCallback receives successful layout mutations.
type ChangeCallback func(Change)

// SetChangeCallback sets the sole mutation callback. Passing nil detaches it.
func (l *Layout) SetChangeCallback(callback ChangeCallback) error {
	if callback != nil && l.changeCallback != nil {
		return ErrChangeCallbackAttached
	}
	l.changeCallback = callback
	return nil
}

func (l *Layout) recordChange(change historyChange) {
	if !l.recordingChanges() {
		return
	}
	l.changeCallback(Change{value: change})
}

func (l *Layout) recordingChanges() bool {
	return l.changeCallback != nil && !l.replaying
}

// Snapshot returns an independent copy of the layout's semantic state.
func (l *Layout) Snapshot() Snapshot {
	return Snapshot{state: l.historyState()}
}

// Digest returns a stable semantic digest of snapshot.
func (s Snapshot) Digest() (string, error) {
	return semanticHistoryDigest(s.state)
}

// Restore atomically replaces the layout with snapshot.
func (l *Layout) Restore(snapshot Snapshot) error {
	rollback := l.historyState()
	rollbackSelection := l.selection.state()
	wasReplaying := l.replaying
	l.replaying = true
	defer func() {
		l.replaying = wasReplaying
	}()
	if err := l.restoreHistoryState(snapshot.state); err != nil {
		if restoreErr := l.restoreHistoryState(rollback); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore layout rollback: %w", restoreErr))
		}
		l.selection.restore(rollbackSelection)
		return err
	}
	return nil
}

// Replay atomically applies changes in direction and builds the result once.
func (l *Layout) Replay(changes []Change, direction ReplayDirection) error {
	if direction != ReplayBackward && direction != ReplayForward {
		return fmt.Errorf("invalid layout replay direction %d", direction)
	}
	rollback := l.historyState()
	rollbackSelection := l.selection.state()
	wasReplaying := l.replaying
	l.replaying = true
	defer func() {
		l.replaying = wasReplaying
	}()

	var err error
	if direction == ReplayForward {
		for i := range changes {
			if err = l.applyChange(changes[i].value, true); err != nil {
				break
			}
		}
	} else {
		for i := len(changes) - 1; i >= 0; i-- {
			if err = l.applyChange(changes[i].value, false); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = l.graph.Validate()
	}
	if err == nil {
		err = validateDrawOrder(&l.graph, l.drawOrder)
	}
	if err == nil {
		err = l.Build()
	}
	if err == nil {
		return nil
	}
	if restoreErr := l.restoreHistoryState(rollback); restoreErr != nil {
		return errors.Join(err, fmt.Errorf("restore replay rollback: %w", restoreErr))
	}
	l.selection.restore(rollbackSelection)
	return err
}

// CoalesceChanges merges change into an earlier compatible mutation and reports
// whether the result consumed change.
func CoalesceChanges(changes []Change, change Change) ([]Change, bool) {
	next := change.value
	if next.Kind != historySetLabel &&
		next.Kind != historyPlaceNode &&
		next.Kind != historySetNodeSize &&
		next.Kind != historySetLayer &&
		next.Kind != historySetNodeStyle &&
		next.Kind != historySetEdgeStyle &&
		next.Kind != historySetRouter &&
		next.Kind != historySetAttachment &&
		next.Kind != historySetPinnedBends {
		return changes, false
	}
	if next.Kind == historySetLayer {
		return coalesceLayerChanges(changes, next)
	}
	for i := len(changes) - 1; i >= 0; i-- {
		previous := &changes[i].value
		if changeCoalescingBarrier(*previous, next) {
			return changes, false
		}
		if previous.Kind != next.Kind || previous.ID != next.ID {
			continue
		}
		if coalesceChange(previous, next) {
			changes = slices.Delete(changes, i, i+1)
		}
		return changes, true
	}
	return changes, false
}

func changeCoalescingBarrier(previous, next historyChange) bool {
	if previous.ID == next.ID &&
		(previous.Kind == historyCreateNode || previous.Kind == historyDeleteNode) {
		return true
	}
	if next.Kind == historySetAttachment {
		return previous.Kind == historyCreateEdge || previous.Kind == historyDeleteEdge
	}
	return previous.ID == next.ID &&
		(next.Kind == historySetEdgeStyle || next.Kind == historySetPinnedBends) &&
		(previous.Kind == historyCreateEdge || previous.Kind == historyDeleteEdge)
}

func coalesceLayerChanges(changes []Change, change historyChange) ([]Change, bool) {
	for i := len(changes) - 1; i >= 0; i-- {
		previous := &changes[i].value
		if previous.Kind != historySetLayer {
			if layerCoalescingBarrier(previous.Kind) {
				return changes, false
			}
			continue
		}
		if previous.LayerHit != change.LayerHit {
			return changes, false
		}
		if coalesceChange(previous, change) {
			changes = slices.Delete(changes, i, i+1)
		}
		return changes, true
	}
	return changes, false
}

func layerCoalescingBarrier(kind historyKind) bool {
	return kind == historyCreateNode ||
		kind == historyDeleteNode ||
		kind == historyCreateEdge ||
		kind == historyDeleteEdge
}

func coalesceChange(previous *historyChange, change historyChange) bool {
	previous.After = change.After
	switch change.Kind {
	case historySetLabel:
		return previous.Before.Label == previous.After.Label
	case historyPlaceNode:
		return previous.Before.Point == previous.After.Point
	case historySetNodeSize:
		return previous.Before.Size == previous.After.Size
	case historySetLayer:
		return previous.Before.Layer == previous.After.Layer
	case historySetNodeStyle:
		return previous.Before.NodeStyle == previous.After.NodeStyle
	case historySetEdgeStyle:
		return previous.Before.EdgeStyle == previous.After.EdgeStyle
	case historySetRouter:
		return previous.Before.Router == previous.After.Router
	case historySetAttachment:
		return previous.Before.Attached == previous.After.Attached &&
			(!previous.Before.Attached ||
				previous.Before.Attachment == previous.After.Attachment) &&
			previous.Before.Point == previous.After.Point
	case historySetPinnedBends:
		return slices.Equal(previous.Before.Bends, previous.After.Bends)
	default:
		return false
	}
}

func (l *Layout) applyChange(change historyChange, forward bool) error {
	state := change.Before
	if forward {
		state = change.After
	}
	switch change.Kind {
	case historyCreateNode:
		if forward {
			return l.restoreHistoryNode(state.Node)
		}
		return l.DeleteNode(change.ID)
	case historyDeleteNode:
		if forward {
			return l.DeleteNode(change.ID)
		}
		return l.restoreHistoryNode(state.Node)
	case historySetLabel:
		return l.SetNodeLabel(change.ID, state.Label)
	case historyPlaceNode:
		return l.PlaceNode(change.ID, state.Point)
	case historySetNodeSize:
		return l.setNodeSize(change.ID, state.Size)
	case historyCreateEdge, historyDeleteEdge, historyReconnectEdge:
		return l.applyEdgeChange(change, forward)
	case historySetLayer:
		return l.setLayerIndex(change.LayerHit, int(state.Layer))
	case historySetNodeStyle:
		return l.SetNodeStyle(change.ID, state.NodeStyle)
	case historySetEdgeStyle, historySetPinnedBends:
		return l.applyEdgeProperty(change, forward)
	case historySetRouter:
		l.SetRouter(state.Router)
		return nil
	case historySetAttachment:
		l.setAttachmentState(change.ID, state.Attachment, state.Attached)
		return l.PlaceNode(change.ID, state.Point)
	default:
		return fmt.Errorf("unknown layout change %d", change.Kind)
	}
}

func (l *Layout) applyEdgeChange(change historyChange, forward bool) error {
	state := change.Before
	if forward {
		state = change.After
	}
	switch change.Kind {
	case historyCreateEdge:
		if forward {
			return l.restoreHistoryEdge(change.ID, state.Edge, state.EdgeStyle, state.Bends, int(state.Layer), false)
		}
		return l.DeleteEdge(change.ID)
	case historyDeleteEdge:
		if forward {
			return l.DeleteEdge(change.ID)
		}
		if err := l.restoreHistoryEdge(change.ID, state.Edge, state.EdgeStyle, state.Bends, int(state.Layer), false); err != nil {
			return err
		}
		for _, attachment := range state.Attachments {
			l.setAttachmentState(attachment.NodeID, attachment, true)
		}
		return nil
	case historyReconnectEdge:
		return l.restoreHistoryEdge(change.ID, state.Edge, state.EdgeStyle, state.Bends, -1, true)
	default:
		return fmt.Errorf("unknown edge layout change %d", change.Kind)
	}
}

func (l *Layout) applyEdgeProperty(change historyChange, forward bool) error {
	state := change.Before
	if forward {
		state = change.After
	}
	switch change.Kind {
	case historySetEdgeStyle:
		return l.SetEdgeStyle(change.ID, state.EdgeStyle)
	case historySetPinnedBends:
		return l.SetPinnedBends(change.ID, state.Bends)
	default:
		return fmt.Errorf("unknown edge property layout change %d", change.Kind)
	}
}
