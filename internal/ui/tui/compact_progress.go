package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	gtui "github.com/grindlemire/go-tui"
)

const compactProgressFrameInterval = 125 * time.Millisecond

type CompactionProgressStage string

const (
	CompactionProgressPreparing   CompactionProgressStage = "preparing"
	CompactionProgressSummarizing CompactionProgressStage = "summarizing"
	CompactionProgressInstalling  CompactionProgressStage = "installing"
	CompactionProgressPersisting  CompactionProgressStage = "persisting"
	CompactionProgressCompleted   CompactionProgressStage = "completed"
	CompactionProgressFailed      CompactionProgressStage = "failed"
	CompactionProgressCancelled   CompactionProgressStage = "cancelled"
)

// CompactionProgressStatus is transient UI state for one context-compaction
// transaction. Protocol stages remain untranslated here; the active runtime
// language is read on every render.
type CompactionProgressStatus struct {
	SessionID      string
	Epoch          uint64
	Trigger        string
	Stage          CompactionProgressStage
	StartedAt      time.Time
	UpdatedAt      time.Time
	BeforeTokens   int
	AfterTokens    int
	BeforeMessages int
	AfterMessages  int
	LocalEstimate  bool
	Error          string
}

func (s CompactionProgressStatus) Running() bool {
	switch s.Stage {
	case CompactionProgressCompleted, CompactionProgressFailed, CompactionProgressCancelled:
		return false
	default:
		return true
	}
}

func compactionProgressStage(stage string) (CompactionProgressStage, int, bool) {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "compact_start", "compact_accepted", "compact_preparing", "auto_compact_attempt":
		return CompactionProgressPreparing, 1, true
	case "compact_summarizing":
		return CompactionProgressSummarizing, 2, true
	case "compact_installing":
		return CompactionProgressInstalling, 3, true
	case "compact_persisting":
		return CompactionProgressPersisting, 4, true
	case "compact_end", "compact_success", "auto_compact_success":
		return CompactionProgressCompleted, 5, true
	case "compact_failed", "compact_failure", "auto_compact_failure":
		return CompactionProgressFailed, 5, true
	case "compact_cancelled":
		return CompactionProgressCancelled, 5, true
	default:
		return "", 0, false
	}
}

func compactionProgressStageRank(stage CompactionProgressStage) int {
	switch stage {
	case CompactionProgressPreparing:
		return 1
	case CompactionProgressSummarizing:
		return 2
	case CompactionProgressInstalling:
		return 3
	case CompactionProgressPersisting:
		return 4
	case CompactionProgressCompleted, CompactionProgressFailed, CompactionProgressCancelled:
		return 5
	default:
		return 0
	}
}

