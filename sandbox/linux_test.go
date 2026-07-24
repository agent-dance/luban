//go:build linux

package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// TestBwrapArgs verifies bwrap argument generation without running bwrap.
func TestBwrapArgs(t *testing.T) {
	// BwrapBackend.Command requires bwrap to be on PATH; skip if unavailable.
	b := &BwrapBackend{}
	if !b.Available() {
		t.Skip("bwrap not available on this system")
	}

	ctx := context.Background()

	t.Run("name is bwrap", func(t *testing.T) {
		if b.Name() != "bwrap" {
			t.Errorf("Name() = %q, want %q", b.Name(), "bwrap")
		}
	})

	t.Run("new-session flag present", func(t *testing.T) {
		cfg := Config{ReadWritePaths: []string{"/tmp"}}
		cmd, err := b.Command(ctx, cfg, "echo", "hello")
		if err != nil {
			t.Fatalf("Command() error: %v", err)
		}
		args := strings.Join(cmd.Args, " ")
		if !strings.Contains(args, "--new-session") {
			t.Errorf("--new-session not found in bwrap args: %s", args)
		}
	})

	t.Run("die-with-parent flag present", func(t *testing.T) {
		cfg := Config{ReadWritePaths: []string{"/tmp"}}
		cmd, err := b.Command(ctx, cfg, "echo")
		if err != nil {
			t.Fatalf("Command() error: %v", err)
		}
		args := strings.Join(cmd.Args, " ")
		if !strings.Contains(args, "--die-with-parent") {
			t.Errorf("--die-with-parent not found in bwrap args: %s", args)
		}
	})

	t.Run("unshare-net when AllowedDomains empty", func(t *testing.T) {
		cfg := Config{AllowedDomains: []string{}}
		cmd, err := b.Command(ctx, cfg, "echo")
		if err != nil {
			t.Fatalf("Command() error: %v", err)
		}
		args := strings.Join(cmd.Args, " ")
		mustContain(t, args, "--unshare-net")
	})

	// F3: specific domains must NOT silently upgrade to allow-all; net stays unshared.
	t.Run("unshare-net for specific domains (phase 2)", func(t *testing.T) {
		cfg := Config{AllowedDomains: []string{"example.com"}}
		cmd, err := b.Command(ctx, cfg, "echo")
		if err != nil {
			t.Fatalf("Command() error: %v", err)
		}
		args := strings.Join(cmd.Args, " ")
		mustContain(t, args, "--unshare-net")
	})

	t.Run("no unshare-net with wildcard domain", func(t *testing.T) {
		cfg := Config{AllowedDomains: []string{"*"}}
		cmd, err := b.Command(ctx, cfg, "echo")
		if err != nil {
			t.Fatalf("Command() error: %v", err)
		}
		args := strings.Join(cmd.Args, " ")
		mustNotContain(t, args, "--unshare-net")
	})

	t.Run("read-write path bound", func(t *testing.T) {
		cfg := Config{ReadWritePaths: []string{"/tmp"}}
		cmd, err := b.Command(ctx, cfg, "echo")
		if err != nil {
			t.Fatalf("Command() error: %v", err)
		}
		args := strings.Join(cmd.Args, " ")
		mustContain(t, args, "--bind")
	})

	t.Run("host filesystem read-write bound before virtual filesystems", func(t *testing.T) {
		cmd, err := b.Command(ctx, Config{ReadWritePaths: []string{"/"}}, "echo")
		if err != nil {
			t.Fatalf("Command() error: %v", err)
		}
		args := strings.Join(cmd.Args, " ")
		mustContain(t, args, "--bind / /")
		mustNotContain(t, args, "--tmpfs /tmp")
	})

	t.Run("read-only path ro-bound", func(t *testing.T) {
		cfg := Config{ReadOnlyPaths: []string{"/tmp"}}
		cmd, err := b.Command(ctx, cfg, "echo")
		if err != nil {
			t.Fatalf("Command() error: %v", err)
		}
		args := strings.Join(cmd.Args, " ")
		mustContain(t, args, "--ro-bind")
	})

	t.Run("workdir chdir set", func(t *testing.T) {
		cfg := Config{WorkDir: "/tmp"}
		cmd, err := b.Command(ctx, cfg, "echo")
		if err != nil {
			t.Fatalf("Command() error: %v", err)
		}
		args := strings.Join(cmd.Args, " ")
		mustContain(t, args, "--chdir /tmp")
		if cmd.Dir != "/tmp" {
			t.Errorf("cmd.Dir = %q, want /tmp", cmd.Dir)
		}
	})
}

