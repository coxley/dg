package tui

import (
	"context"
	"errors"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

// ErrDevReload reports a successful development session handoff.
var ErrDevReload = errors.New("development reload requested")

type devReloadState struct {
	markerPath  string
	sessionPath string
	feed        <-chan devReloadMsg
	cancel      context.CancelFunc
	requested   bool
}

type devReloadMsg struct {
	err    error
	closed bool
}

func (m *Model) startDevReloadWatch() tea.Cmd {
	if m.devReload.markerPath == "" || m.devReload.feed != nil {
		return waitDevReload(m.devReload.feed)
	}
	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // The TUI model owns this process-lifetime context.
	feed, err := watchDevReload(ctx, m.devReload.markerPath)
	if err != nil {
		cancel()
		return func() tea.Msg { return devReloadMsg{err: err, closed: true} }
	}
	m.devReload.cancel = cancel
	m.devReload.feed = feed
	return waitDevReload(feed)
}

func watchDevReload(ctx context.Context, markerPath string) (<-chan devReloadMsg, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(filepath.Dir(markerPath)); err != nil {
		_ = watcher.Close()
		return nil, err
	}
	events := make(chan devReloadMsg)
	go func() {
		defer close(events)
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				if !sendDevReload(ctx, events, devReloadMsg{err: err}) {
					return
				}
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Clean(event.Name) != filepath.Clean(markerPath) ||
					event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
					continue
				}
				sendDevReload(ctx, events, devReloadMsg{})
				return
			}
		}
	}()
	return events, nil
}

func sendDevReload(ctx context.Context, events chan<- devReloadMsg, event devReloadMsg) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func waitDevReload(events <-chan devReloadMsg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return devReloadMsg{closed: true}
		}
		return event
	}
}

func (m *Model) updateDevReload(message tea.Msg) (tea.Cmd, bool) {
	event, ok := message.(devReloadMsg)
	if !ok {
		return nil, false
	}
	if event.err != nil {
		m.setError("watch development reload: " + event.err.Error())
		return waitDevReload(m.devReload.feed), true
	}
	if event.closed {
		return nil, true
	}
	if err := m.writeDevSession(m.devReload.sessionPath); err != nil {
		m.setError(err.Error())
		return nil, true
	}
	m.devReload.requested = true
	return tea.Quit, true
}
