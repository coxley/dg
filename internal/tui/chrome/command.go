package chrome

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ScopeID identifies a command and focus scope.
type ScopeID string

// CommandID identifies an application-owned action.
type CommandID string

// ControlIntent identifies one semantic control operation.
type ControlIntent uint8

const (
	// NoControlIntent leaves the key available for component-specific input.
	NoControlIntent ControlIntent = iota
	// NavigateLeft moves or changes the current control to the left.
	NavigateLeft
	// NavigateRight moves or changes the current control to the right.
	NavigateRight
	// FocusNext advances to the next control.
	FocusNext
	// FocusPrevious returns to the previous control.
	FocusPrevious
	// Activate executes the focused control.
	Activate
)

// ResolveControlIntent maps one key press to control semantics.
func ResolveControlIntent(message tea.KeyPressMsg, textEntry bool) ControlIntent {
	switch {
	case message.Code == tea.KeyLeft || !textEntry && message.Mod == 0 && message.Text == "h":
		return NavigateLeft
	case message.Code == tea.KeyRight || !textEntry && message.Mod == 0 && message.Text == "l":
		return NavigateRight
	case message.Code == tea.KeyUp ||
		message.Code == tea.KeyTab && message.Mod == tea.ModShift ||
		!textEntry && message.Mod == 0 && message.Text == "k":
		return FocusPrevious
	case message.Code == tea.KeyDown ||
		message.Code == tea.KeyTab && message.Mod == 0 ||
		!textEntry && message.Mod == 0 && message.Text == "j":
		return FocusNext
	case message.Code == tea.KeyEnter:
		return Activate
	default:
		return NoControlIntent
	}
}

// KeyProfile controls Primary chord projection.
type KeyProfile uint8

const (
	// ProfileAuto selects the local platform convention.
	ProfileAuto KeyProfile = iota
	// ProfileMac resolves Primary to Super.
	ProfileMac
	// ProfileStandard resolves Primary to Control.
	ProfileStandard
)

// Chord is a normalized physical key chord.
type Chord string

// Keys normalizes physical chord declarations.
func Keys(chords ...string) []Chord {
	result := make([]Chord, len(chords))
	for i, chord := range chords {
		result[i] = normalizeChord(chord)
	}
	return result
}

// Primary returns one profile-aware chord declaration.
func Primary(key string) Chord {
	return Chord("primary+" + strings.ToLower(strings.TrimSpace(key)))
}

// Binding declares an application command.
type Binding struct {
	Scope   ScopeID
	Chords  []Chord
	Command CommandID
	Label   string
}

// CommandMsg reports a resolved semantic command.
type CommandMsg struct {
	Command CommandID
}

// EffectiveBinding is one projected, executable binding.
type EffectiveBinding struct {
	Scope   ScopeID
	Chord   Chord
	Command CommandID
	Label   string
}

// ErrBindingCollision reports two commands at one active priority.
var ErrBindingCollision = errors.New("binding collision")

// CollisionError identifies conflicting commands.
type CollisionError struct {
	Scope ScopeID
	Chord Chord
	First CommandID
	Next  CommandID
}

func (e CollisionError) Error() string {
	return fmt.Sprintf(
		"%v in scope %q for %q: %q and %q",
		ErrBindingCollision,
		e.Scope,
		e.Chord,
		e.First,
		e.Next,
	)
}

func (e CollisionError) Unwrap() error {
	return ErrBindingCollision
}

// Resolver projects bindings for active scope precedence.
type Resolver struct {
	bindings []Binding
	profile  KeyProfile
	super    bool
}

// NewResolver validates and returns a binding resolver.
func NewResolver(bindings []Binding) (*Resolver, error) {
	r := &Resolver{bindings: append([]Binding(nil), bindings...)}
	if err := r.validate(); err != nil {
		return nil, err
	}
	return r, nil
}

// SetProfile selects profile projection.
func (r *Resolver) SetProfile(profile KeyProfile) {
	r.profile = profile
}

// SetSuperAvailable controls whether projected Super chords are advertised.
func (r *Resolver) SetSuperAvailable(available bool) {
	r.super = available
}

// Resolve returns the first command in active scope order.
func (r *Resolver) Resolve(
	keystroke string,
	scopes []ScopeID,
	textEntry bool,
) (CommandMsg, bool) {
	chord := normalizeChord(keystroke)
	if textEntry && typableChord(chord) {
		return CommandMsg{}, false
	}
	for _, scope := range scopes {
		for _, binding := range r.bindings {
			if binding.Scope != scope {
				continue
			}
			for _, declared := range binding.Chords {
				projected, ok := r.project(declared)
				if ok && projected == chord {
					return CommandMsg{Command: binding.Command}, true
				}
			}
		}
	}
	return CommandMsg{}, false
}

// Effective returns the merged executable binding list in active precedence.
func (r *Resolver) Effective(scopes []ScopeID) []EffectiveBinding {
	seen := make(map[Chord]bool)
	var result []EffectiveBinding
	for _, scope := range scopes {
		for _, binding := range r.bindings {
			if binding.Scope != scope {
				continue
			}
			for _, declared := range binding.Chords {
				chord, ok := r.project(declared)
				if !ok || seen[chord] {
					continue
				}
				seen[chord] = true
				result = append(result, EffectiveBinding{
					Scope: scope, Chord: chord, Command: binding.Command, Label: binding.Label,
				})
			}
		}
	}
	return result
}

func (r *Resolver) validate() error {
	seen := make(map[[2]string]CommandID)
	for _, binding := range r.bindings {
		for _, chord := range binding.Chords {
			key := [2]string{string(binding.Scope), string(chord)}
			if first, ok := seen[key]; ok && first != binding.Command {
				return CollisionError{
					Scope: binding.Scope,
					Chord: chord,
					First: first,
					Next:  binding.Command,
				}
			}
			seen[key] = binding.Command
		}
	}
	return nil
}

func (r *Resolver) project(chord Chord) (Chord, bool) {
	value := string(chord)
	if !strings.HasPrefix(value, "primary+") {
		if strings.HasPrefix(value, "super+") && !r.super {
			return "", false
		}
		return chord, true
	}
	key := strings.TrimPrefix(value, "primary+")
	profile := r.profile
	if profile == ProfileAuto {
		if runtime.GOOS == "darwin" {
			profile = ProfileMac
		} else {
			profile = ProfileStandard
		}
	}
	if profile == ProfileMac {
		if !r.super {
			return "", false
		}
		return Chord("super+" + key), true
	}
	return Chord("ctrl+" + key), true
}

func normalizeChord(chord string) Chord {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(chord)), "+")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		switch parts[i] {
		case "cmd", "command", "meta":
			parts[i] = "super"
		case "control":
			parts[i] = "ctrl"
		case "option":
			parts[i] = "alt"
		}
	}
	if len(parts) > 1 {
		modifiers := parts[:len(parts)-1]
		slices.Sort(modifiers)
	}
	return Chord(strings.Join(parts, "+"))
}

func typableChord(chord Chord) bool {
	value := string(chord)
	return !strings.Contains(value, "+") && value != "esc" &&
		value != "enter" && value != "tab" && value != "backspace"
}
