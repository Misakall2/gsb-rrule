package rrule

import (
	"testing"
	"time"
)

func daysOf(occs []Occurrence) []int {
	days := make([]int, len(occs))
	for i, o := range occs {
		days[i] = o.Wall.Day
	}
	return days
}

// New York summer meetings: a weekly Monday standup plus a one-off
// RDATE on the Jul 4 Independence Day holiday (a Thursday). The
// counter-rule generates Thursdays and stops (UNTIL) just after the
// holiday, so it removes only Jul 4.
func TestNYSummerStandupWithHolidayExRule(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.July, 1, 9, 0),
		Rule:     mustParse(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=3"), // Jul 1, 8, 15
		RDate:    []time.Time{zonedTime(ny, 2024, time.July, 4, 9, 0)},
		ExRule:   mustParse(t, "FREQ=WEEKLY;BYDAY=TH;UNTIL=20240704T120000"),
		Duration: time.Hour,
	}
	from := zonedTime(ny, 2024, time.June, 30, 0, 0)
	to := zonedTime(ny, 2024, time.July, 16, 0, 0)

	occs, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		m time.Month
		d int
	}{
		{time.July, 1}, {time.July, 8}, {time.July, 15},
	}
	if len(occs) != len(want) {
		t.Fatalf("got %v, want the 3 Mondays, with the Jul 4 holiday cut", daysOf(occs))
	}
	for i, o := range occs {
		if o.Wall.Month != want[i].m || o.Wall.Day != want[i].d {
			t.Fatalf("occ %d = %v %d, want %v %d", i, o.Wall.Month, o.Wall.Day, want[i].m, want[i].d)
		}
		if o.Instant.UTC().Format("15:04") != "13:00" {
			t.Fatalf("July Monday 09:00 EDT must be 13:00Z, got %v", o.Instant.UTC())
		}
	}
}

// EXRULE without COUNT or UNTIL must still terminate: the expansion
// window bounds the counter-rule generation, and every matching
// instance inside the window is removed.
func TestOpenEndedExRuleTerminates(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 4, 9, 0),         // Mon
		Rule:    mustParse(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=3"), // 4,11,18
		ExRule:  mustParse(t, "FREQ=WEEKLY;BYDAY=MO"),         // open-ended, never hangs
	}
	from, to := windowFloat(2024, time.March, 11, 14) // visible Mar 11..25
	occs, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	// Only Mar 18 is visible and matches the counter-rule; it is cut.
	if got := daysOf(occs); len(got) != 0 {
		t.Fatalf("open-ended EXRULE: got days %v, want []", got)
	}
}

// An EXRULE with its own COUNT removes only that many counter-instances.
func TestExRuleOwnCount(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 4, 9, 0),
		Rule:    mustParse(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=4"), // 4,11,18,25
		ExRule:  mustParse(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=2"),
	}
	from, to := windowFloat(2024, time.March, 1, 25)
	occs, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if got := daysOf(occs); len(got) != 2 || got[0] != 18 || got[1] != 25 {
		t.Fatalf("EXRULE COUNT=2: got %v, want [18 25]", got)
	}
}

// Floating daily 09:00 standups pinned into a New York room merge with
// bookings that touch (half-open boundary rule) or overlap them.
func TestFloatingDailyMergesIntoBusy(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	daily := &Schedule{
		DTStart:  floatTime(2024, time.March, 4, 9, 0),
		Rule:     mustParse(t, "FREQ=DAILY;COUNT=3"),
		Duration: time.Hour,
	}
	touching := &Schedule{
		DTStart:  floatTime(2024, time.March, 4, 10, 0),
		Duration: 30 * time.Minute,
	}
	overlap := &Schedule{
		DTStart:  floatTime(2024, time.March, 5, 9, 30),
		Duration: time.Hour,
	}

	from := zonedTime(ny, 2024, time.March, 4, 0, 0)
	to := zonedTime(ny, 2024, time.March, 7, 0, 0)
	busy, err := BusyIntervals([]*Schedule{daily, touching, overlap}, from, to, ny)
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 3 {
		t.Fatalf("got %d blocks %v, want 3", len(busy), busy)
	}
	checks := []struct {
		startYDay                  int
		startH, startM, endH, endM int
	}{
		{4, 9, 0, 10, 30},
		{5, 9, 0, 10, 30},
		{6, 9, 0, 10, 0},
	}
	for i, c := range checks {
		ss := busy[i].Start.In(ny)
		ee := busy[i].End.In(ny)
		if ss.Day() != c.startYDay || ss.Hour() != c.startH || ss.Minute() != c.startM ||
			ee.Hour() != c.endH || ee.Minute() != c.endM {
			t.Fatalf("block %d = %v..%v, want Mar%d %02d:%02d-%02d:%02d NY",
				i, ss, ee, c.startYDay, c.startH, c.startM, c.endH, c.endM)
		}
	}
}

// Two meetings in different zones whose durations overlap on the UTC
// time line merge into one busy block.
func TestCrossZoneDurationMerges(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	lon := loadZone(t, "Europe/London")
	londonMeeting := &Schedule{
		DTStart:  zonedTime(lon, 2024, time.March, 6, 10, 0), // 10:00-11:30Z
		Duration: 90 * time.Minute,
	}
	nyMeeting := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 6, 6, 0), // 11:00-12:00Z
		Duration: time.Hour,
	}
	from := time.Date(2024, time.March, 6, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, time.March, 7, 0, 0, 0, 0, time.UTC)
	busy, err := BusyIntervals([]*Schedule{londonMeeting, nyMeeting}, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 1 {
		t.Fatalf("got %d blocks %v, want one merged block", len(busy), busy)
	}
	wantStart := time.Date(2024, time.March, 6, 10, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2024, time.March, 6, 12, 0, 0, 0, time.UTC)
	if !busy[0].Start.Equal(wantStart) || !busy[0].End.Equal(wantEnd) {
		t.Fatalf("merged = %v..%v, want 10:00Z..12:00Z", busy[0].Start, busy[0].End)
	}
}

