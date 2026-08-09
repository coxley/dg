package chrome

import (
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// TextEditIntent identifies one editing operation shared by text controls.
type TextEditIntent uint8

const (
	// TextEditNone leaves the key available for component-specific input.
	TextEditNone TextEditIntent = iota
	// TextEditBackward moves one grapheme backward.
	TextEditBackward
	// TextEditForward moves one grapheme forward.
	TextEditForward
	// TextEditLineStart moves to the start of the current logical line.
	TextEditLineStart
	// TextEditLineEnd moves to the end of the current logical line.
	TextEditLineEnd
	// TextEditWordBackward moves to the previous word boundary.
	TextEditWordBackward
	// TextEditWordForward moves to the next word boundary.
	TextEditWordForward
	// TextEditDeleteBackward deletes the previous grapheme.
	TextEditDeleteBackward
	// TextEditDeleteForward deletes the next grapheme.
	TextEditDeleteForward
	// TextEditDeleteWordBackward deletes to the previous word boundary.
	TextEditDeleteWordBackward
	// TextEditDeleteToLineStart deletes to the start of the current logical line.
	TextEditDeleteToLineStart
	// TextEditDeleteToLineEnd deletes to the end of the current logical line.
	TextEditDeleteToLineEnd
)

// ResolveTextEditIntent maps one key press to shared text-editing semantics.
func ResolveTextEditIntent(message tea.KeyPressMsg) TextEditIntent {
	switch ChordForKey(message) {
	case "left", "ctrl+b":
		return TextEditBackward
	case "right", "ctrl+f":
		return TextEditForward
	case "home", "ctrl+a":
		return TextEditLineStart
	case "end", "ctrl+e":
		return TextEditLineEnd
	case "alt+b", "alt+shift+b":
		return TextEditWordBackward
	case "alt+f", "alt+shift+f":
		return TextEditWordForward
	case "backspace", "ctrl+h":
		return TextEditDeleteBackward
	case "delete", "ctrl+d":
		return TextEditDeleteForward
	case "ctrl+w", "alt+backspace":
		return TextEditDeleteWordBackward
	case "ctrl+u":
		return TextEditDeleteToLineStart
	case "ctrl+k":
		return TextEditDeleteToLineEnd
	default:
		return TextEditNone
	}
}

// IsTextWordBoundary reports whether a grapheme contains only whitespace or slashes.
func IsTextWordBoundary(cluster []byte) bool {
	if len(cluster) == 0 {
		return false
	}
	for len(cluster) != 0 {
		r, size := utf8.DecodeRune(cluster)
		if !unicode.IsSpace(r) && r != '/' {
			return false
		}
		cluster = cluster[size:]
	}
	return true
}
