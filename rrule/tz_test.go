package rrule

import (
	"testing"
	"time"
)

func loadZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("timezone database unavailable for %q: %v", name, err)
	}
	return loc
}

func zonedTime(loc *time.Location, y int, m time.Month, d, h, mi int) time.Time {
	return time.Date(y, m, d, h, mi, 0, 0, loc)
}

func TestNYSpringForwardSkipsNonexistentTime(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	// 2024-03-10 02:30 does not exist: EST jumps 02:00 -> 03:00.
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 9, 2, 30),
		Rule:     mustParse(t, "FREQ=DAILY;COUNT=3"),
		Duration: time.Hour,
	}
	from := zonedTime(ny, 2024, time.March, 9, 0, 0)
	to := zonedTime(ny, 2024, time.March, 12, 0, 0)
	got, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	// Mar 9 02:30, Mar 10 skipped, Mar 11 02:30.
	if len(got) != 2 {
		t.Fatalf("got %d occurrences, want 2 (gap occurrence skipped): %+v", len(got), got)
	}
	if got[0].Wall.Day != 9 || got[1].Wall.Day != 11 {
		t.Fatalf("days = %d,%d want 9,11", got[0].Wall.Day, got[1].Wall.Day)
	}
	for _, o := range got {
		if o.Floating {
			t.Fatal("zoned occurrence must not be floating")
		}
		if o.Instant.Location() != ny {
			t.Fatalf("instant lost its zone: %v", o.Instant.Location())
		}
	}
	// Mar 11 02:30 is EDT, i.e. 06:30 UTC.
	if got[1].Instant.UTC().Format("15:04") != "06:30" {
		t.Fatalf("Mar 11 02:30 EDT = %v UTC, want 06:30", got[1].Instant.UTC())
	}
}

func TestNYFallBackHappensOnce(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	// 2024-11-03 01:30 occurs twice on the wall but must expand once,
	// resolving to the first (EDT) instant: 05:30 UTC.
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.November, 2, 1, 30),
		Rule:     mustParse(t, "FREQ=DAILY;COUNT=3"),
		Duration: time.Hour,
	}
	from := zonedTime(ny, 2024, time.November, 2, 0, 0)
	to := zonedTime(ny, 2024, time.November, 5, 0, 0)
	got, _ := Expand(s, from, to)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3; fall-back must not duplicate an occurrence: %+v", len(got), got)
	}
	fb := got[1]
	if fb.Wall.Day != 3 {
		t.Fatalf("second occ day = %d, want 3", fb.Wall.Day)
	}
	if fb.Instant.UTC().Format("15:04") != "05:30" {
		t.Fatalf("ambiguous 01:30 resolved to %v UTC, want 05:30 (first/EDT)", fb.Instant.UTC())
	}
}

func TestUTCSchedule(t *testing.T) {
	s := &Schedule{
		DTStart:  time.Date(2024, time.March, 1, 9, 0, 0, 0, time.UTC),
		Rule:     mustParse(t, "FREQ=DAILY;COUNT=2"),
		Duration: 30 * time.Minute,
	}
	from := time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC)
	got, _ := Expand(s, from, to)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	for _, o := range got {
		if o.Floating || o.Instant.Location() != time.UTC {
			t.Fatalf("UTC schedule lost its zone: %+v", o)
		}
	}
}

func TestLondonDSTWithUntilZ(t *testing.T) {
	lon := loadZone(t, "Europe/London")
	// Last Sunday of March (the 31st) is the spring-forward day in London.
	s := &Schedule{
		DTStart:  zonedTime(lon, 2024, time.March, 29, 9, 0),
		Rule:     mustParse(t, "FREQ=DAILY;UNTIL=20240402T090000Z"),
		Duration: time.Hour,
	}
	from := zonedTime(lon, 2024, time.March, 29, 0, 0)
	to := zonedTime(lon, 2024, time.April, 5, 0, 0)
	got, _ := Expand(s, from, to)
	// 09:00 exists every day; UNTIL 09:00Z on Apr 2 = 10:00 BST, so
	// Apr 2 09:00 BST (08:00Z) is included, Apr 3 is not.
	if len(got) != 5 {
		t.Fatalf("got %d occurrences %+v, want Mar29..Apr2", len(got), got)
	}
	// Mar 31 09:00 BST = 08:00 UTC.
	if got[2].Instant.UTC().Format("15:04") != "08:00" {
		t.Fatalf("Mar 31 09:00 BST = %v UTC, want 08:00", got[2].Instant.UTC())
	}
}

func TestFloatingPinnedInResourceZone(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart:  time.Date(2024, time.March, 9, 2, 30, 0, 0, Floating),
		Rule:     mustParse(t, "FREQ=DAILY;COUNT=3"),
		Duration: time.Hour,
	}
	from := time.Date(2024, time.March, 9, 0, 0, 0, 0, Floating)
	to := time.Date(2024, time.March, 12, 0, 0, 0, 0, Floating)
	occs, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	// Floating expansion ignores zones: all three wall clocks survive.
	if len(occs) != 3 {
		t.Fatalf("floating expand got %d, want 3 wall clocks: %+v", len(occs), occs)
	}
	ivs, err := Intervals(s, from, to, ny)
	if err != nil {
		t.Fatal(err)
	}
	// But when pinned into the New York room, Mar 10 02:30 does not exist.
	if len(ivs) != 2 {
		t.Fatalf("intervals in NY got %d, want 2: %+v", len(ivs), ivs)
	}
}

func TestFloatingNeedsLocation(t *testing.T) {
	o := Occurrence{Wall: WallClock{Year: 2024, Month: time.March, Day: 10, Hour: 9}, Floating: true}
	if _, _, err := OccurrenceInterval(o, time.Hour, nil); err == nil {
		t.Fatal("floating occurrence without resource zone must error")
	}
}
