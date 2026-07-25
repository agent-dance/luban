package app

import (
	"context"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

const signalTestTimeout = 2 * time.Second

func TestSignalHandlerInterruptCancelsQueryBeforeGlobalContext(t *testing.T) {
	globalCtx, globalCancel := context.WithCancel(context.Background())
	var globalOnce sync.Once
	globalCalled := make(chan struct{})
	handler := NewSignalHandler(func() {
		globalOnce.Do(func() { close(globalCalled) })
		globalCancel()
	})
	sigCh := make(chan os.Signal, 2)
	handler.Start(globalCtx, sigCh)
	t.Cleanup(handler.StopAndWait)

	queryCalled := make(chan struct{})
	var queryOnce sync.Once
	handler.SetQueryCancel(func() { queryOnce.Do(func() { close(queryCalled) }) })
	sigCh <- os.Interrupt
	waitSignalTestChannel(t, queryCalled, "query cancellation")
	select {
	case <-globalCalled:
		t.Fatal("SIGINT during a query cancelled the global context")
	default:
	}

	handler.ClearQueryCancel()
	sigCh <- os.Interrupt
	waitSignalTestChannel(t, globalCalled, "global cancellation")
	waitSignalTestChannel(t, handler.doneSignal(), "listener shutdown")
}

func TestSignalHandlerTerminateAlwaysCancelsGlobalContext(t *testing.T) {
	globalCalled := make(chan struct{})
	var globalOnce sync.Once
	handler := NewSignalHandler(func() { globalOnce.Do(func() { close(globalCalled) }) })
	sigCh := make(chan os.Signal, 1)
	handler.Start(context.Background(), sigCh)
	t.Cleanup(handler.StopAndWait)

	queryCalled := make(chan struct{})
	handler.SetQueryCancel(func() { close(queryCalled) })
	sigCh <- syscall.SIGTERM
	waitSignalTestChannel(t, globalCalled, "global cancellation")
	waitSignalTestChannel(t, handler.doneSignal(), "listener shutdown")
	select {
	case <-queryCalled:
		t.Fatal("SIGTERM used the query cancellation path")
	default:
	}
}

func TestSignalHandlerOwnerStopIsIdempotentAndWaitable(t *testing.T) {
	handler := NewSignalHandler(nil)
	sigCh := make(chan os.Signal)
	handler.Start(context.Background(), sigCh)

	handler.StopAndWait()
	handler.StopAndWait()
	waitSignalTestChannel(t, handler.doneSignal(), "listener shutdown")
}

func TestSignalHandlerListenerStopsAtContextBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	handler := NewSignalHandler(nil)
	handler.Start(ctx, make(chan os.Signal))
	cancel()
	waitSignalTestChannel(t, handler.doneSignal(), "context-bound listener shutdown")
}

func TestSignalHandlerListenerStopsWhenSignalChannelCloses(t *testing.T) {
	handler := NewSignalHandler(nil)
	sigCh := make(chan os.Signal)
	handler.Start(context.Background(), sigCh)
	close(sigCh)
	waitSignalTestChannel(t, handler.doneSignal(), "closed-channel listener shutdown")
}

func waitSignalTestChannel(t *testing.T, ch <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(signalTestTimeout):
		t.Fatalf("timed out waiting for %s", description)
	}
}
