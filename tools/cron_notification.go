package tools

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// escapeMarkdownFence makes a user-supplied string safe to embed inside a
// triple-backtick markdown fence. It escapes backticks and any 3+ run of
// backticks that could close the surrounding fence, and replaces backslashes
// only when followed by a backtick (to avoid double-escaping arbitrary text).
//
// Mirrors the TS scheduler's buildMissedTaskNotification fence escape.
func escapeMarkdownFence(s string) string {
	if s == "" {
		return s
	}
	// Replace runs of 3+ backticks with a zero-width-joined version so the
	// surrounding code fence is preserved.
	out := strings.ReplaceAll(s, "```", "`​``")
	return out
}

// MissedRun describes a one-shot cron job that should have fired while the
// scheduler was offline.
type MissedRun struct {
	JobID       string
	Cron        string
	Prompt      string
	CreatedAt   time.Time
	LastFiredAt *time.Time
	MissedSince time.Time
	MissedCount int
}

// BuildMissedTaskNotification renders a markdown notification listing the
// jobs that were missed while the scheduler was idle. The result is suitable
// for direct injection into a chat-message body. Backticks in user-supplied
// prompts and IDs are escaped so they can't break out of the surrounding code
// fence.
func BuildMissedTaskNotification(missed []MissedRun) string {
	if len(missed) == 0 {
		return ""
	}
	plural := len(missed) > 1
	var sb strings.Builder
	if plural {
		sb.WriteString(toolRuntimeText(i18n.KeyToolRuntimeCronMissedPlural))
	} else {
		sb.WriteString(toolRuntimeText(i18n.KeyToolRuntimeCronMissedSingle))
	}
	for _, m := range missed {
		createdAt := m.CreatedAt
		if createdAt.IsZero() {
			createdAt = m.MissedSince
		}
		fmt.Fprintf(&sb, "\n\n%s\n%s",
			toolRuntimeFormat(i18n.KeyToolRuntimeCronMissedEntry,
				cronToHuman(m.Cron),
				createdAt.In(time.Local).Format(time.RFC3339)),
			fenceCronPrompt(m.Prompt),
		)
	}
	return sb.String()
}