// TestBwrapValidation verifies that Command() rejects invalid paths.
func TestBwrapValidation(t *testing.T) {
	b := &BwrapBackend{}
	if !b.Available() {
		t.Skip("bwrap not available on this system")
	}

	ctx := context.Background()

	t.Run("relative ReadWritePath rejected", func(t *testing.T) {
		cfg := Config{ReadWritePaths: []string{"relative/path"}}
		_, err := b.Command(ctx, cfg, "echo")
		if err == nil {
			t.Error("expected error for relative ReadWritePath, got nil")
		}
	})

	t.Run("path with newline rejected", func(t *testing.T) {
		cfg := Config{ReadWritePaths: []string{"/tmp/foo\nbar"}}
		_, err := b.Command(ctx, cfg, "echo")
		if err == nil {
			t.Error("expected error for path containing newline, got nil")
		}
	})
}

type fakeExecutableRecord struct {
	resolved string
	identity string
	trusted  bool
}

type fakeExecutableWorld struct {
	mu      sync.RWMutex
	lookup  string
	records map[string]fakeExecutableRecord
}

func (w *fakeExecutableWorld) setLookup(path string) {
	w.mu.Lock()
	w.lookup = path
	w.mu.Unlock()
}

func (w *fakeExecutableWorld) setRecord(path string, record fakeExecutableRecord) {
	w.mu.Lock()
	if w.records == nil {
		w.records = make(map[string]fakeExecutableRecord)
	}
	w.records[path] = record
	w.mu.Unlock()
}

func (w *fakeExecutableWorld) ops() *executableAuthorityOps {
	return &executableAuthorityOps{
		resolve: func() (string, error) {
			w.mu.RLock()
			defer w.mu.RUnlock()
			if w.lookup == "" {
				return "", errors.New("not found")
			}
			return w.lookup, nil
		},
		probe: func(path string) (trustedExecutable, error) {
			w.mu.RLock()
			record, ok := w.records[path]
			w.mu.RUnlock()
			if !ok {
				return trustedExecutable{}, errors.New("not found")
			}
			if !record.trusted {
				return trustedExecutable{}, errors.New("writable authority")
			}
			return trustedExecutable{path: record.resolved, identity: record.identity}, nil
		},
		launch: func(ctx context.Context, executable trustedExecutable, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, executable.path, args...)
		},
	}
}

func newFakeBwrap(world *fakeExecutableWorld) *BwrapBackend {
	return &BwrapBackend{authority: &executableAuthority{}, authorityOps: world.ops()}
}

func TestBwrapCapabilityRejectsWritablePATHPredecessorBeforeTrustedPreparation(t *testing.T) {
	world := &fakeExecutableWorld{}
	world.setRecord("/home/user/bin/bwrap", fakeExecutableRecord{
		resolved: "/home/user/bin/bwrap", identity: "fake-1", trusted: false,
	})
	world.setRecord("/usr/bin/bwrap", fakeExecutableRecord{
		resolved: "/usr/bin/bwrap", identity: "system-1", trusted: true,
	})
	world.setLookup("/home/user/bin/bwrap")
	backend := newFakeBwrap(world)
	if backend.Available() {
		t.Fatal("writable PATH predecessor became sandbox authority")
	}

	// A failed untrusted discovery does not prevent a later first trusted
	// preparation. Only a successfully prepared capability is immutable.
	world.setLookup("/usr/bin/bwrap")
	capability, ok := backend.SandboxCapability()
	if !ok || capability.ExecutablePath != "/usr/bin/bwrap" || capability.ID() == "" {
		t.Fatalf("trusted capability = %#v, ok=%v", capability, ok)
	}
}

