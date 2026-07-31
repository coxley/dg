package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

const panicFlushTimeout = 2 * time.Second

type flushRequestMsg struct {
	done chan<- error
	quit bool
}

type messageSender interface {
	Send(tea.Msg)
}

// RequestFlush asks a running program's owner event loop to persist the active
// document and history. A terminated program cannot acknowledge the request.
func RequestFlush(program *tea.Program) <-chan error {
	if program == nil {
		done := make(chan error, 1)
		done <- errors.New("flush nil TUI program")
		close(done)
		return done
	}
	return requestProgramFlush(program, false)
}

func requestProgramFlush(program messageSender, quit bool) <-chan error {
	done := make(chan error, 1)
	go program.Send(flushRequestMsg{done: done, quit: quit})
	return done
}

func (m *Model) handleFlushRequest(request flushRequestMsg) tea.Cmd {
	err := m.flushActive()
	if request.quit {
		m.exitErr = errors.Join(m.exitErr, err)
	}
	if request.done != nil {
		request.done <- err
		close(request.done)
	}
	if request.quit {
		return tea.Quit
	}
	return nil
}

func forwardSignals(
	signals <-chan os.Signal,
	stopped <-chan struct{},
	program messageSender,
	exit func(int),
) {
	seen := false
	for {
		select {
		case <-stopped:
			return
		case signal := <-signals:
			if seen {
				exit(signalExitCode(signal))
				return
			}
			seen = true
			requestProgramFlush(program, true)
		}
	}
}

func signalExitCode(signal os.Signal) int {
	switch signal {
	case os.Interrupt:
		return 130
	case syscall.SIGTERM:
		return 143
	default:
		return 1
	}
}

func flushAfterPanic(model *Model, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- fmt.Errorf("panic during cleanup: %v", recovered)
			}
		}()
		done <- model.flushActive()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("panic cleanup exceeded %s", timeout)
	}
}

func reportPanicCleanup(writer io.Writer, err error) {
	if err != nil {
		_, _ = fmt.Fprintf(writer, "panic cleanup: %v\n", err)
	}
}

func (m *Model) retainPanic() {
	if recovered := recover(); recovered != nil {
		m.panicValue = recovered
		panic(recovered)
	}
}
