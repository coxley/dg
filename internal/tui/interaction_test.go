package tui

import (
	"testing"

	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestInteractionStateProjectsToolAndSessionModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		state    interactionState
		want     mode
		wantIdle bool
	}{
		{
			name:     "idle",
			want:     modeNavigate,
			wantIdle: true,
		},
		{
			name:  "rectangle tool",
			state: interactionState{tool: toolRectangle},
			want:  modeRectangle,
		},
		{
			name:  "connection tool",
			state: interactionState{tool: toolConnect},
			want:  modeConnect,
		},
		{
			name: "label session",
			state: interactionState{
				tool:    toolRectangle,
				session: interactionSession{kind: sessionLabelEdit},
			},
			want: modeEditLabel,
		},
		{
			name: "connection session",
			state: interactionState{
				session: interactionSession{kind: sessionConnection},
			},
			want: modeConnect,
		},
		{
			name: "gesture does not become a session",
			state: interactionState{
				gesture: pointerGesture{kind: gestureAreaSelection},
			},
			want: modeNavigate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, test.state.mode())
			require.Equal(t, test.wantIdle, test.state.idle())
		})
	}
}

func TestInteractionStateResetsOnlyPointerGesture(t *testing.T) {
	t.Parallel()

	state := interactionState{
		tool: toolConnect,
		session: interactionSession{
			kind: sessionConnection,
			connection: connectionSession{
				source: 7,
			},
		},
		gesture: pointerGesture{
			kind:  gestureConnection,
			start: layout.NewPoint(3, 4),
		},
		click: clickTracker{
			point: layout.NewPoint(1, 2),
			valid: true,
		},
		render: interactionRenderCache{
			connectionPreview: []layout.Point{layout.NewPoint(5, 6)},
		},
		transaction: interactionTransaction{owner: transactionConnection},
	}

	state.resetGesture()

	require.Equal(t, pointerGesture{}, state.gesture)
	require.Equal(t, toolConnect, state.tool)
	require.Equal(t, sessionConnection, state.session.kind)
	require.Equal(t, uint32(7), state.session.connection.source)
	require.True(t, state.click.valid)
	require.Len(t, state.render.connectionPreview, 1)
	require.True(t, state.transaction.open())
}
