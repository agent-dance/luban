package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	gtui "github.com/grindlemire/go-tui"
)

func compactProgressFixture() (*AppState, *RootComponent) {
	state := NewAppState()
	state.SessionID.Set("compact-session")
	state.SessionEpoch.Set(7)
	state.Language.Set(i18n.LangZH)
	root := NewRootComponent(state, nil, nil)
	return state, root
}

func TestCompactionProgressReducerIsMonotonicAndTerminal(t *testing.T) {
	state, _ := compactProgressFixture()
	apply := func(stage string) bool {
		return state.ApplyCompactionProgress("compact-session", 7, stream.ProgressEvent{
			Stage: stage, Metadata: map[string]any{"trigger": "manual"},
		})
	}
	for _, stage := range []string{"compact_accepted", "compact_preparing", "compact_summarizing", "compact_installing", "compact_persisting"} {
		if !apply(stage) {
			t.Fatalf("stage %q was rejected", stage)
		}
	}
	if apply("compact_summarizing") {
		t.Fatal("late summarizing stage regressed a persisting compaction")
	}
	if !apply("compact_end") || state.CompactionProgress.Get().Stage != CompactionProgressCompleted {
		t.Fatalf("terminal state = %+v", state.CompactionProgress.Get())
	}
	if apply("compact_installing") {
		t.Fatal("terminal compaction accepted a late running stage")
	}
}

func TestRunningCompactionRendersIndeterminateMotionElapsedCancelAndQueue(t *testing.T) {
	state, root := compactProgressFixture()
	if !state.ApplyCompactionProgress("compact-session", 7, stream.ProgressEvent{
		Stage: "compact_summarizing", Metadata: map[string]any{"trigger": "manual"},
	}) {
		t.Fatal("summarizing progress rejected")
	}
	status := state.CompactionProgress.Get()
	status.StartedAt = time.Unix(100, 0)
	root.now = func() time.Time { return time.Unix(184, 0) }
	root.compactProgressFrame.Set(2)
	text := collectElementText(root.renderCompactionProgress(status, 2))
	for _, want := range []string{"正在压缩上下文", "生成 LLM 摘要", "01:24", "Esc", "2 条等待中"} {
		if !strings.Contains(text, want) {
			t.Fatalf("running compaction omitted %q: %q", want, text)
		}
	}
	if strings.Contains(text, "%") {
		t.Fatalf("indeterminate progress invented a percentage: %q", text)
	}
	if first, second := compactIndeterminateBar(2, 12), compactIndeterminateBar(3, 12); first == second {
		t.Fatalf("indeterminate frames did not move: %q", first)
	}
}

func TestCompletedCompactionUsesStatusBarWithoutAStandaloneReceipt(t *testing.T) {
	state, root := compactProgressFixture()
	state.ApplyCompactionProgress("compact-session", 7, stream.ProgressEvent{Stage: "compact_summarizing", Metadata: map[string]any{"trigger": "manual"}})
	state.ApplyCompactionBoundary("compact-session", 7, stream.CompactBoundaryEvent{
		PreCompactTokenCount: 50098, TruePostCompactTokenCount: 30131,
	})
	state.ApplyCompactionProgress("compact-session", 7, stream.ProgressEvent{
		Stage: "compact_end",
		Metadata: map[string]any{
			"trigger": "manual", "before_messages": 45, "after_messages": 25, "measurement": "local_estimate",
		},
	})
	status := state.CompactionProgress.Get()
	status.StartedAt = time.Unix(100, 0)
	status.UpdatedAt = time.Unix(231, 0)
	text := collectElementText(root.renderCompactionProgress(status, 0))
	if text != "" || compactionProgressRows(status) != 0 {
		t.Fatalf("completed compaction still occupied standalone rows: %q", text)
	}
}

func TestProgressiveContextMetricsAccumulateOnlyAppliedReceipts(t *testing.T) {
	state, _ := compactProgressFixture()
	applied := stream.ProgressEvent{Stage: "progressive_context_projection", Metadata: map[string]any{
		"applied": true, "projection_count": 2, "tokens_saved": 12_345, "estimated_net_savings_usd": 0.0042,
	}}
	if !state.ApplyProgressiveContextMetrics("compact-session", 7, "projection:1", applied) {
		t.Fatal("applied progressive receipt was rejected")
	}
	if state.ApplyProgressiveContextMetrics("compact-session", 7, "projection:1", applied) {
		t.Fatal("duplicate progressive receipt was counted twice")
	}
	rejected := applied
	rejected.Metadata = map[string]any{"applied": false, "projection_count": 3, "tokens_saved": 9_999}
	if state.ApplyProgressiveContextMetrics("compact-session", 7, "projection:2", rejected) {
		t.Fatal("rejected candidate was counted as savings")
	}
	shadow := applied
	shadow.Metadata = map[string]any{"applied": true, "shadow": true, "projection_count": 3, "tokens_saved": 9_999}
	if state.ApplyProgressiveContextMetrics("compact-session", 7, "projection:3", shadow) {
		t.Fatal("shadow candidate was counted as savings")
	}
	usage := state.ActiveSessionUsage()
	if usage.ProgressiveProjectionCount != 1 || usage.ProgressiveProjectedTools != 2 || usage.ProgressiveTokensSaved != 12_345 || usage.ProgressiveSavingsUSD != 0.0042 {
		t.Fatalf("progressive benefit ledger = %+v", usage)
	}
}

