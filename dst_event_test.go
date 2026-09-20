package rrule

import (
	"testing"
	"time"
)

func TestWindowIsClosed(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	start := at(2024, time.March, 9, 2, 30, ny)
	out := expand(t, start, "FREQ=DAILY", start, start)
	if len(out) != 1 || !out[0].Equal(start) {
		t.Fatalf("closed window must include DTSTART, got %v", out)
	}
}

func TestDSTSpringForwardGapSkipped(t *testing.T) {
	// 2024-03-10 02:30 does not exist in New York: skipped, not moved.
	ny := mustLoc(t, "America/New_York")
	start := at(2024, time.March, 9, 2, 30, ny)
	cal := &Calendar{DTStart: start, Rule: rules(t, "FREQ=DAILY")}
	out, err := cal.Between(
		at(2024, time.March, 10, 0, 0, ny),
		at(2024, time.March, 10, 23, 59, ny))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("gap occurrence must not exist, got %v", out)
	}

	start2 := at(2024, time.March, 3, 2, 30, ny) // Sunday
	cal = &Calendar{DTStart: start2, Rule: rules(t, "FREQ=WEEKLY;BYDAY=SU")}
	out, _ = cal.Between(
		at(2024, time.March, 10, 0, 0, ny),
		at(2024, time.March, 10, 23, 59, ny))
	if len(out) != 0 {
		t.Fatalf("weekly gap occurrence must be skipped, got %v", out)
	}
}

func TestDSTGapDTStartDoesNotExist(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	// Go normalizes a nonexistent time.Date forward, so callers express a
	// gap DTSTART by parsing a naive wall time; verify buildWall reports
	// the gap and the expansion skips it using a rule that lands on
	// the gap day (covered by TestDSTSpringForwardGapSkipped). Here we
	// pin the buildWall contract directly.
	if gapT, gap := buildWall(2024, time.March, 10, 2, 30, 0, ny, FirstAmbiguous); !gap {
		t.Fatalf("02:30 must be a gap, got %v", gapT)
	}
	if gapT, gap := buildWall(2024, time.March, 10, 3, 30, 0, ny, FirstAmbiguous); gap || gapT.Hour() != 3 {
		t.Fatalf("03:30 must exist, got %v gap=%v", gapT, gap)
	}
	start := at(2024, time.March, 9, 2, 30, ny)
	cal := &Calendar{DTStart: start, Rule: rules(t, "FREQ=DAILY;COUNT=3")}
	out, err := cal.Between(
		at(2024, time.March, 8, 0, 0, ny),
		at(2024, time.March, 20, 0, 0, ny))
	if err != nil {
		t.Fatal(err)
	}
	// Mar 9 anchor, Mar 10 gap skipped (not counted), Mar 11/12 fill
	// the count of three; the gap day is absent from the output.
	var got []string
	for _, o := range out {
		got = append(got, o.Format("2006-01-02 15:04"))
	}
	want := []string{"2024-03-09 02:30", "2024-03-11 02:30", "2024-03-12 02:30"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestCountIncludesDTStart(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	start := at(2024, time.March, 4, 9, 0, ny)
	cal := &Calendar{DTStart: start, Rule: rules(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=2")}
	out, err := cal.Between(
		at(2024, time.March, 1, 0, 0, ny),
		at(2024, time.April, 1, 0, 0, ny))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("count should include DTSTART, got %v", out)
	}
	if out[0].Format("2006-01-02") != "2024-03-04" ||
		out[1].Format("2006-01-02") != "2024-03-11" {
		t.Fatalf("got %v", out)
	}
}

func TestDSTFallBackSingle(t *testing.T) {
	// 2024-11-03 01:30 exists twice (EDT, then EST). Exactly one emit.
	ny := mustLoc(t, "America/New_York")
	// DTSTART safely before the fallback; the fallback day is one period later.
	start := at(2024, time.November, 1, 1, 30, ny)
	from := at(2024, time.November, 3, 0, 0, ny)
	to := at(2024, time.November, 3, 23, 59, ny)

	cal := &Calendar{DTStart: start, Rule: rules(t, "FREQ=DAILY;INTERVAL=2")}
	out, err := cal.Between(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("fallback must emit once, got %v", out)
	}
	if off := offsetSeconds(out[0]); off != -4*3600 {
		t.Fatalf("default should keep EDT (-04:00), got offset %d", off)
	}

	r2 := rules(t, "FREQ=DAILY;INTERVAL=2")
	r2.Ambiguous = SecondAmbiguous
	cal = &Calendar{DTStart: start, Rule: r2}
	out, _ = cal.Between(from, to)
	if len(out) != 1 || offsetSeconds(out[0]) != -5*3600 {
		t.Fatalf("SecondAmbiguous should keep EST, got %v", out)
	}
}

func TestLondonDST(t *testing.T) {
	// London springs forward 2024-03-31 01:00 -> 02:00; a 01:30 daily
	// meeting vanishes that day.
	lon := mustLoc(t, "Europe/London")
	start := at(2024, time.March, 30, 1, 30, lon)
	cal := &Calendar{DTStart: start, Rule: rules(t, "FREQ=DAILY")}
	out, _ := cal.Between(
		at(2024, time.March, 31, 0, 0, lon),
		at(2024, time.March, 31, 23, 59, lon))
	if len(out) != 0 {
		t.Fatalf("London gap day must emit nothing, got %v", out)
	}
}

func TestFloatingWallClock(t *testing.T) {
	start := time.Date(2024, time.March, 9, 2, 30, 0, 0, Floating)
	cal := &Calendar{DTStart: start, Rule: rules(t, "FREQ=DAILY")}
	out, err := cal.Between(
		time.Date(2024, time.March, 10, 0, 0, 0, 0, Floating),
		time.Date(2024, time.March, 10, 23, 59, 0, 0, Floating))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Hour() != 2 || out[0].Minute() != 30 ||
		out[0].Location() != Floating {
		t.Fatalf("floating: got %v", out)
	}
}

func TestEXDATEAndRDATE(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	start := at(2024, time.March, 4, 9, 0, ny) // Monday
	ex := at(2024, time.March, 11, 9, 0, ny)
	rd := at(2024, time.March, 13, 14, 0, ny)
	cal := &Calendar{
		DTStart: start,
		Rule:    rules(t, "FREQ=WEEKLY;BYDAY=MO"),
		ExDates: []time.Time{ex},
		RDates:  []time.Time{rd},
	}
	out, err := cal.Between(
		at(2024, time.March, 4, 0, 0, ny),
		at(2024, time.March, 18, 23, 59, ny))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range out {
		got = append(got, o.Format("02 15:04"))
	}
	want := []string{"04 09:00", "13 14:00", "18 09:00"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func offsetSeconds(t time.Time) int {
	_, off := t.Zone()
	return off
}

func TestParseDate(t *testing.T) {
	if _, err := ParseDate("20240310"); err != nil {
		t.Fatal(err)
	}
	tm, err := ParseDate("20240310T133000Z")
	if err != nil {
		t.Fatal(err)
	}
	if tm.Location() != time.UTC || tm.Hour() != 13 {
		t.Fatalf("utc parse: %v", tm)
	}
	ny := mustLoc(t, "America/New_York")
	tm, _ = ParseDateIn("20240310T090000", ny)
	if _, off := tm.Zone(); off != -4*3600 {
		t.Fatalf("tzid parse offset: %v", tm)
	}
	f, _ := ParseDate("20240310T090000")
	if f.Location() != Floating {
		t.Fatalf("naive should be floating: %v", f)
	}
}
