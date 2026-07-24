package tools

// cron_errors.go — typed sentinel errors for the cron parser / scheduler.
//
// Tools that wrap cron want to match on stable error values rather than
// scrape error strings. We expose a small set of sentinels here and have
// the parser/scheduler wrap them so callers can use errors.Is.

import "errors"

// ErrCronInvalidFieldCount is returned by ParseCron when the expression
// does not contain exactly 5 fields. The wrapping error message still
// names the offending field count for human consumption.
var ErrCronInvalidFieldCount = errors.New("cron: 5 fields required")

// ErrCronInvalidField is returned for any per-field parse failure
// (range / step / list malformed).
var ErrCronInvalidField = errors.New("cron: invalid field")

// ErrCronUnknownSentinel is returned by ResolvePrompt when a `<<...>>`
// sentinel is not recognised in the cron context.
var ErrCronUnknownSentinel = errors.New("cron: unknown prompt sentinel")

// ErrCronFeatureDisabled is returned when the cron feature flag is
// disabled at runtime (CLAUDE_CODE_DISABLE_CRON=1).
var ErrCronFeatureDisabled = errors.New("cron: feature disabled")

// CronSentinelErrors returns the registered sentinel errors. Tests probe
// this to verify the typed-error contract.
func CronSentinelErrors() []error {
	return []error{
		ErrCronInvalidFieldCount,
		ErrCronInvalidField,
		ErrCronUnknownSentinel,
		ErrCronFeatureDisabled,
	}
}
