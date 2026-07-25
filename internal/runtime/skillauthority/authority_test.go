package skillauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestCaptureDisabledManagerReturnsInactiveAuthority(t *testing.T) {
	authority, err := Capture(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if authority.enabled || authority.generation != 0 || authority.identity != (executioncontract.RuntimeOwnerIdentity{}) {
		t.Fatalf("disabled authority = %#v", authority)
	}
	called := false
	if err := authority.WithGenerationLease(nil, func() error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("disabled lease called=%t err=%v", called, err)
	}
}

func TestCaptureRejectsContextWithoutRuntimeAuthority(t *testing.T) {
	manager := skills.NewManager()
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		SessionID:   "public-session",
		ProjectRoot: t.TempDir(),
	})
	_, err := Capture(ctx, manager)
	if !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
		t.Fatalf("Capture error = %v", err)
	}
}

func TestAuthorityGenerationLeaseExcludesProjectRetarget(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	store, err := skills.NewFileOverrideStore(rootA, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := skills.ProjectDirs(rootA)
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(dirs...)
	manager.SetOverrideStore(store)
	plan, err := manager.PrepareProjectSources(rootB)
	if err != nil {
		t.Fatal(err)
	}
	authority := Authority{generation: manager.ProjectGeneration(), enabled: true}

	leaseEntered := make(chan struct{})
	releaseLease := make(chan struct{})
	leaseDone := make(chan error, 1)
	go func() {
		leaseDone <- authority.WithGenerationLease(manager, func() error {
			close(leaseEntered)
			<-releaseLease
			return nil
		})
	}()
	<-leaseEntered

	retargetDone := make(chan error, 1)
	go func() { retargetDone <- manager.ApplyProjectSources(plan) }()
	select {
	case err := <-retargetDone:
		t.Fatalf("project retarget crossed generation lease: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseLease)
	if err := <-leaseDone; err != nil {
		t.Fatal(err)
	}
	if err := <-retargetDone; err != nil {
		t.Fatal(err)
	}
	if err := authority.WithGenerationLease(manager, func() error {
		t.Fatal("stale callback executed")
		return nil
	}); !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
		t.Fatalf("stale lease error = %v", err)
	}
}

func TestAuthorityValidatesCanonicalRuntimeIdentity(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	canonicalRoot, ok := canonicalProjectRoot(root)
	if !ok {
		t.Fatal("canonical project root unavailable")
	}
	authority := Authority{
		identity:             executioncontract.RuntimeOwnerIdentity{SessionID: "session-a", ProjectRoot: root},
		canonicalProjectRoot: canonicalRoot,
		enabled:              true,
	}
	if err := authority.ValidateRuntime(types.ToolRuntimeContext{SessionID: " session-a ", ProjectRoot: alias}); err != nil {
		t.Fatalf("canonical runtime rejected: %v", err)
	}
	for name, runtime := range map[string]types.ToolRuntimeContext{
		"session": {SessionID: "session-b", ProjectRoot: root},
		"root":    {SessionID: "session-a", ProjectRoot: t.TempDir()},
		"empty":   {SessionID: "session-a"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := authority.ValidateRuntime(runtime); !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
				t.Fatalf("ValidateRuntime error = %v", err)
			}
		})
	}
	if err := (Authority{}).ValidateRuntime(types.ToolRuntimeContext{}); err != nil {
		t.Fatalf("inactive ValidateRuntime error = %v", err)
	}
}

func TestAuthorityRejectsProjectRootSymlinkRetarget(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	alias := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(rootA, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	capturedRoot, ok := canonicalProjectRoot(alias)
	if !ok {
		t.Fatal("capture symlink root")
	}
	authority := Authority{
		identity:             executioncontract.RuntimeOwnerIdentity{SessionID: "session-a", ProjectRoot: alias},
		canonicalProjectRoot: capturedRoot,
		enabled:              true,
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(rootB, alias); err != nil {
		t.Fatal(err)
	}
	if err := authority.ValidateRuntime(types.ToolRuntimeContext{SessionID: "session-a", ProjectRoot: alias}); !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
		t.Fatalf("retargeted symlink error = %v", err)
	}
	if err := authority.ValidateRuntime(types.ToolRuntimeContext{SessionID: "session-a", ProjectRoot: rootA}); err != nil {
		t.Fatalf("captured real root rejected: %v", err)
	}
	if authority.identity.ProjectRoot != alias {
		t.Fatalf("captured identity root = %q, want original %q", authority.identity.ProjectRoot, alias)
	}
}

func TestAuthorityRelativeProjectRootDoesNotDriftWithCWD(t *testing.T) {
	baseA := t.TempDir()
	baseB := t.TempDir()
	rootA := filepath.Join(baseA, "project")
	rootB := filepath.Join(baseB, "project")
	if err := os.Mkdir(rootA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(rootB, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(baseA)
	capturedRoot, ok := canonicalProjectRoot("project")
	if !ok {
		t.Fatal("capture relative project root")
	}
	authority := Authority{
		identity:             executioncontract.RuntimeOwnerIdentity{SessionID: "session-a", ProjectRoot: "project"},
		canonicalProjectRoot: capturedRoot,
		enabled:              true,
	}

	t.Chdir(baseB)
	if err := authority.ValidateRuntime(types.ToolRuntimeContext{SessionID: "session-a", ProjectRoot: "project"}); !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
		t.Fatalf("cwd-retargeted relative root error = %v", err)
	}
	if err := authority.ValidateRuntime(types.ToolRuntimeContext{SessionID: "session-a", ProjectRoot: rootA}); err != nil {
		t.Fatalf("captured absolute root rejected after cwd change: %v", err)
	}
}

func TestCanonicalProjectRootFailsClosed(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"empty":   "",
		"missing": filepath.Join(t.TempDir(), "missing"),
		"file":    file,
	} {
		t.Run(name, func(t *testing.T) {
			if canonical, ok := canonicalProjectRoot(path); ok || canonical != "" {
				t.Fatalf("canonical root = %q, ok=%t", canonical, ok)
			}
		})
	}
}

func TestAuthorityGenerationLeaseFailsClosed(t *testing.T) {
	manager := skills.NewManager()
	authority := Authority{generation: manager.ProjectGeneration(), enabled: true}
	called := false
	if err := authority.WithGenerationLease(manager, func() error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("current lease called=%t err=%v", called, err)
	}

	called = false
	stale := Authority{generation: authority.generation + 1, enabled: true}
	if err := stale.WithGenerationLease(manager, func() error {
		called = true
		return nil
	}); !errors.Is(err, skills.ErrSkillProjectGenerationChanged) || called {
		t.Fatalf("stale lease called=%t err=%v", called, err)
	}
	if err := (Authority{}).WithGenerationLease(manager, func() error {
		t.Fatal("unpinned callback executed")
		return nil
	}); !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
		t.Fatalf("unpinned lease error = %v", err)
	}
	if err := stale.WithGenerationLease(manager, nil); err != nil {
		t.Fatalf("nil callback error = %v", err)
	}

	want := errors.New("commit failed")
	calls := 0
	if err := authority.WithGenerationLease(manager, func() error {
		calls++
		return want
	}); !errors.Is(err, want) || calls != 1 {
		t.Fatalf("callback error = %v, calls = %d", err, calls)
	}
}
