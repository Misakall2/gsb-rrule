package rrule

import (
	"testing"
	"time"
)

// New York summer weekly Monday meeting; a yearly EXRULE strips the
// observed July 4 holiday (second Monday of July 2024).
func TestEXRuleStripsHoliday(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.July, 1, 9, 0),
		Rule:     mustParse(t, "FREQ=WEEKLY;BYDAY=MO;UNTIL=20240729T130000Z"),
		ExRule:   []*Rule{mustParse(t, "FREQ=YEARLY;BYMONTH=7;BYDAY=2MO;COUNT=1")},
		Duration: time.Hour,
	}
	from := zonedTime(ny, 2024, time.July, 1, 0, 0)
	to := zonedTime(ny, 2024, time.July, 30, 0, 0)
	got, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	wantDays := []int{1, 15, 22, 29} // Jul 8 (holiday) removed
	if len(got) != len(wantDays) {
		t.Fatalf("got %d occurrences %+v, want %d", len(got), got, len(wantDays))
	}
	for i, o := range got {
		if o.Wall.Day != wantDays[i] || o.Wall.Month != time.July {
			t.Fatalf("occ %d = %v %d, want July %d", i, o.Wall.Month, o.Wall.Day, wantDays[i])
		}
	}
}

func TestEXRuleCountAndUntil(t *testing.T) {
	// COUNT: exclude exactly the first Monday.
	count := &Schedule{
		DTStart: floatTime(2024, time.March, 4, 9, 0),
		Rule:    mustParse(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=4"),
		ExRule:  []*Rule{mustParse(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=1")},
	}
	from, to := windowFloat(2024, time.March, 1, 31)
	got, err := Expand(count, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Wall.Day != 11 {
		t.Fatalf("COUNT EXRULE: got %+v, want Mar 11/18/25", got)
	}

	// UNTIL on the EXRULE, shared with the main DTSTART: Monday
	// instances up to and including Mar 11 are excluded.
	until := &Schedule{
		DTStart: floatTime(2024, time.March, 4, 9, 0),
		Rule:    mustParse(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=4"),
		ExRule:  []*Rule{mustParse(t, "FREQ=WEEKLY;BYDAY=MO;UNTIL=20240311T090000")},
	}
	got, err = Expand(until, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Wall.Day != 18 || got[1].Wall.Day != 25 {
		t.Fatalf("UNTIL EXRULE: got %+v, want Mar 18/25", got)
	}

	// An EXRULE without COUNT/UNTIL is clamped by the main rule's
	// UNTIL and must terminate, excluding every matching instance.
	open := &Schedule{
		DTStart: floatTime(2024, time.March, 4, 9, 0),
		Rule:    mustParse(t, "FREQ=WEEKLY;BYDAY=MO;UNTIL=20240318T090000"),
		ExRule:  []*Rule{mustParse(t, "FREQ=WEEKLY;BYDAY=MO")},
	}
	got, err = Expand(open, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("open-ended EXRULE bounded by main UNTIL: got %+v, want none", got)
	}
}

func TestFloatingDailyMergedBusy(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	daily := &Schedule{
		DTStart:  floatTime(2024, time.July, 1, 9, 0),
		Rule:     mustParse(t, "FREQ=DAILY;COUNT=3"),
		Duration: time.Hour,
	}
	// Jul 2 10:00-11:00 meets the daily 09:00-10:00 end-to-start.
	followup := &Schedule{
		DTStart:  floatTime(2024, time.July, 2, 10, 0),
		Duration: time.Hour,
	}
	from := time.Date(2024, time.July, 1, 0, 0, 0, 0, ny)
	to := time.Date(2024, time.July, 4, 0, 0, 0, 0, ny)
	busy, err := BusyIntervals([]Booking{
		{Schedule: daily, Loc: ny},
		{Schedule: followup, Loc: ny},
	}, from, to)
	if err != nil {
		t.Fatal(err)
	}
	// July is EDT (UTC-4): blocks land at 13:00Z; Jul 2 merges to 15:00Z.
	want := []Interval{
		{Start: time.Date(2024, time.July, 1, 13, 0, 0, 0, time.UTC),
			End: time.Date(2024, time.July, 1, 14, 0, 0, 0, time.UTC)},
		{Start: time.Date(2024, time.July, 2, 13, 0, 0, 0, time.UTC),
			End: time.Date(2024, time.July, 2, 15, 0, 0, 0, time.UTC)},
		{Start: time.Date(2024, time.July, 3, 13, 0, 0, 0, time.UTC),
			End: time.Date(2024, time.July, 3, 14, 0, 0, 0, time.UTC)},
	}
	if len(busy) != len(want) {
		t.Fatalf("got %d busy blocks %+v, want %d", len(busy), busy, len(want))
	}
	for i := range want {
		if !busy[i].Start.Equal(want[i].Start) || !busy[i].End.Equal(want[i].End) {
			t.Fatalf("busy[%d] = %v..%v, want %v..%v", i, busy[i].Start, busy[i].End, want[i].Start, want[i].End)
		}
	}
}

func TestCrossZoneDurationBusy(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	lon := loadZone(t, "Europe/London")
	// 09:00-10:30 EST = 14:00-15:30Z; 14:00-15:30 GMT = the same window.
	nyMeeting := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 6, 9, 0),
		Duration: 90 * time.Minute,
	}
	lonMeeting := &Schedule{
		DTStart:  zonedTime(lon, 2024, time.March, 6, 14, 0),
		Duration: 90 * time.Minute,
	}
	from := time.Date(2024, time.March, 6, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, time.March, 7, 0, 0, 0, 0, time.UTC)
	conflict, err := Conflicts(nyMeeting, lonMeeting, from, to, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !conflict {
		t.Fatal("two 90-minute cross-zone meetings occupy the same 14:00-15:30Z window")
	}
	busy, err := BusyIntervals([]Booking{
		{Schedule: nyMeeting},
		{Schedule: lonMeeting},
	}, from, to)
	if err != nil {
		t.Fatal(err)
	}
	wantStart := time.Date(2024, time.March, 6, 14, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2024, time.March, 6, 15, 30, 0, 0, time.UTC)
	if len(busy) != 1 || !busy[0].Start.Equal(wantStart) || !busy[0].End.Equal(wantEnd) {
		t.Fatalf("busy = %+v, want single merged %v..%v", busy, wantStart, wantEnd)
	}
}

func TestMonthEnd31DurationCrossesMonth(t *testing.T) {
	s := &Schedule{
		DTStart:  time.Date(2024, time.January, 31, 23, 0, 0, 0, time.UTC),
		Rule:     mustParse(t, "FREQ=MONTHLY;BYMONTHDAY=31;COUNT=2"),
		Duration: 3 * time.Hour, // runs into Feb 1 / Apr 1
	}
	from := time.Date(2024, time.January, 31, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, time.April, 2, 0, 0, 0, 0, time.UTC)
	ivs, err := Intervals(s, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ivs) != 2 {
		t.Fatalf("got %d intervals, want 2 (Jan 31, Mar 31): %+v", len(ivs), ivs)
	}
	if ivs[0].Start.Day() != 31 || ivs[0].End.Day() != 1 || ivs[0].End.Month() != time.February {
		t.Fatalf("first interval = %v..%v, want Jan 31 23:00 -> Feb 1 02:00", ivs[0].Start, ivs[0].End)
	}
	if ivs[1].End.Month() != time.April || ivs[1].End.Day() != 1 {
		t.Fatalf("second interval = %v..%v, want Mar 31 -> Apr 1", ivs[1].Start, ivs[1].End)
	}

	// A Feb 1 01:30 event still conflicts with the January instance.
	feb := &Schedule{
		DTStart:  time.Date(2024, time.February, 1, 1, 30, 0, 0, time.UTC),
		Duration: time.Hour,
	}
	ok, err := Conflicts(s, feb,
		time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("the month-end booking still occupies the room on Feb 1 01:30")
	}
}

func TestDurationAcrossDSTKeepsEndInstant(t *testing.T) {
	ny := loadZone(t, "America/New_York")

	// Spring forward: Mar 10 00:00 EST (05:00Z) plus 4 hours ends at
	// 05:00 local (09:00Z); the end wall clock is not shifted back.
	spring := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 10, 0, 0),
		Duration: 4 * time.Hour,
	}
	from := zonedTime(ny, 2024, time.March, 9, 0, 0)
	to := zonedTime(ny, 2024, time.March, 11, 0, 0)
	ivs, err := Intervals(spring, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantEnd := time.Date(2024, time.March, 10, 9, 0, 0, 0, time.UTC)
	if len(ivs) != 1 || !ivs[0].End.Equal(wantEnd) {
		t.Fatalf("spring-forward end = %+v, want %v", ivs, wantEnd)
	}

	// Fall back: Nov 3 01:00 EDT (05:00Z) plus 5 hours ends at 06:00
	// local EST (10:00Z), spanning the repeated wall hour exactly once.
	fall := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.November, 3, 1, 0),
		Duration: 5 * time.Hour,
	}
	from = zonedTime(ny, 2024, time.November, 2, 0, 0)
	to = zonedTime(ny, 2024, time.November, 4, 0, 0)
	ivs, err = Intervals(fall, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantEnd = time.Date(2024, time.November, 3, 10, 0, 0, 0, time.UTC)
	if len(ivs) != 1 || !ivs[0].End.Equal(wantEnd) {
		t.Fatalf("fall-back end = %+v, want %v", ivs, wantEnd)
	}
}

func TestMergeBusyAdjacentOverlappingOrdered(t *testing.T) {
	base := time.Date(2024, time.March, 1, 9, 0, 0, 0, time.UTC)
	ivs := []Interval{
		{Start: base.Add(3 * time.Hour), End: base.Add(4 * time.Hour)},    // out of order
		{Start: base, End: base.Add(time.Hour)},                           // start
		{Start: base.Add(time.Hour), End: base.Add(2 * time.Hour)},        // adjacent
		{Start: base.Add(90 * time.Minute), End: base.Add(3 * time.Hour)}, // overlaps
		{Start: base.Add(5 * time.Hour), End: base.Add(5 * time.Hour)},    // zero, dropped
	}
	got := MergeBusy(ivs)
	if len(got) != 1 {
		t.Fatalf("merged = %+v, want one block", got)
	}
	if !got[0].Start.Equal(base) || !got[0].End.Equal(base.Add(4*time.Hour)) {
		t.Fatalf("merged = %v..%v, want %v..%v", got[0].Start, got[0].End, base, base.Add(4*time.Hour))
	}
}
