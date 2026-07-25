package schedule

import (
	"testing"
	"time"
)

func TestRecurringJitterIsDeterministicAndBounded(t *testing.T) {
	t.Parallel()

	period := time.Hour
	first := recurringJitter(period, "80000000")
	second := recurringJitter(period, "80000000")
	if first != second {
		t.Fatalf("recurring jitter changed: %v != %v", first, second)
	}
	bound := time.Duration(float64(period) * recurringJitterPct)
	if first < 0 || first >= bound {
		t.Fatalf("recurring jitter=%v, want [0,%v)", first, bound)
	}
	if got := recurringJitter(24*time.Hour, "ffffffff"); got < 0 || got >= maxRecurringJitter {
		t.Fatalf("capped recurring jitter=%v, want [0,%v)", got, maxRecurringJitter)
	}
	if got := recurringJitter(0, "80000000"); got != 0 {
		t.Fatalf("zero-period jitter=%v", got)
	}
	if got := recurringJitter(time.Hour, "not-hex!"); got != 0 {
		t.Fatalf("invalid-ID jitter=%v", got)
	}
}

func TestOneshotJitterOnlyMovesRoundMinutesEarlier(t *testing.T) {
	t.Parallel()

	for _, minute := range []int{0, 30} {
		target := time.Date(2026, time.July, 25, 9, minute, 0, 0, time.UTC)
		got := oneshotJitter(target, "80000000")
		if got >= 0 || got <= -maxOneshotEarly {
			t.Fatalf("minute %d jitter=%v, want (-%v,0)", minute, got, maxOneshotEarly)
		}
	}
	if got := oneshotJitter(time.Date(2026, time.July, 25, 9, 15, 0, 0, time.UTC), "80000000"); got != 0 {
		t.Fatalf("non-boundary jitter=%v", got)
	}
}
