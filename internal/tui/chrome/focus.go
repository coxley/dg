package chrome

// FocusID identifies one interactive target.
type FocusID string

// FocusTarget registers one enabled arranged target.
type FocusTarget struct {
	ID      FocusID
	Rect    Rect
	Enabled bool
}

// FocusRegistry retains focus and restoration per scope.
type FocusRegistry struct {
	scopes map[ScopeID][]FocusTarget
	last   map[ScopeID]FocusID
	stack  []focusFrame
	scope  ScopeID
	focus  FocusID
}

type focusFrame struct {
	scope ScopeID
	focus FocusID
}

// NewFocusRegistry returns an empty focus registry.
func NewFocusRegistry() *FocusRegistry {
	return &FocusRegistry{
		scopes: make(map[ScopeID][]FocusTarget),
		last:   make(map[ScopeID]FocusID),
	}
}

// Register replaces one scope's arranged focus order.
func (r *FocusRegistry) Register(scope ScopeID, targets []FocusTarget) {
	r.scopes[scope] = append(r.scopes[scope][:0], targets...)
	if r.scope == scope && !r.valid(scope, r.focus) {
		r.focus = r.first(scope)
	}
}

// Open saves current focus and enters scope.
func (r *FocusRegistry) Open(scope ScopeID) FocusID {
	r.stack = append(r.stack, focusFrame{scope: r.scope, focus: r.focus})
	r.scope = scope
	if r.valid(scope, r.last[scope]) {
		r.focus = r.last[scope]
	} else {
		r.focus = r.first(scope)
	}
	return r.focus
}

// Close restores the previously active scope and valid focus.
func (r *FocusRegistry) Close(fallback ScopeID) FocusID {
	if r.scope != "" && r.focus != "" {
		r.last[r.scope] = r.focus
	}
	if len(r.stack) == 0 {
		r.scope = fallback
		r.focus = r.first(fallback)
		return r.focus
	}
	frame := r.stack[len(r.stack)-1]
	r.stack = r.stack[:len(r.stack)-1]
	r.scope = frame.scope
	if r.valid(frame.scope, frame.focus) {
		r.focus = frame.focus
	} else {
		r.focus = r.first(frame.scope)
	}
	return r.focus
}

// Move advances focus by delta with wrapping.
func (r *FocusRegistry) Move(delta int) FocusID {
	targets := r.enabled(r.scope)
	if len(targets) == 0 {
		r.focus = ""
		return ""
	}
	index := 0
	for i, target := range targets {
		if target.ID == r.focus {
			index = i
			break
		}
	}
	index = (index + delta%len(targets) + len(targets)) % len(targets)
	r.focus = targets[index].ID
	r.last[r.scope] = r.focus
	return r.focus
}

// Current returns active scope and focus.
func (r *FocusRegistry) Current() (ScopeID, FocusID) {
	return r.scope, r.focus
}

// Reveal exposes focused geometry through viewport mechanics.
func (r *FocusRegistry) Reveal(viewport *Viewport) {
	if viewport == nil {
		return
	}
	for _, target := range r.scopes[r.scope] {
		if target.ID == r.focus {
			viewport.Reveal(target.Rect)
			return
		}
	}
}

func (r *FocusRegistry) first(scope ScopeID) FocusID {
	for _, target := range r.scopes[scope] {
		if target.Enabled {
			return target.ID
		}
	}
	return ""
}

func (r *FocusRegistry) valid(scope ScopeID, id FocusID) bool {
	if id == "" {
		return false
	}
	for _, target := range r.scopes[scope] {
		if target.ID == id {
			return target.Enabled
		}
	}
	return false
}

func (r *FocusRegistry) enabled(scope ScopeID) []FocusTarget {
	var result []FocusTarget
	for _, target := range r.scopes[scope] {
		if target.Enabled {
			result = append(result, target)
		}
	}
	return result
}
