package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// A Gregorian calendar repeats its dates and weekdays every 400 years.
// Including the equivalent date at the far boundary lets a search that starts
// partway through a day still inspect every possible wall-clock slot.
const gregorianCycleDays = 146097

type expression struct {
	minute     field
	hour       field
	dayOfMonth field
	month      field
	dayOfWeek  field
}

type field struct {
	values       []bool
	ordered      []int
	unrestricted bool
}

func (f field) has(value int) bool {
	return value >= 0 && value < len(f.values) && f.values[value]
}

// parseExpression parses the traditional five-field cron format:
// minute, hour, day of month, month, and day of week. It deliberately rejects
// seconds, names, aliases, question marks, and Sunday=7 compatibility syntax.
func parseExpression(raw string) (*expression, error) {
	parts := strings.Fields(raw)
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron field count: got %d, want 5", len(parts))
	}

	minute, err := parseField(parts[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	hour, err := parseField(parts[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	dayOfMonth, err := parseField(parts[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("cron day of month: %w", err)
	}
	month, err := parseField(parts[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	dayOfWeek, err := parseField(parts[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("cron day of week: %w", err)
	}

	return &expression{
		minute:     minute,
		hour:       hour,
		dayOfMonth: dayOfMonth,
		month:      month,
		dayOfWeek:  dayOfWeek,
	}, nil
}

func parseField(raw string, minimum, maximum int) (field, error) {
	if raw == "" {
		return field{}, fmt.Errorf("empty cron field")
	}

	values := make([]bool, maximum+1)
	for _, item := range strings.Split(raw, ",") {
		if item == "" {
			return field{}, fmt.Errorf("empty cron list item")
		}
		if err := addFieldItem(values, item, minimum, maximum); err != nil {
			return field{}, err
		}
	}

	ordered := make([]int, 0, maximum-minimum+1)
	for value := minimum; value <= maximum; value++ {
		if values[value] {
			ordered = append(ordered, value)
		}
	}
	if len(ordered) == 0 {
		return field{}, fmt.Errorf("cron field has no values")
	}

	return field{
		values:       values,
		ordered:      ordered,
		unrestricted: len(ordered) == maximum-minimum+1,
	}, nil
}

func addFieldItem(values []bool, item string, minimum, maximum int) error {
	if strings.Count(item, "/") > 1 {
		return fmt.Errorf("invalid cron step %q", item)
	}

	base := item
	step := 1
	hasStep := false
	if before, after, ok := strings.Cut(item, "/"); ok {
		base = before
		hasStep = true
		if base == "" || after == "" {
			return fmt.Errorf("invalid cron step %q", item)
		}
		parsed, err := parseDecimal(after)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("invalid cron step %q", after)
		}
		step = parsed
	}

	var first, last int
	switch {
	case base == "*":
		first, last = minimum, maximum
	case strings.Count(base, "-") == 1:
		left, right, _ := strings.Cut(base, "-")
		var err error
		first, err = parseDecimal(left)
		if err != nil {
			return fmt.Errorf("invalid cron range %q", base)
		}
		last, err = parseDecimal(right)
		if err != nil {
			return fmt.Errorf("invalid cron range %q", base)
		}
		if first > last {
			return fmt.Errorf("descending cron range %q", base)
		}
	case strings.Contains(base, "-"):
		return fmt.Errorf("invalid cron range %q", base)
	default:
		parsed, err := parseDecimal(base)
		if err != nil {
			return fmt.Errorf("invalid cron value %q", base)
		}
		first = parsed
		last = parsed
		// N/S means every S values beginning at N, through the end of the
		// field. This is the conventional five-field cron interpretation.
		if hasStep {
			last = maximum
		}
	}

	if first < minimum || first > maximum || last < minimum || last > maximum {
		return fmt.Errorf("cron range %d-%d outside %d-%d", first, last, minimum, maximum)
	}
	for value := first; ; value += step {
		values[value] = true
		// Besides avoiding an unnecessary final addition, this guards the
		// parser against integer overflow from a syntactically valid but very
		// large step.
		if step > last-value {
			break
		}
	}
	return nil
}

func parseDecimal(raw string) (int, error) {
	if raw == "" {
		return 0, strconv.ErrSyntax
	}
	for _, char := range raw {
		if char < '0' || char > '9' {
			return 0, strconv.ErrSyntax
		}
	}
	return strconv.Atoi(raw)
}

// nextRun finds the first existing local wall-clock minute strictly after
// from. The returned wall key must be passed back as afterWallKey after a fire;
// doing so prevents an ambiguous minute during a DST fall-back fold from being
// delivered twice even when from includes a post-fire jitter offset.
func nextRun(e *expression, from time.Time, loc *time.Location, afterWallKey string) (time.Time, string, bool) {
	if e == nil {
		return time.Time{}, "", false
	}
	if loc == nil {
		loc = time.Local
	}

	localFrom := from.In(loc)
	day := time.Date(localFrom.Year(), localFrom.Month(), localFrom.Day(), 12, 0, 0, 0, loc)
	for days := 0; days <= gregorianCycleDays; days++ {
		year, month, date := day.Date()
		if e.month.has(int(month)) && e.matchesDate(year, month, date, loc) {
			for _, hour := range e.hour.ordered {
				for _, minute := range e.minute.ordered {
					candidate := time.Date(year, month, date, hour, minute, 0, 0, loc)
					wall := candidate.In(loc)
					// time.Date normalizes a nonexistent spring-forward time. A
					// round-trip mismatch therefore means this wall slot never
					// occurred and must not be scheduled.
					if wall.Year() != year || wall.Month() != month || wall.Day() != date ||
						wall.Hour() != hour || wall.Minute() != minute {
						continue
					}
					if !candidate.After(from) {
						continue
					}
					key := wallMinuteKey(wall, loc)
					if key == afterWallKey {
						continue
					}
					return candidate, key, true
				}
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return time.Time{}, "", false
}

func (e *expression) matchesDate(year int, month time.Month, day int, loc *time.Location) bool {
	// Noon is intentionally used for weekday calculation: it remains a stable
	// representative of the civil date across ordinary DST transitions.
	date := time.Date(year, month, day, 12, 0, 0, 0, loc)
	dayOfMonthMatches := e.dayOfMonth.has(day)
	dayOfWeekMatches := e.dayOfWeek.has(int(date.Weekday()))
	if !e.dayOfMonth.unrestricted && !e.dayOfWeek.unrestricted {
		return dayOfMonthMatches || dayOfWeekMatches
	}
	return dayOfMonthMatches && dayOfWeekMatches
}

func wallMinuteKey(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	wall := t.In(loc)
	return fmt.Sprintf("%s|%04d-%02d-%02dT%02d:%02d", loc.String(), wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute())
}
