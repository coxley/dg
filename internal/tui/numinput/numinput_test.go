package numinput

import (
	"math"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

func TestModelStepsWithinExplicitLimit(t *testing.T) {
	t.Parallel()

	var value uint8
	model := New("Cost", &value, uint8(1), testStyles())
	next, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	require.Same(t, model, next)
	require.NotNil(t, command)
	require.Zero(t, value)
	require.Equal(t, -1, model.Flash())

	next, command = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	require.Same(t, model, next)
	require.NotNil(t, command)
	require.Equal(t, uint8(1), value)
	require.Equal(t, 1, model.Flash())

	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	require.Equal(t, uint8(1), value)

	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	require.Zero(t, value)
	_, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	require.Equal(t, uint8(1), value)
}

func TestModelIgnoresStaleFlash(t *testing.T) {
	t.Parallel()

	value := uint32(1)
	model := New("Cost", &value, uint32(10), testStyles())
	_, first := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	_, second := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))

	require.True(t, model.HandleFlash(first().(FlashExpiredMsg)))
	require.Equal(t, 1, model.Flash())
	require.True(t, model.HandleFlash(second().(FlashExpiredMsg)))
	require.Zero(t, model.Flash())
}

func TestModelImplementsTeaModel(t *testing.T) {
	t.Parallel()

	value := 1
	var model tea.Model = New("Cost", &value, 10, testStyles())
	require.Contains(t, model.View().Content, "Cost")
}

func TestModelClampsInitialValue(t *testing.T) {
	t.Parallel()

	value := 12
	model := New("Cost", &value, 10, testStyles())
	require.Equal(t, 10, value)
	require.Contains(t, model.View().Content, "10")

	negative := -1
	_ = New("Cost", &negative, 10, testStyles())
	require.Zero(t, negative)
}

func TestModelRendersFullUint64Range(t *testing.T) {
	t.Parallel()

	value := uint64(math.MaxUint64)
	model := New("Cost", &value, uint64(math.MaxUint64), testStyles())
	require.Contains(t, model.View().Content, "18446744073709551615")
}

func testStyles() Styles {
	return Styles{
		Title:        lipgloss.NewStyle(),
		FocusedTitle: lipgloss.NewStyle().Bold(true),
		Button:       lipgloss.NewStyle(),
		ActiveButton: lipgloss.NewStyle().Bold(true),
	}
}
