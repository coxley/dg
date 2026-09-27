package clipboard

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileURI(t *testing.T) {
	t.Parallel()

	path := []byte("/tmp/diagram with # and %.dg")
	uri, err := encodeFileURI(path)
	require.NoError(t, err)
	require.Equal(t, "file:///tmp/diagram%20with%20%23%20and%20%25.dg", string(uri))

	decoded, err := decodeFileURI(uri)
	require.NoError(t, err)
	require.Equal(t, path, decoded)
}

func TestFileURIList(t *testing.T) {
	t.Parallel()

	encoded, err := encodeFileURIList([]byte("/tmp/canvas.dg"))
	require.NoError(t, err)
	require.Equal(t, "file:///tmp/canvas.dg\r\n", string(encoded))

	decoded, err := decodeFileURIList([]byte("# copied file\r\n\r\nfile:///tmp/canvas.dg\r\n"))
	require.NoError(t, err)
	require.Equal(t, []byte("/tmp/canvas.dg"), decoded)
}

func TestFileURIRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	_, err := encodeFileURI(nil)
	require.Error(t, err)
	_, err = encodeFileURI([]byte{'/', 'a', 0, 'b'})
	require.Error(t, err)
	_, err = decodeFileURI([]byte("https://example.com/file"))
	require.Error(t, err)
	_, err = decodeFileURIList([]byte("# no files\r\n"))
	require.Error(t, err)
}

func TestFileFormatIsBuiltIn(t *testing.T) {
	t.Parallel()

	require.Equal(t, fileURIListMIME, FmtFile.MIME())
	require.Equal(
		t,
		[]Format{FmtText, FmtImage, FmtFile},
		normalizeFormats([]Format{FmtFile, FmtImage, FmtText, FmtFile}),
	)
}
