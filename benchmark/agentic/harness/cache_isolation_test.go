package harness

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePromptCacheKeyIsolationRejectsCrossRunReuse(t *testing.T) {
	shared := strings.Repeat("a", 64)
	state := ExperimentState{Runs: map[string]RunRecord{
		"001/task-a/codex": fixtureCacheIsolationRun(t, shared),
		"001/task-b/luban": fixtureCacheIsolationRun(t, shared),
	}}
	if err := ValidatePromptCacheKeyIsolation(state); err == nil {
		t.Fatal("cross-run prompt cache key reuse was accepted")
	}
}

func TestValidatePromptCacheKeyIsolationAcceptsDistinctRunKeys(t *testing.T) {
	state := ExperimentState{Runs: map[string]RunRecord{
		"001/task-a/codex": fixtureCacheIsolationRun(t, strings.Repeat("a", 64)),
		"001/task-a/luban": fixtureCacheIsolationRun(t, strings.Repeat("b", 64)),
	}}
	if err := ValidatePromptCacheKeyIsolation(state); err != nil {
		t.Fatal(err)
	}
}

func fixtureCacheIsolationRun(t testing.TB, key string) RunRecord {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider.jsonl")
	raw, err := json.Marshal(ProviderRoundEvidence{
		ProviderAttemptKind: "inference", PromptCacheKeyPresent: true, PromptCacheKeyHash: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteBytesAtomic(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return RunRecord{Execution: &AgentExecution{EvidencePath: path}}
}
