package modal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/coxley/dg/internal/tui/chrome"
	"github.com/stretchr/testify/require"
)

const keepEditing chrome.ID = "keep-editing"

func TestConfirmationAnchorsDeclaredActionsAtBottom(t *testing.T) {
	t.Parallel()

	confirmation := newTestConfirmation()
	confirmation.SetBounds(chrome.Rect{X: 2, Y: 3, Width: 32, Height: 8})
	plan := confirmation.Plan()

	require.Equal(t, chrome.ID("discard.actions"), plan.ActionBarID)
	require.Equal(t, chrome.ID("discard.spacer"), plan.SpacerID)
	require.Equal(t, plan.Bounds.Bottom(), plan.Actions[0].Rect.Bottom())
	require.Contains(t, confirmation.View().Content, "Discard changes?")
}

func TestConfirmationEmitsSelectedSemanticAction(t *testing.T) {
	t.Parallel()

	confirmation := newTestConfirmation()
	confirmation.SetBounds(chrome.Rect{Width: 32, Height: 8})
	_, _ = confirmation.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	_, command := confirmation.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	require.NotNil(t, command)
	require.Equal(
		t,
		chrome.FormSubmitMsg{ID: keepEditing},
		command(),
	)
}

func TestConfirmationPointerUsesRetainedActionGeometry(t *testing.T) {
	t.Parallel()

	confirmation := newTestConfirmation()
	confirmation.SetBounds(chrome.Rect{Width: 32, Height: 8})
	cancel := confirmation.Plan().Actions[1].Rect

	command := confirmation.Click(chrome.Point{X: cancel.X, Y: cancel.Y})

	require.NotNil(t, command)
	require.Equal(
		t,
		chrome.FormSubmitMsg{ID: keepEditing},
		command(),
	)
}

func TestConfirmationClipsPromptBeforeActions(t *testing.T) {
	t.Parallel()

	confirmation := newTestConfirmation()
	bounds := chrome.Rect{Width: 32, Height: 2}
	confirmation.SetBounds(bounds)

	for _, action := range confirmation.Plan().Actions {
		require.True(t, bounds.Contains(chrome.Point{
			X: action.Rect.X,
			Y: action.Rect.Y,
		}))
		require.LessOrEqual(t, action.Rect.Bottom(), bounds.Bottom())
	}
	require.Equal(t, bounds.Height, strings.Count(confirmation.View().Content, "\n")+1)
}

func TestConfirmationPreservesAccessiblePromptAndActions(t *testing.T) {
	t.Parallel()

	confirmation := newTestConfirmation()

	require.Equal(t, []string{
		"Discard changes?",
		"Closing now loses edits.",
		"action: Discard",
		"action: Keep editing",
	}, confirmation.AccessibleLines())
}

func newTestConfirmation() *Confirmation {
	return NewConfirmation(
		ConfirmationDeclaration{
			ID:      "discard",
			Title:   "Discard changes?",
			Message: "Closing now loses edits.",
			Confirm: chrome.FormAction{ID: "discard", Label: "Discard"},
			Cancel:  chrome.FormAction{ID: keepEditing, Label: "Keep editing"},
		},
		ConfirmationStyles{
			Title:   lipgloss.NewStyle().Bold(true),
			Message: lipgloss.NewStyle(),
			Actions: chrome.FormStyles{
				Action:         lipgloss.NewStyle().Padding(0, 1),
				SelectedAction: lipgloss.NewStyle().Reverse(true).Padding(0, 1),
			},
		},
	)
}
