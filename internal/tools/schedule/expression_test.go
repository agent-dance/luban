package schedule

import (
	"testing"
	"time"
)

func TestParseExpressionStrictFiveFields(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"",
		"* * * *",
		"* * * * * *",
		"@daily",
		"0 0 * JAN *",
		"0 0 * * MON",
		"0 0 * * 7",
		"60 * * * *",
		"* 24 * * *",
		"* * 0 * *",
		"* * * 13 *",
		"*/0 * * * *",
		"*/x * * * *",
		"1//2 * * * *",
		"1, * * * *",
		"1,,2 * * * *",
		"5-3 * * * *",
		"1-2-3 * * * *",
		"+1 * * * *",
		"? * * * *",
	}
	for _, raw := range invalid {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := parseExpression(raw); err == nil {
				t.Fatalf("parseExpression(%q) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestParseExpressionRangesListsAndSteps(t *testing.T) {
	t.Parallel()

	expr, err := parseExpression("0/15 8-10/2 1,15 * 1-5")
	if err != nil {
		t.Fatal(err)
	}
	for _, minute := range []int{0, 15, 30, 45} {
		if !expr.minute.has(minute) {
			t.Errorf("minute %d was not selected", minute)
		}
	}
	if expr.minute.has(1) || !expr.hour.has(8) || expr.hour.has(9) || !expr.hour.has(10) {
		t.Fatalf("unexpected parsed fields: minute=%v hour=%v", expr.minute.ordered, expr.hour.ordered)
	}
}

func TestExpressionDayOfMonthAndWeekUseStandardOrRule(t *testing.T) {
	t.Parallel()

	expr, err := parseExpression("0 9 13 * 1")
	if err != nil {
		t.Fatal(err)
	}

	// Wednesday the 13th matches by day of month.
	if got := time.Date(2026, time.May, 13, 9, 0, 0, 0, time.UTC); got.Weekday() == time.Monday || !expr.matchesDate(got.Year(), got.Month(), got.Day(), time.UTC) {
		t.Fatalf("restricted day-of-month did not select %v", got)
	}
	// A Monday other than the 13th matches by day of week.
	if got := time.Date(2026, time.May, 11, 9, 0, 0, 0, time.UTC); got.Day() == 13 || !expr.matchesDate(got.Year(), got.Month(), got.Day(), time.UTC) {
		t.Fatalf("restricted day-of-week did not select %v", got)
	}
	if got := time.Date(2026, time.May, 12, 9, 0, 0, 0, time.UTC); expr.matchesDate(got.Year(), got.Month(), got.Day(), time.UTC) {
		t.Fatalf("unselected day unexpectedly matched %v", got)
	}

	weekdayOnly, err := parseExpression("0 9 * * 1")
	if err != nil {
		t.Fatal(err)
	}
	if !weekdayOnly.matchesDate(2026, time.May, 11, time.UTC) ||
		weekdayOnly.matchesDate(2026, time.May, 12, time.UTC) {
		t.Fatal("an unrestricted day-of-month did not preserve the weekday restriction")
	}

	monthDayOnly, err := parseExpression("0 9 13 * *")
	if err != nil {
		t.Fatal(err)
	}
	if !monthDayOnly.matchesDate(2026, time.May, 13, time.UTC) ||
		monthDayOnly.matchesDate(2026, time.May, 11, time.UTC) {
		t.Fatal("an unrestricted day-of-week did not preserve the day-of-month restriction")
	}
}

func TestNextRunCoversGregorianLeapCycle(t *testing.T) {
	t.Parallel()

	expr, err := parseExpression("0 0 29 2 *")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2096, time.March, 1, 0, 0, 0, 0, time.UTC)
	got, _, ok := nextRun(expr, from, time.UTC, "")
	if !ok {
		t.Fatal("nextRun did not find a leap day beyond the old 366-day horizon")
	}
	want := time.Date(2104, time.February, 29, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("nextRun=%v, want %v", got, want)
	}

	impossible, err := parseExpression("0 0 30 2 *")
	if err != nil {
		t.Fatal(err)
	}
	if got, key, ok := nextRun(impossible, from, time.UTC, ""); ok {
		t.Fatalf("impossible schedule returned %v (%q)", got, key)
	}
}

func TestNextRunSkipsDSTSpringGap(t *testing.T) {
	t.Parallel()

	loc := loadLocation(t, "America/New_York")
	expr, err := parseExpression("30 2 * * *")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, time.March, 7, 3, 0, 0, 0, loc)
	got, _, ok := nextRun(expr, from, loc, "")
	if !ok {
		t.Fatal("nextRun did not find the first 02:30 after the DST gap")
	}
	want := time.Date(2026, time.March, 9, 2, 30, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("nextRun=%v, want %v", got, want)
	}
}

func TestNextRunEmitsDSTFoldWallMinuteOnce(t *testing.T) {
	t.Parallel()

	loc := loadLocation(t, "America/New_York")
	expr, err := parseExpression("30 1 * * *")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, time.November, 1, 0, 0, 0, 0, loc)
	first, firstKey, ok := nextRun(expr, from, loc, "")
	if !ok {
		t.Fatal("nextRun did not find the fall-fold minute")
	}
	firstWall := first.In(loc)
	if firstWall.Year() != 2026 || firstWall.Month() != time.November || firstWall.Day() != 1 ||
		firstWall.Hour() != 1 || firstWall.Minute() != 30 {
		t.Fatalf("first wall time=%v, want 2026-11-01 01:30", firstWall)
	}

	// Simulate a persisted fire boundary after a small positive jitter. The
	// repeated 01:30 has the same wall key and must not be returned again.
	second, secondKey, ok := nextRun(expr, first.Add(time.Minute), loc, firstKey)
	if !ok {
		t.Fatal("nextRun did not find the following day's run")
	}
	secondWall := second.In(loc)
	if secondWall.Year() != 2026 || secondWall.Month() != time.November || secondWall.Day() != 2 ||
		secondWall.Hour() != 1 || secondWall.Minute() != 30 {
		t.Fatalf("second wall time=%v, want 2026-11-02 01:30", secondWall)
	}
	if secondKey == firstKey {
		t.Fatalf("wall keys did not advance: %q", firstKey)
	}
}

func loadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("time zone database unavailable: %v", err)
	}
	return loc
}
