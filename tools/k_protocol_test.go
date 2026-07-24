package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ─── SM-08: IsStructuredProtocolMessage ───────────────────────────────────────

func TestStructuredProtocolMessage_RecognisesPermissionRequest(t *testing.T) {
	body := `{"type":"permission_request","tool":"Bash"}`
	kind, ok, err := IsStructuredProtocolMessage(body)
	if err != nil || !ok {
		t.Fatalf("expected permission_request to be recognised: ok=%v err=%v", ok, err)
	}
	if kind != "permission_request" {
		t.Fatalf("kind=%q, want permission_request", kind)
	}
}

func TestStructuredProtocolMessage_RejectsPlainText(t *testing.T) {
	if _, ok, _ := IsStructuredProtocolMessage("hello there"); ok {
		t.Fatalf("plain text must not be classified as structured")
	}
}

func TestStructuredProtocolMessage_RejectsUnknownType(t *testing.T) {
	if _, ok, _ := IsStructuredProtocolMessage(`{"type":"random_chat"}`); ok {
		t.Fatalf("unknown type must not be classified")
	}
}

func TestStructuredProtocolMessage_PropagatesMalformedJSONError(t *testing.T) {
	_, ok, err := IsStructuredProtocolMessage(`{"type":}`)
	if ok {
		t.Fatalf("malformed JSON must not be classified as ok")
	}
	if err == nil {
		t.Fatalf("expected error for malformed JSON")
	}
}

// ─── SM-07: request id collision avoidance ─────────────────────────────────────

func TestRequestID_NoCollisionsUnderConcurrency(t *testing.T) {
	const N = 1000
	ids := make([]string, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			ids[idx] = generateMessageRequestID("shutdown", "worker-1")
		}(i)
	}
	wg.Wait()
	seen := map[string]struct{}{}
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate request id detected: %s", id)
		}
		seen[id] = struct{}{}
	}
}

// ─── SM-01: bridge: scheme handling ────────────────────────────────────────────

func TestParsePeerAddress_BridgeScheme(t *testing.T) {
	addr := parsePeerAddress("bridge:session-abc")
	if addr.scheme != "bridge" {
		t.Fatalf("scheme=%q, want bridge", addr.scheme)
	}
	if addr.target != "session-abc" {
		t.Fatalf("target=%q, want session-abc", addr.target)
	}
}

func TestRequiresBridgePermission_ClassifierApprovableFalse(t *testing.T) {
	id, ok := RequiresBridgePermission("bridge:session-xyz")
	if !ok || id != "session-xyz" {
		t.Fatalf("expected bridge classification: ok=%v id=%q", ok, id)
	}
	if _, ok := RequiresBridgePermission("worker-1"); ok {
		t.Fatalf("plain teammate name must not be a bridge target")
	}
}

func TestSendMessage_BridgeRequiresExplicitGrant(t *testing.T) {
	mgr := newTestManager(t)
	res, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to":      "bridge:peer-1",
		"summary": "ping the peer",
		"message": "hello peer",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected bridge send to be denied without grant; got %s", res.Content)
	}
	if !strings.Contains(res.Content, "classifierApprovable=false") {
		t.Fatalf("expected classifierApprovable=false in error: %s", res.Content)
	}

	// After explicit grant the same call no longer returns the gate error.
	GrantBridgePermission("peer-1")
	defer RevokeBridgePermission("peer-1")
	res2, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to":      "bridge:peer-1",
		"summary": "ping the peer",
		"message": "hello peer",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// We don't have a bridge transport so the call falls through to the
	// "no team / no peer" path, but it must not be the bridge-gate refusal.
	if res2.IsError && strings.Contains(res2.Content, "classifierApprovable=false") {
		t.Fatalf("expected gate to lift after grant; still got refusal: %s", res2.Content)
	}
}

// ─── SM-04: mailbox retry classifies lock errors ───────────────────────────────

func TestIsMailboxRetryable_RecognisesLockContention(t *testing.T) {
	cases := map[string]bool{
		"locked: another writer":           true,
		"resource temporarily unavailable": true,
		"context deadline exceeded":        true,
		"would block":                      true,
		"json marshal: invalid":            false,
		"":                                 false,
	}
	for input, want := range cases {
		got := isMailboxRetryable(toErr(input))
		if got != want {
			t.Fatalf("isMailboxRetryable(%q)=%v, want %v", input, got, want)
		}
	}
}

type stringErr string

func (e stringErr) Error() string { return string(e) }

