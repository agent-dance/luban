package tools

import (
	"sync"
	"testing"
	"time"
)

func TestPermissionModeTransitionsPublishDispatcherAndScopeInOrder(t *testing.T) {
	scope := NewRuntimeScope(t.TempDir(), true)
	var mu sync.Mutex
	actual := "default"
	autoEntered := make(chan struct{})
	releaseAuto := make(chan struct{})
	scope.SetPermissionModeDispatcher(func() string {
		mu.Lock()
		defer mu.Unlock()
		return actual
	}, func(mode string) error {
		mu.Lock()
		actual = mode
		mu.Unlock()
		if mode == "bypassPermissions" {
			close(autoEntered)
			<-releaseAuto
		}
		return nil
	})
	var observed []string
	scope.SetPermissionModeObserver(func(mode string) {
		mu.Lock()
		observed = append(observed, mode)
		mu.Unlock()
	})
	mu.Lock()
	observed = nil
	mu.Unlock()

	firstDone := make(chan error, 1)
	go func() { firstDone <- scope.TransitionPermissionMode("bypassPermissions") }()
	<-autoEntered
	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		secondDone <- scope.TransitionPermissionMode("plan")
	}()
	<-secondStarted
	select {
	case err := <-secondDone:
		t.Fatalf("second transition overtook blocked first transition: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseAuto)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if actual != "plan" || scope.PermissionMode() != "plan" {
		t.Fatalf("permission owners diverged: dispatcher=%q scope=%q", actual, scope.PermissionMode())
	}
	if len(observed) != 2 || observed[0] != "bypassPermissions" || observed[1] != "plan" {
		t.Fatalf("observer publication order = %#v", observed)
	}
}
