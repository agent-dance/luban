package tools

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// cron_sentinel.go — resolves prompt sentinels at cron fire time.
//
// Sentinels let users pin a stable canonical instruction for a job
// without baking the (possibly long) text into the cron entry. At fire
// time we expand the sentinel just before invoking the agent.
//
// Currently supported:
//   <<autonomous-loop>>          → autonomous loop instructions
//
// Explicitly REJECTED here:
//   <<autonomous-loop-dynamic>>  → ScheduleWakeup territory, not Cron.
//
// Mirrors TS sentinelResolver.ts.

const (
	sentinelAutonomousLoop        = "<<autonomous-loop>>"
	sentinelAutonomousLoopDynamic = "<<autonomous-loop-dynamic>>"
)

// autonomousLoopPrompt is the canonical expansion of <<autonomous-loop>>.
// Kept short and self-contained so each fire renders identically.
const autonomousLoopPrompt = `You are running as part of an autonomous scheduled loop.
Continue the previously-defined task. Read the most recent state, decide
what to do next, and act. If nothing remains to do, say so explicitly
and exit without further action.`

// ResolvePrompt expands recognised sentinels in `prompt`. Plain prompts
// pass through untouched. Returns an error if the prompt contains a
// sentinel that is not valid in the cron context.
func ResolvePrompt(prompt string) (string, error) {
	trimmed := strings.TrimSpace(prompt)

	switch trimmed {
	case sentinelAutonomousLoop:
		return autonomousLoopPrompt, nil
	case sentinelAutonomousLoopDynamic:
		return "", i18n.NewError(
			i18n.KeyToolRuntimeCronSentinelReserved,
			"sentinel", sentinelAutonomousLoopDynamic, "ScheduleWakeup", sentinelAutonomousLoop, "Cron",
		)
	}

	// Reject any other unknown <<…>> sentinel with a clear message.
	if strings.HasPrefix(trimmed, "<<") && strings.HasSuffix(trimmed, ">>") {
		return "", i18n.NewError(i18n.KeyToolRuntimeCronPromptSentinelUnknown, "prompt sentinel", trimmed)
	}

	// Non-sentinel: pass through.
	return prompt, nil
}
