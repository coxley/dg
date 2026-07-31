// Package history records reversible layout mutations as bounded interactions.
package history

import (
	"errors"
	"fmt"

	"github.com/coxley/dg/layout"
)

const defaultLimit = 256

var (
	// ErrAttached reports an attempt to attach history to a layout with a change callback.
	ErrAttached = errors.New("history already attached")
	// ErrTransactionClosed reports use of a completed or interrupted transaction.
	ErrTransactionClosed = errors.New("history transaction closed")
)

type entry struct {
	changes []layout.Change
}

// History records successful layout mutations as bounded undo transactions.
type History struct {
	layout *layout.Layout
	limit  int

	entries []entry
	cursor  int
	active  entry

	generation uint64
	activeGen  uint64
	resetting  bool

	cache cacheState
}

// Option configures History.
type Option func(*History)

// WithLimit limits the number of committed interactions retained.
func WithLimit(limit int) Option {
	return func(h *History) {
		h.limit = limit
	}
}

// New attaches a history to geo. By default, it retains 256 interactions.
func New(geo *layout.Layout, options ...Option) (*History, error) {
	if geo == nil {
		return nil, errors.New("history layout must not be nil")
	}
	h := &History{layout: geo, limit: defaultLimit}
	h.cache.delay = defaultCacheDelay
	for _, option := range options {
		option(h)
	}
	if h.limit < 1 {
		return nil, errors.New("history limit must be positive")
	}
	if err := h.cache.validate(); err != nil {
		return nil, err
	}
	if err := geo.SetChangeCallback(h.record); err != nil {
		if errors.Is(err, layout.ErrChangeCallbackAttached) {
			return nil, ErrAttached
		}
		return nil, fmt.Errorf("attach history: %w", err)
	}
	return h, nil
}

// Layout returns the layout whose mutations h records.
func (h *History) Layout() *layout.Layout {
	if h == nil {
		return nil
	}
	return h.layout
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
	h.active = entry{}
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
	active := h.active
	if err := h.layout.Replay(active.changes, layout.ReplayBackward); err != nil {
		return fmt.Errorf("cancel history transaction: %w", err)
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
	if err := h.layout.Replay(h.entries[h.cursor-1].changes, layout.ReplayBackward); err != nil {
		return false, fmt.Errorf("undo history: %w", err)
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
	if err := h.layout.Replay(h.entries[h.cursor].changes, layout.ReplayForward); err != nil {
		return false, fmt.Errorf("redo history: %w", err)
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
	h.cache.clearPending()
	clear(h.entries)
	h.entries = h.entries[:0]
	h.cursor = 0
	h.cache.savedCursor = 0
	h.closeActive()
	if h.layout != nil && h.cache.path != "" {
		h.cache.saved = h.layout.Snapshot()
		h.cache.branchValid = true
		h.scheduleCache()
		return
	}
	h.cache.saved = layout.Snapshot{}
	h.cache.branchValid = false
}

// Reset replaces the layout while suppressing and then clearing history.
func (h *History) Reset(replace func() error) error {
	if h == nil || h.layout == nil {
		return errors.New("history is not attached")
	}
	if replace == nil {
		return errors.New("history reset callback must not be nil")
	}
	rollback := h.layout.Snapshot()
	err := func() error {
		h.resetting = true
		defer func() {
			h.resetting = false
		}()
		return replace()
	}()
	if err != nil {
		if restoreErr := h.layout.Restore(rollback); restoreErr != nil {
			return errors.Join(
				fmt.Errorf("reset history layout: %w", err),
				fmt.Errorf("restore history reset rollback: %w", restoreErr),
			)
		}
		return fmt.Errorf("reset history layout: %w", err)
	}
	h.clearAfterReset()
	h.generation++
	return nil
}

func (h *History) clearAfterReset() {
	h.cache.clearPending()
	clear(h.entries)
	h.entries = h.entries[:0]
	h.cursor = 0
	h.closeActive()
	h.cache.path = ""
	h.cache.key = ""
	h.cache.saved = layout.Snapshot{}
	h.cache.savedCursor = 0
	h.cache.branchValid = false
	h.setCacheError(nil)
}

func (h *History) record(change layout.Change) {
	if h == nil || h.resetting {
		return
	}
	if h.activeGen == 0 {
		h.discardRedo()
		h.entries = append(h.entries, entry{changes: []layout.Change{change}})
		h.cursor = len(h.entries)
		h.enforceLimit()
		h.scheduleCache()
		return
	}
	h.discardRedo()
	changes, ok := layout.CoalesceChanges(h.active.changes, change)
	h.active.changes = changes
	if !ok {
		h.active.changes = append(h.active.changes, change)
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
	h.active = entry{}
	h.activeGen = 0
}

func (h *History) discardRedo() {
	if h.cursor == len(h.entries) {
		return
	}
	if h.cache.path != "" && h.cursor < h.cache.savedCursor {
		h.cache.branchValid = false
	}
	clear(h.entries[h.cursor:])
	h.entries = h.entries[:h.cursor]
}

func (h *History) enforceLimit() {
	excess := len(h.entries) - h.limit
	if excess <= 0 {
		return
	}
	if h.cache.path != "" && excess > h.cache.savedCursor {
		h.cache.branchValid = false
	}
	copy(h.entries, h.entries[excess:])
	clear(h.entries[len(h.entries)-excess:])
	h.entries = h.entries[:len(h.entries)-excess]
	h.cursor = max(0, h.cursor-excess)
	h.cache.savedCursor = max(0, h.cache.savedCursor-excess)
}