func TestProgressiveContextMetricsTracksPendingSnapshotWithoutAddingSavings(t *testing.T) {
	state, _ := compactProgressFixture()
	pending := stream.ProgressEvent{Stage: "progressive_context_projection", Metadata: map[string]any{
		"pending_only": true, "pending_tools": 3, "pending_tokens": 5_432,
	}}
	if !state.ApplyProgressiveContextMetrics("compact-session", 7, "pending:1", pending) {
		t.Fatal("pending progressive snapshot was rejected")
	}
	usage := state.ActiveSessionUsage()
	if usage.ProgressiveProjectionCount != 0 || usage.ProgressiveTokensSaved != 0 || state.ProgressivePendingTools.Get() != 3 || state.ProgressivePendingTokens.Get() != 5_432 {
		t.Fatalf("pending snapshot changed realized ledger: usage=%+v pending=%d/%d", usage, state.ProgressivePendingTools.Get(), state.ProgressivePendingTokens.Get())
	}
	pending.Metadata = map[string]any{"pending_only": true, "pending_tools": 0, "pending_tokens": 0}
	if !state.ApplyProgressiveContextMetrics("compact-session", 7, "pending:2", pending) || state.ProgressivePendingTools.Get() != 0 || state.ProgressivePendingTokens.Get() != 0 {
		t.Fatalf("pending snapshot did not clear: %d/%d", state.ProgressivePendingTools.Get(), state.ProgressivePendingTokens.Get())
	}
}

func TestCompactionProgressRejectsWrongSessionAndEpoch(t *testing.T) {
	state, _ := compactProgressFixture()
	event := stream.ProgressEvent{Stage: "compact_accepted"}
	if state.ApplyCompactionProgress("other", 7, event) || state.ApplyCompactionProgress("compact-session", 8, event) {
		t.Fatal("foreign compaction progress crossed the session fence")
	}
}

func TestCancelledCompactionDoesNotRenderRacingProviderFailure(t *testing.T) {
	state, root := compactProgressFixture()
	state.ApplyCompactionProgress("compact-session", 7, stream.ProgressEvent{Stage: "compact_summarizing"})
	state.ApplyCompactionProgress("compact-session", 7, stream.ProgressEvent{
		Stage:    "compact_cancelled",
		Metadata: map[string]any{"error": "response did not contain valid text"},
	})
	text := collectElementText(root.renderCompactionProgress(state.CompactionProgress.Get(), 0))
	if !strings.Contains(text, "已取消上下文压缩") {
		t.Fatalf("cancelled terminal missing: %q", text)
	}
	if strings.Contains(text, "失败") || strings.Contains(text, "valid text") || strings.Contains(text, "原因") {
		t.Fatalf("cancelled terminal leaked racing provider failure: %q", text)
	}
}

func TestFailedCompactionRetainsActionableCause(t *testing.T) {
	state, root := compactProgressFixture()
	state.ApplyCompactionProgress("compact-session", 7, stream.ProgressEvent{Stage: "compact_summarizing"})
	state.ApplyCompactionProgress("compact-session", 7, stream.ProgressEvent{
		Stage: "compact_failed", Metadata: map[string]any{"error": "provider timed out"},
	})
	status := state.CompactionProgress.Get()
	text := collectElementText(root.renderCompactionProgress(status, 0))
	if compactionProgressRows(status) != 2 || !strings.Contains(text, "上下文压缩失败") || !strings.Contains(text, "provider timed out") {
		t.Fatalf("failed compaction lost actionable cause: rows=%d text=%q", compactionProgressRows(status), text)
	}
}

func TestEscapeCancelsRunningCompaction(t *testing.T) {
	state, root := compactProgressFixture()
	cancelled := false
	state.SetQueryCancel(func() { cancelled = true })
	state.ApplyCompactionProgress("compact-session", 7, stream.ProgressEvent{Stage: "compact_summarizing"})
	if stopped := dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEscape}); !stopped {
		t.Fatal("Escape did not stop propagation while compaction was active")
	}
	if !cancelled {
		t.Fatal("Escape did not invoke the active compaction cancel function")
	}
}

func TestFullRootFrameKeepsCompactionProgressVisible(t *testing.T) {
	state, root := compactProgressFixture()
	state.ApplyCompactionProgress("compact-session", 7, stream.ProgressEvent{Stage: "compact_summarizing"})
	frame := renderElementText(root.renderAtSize(nil, 100, 24), 100, 24)
	compactFrame := strings.ReplaceAll(frame, " ", "")
	if !strings.Contains(compactFrame, "正在压缩上下文") || !strings.Contains(compactFrame, "生成LLM摘要") || !strings.Contains(compactFrame, "Esc") {
		t.Fatalf("full TUI frame omitted active compaction progress:\n%s", frame)
	}
}
