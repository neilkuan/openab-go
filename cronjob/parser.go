package cronjob

import (
	"fmt"
	"strings"
	"time"
)

// ParseSchedule converts a user-supplied schedule string into a
// Schedule + Kind. Accepted forms (case-insensitive on the keyword):
//
//   - "0 9 * * *"            standard 5-field cron
//   - "every <duration>"     interval; rejected if < minInterval
//   - "in <duration>"        oneshot from now
//   - "at HH:MM"             oneshot today (or tomorrow if already passed)
//   - "at YYYY-MM-DD HH:MM"  oneshot at absolute time
//
// `tz` controls the interpretation of "at HH:MM" / "at YYYY-MM-DD HH:MM".
// Internally all schedules return UTC times.
//
// `now` is passed in for testability — production callers pass time.Now().UTC().
func ParseSchedule(input string, tz *time.Location, minInterval time.Duration, now time.Time) (Schedule, Kind, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return nil, "", fmt.Errorf("schedule cannot be empty")
	}

	lower := strings.ToLower(in)

	switch {
	case strings.HasPrefix(lower, "every "):
		dur, err := time.ParseDuration(strings.TrimSpace(in[len("every "):]))
		if err != nil {
			return nil, "", fmt.Errorf("invalid interval duration: %w", err)
		}
		if dur < minInterval {
			return nil, "", fmt.Errorf("interval %v is below minimum %v", dur, minInterval)
		}
		s, err := newIntervalSchedule(dur)
		if err != nil {
			return nil, "", err
		}
		return s, KindInterval, nil

	case strings.HasPrefix(lower, "in "):
		dur, err := time.ParseDuration(strings.TrimSpace(in[len("in "):]))
		if err != nil {
			return nil, "", fmt.Errorf("invalid duration: %w", err)
		}
		if dur <= 0 {
			return nil, "", fmt.Errorf("'in' duration must be positive, got %v", dur)
		}
		return newOneshotSchedule(now.Add(dur)), KindOneshot, nil

	case strings.HasPrefix(lower, "at "):
		rest := strings.TrimSpace(in[len("at "):])
		// Try absolute YYYY-MM-DD HH:MM first
		if t, err := time.ParseInLocation("2006-01-02 15:04", rest, tz); err == nil {
			at := t.UTC()
			if !at.After(now.UTC()) {
				return nil, "", fmt.Errorf("'at' time %v is in the past", at)
			}
			return newOneshotSchedule(at), KindOneshot, nil
		}
		// Fallback: HH:MM (today, or tomorrow if already passed)
		if t, err := time.ParseInLocation("15:04", rest, tz); err == nil {
			nowInTZ := now.In(tz)
			candidate := time.Date(nowInTZ.Year(), nowInTZ.Month(), nowInTZ.Day(),
				t.Hour(), t.Minute(), 0, 0, tz)
			if !candidate.After(now) {
				candidate = candidate.Add(24 * time.Hour)
			}
			return newOneshotSchedule(candidate.UTC()), KindOneshot, nil
		}
		return nil, "", fmt.Errorf("invalid 'at' time %q (use 'HH:MM' or 'YYYY-MM-DD HH:MM')", rest)
	}

	// Try cron expression as the last resort.
	if c, err := newCronSchedule(in); err == nil {
		return c, KindCron, nil
	}

	return nil, "", fmt.Errorf("unrecognised schedule %q (try '0 9 * * *', 'every 5m', 'in 2h', 'at 09:00')", input)
}
