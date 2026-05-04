package cronjob

import (
	"strings"
	"testing"
	"time"
)

func TestParseScheduleCron(t *testing.T) {
	s, kind, err := ParseSchedule("0 9 * * *", time.UTC, time.Minute, mustTime(t, "2026-05-04T00:00:00Z"))
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	if kind != KindCron {
		t.Errorf("Kind=%q want %q", kind, KindCron)
	}
	if s.Kind() != KindCron {
		t.Errorf("schedule.Kind=%q", s.Kind())
	}
}

func TestParseScheduleInterval(t *testing.T) {
	s, kind, err := ParseSchedule("every 5m", time.UTC, time.Minute, mustTime(t, "2026-05-04T00:00:00Z"))
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	if kind != KindInterval {
		t.Errorf("Kind=%q want %q", kind, KindInterval)
	}
	if s.Kind() != KindInterval {
		t.Errorf("schedule.Kind=%q", s.Kind())
	}
}

func TestParseScheduleIntervalBelowMinimum(t *testing.T) {
	_, _, err := ParseSchedule("every 30s", time.UTC, time.Minute, time.Now())
	if err == nil || !strings.Contains(err.Error(), "minimum") {
		t.Errorf("expected 'minimum' error, got %v", err)
	}
}

func TestParseScheduleOneshotIn(t *testing.T) {
	now := mustTime(t, "2026-05-04T10:00:00Z")
	s, kind, err := ParseSchedule("in 30m", time.UTC, time.Minute, now)
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	if kind != KindOneshot {
		t.Errorf("Kind=%q want %q", kind, KindOneshot)
	}
	want := now.Add(30 * time.Minute)
	if got := s.Next(now); !got.Equal(want) {
		t.Errorf("Next=%v want %v", got, want)
	}
}

func TestParseScheduleOneshotAtClock(t *testing.T) {
	// 2026-05-04 14:00 UTC, "at 09:00" should be tomorrow 09:00
	now := mustTime(t, "2026-05-04T14:00:00Z")
	s, kind, err := ParseSchedule("at 09:00", time.UTC, time.Minute, now)
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	if kind != KindOneshot {
		t.Errorf("Kind=%q", kind)
	}
	want := mustTime(t, "2026-05-05T09:00:00Z")
	if got := s.Next(now); !got.Equal(want) {
		t.Errorf("Next=%v want %v", got, want)
	}
}

func TestParseScheduleOneshotAtPast(t *testing.T) {
	now := mustTime(t, "2026-05-04T14:00:00Z")
	_, _, err := ParseSchedule("at 2020-01-01 00:00", time.UTC, time.Minute, now)
	if err == nil || !strings.Contains(err.Error(), "past") {
		t.Errorf("expected 'past' error, got %v", err)
	}
}

func TestParseScheduleInvalid(t *testing.T) {
	_, _, err := ParseSchedule("garbage", time.UTC, time.Minute, time.Now())
	if err == nil {
		t.Error("expected error for garbage input")
	}
}
