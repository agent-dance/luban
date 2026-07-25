package schedule

import (
	"strconv"
	"time"
)

const (
	maxRecurringJitter = 15 * time.Minute
	maxOneshotEarly    = 90 * time.Second
	recurringJitterPct = 0.10
)

func jitterFraction(jobID string) float64 {
	if len(jobID) < 8 {
		return 0
	}
	value, err := strconv.ParseUint(jobID[:8], 16, 32)
	if err != nil {
		return 0
	}
	return float64(value) / float64(uint64(1)<<32)
}

func recurringJitter(period time.Duration, jobID string) time.Duration {
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
	return time.Duration(jitterFraction(jobID) * float64(bound)).Truncate(time.Millisecond)
}

func oneshotJitter(target time.Time, jobID string) time.Duration {
	if target.Minute() != 0 && target.Minute() != 30 {
		return 0
	}
	return -time.Duration(jitterFraction(jobID) * float64(maxOneshotEarly)).Truncate(time.Millisecond)
}
