package rrule

import (
	"testing"
	"time"
)

func TestTimedOverlapHalfOpen(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	a := NewTimedEvent("room-1",
		at(2024, time.March, 4, 9, 0, ny),
		at(2024, time.March, 4, 10, 0, ny))
	// Back-to-back: end of a equals start of b -> no overlap.
	b := NewTimedEvent("room-1",
		at(2024, time.March, 4, 10, 0, ny),
		at(2024, time.March, 4, 11, 0, ny))
	if Overlaps(a, b, ny) {
		t.Fatal("touching endpoints must not overlap")
	}
	// One minute of real overlap.
	c := NewTimedEvent("room-1",
		at(2024, time.March, 4, 9, 30, ny),
		at(2024, time.March, 4, 10, 30, ny))
	if !Overlaps(a, c, ny) {
		t.Fatal("overlapping spans must collide")
	}
	// Different resource never conflicts.
	d := NewTimedEvent("room-2",
		at(2024, time.March, 4, 9, 30, ny),
		at(2024, time.March, 4, 10, 30, ny))
	if Overlaps(a, d, ny) {
		t.Fatal("different resources must not conflict")
	}
}

func TestOverlapCrossZoneAndUTC(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	lon := mustLoc(t, "Europe/London")
	// 09:00-10:00 New York (EST) = 14:00-15:00 UTC = 14:00-15:00 London.
	a := NewTimedEvent("r",
		at(2024, time.January, 4, 9, 0, ny),
		at(2024, time.January, 4, 10, 0, ny))
	b := NewTimedEvent("r",
		at(2024, time.January, 4, 14, 30, lon),
		at(2024, time.January, 4, 15, 30, lon))
	if !Overlaps(a, b, ny) {
		t.Fatal("cross-zone overlap missed")
	}
	utc := NewTimedEvent("r",
		at(2024, time.January, 4, 15, 0, time.UTC),
		at(2024, time.January, 4, 16, 0, time.UTC))
	if Overlaps(a, utc, ny) {
		t.Fatal("UTC event starting exactly at end must not overlap")
	}
}

func TestAllDayVersusTimedAndMultiDay(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	day := NewAllDayEvent("r", at(2024, time.March, 4, 0, 0, ny))
	morning := NewTimedEvent("r",
		at(2024, time.March, 4, 8, 0, ny),
		at(2024, time.March, 4, 9, 0, ny))
	if !Overlaps(day, morning, ny) {
		t.Fatal("all-day should overlap morning meeting")
	}
	// Meeting at exactly midnight next day: all-day ends then, no overlap.
	midnight := NewTimedEvent("r",
		at(2024, time.March, 5, 0, 0, ny),
		at(2024, time.March, 5, 1, 0, ny))
	if Overlaps(day, midnight, ny) {
		t.Fatal("meeting at end midnight must not overlap")
	}

	multi := NewAllDayEvent("r",
		at(2024, time.March, 4, 0, 0, ny),
		at(2024, time.March, 5, 0, 0, ny))
	if !Overlaps(multi, midnight, ny) {
		t.Fatal("multi-day all-day should cover next midnight")
	}
}

func TestAllDayCrossZoneComparison(t *testing.T) {
	// All-day event in New York is [05:00,next 05:00) UTC on Jan 4.
	ny := mustLoc(t, "America/New_York")
	day := NewAllDayEvent("r", at(2024, time.January, 4, 0, 0, ny))
	// 04:00 UTC = 23:00 Jan 3 New York -> before the day.
	before := NewTimedEvent("r",
		at(2024, time.January, 4, 4, 0, time.UTC),
		at(2024, time.January, 4, 4, 30, time.UTC))
	if Overlaps(day, before, ny) {
		t.Fatal("04:00 UTC is before NY all-day start")
	}
	inside := NewTimedEvent("r",
		at(2024, time.January, 4, 6, 0, time.UTC),
		at(2024, time.January, 4, 6, 30, time.UTC))
	if !Overlaps(day, inside, ny) {
		t.Fatal("06:00 UTC is inside the NY all-day span")
	}
}

func TestFloatingAnchoredForConflict(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	// Floating 09:00 anchored in New York.
	floating := time.Date(2024, time.January, 4, 9, 0, 0, 0, Floating)
	a := NewTimedEvent("r", floating,
		time.Date(2024, time.January, 4, 10, 0, 0, 0, Floating))
	// 09:30 New York collides with the anchored floating 09:00-10:00.
	b := NewTimedEvent("r",
		at(2024, time.January, 4, 9, 30, ny),
		at(2024, time.January, 4, 10, 30, ny))
	if !Overlaps(a, b, ny) {
		t.Fatal("floating anchored to NY should overlap")
	}
	// Same floating event anchored in London instead: 09:00 London = 04:00 NY.
	if Overlaps(a, b, mustLoc(t, "Europe/London")) {
		t.Fatal("floating anchored to London must not overlap a 09:30 NY event")
	}
}

func TestConflictsList(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	ev := NewTimedEvent("r",
		at(2024, time.March, 4, 9, 0, ny),
		at(2024, time.March, 4, 10, 0, ny))
	others := []Event{
		NewTimedEvent("r", at(2024, time.March, 4, 8, 0, ny), at(2024, time.March, 4, 8, 30, ny)),
		NewTimedEvent("r", at(2024, time.March, 4, 9, 30, ny), at(2024, time.March, 4, 10, 0, ny)),
		NewTimedEvent("other", at(2024, time.March, 4, 9, 30, ny), at(2024, time.March, 4, 10, 0, ny)),
	}
	got := Conflicts(ev, others, ny)
	if len(got) != 1 {
		t.Fatalf("want 1 conflict, got %d", len(got))
	}
}
