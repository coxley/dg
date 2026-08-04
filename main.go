package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/coxley/dg/document"
	"github.com/coxley/dg/history"
	"github.com/coxley/dg/internal/settings"
	"github.com/coxley/dg/internal/tui"
	"github.com/coxley/dg/layout"
	canvasstore "github.com/coxley/dg/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, tui.ErrDevReload) && os.Getenv(devChildEnv) == "1" {
			os.Exit(devReloadExitCode)
		}
		var childExit *devChildExitError
		if errors.As(err, &childExit) {
			os.Exit(childExit.code)
		}
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "dev" {
		return runDev(args[1:])
	}
	return runEditor(args)
}

func runEditor(args []string) error {
	store, err := settings.DefaultStore()
	if err != nil {
		return fmt.Errorf("configure settings: %w", err)
	}
	snapshot, err := store.Load()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	preferred := snapshot.SaveDirectory
	if preferred == "" {
		preferred = "."
	}
	canvases, err := canvasstore.New(preferred)
	if err != nil {
		return fmt.Errorf("configure canvas store: %w", err)
	}
	devConfig, development := devChildConfigFromEnv()
	var devSession *tui.DevSession
	if development {
		session, found, err := tui.ConsumeDevSession(devConfig.sessionPath)
		if err != nil {
			return fmt.Errorf("restore development session: %w", err)
		}
		if found {
			devSession = &session
		}
	}
	geo, doc, entry, err := initialEditorCanvas(args, snapshot, canvases, devSession)
	if err != nil {
		return err
	}
	undo, err := history.New(geo, history.WithStore(canvases.History()))
	if err != nil {
		return fmt.Errorf("configure history: %w", err)
	}
	if _, err := undo.Restore(doc); err != nil {
		log.Printf("restore undo history: %v", err)
	}
	modelOptions := []tui.Option{
		tui.WithDocument(doc),
		tui.WithHistory(undo),
		tui.WithCanvasStore(canvases, entry),
		tui.WithSettings(snapshot, store),
	}
	if devSession != nil {
		modelOptions = append(modelOptions, tui.WithDevSession(*devSession))
	}
	if development {
		modelOptions = append(modelOptions, tui.WithDevReload(
			devConfig.markerPath,
			devConfig.sessionPath,
		))
	}
	if err := tui.Run(geo, modelOptions...); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}
	return nil
}

func initialEditorCanvas(
	args []string,
	snapshot settings.Snapshot,
	canvases *canvasstore.Store,
	session *tui.DevSession,
) (*layout.Layout, document.Document, *canvasstore.Entry, error) {
	if session == nil {
		return initialCanvas(args, snapshot, canvases)
	}
	geo, err := session.Document.Convert()
	if err != nil {
		return nil, document.Document{}, nil, fmt.Errorf("convert development session: %w", err)
	}
	entries, err := canvases.List()
	if err != nil {
		return nil, document.Document{}, nil, fmt.Errorf("list development canvases: %w", err)
	}
	for _, entry := range entries {
		if entry.ID == session.EntryID {
			return geo, session.Document, &entry, nil
		}
	}
	return geo, session.Document, nil, nil
}

func initialCanvas(
	args []string,
	snapshot settings.Snapshot,
	canvases *canvasstore.Store,
) (*layout.Layout, document.Document, *canvasstore.Entry, error) {
	switch len(args) {
	case 0:
		entries, err := canvases.List()
		if err != nil {
			return nil, document.Document{}, nil, fmt.Errorf("list canvases: %w", err)
		}
		if len(entries) != 0 {
			entry := slices.MaxFunc(entries, func(a, b canvasstore.Entry) int {
				return a.Modified.Compare(b.Modified)
			})
			doc, err := canvases.Load(entry)
			if err != nil {
				return nil, document.Document{}, nil, fmt.Errorf("load recent canvas: %w", err)
			}
			geo, err := doc.Convert()
			if err != nil {
				return nil, document.Document{}, nil, fmt.Errorf("convert recent canvas: %w", err)
			}
			return geo, doc, &entry, nil
		}
		geo, err := emptyLayoutWithSettings(snapshot)
		if err != nil {
			return nil, document.Document{}, nil, err
		}
		return geo, document.New(geo), nil, nil
	case 1:
		entry, doc, err := canvases.Import(args[0])
		if err != nil {
			return nil, document.Document{}, nil, fmt.Errorf("import diagram %q: %w", args[0], err)
		}
		geo, err := doc.Convert()
		if err != nil {
			return nil, document.Document{}, nil, fmt.Errorf("load diagram %q: %w", args[0], err)
		}
		return geo, doc, &entry, nil
	default:
		return nil, document.Document{}, nil, errors.New("usage: dg [path] | dg dev [path]")
	}
}

func emptyLayoutWithSettings(snapshot settings.Snapshot) (*layout.Layout, error) {
	var options []layout.Option
	if snapshot.ApplyToFuture {
		options = append(options, layout.WithRouter(snapshot.Router))
	}
	return layout.New(options...)
}
