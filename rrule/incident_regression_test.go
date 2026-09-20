package rrule

import (
	"testing"
	"time"
)

func TestIncidentWeeklySpringForwardCountDoesNotConsumeGap(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 3, 2, 0),
		Rule:     mustParse(t, "FREQ=WEEKLY;BYDAY=SU;COUNT=2"),
		Duration: time.Hour,
	}
	from := zonedTime(ny, 2024, time.March, 1, 0, 0)
	to := zonedTime(ny, 2024, time.March, 24, 0, 0)

	got, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d occurrences, want Mar 3 and Mar 17; the Mar 10 gap must not consume COUNT: %+v", len(got), got)
	}
	if got[0].Wall.Day != 3 || got[1].Wall.Day != 17 {
		t.Fatalf("days = %d,%d, want 3,17", got[0].Wall.Day, got[1].Wall.Day)
	}
}

func TestIncidentWeeklyFallBackRemainsOneOccurrence(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.October, 27, 1, 30),
		Rule:     mustParse(t, "FREQ=WEEKLY;BYDAY=SU;COUNT=3"),
		Duration: time.Hour,
	}
	from := zonedTime(ny, 2024, time.October, 27, 0, 0)
	to := zonedTime(ny, 2024, time.November, 11, 0, 0)

	got, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d occurrences, want one for each Sunday: %+v", len(got), got)
	}
	wantDays := []int{27, 3, 10}
	wantMonths := []time.Month{time.October, time.November, time.November}
	for i, wantDay := range wantDays {
		if got[i].Wall.Month != wantMonths[i] || got[i].Wall.Day != wantDay {
			t.Fatalf("occ %d = %v %d, want %v %d", i, got[i].Wall.Month, got[i].Wall.Day, wantMonths[i], wantDay)
		}
	}
	if got[1].Instant.UTC().Format("15:04") != "05:30" {
		t.Fatalf("ambiguous occurrence = %v UTC, want first instant 05:30Z", got[1].Instant.UTC())
	}
}

func TestIncidentUntilUTCWithZonedStart(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart: zonedTime(ny, 2024, time.March, 1, 9, 0),
		Rule:    mustParse(t, "FREQ=DAILY;UNTIL=20240305T140000Z"),
	}
	from := zonedTime(ny, 2024, time.March, 1, 0, 0)
	to := zonedTime(ny, 2024, time.March, 10, 0, 0)

	got, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got[4].Wall.Day != 5 {
		t.Fatalf("got %d occurrences, want through Mar 5 09:00 EST exactly at 14:00Z UNTIL: %+v", len(got), got)
	}

	s.Rule = mustParse(t, "FREQ=DAILY;UNTIL=20240305T135959Z")
	got, err = Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[3].Wall.Day != 4 {
		t.Fatalf("got %d occurrences, want Mar 5 excluded one second before its UTC instant: %+v", len(got), got)
	}
}

func TestIncidentUntilFloatingWithZonedStart(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart: zonedTime(ny, 2024, time.March, 1, 9, 0),
		Rule:    mustParse(t, "FREQ=DAILY;UNTIL=20240305T090000"),
	}
	from := zonedTime(ny, 2024, time.March, 1, 0, 0)
	to := zonedTime(ny, 2024, time.March, 10, 0, 0)

	got, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got[4].Wall.Day != 5 {
		t.Fatalf("got %d occurrences, want through local Mar 5 09:00: %+v", len(got), got)
	}
}

func TestIncidentBySetPosLastFridayAcrossShortAndLongMonths(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.January, 5, 9, 0),
		Rule:    mustParse(t, "FREQ=MONTHLY;BYDAY=FR;BYSETPOS=-1;COUNT=3"),
	}
	from, to := windowFloat(2024, time.January, 1, 120)

	got, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		month time.Month
		day   int
	}{
		{time.January, 26},
		{time.February, 23},
		{time.March, 29},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d occurrences %+v, want 3 last Fridays", len(got), got)
	}
	for i, w := range want {
		if got[i].Wall.Month != w.month || got[i].Wall.Day != w.day {
			t.Fatalf("occ %d = %v %d, want %v %d", i, got[i].Wall.Month, got[i].Wall.Day, w.month, w.day)
		}
	}
}

func TestIncidentAllDayAndMidnightBoundariesAgree(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	allDay := &Schedule{
		DTStart: zonedTime(ny, 2024, time.March, 6, 0, 0),
		AllDay:  true,
	}
	from := zonedTime(ny, 2024, time.March, 6, 0, 0)
	to := zonedTime(ny, 2024, time.March, 7, 0, 0)

	startMidnight := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 6, 0, 0),
		Duration: time.Hour,
	}
	hit, err := Conflicts(allDay, startMidnight, from, to, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("a positive-duration timed event starting at day-start must overlap the all-day block")
	}

	endMidnight := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 7, 0, 0),
		Duration: time.Hour,
	}
	hit, err = Conflicts(allDay, endMidnight, from, to, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("a timed event starting when the half-open all-day block ends must not overlap")
	}

	busy, err := BusyIntervals([]*Schedule{allDay, startMidnight}, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 1 {
		t.Fatalf("busy path got %d blocks, want the same overlap result as Conflicts", len(busy))
	}

	zeroDurationMidnight := &Schedule{
		DTStart: zonedTime(ny, 2024, time.March, 6, 0, 0),
	}
	hit, err = Conflicts(allDay, zeroDurationMidnight, from, to, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("a zero-duration timed moment must remain non-overlapping under the existing semantics")
	}
	busy, err = BusyIntervals([]*Schedule{allDay, zeroDurationMidnight}, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 1 {
		t.Fatalf("busy path got %d blocks, want the zero-duration moment not to merge", len(busy))
	}
}

func TestIncidentClosedExpansionBoundaryVisibleToOverlap(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	from := zonedTime(ny, 2024, time.March, 6, 0, 0)
	to := zonedTime(ny, 2024, time.March, 7, 0, 0)
	a := &Schedule{
		DTStart:  to,
		Duration: time.Hour,
	}
	b := &Schedule{
		DTStart:  to,
		Duration: time.Hour,
	}

	occs, err := Expand(a, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) != 1 {
		t.Fatalf("Expand closed boundary got %d occurrences, want 1", len(occs))
	}
	hit, err := Conflicts(a, b, from, to, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("two events at the inclusive expansion end must conflict")
	}
	busy, err := BusyIntervals([]*Schedule{a, b}, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 1 {
		t.Fatalf("busy path got %d blocks at closed boundary, want 1: %+v", len(busy), busy)
	}
}
