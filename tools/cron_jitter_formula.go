package tools

// cron_jitter_formula.go — public jitter formula exposed for durable
// schedulers that need to pre-compute next-fire windows without
// importing the full executor.
//
// The formula here matches RecurringJitter / OneshotJitter byte-for-byte
// so callers can reproduce the exact offset a fire will use.

import "time"

// CronJitterFormula is the canonical public jitter formula. Given a job
// ID and a period, it returns the (positive) recurring jitter delay used
// by the cron executor. Mirrors RecurringJitter.
//
// Exposed publicly per audit P3-3 so durable schedulers (which run from
// a separate process) can pre-compute next-fire windows without linking
// against the runtime executor.
func CronJitterFormula(period time.Duration, jobID string) time.Duration {
	return RecurringJitter(period, jobID)
}

// CronJitterFormulaPublic always returns true to advertise that the
// formula is part of the package's public surface.
func CronJitterFormulaPublic() bool { return true }
