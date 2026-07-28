package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type keyMap struct {
	help          key.Binding
	delete        key.Binding
	duplicate     key.Binding
	rectangle     key.Binding
	edit          key.Binding
	line          key.Binding
	border        key.Binding
	dashed        key.Binding
	arrows        key.Binding
	focus         key.Binding
	move          key.Binding
	expand        key.Binding
	layer         key.Binding
	layerExtremes key.Binding
	save          key.Binding
	undo          key.Binding
	redo          key.Binding
	duplicateDrag key.Binding
	addSelection  key.Binding
	close         key.Binding
	textAlign     key.Binding
	copy          key.Binding
}

func newKeyMap() keyMap {
	binding := func(keys, display, description string) key.Binding {
		return key.NewBinding(
			key.WithKeys(keys),
			key.WithHelp(display, description),
		)
	}
	return keyMap{
		help:          binding("?", "?", "help"),
		delete:        binding("backspace", "backspace", "delete"),
		duplicate:     binding("d", "d", "duplicate"),
		rectangle:     binding("r", "r", "rectangle"),
		edit:          binding("e", "e", "edit label"),
		line:          binding("l", "l", "line"),
		border:        binding("b", "b", "border"),
		dashed:        binding("-", "-", "dashed"),
		arrows:        binding("a", "a / A", "arrows"),
		focus:         binding("tab", "tab / shift-tab", "focus"),
		move:          binding("up", "arrows", "move"),
		expand:        binding("ctrl+a", "ctrl+a", "expand"),
		layer:         binding("[", "[ / ]", "layer"),
		layerExtremes: binding("{", "{ / }", "back / front"),
		save:          binding("ctrl+s", "ctrl+s", "save"),
		undo:          binding("ctrl+z", "u / ctrl+z", "undo"),
		redo:          binding("ctrl+y", "ctrl+r / ctrl+y", "redo"),
		duplicateDrag: binding("alt+mouse", "alt-drag", "duplicate"),
		addSelection:  binding("ctrl+mouse", "ctrl-click", "add / remove"),
		close:         binding("esc", "esc / q", "close"),
		textAlign:     binding("t", "t / T", "text align"),
		copy: key.NewBinding(
			key.WithKeys("super+c", "ctrl+c"),
			key.WithHelp("ctrl+c", "copy"),
		),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.help, k.save, k.undo, k.redo}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			k.help,
			k.delete,
			k.duplicate,
			k.rectangle,
			k.edit,
			k.line,
			k.border,
			k.dashed,
			k.arrows,
		},
		{
			k.focus,
			k.move,
			k.expand,
			k.layer,
			k.layerExtremes,
			k.save,
			k.undo,
			k.redo,
		},
		{
			k.duplicateDrag,
			k.addSelection,
			k.close,
			k.textAlign,
			k.copy,
		},
	}
}

func (k *keyMap) setKeyboardEnhancements(enabled bool) {
	if enabled {
		k.copy.SetHelp("super+c / ctrl+c", "copy")
		return
	}
	k.copy.SetHelp("ctrl+c", "copy")
}

// isCopyKey uses Keystroke because printable enhanced-key text hides modifiers in String.
func isCopyKey(message tea.KeyPressMsg) bool {
	switch message.Keystroke() {
	case "ctrl+c", "super+c":
		return true
	default:
		return false
	}
}

func isModifierKey(message tea.KeyPressMsg) bool {
	switch message.Key().Code {
	case tea.KeyLeftShift, tea.KeyRightShift,
		tea.KeyLeftAlt, tea.KeyRightAlt,
		tea.KeyLeftCtrl, tea.KeyRightCtrl,
		tea.KeyLeftMeta, tea.KeyRightMeta,
		tea.KeyLeftHyper, tea.KeyRightHyper,
		tea.KeyLeftSuper, tea.KeyRightSuper:
		return true
	default:
		return false
	}
}
