package rrule

import (
	"fmt"
	"testing"
	"time"
)

// Regression 1: a WEEKLY rule at 02:00 hitting the New York
// spring-forward must never materialize the nonexistent wall time, and
// the skipped instance must not silently consume COUNT.
func TestRegWeeklySpringForwardGap(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 3, 2, 0),
		Rule:     mustParse(t, "FREQ=WEEKLY;BYDAY=SU;COUNT=3"),
		Duration: 30 * time.Minute,
	}
	from := zonedTime(ny, 2024, time.March, 1, 0, 0)
	to := zonedTime(ny, 2024, time.March, 31, 0, 0)
	got, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	wantDays := []int{3, 17, 24} // Mar 10 gap skipped without eating COUNT
	if len(got) != len(wantDays) {
		t.Fatalf("got %d occurrences %v, want %d (gap skipped without eating COUNT)",
			len(got), daysOf(got), len(wantDays))
	}
	for i, o := range got {
		if o.Wall.Day != wantDays[i] {
			t.Fatalf("occ %d day = %d, want %d", i, o.Wall.Day, wantDays[i])
		}
	}
	for _, o := range got {
		if o.Wall.Hour != 2 || o.Wall.Minute != 0 {
			t.Fatalf("wall time moved to %02d:%02d, want 02:00", o.Wall.Hour, o.Wall.Minute)
		}
	}
	for _, o := range got {
		u := o.Instant.UTC()
		if u.Day() == 10 && u.Hour() >= 6 && u.Hour() < 7 {
			t.Fatalf("occurrence materialized inside the gap: %v", u)
		}
	}
}

// Regression 1b: the ambiguous fall-back wall time expands exactly
// once and resolves to the first (EDT) instant.
func TestRegWeeklyFallBackOnce(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.October, 27, 1, 30),
		Rule:     mustParse(t, "FREQ=WEEKLY;BYDAY=SU;COUNT=3"),
		Duration: 30 * time.Minute,
	}
	from := zonedTime(ny, 2024, time.October, 20, 0, 0)
	to := zonedTime(ny, 2024, time.November, 17, 0, 0)
	got, _ := Expand(s, from, to)
	if len(got) != 3 {
		t.Fatalf("got %d occurrences, want 3: %v", len(got), daysOf(got))
	}
	want := []struct {
		day   int
		utcHM string
	}{
		{27, "05:30"},
		{3, "05:30"}, // Nov 3 ambiguous -> first (EDT) instant, once
		{10, "06:30"},
	}
	for i, o := range got {
		if o.Wall.Day != want[i].day {
			t.Fatalf("occ %d day = %d, want %d", i, o.Wall.Day, want[i].day)
		}
		if hm := o.Instant.UTC().Format("15:04"); hm != want[i].utcHM {
			t.Fatalf("occ %d = %s UTC, want %s", i, hm, want[i].utcHM)
		}
	}
}

// Regression 2a: zoned DTSTART with a UTC ("...Z") UNTIL bounds on the
// instant line, not by comparing naked wall clocks.
func TestRegZonedUntilZ(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	// 09:30 EST daily = 14:30Z. UNTIL 14:30Z on Mar 5 includes Mar 5.
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 1, 9, 30),
		Rule:     mustParse(t, "FREQ=DAILY;UNTIL=20240305T143000Z"),
		Duration: time.Minute,
	}
	from := zonedTime(ny, 2024, time.March, 1, 0, 0)
	to := zonedTime(ny, 2024, time.March, 10, 0, 0)
	got, _ := Expand(s, from, to)
	if len(got) != 5 {
		t.Fatalf("got %d occurrences, want Mar1..Mar5: %v", len(got), daysOf(got))
	}
	last := got[len(got)-1]
	if last.Wall.Day != 5 || last.Instant.UTC().Format("15:04") != "14:30" {
		t.Fatalf("last = day %d %vZ, want Mar 5 14:30Z", last.Wall.Day, last.Instant.UTC())
	}

	// One second earlier on the UTC line cuts the Mar 5 instance.
	s2 := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 1, 9, 30),
		Rule:     mustParse(t, "FREQ=DAILY;UNTIL=20240305T142959Z"),
		Duration: time.Minute,
	}
	got2, _ := Expand(s2, from, to)
	if len(got2) != 4 || got2[len(got2)-1].Wall.Day != 4 {
		t.Fatalf("UNTIL just before the Mar 5 instant: got %v, want last day 4", daysOf(got2))
	}
}

// Regression 2b: a floating UNTIL mixed with a zoned DTSTART compares
// wall clocks as written.
func TestRegZonedFloatingUntil(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 1, 9, 30),
		Rule:     mustParse(t, "FREQ=DAILY;UNTIL=20240305T093000"),
		Duration: time.Minute,
	}
	from := zonedTime(ny, 2024, time.March, 1, 0, 0)
	to := zonedTime(ny, 2024, time.March, 10, 0, 0)
	got, _ := Expand(s, from, to)
	if len(got) != 5 || got[len(got)-1].Wall.Day != 5 {
		t.Fatalf("floating UNTIL against zoned rule: got %v, want last day 5", daysOf(got))
	}
}

