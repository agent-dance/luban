package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// TestDetectMissedRuns_FormatsNotification verifies TS startup semantics:
// missed durable one-shot tasks are surfaced once and removed from disk.
func TestDetectMissedRuns_FormatsNotification(t *testing.T) {
	tmp := t.TempDir()
	scope := NewRuntimeScope(tmp, true)
	store := NewCronStore(tmp, scope)

	cronPath := store.cronFilePath()
	if err := os.MkdirAll(filepath.Dir(cronPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := cronFile{
		Tasks: []persistedCronJob{
			{
				ID:        "missjob",
				Cron:      "0 * * * *",
				Prompt:    "ping with ```fence",
				CreatedAt: time.Now().Add(-3 * time.Hour).UnixMilli(),
				Recurring: false,
			},
		},
	}
	data, _ := json.MarshalIndent(body, "", "  ")
	if err := os.WriteFile(cronPath, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	captured := make(chan []MissedRun, 1)
	store.SetMissedRunHandler(func(missed []MissedRun) { captured <- missed })

	store.detectMissedRuns()

	select {
	case missed := <-captured:
		if len(missed) != 1 {
			t.Fatalf("missed count: got %d", len(missed))
		}
		out := BuildMissedTaskNotification(missed)
		for _, want := range []string{
			"missed while Claude was not running",
			"already been removed from .claude/scheduled_tasks.json",
			"Do NOT execute this prompt yet",
			"AskUserQuestion",
			"````\nping with ```fence\n````",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("notification missing %q: %s", want, out)
			}
		}
		jobs, _, err := store.readDurableJobsLocked()
		if err != nil {
			t.Fatalf("read durable: %v", err)
		}
		if len(jobs) != 0 {
			t.Fatalf("missed one-shot was not removed: %#v", jobs)
		}
		store.schedLock = nil
		if due := store.collectDueJobs(time.Now()); len(due) != 0 {
			t.Fatalf("missed one-shot raw prompt reached scheduler after notification: %#v", due)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("missed-run handler not invoked")
	}
}

func TestDetectMissedRuns_DoesNotSurfaceRecurring(t *testing.T) {
	tmp := t.TempDir()
	scope := NewRuntimeScope(tmp, true)
	store := NewCronStore(tmp, scope)
	cronPath := store.cronFilePath()
	if err := os.MkdirAll(filepath.Dir(cronPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	last := time.Now().Add(-2 * time.Hour).UnixMilli()
	body := cronFile{
		Tasks: []persistedCronJob{
			{
				ID:          "recurring",
				Cron:        "0 * * * *",
				Prompt:      "ping",
				CreatedAt:   time.Now().Add(-3 * time.Hour).UnixMilli(),
				LastFiredAt: &last,
				Recurring:   true,
			},
		},
	}
	data, _ := json.MarshalIndent(body, "", "  ")
	if err := os.WriteFile(cronPath, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	captured := make(chan []MissedRun, 1)
	store.SetMissedRunHandler(func(missed []MissedRun) { captured <- missed })
	store.detectMissedRuns()
	select {
	case missed := <-captured:
		t.Fatalf("recurring missed runs should not be surfaced on startup: %#v", missed)
	case <-time.After(100 * time.Millisecond):
	}
	jobs, _, err := store.readDurableJobsLocked()
	if err != nil {
		t.Fatalf("read durable: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "recurring" {
		t.Fatalf("recurring job should remain for normal scheduler path: %#v", jobs)
	}
}

func TestDetectMissedRuns_WithoutHandlerDoesNotWriteStderr(t *testing.T) {
	tmp := t.TempDir()
	store := NewCronStore(tmp, NewRuntimeScope(tmp, true))
	cronPath := store.cronFilePath()
	if err := os.MkdirAll(filepath.Dir(cronPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := cronFile{Tasks: []persistedCronJob{{
		ID:        "silent-missed-job",
		Cron:      "0 * * * *",
		Prompt:    "do not print me",
		CreatedAt: time.Now().Add(-3 * time.Hour).UnixMilli(),
		Recurring: false,
	}}}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cronPath, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	stderr := captureCronStderr(t, store.detectMissedRuns)
	if stderr != "" {
		t.Fatalf("ownerless missed-run notification wrote stderr: %q", stderr)
	}
	jobs, _, err := store.readDurableJobsLocked()
	if err != nil {
		t.Fatalf("read durable: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("missed one-shot was not removed: %#v", jobs)
	}
}

func captureCronStderr(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stderr
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = write
	defer func() { os.Stderr = original }()

	fn()
	if err := write.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	var captured bytes.Buffer
	if _, err := captured.ReadFrom(read); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := read.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return captured.String()
}

// TestAcquireInFlight_DedupesSameTick verifies CR-03's inFlight guard.
// Two collectDueJobs ticks for the SAME minute on the same id must only
// fire once.
func TestAcquireInFlight_DedupesSameTick(t *testing.T) {
	store := NewCronStore(t.TempDir(), NewRuntimeScope(t.TempDir(), true))
	tick := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	if !store.acquireInFlight("job1", tick) {
		t.Fatal("first acquire must succeed")
	}
	if store.acquireInFlight("job1", tick) {
		t.Fatal("second acquire for same tick must fail (CR-03 dedupe)")
	}
	// Different tick → must succeed.
	if !store.acquireInFlight("job1", tick.Add(time.Minute)) {
		t.Fatal("acquire for next tick must succeed")
	}
	// Different id → must succeed.
	if !store.acquireInFlight("job2", tick) {
		t.Fatal("acquire for different id must succeed")
	}
}

// TestGuardDurableCreation_RejectsTeammate verifies CR-04: when the env
// declares this session as a non-durable teammate, durable cron creation
// must be refused with the structured errorCode.
func TestGuardDurableCreation_RejectsTeammate(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CRON_NON_DURABLE_TEAMMATE", "1")
	store := NewCronStore(t.TempDir(), NewRuntimeScope(t.TempDir(), true))
	tool := NewCronCreateTool(store)

	res, err := tool.Execute(context.Background(), map[string]any{
		"cron":    "*/5 * * * *",
		"prompt":  "do work",
		"durable": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error for teammate durable create: %s", res.Content)
	}
	if !strings.Contains(res.Content, "errorCode=4") {
		t.Fatalf("expected errorCode=4 in refusal, got: %s", res.Content)
	}
}

func TestCronCreateTeammateDurableGuardUsesRuntimeAgent(t *testing.T) {
	root := t.TempDir()
	scope := NewRuntimeScope(root, true)
	scope.SetAgentIDFunc(func() string { return "agent-a@team" })
	store := NewCronStore(root, scope)

	res, err := NewCronCreateTool(store).Execute(context.Background(), map[string]any{
		"cron":    "*/5 * * * *",
		"prompt":  "must remain session scoped",
		"durable": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError || res.Metadata["errorCode"] != "4" {
		t.Fatalf("expected runtime teammate durable refusal, got %#v", res)
	}
	if len(store.sessionJobs) != 0 {
		t.Fatalf("durable teammate refusal created session jobs: %#v", store.sessionJobs)
	}
	if _, statErr := os.Stat(store.cronFilePath()); !os.IsNotExist(statErr) {
		t.Fatalf("durable teammate refusal touched cron file: %v", statErr)
	}
}

func TestCronTeammateRegistryBindingIsConcurrentAndImmutable(t *testing.T) {
	root := t.TempDir()
	store := NewCronStore(root, NewRuntimeScope(root, true))
	base := registry.New()
	base.Register(NewCronCreateTool(store))
	base.Register(NewCronDeleteTool(store))
	base.Register(NewCronListTool(store))

	agentA := base.Clone()
	agentB := base.Clone()
	bindInProcessAgentScopedTools(agentA, "agent-a@team")
	bindInProcessAgentScopedTools(agentB, "agent-b@team")

	type createResult struct {
		agentID string
		result  types.ToolResult
		err     error
	}
	created := make(chan createResult, 2)
	for _, tc := range []struct {
		agentID string
		reg     *registry.Registry
	}{
		{agentID: "agent-a@team", reg: agentA},
		{agentID: "agent-b@team", reg: agentB},
	} {
		tc := tc
		go func() {
			result, err := tc.reg.Get("CronCreate").Execute(context.Background(), map[string]any{
				"cron": "*/5 * * * *", "prompt": tc.agentID,
			})
			created <- createResult{agentID: tc.agentID, result: result, err: err}
		}()
	}

	ids := make(map[string]string, 2)
	for range 2 {
		out := <-created
		if out.err != nil || out.result.IsError {
			t.Fatalf("create for %s: err=%v result=%#v", out.agentID, out.err, out.result)
		}
		ids[out.agentID] = extractCronID(t, out.result.Content)
	}
	owners := make(map[string]string, 2)
	for _, job := range store.list() {
		owners[job.ID] = job.AgentID
	}
	for agentID, id := range ids {
		if owners[id] != agentID {
			t.Fatalf("job %s owner=%q, want %q; all=%#v", id, owners[id], agentID, owners)
		}
	}

	denied, err := agentB.Get("CronDelete").Execute(context.Background(), map[string]any{"id": ids["agent-a@team"]})
	if err != nil || !denied.IsError || denied.Metadata["errorCode"] != "2" {
		t.Fatalf("agent B must not delete agent A job: err=%v result=%#v", err, denied)
	}
	deleted, err := agentA.Get("CronDelete").Execute(context.Background(), map[string]any{"id": ids["agent-a@team"]})
	if err != nil || deleted.IsError {
		t.Fatalf("agent A must delete its own job: err=%v result=%#v", err, deleted)
	}

	durable, err := agentB.Get("CronCreate").Execute(context.Background(), map[string]any{
		"cron": "*/5 * * * *", "prompt": "orphan", "durable": true,
	})
	if err != nil || !durable.IsError || durable.Metadata["errorCode"] != "4" {
		t.Fatalf("bound teammate must reject durable creation: err=%v result=%#v", err, durable)
	}

	leader, err := base.Get("CronCreate").Execute(context.Background(), map[string]any{
		"cron": "*/5 * * * *", "prompt": "leader",
	})
	if err != nil || leader.IsError {
		t.Fatalf("base registry must remain unbound: err=%v result=%#v", err, leader)
	}
	leaderID := extractCronID(t, leader.Content)
	if result, err := base.Get("CronDelete").Execute(context.Background(), map[string]any{"id": leaderID}); err != nil || result.IsError {
		t.Fatalf("leader must delete unowned job: err=%v result=%#v", err, result)
	}
}

// TestGuardDurableCreation_AllowsLeader verifies that a normal session
// (env unset) can still create durable jobs.
func TestGuardDurableCreation_AllowsLeader(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CRON_NON_DURABLE_TEAMMATE", "")
	store := NewCronStore(t.TempDir(), NewRuntimeScope(t.TempDir(), true))
	tool := NewCronCreateTool(store)

	res, err := tool.Execute(context.Background(), map[string]any{
		"cron":    "*/5 * * * *",
		"prompt":  "do work",
		"durable": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success for leader durable create, got: %s", res.Content)
	}
}

// TestNextCronRunInLocation_HonoursTimezone verifies CR-07: cron schedules
// must match in the configured IANA timezone, not UTC.
func TestNextCronRunInLocation_HonoursTimezone(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	// At 12:00 UTC on 2026-06-01, LA is 05:00 (during DST). A cron
	// "0 9 * * *" should next fire at 09:00 LA time, which is 16:00 UTC.
	from := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	next, ok := nextCronRunInLocation("0 9 * * *", from, la)
	if !ok {
		t.Fatal("no next fire computed")
	}
	wantUTC := time.Date(2026, 6, 1, 16, 0, 0, 0, time.UTC)
	if !next.Equal(wantUTC) {
		t.Fatalf("next-fire: got %s, want %s (in LA: %s)",
			next.UTC(), wantUTC, next.In(la))
	}
}

// TestCronStore_SetTimezone_WiresEffectiveTZ ensures SetTimezone reaches
// effectiveTZ.
func TestCronStore_SetTimezone_WiresEffectiveTZ(t *testing.T) {
	store := NewCronStore(t.TempDir(), NewRuntimeScope(t.TempDir(), true))
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	store.SetTimezone(tokyo)
	if store.effectiveTZ() != tokyo {
		t.Fatalf("effectiveTZ did not honour SetTimezone")
	}
}

func TestCronCreateStructuredResultAndErrorCodes(t *testing.T) {
	store := NewCronStore(t.TempDir(), NewRuntimeScope(t.TempDir(), true))
	tool := NewCronCreateTool(store)

	res, err := tool.Execute(context.Background(), map[string]any{
		"cron":      "*/5 * * * *",
		"prompt":    "do work",
		"recurring": "false",
		"durable":   "false",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}
	data, ok := res.Data.(cronCreateOutput)
	if !ok {
		t.Fatalf("expected cronCreateOutput Data, got %#v", res.Data)
	}
	if data.ID == "" || data.HumanSchedule == "" || data.Recurring || data.Durable {
		t.Fatalf("unexpected structured data: %#v", data)
	}
	if res.Metadata["id"] != data.ID || res.Metadata["humanSchedule"] == "" || res.Metadata["recurring"] != "false" || res.Metadata["durable"] != "false" {
		t.Fatalf("unexpected metadata: %#v", res.Metadata)
	}

	cases := []struct {
		name string
		in   map[string]any
		code string
	}{
		{"invalid syntax", map[string]any{"cron": "* * *", "prompt": "x"}, "1"},
		{"no future match", map[string]any{"cron": "0 0 31 2 *", "prompt": "x"}, "2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Execute(context.Background(), tc.in)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !res.IsError || res.Metadata["errorCode"] != tc.code || !strings.Contains(res.Content, "errorCode="+tc.code) {
				t.Fatalf("expected errorCode=%s, got content=%q metadata=%#v", tc.code, res.Content, res.Metadata)
			}
		})
	}

	res, err = tool.Execute(context.Background(), map[string]any{
		"cron":    "* * * * *",
		"prompt":  "x",
		"durable": "yes please",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "\"true\"/\"false\"") {
		t.Fatalf("expected semantic boolean refusal, got: %#v", res)
	}
}

func TestCronCreateTSContractAndExactMappedContent(t *testing.T) {
	store := newTestCronStore(t)
	tool := NewCronCreateTool(store)

	if !tool.Schema().RejectsUnknownFields() {
		t.Fatalf("CronCreate input schema must mirror z.strictObject")
	}
	contractProvider, ok := any(tool).(interface{ ToolContract() types.ToolContract })
	if !ok {
		t.Fatal("CronCreate must expose its structured output contract")
	}
	contract := contractProvider.ToolContract()
	if contract.OutputSchema == nil || !contract.Strict || contract.MaxResultSizeChars != 100_000 {
		t.Fatalf("unexpected CronCreate contract: %#v", contract)
	}
	if _, ok := contract.OutputSchema.Properties["next_fire"]; ok {
		t.Fatalf("next_fire belongs in ToolResult metadata, not TS structured data")
	}
	if _, ok := contract.OutputSchema.Properties["tz"]; ok {
		t.Fatalf("tz belongs in ToolResult metadata, not TS structured data")
	}
	if len(contract.OutputSchema.Required) != 3 {
		t.Fatalf("TS output requires id/humanSchedule/recurring only: %#v", contract.OutputSchema.Required)
	}

	res, err := tool.Execute(context.Background(), map[string]any{
		"cron":   "*/5 * * * *",
		"prompt": "do work",
	})
	if err != nil || res.IsError {
		t.Fatalf("Execute: err=%v result=%#v", err, res)
	}
	data := res.Data.(cronCreateOutput)
	want := fmt.Sprintf(
		"Scheduled recurring job %s (Every 5 minutes). Session-only (not written to disk, dies when Claude exits). Auto-expires after 7 days. Use CronDelete to cancel sooner.",
		data.ID,
	)
	block := types.MapToolResult(tool, res, "toolu_create")
	if block.Content != want || res.Content != want {
		t.Fatalf("CronCreate model text mismatch:\nwant=%q\nresult=%q\nblock=%q", want, res.Content, block.Content)
	}
	if data.HumanSchedule != "Every 5 minutes" {
		t.Fatalf("humanSchedule must use cronToHuman, got %#v", data)
	}

	oneShot, err := tool.Execute(context.Background(), map[string]any{
		"cron": "0 9 * * *", "prompt": "morning", "recurring": false,
	})
	if err != nil || oneShot.IsError {
		t.Fatalf("one-shot Execute: err=%v result=%#v", err, oneShot)
	}
	oneShotData := oneShot.Data.(cronCreateOutput)
	oneShotWant := fmt.Sprintf(
		"Scheduled one-shot task %s (Every day at 9:00 AM). Session-only (not written to disk, dies when Claude exits). It will fire once then auto-delete.",
		oneShotData.ID,
	)
	if oneShot.Content != oneShotWant {
		t.Fatalf("one-shot model text mismatch:\nwant=%q\ngot=%q", oneShotWant, oneShot.Content)
	}
	if strings.Contains(oneShot.Content, "tz=") || strings.Contains(oneShot.Content, "next_fire=") {
		t.Fatalf("Go-only metadata leaked into TS model content: %q", oneShot.Content)
	}

	unknown, err := tool.Execute(context.Background(), map[string]any{
		"cron": "* * * * *", "prompt": "x", "extra": true,
	})
	if err != nil {
		t.Fatalf("unknown input Execute: %v", err)
	}
	if !unknown.IsError || !strings.Contains(unknown.Content, "unexpected parameter `extra`") {
		t.Fatalf("expected strict unknown-field rejection, got %#v", unknown)
	}
}

func TestCronCreateEffectiveDurablePrecedesTeammateGuard(t *testing.T) {
	t.Setenv("CLAUDE_CODE_DISABLE_DURABLE_CRON", "1")
	t.Setenv("CLAUDE_CODE_CRON_NON_DURABLE_TEAMMATE", "1")
	store := newTestCronStore(t)
	res, err := NewCronCreateTool(store).Execute(context.Background(), map[string]any{
		"cron": "* * * * *", "prompt": "session fallback", "durable": true,
	})
	if err != nil || res.IsError {
		t.Fatalf("disabled durable gate must coerce before teammate guard: err=%v result=%#v", err, res)
	}
	data := res.Data.(cronCreateOutput)
	if data.Durable || len(store.sessionJobs) != 1 {
		t.Fatalf("expected session-only fallback, data=%#v jobs=%#v", data, store.sessionJobs)
	}
	if strings.Contains(strings.ToLower(NewCronCreateTool(store).Description()), "durable") {
		t.Fatalf("disabled durable gate description must not advertise durable jobs")
	}
}

func TestCronJitterMatchesTSHexFractionFormula(t *testing.T) {
	const id = "80000000"
	if got, want := RecurringJitter(time.Hour, id), 3*time.Minute; got != want {
		t.Fatalf("recurring jitter=%v want %v", got, want)
	}
	target := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	if got, want := OneshotJitter(target, id), -45*time.Second; got != want {
		t.Fatalf("one-shot jitter=%v want %v", got, want)
	}
	if got := RecurringJitter(time.Hour, "not-hex"); got != 0 {
		t.Fatalf("hand-edited non-hex ids must get zero jitter, got %v", got)
	}
}

func TestCronSchedulerUsesSecondPrecisionJitteredNextFire(t *testing.T) {
	store := newTestCronStore(t)
	createdAt := time.Date(2026, 5, 17, 11, 0, 0, 0, time.UTC)
	id := "60000000"
	store.mu.Lock()
	store.sessionJobs[id] = &CronJob{ID: id, Cron: "0 * * * *", Prompt: "hourly", Recurring: true, CreatedAt: createdAt}
	store.sessionOrder = append(store.sessionOrder, id)
	store.mu.Unlock()

	tickCh := make(chan time.Time, 3)
	tickDone := make(chan time.Time)
	fired := make(chan *CronJob, 1)
	store.startWithCompletion(func(job *CronJob) { fired <- job }, tickCh, tickDone)
	defer store.Stop()
	before := time.Date(2026, 5, 17, 12, 2, 14, 0, time.UTC)
	processSchedulerTick(t, tickCh, tickDone, before)
	select {
	case job := <-fired:
		t.Fatalf("scheduler fired before jittered next time: %#v", job)
	default:
	}
	due := time.Date(2026, 5, 17, 12, 2, 15, 0, time.UTC)
	processSchedulerTick(t, tickCh, tickDone, due)
	select {
	case job := <-fired:
		if job.ID != id {
			t.Fatalf("unexpected job: %#v", job)
		}
	default:
		t.Fatal("scheduler did not fire at jittered second-precision next time")
	}
}

func TestCronDurableRecurringPersistsAndReanchorsLastFired(t *testing.T) {
	root := t.TempDir()
	store := NewCronStore(root, NewRuntimeScope(root, true))
	store.schedLock = nil
	createdAt := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	job := &CronJob{ID: "80000000", Cron: "0 * * * *", Prompt: "hourly", Recurring: true, Durable: true, CreatedAt: createdAt}
	if err := store.writeDurableJobsLocked([]*CronJob{job}); err != nil {
		t.Fatalf("seed durable: %v", err)
	}
	dueAt := time.Date(2026, 5, 17, 11, 3, 0, 0, time.UTC)
	due := store.collectDueJobs(dueAt)
	if len(due) != 1 || due[0].ID != job.ID {
		t.Fatalf("expected durable fire at %s, got %#v", dueAt, due)
	}
	persisted, _, err := store.readDurableJobsLocked()
	if err != nil || len(persisted) != 1 || persisted[0].LastFiredAt == nil || !persisted[0].LastFiredAt.Equal(dueAt) {
		t.Fatalf("lastFiredAt was not persisted: jobs=%#v err=%v", persisted, err)
	}

	rebuilt := NewCronStore(root, NewRuntimeScope(root, true))
	loaded, _, err := rebuilt.readDurableJobsLocked()
	if err != nil || len(loaded) != 1 {
		t.Fatalf("rebuild read: jobs=%#v err=%v", loaded, err)
	}
	next, ok := cronNextFireAt(loaded[0], dueAt, time.UTC)
	want := time.Date(2026, 5, 17, 12, 3, 0, 0, time.UTC)
	if !ok || !next.Equal(want) {
		t.Fatalf("rebuilt next=%s want=%s ok=%v", next, want, ok)
	}
	if again := rebuilt.collectDueJobs(dueAt); len(again) != 0 {
		t.Fatalf("rebuilt scheduler immediately refired durable task: %#v", again)
	}
}

func TestCronCreateNextFireMatchesSchedulerState(t *testing.T) {
	store := newTestCronStore(t)
	res, err := NewCronCreateTool(store).Execute(context.Background(), map[string]any{
		"cron": "0 * * * *", "prompt": "hourly",
	})
	if err != nil || res.IsError {
		t.Fatalf("Execute: err=%v result=%#v", err, res)
	}
	data := res.Data.(cronCreateOutput)
	reported, err := time.Parse(time.RFC3339Nano, res.Metadata["next_fire"])
	if err != nil {
		t.Fatalf("parse next_fire: %v (%q)", err, res.Metadata["next_fire"])
	}
	store.mu.Lock()
	job := cloneCronJob(store.sessionJobs[data.ID])
	store.mu.Unlock()
	want, ok := cronNextFireAt(job, job.CreatedAt, store.effectiveTZ())
	if !ok || !reported.Equal(want) {
		t.Fatalf("reported next_fire=%s scheduler next=%s ok=%v", reported, want, ok)
	}
}

func TestCronMissedOneShotUsesRawScheduledTime(t *testing.T) {
	createdAt := time.Date(2026, 5, 17, 11, 59, 30, 0, time.UTC)
	job := &CronJob{ID: "80000000", Cron: "0 12 * * *", Prompt: "no early catchup", CreatedAt: createdAt}
	if cronTaskMissedOneShot(job, createdAt.Add(10*time.Second), time.UTC) {
		t.Fatal("one-shot must not be considered missed before its raw 12:00 schedule")
	}
}

func TestStartCronPromptExecutionHonorsRuntimeGatesBeforeLaunch(t *testing.T) {
	job := &CronJob{ID: "gated", Prompt: "must not launch"}
	old := IdleGate
	IdleGate = IdlePolicyFunc(func(context.Context, string) bool { return false })
	t.Cleanup(func() { IdleGate = old })
	if err := StartCronPromptExecution(nil, nil, job); err != nil {
		t.Fatalf("idle-gated execution should defer without touching dependencies: %v", err)
	}

	IdleGate = old
	t.Setenv("CLAUDE_CODE_DISABLE_CRON", "1")
	if err := StartCronPromptExecution(nil, nil, job); err != nil {
		t.Fatalf("killed execution should stop without touching dependencies: %v", err)
	}
}

func TestCronSchedulerDefersExistingJobsForKillAndIdleGates(t *testing.T) {
	store := newTestCronStore(t)
	sched, _ := ParseCron("* * * * *")
	id, err := store.create("* * * * *", "defer me", true, false, sched)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	dueAt := time.Now().Add(time.Minute)
	store.cacheNextFire(id, dueAt)

	t.Setenv("CLAUDE_CODE_DISABLE_CRON", "1")
	if due := store.collectDueJobs(dueAt); len(due) != 0 {
		t.Fatalf("runtime kill switch fired existing jobs: %#v", due)
	}

	t.Setenv("CLAUDE_CODE_DISABLE_CRON", "0")
	old := IdleGate
	IdleGate = IdlePolicyFunc(func(context.Context, string) bool { return false })
	t.Cleanup(func() { IdleGate = old })
	if due := store.collectDueJobs(dueAt); len(due) != 0 {
		t.Fatalf("idle gate fired existing jobs: %#v", due)
	}
	if next, ok := store.cachedNextFire(id); !ok || !next.Equal(dueAt) {
		t.Fatalf("deferred job lost its next fire: next=%s ok=%v", next, ok)
	}

	IdleGate = IdlePolicyFunc(func(context.Context, string) bool { return true })
	if due := store.collectDueJobs(dueAt); len(due) != 1 || due[0].ID != id {
		t.Fatalf("default allow path did not fire deferred job: %#v", due)
	}
}

func TestCronCreateSemanticBooleanAllAcceptedForms(t *testing.T) {
	cases := []struct {
		name      string
		key       string
		value     any
		wantValue bool
	}{
		{"recurring quoted true", "recurring", "true", true},
		{"recurring quoted false", "recurring", "false", false},
		{"durable quoted true", "durable", "true", true},
		{"durable quoted false", "durable", "false", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestCronStore(t)
			res, err := NewCronCreateTool(store).Execute(context.Background(), map[string]any{
				"cron": "* * * * *", "prompt": "x", tc.key: tc.value,
			})
			if err != nil || res.IsError {
				t.Fatalf("Execute: err=%v result=%#v", err, res)
			}
			data := res.Data.(cronCreateOutput)
			got := data.Recurring
			if tc.key == "durable" {
				got = data.Durable
			}
			if got != tc.wantValue {
				t.Fatalf("%s=%v want %v", tc.key, got, tc.wantValue)
			}
		})
	}

	res, err := NewCronCreateTool(newTestCronStore(t)).Execute(context.Background(), map[string]any{
		"cron": "* * * * *", "prompt": "x", "recurring": "truthy",
	})
	if err != nil || !res.IsError {
		t.Fatalf("arbitrary truthy strings must be rejected: err=%v result=%#v", err, res)
	}
}

func TestCronCreateCombinedLimitRejectsDurableDirection(t *testing.T) {
	root := t.TempDir()
	store := NewCronStore(root, NewRuntimeScope(root, true))
	jobs := make([]*CronJob, 0, 49)
	createdAt := time.Now().Add(-time.Hour)
	for i := 0; i < 49; i++ {
		jobs = append(jobs, &CronJob{
			ID: fmt.Sprintf("%08x", i), Cron: "* * * * *", Prompt: "p", Recurring: true, Durable: true, CreatedAt: createdAt,
		})
	}
	if err := store.writeDurableJobsLocked(jobs); err != nil {
		t.Fatalf("seed durable: %v", err)
	}
	sched, _ := ParseCron("* * * * *")
	if _, err := store.create("* * * * *", "session", true, false, sched); err != nil {
		t.Fatalf("create 50th session job: %v", err)
	}
	res, err := NewCronCreateTool(store).Execute(context.Background(), map[string]any{
		"cron": "* * * * *", "prompt": "durable over cap", "durable": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError || res.Metadata["errorCode"] != "3" || res.Content != "Too many scheduled jobs (max 50). Cancel one first. (errorCode=3)" {
		t.Fatalf("expected combined durable cap code 3, got %#v", res)
	}
}

func TestCronDurableMixedEntriesRoundTripOnlyValid(t *testing.T) {
	root := t.TempDir()
	store := NewCronStore(root, NewRuntimeScope(root, true))
	path := store.cronFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	createdAt := time.Now().Add(-time.Hour).UnixMilli()
	body := cronFile{Tasks: []persistedCronJob{
		{ID: "valid001", Cron: "* * * * *", Prompt: "keep", CreatedAt: createdAt, Recurring: true},
		{ID: "badcron1", Cron: "* * *", Prompt: "drop", CreatedAt: createdAt},
		{ID: "missing1", Cron: "* * * * *", Prompt: "", CreatedAt: createdAt},
		{ID: "badtime1", Cron: "* * * * *", Prompt: "drop", CreatedAt: 0},
	}}
	data, _ := json.Marshal(body)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	res, err := NewCronCreateTool(store).Execute(context.Background(), map[string]any{
		"cron": "*/5 * * * *", "prompt": "new", "durable": true,
	})
	if err != nil || res.IsError {
		t.Fatalf("durable create with mixed state: err=%v result=%#v", err, res)
	}
	loaded, _, err := store.readDurableJobsLocked()
	if err != nil || len(loaded) != 2 || loaded[0].ID != "valid001" {
		t.Fatalf("mixed round trip mismatch: jobs=%#v err=%v", loaded, err)
	}
}

func TestCronCreateDurableGateCoercesToSessionOnly(t *testing.T) {
	t.Setenv("CLAUDE_CODE_DISABLE_DURABLE_CRON", "1")
	root := t.TempDir()
	store := NewCronStore(root, NewRuntimeScope(root, true))
	tool := NewCronCreateTool(store)

	res, err := tool.Execute(context.Background(), map[string]any{
		"cron":    "*/5 * * * *",
		"prompt":  "do work",
		"durable": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected durable gate to coerce, not reject: %s", res.Content)
	}
	if got := res.Metadata["durable"]; got != "false" {
		t.Fatalf("expected effective durable=false, got metadata=%#v content=%s", res.Metadata, res.Content)
	}
	if _, err := os.Stat(store.cronFilePath()); err == nil {
		t.Fatalf("durable gate disabled but cron file was written")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat cron file: %v", err)
	}
	jobs := store.list()
	if len(jobs) != 1 || jobs[0].Durable {
		t.Fatalf("expected one session-only job, got %#v", jobs)
	}
}

func TestCronCreateMixedStoreMaxAndMalformedDurableState(t *testing.T) {
	root := t.TempDir()
	store := NewCronStore(root, NewRuntimeScope(root, true))
	cronPath := store.cronFilePath()
	if err := os.MkdirAll(filepath.Dir(cronPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(cronPath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write malformed: %v", err)
	}
	tool := NewCronCreateTool(store)
	res, err := tool.Execute(context.Background(), map[string]any{
		"cron":    "*/5 * * * *",
		"prompt":  "durable survives malformed state",
		"durable": true,
	})
	if err != nil {
		t.Fatalf("Execute malformed durable: %v", err)
	}
	if res.IsError {
		t.Fatalf("malformed durable file should not block create: %s", res.Content)
	}
	jobs, _, err := store.readDurableJobsLocked()
	if err != nil {
		t.Fatalf("read durable: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected malformed file to be replaced by one valid job, got %d", len(jobs))
	}

	tasks := make([]persistedCronJob, 0, 52)
	for i := 0; i < 50; i++ {
		tasks = append(tasks, persistedCronJob{
			ID:        fmt.Sprintf("job%02d", i),
			Cron:      "* * * * *",
			Prompt:    "p",
			CreatedAt: time.Now().Add(-time.Hour).UnixMilli(),
			Recurring: true,
		})
	}
	tasks = append(tasks,
		persistedCronJob{ID: "badcron", Cron: "* * *", Prompt: "p", CreatedAt: time.Now().UnixMilli()},
		persistedCronJob{ID: "", Cron: "* * * * *", Prompt: "p", CreatedAt: time.Now().UnixMilli()},
	)
	body := cronFile{Tasks: tasks}
	data, _ := json.MarshalIndent(body, "", "  ")
	if err := os.WriteFile(cronPath, data, 0o644); err != nil {
		t.Fatalf("write max state: %v", err)
	}
	res, err = tool.Execute(context.Background(), map[string]any{
		"cron":   "* * * * *",
		"prompt": "session should hit mixed cap",
	})
	if err != nil {
		t.Fatalf("Execute max: %v", err)
	}
	if !res.IsError || res.Metadata["errorCode"] != "3" {
		t.Fatalf("expected mixed max errorCode=3, got content=%q metadata=%#v", res.Content, res.Metadata)
	}
}

func TestCronDeleteOwnerGuard(t *testing.T) {
	root := t.TempDir()
	scope := NewRuntimeScope(root, true)
	currentAgentID := "agent-a@team"
	scope.SetAgentIDFunc(func() string { return currentAgentID })
	store := NewCronStore(root, scope)
	create := NewCronCreateTool(store)
	del := NewCronDeleteTool(store)

	res, err := create.Execute(context.Background(), map[string]any{
		"cron":   "*/5 * * * *",
		"prompt": "owned session work",
	})
	if err != nil || res.IsError {
		t.Fatalf("create owned job: err=%v result=%#v", err, res)
	}
	id := extractCronID(t, res.Content)
	jobs := store.list()
	if len(jobs) != 1 || jobs[0].AgentID != "agent-a@team" {
		t.Fatalf("expected session job owner agent-a@team, got %#v", jobs)
	}
	if _, ok := store.cachedNextFire(id); !ok {
		t.Fatalf("expected scheduler cache for owned job %q", id)
	}

	currentAgentID = "agent-b@team"
	res, err = del.Execute(context.Background(), map[string]any{"id": id})
	if err != nil {
		t.Fatalf("delete other owner: %v", err)
	}
	if !res.IsError || res.Metadata["errorCode"] != "2" {
		t.Fatalf("expected owner-denied errorCode=2, got content=%q metadata=%#v", res.Content, res.Metadata)
	}
	jobs = store.list()
	if len(jobs) != 1 || jobs[0].ID != id {
		t.Fatalf("owner-denied delete removed job: %#v", jobs)
	}
	if _, ok := store.cachedNextFire(id); !ok {
		t.Fatalf("owner-denied delete removed scheduler cache for %q", id)
	}

	currentAgentID = "agent-a@team"
	res, err = del.Execute(context.Background(), map[string]any{"id": id})
	if err != nil {
		t.Fatalf("delete owner: %v", err)
	}
	if res.IsError {
		t.Fatalf("owner should delete own job: %s", res.Content)
	}
	if got := store.list(); len(got) != 0 {
		t.Fatalf("owned job not deleted: %#v", got)
	}
	if _, ok := store.cachedNextFire(id); ok {
		t.Fatalf("successful owner delete left scheduler cache for %q", id)
	}
}

func TestCronDeleteLeaderCanDeleteUnownedSessionJob(t *testing.T) {
	root := t.TempDir()
	scope := NewRuntimeScope(root, true)
	store := NewCronStore(root, scope)
	create := NewCronCreateTool(store)
	del := NewCronDeleteTool(store)

	res, err := create.Execute(context.Background(), map[string]any{
		"cron":   "*/5 * * * *",
		"prompt": "leader work",
	})
	if err != nil || res.IsError {
		t.Fatalf("create leader job: err=%v result=%#v", err, res)
	}
	id := extractCronID(t, res.Content)
	if jobs := store.list(); len(jobs) != 1 || jobs[0].AgentID != "" {
		t.Fatalf("expected unowned leader job, got %#v", jobs)
	}

	res, err = del.Execute(context.Background(), map[string]any{"id": id})
	if err != nil {
		t.Fatalf("delete leader job: %v", err)
	}
	if res.IsError {
		t.Fatalf("leader should delete unowned job: %s", res.Content)
	}
}

func TestCronListTeammateScopedVisibility(t *testing.T) {
	root := t.TempDir()
	scope := NewRuntimeScope(root, true)
	currentAgentID := ""
	scope.SetAgentIDFunc(func() string { return currentAgentID })
	store := NewCronStore(root, scope)
	create := NewCronCreateTool(store)
	list := NewCronListTool(store)

	res, err := create.Execute(context.Background(), map[string]any{
		"cron":    "0 9 * * *",
		"prompt":  "leader durable work",
		"durable": true,
	})
	if err != nil || res.IsError {
		t.Fatalf("create leader durable: err=%v result=%#v", err, res)
	}
	durableID := extractCronID(t, res.Content)

	currentAgentID = "agent-a@team"
	res, err = create.Execute(context.Background(), map[string]any{
		"cron":   "*/5 * * * *",
		"prompt": "agent A work",
	})
	if err != nil || res.IsError {
		t.Fatalf("create agent A: err=%v result=%#v", err, res)
	}
	agentAID := extractCronID(t, res.Content)

	currentAgentID = "agent-b@team"
	res, err = create.Execute(context.Background(), map[string]any{
		"cron":   "* * * * *",
		"prompt": "agent B work",
	})
	if err != nil || res.IsError {
		t.Fatalf("create agent B: err=%v result=%#v", err, res)
	}
	agentBID := extractCronID(t, res.Content)

	result, err := list.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list as B: %v", err)
	}
	if strings.Contains(result.Content, agentAID) || strings.Contains(result.Content, durableID) || !strings.Contains(result.Content, agentBID) {
		t.Fatalf("agent B visibility mismatch:\n%s", result.Content)
	}

	currentAgentID = "agent-a@team"
	result, err = list.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list as A: %v", err)
	}
	if !strings.Contains(result.Content, agentAID) || strings.Contains(result.Content, agentBID) || strings.Contains(result.Content, durableID) {
		t.Fatalf("agent A visibility mismatch:\n%s", result.Content)
	}

	currentAgentID = ""
	result, err = list.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list as leader: %v", err)
	}
	for _, id := range []string{durableID, agentAID, agentBID} {
		if !strings.Contains(result.Content, id) {
			t.Fatalf("leader should see %s in:\n%s", id, result.Content)
		}
	}

	raw, err := os.ReadFile(store.cronFilePath())
	if err != nil {
		t.Fatalf("read cron file: %v", err)
	}
	if strings.Contains(string(raw), "agentId") || strings.Contains(string(raw), "agent_id") || strings.Contains(string(raw), "agent-a@team") || strings.Contains(string(raw), "agent-b@team") {
		t.Fatalf("AgentID leaked to durable state: %s", raw)
	}
}

func TestCronPermanentDurableStateRoundTripAndNoExpiry(t *testing.T) {
	root := t.TempDir()
	store := NewCronStore(root, NewRuntimeScope(root, true))
	store.schedLock = nil
	cronPath := store.cronFilePath()
	if err := os.MkdirAll(filepath.Dir(cronPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := cronFile{
		Tasks: []persistedCronJob{
			{
				ID:        "permjob",
				Cron:      "* * * * *",
				Prompt:    "permanent recurring",
				CreatedAt: time.Now().Add(-8 * 24 * time.Hour).UnixMilli(),
				Recurring: true,
				Permanent: true,
			},
		},
	}
	data, _ := json.MarshalIndent(body, "", "  ")
	if err := os.WriteFile(cronPath, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	jobs, _, err := store.readDurableJobsLocked()
	if err != nil {
		t.Fatalf("read durable: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].Permanent {
		t.Fatalf("permanent flag not read: %#v", jobs)
	}
	if err := store.writeDurableJobsLocked(jobs); err != nil {
		t.Fatalf("write durable: %v", err)
	}
	roundTrip, _, err := store.readDurableJobsLocked()
	if err != nil {
		t.Fatalf("read round trip: %v", err)
	}
	if len(roundTrip) != 1 || !roundTrip[0].Permanent {
		t.Fatalf("permanent flag not preserved: %#v", roundTrip)
	}

	fireMinute := time.Now().Truncate(time.Minute)
	due := store.collectDueJobs(fireMinute)
	if len(due) != 1 || due[0].ID != "permjob" {
		t.Fatalf("expected permanent job to fire once, got %#v", due)
	}
	afterFire, _, err := store.readDurableJobsLocked()
	if err != nil {
		t.Fatalf("read after fire: %v", err)
	}
	if len(afterFire) != 1 || afterFire[0].ID != "permjob" || !afterFire[0].Permanent {
		t.Fatalf("permanent recurring job expired unexpectedly: %#v", afterFire)
	}

	result, err := NewCronListTool(store).Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Contains(result.Content, "expires_in") || strings.Contains(result.Content, "aging") {
		t.Fatalf("CronList should not show permanent expiry diagnostics: %s", result.Content)
	}
}

func TestCronDeleteStorageFailureDistinguishableFromNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based write failure is not stable on Windows")
	}
	root := t.TempDir()
	store := NewCronStore(root, NewRuntimeScope(root, true))
	create := NewCronCreateTool(store)
	del := NewCronDeleteTool(store)

	res, err := create.Execute(context.Background(), map[string]any{
		"cron":    "*/5 * * * *",
		"prompt":  "durable work",
		"durable": true,
	})
	if err != nil || res.IsError {
		t.Fatalf("create durable job: err=%v result=%#v", err, res)
	}
	id := extractCronID(t, res.Content)

	claudeDir := filepath.Dir(store.cronFilePath())
	if err := os.Chmod(claudeDir, 0o555); err != nil {
		t.Fatalf("chmod read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(claudeDir, 0o755) })

	res, err = del.Execute(context.Background(), map[string]any{"id": id})
	if err != nil {
		t.Fatalf("delete durable job: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected storage error, got success: %#v", res)
	}
	if res.Metadata["errorCode"] == "1" || strings.Contains(res.Content, "No scheduled job") || strings.Contains(res.Content, "not found") {
		t.Fatalf("storage failure was reported as not-found: content=%q metadata=%#v", res.Content, res.Metadata)
	}
	if _, ok := store.cachedNextFire(id); !ok {
		t.Fatalf("failed durable delete removed scheduler cache for %q", id)
	}
}

func TestCronDeleteDurableStateAndSchedulerCache(t *testing.T) {
	root := t.TempDir()
	store := NewCronStore(root, NewRuntimeScope(root, true))
	create := NewCronCreateTool(store)
	del := NewCronDeleteTool(store)

	created, err := create.Execute(context.Background(), map[string]any{
		"cron":    "*/5 * * * *",
		"prompt":  "durable cancellation",
		"durable": true,
	})
	if err != nil || created.IsError {
		t.Fatalf("create durable: err=%v result=%#v", err, created)
	}
	id := extractCronID(t, created.Content)
	if _, ok := store.cachedNextFire(id); !ok {
		t.Fatalf("expected scheduler cache for durable job %q", id)
	}

	deleted, err := del.Execute(context.Background(), map[string]any{"id": id})
	if err != nil || deleted.IsError {
		t.Fatalf("delete durable: err=%v result=%#v", err, deleted)
	}
	if _, ok := store.cachedNextFire(id); ok {
		t.Fatalf("durable delete left scheduler cache for %q", id)
	}
	jobs, _, err := store.readDurableJobsForDeleteLocked()
	if err != nil {
		t.Fatalf("read durable after delete: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("durable delete left file-backed jobs: %#v", jobs)
	}
	if listed := store.list(); len(listed) != 0 {
		t.Fatalf("durable delete left visible jobs: %#v", listed)
	}

	again, err := del.Execute(context.Background(), map[string]any{"id": id})
	if err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if !again.IsError || again.Metadata["errorCode"] != "1" {
		t.Fatalf("second delete must be not-found: %#v", again)
	}
}

func TestCronDeleteStorageReadFailuresAreOperationalErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		body func(t *testing.T, cronPath string)
	}{
		{
			name: "malformed JSON",
			body: func(t *testing.T, cronPath string) {
				t.Helper()
				if err := os.WriteFile(cronPath, []byte("{\n"), 0o644); err != nil {
					t.Fatalf("write malformed cron file: %v", err)
				}
			},
		},
		{
			name: "path is directory",
			body: func(t *testing.T, cronPath string) {
				t.Helper()
				if err := os.Remove(cronPath); err != nil {
					t.Fatalf("remove cron file: %v", err)
				}
				if err := os.Mkdir(cronPath, 0o755); err != nil {
					t.Fatalf("replace cron file with directory: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			store := NewCronStore(root, NewRuntimeScope(root, true))
			create := NewCronCreateTool(store)
			del := NewCronDeleteTool(store)

			created, err := create.Execute(context.Background(), map[string]any{
				"cron":    "*/5 * * * *",
				"prompt":  "read failure",
				"durable": true,
			})
			if err != nil || created.IsError {
				t.Fatalf("create durable: err=%v result=%#v", err, created)
			}
			id := extractCronID(t, created.Content)
			tc.body(t, store.cronFilePath())

			result, err := del.Execute(context.Background(), map[string]any{"id": id})
			if err != nil {
				t.Fatalf("delete: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected storage error, got success: %#v", result)
			}
			if result.Metadata["errorCode"] == "1" || strings.Contains(result.Content, "No scheduled job") {
				t.Fatalf("read failure was reported as not-found: %#v", result)
			}
			if _, ok := store.cachedNextFire(id); !ok {
				t.Fatalf("failed delete removed scheduler cache for %q", id)
			}
		})
	}
}
