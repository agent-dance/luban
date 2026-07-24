package swarm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

type transactionTmuxBackend struct {
	mu          sync.Mutex
	inside      bool
	nextPane    int
	failures    map[string]int
	calls       map[string]int
	live        map[string]struct{}
	onSendKeys  func()
	killCtxErrs []error
}

func newTransactionTmuxBackend() *transactionTmuxBackend {
	return &transactionTmuxBackend{
		failures: make(map[string]int), calls: make(map[string]int), live: make(map[string]struct{}),
	}
}

func (b *transactionTmuxBackend) call(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls[name]++
	if b.failures[name] > 0 {
		b.failures[name]--
		return fmt.Errorf("injected %s failure", name)
	}
	return nil
}

func (b *transactionTmuxBackend) Available() bool  { return true }
func (b *transactionTmuxBackend) InsideTmux() bool { return b.inside }
func (b *transactionTmuxBackend) CreateSession(_ context.Context, _ string) (string, error) {
	if err := b.call("create"); err != nil {
		return "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextPane++
	id := fmt.Sprintf("%%%d", b.nextPane)
	b.live[id] = struct{}{}
	return id, nil
}
func (b *transactionTmuxBackend) SplitPane(_ context.Context, _ string, _ bool, _ int) (string, error) {
	if err := b.call("split"); err != nil {
		return "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextPane++
	id := fmt.Sprintf("%%%d", b.nextPane)
	b.live[id] = struct{}{}
	return id, nil
}
func (b *transactionTmuxBackend) SetPaneTitle(_ context.Context, _, _, _ string) error {
	return b.call("title")
}
func (b *transactionTmuxBackend) SetPaneBorderColor(_ context.Context, _, _ string) error {
	return b.call("border")
}
func (b *transactionTmuxBackend) SendKeys(_ context.Context, _, _ string) error {
	if b.onSendKeys != nil {
		b.onSendKeys()
	}
	return b.call("send_keys")
}
func (b *transactionTmuxBackend) KillPane(ctx context.Context, paneID string) error {
	b.mu.Lock()
	b.killCtxErrs = append(b.killCtxErrs, ctx.Err())
	b.mu.Unlock()
	if err := b.call("kill"); err != nil {
		return err
	}
	b.mu.Lock()
	delete(b.live, paneID)
	b.mu.Unlock()
	return nil
}
func (b *transactionTmuxBackend) SelectLayout(_ context.Context, _, _ string) error {
	return b.call("layout")
}

func newTransactionExecutor(t *testing.T, backend *transactionTmuxBackend) *TeammateExecutor {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	executor, err := NewTeammateExecutor(backend, "transaction-team", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func TestSpawnPreflightAndCreateFailuresAcquireNoPane(t *testing.T) {
	backend := newTransactionTmuxBackend()
	executor := newTransactionExecutor(t, backend)
	if _, err := executor.Spawn(context.Background(), SpawnOpts{
		Name: "worker", Permissions: []string{"--unknown value"},
	}); err == nil {
		t.Fatal("expected preflight error")
	}
	backend.failures["create"] = 1
	if _, err := executor.Spawn(context.Background(), SpawnOpts{Name: "worker"}); err == nil {
		t.Fatal("expected create error")
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.calls["create"] != 1 || backend.calls["kill"] != 0 || len(backend.live) != 0 {
		t.Fatalf("calls=%#v live=%#v", backend.calls, backend.live)
	}
}

func TestSpawnPostCreateFailureRollsBackPaneAtEveryFatalStage(t *testing.T) {
	for _, stage := range []string{"send_keys", "mailbox"} {
		t.Run(stage, func(t *testing.T) {
			backend := newTransactionTmuxBackend()
			executor := newTransactionExecutor(t, backend)
			if stage == "send_keys" {
				backend.failures["send_keys"] = 1
			} else {
				executor.sendMessage = func(context.Context, string, Message) error {
					return errors.New("injected mailbox failure")
				}
			}
			_, err := executor.Spawn(context.Background(), SpawnOpts{Name: "worker", Task: "work"})
			if err == nil {
				t.Fatal("expected spawn failure")
			}
			backend.mu.Lock()
			defer backend.mu.Unlock()
			if backend.calls["kill"] != 1 || len(backend.live) != 0 {
				t.Fatalf("calls=%#v live=%#v", backend.calls, backend.live)
			}
		})
	}
}

func TestSpawnRollbackIgnoresCancelledCallerContext(t *testing.T) {
	backend := newTransactionTmuxBackend()
	executor := newTransactionExecutor(t, backend)
	ctx, cancel := context.WithCancel(context.Background())
	backend.onSendKeys = cancel
	backend.failures["send_keys"] = 1
	if _, err := executor.Spawn(ctx, SpawnOpts{Name: "worker"}); err == nil {
		t.Fatal("expected send failure")
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.killCtxErrs) != 1 || backend.killCtxErrs[0] != nil {
		t.Fatalf("rollback inherited cancelled context: %#v", backend.killCtxErrs)
	}
}

func TestSpawnRollbackFailureRemainsRetryableByCleanup(t *testing.T) {
	backend := newTransactionTmuxBackend()
	executor := newTransactionExecutor(t, backend)
	executor.sendMessage = func(context.Context, string, Message) error {
		return errors.New("injected mailbox failure")
	}
	backend.failures["kill"] = 1
	_, err := executor.Spawn(context.Background(), SpawnOpts{Name: "worker", Task: "work"})
	if err == nil || !containsAll(err.Error(), "mailbox failure", "rollback pane") {
		t.Fatalf("joined rollback error = %v", err)
	}
	if err := executor.Cleanup(context.Background(), nil); err != nil {
		t.Fatalf("cleanup retry: %v", err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.calls["kill"] != 2 || len(backend.live) != 0 {
		t.Fatalf("calls=%#v live=%#v", backend.calls, backend.live)
	}
}

func TestCleanupPreservesInventoryUntilAllPanesTerminateAndDeduplicates(t *testing.T) {
	backend := newTransactionTmuxBackend()
	executor := newTransactionExecutor(t, backend)
	backend.inside = true
	if err := SaveTeamConfig(&TeamConfig{Name: "transaction-team", LeadAgentID: "lead"}); err != nil {
		t.Fatal(err)
	}
	backend.live["%1"] = struct{}{}
	backend.live["%2"] = struct{}{}
	backend.failures["kill"] = 1
	members := []TeamMember{
		{Name: "a", TmuxPaneID: "%1"}, {Name: "a-duplicate", TmuxPaneID: "%1"}, {Name: "b", TmuxPaneID: "%2"},
	}
	if err := executor.Cleanup(context.Background(), members); err == nil {
		t.Fatal("expected first cleanup failure")
	}
	if _, err := LoadTeamConfig("transaction-team"); err != nil {
		t.Fatalf("config inventory was deleted before retry: %v", err)
	}
	if err := executor.Cleanup(context.Background(), members); err != nil {
		t.Fatalf("cleanup retry: %v", err)
	}
	if _, err := LoadTeamConfig("transaction-team"); err == nil {
		t.Fatal("config remained after successful cleanup")
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.calls["kill"] != 3 || len(backend.live) != 0 {
		t.Fatalf("calls=%#v live=%#v", backend.calls, backend.live)
	}
}

func TestSpawnCosmeticFailuresAreNonFatal(t *testing.T) {
	backend := newTransactionTmuxBackend()
	executor := newTransactionExecutor(t, backend)
	backend.failures["title"] = 1
	backend.failures["border"] = 1
	backend.failures["layout"] = 1
	member, err := executor.Spawn(context.Background(), SpawnOpts{Name: "worker", LeaderPane: "%0"})
	if err != nil || member == nil {
		t.Fatalf("spawn = %#v, %v", member, err)
	}
}

func containsAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			return false
		}
	}
	return true
}