func TestBwrapCapabilityRejectsRelativeExecutable(t *testing.T) {
	world := &fakeExecutableWorld{}
	world.setLookup("bin/bwrap")
	world.setRecord("bin/bwrap", fakeExecutableRecord{resolved: "bin/bwrap", identity: "relative", trusted: true})
	if newFakeBwrap(world).Available() {
		t.Fatal("relative bwrap executable became sandbox authority")
	}
}

func TestBwrapCapabilityPATHReplacementPoisonsReverseABA(t *testing.T) {
	world := &fakeExecutableWorld{}
	world.setRecord("/usr/bin/bwrap", fakeExecutableRecord{
		resolved: "/usr/bin/bwrap", identity: "system-1", trusted: true,
	})
	world.setRecord("/tmp/bin/bwrap", fakeExecutableRecord{
		resolved: "/tmp/bin/bwrap", identity: "fake-1", trusted: true,
	})
	world.setLookup("/usr/bin/bwrap")
	backend := newFakeBwrap(world)
	original, ok := backend.SandboxCapability()
	if !ok {
		t.Fatal("trusted initial capability unavailable")
	}

	world.setLookup("/tmp/bin/bwrap")
	if backend.Available() {
		t.Fatal("PATH replacement retained sandbox authority")
	}
	world.setLookup("/usr/bin/bwrap")
	if capability, ok := backend.SandboxCapability(); ok || capability.ID() != "" {
		t.Fatalf("reverse ABA resurrected capability: %#v, original=%#v", capability, original)
	}
	if _, err := backend.Command(context.Background(), Config{}, "echo"); err == nil {
		t.Fatal("poisoned PATH authority constructed a command")
	}
}

func TestBwrapCapabilitySamePathIdentityReplacementPoisonsReverseABA(t *testing.T) {
	const path = "/usr/bin/bwrap"
	world := &fakeExecutableWorld{}
	world.setLookup(path)
	world.setRecord(path, fakeExecutableRecord{resolved: path, identity: "inode-a", trusted: true})
	backend := newFakeBwrap(world)
	if _, ok := backend.SandboxCapability(); !ok {
		t.Fatal("trusted initial capability unavailable")
	}

	world.setRecord(path, fakeExecutableRecord{resolved: path, identity: "inode-b", trusted: true})
	if backend.Available() {
		t.Fatal("same-path identity replacement retained sandbox authority")
	}
	world.setRecord(path, fakeExecutableRecord{resolved: path, identity: "inode-a", trusted: true})
	if backend.Available() {
		t.Fatal("same-path reverse ABA resurrected sandbox authority")
	}
}

func TestBwrapCapabilityConcurrentIdentityReplacementFailsClosed(t *testing.T) {
	const path = "/usr/bin/bwrap"
	world := &fakeExecutableWorld{}
	world.setLookup(path)
	world.setRecord(path, fakeExecutableRecord{resolved: path, identity: "inode-a", trusted: true})
	backend := newFakeBwrap(world)
	if _, ok := backend.SandboxCapability(); !ok {
		t.Fatal("trusted initial capability unavailable")
	}

	world.setRecord(path, fakeExecutableRecord{resolved: path, identity: "inode-b", trusted: true})
	const workers = 32
	start := make(chan struct{})
	results := make(chan bool, workers)
	var ready sync.WaitGroup
	ready.Add(workers)
	for range workers {
		go func() {
			ready.Done()
			<-start
			_, ok := backend.SandboxCapability()
			results <- ok
		}()
	}
	ready.Wait()
	close(start)
	for range workers {
		if <-results {
			t.Fatal("concurrent replacement retained sandbox authority")
		}
	}

	world.setRecord(path, fakeExecutableRecord{resolved: path, identity: "inode-a", trusted: true})
	if backend.Available() {
		t.Fatal("concurrent reverse ABA resurrected sandbox authority")
	}
}
