package rrule

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) *Rule {
	t.Helper()
	r, err := ParseRule(s)
	if err != nil {
		t.Fatalf("ParseRule(%q): %v", s, err)
	}
	return r
}

func floatTime(y int, m time.Month, d, h, mi int) time.Time {
	return time.Date(y, m, d, h, mi, 0, 0, Floating)
}

func windowFloat(y int, m time.Month, d int, days int) (time.Time, time.Time) {
	return floatTime(y, m, d, 0, 0), floatTime(y, m, d+days, 23, 59)
}

func TestDailyIntervalCount(t *testing.T) {
	s := &Schedule{
		DTStart:  floatTime(2024, time.March, 1, 9, 0),
		Rule:     mustParse(t, "FREQ=DAILY;INTERVAL=2;COUNT=5"),
		Duration: time.Hour,
	}
	from, to := windowFloat(2024, time.January, 1, 365)
	got, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d occurrences, want 5", len(got))
	}
	want := []int{1, 3, 5, 7, 9}
	for i, o := range got {
		if o.Wall.Day != want[i] {
			t.Fatalf("occ %d: got day %d, want %d", i, o.Wall.Day, want[i])
		}
		if !o.Floating || !o.Instant.IsZero() {
			t.Fatalf("occ %d: want floating without instant", i)
		}
		if o.Wall.Hour != 9 {
			t.Fatalf("occ %d: got hour %d, want 9", i, o.Wall.Hour)
		}
	}
}

func TestMonthEnd31(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.January, 31, 9, 0),
		Rule:    mustParse(t, "FREQ=MONTHLY;BYMONTHDAY=31;COUNT=6"),
	}
	from, to := windowFloat(2024, time.January, 1, 366)
	got, _ := Expand(s, from, to)
	want := []struct {
		m time.Month
		d int
	}{
		{time.January, 31}, {time.March, 31}, {time.May, 31},
		{time.July, 31}, {time.August, 31}, {time.October, 31},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d, want %d", len(got), len(want))
	}
	for i, o := range got {
		if o.Wall.Month != want[i].m || o.Wall.Day != want[i].d {
			t.Fatalf("occ %d = %v %d, want %v %d", i, o.Wall.Month, o.Wall.Day, want[i].m, want[i].d)
		}
	}
}

func TestMonthEnd31Implicit(t *testing.T) {
	// No BYMONTHDAY: DTSTART day 31; short months are skipped, including Feb in leap 2024.
	s := &Schedule{
		DTStart: floatTime(2023, time.December, 31, 9, 0),
		Rule:    mustParse(t, "FREQ=MONTHLY;COUNT=3"),
	}
	from, to := windowFloat(2023, time.December, 1, 160)
	got, _ := Expand(s, from, to)
	want := []struct {
		y int
		m time.Month
	}{
		{2023, time.December}, {2024, time.January}, {2024, time.March},
	}
	if len(got) != 3 {
		t.Fatalf("got %d, want 3: %+v", len(got), got)
	}
	for i, o := range got {
		if o.Wall.Year != want[i].y || o.Wall.Month != want[i].m {
			t.Fatalf("occ %d = %d-%v, want %d-%v", i, o.Wall.Year, o.Wall.Month, want[i].y, want[i].m)
		}
	}
}

func TestNegativeMonthDay(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.February, 29, 9, 0),
		Rule:    mustParse(t, "FREQ=MONTHLY;BYMONTHDAY=-1;COUNT=4"),
	}
	from, to := windowFloat(2024, time.February, 1, 150)
	got, _ := Expand(s, from, to)
	want := []int{29, 31, 30, 31} // last days Feb, Mar, Apr, May
	wantMonths := []time.Month{time.February, time.March, time.April, time.May}
	if len(got) != 4 {
		t.Fatalf("got %d, want 4: %+v", len(got), got)
	}
	for i, o := range got {
		if o.Wall.Day != want[i] || o.Wall.Month != wantMonths[i] {
			t.Fatalf("occ %d = %v-%d, want %v-%d", i, o.Wall.Month, o.Wall.Day, wantMonths[i], want[i])
		}
	}
}

func TestYearlyFeb29(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.February, 29, 9, 0),
		Rule:    mustParse(t, "FREQ=YEARLY;COUNT=3"),
	}
	from, to := windowFloat(2024, time.January, 1, 365*10)
	got, _ := Expand(s, from, to)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3 leap-day instances", len(got))
	}
	for i, o := range got {
		if o.Wall.Year != 2024+i*4 || o.Wall.Month != time.February || o.Wall.Day != 29 {
			t.Fatalf("occ %d = %d-%v-%d, want leap day", i, o.Wall.Year, o.Wall.Month, o.Wall.Day)
		}
	}
}

func TestOrdinalByDaySecondMonday(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.January, 8, 9, 0),
		Rule:    mustParse(t, "FREQ=MONTHLY;BYDAY=2MO;COUNT=3"),
	}
	from, to := windowFloat(2024, time.January, 1, 120)
	got, _ := Expand(s, from, to)
	want := []int{8, 12, 11} // 2nd Monday Jan, Feb, Mar
	if len(got) != 3 {
		t.Fatalf("got %d, want 3: %+v", len(got), got)
	}
	for i, o := range got {
		if o.Wall.Day != want[i] {
			t.Fatalf("occ %d day = %d, want %d (%v)", i, o.Wall.Day, want[i], o.Wall.Month)
		}
	}
}

