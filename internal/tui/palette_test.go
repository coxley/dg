package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/stretchr/testify/require"
)

func TestPaletteOpensOnlyWithKeyboardDisambiguation(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	open := helpKeyPress(t, chrome.NormalizeChord(paletteDefaultChord))
	chord, ok := model.bindings.ChordFor(scopeGlobal, commandPalette)
	require.True(t, ok)
	require.Equal(t, chrome.Chord("super+/"), chord)
	require.Equal(t, "cmd+/", chrome.DisplayChord(chord, chrome.VocabularyMac))

	updateModel(t, model, open)
	require.Equal(t, surfaceNone, model.dialogs.ActiveID())

	updateModel(t, model, tea.KeyboardEnhancementsMsg{
		Flags: ansi.KittyDisambiguateEscapeCodes,
	})
	require.True(t, model.keyboardDisambiguation)
	require.True(t, model.bindings.MatchesKey(open, commandPalette))
	updateModel(t, model, open)
	require.Equal(t, surfacePalette, model.dialogs.ActiveID())
}

func TestPaletteOpensThroughAdvertisedHelpBinding(t *testing.T) {
	t.Parallel()

	model := newHelpScenarioModel(t, helpExistingDiagramBlankCell, 1, 1)
	setHelpKeyboardEnhancements(t, model, true)
	key := helpKeyPress(t, chrome.NormalizeChord(paletteDefaultChord))
	resolved, ok := model.bindings.ResolveKey(
		key,
		model.activeBindingScopes(),
		model.textEntryActive(),
	)
	require.True(t, ok)
	require.Equal(t, commandPalette, resolved.Command)

	before := snapshotHelpCommand(model)
	updateModel(t, model, key)
	require.Equal(t, surfacePalette, model.dialogs.ActiveID())
	require.True(t, helpCommandPerformed(model, commandPalette, nil, before))
}

func TestPaletteRestoresUnderlyingDialog(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.dialogs.OpenNotice("notice", surfaceNone)
	model.keyboardDisambiguation = true
	model.bindings.SetKeyDisambiguation(true)

	updateModel(t, model, helpKeyPress(t, chrome.NormalizeChord(paletteDefaultChord)))
	require.Equal(t, surfacePalette, model.dialogs.ActiveID())

	updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	require.Equal(t, surfaceNotice, model.dialogs.ActiveID())
}

func TestPaletteDoesNotOpenOverConfirmation(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.dialogs.OpenConfirmation("Title", "Message", "Confirm", clearDraftsMsg{})
	model.keyboardDisambiguation = true
	model.bindings.SetKeyDisambiguation(true)

	updateModel(t, model, helpKeyPress(t, chrome.NormalizeChord(paletteDefaultChord)))
	require.Equal(t, surfaceConfirmation, model.dialogs.ActiveID())
}

func TestPaletteDoesNotOpenWhileEnteringText(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.dialogs.OpenSave()
	model.keyboardDisambiguation = true
	model.bindings.SetKeyDisambiguation(true)

	updateModel(t, model, helpKeyPress(t, chrome.NormalizeChord(paletteDefaultChord)))
	require.Equal(t, surfaceSave, model.dialogs.ActiveID())
}

func TestPaletteOpensFromNonTextFormControl(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.dialogs.OpenSave()
	require.True(t, model.dialogs.save.form.Focus(saveConfirmAction))
	model.keyboardDisambiguation = true
	model.bindings.SetKeyDisambiguation(true)

	updateModel(t, model, helpKeyPress(t, chrome.NormalizeChord(paletteDefaultChord)))
	require.Equal(t, surfacePalette, model.dialogs.ActiveID())
}

func TestPaletteFiltersRelevantActionsBeforeBetterFuzzyMatches(t *testing.T) {
	t.Parallel()

	const (
		foo    = "foo"
		foobar = "foobar"
		foobaz = "foobaz"
	)
	body := newPaletteDialogBody(DefaultTheme(true).Palette)
	body.Reset([]paletteItem{
		{Command: foo, Label: foo},
		{Command: foobar, Label: foobar},
		{Command: foobaz, Label: foobaz, Relevant: true},
	})
	body.SetBounds(chrome.Rect{Width: 40})
	body.query.SetValue(foo)
	body.filter()

	require.Equal(t, []string{foobaz, foo, foobar}, paletteLabels(body.filtered))
}

func TestPaletteHidesActionsAndDeduplicatesScopedBindings(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	items := model.paletteItems(canvasBindingScopes[:])
	commands := make([]chrome.CommandID, len(items))
	for i := range items {
		commands[i] = items[i].Command
	}

	require.Equal(t, 1, countCommand(commands, commandNewCanvas))
	require.NotContains(t, commands, commandBack)
	require.NotContains(t, commands, commandMoveUp)
	require.NotContains(t, commands, commandMoveDown)
	require.NotContains(t, commands, commandMoveLeft)
	require.NotContains(t, commands, commandMoveRight)
	require.NotContains(t, commands, commandPalette)
}

