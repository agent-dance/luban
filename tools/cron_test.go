package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func newTestCronStore(t *testing.T) *CronStore {
	t.Helper()
	return NewCronStore(t.TempDir(), NewRuntimeScope(t.TempDir(), true))
}

var cronIDPattern = regexp.MustCompile(`(?:id=|(?:job|task) )([a-z0-9]+)`)

func extractCronID(t *testing.T, content string) string {
	t.Helper()
	matches := cronIDPattern.FindStringSubmatch(content)
	if len(matches) != 2 {
		t.Fatalf("failed to extract cron id from %q", content)
	}
	return matches[1]
}

func processSchedulerTick(t *testing.T, tickCh chan<- time.Time, tickDone <-chan time.Time, tick time.Time) {
	t.Helper()
	select {
	case tickCh <- tick:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler did not accept injected tick")
	}
	select {
	case completed := <-tickDone:
		if !completed.Equal(tick) {
			t.Fatalf("completed tick=%s want %s", completed, tick)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler did not complete injected tick")
	}
}

// ─── CronStore helpers ────────────────────────────────────────────────────────

func TestValidateCron(t *testing.T) {
	cases := []struct {
		expr  string
		valid bool
	}{
		{"* * * * *", true},
		{"0 9 * * 1-5", true},
		{"*/5 * * * *", true},
		{"* *", false},
		{"", false},
		{"a b c d e f", false}, // 6 fields
	}
	for _, tc := range cases {
		err := validateCron(tc.expr)
		if tc.valid && err != nil {
			t.Errorf("expected valid for %q, got error: %v", tc.expr, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("expected invalid for %q, got no error", tc.expr)
		}
	}
}

// ─── CronCreateTool ───────────────────────────────────────────────────────────

func TestCronCreateTool(t *testing.T) {
	store := newTestCronStore(t)
	tool := NewCronCreateTool(store)
	ctx := context.Background()

	t.Run("creates job and returns ID", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"cron":   "* * * * *",
			"prompt": "say hello",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", result.Content)
		}
		if extractCronID(t, result.Content) == "" {
			t.Errorf("expected id in content, got: %s", result.Content)
		}
		data, ok := result.Data.(cronCreateOutput)
		if !ok {
			t.Fatalf("expected cronCreateOutput, got %#v", result.Data)
		}
		if !data.Recurring || data.Durable {
			t.Errorf("unexpected defaults: %#v", data)
		}
		if result.Metadata["next_fire"] == "" {
			t.Errorf("expected next_fire metadata, got: %#v", result.Metadata)
		}
	})

	t.Run("increments ID for each new job", func(t *testing.T) {
		r1, _ := tool.Execute(ctx, map[string]any{"cron": "0 * * * *", "prompt": "p2"})
		r2, _ := tool.Execute(ctx, map[string]any{"cron": "0 * * * *", "prompt": "p3"})
		if extractCronID(t, r1.Content) == extractCronID(t, r2.Content) {
			t.Errorf("expected unique cron ids, got %q and %q", r1.Content, r2.Content)
		}
	})

	t.Run("respects recurring=false", func(t *testing.T) {
		f := false
		result, err := tool.Execute(ctx, map[string]any{
			"cron":      "0 0 * * *",
			"prompt":    "one-shot",
			"recurring": f,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", result.Content)
		}
		data, ok := result.Data.(cronCreateOutput)
		if !ok || data.Recurring {
			t.Errorf("expected recurring=false, got data=%#v content=%s", result.Data, result.Content)
		}
	})

	t.Run("respects durable=true", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"cron":    "0 0 1 * *",
			"prompt":  "monthly",
			"durable": true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", result.Content)
		}
		data, ok := result.Data.(cronCreateOutput)
		if !ok || !data.Durable {
			t.Errorf("expected durable=true, got data=%#v content=%s", result.Data, result.Content)
		}
	})

	t.Run("error on missing cron", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"cron": "   ", "prompt": "x"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected IsError=true for empty cron")
		}
	})

	t.Run("error on invalid cron field count", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"cron": "* * *", "prompt": "x"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected IsError=true for 3-field cron")
		}
	})

	t.Run("error on missing prompt", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"cron": "* * * * *", "prompt": "  "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected IsError=true for empty prompt")
		}
	})
}

// ─── CronDeleteTool ───────────────────────────────────────────────────────────