func toErr(s string) error {
	if s == "" {
		return nil
	}
	return stringErr(s)
}

// ─── TD-01: agent-id keyed todos ───────────────────────────────────────────────

func TestRuntimeScope_TodoKey_PrefersAgentID(t *testing.T) {
	scope := NewRuntimeScope(t.TempDir(), true)
	scope.SetSessionIDFunc(func() string { return "session-A" })
	scope.SetAgentIDFunc(func() string { return "agent-B" })
	if got := scope.TodoKey(); got != "agent-B" {
		t.Fatalf("TodoKey()=%q, want agent-B", got)
	}
	scope.SetAgentIDFunc(func() string { return "" })
	if got := scope.TodoKey(); got != "session-A" {
		t.Fatalf("with empty agent id, TodoKey()=%q want session-A", got)
	}
}

// ─── TD-04: LoadAndSave atomically returns prior snapshot ──────────────────────

func TestTodoStore_LoadAndSave_AtomicSnapshot(t *testing.T) {
	dir := t.TempDir()
	store := NewTodoStore(dir)
	scope := NewRuntimeScope(dir, true)
	store.SetScopeResolver(scope)

	first := []TodoItem{{Content: "step 1", ActiveForm: "Doing step 1", Status: "pending"}}
	if err := store.Save(first); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	old, _, err := store.LoadAndSave(func(prior []TodoItem) ([]TodoItem, error) {
		if len(prior) != 1 || prior[0].Content != "step 1" {
			t.Fatalf("prior snapshot lost; got %+v", prior)
		}
		return []TodoItem{{Content: "step 2", ActiveForm: "Doing step 2", Status: "in_progress"}}, nil
	})
	if err != nil {
		t.Fatalf("LoadAndSave: %v", err)
	}
	if len(old) != 1 || old[0].Content != "step 1" {
		t.Fatalf("returned old snapshot incorrect: %+v", old)
	}
}

// ─── TD-02 + TD-03 + TD-04 wiring through TodoWriteTool ────────────────────────

func TestTodoWrite_RegressionWarningSurfaces(t *testing.T) {
	dir := t.TempDir()
	store := NewTodoStore(dir)
	store.SetScopeResolver(NewRuntimeScope(dir, true))
	tool := NewTodoWriteTool(store)

	if _, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "Task A", "activeForm": "Doing A", "status": "completed"},
			map[string]any{"content": "Task B", "activeForm": "Doing B", "status": "pending"},
		},
	}); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	res, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "Task A", "activeForm": "Doing A", "status": "pending"},
			map[string]any{"content": "Task B", "activeForm": "Doing B", "status": "in_progress"},
		},
	})
	if err != nil {
		t.Fatalf("regression write: %v", err)
	}
	if !strings.Contains(res.Content, "regressed") {
		t.Fatalf("expected regression warning in content: %s", res.Content)
	}
	if got, ok := res.Metadata["regressed"]; !ok || !strings.Contains(got, "Task A") {
		t.Fatalf("expected regressed metadata to mention Task A; got %q", got)
	}
}

func TestTodoWrite_RejectsExceedingSizeCap(t *testing.T) {
	dir := t.TempDir()
	store := NewTodoStore(dir)
	store.SetScopeResolver(NewRuntimeScope(dir, true))
	tool := NewTodoWriteTool(store)
	items := make([]any, MaxTodoListSize+1)
	for i := range items {
		items[i] = map[string]any{
			"content":    fmtItoa(i),
			"activeForm": "Doing task " + fmtItoa(i),
			"status":     "pending",
		}
	}
	res, err := tool.Execute(context.Background(), map[string]any{"todos": items})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error for oversize list; got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "exceeds") {
		t.Fatalf("expected exceeds-cap message; got: %s", res.Content)
	}
}

func fmtItoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	negative := i < 0
	if negative {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// ─── TS-02: pending MCP server reporting ───────────────────────────────────────

func TestToolSearch_EmptyResult_SurfacesPendingMCPServers(t *testing.T) {
	prev := PendingMCPServersFn
	defer func() { PendingMCPServersFn = prev }()
	PendingMCPServersFn = func() []string { return []string{"slack", "github"} }

	res := buildToolSearchResult("slack tool", 0, nil, nil, nil, "")
	if !strings.Contains(res.Content, "Pending MCP servers") {
		t.Fatalf("expected pending MCP hint; got: %s", res.Content)
	}
	if got := res.Metadata["pending_mcp_servers"]; !strings.Contains(got, "slack") {
		t.Fatalf("expected metadata to list slack; got %q", got)
	}
}

func TestToolSearch_EmptyResult_NoPendingHintWhenSilent(t *testing.T) {
	prev := PendingMCPServersFn
	defer func() { PendingMCPServersFn = prev }()
	PendingMCPServersFn = nil

	res := buildToolSearchResult("missing", 5, nil, nil, nil, "")
	if strings.Contains(res.Content, "Pending MCP servers") {
		t.Fatalf("did not expect pending hint without resolver: %s", res.Content)
	}
}

// ─── TK-05: concurrent task ID generation ─────────────────────────────────────

func TestTaskStore_ConcurrentCreateProducesUniqueIDs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_HOME", dir)
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "k-protocol-tk05")
	store := NewTaskStore()
	store.SetScopeResolver(NewRuntimeScope(dir, true))

	const N = 32
	ids := make([]string, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			task := store.createDetailed(fmt.Sprintf("subj-%d", idx), "desc", "", nil)
			if task != nil {
				ids[idx] = task.ID
			}
		}(i)
	}
	wg.Wait()
	seen := map[string]struct{}{}
	for _, id := range ids {
		if id == "" {
			t.Fatalf("missing id in concurrent create result")
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate task id under contention: %s", id)
		}
		seen[id] = struct{}{}
	}
}

// ─── PM-04: refuse plan-mode entry mid-interview ──────────────────────────────

func TestEnterPlanMode_RefusedDuringInterview(t *testing.T) {
	dir := t.TempDir()
	state := NewPlanState(dir)
	state.SetInterviewPhase("permissions-confirm")

	res, err := NewEnterPlanModeTool(state).Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected refusal during interview phase; got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "permissions-confirm") {
		t.Fatalf("expected refusal to mention phase; got: %s", res.Content)
	}
}

// ─── PM-05: glob/regex AllowedPrompts matcher ─────────────────────────────────

func TestPlanState_AllowedPromptMatches_LiteralPrefix(t *testing.T) {
	state := NewPlanState(t.TempDir())
	state.SetAllowedPrompts([]PlanAllowedPrompt{{Tool: "Bash", Prompt: "npm test"}})
	if !state.AllowedPromptMatches("Bash", "npm test --watch") {
		t.Fatalf("literal prefix match must allow trailing args")
	}
	if state.AllowedPromptMatches("Bash", "npm publish") {
		t.Fatalf("literal prefix must not match unrelated commands")
	}
}

func TestPlanState_AllowedPromptMatches_GlobPrefix(t *testing.T) {
	state := NewPlanState(t.TempDir())
	state.SetAllowedPrompts([]PlanAllowedPrompt{{Tool: "Bash", Prompt: "glob:npm * test"}})
	if !state.AllowedPromptMatches("Bash", "npm run test") {
		t.Fatalf("glob with * should match npm run test")
	}
	if !state.AllowedPromptMatches("Bash", "NPM RUN TEST") {
		t.Fatalf("glob match must be case-insensitive")
	}
	if state.AllowedPromptMatches("Bash", "yarn run test") {
		t.Fatalf("glob anchored at npm should not match yarn")
	}
}

func TestPlanState_AllowedPromptMatches_RegexPrefix(t *testing.T) {
	state := NewPlanState(t.TempDir())
	state.SetAllowedPrompts([]PlanAllowedPrompt{{Tool: "Bash", Prompt: `regex:^go (build|test) `}})
	if !state.AllowedPromptMatches("Bash", "go test ./tools/") {
		t.Fatalf("regex should match go test invocations")
	}
	if state.AllowedPromptMatches("Bash", "go vet ./...") {
		t.Fatalf("regex should not match go vet")
	}
}

// ─── PM-03: ExitPlanMode persists user-edited plan ────────────────────────────

