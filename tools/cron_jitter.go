package tools

import (
	"strconv"
	"time"
)

// cron_jitter.go — deterministic jitter for cron fire times.
//
// Mirrors the TS scheduler's load-spreading behaviour:
//   * Recurring jobs may fire up to min(period*0.1, 15min) LATE.
//   * One-shot jobs whose target lands on minute :00 or :30 may fire
//     up to 90 seconds EARLY (so users get fast results on round
//     boundaries while spreading load).
//
// Determinism: jitter for the same job ID is stable across process
// restarts so a job persisted to disk fires at a predictable offset.
// Cron IDs are eight hex characters; interpreting that value as a fraction
// exactly mirrors the TypeScript scheduler.

const (
	maxRecurringJitter = 15 * time.Minute
	maxOneshotEarly    = 90 * time.Second
	recurringJitterPct = 0.10 // 10 % of period
)

func cronJitterFraction(jobID string) float64 {
	if len(jobID) < 8 {
		return 0
	}
	value, err := strconv.ParseUint(jobID[:8], 16, 32)
	if err != nil {
		return 0
	}
	return float64(value) / float64(uint64(1)<<32)
}

// RecurringJitter returns the (positive) delay to add before firing a
// recurring job whose period is `period`. Bound by min(period*0.1, 15m).
// For periods <= 0 the function returns 0.
func RecurringJitter(period time.Duration, jobID string) time.Duration {
	if period <= 0 {
		return 0
	}
	bound := time.Duration(float64(period) * recurringJitterPct)
	if bound > maxRecurringJitter {
		bound = maxRecurringJitter
	}
	if bound <= 0 {
		return 0
	}
	return time.Duration(cronJitterFraction(jobID) * float64(bound)).Truncate(time.Millisecond)
}

// OneshotJitter returns a (signed) offset to apply to a one-shot job
// targeted at `target`. Negative values mean "fire that many ticks
// EARLY". The offset is non-zero only when target lands on minute :00
// or :30 — those round boundaries get up to 90 s of early firing.
// Otherwise returns 0.
func OneshotJitter(target time.Time, jobID string) time.Duration {
	min := target.Minute()
	if min != 0 && min != 30 {
		return 0
	}
	// Up to maxOneshotEarly seconds early.
	return -time.Duration(cronJitterFraction(jobID) * float64(maxOneshotEarly)).Truncate(time.Millisecond)
}
