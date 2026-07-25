package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestToolAgentDeepKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolAgentDeepKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolAgentDeepEnglishContract(t *testing.T) {
	cause := errors.New("raw-cause-42")
	tests := map[Key]struct {
		args []any
		want string
	}{
		KeyToolAgentDeepPermissionSnapshotUnavailable:    {want: "cannot resume subagent without its complete parent permission snapshot"},
		KeyToolAgentDeepResumeContextUntrusted:           {want: "cannot resume subagent from untrusted or modified persisted security metadata"},
		KeyToolAgentDeepRunOutcome:                       {args: []any{"failed", "raw reason"}, want: "agent run failed: raw reason"},
		KeyToolAgentDeepBackgroundManagerUnavailable:     {want: "background manager is not available"},
		KeyToolAgentDeepAutoBackgroundStartFailed:        {args: []any{cause}, want: "failed to start auto-background agent: raw-cause-42"},
		KeyToolAgentDeepProviderNotConfigured:            {want: "Agent error: provider is not configured"},
		KeyToolAgentDeepRegistryNotConfigured:            {want: "Agent error: registry is not configured"},
		KeyToolAgentDeepCWDWorktreeConflict:              {want: `Agent error: cwd cannot be combined with isolation="worktree"`},
		KeyToolAgentDeepWorktreeTrustedRootRequired:      {args: []any{cause}, want: "Agent error: isolation=worktree requires a trusted parent project root: raw-cause-42"},
		KeyToolAgentDeepSessionMissingInput:              {args: []any{"agent-42"}, want: `agent session "agent-42" is missing persisted input`},
		KeyToolAgentDeepErrorCause:                       {args: []any{cause}, want: "Agent error: raw-cause-42"},
		KeyToolAgentDeepCWDAbsoluteRequired:              {want: "Agent error: cwd must be an absolute path"},
		KeyToolAgentDeepCWDInaccessible:                  {args: []any{cause}, want: "Agent error: cwd is not accessible: raw-cause-42"},
		KeyToolAgentDeepCWDDirectoryRequired:             {want: "Agent error: cwd must be a directory"},
		KeyToolAgentDeepCWDOutsideParentScope:            {want: "Agent error: cwd is outside the parent permission scope"},
		KeyToolAgentDeepEncounteredError:                 {args: []any{cause}, want: "(Agent encountered error: raw-cause-42)"},
		KeyToolAgentDeepRunFailedWithDetail:              {args: []any{"detail", cause}, want: "Agent error: detail: raw-cause-42"},
		KeyToolAgentDeepSubagentTypeNotAllowed:           {args: []any{"plan", "explore, executor"}, want: `Agent error: subagent_type "plan" is not allowed by this agent's Agent(...) tool restriction. Allowed agents: explore, executor`},
		KeyToolAgentDeepForkContextRequired:              {want: "Agent error: fork subagent requires parent tool execution context"},
		KeyToolAgentDeepForkNestedUnavailable:            {want: "Agent error: fork is not available inside a forked worker. Complete the task directly using available tools"},
		KeyToolAgentDeepIsolationUnsupported:             {args: []any{"container"}, want: `Agent error: unsupported isolation mode "container"`},
		KeyToolAgentDeepUnknownSubagentType:              {args: []any{"mystery", "explore, plan"}, want: `Agent error: unknown subagent_type "mystery". Available agents: explore, plan`},
		KeyToolAgentDeepMCPServersRequired:               {args: []any{"reviewer", "github, slack"}, want: `Agent error: agent "reviewer" requires MCP servers with tools: github, slack`},
		KeyToolAgentDeepFrontmatterParseFailed:           {args: []any{"/tmp/agent.md", cause}, want: "Agent error: failed to parse agent frontmatter in /tmp/agent.md: raw-cause-42"},
		KeyToolAgentDeepCustomPromptEmpty:                {args: []any{"reviewer"}, want: `Agent error: custom agent "reviewer" has an empty prompt`},
		KeyToolAgentDeepMCPServerNamedError:              {args: []any{"github", cause}, want: "github: raw-cause-42"},
		KeyToolAgentDeepMCPServerConfigExpected:          {want: "expected server name or inline server config"},
		KeyToolAgentDeepMCPServersValueExpected:          {want: "expected string, list, or object"},
		KeyToolAgentDeepMCPCommandRequired:               {want: "command is required"},
		KeyToolAgentDeepAgentsJSONParseFailed:            {args: []any{cause}, want: "Agent error: failed to parse --agents JSON: raw-cause-42"},
		KeyToolAgentDeepJSONNameEmpty:                    {want: "Agent error: JSON agent name must not be empty"},
		KeyToolAgentDeepJSONDescriptionMissing:           {args: []any{"reviewer"}, want: `Agent error: JSON agent "reviewer" is missing description`},
		KeyToolAgentDeepJSONPromptMissing:                {args: []any{"reviewer"}, want: `Agent error: JSON agent "reviewer" is missing prompt`},
		KeyToolAgentDeepJSONModelEmpty:                   {args: []any{"reviewer"}, want: `Agent error: JSON agent "reviewer" uses empty model`},
		KeyToolAgentDeepJSONMaxTurnsUnsupported:          {args: []any{"reviewer", 0}, want: `Agent error: JSON agent "reviewer" uses unsupported maxTurns 0`},
		KeyToolAgentDeepJSONMCPServersInvalid:            {args: []any{"reviewer", cause}, want: `Agent error: JSON agent "reviewer" has invalid mcpServers: raw-cause-42`},
		KeyToolAgentDeepJSONHooksInvalid:                 {args: []any{"reviewer", cause}, want: `Agent error: JSON agent "reviewer" has invalid hooks: raw-cause-42`},
		KeyToolAgentDeepJSONMemoryUnsupported:            {args: []any{"reviewer", "global"}, want: `Agent error: JSON agent "reviewer" uses unsupported memory scope "global"`},
		KeyToolAgentDeepJSONIsolationUnsupported:         {args: []any{"reviewer", "remote"}, want: `Agent error: JSON agent "reviewer" uses unsupported isolation "remote"`},
		KeyToolAgentDeepJSONArrayExpected:                {want: "expected array"},
		KeyToolAgentDeepParentProjectRootEmpty:           {want: "parent project root is empty"},
		KeyToolAgentDeepWorktreeGitRepoRequired:          {want: "Agent error: isolation=worktree requires running inside a git repository"},
		KeyToolAgentDeepWorktreeCommitRequired:           {want: "Agent error: isolation=worktree requires a git repository with at least one commit"},
		KeyToolAgentDeepWorktreeCreateFailed:             {args: []any{"raw-git-output"}, want: "Agent error: failed to create worktree: raw-git-output"},
		KeyToolAgentDeepPersistedWorktreeBranchMissing:   {args: []any{"/tmp/wt"}, want: `Agent error: persisted worktree "/tmp/wt" is missing branch metadata`},
		KeyToolAgentDeepPersistedWorktreeRepoRootMissing: {args: []any{"/tmp/wt"}, want: `Agent error: persisted worktree "/tmp/wt" is missing repo root metadata`},
		KeyToolAgentDeepWorktreeRestoreFailed:            {args: []any{"/tmp/wt", "branch-42", "raw-git-output"}, want: `Agent error: failed to restore worktree "/tmp/wt" from branch "branch-42": raw-git-output`},
		KeyToolAgentDeepWorktreeRemoveFailed:             {args: []any{"/tmp/wt", "raw-git-output"}, want: `Agent error: failed to remove clean worktree "/tmp/wt": raw-git-output`},
		KeyToolAgentDeepOutputDirCreateFailed:            {args: []any{cause}, want: "create background task output dir: raw-cause-42"},
		KeyToolAgentDeepSessionRecordIDMissing:           {want: "agent session record is missing id"},
		KeyToolAgentDeepSessionRunningElsewhere:          {args: []any{"agent-42"}, want: `agent "agent-42" is running in another process and cannot be resumed from this session`},
		KeyToolAgentDeepSessionRestoreUnsupported:        {args: []any{"agent-42"}, want: `agent "agent-42" is persisted but this runtime cannot restore agent sessions`},
		KeyToolAgentDeepSessionRestoreEmpty:              {args: []any{"agent-42"}, want: `agent "agent-42" restore produced no runnable session`},
		KeyToolAgentDeepSessionUnavailable:               {args: []any{errors.New("agent_run_interrupted")}, want: "agent_run_interrupted: agent session is unavailable"},
		KeyToolAgentDeepPromptEmpty:                      {want: "prompt must not be empty"},
		KeyToolAgentDeepTaskKilledBeforeStart:            {want: "agent task was killed before it started"},
	}

	if len(tests) != len(toolAgentDeepKeys) {
		t.Fatalf("English compatibility cases = %d, keys = %d", len(tests), len(toolAgentDeepKeys))
	}
	for _, key := range toolAgentDeepKeys {
		test, ok := tests[key]
		if !ok {
			t.Errorf("missing English compatibility case for %s", key)
			continue
		}
		if got := Format(LangEN, key, test.args...); got != test.want {
			t.Errorf("Format(LangEN, %s) = %q, want %q", key, got, test.want)
		}
	}
}

func TestToolAgentDeepErrorsUseRuntimeLanguageAndPreserveCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := errors.New("raw-cause-42")
	err := WrapError(KeyToolAgentDeepAutoBackgroundStartFailed, cause)
	if !errors.Is(err, cause) {
		t.Fatal("semantic Agent error did not preserve its cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese {
		t.Fatalf("runtime language did not change Agent error: %q", english)
	}
	if !strings.Contains(chinese, "raw-cause-42") {
		t.Fatalf("localized Agent error omitted raw cause: %q", chinese)
	}
	if !errors.Is(err, cause) {
		t.Fatal("localized Agent error no longer preserves its cause")
	}
}