func compactionMetadataInt(metadata map[string]any, key string) int {
	switch value := metadata[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func (s *AppState) ApplyCompactionProgress(sessionID string, epoch uint64, progress stream.ProgressEvent) bool {
	stage, rank, ok := compactionProgressStage(progress.Stage)
	if !ok || s == nil || sessionID == "" || s.SessionID.Get() != sessionID || s.SessionEpoch.Get() != epoch {
		return false
	}
	now := time.Now()
	current := s.CompactionProgress.Get()
	if current != nil {
		if !current.Running() && rank < 5 {
			// A fresh accepted/preparing event starts the next transaction.
			if stage != CompactionProgressPreparing {
				return false
			}
			current = nil
		} else if current.Running() && rank < compactionProgressStageRank(current.Stage) {
			return false
		} else if !current.Running() {
			return false
		}
	}
	next := &CompactionProgressStatus{SessionID: sessionID, Epoch: epoch, Stage: stage, StartedAt: now, UpdatedAt: now}
	if current != nil {
		copy := *current
		next = &copy
		next.Stage = stage
		next.UpdatedAt = now
	}
	if trigger, ok := progress.Metadata["trigger"].(string); ok && strings.TrimSpace(trigger) != "" {
		next.Trigger = strings.ToLower(strings.TrimSpace(trigger))
	}
	if before := compactionMetadataInt(progress.Metadata, "pre_compact_token_count"); before > 0 {
		next.BeforeTokens = before
	}
	if after := compactionMetadataInt(progress.Metadata, "post_compact_token_count"); after > 0 {
		next.AfterTokens = after
	}
	if before := compactionMetadataInt(progress.Metadata, "before_messages"); before > 0 {
		next.BeforeMessages = before
	}
	if after := compactionMetadataInt(progress.Metadata, "after_messages"); after > 0 {
		next.AfterMessages = after
	}
	if measurement, ok := progress.Metadata["measurement"].(string); ok {
		next.LocalEstimate = measurement == "local_estimate"
	}
	if rawError, ok := progress.Metadata["error"].(string); ok {
		next.Error = strings.TrimSpace(rawError)
	}
	s.CompactionProgress.Set(next)
	return true
}

func (s *AppState) ApplyCompactionBoundary(sessionID string, epoch uint64, boundary stream.CompactBoundaryEvent) bool {
	if s == nil || s.SessionID.Get() != sessionID || s.SessionEpoch.Get() != epoch {
		return false
	}
	current := s.CompactionProgress.Get()
	if current == nil || current.SessionID != sessionID || current.Epoch != epoch {
		return false
	}
	next := *current
	next.BeforeTokens = boundary.PreCompactTokenCount
	next.AfterTokens = boundary.TruePostCompactTokenCount
	if next.AfterTokens == 0 {
		next.AfterTokens = boundary.PostCompactTokenCount
	}
	next.LocalEstimate = true
	next.UpdatedAt = time.Now()
	s.CompactionProgress.Set(&next)
	return true
}

func compactProgressStageText(lang i18n.Language, stage CompactionProgressStage) string {
	key := i18n.KeyTUICompactProgressPreparing
	switch stage {
	case CompactionProgressSummarizing:
		key = i18n.KeyTUICompactProgressSummarizing
	case CompactionProgressInstalling:
		key = i18n.KeyTUICompactProgressInstalling
	case CompactionProgressPersisting:
		key = i18n.KeyTUICompactProgressPersisting
	}
	return i18n.Text(lang, key)
}

func formatCompactProgressElapsed(elapsed time.Duration) string {
	if elapsed < 0 {
		elapsed = 0
	}
	seconds := int(elapsed / time.Second)
	if seconds >= 3600 {
		return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds%3600/60, seconds%60)
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func compactIndeterminateBar(frame uint64, width int) string {
	if width < 5 {
		width = 5
	}
	if width > 16 {
		width = 16
	}
	cells := make([]rune, width)
	for i := range cells {
		cells[i] = '░'
	}
	span := 3
	cycle := width + span
	head := int(frame % uint64(cycle))
	for offset := 0; offset < span; offset++ {
		index := head - offset
		if index >= 0 && index < width {
			cells[index] = '█'
		}
	}
	return string(cells)
}

func (c *RootComponent) renderCompactionProgress(status *CompactionProgressStatus, queued int) *gtui.Element {
	if status == nil {
		return gtui.New(gtui.WithHeight(0), gtui.WithWidthPercent(100))
	}
	lang := c.state.Language.Get()
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	elapsed := now.Sub(status.StartedAt)
	if !status.Running() {
		elapsed = status.UpdatedAt.Sub(status.StartedAt)
	}
	duration := formatCompactProgressElapsed(elapsed)
	container := gtui.New(gtui.WithDirection(gtui.Column), gtui.WithWidthPercent(100), gtui.WithHeight(2))
	if status.Running() {
		line := i18n.Text(lang, i18n.KeyTUICompactProgressTitle) + "  " + compactIndeterminateBar(c.compactProgressFrame.Get(), 12) +
			"  " + compactProgressStageText(lang, status.Stage) + " · " + i18n.Format(lang, i18n.KeyTUICompactProgressElapsedCancel, duration)
		queueText := i18n.Text(lang, i18n.KeyTUICompactProgressInputQueues)
		if queued > 0 {
			queueText = i18n.Format(lang, i18n.KeyTUICompactProgressInputQueued, queued)
		}
		container.AddChild(gtui.New(gtui.WithText(line), gtui.WithTextStyle(gtui.NewStyle().Foreground(gtui.Cyan).Bold()), gtui.WithHeight(1), gtui.WithTruncate(true)))
		container.AddChild(gtui.New(gtui.WithText(queueText), gtui.WithTextStyle(gtui.NewStyle().Dim()), gtui.WithHeight(1), gtui.WithTruncate(true)))
		return container
	}
	style := gtui.NewStyle().Foreground(gtui.Green)
	line := i18n.Format(lang, i18n.KeyTUICompactProgressCompletedNoCounts, duration)
	switch status.Stage {
	case CompactionProgressCompleted:
		if status.BeforeTokens > 0 && status.AfterTokens > 0 && status.BeforeMessages > 0 && status.AfterMessages > 0 {
			line = i18n.Format(lang, i18n.KeyTUICompactProgressCompleted,
				status.BeforeTokens, status.AfterTokens, status.BeforeMessages, status.AfterMessages, duration)
		}
	case CompactionProgressFailed:
		style = gtui.NewStyle().Foreground(gtui.Red)
		line = i18n.Format(lang, i18n.KeyTUICompactProgressFailed, duration)
	case CompactionProgressCancelled:
		style = gtui.NewStyle().Foreground(gtui.Yellow)
		line = i18n.Format(lang, i18n.KeyTUICompactProgressCancelled, duration)
	}
	container.AddChild(gtui.New(gtui.WithText(line), gtui.WithTextStyle(style.Bold()), gtui.WithHeight(1), gtui.WithTruncate(true)))
	detail := ""
	if status.Stage == CompactionProgressCompleted && status.LocalEstimate {
		detail = i18n.Text(lang, i18n.KeyTUICompactProgressProviderCalibration)
	} else if status.Stage == CompactionProgressFailed && status.Error != "" {
		detail = i18n.Format(lang, i18n.KeyTUICompactProgressCause, sanitizePresentationTerminalText(status.Error))
	}
	container.AddChild(gtui.New(gtui.WithText(detail), gtui.WithTextStyle(gtui.NewStyle().Dim()), gtui.WithHeight(1), gtui.WithTruncate(true)))
	return container
}

func (c *RootComponent) tickCompactProgress() {
	status := c.state.CompactionProgress.Get()
	if status == nil || !status.Running() {
		if c.compactProgressFrame.Get() != 0 {
			c.compactProgressFrame.Set(0)
		}
		return
	}
	c.compactProgressFrame.Set(c.compactProgressFrame.Get() + 1)
}

type compactProgressWatcher struct{ root *RootComponent }

func (w compactProgressWatcher) Start(eventQueue chan<- func(), stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(compactProgressFrameInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				status := w.root.state.CompactionProgress.Get()
				if status == nil || !status.Running() {
					continue
				}
				select {
				case eventQueue <- w.root.tickCompactProgress:
				case <-stopCh:
					return
				}
			}
		}
	}()
}
