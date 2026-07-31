package tui

import (
	"bytes"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/coxley/dg/document"
	"github.com/stretchr/testify/require"
)

func TestFlushRequestPersistsDocumentAndHistory(t *testing.T) {
	t.Parallel()

	model, nodeID, store := newStoredTestModel(t, "original")
	transaction := model.history.Begin()
	require.NoError(t, model.geo.SetNodeLabel(nodeID, "changed"))
	require.NoError(t, transaction.Commit())
	done := make(chan error, 1)
	command, handled := model.updatePersistence(flushRequestMsg{done: done})
	require.True(t, handled)
	require.Nil(t, command)
	require.NoError(t, <-done)
	require.False(t, model.history.Dirty())
	require.Equal(t, model.dirty, model.saved)
	loaded, err := store.Load(*model.entry)
	require.NoError(t, err)
	require.Equal(t, "changed", loaded.Nodes[nodeID].Label)
}

func TestRequestFlushMarshalsThroughRunningProgram(t *testing.T) {
	t.Parallel()

	model, nodeID, store := newStoredTestModel(t, "original")
	transaction := model.history.Begin()
	require.NoError(t, model.geo.SetNodeLabel(nodeID, "changed"))
	require.NoError(t, transaction.Commit())
	program := tea.NewProgram(
		model,
		tea.WithInput(nil),
		tea.WithoutRenderer(),
		tea.WithoutSignalHandler(),
	)
	runDone := make(chan error, 1)
	go func() {
		_, err := program.Run()
		runDone <- err
	}()
	select {
	case err := <-RequestFlush(program):
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "flush request was not acknowledged")
	}
	program.Quit()
	require.NoError(t, <-runDone)
	loaded, err := store.Load(*model.entry)
	require.NoError(t, err)
	require.Equal(t, "changed", loaded.Nodes[nodeID].Label)
}

func TestQuitFlushReportsFailureAndStillQuits(t *testing.T) {
	t.Parallel()

	model, store := newNamedStoredTestModel(t, "original")
	const nodeID = 0
	external := model.document
	external.Nodes = append([]document.Node(nil), model.document.Nodes...)
	external.Nodes[nodeID].Label = "external"
	replaceStoredDocument(t, store, *model.entry, external)
	transaction := model.history.Begin()
	require.NoError(t, model.geo.SetNodeLabel(nodeID, "local"))
	require.NoError(t, transaction.Commit())
	done := make(chan error, 1)
	command := model.handleFlushRequest(flushRequestMsg{done: done, quit: true})
	require.Error(t, <-done)
	require.Error(t, model.exitErr)
	_, ok := command().(tea.QuitMsg)
	require.True(t, ok)
}

func TestFirstSignalRequestsFlushAndSecondExitsImmediately(t *testing.T) {
	t.Parallel()

	signals := make(chan os.Signal, 2)
	stopped := make(chan struct{})
	sender := &blockingMessageSender{
		messages: make(chan tea.Msg, 1),
		release:  make(chan struct{}),
	}
	exits := make(chan int, 1)
	finished := make(chan struct{})
	go func() {
		forwardSignals(signals, stopped, sender, func(code int) { exits <- code })
		close(finished)
	}()

	signals <- os.Interrupt
	message := (<-sender.messages).(flushRequestMsg)
	require.True(t, message.quit)
	signals <- syscall.SIGTERM
	require.Equal(t, 143, <-exits)
	close(sender.release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		require.FailNow(t, "signal forwarder did not stop")
	}
}

func TestPanicCleanupFlushesAndReportsWithoutPanicking(t *testing.T) {
	t.Parallel()

	model, nodeID, store := newStoredTestModel(t, "original")
	transaction := model.history.Begin()
	require.NoError(t, model.geo.SetNodeLabel(nodeID, "changed"))
	require.NoError(t, transaction.Commit())
	require.NoError(t, flushAfterPanic(model, time.Second))
	loaded, err := store.Load(*model.entry)
	require.NoError(t, err)
	require.Equal(t, "changed", loaded.Nodes[nodeID].Label)

	var output bytes.Buffer
	reportPanicCleanup(&output, errors.New("cleanup failed"))
	require.Equal(t, "panic cleanup: cleanup failed\n", output.String())
}

func TestUpdateRetainsOriginalPanicValue(t *testing.T) {
	t.Parallel()

	model, _ := newTestModel(t)
	model.bindings = nil
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		model.Update(keyPress('x', "x"))
	}()
	require.NotNil(t, recovered)
	require.Equal(t, recovered, model.panicValue)
}

type blockingMessageSender struct {
	messages chan tea.Msg
	release  chan struct{}
}

func (s *blockingMessageSender) Send(message tea.Msg) {
	s.messages <- message
	<-s.release
}
