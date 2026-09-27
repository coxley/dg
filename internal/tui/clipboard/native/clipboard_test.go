package clipboard

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareWriteManyOwnsDistinctFormats(t *testing.T) {
	t.Parallel()

	custom := Register("application/vnd.dg.fragment+json")
	text := []byte("diagram")
	fragment := []byte(`{"version":1}`)
	values, err := prepareWriteMany([]Data{
		{Format: FmtText, Bytes: text},
		{Format: FmtFile, Bytes: []byte("/tmp/diagram.dg")},
		{Format: custom, Bytes: fragment},
	})
	require.NoError(t, err)
	text[0] = 'X'
	fragment[0] = 'X'
	require.Equal(t, []byte("diagram"), values[0].Bytes)
	require.Equal(t, []byte("/tmp/diagram.dg"), values[1].Bytes)
	require.Equal(t, []byte(`{"version":1}`), values[2].Bytes)
}

func TestPrepareWriteManyRejectsInvalidFormats(t *testing.T) {
	t.Parallel()

	_, err := prepareWriteMany(nil)
	require.Error(t, err)
	_, err = prepareWriteMany([]Data{
		{Format: FmtText, Bytes: []byte("one")},
		{Format: FmtText, Bytes: []byte("two")},
	})
	require.ErrorContains(t, err, "appears more than once")
	_, err = prepareWriteMany([]Data{{Format: Format(1 << 20)}})
	require.ErrorIs(t, err, errUnsupported)
}