// Month-end (day 31) occurrences with a duration crossing into the
// next month still occupy the room at month boundaries.
func TestMonthEnd31DurationCrossesMonth(t *testing.T) {
	s := &Schedule{
		DTStart:  time.Date(2024, time.January, 31, 22, 0, 0, 0, time.UTC),
		Rule:     mustParse(t, "FREQ=MONTHLY;BYMONTHDAY=31;COUNT=3"),
		Duration: 4 * time.Hour,
	}
	from := time.Date(2024, time.January, 31, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, time.June, 2, 0, 0, 0, 0, time.UTC)
	busy, err := BusyIntervals([]*Schedule{s}, from, to, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 3 {
		t.Fatalf("got %d blocks %v, want 3 month-end blocks", len(busy), busy)
	}
	wantEnd := time.Date(2024, time.February, 1, 2, 0, 0, 0, time.UTC)
	if !busy[0].End.Equal(wantEnd) {
		t.Fatalf("Jan 31 block ends %v, want Feb 1 02:00", busy[0].End)
	}
}

// A fixed duration across the New York fall-back keeps its absolute
// length; the end wall clock shifts instead of drifting.
func TestDurationAcrossFallBackKeepsLength(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.November, 3, 0, 30),
		Duration: 3 * time.Hour,
	}
	from := zonedTime(ny, 2024, time.November, 3, 0, 0)
	to := zonedTime(ny, 2024, time.November, 4, 0, 0)
	ivs, err := Intervals(s, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ivs) != 1 {
		t.Fatalf("got %d intervals, want 1", len(ivs))
	}
	if d := ivs[0].End.Sub(ivs[0].Start); d != 3*time.Hour {
		t.Fatalf("duration across fall-back = %v, want 3h", d)
	}
	if got := ivs[0].End.UTC().Format("15:04"); got != "07:30" {
		t.Fatalf("end = %vZ, want 07:30Z (02:30 EST)", got)
	}
	if got := ivs[0].End.In(ny).Format("15:04"); got != "02:30" {
		t.Fatalf("end local = %v, want 02:30 wall", got)
	}
}

// The same fixed duration across the spring-forward jump is still 3
// absolute hours; the end wall clock moves with the gap.
func TestDurationAcrossSpringForwardKeepsLength(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 10, 0, 30),
		Duration: 3 * time.Hour,
	}
	from := zonedTime(ny, 2024, time.March, 10, 0, 0)
	to := zonedTime(ny, 2024, time.March, 11, 0, 0)
	ivs, err := Intervals(s, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ivs) != 1 {
		t.Fatalf("got %d intervals, want 1", len(ivs))
	}
	if d := ivs[0].End.Sub(ivs[0].Start); d != 3*time.Hour {
		t.Fatalf("duration across spring-forward = %v, want 3h", d)
	}
	if got := ivs[0].End.In(ny).Format("15:04"); got != "04:30" {
		t.Fatalf("end local = %v, want 04:30 wall after jump", got)
	}
}

// Busy output clips to the window, stays ordered and keeps real gaps.
func TestBusyClippingAndOrder(t *testing.T) {
	utc := time.UTC
	long := &Schedule{
		DTStart:  time.Date(2024, time.March, 5, 20, 0, 0, 0, utc),
		Duration: 6 * time.Hour, // ends Mar 6 02:00
	}
	next := &Schedule{
		DTStart:  time.Date(2024, time.March, 6, 2, 0, 0, 0, utc),
		Duration: time.Hour, // touches long's end
	}
	later := &Schedule{
		DTStart:  time.Date(2024, time.March, 6, 5, 0, 0, 0, utc),
		Duration: time.Hour,
	}
	from := time.Date(2024, time.March, 6, 0, 0, 0, 0, utc)
	to := time.Date(2024, time.March, 6, 12, 0, 0, 0, utc)
	busy, err := BusyIntervals([]*Schedule{later, long, next}, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 2 {
		t.Fatalf("got %d blocks %v, want [00:00-03:00, 05:00-06:00]", len(busy), busy)
	}
	if !busy[0].Start.Equal(from) || !busy[0].End.Equal(time.Date(2024, time.March, 6, 3, 0, 0, 0, utc)) {
		t.Fatalf("block 0 = %v..%v", busy[0].Start, busy[0].End)
	}
	if busy[1].Start.Hour() != 5 || busy[1].End.Hour() != 6 {
		t.Fatalf("block 1 = %v..%v", busy[1].Start, busy[1].End)
	}
}

// All-day and timed zoned schedules on the same resource merge by the
// existing all-day semantics.
func TestAllDayAndTimedMerge(t *testing.T) {
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
	to := zonedTime(ny, 2024, time.March, 7, 12, 0)
	busy, err := BusyIntervals([]*Schedule{allDay, meeting}, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 1 {
		t.Fatalf("got %d blocks %v, want one block Mar 6 00:00 - Mar 7 00:30 NY", len(busy), busy)
	}
	if !busy[0].Start.Equal(zonedTime(ny, 2024, time.March, 6, 0, 0).UTC()) ||
		!busy[0].End.Equal(zonedTime(ny, 2024, time.March, 7, 0, 30).UTC()) {
		t.Fatalf("block = %v..%v", busy[0].Start, busy[0].End)
	}
}