func TestExitPlanMode_PersistsUserEditedPlan(t *testing.T) {
	dir := t.TempDir()
	state := NewPlanState(dir)
	planFile := filepath.Join(dir, "plan-test.md")
	if err := os.WriteFile(planFile, []byte("# original"), 0o644); err != nil {
		t.Fatalf("seed plan file: %v", err)
	}
	state.Enter(planFile)

	curated := "# curated\nstep 1\nstep 2"
	res, err := executeApprovedToolForTest(t, NewExitPlanModeTool(state), map[string]any{
		"plan": curated,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success: %s", res.Content)
	}
	body, err := os.ReadFile(planFile)
	if err != nil {
		t.Fatalf("read persisted plan: %v", err)
	}
	if string(body) != curated {
		t.Fatalf("expected persisted curated plan; got %q", string(body))
	}
}

// ─── PM-02: gate-fallback notification ───────────────────────────────────────

func TestExitPlanMode_GateFallbackAttachesReason(t *testing.T) {
	dir := t.TempDir()
	state := NewPlanState(dir)
	planFile := filepath.Join(dir, "plan.md")
	_ = os.WriteFile(planFile, []byte("# plan"), 0o644)
	if err := state.enterWithSnapshot(planFile, map[string]any{
		"permission_mode": "auto",
		"auto_mode":       true,
	}); err != nil {
		t.Fatalf("enter auto plan state: %v", err)
	}

	prev := AutoModeGateFn
	defer SetAutoModeGateFn(prev)
	SetAutoModeGateFn(func(ctx context.Context) (bool, string) {
		return false, "agent runtime offline"
	})

	res, err := executeApprovedToolForTest(t, NewExitPlanModeTool(state), map[string]any{})
	if err != nil || res.IsError {
		t.Fatalf("unexpected failure: err=%v isErr=%v %s", err, res.IsError, res.Content)
	}
	if got := res.Metadata["gateFallbackReason"]; got != "agent runtime offline" {
		t.Fatalf("expected gate_fallback_reason to surface; got %q", got)
	}
	_, needs := state.ConsumePlanModeExitAttachments()
	if !needs {
		t.Fatalf("expected needs_auto_mode_attachment=true")
	}
}

// ─── PM-06: schema versioning round-trip ──────────────────────────────────────

func TestPlanState_PersistedSchemaVersionRoundTrips(t *testing.T) {
	dir := t.TempDir()
	state := NewPlanState(dir)
	state.SetInterviewPhase("scope-confirm")
	state.SetPrePlanState(map[string]any{"mode": "default"})

	// Re-load fresh from disk and confirm fields survive.
	state2 := NewPlanState(dir)
	if got := state2.InterviewPhase(); got != "scope-confirm" {
		t.Fatalf("interview phase lost across reload: %q", got)
	}
	if snap := state2.PrePlanState(); snap == nil || snap["mode"] != "default" {
		t.Fatalf("pre-plan snapshot lost across reload: %+v", snap)
	}
}

// ─── TK-06: blockTask cycle returns false (sanity check) ─────────────────────
func TestTaskStore_BlockTask_RefusesCycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_HOME", dir)
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "k-protocol-tk06")
	store := NewTaskStore()
	store.SetScopeResolver(NewRuntimeScope(dir, true))

	a := store.createDetailed("a", "", "", nil)
	b := store.createDetailed("b", "", "", nil)

	if !store.blockTask(a.ID, b.ID) {
		t.Fatalf("first edge a→b should succeed")
	}
	if store.blockTask(b.ID, a.ID) {
		t.Fatalf("cycle b→a must be refused, got success")
	}
}

// ─── helper used by Bridge test ───────────────────────────────────────────────

func decodeJSON(t *testing.T, body string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode: %v\nbody: %s", err, body)
	}
	return out
}

// ─── TK-04: watchdog idle/deadline detection ───────────────────────────────────

func TestEvaluateWatchdog_IdleTimeoutFires(t *testing.T) {
	now := time.Now()
	cfg := TaskWatchdogConfig{IdleTimeout: time.Second}
	reason := EvaluateWatchdog(cfg, now.Add(-30*time.Second), now.Add(-5*time.Second), now)
	if !strings.Contains(reason, "idle") {
		t.Fatalf("expected idle warning; got %q", reason)
	}
}

func TestEvaluateWatchdog_HardDeadlineFires(t *testing.T) {
	now := time.Now()
	cfg := TaskWatchdogConfig{HardDeadline: 10 * time.Second}
	reason := EvaluateWatchdog(cfg, now.Add(-2*time.Minute), now, now)
	if !strings.Contains(reason, "hard deadline") {
		t.Fatalf("expected hard deadline warning; got %q", reason)
	}
}

func TestEvaluateWatchdog_HealthyTaskHasEmptyReason(t *testing.T) {
	now := time.Now()
	cfg := TaskWatchdogConfig{IdleTimeout: time.Minute, HardDeadline: time.Hour}
	if got := EvaluateWatchdog(cfg, now.Add(-30*time.Second), now.Add(-1*time.Second), now); got != "" {
		t.Fatalf("expected healthy task; got reason=%q", got)
	}
}
