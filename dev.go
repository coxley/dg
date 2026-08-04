package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	devChildEnv       = "DG_DEV_CHILD"
	devMarkerEnv      = "DG_DEV_MARKER"
	devSessionEnv     = "DG_DEV_SESSION"
	devReloadExitCode = 75
	devDebounce       = 200 * time.Millisecond
)

type devChildConfig struct {
	markerPath  string
	sessionPath string
}

type devChildExitError struct {
	code int
}

func (e *devChildExitError) Error() string {
	return fmt.Sprintf("development editor exited with status %d", e.code)
}

type devSourceEvent struct {
	err error
}

func devChildConfigFromEnv() (devChildConfig, bool) {
	config := devChildConfig{
		markerPath:  os.Getenv(devMarkerEnv),
		sessionPath: os.Getenv(devSessionEnv),
	}
	return config, os.Getenv(devChildEnv) == "1" &&
		config.markerPath != "" && config.sessionPath != ""
}

func runDev(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: dg dev [path]")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("find working directory: %w", err)
	}
	moduleRoot, err := findModuleRoot(workingDirectory)
	if err != nil {
		return err
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("find development cache: %w", err)
	}
	cacheRoot = filepath.Join(cacheRoot, "dg", "dev")
	if err := os.MkdirAll(cacheRoot, 0o700); err != nil {
		return fmt.Errorf("create development cache: %w", err)
	}
	workspace, err := os.MkdirTemp(cacheRoot, "session-")
	if err != nil {
		return fmt.Errorf("create development workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sources, err := watchDevSources(ctx, moduleRoot)
	if err != nil {
		return fmt.Errorf("watch development sources: %w", err)
	}
	binaryPath := filepath.Join(workspace, "dg")
	output, err := buildDevBinary(ctx, moduleRoot, binaryPath)
	if err != nil {
		return formatDevBuildError(err, output)
	}
	config := devChildConfig{
		markerPath:  filepath.Join(workspace, "reload"),
		sessionPath: filepath.Join(workspace, "session.json.gz"),
	}
	childDone, err := startDevChild(ctx, binaryPath, args, config)
	if err != nil {
		return err
	}

	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	var (
		debounce   <-chan time.Time
		generation uint64
	)
	for {
		select {
		case event, ok := <-sources:
			if !ok {
				return errors.New("development source watcher closed")
			}
			if event.err != nil {
				return fmt.Errorf("watch development sources: %w", event.err)
			}
			resetTimer(timer, devDebounce)
			debounce = timer.C
		case <-debounce:
			debounce = nil
			_, err := buildDevBinary(ctx, moduleRoot, binaryPath)
			if err != nil {
				// Build failures leave the current editor running. Surface diagnostics
				// here if development reloads need feedback later.
				continue
			}
			generation++
			if err := os.WriteFile(
				config.markerPath,
				[]byte(strconv.FormatUint(generation, 10)),
				0o600,
			); err != nil {
				return fmt.Errorf("signal development reload: %w", err)
			}
		case err := <-childDone:
			if !isDevReloadExit(err) {
				return devChildResult(err)
			}
			if err := removeIfExists(config.markerPath); err != nil {
				return fmt.Errorf("reset development marker: %w", err)
			}
			childDone, err = startDevChild(ctx, binaryPath, args, config)
			if err != nil {
				return err
			}
		}
	}
}

func findModuleRoot(start string) (string, error) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	for {
		_, err := os.Stat(filepath.Join(directory, "go.mod"))
		if err == nil {
			return directory, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect module root: %w", err)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("dg dev requires a Go module")
		}
		directory = parent
	}
}

func buildDevBinary(ctx context.Context, moduleRoot, binaryPath string) ([]byte, error) {
	nextPath := binaryPath + ".next"
	if err := removeIfExists(nextPath); err != nil {
		return nil, fmt.Errorf("prepare development build: %w", err)
	}
	command := exec.CommandContext(ctx, "go", "build", "-o", nextPath, ".")
	command.Dir = moduleRoot
	output, err := command.CombinedOutput()
	if err != nil {
		_ = removeIfExists(nextPath)
		return output, err
	}
	if err := os.Rename(nextPath, binaryPath); err != nil {
		_ = removeIfExists(nextPath)
		return output, fmt.Errorf("activate development build: %w", err)
	}
	return output, nil
}