// Regression 2c: floating DTSTART plus a Z UNTIL gives an exact count;
// the UTC fields are taken literally as wall clocks.
func TestRegFloatingDTStartUntilZ(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 1, 9, 30),
		Rule:    mustParse(t, "FREQ=DAILY;UNTIL=20240305T093000Z"),
	}
	from, to := windowFloat(2024, time.March, 1, 10)
	got, _ := Expand(s, from, to)
	if len(got) != 5 || got[len(got)-1].Wall.Day != 5 {
		t.Fatalf("floating DTSTART + Z UNTIL: got %v, want Mar1..Mar5", daysOf(got))
	}
}

// Regression 2d: UNTIL cuts within a BYSETPOS period: earlier picks
// in that period survive, later ones (and subsequent periods) stop.
func TestRegUntilCutsInsideSetPosPeriod(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 1, 9, 0),
		Rule:    mustParse(t, "FREQ=MONTHLY;BYDAY=MO,WE,FR;BYSETPOS=1,2,3;UNTIL=20240410T090000"),
	}
	from, to := windowFloat(2024, time.March, 1, 60)
	got, _ := Expand(s, from, to)
	want := []string{"3-1", "3-4", "3-6", "4-1", "4-3", "4-5"}
	if len(got) != len(want) {
		t.Fatalf("got %d occurrences, want %v", len(got), want)
	}
	for i, o := range got {
		key := fmt.Sprintf("%d-%d", o.Wall.Month, o.Wall.Day)
		if key != want[i] {
			t.Fatalf("occ %d = %s, want %s", i, key, want[i])
		}
	}
}

// Regression 3: negative BYSETPOS over BYDAY indexes the chronologically
// sorted candidate set even when BYMONTH is listed out of order.
func TestRegBySetPosNegativeSorted(t *testing.T) {
	// Generator order is Mar, Jan, Feb. Without chronological sorting,
	// BYSETPOS=-1 over BYDAY=SU would pick the last Sunday the
	// generator emitted (February's: 2024-02-25); the correct answer is
	// the chronologically last Sunday of Jan..Mar (2024-03-31).
	s := &Schedule{
		DTStart: floatTime(2024, time.January, 1, 9, 0),
		Rule:    mustParse(t, "FREQ=YEARLY;BYMONTH=3,1,2;BYDAY=SU;BYSETPOS=-1"),
	}
	from, to := windowFloat(2024, time.January, 1, 400)
	got, _ := Expand(s, from, to)
	if len(got) < 2 {
		t.Fatalf("got %d occurrences", len(got))
	}
	// Jan 1 is the anchored DTSTART (this library always anchors the
	// first instance); the rule-generated set begins after it.
	ruleOut := got[1:]
	if len(ruleOut) == 0 {
		t.Fatal("no rule-generated occurrences")
	}
	first := ruleOut[0]
	if first.Wall.Month != time.March || first.Wall.Day != 31 {
		t.Fatalf("2024 last Sunday in Jan-Mar = %v %d, want Mar 31", first.Wall.Month, first.Wall.Day)
	}
	// 2025: Mar 30, never the generator-last month's Feb 23.
	if len(ruleOut) < 2 {
		from2, to2 := windowFloat(2024, time.January, 1, 800)
		got2, _ := Expand(s, from2, to2)
		ruleOut = got2[1:]
	}
	if len(ruleOut) < 2 {
		t.Fatalf("rule-generated occurrences = %d, want >= 2", len(ruleOut))
	}
	second := ruleOut[1]
	if second.Wall.Year != 2025 || second.Wall.Month != time.March || second.Wall.Day != 30 {
		t.Fatalf("2025 last Sunday in Jan-Mar = %d-%v-%d, want 2025-Mar-30",
			second.Wall.Year, second.Wall.Month, second.Wall.Day)
	}
}

// Regression 3b: monthly negative BYSETPOS over BYDAY verified against
// a direct calendar scan for leap February and the 31-day months.
func TestRegBySetPosLastFridays(t *testing.T) {
	for y := 2020; y <= 2030; y++ {
		s := &Schedule{
			DTStart: floatTime(y, time.January, 1, 9, 0),
			Rule:    mustParse(t, "FREQ=MONTHLY;BYDAY=FR;BYSETPOS=-1"),
		}
		from, to := windowFloat(y, time.January, 1, 380)
		got, _ := Expand(s, from, to)
		for _, o := range got {
			if o.Wall.Month == time.January && o.Wall.Day == 1 {
				continue // anchored DTSTART, not a rule-generated pick
			}
			want := lastWeekdayOfMonth(o.Wall.Year, o.Wall.Month, time.Friday)
			if o.Wall.Day != want {
				t.Fatalf("%d-%v: last Friday = %d, want %d",
					o.Wall.Year, o.Wall.Month, o.Wall.Day, want)
			}
		}
	}
}