func TestCronDeleteTool(t *testing.T) {
	store := newTestCronStore(t)
	create := NewCronCreateTool(store)
	del := NewCronDeleteTool(store)
	list := NewCronListTool(store)
	ctx := context.Background()

	createResult, _ := create.Execute(ctx, map[string]any{"cron": "* * * * *", "prompt": "hello"})
	createdID := extractCronID(t, createResult.Content)

	t.Run("deletes existing job", func(t *testing.T) {
		result, err := del.Execute(ctx, map[string]any{"id": createdID})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", result.Content)
		}
		if result.Content != "Cancelled job "+createdID+"." {
			t.Errorf("unexpected confirmation: %s", result.Content)
		}
		if result.Metadata["id"] != createdID {
			t.Errorf("expected id metadata %q, got %#v", createdID, result.Metadata)
		}
		data, ok := result.Data.(cronDeleteOutput)
		if !ok || data.ID != createdID {
			t.Errorf("expected structured delete output id=%q, got %#v", createdID, result.Data)
		}
	})

	t.Run("job is gone from list after delete", func(t *testing.T) {
		r, _ := list.Execute(ctx, map[string]any{})
		if r.Content != "No scheduled jobs." {
			t.Errorf("expected no jobs after delete, got: %s", r.Content)
		}
	})

	t.Run("error on unknown ID", func(t *testing.T) {
		result, err := del.Execute(ctx, map[string]any{"id": "999"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected IsError=true for unknown ID")
		}
		if result.Metadata["errorCode"] != "1" {
			t.Errorf("expected not-found errorCode=1, got content=%q metadata=%#v", result.Content, result.Metadata)
		}
	})

	t.Run("error on empty ID", func(t *testing.T) {
		result, err := del.Execute(ctx, map[string]any{"id": "  "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected IsError=true for empty ID")
		}
	})

	t.Run("rejects unknown fields", func(t *testing.T) {
		created, err := create.Execute(ctx, map[string]any{"cron": "* * * * *", "prompt": "keep on invalid delete"})
		if err != nil || created.IsError {
			t.Fatalf("create strict fixture: err=%v result=%#v", err, created)
		}
		id := extractCronID(t, created.Content)
		result, err := del.Execute(ctx, map[string]any{"id": id, "extra": true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError || !strings.Contains(result.Content, `unknown field "extra"`) {
			t.Fatalf("expected strict unknown-field error, got %#v", result)
		}
		if jobs := store.list(); len(jobs) != 1 || jobs[0].ID != id {
			t.Fatalf("strict rejection mutated cron state: %#v", jobs)
		}
		if cleanup, cleanupErr := del.Execute(ctx, map[string]any{"id": id}); cleanupErr != nil || cleanup.IsError {
			t.Fatalf("cleanup strict fixture: err=%v result=%#v", cleanupErr, cleanup)
		}
	})

	t.Run("rejects wrong id type", func(t *testing.T) {
		result, err := del.Execute(ctx, map[string]any{"id": 123})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError || !strings.Contains(result.Content, "cannot unmarshal") {
			t.Fatalf("expected id type error, got %#v", result)
		}
	})
}

func TestCronDeleteToolOutputContract(t *testing.T) {
	store := newTestCronStore(t)
	create := NewCronCreateTool(store)
	del := NewCronDeleteTool(store)
	schema := del.Schema()
	if schema.Type != "object" || !schema.RejectsUnknownFields() || len(schema.Required) != 1 || schema.Required[0] != "id" {
		t.Fatalf("unexpected strict delete schema: %#v", schema)
	}
	contract := del.ToolContract()
	if !contract.Strict || contract.OutputSchema == nil || contract.MaxResultSizeChars != 100_000 {
		t.Fatalf("unexpected delete contract: %#v", contract)
	}
	if len(contract.OutputSchema.Required) != 1 || contract.OutputSchema.Required[0] != "id" {
		t.Fatalf("unexpected delete output schema: %#v", contract.OutputSchema)
	}

	createResult, err := create.Execute(context.Background(), map[string]any{"cron": "* * * * *", "prompt": "hello"})
	if err != nil || createResult.IsError {
		t.Fatalf("create: err=%v result=%#v", err, createResult)
	}
	id := extractCronID(t, createResult.Content)

	result, err := del.Execute(context.Background(), map[string]any{"id": id})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if result.IsError {
		t.Fatalf("delete tool error: %s", result.Content)
	}
	if want := "Cancelled job " + id + "."; result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
	if result.Metadata["id"] != id {
		t.Fatalf("metadata id = %q, want %q", result.Metadata["id"], id)
	}
	if output, ok := result.Data.(cronDeleteOutput); !ok || output.ID != id {
		t.Fatalf("structured output = %#v, want id %q", result.Data, id)
	}

	block := types.MapToolResult(del, result, "toolu_delete")
	if block.Content != "Cancelled job "+id+"." {
		t.Fatalf("mapped content = %q", block.Content)
	}
	if output, ok := block.Data.(cronDeleteOutput); !ok || output.ID != id {
		t.Fatalf("mapped data = %#v, want id %q", block.Data, id)
	}
}

func TestCronDeleteConcurrentSameID(t *testing.T) {
	for _, durable := range []bool{false, true} {
		durable := durable
		name := "session"
		if durable {
			name = "durable"
		}
		t.Run(name, func(t *testing.T) {
			store := newTestCronStore(t)
			create := NewCronCreateTool(store)
			del := NewCronDeleteTool(store)
			ctx := context.Background()

			created, err := create.Execute(ctx, map[string]any{
				"cron": "* * * * *", "prompt": "delete once", "durable": durable,
			})
			if err != nil || created.IsError {
				t.Fatalf("create: err=%v result=%#v", err, created)
			}
			id := extractCronID(t, created.Content)

			const callers = 8
			results := make(chan types.ToolResult, callers)
			errs := make(chan error, callers)
			var wg sync.WaitGroup
			for range callers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					result, executeErr := del.Execute(ctx, map[string]any{"id": id})
					if executeErr != nil {
						errs <- executeErr
						return
					}
					results <- result
				}()
			}
			wg.Wait()
			close(results)
			close(errs)
			for executeErr := range errs {
				t.Errorf("concurrent delete: %v", executeErr)
			}

			successes := 0
			notFound := 0
			for result := range results {
				switch {
				case !result.IsError:
					successes++
				case result.Metadata["errorCode"] == "1":
					notFound++
				default:
					t.Errorf("unexpected concurrent delete result: %#v", result)
				}
			}
			if successes != 1 || notFound != callers-1 {
				t.Fatalf("successes=%d notFound=%d, want 1/%d", successes, notFound, callers-1)
			}
			if jobs := store.list(); len(jobs) != 0 {
				t.Fatalf("concurrent delete left jobs: %#v", jobs)
			}
			if _, ok := store.cachedNextFire(id); ok {
				t.Fatalf("concurrent delete left scheduler cache for %q", id)
			}
		})
	}
}

// ─── CronListTool ─────────────────────────────────────────────────────────────

func TestCronListTool(t *testing.T) {
	store := newTestCronStore(t)
	create := NewCronCreateTool(store)
	list := NewCronListTool(store)
	ctx := context.Background()

	t.Run("schema is strict empty object", func(t *testing.T) {
		schema := list.Schema()
		if schema.Type != "object" || len(schema.Properties) != 0 || len(schema.Required) != 0 || !schema.RejectsUnknownFields() {
			t.Fatalf("unexpected schema: %#v", schema)
		}
	})

	t.Run("empty store returns no-jobs message", func(t *testing.T) {
		result, err := list.Execute(ctx, map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content != "No scheduled jobs." {
			t.Errorf("unexpected content: %s", result.Content)
		}
		data, ok := result.Data.(cronListOutput)
		if !ok || len(data.Jobs) != 0 {
			t.Fatalf("expected empty structured jobs, got %#v", result.Data)
		}
	})

	t.Run("lists jobs after creation with TS text and data shape", func(t *testing.T) {
		longPrompt := strings.Repeat("x", 90)
		create.Execute(ctx, map[string]any{"cron": "*/5 * * * *", "prompt": "health check"})
		create.Execute(ctx, map[string]any{"cron": "0 9 * * 1", "prompt": longPrompt, "recurring": false})

		result, err := list.Execute(ctx, map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(result.Content, "state_file") || strings.Contains(result.Content, "concurrent_safe") ||
			strings.Contains(result.Content, "created_at") || strings.Contains(result.Content, "next_fire") ||
			strings.Contains(result.Content, "expires_in") || strings.Contains(result.Content, "EXPIRES") {
			t.Fatalf("CronList leaked Go-only diagnostics: %s", result.Content)
		}
		if !strings.Contains(result.Content, "Every 5 minutes (recurring) [session-only]: health check") {
			t.Errorf("expected recurring session-only TS line, got: %s", result.Content)
		}
		wantTruncated := strings.Repeat("x", 79) + "…"
		if !strings.Contains(result.Content, "Every Monday at 9:00 AM (one-shot) [session-only]: "+wantTruncated) {
			t.Errorf("expected one-shot truncated prompt line, got: %s", result.Content)
		}
		data, ok := result.Data.(cronListOutput)
		if !ok {
			t.Fatalf("expected cronListOutput, got %#v", result.Data)
		}
		if len(data.Jobs) != 2 {
			t.Fatalf("expected two jobs, got %#v", data.Jobs)
		}
		if data.Jobs[0].Recurring == nil || !*data.Jobs[0].Recurring || data.Jobs[0].Durable == nil || *data.Jobs[0].Durable {
			t.Fatalf("unexpected optional fields for recurring session job: %#v", data.Jobs[0])
		}
		if data.Jobs[1].Recurring != nil || data.Jobs[1].Durable == nil || *data.Jobs[1].Durable {
			t.Fatalf("unexpected optional fields for one-shot session job: %#v", data.Jobs[1])
		}
		golden := data
		golden.Jobs = append([]cronListJobOutput(nil), data.Jobs...)
		golden.Jobs[0].ID = "recurring-job"
		golden.Jobs[1].ID = "one-shot-job"
		encoded, err := json.Marshal(golden)
		if err != nil {
			t.Fatalf("marshal structured CronList output: %v", err)
		}
		wantJSON := `{"jobs":[{"id":"recurring-job","cron":"*/5 * * * *","humanSchedule":"Every 5 minutes","prompt":"health check","recurring":true,"durable":false},{"id":"one-shot-job","cron":"0 9 * * 1","humanSchedule":"Every Monday at 9:00 AM","prompt":"` + longPrompt + `","durable":false}]}`
		if got := string(encoded); got != wantJSON {
			t.Fatalf("structured CronList JSON differs from TS projection:\ngot  %s\nwant %s", got, wantJSON)
		}

		block := types.MapToolResult(list, result, "toolu_list")
		if block.Content != result.Content {
			t.Fatalf("mapped content mismatch:\nblock=%q\nresult=%q", block.Content, result.Content)
		}
	})

	t.Run("truncates prompts by TS display width and grapheme boundaries", func(t *testing.T) {
		cjkPrompt := strings.Repeat("界", 50)
		if got, want := truncateCronListPrompt(cjkPrompt), strings.Repeat("界", 39)+"…"; got != want {
			t.Fatalf("CJK truncation = %q, want %q", got, want)
		}

		family := "👨‍👩‍👧‍👦"
		emojiPrompt := strings.Repeat(family, 41)
		if got, want := truncateCronListPrompt(emojiPrompt), strings.Repeat(family, 39)+"…"; got != want {
			t.Fatalf("emoji truncation split a grapheme or used rune count:\ngot  %q\nwant %q", got, want)
		}
	})

	t.Run("IsConcurrentSafe returns true", func(t *testing.T) {
		if !list.IsConcurrentSafe() {
			t.Error("CronListTool should be concurrent safe")
		}
	})

	t.Run("rejects removed format parameter", func(t *testing.T) {
		result, err := list.Execute(ctx, map[string]any{"format": "json"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError || !strings.Contains(result.Content, "unexpected parameter `format`") {
			t.Fatalf("expected strict format rejection, got %#v", result)
		}
	})
}

func TestCronListFeatureDescriptionReflectsDurableGate(t *testing.T) {
	t.Setenv("CLAUDE_CODE_DISABLE_DURABLE_CRON", "")
	enabled := NewCronListTool(newTestCronStore(t)).Description()
	if !strings.Contains(enabled, "durable") || !strings.Contains(enabled, "session-only") {
		t.Fatalf("durable-enabled description should mention durable and session-only scope: %q", enabled)
	}

	t.Setenv("CLAUDE_CODE_DISABLE_DURABLE_CRON", "1")
	disabled := NewCronListTool(newTestCronStore(t)).Description()
	if strings.Contains(disabled, "scheduled_tasks.json") || !strings.Contains(disabled, "in this session") {
		t.Fatalf("durable-disabled description should be session-only: %q", disabled)
	}
}

// ─── ParseCron ────────────────────────────────────────────────────────────────

func TestParseCron(t *testing.T) {
	t.Run("wildcard every field", func(t *testing.T) {
		sched, err := ParseCron("* * * * *")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// All minutes should be valid.
		for i := 0; i <= 59; i++ {
			if !sched.Minute.Has(i) {
				t.Errorf("expected minute %d to be set", i)
			}
		}
	})

	t.Run("specific number", func(t *testing.T) {
		sched, err := ParseCron("5 3 * * *")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !sched.Minute.Has(5) {
			t.Error("expected minute 5")
		}
		if sched.Minute.Has(6) {
			t.Error("did not expect minute 6")
		}
		if !sched.Hour.Has(3) {
			t.Error("expected hour 3")
		}
	})

	t.Run("range", func(t *testing.T) {
		sched, err := ParseCron("1-5 * * * *")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for i := 1; i <= 5; i++ {
			if !sched.Minute.Has(i) {
				t.Errorf("expected minute %d in range", i)
			}
		}
		if sched.Minute.Has(0) || sched.Minute.Has(6) {
			t.Error("values outside range should not be set")
		}
	})

	t.Run("step wildcard */15", func(t *testing.T) {
		sched, err := ParseCron("*/15 * * * *")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, m := range []int{0, 15, 30, 45} {
			if !sched.Minute.Has(m) {
				t.Errorf("expected minute %d for */15", m)
			}
		}
		if sched.Minute.Has(1) || sched.Minute.Has(16) {
			t.Error("non-step values should not be set")
		}
	})

	t.Run("list", func(t *testing.T) {
		sched, err := ParseCron("1,3,5 * * * *")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, m := range []int{1, 3, 5} {
			if !sched.Minute.Has(m) {
				t.Errorf("expected minute %d in list", m)
			}
		}
		if sched.Minute.Has(2) || sched.Minute.Has(4) {
			t.Error("non-list values should not be set")
		}
	})

	t.Run("range with step", func(t *testing.T) {
		sched, err := ParseCron("0-30/10 * * * *")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, m := range []int{0, 10, 20, 30} {
			if !sched.Minute.Has(m) {
				t.Errorf("expected minute %d", m)
			}
		}
		if sched.Minute.Has(5) || sched.Minute.Has(31) {
			t.Error("unexpected values set")
		}
	})

	t.Run("day-of-week range", func(t *testing.T) {
		sched, err := ParseCron("0 9 * * 1-5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for d := 1; d <= 5; d++ {
			if !sched.DayOfWeek.Has(d) {
				t.Errorf("expected day %d (weekday)", d)
			}
		}
		if sched.DayOfWeek.Has(0) || sched.DayOfWeek.Has(6) {
			t.Error("weekend should not be set")
		}
	})

	t.Run("invalid: wrong field count", func(t *testing.T) {
		if _, err := ParseCron("* * *"); err == nil {
			t.Error("expected error for 3-field expression")
		}
	})

	t.Run("invalid: empty", func(t *testing.T) {
		if _, err := ParseCron(""); err == nil {
			t.Error("expected error for empty expression")
		}
	})

	t.Run("invalid: non-numeric value", func(t *testing.T) {
		if _, err := ParseCron("abc * * * *"); err == nil {
			t.Error("expected error for non-numeric minute")
		}
	})

	t.Run("invalid: out-of-range minute", func(t *testing.T) {
		if _, err := ParseCron("60 * * * *"); err == nil {
			t.Error("expected error for minute=60")
		}
	})

	t.Run("invalid: out-of-range hour", func(t *testing.T) {
		if _, err := ParseCron("* 24 * * *"); err == nil {
			t.Error("expected error for hour=24")
		}
	})

	t.Run("invalid: bad step", func(t *testing.T) {
		if _, err := ParseCron("*/0 * * * *"); err == nil {
			t.Error("expected error for step=0")
		}
	})

	t.Run("invalid: bad range order", func(t *testing.T) {
		if _, err := ParseCron("5-3 * * * *"); err == nil {
			t.Error("expected error for inverted range")
		}
	})
}

// ─── CronSchedule.Matches ────────────────────────────────────────────────────

func TestCronScheduleMatches(t *testing.T) {
	// "At 09:05 on weekdays" → 5 9 * * 1-5
	sched, err := ParseCron("5 9 * * 1-5")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Monday 2024-01-08 09:05 — should match.
	match := time.Date(2024, 1, 8, 9, 5, 0, 0, time.UTC)
	if !sched.Matches(match) {
		t.Errorf("expected match at %v", match)
	}

	// Same time but Saturday — should not match.
	sat := time.Date(2024, 1, 6, 9, 5, 0, 0, time.UTC)
	if sched.Matches(sat) {
		t.Errorf("did not expect match on Saturday at %v", sat)
	}

	// Monday but wrong minute — should not match.
	wrongMin := time.Date(2024, 1, 8, 9, 6, 0, 0, time.UTC)
	if sched.Matches(wrongMin) {
		t.Errorf("did not expect match at wrong minute %v", wrongMin)
	}

	// "Every minute" wildcard — should match any time.
	every, err := ParseCron("* * * * *")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	for _, ts := range []time.Time{match, sat, wrongMin} {
		if !every.Matches(ts) {
			t.Errorf("* * * * * should match any time, failed at %v", ts)
		}
	}

	// "*/15 minutes" — 00, 15, 30, 45.
	steps, _ := ParseCron("*/15 * * * *")
	for _, m := range []int{0, 15, 30, 45} {
		ts := time.Date(2024, 1, 8, 10, m, 0, 0, time.UTC)
		if !steps.Matches(ts) {
			t.Errorf("*/15 should match minute %d", m)
		}
	}
	ts7 := time.Date(2024, 1, 8, 10, 7, 0, 0, time.UTC)
	if steps.Matches(ts7) {
		t.Error("*/15 should not match minute 7")
	}
}

// ─── Scheduler fires ─────────────────────────────────────────────────────────

func TestSchedulerFires(t *testing.T) {
	store := newTestCronStore(t)

	// Create a job before starting the scheduler.
	sched, err := ParseCron("* * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	id, err := store.create("* * * * *", "say hello", true, false, sched)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t1 := time.Date(2024, 1, 8, 10, 0, 0, 0, time.UTC)
	store.mu.Lock()
	store.sessionJobs[id].CreatedAt = t1.Add(-time.Minute)
	job := cloneCronJob(store.sessionJobs[id])
	store.mu.Unlock()
	store.removeCachedNextFire(id)
	firstFire, ok := cronNextFireAt(job, t1, time.UTC)
	if !ok {
		t.Fatal("expected first next fire")
	}

	var (
		mu    sync.Mutex
		fired []string
	)

	// Use a manual tick channel and completion barrier so assertions observe
	// the entire scheduler transaction, including lifecycle persistence.
	tickCh := make(chan time.Time, 4)
	tickDone := make(chan time.Time)
	store.startWithCompletion(func(job *CronJob) {
		mu.Lock()
		fired = append(fired, job.ID)
		mu.Unlock()
	}, tickCh, tickDone)
	defer store.Stop()

	// Send a tick at the deterministic jittered time — job should fire.
	processSchedulerTick(t, tickCh, tickDone, firstFire)

	mu.Lock()
	count := len(fired)
	mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 fire after first tick, got %d", count)
	}
	if fired[0] != id {
		t.Errorf("expected job id %s to fire, got %s", id, fired[0])
	}

	// Send same minute again — should NOT double-fire.
	processSchedulerTick(t, tickCh, tickDone, firstFire)

	mu.Lock()
	count = len(fired)
	mu.Unlock()
	if count != 1 {
		t.Errorf("expected no double-fire within same minute, got %d fires", count)
	}

	// Advance to the next jittered fire — should fire again (recurring).
	store.mu.Lock()
	job = cloneCronJob(store.sessionJobs[id])
	store.mu.Unlock()
	t2, ok := cronNextFireAt(job, firstFire, time.UTC)
	if !ok {
		t.Fatal("expected recurring next fire")
	}
	processSchedulerTick(t, tickCh, tickDone, t2)

	mu.Lock()
	count = len(fired)
	mu.Unlock()
	if count != 2 {
		t.Errorf("expected 2 total fires after second minute, got %d", count)
	}
}

func TestSchedulerOneShotDeleted(t *testing.T) {
	store := newTestCronStore(t)

	sched, _ := ParseCron("* * * * *")
	id, err := store.create("* * * * *", "one-shot", false, false, sched)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t1 := time.Date(2024, 1, 8, 10, 7, 0, 0, time.UTC)
	store.mu.Lock()
	store.sessionJobs[id].CreatedAt = t1.Add(-time.Minute)
	job := cloneCronJob(store.sessionJobs[id])
	store.mu.Unlock()
	store.removeCachedNextFire(id)
	firstFire, ok := cronNextFireAt(job, t1, time.UTC)
	if !ok || !firstFire.Equal(t1) {
		t.Fatalf("unexpected one-shot fire: %s ok=%v", firstFire, ok)
	}

	var (
		mu    sync.Mutex
		fired int
	)

	tickCh := make(chan time.Time, 4)
	tickDone := make(chan time.Time)
	store.startWithCompletion(func(job *CronJob) {
		mu.Lock()
		fired++
		mu.Unlock()
	}, tickCh, tickDone)
	defer store.Stop()

	processSchedulerTick(t, tickCh, tickDone, firstFire)

	mu.Lock()
	f := fired
	mu.Unlock()
	if f != 1 {
		t.Fatalf("expected 1 fire, got %d", f)
	}

	// Job should have been deleted; list should be empty.
	jobs := store.list()
	if len(jobs) != 0 {
		t.Errorf("expected one-shot job to be deleted after firing, got %d jobs", len(jobs))
	}

	// Another tick must not fire the deleted job.
	t2 := firstFire.Add(time.Minute)
	processSchedulerTick(t, tickCh, tickDone, t2)

	mu.Lock()
	f = fired
	mu.Unlock()
	if f != 1 {
		t.Errorf("one-shot job fired again after deletion, total fires: %d", f)
	}
}

func TestSchedulerStopClean(t *testing.T) {
	store := newTestCronStore(t)
	tickCh := make(chan time.Time, 1)
	store.startWith(func(_ *CronJob) {}, tickCh)
	store.Stop()
	// Calling Stop a second time should be a no-op (no panic).
	store.Stop()
}

func TestSchedulerStopJoinsInFlightTick(t *testing.T) {
	store := newTestCronStore(t)
	sched, err := ParseCron("* * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	id, err := store.create("* * * * *", "blocking callback", false, false, sched)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	dueAt := time.Date(2024, 1, 8, 10, 7, 0, 0, time.UTC)
	store.mu.Lock()
	store.sessionJobs[id].CreatedAt = dueAt.Add(-time.Minute)
	job := cloneCronJob(store.sessionJobs[id])
	store.mu.Unlock()
	store.removeCachedNextFire(id)
	dueAt, ok := cronNextFireAt(job, dueAt, time.UTC)
	if !ok {
		t.Fatal("expected one-shot next fire")
	}

	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	tickCh := make(chan time.Time, 1)
	store.startWith(func(*CronJob) {
		close(callbackStarted)
		<-releaseCallback
	}, tickCh)
	tickCh <- dueAt
	select {
	case <-callbackStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler callback did not start")
	}

	stopReturned := make(chan struct{})
	go func() {
		store.Stop()
		close(stopReturned)
	}()
	stopStartedDeadline := time.NewTimer(5 * time.Second)
	defer stopStartedDeadline.Stop()
	for {
		store.mu.Lock()
		running := store.running
		store.mu.Unlock()
		if !running {
			break
		}
		select {
		case <-stopStartedDeadline.C:
			t.Fatal("Stop did not begin scheduler shutdown")
		default:
			runtime.Gosched()
		}
	}
	select {
	case <-stopReturned:
		t.Fatal("Stop returned while scheduler callback was still running")
	default:
	}
	close(releaseCallback)
	select {
	case <-stopReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not join scheduler after callback completed")
	}
}

// ─── Concurrency ──────────────────────────────────────────────────────────────

func TestCronStoreConcurrency(t *testing.T) {
	store := newTestCronStore(t)
	create := NewCronCreateTool(store)
	del := NewCronDeleteTool(store)
	ctx := context.Background()

	// Create 10 jobs first.
	createdIDs := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		result, _ := create.Execute(ctx, map[string]any{"cron": "* * * * *", "prompt": "p"})
		createdIDs = append(createdIDs, extractCronID(t, result.Content))
	}

	done := make(chan struct{}, 20)

	// Concurrently create more jobs.
	for i := 0; i < 5; i++ {
		go func() {
			create.Execute(ctx, map[string]any{"cron": "0 * * * *", "prompt": "concurrent"})
			done <- struct{}{}
		}()
	}

	// Concurrently delete existing jobs.
	for i := 0; i < 5; i++ {
		id := createdIDs[i]
		go func() {
			del.Execute(ctx, map[string]any{"id": id})
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// ─── Jitter ──────────────────────────────────────────────────────────────────

func TestRecurringJitterDeterministic(t *testing.T) {
	period := time.Hour
	jobID := "abc123"

	a := RecurringJitter(period, jobID)
	b := RecurringJitter(period, jobID)
	if a != b {
		t.Errorf("expected deterministic jitter for same id, got %v vs %v", a, b)
	}

	// Bound: <= min(period*0.1, 15min)
	bound := time.Duration(float64(period) * 0.10)
	if bound > 15*time.Minute {
		bound = 15 * time.Minute
	}
	if a < 0 || a >= bound {
		t.Errorf("recurring jitter %v out of bounds [0,%v)", a, bound)
	}
}

func TestRecurringJitterCappedAt15Min(t *testing.T) {
	got := RecurringJitter(24*time.Hour, "any-id")
	if got >= 15*time.Minute {
		t.Errorf("expected jitter < 15min for long period, got %v", got)
	}
}

func TestRecurringJitterZeroPeriod(t *testing.T) {
	if got := RecurringJitter(0, "id"); got != 0 {
		t.Errorf("expected 0 jitter for zero period, got %v", got)
	}
	if got := RecurringJitter(-time.Second, "id"); got != 0 {
		t.Errorf("expected 0 jitter for negative period, got %v", got)
	}
}

func TestOneshotJitterEarlyOnRoundBoundary(t *testing.T) {
	target := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	got := OneshotJitter(target, "id-1")
	if got > 0 {
		t.Errorf("expected non-positive jitter on :00, got %v", got)
	}
	if got <= -90*time.Second {
		t.Errorf("expected jitter > -90s, got %v", got)
	}

	target30 := time.Date(2026, 5, 17, 12, 30, 0, 0, time.UTC)
	if got := OneshotJitter(target30, "id-1"); got > 0 {
		t.Errorf("expected non-positive jitter on :30, got %v", got)
	}
}

func TestOneshotJitterZeroOffBoundary(t *testing.T) {
	target := time.Date(2026, 5, 17, 12, 7, 0, 0, time.UTC)
	if got := OneshotJitter(target, "id-1"); got != 0 {
		t.Errorf("expected 0 jitter off boundary, got %v", got)
	}
}

// ─── Sentinel resolver ───────────────────────────────────────────────────────

func TestResolvePromptAutonomousLoop(t *testing.T) {
	got, err := ResolvePrompt("<<autonomous-loop>>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "<<autonomous-loop>>" {
		t.Errorf("expected sentinel to be expanded, got raw: %q", got)
	}
	if !strings.Contains(got, "autonomous") {
		t.Errorf("expected expansion to mention 'autonomous', got: %s", got)
	}
}

func TestResolvePromptDynamicSentinelRejected(t *testing.T) {
	if _, err := ResolvePrompt("<<autonomous-loop-dynamic>>"); err == nil {
		t.Error("expected error for autonomous-loop-dynamic in cron context")
	}
}

func TestResolvePromptUnknownSentinelRejected(t *testing.T) {
	if _, err := ResolvePrompt("<<bogus>>"); err == nil {
		t.Error("expected error for unknown sentinel")
	}
}

func TestResolvePromptPlainPassThrough(t *testing.T) {
	plain := "Run the daily report and email Alice."
	got, err := ResolvePrompt(plain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != plain {
		t.Errorf("expected plain prompt unchanged, got: %s", got)
	}
}

func TestCronCreateRejectsBadSentinel(t *testing.T) {
	store := newTestCronStore(t)
	tool := NewCronCreateTool(store)
	res, err := tool.Execute(context.Background(), map[string]any{
		"cron":   "* * * * *",
		"prompt": "<<autonomous-loop-dynamic>>",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected error for invalid sentinel")
	}
}

// ─── 7-day expiry ────────────────────────────────────────────────────────────

func TestRecurringJob7DayExpiry(t *testing.T) {
	store := newTestCronStore(t)

	sched, err := ParseCron("* * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	id, err := store.create("* * * * *", "old-job", true, false, sched)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Manually backdate CreatedAt to 8 days ago.
	store.mu.Lock()
	job := store.sessionJobs[id]
	job.CreatedAt = time.Now().Add(-8 * 24 * time.Hour)
	store.mu.Unlock()
	store.removeCachedNextFire(id)

	fireAt := time.Now()
	due := store.collectDueJobs(fireAt)
	if len(due) != 1 {
		t.Fatalf("expected 1 fire on expiry boundary, got %d", len(due))
	}
	if due[0].ID != id {
		t.Errorf("unexpected fire id: %s", due[0].ID)
	}

	for _, j := range store.list() {
		if j.ID == id {
			t.Errorf("expected job %s to be removed after 7-day expiry, but it remains", id)
		}
	}
}

func TestRecurringJobYoungNotExpired(t *testing.T) {
	store := newTestCronStore(t)

	sched, _ := ParseCron("* * * * *")
	id, err := store.create("* * * * *", "young-job", true, false, sched)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	fireAt, ok := store.cachedNextFire(id)
	if !ok {
		t.Fatal("expected cached jittered next fire")
	}
	due := store.collectDueJobs(fireAt)
	if len(due) != 1 {
		t.Fatalf("expected 1 fire, got %d", len(due))
	}

	found := false
	for _, j := range store.list() {
		if j.ID == id {
			found = true
		}
	}
	if !found {
		t.Errorf("expected young recurring job %s to survive first fire", id)
	}
}
