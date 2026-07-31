//go:build android || ios || (!darwin && !windows && !linux && !freebsd && !openbsd && !netbsd)

package clipboard

import "context"

func initialize() error {
	return errUnavailable
}

// enumerateFormats reports the formats on the clipboard. In a CGO-disabled
// build the clipboard is unavailable, so Formats() returns empty.
func enumerateFormats() []Format { return nil }

// read reports that native clipboard access is unavailable.
func read(t Format) (buf []byte, err error) {
	return nil, errUnavailable
}

func readc(t string) ([]byte, error) {
	return nil, errUnavailable
}

func writeMany([]Data) (<-chan struct{}, error) {
	return nil, errUnavailable
}

func watch(ctx context.Context, t Format) <-chan []byte {
	// The clipboard is unavailable in a CGO-disabled build. Return a
	// closed channel so that receivers observe completion immediately
	// instead of blocking forever, consistent with the documented
	// behavior when the given context is canceled.
	ch := make(chan []byte)
	close(ch)
	return ch
}