func lastWeekdayOfMonth(year int, month time.Month, wd time.Weekday) int {
	daysIn := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	for d := daysIn; d >= 1; d-- {
		if time.Date(year, month, d, 0, 0, 0, 0, time.UTC).Weekday() == wd {
			return d
		}
	}
	return 0
}

// Regression 4: an all-day block and a timed 00:00 occurrence follow
// one half-open overlap rule on every overlap code path: same midnight
// clashes, the next midnight (back-to-back) never clashes.
func TestRegAllDayVersusMidnightConsistent(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	allDay := &Schedule{DTStart: zonedTime(ny, 2024, time.March, 6, 0, 0), AllDay: true}
	sameMid := &Schedule{DTStart: zonedTime(ny, 2024, time.March, 6, 0, 0), Duration: time.Hour}
	nextMid := &Schedule{DTStart: zonedTime(ny, 2024, time.March, 7, 0, 0), Duration: time.Hour}
	from := zonedTime(ny, 2024, time.March, 6, 0, 0)
	to := zonedTime(ny, 2024, time.March, 8, 0, 0)

	for name, b := range map[string]*Schedule{"same-midnight": sameMid, "next-midnight": nextMid} {
		want := name == "same-midnight"
		if ok, _ := Conflicts(allDay, b, from, to, nil, nil); ok != want {
			t.Fatalf("%s: Conflicts = %v, want %v", name, ok, want)
		}
		if _, _, hit, _ := FirstConflict(allDay, b, from, to, nil, nil); hit != want {
			t.Fatalf("%s: FirstConflict hit = %v, want %v", name, hit, want)
		}
		ia, _ := Intervals(allDay, from, to, nil)
		ib, _ := Intervals(b, from, to, nil)
		direct := false
		for _, x := range ia {
			for _, y := range ib {
				if x.Overlaps(y) {
					direct = true
				}
			}
		}
		if direct != want {
			t.Fatalf("%s: direct Overlaps = %v, want %v", name, direct, want)
		}
	}
}

// Regression 4b: floating all-day pinned into a resource zone uses the
// same rule, including across the spring-forward night.
func TestRegFloatingAllDayVersusMidnight(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	fAllDay := &Schedule{DTStart: floatTime(2024, time.March, 9, 0, 0), AllDay: true}
	nextMid := &Schedule{DTStart: zonedTime(ny, 2024, time.March, 10, 0, 0), Duration: time.Hour}
	sameMid := &Schedule{DTStart: zonedTime(ny, 2024, time.March, 9, 0, 0), Duration: time.Hour}
	from := zonedTime(ny, 2024, time.March, 9, 0, 0)
	to := zonedTime(ny, 2024, time.March, 11, 0, 0)
	if ok, _ := Conflicts(fAllDay, nextMid, from, to, ny, nil); ok {
		t.Fatal("floating all-day Mar 9 must not clash with Mar 10 00:00 (half-open)")
	}
	if ok, _ := Conflicts(fAllDay, sameMid, from, to, ny, nil); !ok {
		t.Fatal("floating all-day Mar 9 must clash with a Mar 9 00:00 booking")
	}
}

// Regression 5: expansion is a closed window for occurrence starts, and
// busy output keeps an occurrence starting exactly at the window end,
// matching what Conflicts reports.
func TestRegWindowRightEdgeConsistent(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	// One-hour booking starting exactly at the window end (next midnight).
	atEdge := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 7, 0, 0),
		Duration: time.Hour,
	}
	allDay := &Schedule{DTStart: zonedTime(ny, 2024, time.March, 7, 0, 0), AllDay: true}
	from := zonedTime(ny, 2024, time.March, 6, 0, 0)
	to := zonedTime(ny, 2024, time.March, 7, 0, 0)

	occs, err := Expand(atEdge, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(occs) != 1 {
		t.Fatalf("Expand closed window: got %d, want the midnight occurrence", len(occs))
	}
	busy, err := BusyIntervals([]*Schedule{atEdge}, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(busy) != 1 || !busy[0].Start.Equal(to.UTC()) {
		t.Fatalf("busy must retain the span starting at the closed window end: %+v", busy)
	}
	if ok, _ := Conflicts(allDay, atEdge, from, to, nil, nil); !ok {
		t.Fatal("all-day starting at window end clashes with the timed booking at that instant")
	}

	// Left edge stays half-open for occupancy: a span ending exactly at
	// the window start contributes nothing.
	endsAtStart := &Schedule{
		DTStart:  zonedTime(ny, 2024, time.March, 5, 23, 0),
		Duration: time.Hour,
	}
	busy2, _ := BusyIntervals([]*Schedule{endsAtStart}, from, to, nil)
	if len(busy2) != 0 {
		t.Fatalf("span ending exactly at window start must be excluded: %+v", busy2)
	}
}
