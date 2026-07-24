package loop

import (
	"fmt"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const (
	tokenBudgetCompletionThreshold  = 0.9
	tokenBudgetDiminishingThreshold = 500
)

type BudgetTracker struct {
	ContinuationCount    int
	LastDeltaTokens      int
	LastGlobalTurnTokens int
	StartedAt            time.Time
}

func NewBudgetTracker() *BudgetTracker {
	return &BudgetTracker{StartedAt: time.Now()}
}

type TokenBudgetDecision struct {
	Continue          bool
	NudgeMessage      string
	ContinuationCount int
	Percent           int
	TurnTokens        int
	Budget            int
	CompletionEvent   *TokenBudgetCompletionEvent
}

type TokenBudgetCompletionEvent struct {
	ContinuationCount  int
	Percent            int
	TurnTokens         int
	Budget             int
	DiminishingReturns bool
	Duration           time.Duration
}

func CheckTokenBudget(tracker *BudgetTracker, agentID string, budget int, globalTurnTokens int) TokenBudgetDecision {
	if tracker == nil || agentID != "" || budget <= 0 {
		return TokenBudgetDecision{}
	}

	percent := 0
	if budget > 0 {
		percent = int(float64(globalTurnTokens)/float64(budget)*100 + 0.5)
	}
	deltaSinceLastCheck := globalTurnTokens - tracker.LastGlobalTurnTokens
	isDiminishing := tracker.ContinuationCount >= 3 &&
		deltaSinceLastCheck < tokenBudgetDiminishingThreshold &&
		tracker.LastDeltaTokens < tokenBudgetDiminishingThreshold

	if !isDiminishing && float64(globalTurnTokens) < float64(budget)*tokenBudgetCompletionThreshold {
		tracker.ContinuationCount++
		tracker.LastDeltaTokens = deltaSinceLastCheck
		tracker.LastGlobalTurnTokens = globalTurnTokens
		return TokenBudgetDecision{
			Continue:          true,
			NudgeMessage:      budgetContinuationMessage(percent, globalTurnTokens, budget),
			ContinuationCount: tracker.ContinuationCount,
			Percent:           percent,
			TurnTokens:        globalTurnTokens,
			Budget:            budget,
		}
	}

	if isDiminishing || tracker.ContinuationCount > 0 {
		return TokenBudgetDecision{
			CompletionEvent: &TokenBudgetCompletionEvent{
				ContinuationCount:  tracker.ContinuationCount,
				Percent:            percent,
				TurnTokens:         globalTurnTokens,
				Budget:             budget,
				DiminishingReturns: isDiminishing,
				Duration:           time.Since(tracker.StartedAt),
			},
		}
	}

	return TokenBudgetDecision{}
}

func budgetContinuationMessage(percent int, turnTokens int, budget int) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLoopVisibleTokenBudgetContinuation,
		percent,
		formatTokenCount(turnTokens),
		formatTokenCount(budget),
	)
}

func formatTokenCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
