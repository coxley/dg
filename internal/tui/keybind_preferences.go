package tui

import (
	"cmp"
	"slices"

	"github.com/coxley/dg/internal/settings"
	"github.com/coxley/dg/internal/tui/chrome"
	preferencesview "github.com/coxley/dg/internal/tui/preferences"
)

const canvasScopeTitle = "Canvas"

var keybindScopeOrder = map[chrome.ScopeID]int{
	scopeGlobal:      0,
	scopeCanvas:      1,
	scopeSidebar:     2,
	scopeLabel:       3,
	scopePreferences: 4,
	scopeDirectory:   5,
	scopeModal:       6,
}

var keybindScopeLabels = map[chrome.ScopeID]string{
	scopeGlobal:      "Global",
	scopeCanvas:      canvasScopeTitle,
	scopeSidebar:     "Sidebar",
	scopeLabel:       "Label Editor",
	scopePreferences: "Preferences",
	scopeDirectory:   "Directory Picker",
	scopeModal:       "Modal",
}

func keybindActions() []preferencesview.KeybindAction {
	actions := make([]preferencesview.KeybindAction, 0, len(applicationBindings))
	for _, action := range applicationActions {
		for _, binding := range action.Bindings {
			actions = append(actions, preferencesview.KeybindAction{
				Scope: binding.Scope, ScopeLabel: keybindScopeLabels[binding.Scope],
				Command: action.Command, Label: action.Label,
			})
		}
	}
	slices.SortStableFunc(actions, func(a, b preferencesview.KeybindAction) int {
		if order := cmp.Compare(keybindScopeOrder[a.Scope], keybindScopeOrder[b.Scope]); order != 0 {
			return order
		}
		return cmp.Compare(a.Label, b.Label)
	})
	return actions
}

func keybindValues(bindings []chrome.Binding) []preferencesview.Keybind {
	values := make([]preferencesview.Keybind, 0, len(bindings))
	for _, binding := range bindings {
		value := preferencesview.Keybind{Scope: binding.Scope, Command: binding.Command}
		for i, chord := range binding.Chords[:min(len(binding.Chords), 3)] {
			value.Mappings[i] = chord
		}
		values = append(values, value)
	}
	return values
}

func bindingsFromValues(values []preferencesview.Keybind) []chrome.Binding {
	bindings := cloneBindings(applicationBindings)
	for i := range bindings {
		value, ok := findKeybindValue(values, bindings[i].Scope, bindings[i].Command)
		if !ok {
			continue
		}
		bindings[i].Chords = bindings[i].Chords[:0]
		for _, chord := range value.Mappings {
			if chord != "" {
				bindings[i].Chords = append(bindings[i].Chords, chord)
			}
		}
	}
	return bindings
}

func configuredBindings(snapshot settings.Snapshot) []chrome.Binding {
	if len(snapshot.Keybinds) == 0 {
		return cloneBindings(applicationBindings)
	}
	values := make([]preferencesview.Keybind, 0, len(snapshot.Keybinds))
	for _, configured := range snapshot.Keybinds {
		value := preferencesview.Keybind{
			Scope: chrome.ScopeID(configured.Scope), Command: chrome.CommandID(configured.Action),
		}
		for i, mapping := range configured.Mappings[:min(len(configured.Mappings), 3)] {
			value.Mappings[i] = chrome.NormalizeChord(mapping)
		}
		values = append(values, value)
	}
	values = migrateLabelCommitBinding(values)
	return bindingsFromValues(values)
}

func migrateLabelCommitBinding(values []preferencesview.Keybind) []preferencesview.Keybind {
	commitConfigured := false
	for i := range values {
		value := &values[i]
		if value.Scope == scopeLabel && value.Command == commandCommitLabel {
			commitConfigured = true
		}
		if value.Scope != scopeLabel || value.Command != commandCancel {
			continue
		}
		var retained [3]chrome.Chord
		next := 0
		for _, mapping := range value.Mappings {
			if mapping == labelCommitControlChord || mapping == labelCommitSuperChord {
				continue
			}
			if mapping != "" {
				retained[next] = mapping
				next++
			}
		}
		value.Mappings = retained
	}
	if !commitConfigured {
		values = append(values, preferencesview.Keybind{
			Scope: scopeLabel, Command: commandCommitLabel,
			Mappings: [3]chrome.Chord{labelCommitControlChord, labelCommitSuperChord},
		})
	}
	return values
}

func cloneBindings(bindings []chrome.Binding) []chrome.Binding {
	cloned := make([]chrome.Binding, len(bindings))
	for i, binding := range bindings {
		cloned[i] = binding
		cloned[i].Chords = append([]chrome.Chord(nil), binding.Chords...)
	}
	return cloned
}

func settingsKeybinds(values []preferencesview.Keybind) []settings.Keybind {
	configured := make([]settings.Keybind, 0, len(values))
	for _, value := range values {
		last := len(value.Mappings)
		for last > 0 && value.Mappings[last-1] == "" {
			last--
		}
		mappings := make([]string, last)
		for i, mapping := range value.Mappings[:last] {
			mappings[i] = string(mapping)
		}
		configured = append(configured, settings.Keybind{
			Scope: string(value.Scope), Action: string(value.Command), Mappings: mappings,
		})
	}
	return configured
}

func findKeybindValue(
	values []preferencesview.Keybind,
	scope chrome.ScopeID,
	command chrome.CommandID,
) (preferencesview.Keybind, bool) {
	for _, value := range values {
		if value.Scope == scope && value.Command == command {
			return value, true
		}
	}
	return preferencesview.Keybind{}, false
}