func TestOrdinalByDayLastFriday(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.January, 26, 9, 0),
		Rule:    mustParse(t, "FREQ=MONTHLY;BYDAY=-1FR;COUNT=3"),
	}
	from, to := windowFloat(2024, time.January, 1, 120)
	got, _ := Expand(s, from, to)
	want := []int{26, 23, 29} // last Fridays Jan, Feb (23rd), Mar
	if len(got) != 3 {
		t.Fatalf("got %d, want 3: %+v", len(got), got)
	}
	for i, o := range got {
		if o.Wall.Day != want[i] {
			t.Fatalf("occ %d day = %d, want %d (%v)", i, o.Wall.Day, want[i], o.Wall.Month)
		}
	}
}

func TestBySetPosLastWeekday(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.January, 31, 9, 0),
		Rule:    mustParse(t, "FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1;COUNT=3"),
	}
	from, to := windowFloat(2024, time.January, 1, 120)
	got, _ := Expand(s, from, to)
	want := []struct {
		m time.Month
		d int
	}{{time.January, 31}, {time.February, 29}, {time.March, 29}}
	if len(got) != 3 {
		t.Fatalf("got %d, want 3: %+v", len(got), got)
	}
	for i, o := range got {
		if o.Wall.Month != want[i].m || o.Wall.Day != want[i].d {
			t.Fatalf("occ %d = %v %d, want %v %d", i, o.Wall.Month, o.Wall.Day, want[i].m, want[i].d)
		}
	}
}

func TestExDateAndRDate(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 4, 9, 0), // Mon
		Rule:    mustParse(t, "FREQ=WEEKLY;BYDAY=MO;COUNT=4"),
		ExDate:  []time.Time{floatTime(2024, time.March, 11, 9, 0)},
		RDate:   []time.Time{floatTime(2024, time.March, 12, 9, 0)},
	}
	from, to := windowFloat(2024, time.March, 1, 31)
	got, _ := Expand(s, from, to)
	want := map[string]bool{
		"2024-03-04": true,
		"2024-03-12": true, // RDATE
		"2024-03-18": true,
		"2024-03-25": true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d, want %d: %+v", len(got), len(want), got)
	}
	for _, o := range got {
		key := o.Wall.asTime().Format("2006-01-02")
		if !want[key] {
			t.Fatalf("unexpected occurrence %s (Mar 11 should be EXDATE)", key)
		}
	}
}

func TestClosedWindowBoundaries(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 1, 9, 0),
		Rule:    mustParse(t, "FREQ=DAILY;COUNT=5"),
	}
	from := floatTime(2024, time.March, 2, 9, 0)
	to := floatTime(2024, time.March, 3, 9, 0)
	got, _ := Expand(s, from, to)
	if len(got) != 2 {
		t.Fatalf("closed window: got %d (%+v), want exactly Mar 2 and Mar 3", len(got), got)
	}
}

func TestOneShot(t *testing.T) {
	s := &Schedule{
		DTStart:  floatTime(2024, time.March, 1, 9, 0),
		Duration: time.Hour,
	}
	from, to := windowFloat(2024, time.February, 25, 10)
	got, err := Expand(s, from, to)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %d, err=%v", len(got), err)
	}
}

func TestUntil(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 1, 9, 0),
		Rule:    mustParse(t, "FREQ=DAILY;UNTIL=20240305T090000"),
	}
	from, to := windowFloat(2024, time.March, 1, 10)
	got, _ := Expand(s, from, to)
	if len(got) != 5 || got[4].Wall.Day != 5 {
		t.Fatalf("UNTIL must be inclusive: %+v", got)
	}
}

func TestUntilDateOnly(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 1, 9, 0),
		Rule:    mustParse(t, "FREQ=DAILY;UNTIL=20240303"),
	}
	from, to := windowFloat(2024, time.March, 1, 10)
	got, _ := Expand(s, from, to)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
}

func TestRequiresWindow(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 1, 9, 0),
		Rule:    mustParse(t, "FREQ=DAILY"),
	}
	if _, err := Expand(s, time.Time{}, floatTime(2025, time.January, 1, 0, 0)); err == nil {
		t.Fatal("want error when window is unbounded")
	}
}

func TestWeeklyByDay(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 4, 10, 0),
		Rule:    mustParse(t, "FREQ=WEEKLY;BYDAY=MO,WE,FR;COUNT=6"),
	}
	from, to := windowFloat(2024, time.March, 1, 30)
	got, _ := Expand(s, from, to)
	if len(got) != 6 {
		t.Fatalf("got %d, want 6", len(got))
	}
	want := []time.Weekday{time.Monday, time.Wednesday, time.Friday, time.Monday, time.Wednesday, time.Friday}
	for i, o := range got {
		if o.Wall.asTime().Weekday() != want[i] {
			t.Fatalf("occ %d weekday = %v, want %v", i, o.Wall.asTime().Weekday(), want[i])
		}
	}
}

func TestWeeklyIntervalWKST(t *testing.T) {
	s := &Schedule{
		DTStart: floatTime(2024, time.March, 7, 8, 0),
		Rule:    mustParse(t, "FREQ=WEEKLY;INTERVAL=2;BYDAY=TU,SA;WKST=MO"),
	}
	from, to := windowFloat(2024, time.March, 4, 28)
	got, _ := Expand(s, from, to)
	var days []int
	for _, o := range got {
		days = append(days, o.Wall.Day)
	}
	// DTSTART is Thu Mar 7; the earlier weekday Tue Mar 5 in the same
	// WKST week is never emitted because no instance precedes DTSTART.
	wantDays := []int{7, 9, 19, 23}
	if len(days) != len(wantDays) {
		t.Fatalf("days = %v, want %v", days, wantDays)
	}
	for i := range days {
		if days[i] != wantDays[i] {
			t.Fatalf("days = %v, want %v", days, wantDays)
		}
	}
}
