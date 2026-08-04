package chrome

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const escapeChord = "esc"

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

// ChordVocabulary controls presentation without changing executable chords.
type ChordVocabulary uint8

const (
	// VocabularyStandard preserves normalized Bubble Tea modifier names.
	VocabularyStandard ChordVocabulary = iota
	// VocabularyMac presents Super as Command.
	VocabularyMac
)

// Chord is a normalized logical key chord.
type Chord string

// Keys normalizes logical chord declarations.
func Keys(chords ...string) []Chord {
	result := make([]Chord, len(chords))
	for i, chord := range chords {
		result[i] = normalizeChord(chord)
	}
	return result
}

// NormalizeChord returns the canonical representation of a logical key chord.
func NormalizeChord(chord string) Chord {
	return normalizeChord(chord)
}

// ChordForKey returns the canonical logical chord for one key press.
func ChordForKey(message tea.KeyPressMsg) Chord {
	return keyChord(message)
}

// Primary returns one profile-aware chord declaration.
func Primary(key string) Chord {
	return Chord("primary+" + strings.ToLower(strings.TrimSpace(key)))
}

// VocabularyForProfile returns the display vocabulary for a key profile.
func VocabularyForProfile(profile KeyProfile) ChordVocabulary {
	if effectiveProfile(profile) == ProfileMac {
		return VocabularyMac
	}
	return VocabularyStandard
}

// DisplayChord renders a normalized chord in the requested vocabulary.
func DisplayChord(chord Chord, vocabulary ChordVocabulary) string {
	parts := strings.Split(string(chord), "+")
	if vocabulary == VocabularyMac {
		for i, part := range parts {
			if part == "super" {
				parts[i] = "cmd"
			}
		}
	}
	return strings.Join(parts, "+")
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
	bindings     []Binding
	profile      KeyProfile
	disambiguate bool
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

// SetBindings replaces the declarations used for subsequent resolution.
// Resolution remains deterministic when user-defined mappings conflict: the
// first declaration in the active scope wins.
func (r *Resolver) SetBindings(bindings []Binding) {
	r.bindings = append(r.bindings[:0], bindings...)
}

// SetKeyDisambiguation controls whether chords that require enhanced key
// reporting are advertised.
func (r *Resolver) SetKeyDisambiguation(available bool) {
	r.disambiguate = available
}

// ResolveKey returns the first command matching a key in active scope order.
func (r *Resolver) ResolveKey(
	message tea.KeyPressMsg,
	scopes []ScopeID,
	textEntry bool,
) (CommandMsg, bool) {
	return r.resolve(keyChord(message), scopes, textEntry)
}

// Resolve returns the first command in active scope order.
func (r *Resolver) Resolve(
	keystroke string,
	scopes []ScopeID,
	textEntry bool,
) (CommandMsg, bool) {
	return r.resolve(normalizeChord(keystroke), scopes, textEntry)
}

func (r *Resolver) resolve(
	chord Chord,
	scopes []ScopeID,
	textEntry bool,
) (CommandMsg, bool) {
	if textEntry && typableChord(chord) {
		return CommandMsg{}, false
	}
	for _, scope := range scopes {
		for _, binding := range r.bindings {
			if binding.Scope != scope {
				continue
			}
			for _, declared := range binding.Chords {
				projected, ok := projectChord(declared, r.profile, true)
				if ok && projected == chord {
					return CommandMsg{Command: binding.Command}, true
				}
			}
		}
	}
	return CommandMsg{}, false
}

// MatchesKey reports whether a key projects to any declaration for command.
func (r *Resolver) MatchesKey(message tea.KeyPressMsg, command CommandID) bool {
	chord := keyChord(message)
	for _, binding := range r.bindings {
		if binding.Command != command {
			continue
		}
		for _, declared := range binding.Chords {
			projected, ok := projectChord(declared, r.profile, true)
			if ok && projected == chord {
				return true
			}
		}
	}
	return false
}

// ChordFor returns the first profile-projected chord for a scoped command.
func (r *Resolver) ChordFor(scope ScopeID, command CommandID) (Chord, bool) {
	for _, binding := range r.bindings {
		if binding.Scope != scope || binding.Command != command {
			continue
		}
		for _, declared := range binding.Chords {
			if chord, ok := projectChord(declared, r.profile, true); ok {
				return chord, true
			}
		}
	}
	return "", false
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
	for _, profile := range [...]KeyProfile{ProfileMac, ProfileStandard} {
		seen := make(map[[2]string]CommandID)
		for _, binding := range r.bindings {
			for _, declared := range binding.Chords {
				chord, ok := projectChord(declared, profile, true)
				if !ok {
					continue
				}
				key := [2]string{string(binding.Scope), string(chord)}
				if first, exists := seen[key]; exists && first != binding.Command {
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
	}
	return nil
}

func (r *Resolver) project(chord Chord) (Chord, bool) {
	return projectChord(chord, r.profile, r.disambiguate)
}

func projectChord(chord Chord, profile KeyProfile, disambiguate bool) (Chord, bool) {
	value := string(chord)
	if !strings.HasPrefix(value, "primary+") {
		if chordRequiresDisambiguation(chord) && !disambiguate {
			return "", false
		}
		return chord, true
	}
	key := strings.TrimPrefix(value, "primary+")
	profile = effectiveProfile(profile)
	if profile == ProfileMac {
		if !disambiguate {
			return "", false
		}
		return Chord("super+" + key), true
	}
	return Chord("ctrl+" + key), true
}

func chordRequiresDisambiguation(chord Chord) bool {
	value := string(chord)
	return strings.HasPrefix(value, "super+") ||
		value == "ctrl+enter" ||
		strings.Contains(value, "ctrl+shift+")
}

func effectiveProfile(profile KeyProfile) KeyProfile {
	if profile != ProfileAuto {
		return profile
	}
	if runtime.GOOS == "darwin" {
		return ProfileMac
	}
	return ProfileStandard
}

func keyChord(message tea.KeyPressMsg) Chord {
	key := message.Key()
	// Keystroke prefers the PC-101 BaseCode when present, which would bind
	// shortcuts to QWERTY key positions instead of the active keyboard layout.
	key.BaseCode = 0
	return normalizeChord(key.Keystroke())
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
	if len(parts) == 2 && parts[0] == "shift" {
		switch parts[1] {
		case "/":
			return "?"
		case "[":
			return "{"
		case "]":
			return "}"
		}
	}
	return Chord(strings.Join(parts, "+"))
}

func typableChord(chord Chord) bool {
	value := string(chord)
	return !strings.Contains(value, "+") && value != escapeChord &&
		value != "enter" && value != "tab" && value != "backspace"
}
