package rrule

import (
	"testing"
	"time"
)

func TestHalfOpenBoundary(t *testing.T) {
	t0 := time.Date(2024, time.March, 1, 9, 0, 0, 0, time.UTC)
	a := Interval{Start: t0, End: t0.Add(time.Hour)}
	b := Interval{Start: t0.Add(time.Hour), End: t0.Add(2 * time.Hour)}
	if a.Overlaps(b) {
		t.Fatal("back-to-back bookings must not overlap (half-open [start,end))")
	}
	c := Interval{Start: t0.Add(59 * time.Minute), End: t0.Add(2 * time.Hour)}
	if !a.Overlaps(c) {
		t.Fatal("one minute of real overlap must conflict")
	}
}

func TestCrossZoneConflict(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	lon := loadZone(t, "Europe/London")
	// 10:00-11:00 London on Mar 6 == 05:00-06:00 New York.
	a := &Schedule{
		DTStart:  zonedTime(lon, 2024, time.March, 6, 10, 0),
		Duration: time.Hour,
	}
	b := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 6, 5, 30),
		Duration: time.Hour,
	}
	from := zonedTime(lon, 2024, time.March, 6, 0, 0)
	to := zonedTime(lon, 2024, time.March, 7, 0, 0)
	ok, err := Conflicts(a, b, from, to, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("10:00 London vs 05:30 New York overlap for 30 minutes")
	}

	// 04:00-05:00 NY (EST) = 09:00-10:00 London: ends exactly when the
	// London meeting starts.
	b2 := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 6, 4, 0),
		Duration: time.Hour,
	}
	ok, _ = Conflicts(a, b2, from, to, nil, nil)
	if ok {
		t.Fatal("meeting ending at 10:00 London must not conflict with one starting at 10:00")
	}
}

func TestAllDayVsTimed(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	allDay := &Schedule{
		DTStart: zonedTime(ny, 2024, time.March, 6, 0, 0),
		AllDay:  true,
	}
	meeting := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 6, 23, 30),
		Duration: time.Hour,
	}
	from := zonedTime(ny, 2024, time.March, 6, 0, 0)
	to := zonedTime(ny, 2024, time.March, 7, 0, 0)
	ok, err := Conflicts(allDay, meeting, from, to, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("23:30-00:30 timed event overlaps the all-day block")
	}

	// A meeting starting exactly at midnight ends the previous day's block.
	midnight := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 7, 0, 0),
		Duration: time.Hour,
	}
	ok, _ = Conflicts(allDay, midnight, from, to, nil, nil)
	if ok {
		t.Fatal("event starting exactly at next midnight is outside the all-day [day, day+1) block")
	}
}

func TestMultiDayTimedEvent(t *testing.T) {
	utc := time.UTC
	a := &Schedule{
		DTStart:  time.Date(2024, time.March, 6, 9, 0, 0, 0, utc),
		Duration: 26 * time.Hour, // runs through Mar 7 11:00
	}
	b := &Schedule{
		DTStart:  time.Date(2024, time.March, 7, 10, 0, 0, 0, utc),
		Duration: time.Hour,
	}
	from := time.Date(2024, time.March, 7, 0, 0, 0, 0, utc)
	to := time.Date(2024, time.March, 8, 0, 0, 0, 0, utc)
	ok, _ := Conflicts(a, b, from, to, nil, nil)
	if !ok {
		t.Fatal("event starting the day before still occupies the room at 10:00")
	}
}

func TestFloatingVsZonedConflict(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	floating := &Schedule{
		DTStart:  time.Date(2024, time.March, 6, 9, 0, 0, 0, Floating),
		Duration: time.Hour,
	}
	zoned := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 6, 9, 30),
		Duration: time.Hour,
	}
	from := zonedTime(ny, 2024, time.March, 6, 0, 0)
	to := zonedTime(ny, 2024, time.March, 7, 0, 0)
	ok, err := Conflicts(floating, zoned, from, to, ny, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("floating 09:00 pinned to the NY room clashes with NY 09:30")
	}
}

func TestSpringForwardNoConflictForSkippedBooking(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	// A floating 02:30 daily booking pinned into the NY room simply
	// does not occur on the spring-forward Sunday.
	floating := &Schedule{
		DTStart:  time.Date(2024, time.March, 9, 2, 30, 0, 0, Floating),
		Rule:     mustParse(t, "FREQ=DAILY;COUNT=3"),
		Duration: 30 * time.Minute,
	}
	other := &Schedule{
		// 03:30-04:00 local on the gap day (only EDT 02:30 wall exists at 03:30Z? use 04:30)
		DTStart:  zonedTime(ny, 2024, time.March, 10, 4, 30),
		Duration: 30 * time.Minute,
	}
	from := zonedTime(ny, 2024, time.March, 9, 0, 0)
	to := zonedTime(ny, 2024, time.March, 12, 0, 0)
	ok, err := Conflicts(floating, other, from, to, ny, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("the nonexistent 02:30 booking cannot conflict with the 04:30 one")
	}
}

func TestFallBackOverlapResolvedOnInstant(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	// Ambiguous 01:30 resolves to the first (EDT, 05:30Z) instant.
	a := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.November, 3, 1, 30),
		Duration: time.Hour, // 05:30Z-06:30Z
	}
	b := &Schedule{
		DTStart:  time.Date(2024, time.November, 3, 6, 0, 0, 0, time.UTC),
		Duration: time.Hour, // 06:00Z-07:00Z
	}
	from := time.Date(2024, time.November, 3, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, time.November, 4, 0, 0, 0, 0, time.UTC)
	ok, err := Conflicts(a, b, from, to, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first-instant 01:30 (05:30Z) overlaps the 06:00Z booking")
	}
}

func TestRecurringConflict(t *testing.T) {
	utc := time.UTC
	a := &Schedule{
		DTStart:  time.Date(2024, time.March, 4, 9, 0, 0, 0, utc), // Mon
		Rule:     mustParse(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=5"),
		Duration: time.Hour,
	}
	b := &Schedule{
		DTStart:  time.Date(2024, time.March, 18, 9, 30, 0, 0, utc),
		Duration: time.Hour,
	}
	from := time.Date(2024, time.March, 1, 0, 0, 0, 0, utc)
	to := time.Date(2024, time.April, 30, 0, 0, 0, 0, utc)
	x, y, hit, err := FirstConflict(a, b, from, to, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("weekly Monday standup must clash on Mar 18")
	}
	if x.Start.Day() != 18 || y.Start.Day() != 18 {
		t.Fatalf("conflict pair on wrong day: %v %v", x.Start, y.Start)
	}
}
