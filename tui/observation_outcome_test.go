package tui

import "testing"

func TestObservationOutcomeSessionCodec(t *testing.T) {
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
		{OutcomeOrphan, "orphan"},
		{OutcomeConflict, "conflict"},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			if got := test.outcome.String(); got != test.value {
				t.Fatalf("String() = %q, want %q", got, test.value)
			}
			got, ok := ParseObservationOutcome(test.value)
			if !ok || got != test.outcome {
				t.Fatalf("ParseObservationOutcome(%q) = %v, %v; want %v, true", test.value, got, ok, test.outcome)
			}
		})
	}

	if got, ok := ParseObservationOutcome("future-value"); ok || got != OutcomeUnknown {
		t.Fatalf("unknown value = %v, %v; want unknown, false", got, ok)
	}
}

func TestActivityStateForOutcomePreservesAttentionCategories(t *testing.T) {
	tests := map[ObservationOutcome]ActivityState{
		OutcomeSucceeded: ActivityCompleted,
		OutcomeFailed:    ActivityFailed,
		OutcomePartial:   ActivityFailed,
		OutcomeDenied:    ActivityFailed,
		OutcomeCancelled: ActivityCancelled,
		OutcomeTimedOut:  ActivityCancelled,
		OutcomeRunning:   ActivityRunning,
	}
	for outcome, want := range tests {
		if got := activityStateForOutcome(outcome); got != want {
			t.Errorf("activityStateForOutcome(%s) = %s, want %s", outcome, got, want)
		}
	}
}
