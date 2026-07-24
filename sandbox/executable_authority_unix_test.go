//go:build linux || darwin

package sandbox

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

type unixAuthorityWorld struct {
	mu       sync.RWMutex
	path     string
	identity string
	trusted  bool
}

func (w *unixAuthorityWorld) set(path, identity string, trusted bool) {
	w.mu.Lock()
	w.path, w.identity, w.trusted = path, identity, trusted
	w.mu.Unlock()
}

func (w *unixAuthorityWorld) ops() executableAuthorityOps {
	return executableAuthorityOps{
		resolve: func() (string, error) {
			w.mu.RLock()
			defer w.mu.RUnlock()
			if w.path == "" {
				return "", errors.New("not found")
			}
			return w.path, nil
		},
		probe: func(path string) (trustedExecutable, error) {
			w.mu.RLock()
			defer w.mu.RUnlock()
			if path != w.path || !w.trusted {
				return trustedExecutable{}, errors.New("untrusted")
			}
			return trustedExecutable{path: path, identity: w.identity}, nil
		},
		launch: func(ctx context.Context, executable trustedExecutable, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, executable.path, args...)
		},
	}
}

func TestExecutableAuthorityImmutableIdentityAndReverseABA(t *testing.T) {
	world := &unixAuthorityWorld{}
	world.set("/usr/bin/test-sandbox", "inode-a", true)
	var authority executableAuthority
	authority.configure("test-sandbox", world.ops())
	initial, ok := authority.snapshot()
	if !ok || initial.ID() == "" {
		t.Fatal("trusted executable was not prepared")
	}

	world.set("/usr/bin/test-sandbox", "inode-b", true)
	if _, ok := authority.snapshot(); ok {
		t.Fatal("identity replacement retained authority")
	}
	world.set("/usr/bin/test-sandbox", "inode-a", true)
	if _, ok := authority.snapshot(); ok {
		t.Fatal("reverse ABA restored poisoned authority")
	}
	if _, err := authority.command(context.Background(), "--version"); err == nil {
		t.Fatal("poisoned authority constructed a command")
	}
}

func TestExecutableAuthorityConcurrentReplacementFailsClosed(t *testing.T) {
	world := &unixAuthorityWorld{}
	world.set("/usr/bin/test-sandbox", "inode-a", true)
	var authority executableAuthority
	authority.configure("test-sandbox", world.ops())
	if _, ok := authority.snapshot(); !ok {
		t.Fatal("trusted executable was not prepared")
	}
	world.set("/usr/bin/test-sandbox", "inode-b", true)

	const workers = 64
	start := make(chan struct{})
	results := make(chan bool, workers)
	var ready sync.WaitGroup
	ready.Add(workers)
	for range workers {
		go func() {
			ready.Done()
			<-start
			_, ok := authority.snapshot()
			results <- ok
		}()
	}
	ready.Wait()
	close(start)
	for range workers {
		if <-results {
			t.Fatal("replacement retained authority in concurrent snapshot")
		}
	}
}

func TestExecutableAuthorityRejectsRelativeAndUntrustedFirstCandidate(t *testing.T) {
	world := &unixAuthorityWorld{}
	world.set("bin/test-sandbox", "relative", true)
	var authority executableAuthority
	authority.configure("test-sandbox", world.ops())
	if _, ok := authority.snapshot(); ok {
		t.Fatal("relative executable became authority")
	}

	world.set("/home/user/bin/test-sandbox", "user-owned", false)
	if _, ok := authority.snapshot(); ok {
		t.Fatal("untrusted executable became authority")
	}
	world.set("/usr/bin/test-sandbox", "system", true)
	if _, ok := authority.snapshot(); !ok {
		t.Fatal("first trusted candidate could not prepare authority")
	}
}

func TestDefaultExecutableProbeRejectsUserWritableAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bwrap")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if executable, err := defaultExecutableProbe(path); err == nil {
		executable.close()
		t.Fatal("user-writable executable became sandbox authority")
	}
}
