package compact

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/types"
)

func TestSummaryCompactorRunsCompactHooksAndMergesInstructions(t *testing.T) {
	var seenInstructions string
	sc := &SummaryCompactor{
		KeepRecent:         1,
		CustomInstructions: "user instructions",
		HookRunner: hooks.NewRunner([]hooks.Hook{
			{Type: hooks.HookPreCompact, Command: `printf '{"new_custom_instructions":"hook instructions","user_display_message":"pre display"}'`},
			{Type: hooks.HookSessionStart, Command: `printf '{"system_reminder":"session-start reminder"}'`},
			{Type: hooks.HookPostCompact, Command: `printf '{"user_display_message":"post display"}'`},
		}),
		SummarizeMessages: func(_ context.Context, _ []types.Message, instructions string) (string, error) {
			seenInstructions = instructions
			return "<summary>hook summary</summary>", nil
		},
	}

	result, err := sc.CompactWithTrigger(context.Background(), []types.Message{
		types.UserMessage("old one"),
		types.AssistantMessage("old two"),
		types.UserMessage("tail"),
	}, 1, "manual")
	if err != nil {
		t.Fatalf("CompactWithTrigger: %v", err)
	}
	if seenInstructions != "user instructions\n\nhook instructions" {
		t.Fatalf("merged instructions = %q", seenInstructions)
	}
	if result.UserDisplayMessage != "pre display\npost display" {
		t.Fatalf("display message = %q", result.UserDisplayMessage)
	}
	if len(result.HookResults) != 1 || !strings.Contains(result.HookResults[0].GetText(), "session-start reminder") {
		t.Fatalf("session-start hook results = %#v", result.HookResults)
	}
}

func TestSummaryCompactorPreCompactHookBlockFails(t *testing.T) {
	sc := &SummaryCompactor{
		KeepRecent: 1,
		HookRunner: hooks.NewRunner([]hooks.Hook{{
			Type:    hooks.HookPreCompact,
			Command: `echo blocked >&2; exit 2`,
		}}),
		SummarizeMessages: func(context.Context, []types.Message, string) (string, error) {
			t.Fatal("summarizer should not run after blocking PreCompact hook")
			return "", nil
		},
	}

	_, err := sc.Compact(context.Background(), []types.Message{
		types.UserMessage("old"),
		types.AssistantMessage("old"),
		types.UserMessage("tail"),
	}, 1)
	if err == nil || !strings.Contains(err.Error(), "PreCompact hook blocked compaction") {
		t.Fatalf("error = %v, want PreCompact block", err)
	}
}

func TestSummaryCompactorHookCancellationFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sc := &SummaryCompactor{
		KeepRecent: 1,
		HookRunner: hooks.NewRunner([]hooks.Hook{{
			Type:    hooks.HookPreCompact,
			Command: `sleep 2`,
			Timeout: 5,
		}}),
		SummarizeMessages: func(context.Context, []types.Message, string) (string, error) {
			t.Fatal("summarizer should not run after cancelled PreCompact hook")
			return "", nil
		},
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := sc.Compact(ctx, []types.Message{
		types.UserMessage("old"),
		types.AssistantMessage("old"),
		types.UserMessage("tail"),
	}, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
