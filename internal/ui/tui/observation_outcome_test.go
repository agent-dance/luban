package tui

import "testing"

func TestObservationOutcomeString(t *testing.T) {
	tests := []struct {
		outcome ObservationOutcome
		value   string
	}{
		{OutcomeUnknown, "unknown"},
		{OutcomeRunning, "running"},
		{OutcomeSucceeded, "succeeded"},
		{OutcomeFailed, "failed"},
		{OutcomePartial, "partial"},
		{OutcomeDenied, "denied"},
		{OutcomeCancelled, "cancelled"},
		{OutcomeTimedOut, "timed_out"},
		{OutcomeEscaped, "escaped"},
		{OutcomeShutdown, "shutdown"},
		{OutcomeOrphan, "orphan"},
		{OutcomeConflict, "conflict"},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			if got := test.outcome.String(); got != test.value {
				t.Fatalf("String() = %q, want %q", got, test.value)
			}
		})
	}
}