func TestPaletteShowsUnboundCanvasPathActionByAvailability(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	item := requirePaletteItem(t, model.paletteItems(canvasBindingScopes[:]), commandCopyCanvasPath)
	require.Empty(t, item.Binding)
	require.False(t, item.Relevant)

	stored, _, _ := newStoredTestModel(t, "active draft")
	item = requirePaletteItem(t, stored.paletteItems(canvasBindingScopes[:]), commandCopyCanvasPath)
	require.Empty(t, item.Binding)
	require.True(t, item.Relevant)
}

func TestPaletteDisabledActionRemainsVisibleButCannotExecute(t *testing.T) {
	t.Parallel()

	body := newPaletteDialogBody(DefaultTheme(true).Palette)
	body.Reset([]paletteItem{{Command: "disabled", Label: "Disabled"}})
	body.SetBounds(chrome.Rect{Width: 40})

	require.Len(t, body.filtered, 1)
	require.Equal(t, -1, body.selected)
	result := body.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	require.Nil(t, result.message)
}

func TestPaletteMouseSelectsRelevantAction(t *testing.T) {
	t.Parallel()

	body := newPaletteDialogBody(DefaultTheme(true).Palette)
	body.Reset([]paletteItem{{Command: "enabled", Label: "Enabled", Relevant: true}})
	body.SetBounds(chrome.Rect{Width: 40})
	point := chrome.Point{X: 39, Y: 2}

	require.True(t, body.PointerOccupied(point))
	result := body.Update(dialogClickMsg{Point: point})
	require.Equal(t, paletteSelectedMsg{Command: "enabled"}, result.message)
}

func TestPalettePinsBindingToRightEdge(t *testing.T) {
	t.Parallel()

	line := paletteItemLine(paletteItem{Label: "Bring Forward", Binding: "]"}, 40)
	require.Equal(t, 40, ansi.StringWidth(line))
	require.Equal(t, "]", line[len(line)-1:])
}

func TestPalettePinsBindingBeforeScrollbar(t *testing.T) {
	t.Parallel()

	const binding = "cmd+shift+enter"
	body := newPaletteDialogBody(DefaultTheme(true).Palette)
	items := make([]paletteItem, paletteDefaultRows+1)
	for i := range items {
		items[i] = paletteItem{
			Command:  chrome.CommandID(string(rune('a' + i))),
			Label:    "Action",
			Binding:  binding,
			Relevant: true,
		}
	}
	body.Reset(items)
	body.SetBounds(chrome.Rect{
		Width: 40, Height: paletteDefaultRows + paletteFrameRows,
	})

	plan := body.viewport.Plan()
	require.Equal(t, 39, plan.Content.Width)
	require.Equal(t, 1, plan.VerticalBar.Width)
	content := ansi.Cut(ansi.Strip(body.viewport.Lines()[0]), 0, plan.Content.Width)
	require.Equal(t, " ", content[len(content)-paletteScrollbarGap:])
	content = content[:len(content)-paletteScrollbarGap]
	require.Equal(t, binding, content[len(content)-len(binding):])
}

func TestPaletteFitsRequiredTerminalSizes(t *testing.T) {
	t.Parallel()

	for _, size := range []chrome.Size{
		{Width: 100, Height: 30},
		{Width: 80, Height: 16},
		{Width: 80, Height: 12},
	} {
		model, _ := newTestModel(t)
		updateModel(t, model, tea.WindowSizeMsg{
			Width: size.Width, Height: size.Height,
		})
		model.openPalette()
		overlay := model.dialogs.Overlay()

		require.LessOrEqual(t, overlay.Width, size.Width)
		require.LessOrEqual(t, overlay.Height, size.Height)
		if model.dialogs.Fullscreen() {
			continue
		}
		require.InDelta(t, size.Width, 2*overlay.Left+overlay.Width, 1)
		require.InDelta(t, size.Height, 2*overlay.Top+overlay.Height, 1)
	}
}

func paletteLabels(items []paletteItem) []string {
	labels := make([]string, len(items))
	for i := range items {
		labels[i] = items[i].Label
	}
	return labels
}

func countCommand(commands []chrome.CommandID, command chrome.CommandID) int {
	count := 0
	for _, candidate := range commands {
		if candidate == command {
			count++
		}
	}
	return count
}

func requirePaletteItem(
	t testing.TB,
	items []paletteItem,
	command chrome.CommandID,
) paletteItem {
	t.Helper()
	for _, item := range items {
		if item.Command == command {
			return item
		}
	}
	require.FailNow(t, "palette item not found", "%s", command)
	return paletteItem{}
}
