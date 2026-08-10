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

func TestCompletedCompactionConvergesToOneReceiptAndExplainsCalibration(t *testing.T) {
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
	for _, want := range []string{"上下文已压缩", "50098", "30131", "45", "25", "02:11", "本地估算", "provider", "校准"} {
		if !strings.Contains(text, want) {
			t.Fatalf("completed compaction omitted %q: %q", want, text)
		}
	}
	if strings.Contains(text, "生成 LLM 摘要") || strings.Contains(text, "Esc") {
		t.Fatalf("terminal receipt retained running copy: %q", text)
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
