package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolinspect "github.com/agent-dance/luban/internal/agentic/inspect"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
)

func TestAgenticV2InspectPinsSessionAndWorktreeAndFeedsApplyPatch(t *testing.T) {
	initial := t.TempDir()
	next := t.TempDir()
	writeInspectV2Fixture(t, filepath.Join(initial, "one.txt"), "one\n")
	writeInspectV2Fixture(t, filepath.Join(initial, "two.txt"), "two\n")
	target := filepath.Join(next, "target.txt")
	writeInspectV2Fixture(t, target, "old\n")

	deps := SetupRegistry(provider.NewProviderRef(nil), initial, []string{initial}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() { stopScheduleForTest(t, deps) })
	deps.BindSessionIdentity("inspect-session-one")

	first := executeInspectV2Glob(t, deps, 1)
	if first.Cursor == "" {
		t.Fatalf("initial Inspect result did not paginate: %#v", first)
	}
	deps.PublishSessionID("inspect-session-two")
	staleSession, staleSessionErr := deps.InspectTool.Execute(context.Background(), inspectContinuation(first.Cursor))
	if staleSessionErr != nil || !staleSession.IsError {
		t.Fatalf("cursor crossed session identity: err=%v result=%+v", staleSessionErr, staleSession)
	}

	second := executeInspectV2Glob(t, deps, 1)
	if second.Cursor == "" {
		t.Fatalf("second Inspect result did not paginate: %#v", second)
	}
	if err := deps.WorktreeRuntime.SwitchCWDContext(context.Background(), next); err != nil {
		t.Fatalf("switch worktree runtime: %v", err)
	}
	staleWorkspace, staleWorkspaceErr := deps.InspectTool.Execute(context.Background(), inspectContinuation(second.Cursor))
	if staleWorkspaceErr != nil || !staleWorkspace.IsError {
		t.Fatalf("cursor crossed worktree runtime: err=%v result=%+v", staleWorkspaceErr, staleWorkspace)
	}

	current := executeInspectV2Glob(t, deps, 10)
	if len(current.Requests) != 1 || len(current.Requests[0].Files) != 1 || current.Requests[0].Files[0] != "target.txt" {
		t.Fatalf("Inspect did not follow worktree root: %#v", current)
	}
	read, readErr := deps.InspectTool.Execute(context.Background(), map[string]any{
		"operation": map[string]any{
			"mode":     toolinspect.ModeNew,
			"requests": []any{map[string]any{"id": "target", "kind": toolinspect.KindRead, "path": "target.txt"}},
		},
	})
	if readErr != nil || read.IsError {
		t.Fatalf("Inspect target read failed: err=%v result=%+v", readErr, read)
	}
	patched, patchErr := deps.ApplyPatchTool.Execute(context.Background(), map[string]any{"patch": strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: target.txt",
		"@@",
		"-old",
		"+new",
		"*** End Patch",
	}, "\n")})
	if patchErr != nil || patched.IsError {
		t.Fatalf("ApplyPatch did not accept Inspect evidence: err=%v result=%+v", patchErr, patched)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "new\n" {
		t.Fatalf("patched target = %q, err=%v", got, err)
	}
}

func executeInspectV2Glob(t testing.TB, deps *RegistryDeps, maxFiles int) toolinspect.Result {
	t.Helper()
	result, err := deps.InspectTool.Execute(context.Background(), map[string]any{
		"operation": map[string]any{
			"mode": toolinspect.ModeNew,
			"requests": []any{map[string]any{
				"id": "files", "kind": toolinspect.KindGlob, "path": ".", "pattern": "**/*.txt", "max_results": 10,
			}},
			"page": map[string]any{"max_files": maxFiles},
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("Inspect glob failed: err=%v result=%+v", err, result)
	}
	output, ok := result.Data.(toolinspect.Result)
	if !ok {
		t.Fatalf("Inspect result data = %T", result.Data)
	}
	return output
}

func inspectContinuation(cursor string) map[string]any {
	return map[string]any{"operation": map[string]any{"mode": toolinspect.ModeContinue, "cursor": cursor}}
}

func writeInspectV2Fixture(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
