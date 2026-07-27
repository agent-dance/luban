package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/engine"
)

func TestExecuteApplicationShutdownStepsPreservesOrderAndCollectsSemanticErrors(t *testing.T) {
	wantErr := errors.New("internal shutdown cause")
	var order []string
	issues := executeApplicationShutdownSteps(context.Background(), []applicationShutdownStep{
		{key: i18n.KeyStartupShutdownSchedule, run: func(context.Context) error {
			order = append(order, "schedule")
			return nil
		}},
		{key: i18n.KeyStartupShutdownEngine, run: func(context.Context) error {
			order = append(order, "engine")
			return wantErr
		}},
		{key: i18n.KeyStartupShutdownBackground, run: func(context.Context) error {
			order = append(order, "background")
			return context.DeadlineExceeded
		}},
	})

	if got := len(issues); got != 2 {
		t.Fatalf("issues = %d, want 2", got)
	}
	if got := order; len(got) != 3 || got[0] != "schedule" || got[1] != "engine" || got[2] != "background" {
		t.Fatalf("shutdown order = %v", got)
	}
	if !errors.Is(joinApplicationShutdownIssues(issues), wantErr) || !errors.Is(joinApplicationShutdownIssues(issues), context.DeadlineExceeded) {
		t.Fatalf("shutdown issues do not preserve internal causes: %v", issues)
	}
	for _, issue := range issues {
		if got := issue.Error(); got == wantErr.Error() || got == context.DeadlineExceeded.Error() {
			t.Fatalf("internal cause leaked through semantic copy: %q", got)
		}
	}
}

func TestApplicationShutdownFailureChangesOnlySuccessfulExit(t *testing.T) {
	issues := []error{i18n.NewError(i18n.KeyStartupShutdownEngine)}
	if got := applicationExitCodeAfterShutdown(0, issues); got != 1 {
		t.Fatalf("successful exit after shutdown failure = %d, want 1", got)
	}
	if got := applicationExitCodeAfterShutdown(2, issues); got != 2 {
		t.Fatalf("existing failure after shutdown failure = %d, want 2", got)
	}
	if got := applicationExitCodeAfterShutdown(0, nil); got != 0 {
		t.Fatalf("clean shutdown exit = %d, want 0", got)
	}
}

func TestShutdownApplicationRuntimeTreatsTypedNilEngineAsAbsent(t *testing.T) {
	var core *engine.CoreEngine
	issues := shutdownApplicationRuntime(context.Background(), nil, core)
	if len(issues) != 0 {
		t.Fatalf("typed-nil engine produced shutdown issues: %v", issues)
	}
}

func TestRunOwnsRegistryCleanupBeforeInitialRuntimePreparation(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate runtime shutdown test")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	setupAt := strings.Index(text, "deps := SetupRegistry(")
	if setupAt < 0 {
		t.Fatal("Run no longer composes RegistryDeps")
	}
	shutdownRelative := strings.Index(text[setupAt:], "shutdownApplicationRuntime(")
	prepareRelative := strings.Index(text[setupAt:], "prepareInitialRegistryRuntime(")
	shutdownAt := setupAt + shutdownRelative
	prepareAt := setupAt + prepareRelative
	if shutdownRelative < 0 || prepareRelative < 0 || shutdownAt <= setupAt || prepareAt <= shutdownAt {
		t.Fatalf("registry cleanup owner must be installed between SetupRegistry and initial preparation: setup=%d shutdown=%d prepare=%d", setupAt, shutdownAt, prepareAt)
	}
}
