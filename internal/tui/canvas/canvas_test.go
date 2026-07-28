package canvas

import (
	"testing"

	"github.com/coxley/dg/layout"
	"github.com/stretchr/testify/require"
)

func TestModelRetainsAndClearsFrames(t *testing.T) {
	t.Parallel()

	geo, err := layout.New()
	require.NoError(t, err)
	_, err = geo.NewNodeAt("node", layout.NewPoint(2, 2))
	require.NoError(t, err)
	require.NoError(t, geo.Build())

	var model Model
	require.NoError(t, model.Render(BaseFrame, geo))
	require.NotEmpty(t, model.Frame(BaseFrame).Text)
	require.NotEmpty(t, model.Rows(BaseFrame))

	model.Clear(BaseFrame)
	require.Empty(t, model.Frame(BaseFrame).Text)
	require.Empty(t, model.Rows(BaseFrame))
}
