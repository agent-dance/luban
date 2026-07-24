package i18n

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestToolWorktreeHelperCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range toolWorktreeHelperKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestToolWorktreeHelperEnglishContract(t *testing.T) {
	cause := errors.New("raw-cause")
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolWorktreeNameTooLong, []any{64, 65}, "Invalid worktree name: must be 64 characters or fewer (got 65)"},
		{KeyToolWorktreeNamePathSegment, []any{"../feature"}, `Invalid worktree name "../feature": must not contain "." or ".." path segments`},
		{KeyToolWorktreeNameCharacters, []any{"bad name"}, `Invalid worktree name "bad name": each "/"-separated segment must be non-empty and contain only letters, digits, dots, underscores, and dashes`},
		{KeyToolWorktreeWorkingDirectoryEmpty, nil, "working directory is empty"},
		{KeyToolWorktreePRNumberInvalid, []any{"pr:0"}, `invalid PR number in "pr:0"`},
		{KeyToolWorktreePRReferenceInvalid, []any{"pr:bad"}, "unrecognised PR reference \"pr:bad\" (expected `pr:<num>` or `pr:<owner>/<repo>#<num>`)"},
		{KeyToolWorktreePRReferenceNil, nil, "nil PR ref"},
		{KeyToolWorktreePRFetchFailed, []any{"origin", "pull/7/head:refs/pr/7/head", "raw-git"}, "git fetch origin pull/7/head:refs/pr/7/head failed: raw-git"},
		{KeyToolWorktreePathClaimed, []any{"/repo/wt", "session-7"}, `worktree "/repo/wt" is active in session "session-7"`},
		{KeyToolWorktreeListFailed, []any{"raw-git"}, "git worktree list failed: raw-git"},
		{KeyToolWorktreeRepositoryRootEmpty, nil, "canonical repository root is empty"},
		{KeyToolWorktreeBranchMismatch, []any{"/repo/wt", "actual", "expected"}, `worktree path "/repo/wt" is registered for branch "actual", expected "expected"`},
		{KeyToolWorktreePathUnregistered, []any{"/repo/wt"}, `worktree path "/repo/wt" already exists but is not registered with git`},
		{KeyToolWorktreeInspectPathFailed, []any{"/repo/wt", cause}, `inspect worktree path "/repo/wt": raw-cause`},
		{KeyToolWorktreeCreateParentFailed, []any{cause}, "create worktree parent: raw-cause"},
		{KeyToolWorktreeResolveBaseRefFailed, []any{"main", "raw-git"}, `failed to resolve base ref "main": raw-git`},
		{KeyToolWorktreeCreateFailed, []any{"raw-git"}, "failed to create worktree: raw-git"},
		{KeyToolWorktreeRollbackFailed, []any{"raw-git", cause}, "raw-cause; rollback failed: raw-git"},
		{KeyToolWorktreeRolledBack, []any{cause}, "raw-cause; worktree was rolled back"},
		{KeyToolWorktreeInspectChangesFailed, []any{"raw-git"}, "inspect worktree changes: raw-git"},
		{KeyToolWorktreeUncommittedChanges, nil, "worktree has uncommitted changes"},
		{KeyToolWorktreeRemoveFailed, []any{"raw-git"}, "failed to remove worktree: raw-git"},
	}
	for _, tt := range tests {
		if got := Format(LangEN, tt.key, tt.args...); got != tt.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
		for _, lang := range AllLanguages() {
			if got := Format(lang, tt.key, tt.args...); strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) has a formatting error: %q", lang.Code(), tt.key, got)
			}
		}
	}
}

func TestToolWorktreeHelperErrorsUseActiveLanguageAndPreserveCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := errors.New("raw-path-cause")
	err := WrapError(KeyToolWorktreeInspectPathFailed, cause, "/raw/worktree/path")
	if !errors.Is(err, cause) {
		t.Fatal("worktree helper error did not preserve its cause")
	}
	rollbackErr := WrapError(KeyToolWorktreeRollbackFailed, cause, "raw-rollback-output")
	if !errors.Is(rollbackErr, cause) {
		t.Fatal("worktree rollback error did not preserve its cause")
	}
	typedCause := &os.PathError{Op: "lstat", Path: "/raw/worktree/path", Err: os.ErrPermission}
	typedErr := WrapError(KeyToolWorktreeInspectPathFailed, typedCause, typedCause.Path)
	var pathErr *os.PathError
	if !errors.As(typedErr, &pathErr) || pathErr != typedCause {
		t.Fatal("worktree helper error did not preserve the typed cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	if got := rollbackErr.Error(); got != "raw-path-cause; rollback failed: raw-rollback-output" {
		t.Fatalf("rollback English contract changed: %q", got)
	}
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese {
		t.Fatalf("runtime language did not change the error: %q", english)
	}
	for _, raw := range []string{"/raw/worktree/path", "raw-path-cause"} {
		if !strings.Contains(chinese, raw) {
			t.Errorf("localized error omitted raw value %q: %q", raw, chinese)
		}
	}
}

func TestToolWorktreeHelperTranslationsPreserveRawGitValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyToolWorktreePRFetchFailed, "raw-remote", "raw-refspec", "raw-git-output")
		for _, raw := range []string{"raw-remote", "raw-refspec", "raw-git-output"} {
			if !strings.Contains(got, raw) {
				t.Errorf("%s omitted raw value %q: %q", lang.Code(), raw, got)
			}
		}
		if strings.Contains(got, "%!") {
			t.Errorf("%s has a formatting error: %q", lang.Code(), got)
		}
	}
}