func formatDevBuildError(err error, output []byte) error {
	diagnostics := strings.TrimSpace(string(output))
	if diagnostics == "" {
		return fmt.Errorf("build development editor: %w", err)
	}
	return fmt.Errorf("build development editor: %w\n%s", err, diagnostics)
}

func startDevChild(
	ctx context.Context,
	binaryPath string,
	args []string,
	config devChildConfig,
) (<-chan error, error) {
	// The supervisor chooses binaryPath; args remain argv entries without shell expansion.
	command := exec.CommandContext(ctx, binaryPath, args...) //nolint:gosec
	command.Env = devChildEnvironment(os.Environ(), config)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start development editor: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	return done, nil
}

func devChildEnvironment(environment []string, config devChildConfig) []string {
	result := make([]string, 0, len(environment)+3)
	for _, value := range environment {
		name, _, _ := strings.Cut(value, "=")
		if name == devChildEnv || name == devMarkerEnv || name == devSessionEnv {
			continue
		}
		result = append(result, value)
	}
	return append(
		result,
		devChildEnv+"=1",
		devMarkerEnv+"="+config.markerPath,
		devSessionEnv+"="+config.sessionPath,
	)
}

func isDevReloadExit(err error) bool {
	var exitError *exec.ExitError
	return errors.As(err, &exitError) && exitError.ExitCode() == devReloadExitCode
}

func devChildResult(err error) error {
	if err == nil {
		return nil
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return fmt.Errorf("wait for development editor: %w", err)
	}
	code := exitError.ExitCode()
	if code < 1 {
		code = 1
	}
	return &devChildExitError{code: code}
}

func watchDevSources(ctx context.Context, moduleRoot string) (<-chan devSourceEvent, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := addDevSourceTree(watcher, moduleRoot); err != nil {
		_ = watcher.Close()
		return nil, err
	}
	events := make(chan devSourceEvent)
	go func() {
		defer close(events)
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-watcher.Errors:
				if !ok || !sendDevSourceEvent(ctx, events, devSourceEvent{err: err}) {
					return
				}
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				addedDirectory, err := addCreatedDevDirectory(watcher, event)
				if err != nil && !sendDevSourceEvent(ctx, events, devSourceEvent{err: err}) {
					return
				}
				if addedDirectory {
					if !sendDevSourceEvent(ctx, events, devSourceEvent{}) {
						return
					}
					continue
				}
				if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 ||
					!isDevSource(moduleRoot, event.Name) {
					continue
				}
				if !sendDevSourceEvent(ctx, events, devSourceEvent{}) {
					return
				}
			}
		}
	}()
	return events, nil
}

func addCreatedDevDirectory(
	watcher *fsnotify.Watcher,
	event fsnotify.Event,
) (bool, error) {
	if event.Op&fsnotify.Create == 0 {
		return false, nil
	}
	info, err := os.Stat(event.Name)
	if err != nil || !info.IsDir() {
		return false, nil
	}
	return true, addDevSourceTree(watcher, event.Name)
}

func addDevSourceTree(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if err := watcher.Add(path); err != nil {
			return fmt.Errorf("watch %q: %w", path, err)
		}
		return nil
	})
}

func isDevSource(moduleRoot, path string) bool {
	path = filepath.Clean(path)
	if filepath.Ext(path) == ".go" {
		return true
	}
	return path == filepath.Join(moduleRoot, "go.mod") ||
		path == filepath.Join(moduleRoot, "go.sum")
}

func sendDevSourceEvent(
	ctx context.Context,
	events chan<- devSourceEvent,
	event devSourceEvent,
) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func resetTimer(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
