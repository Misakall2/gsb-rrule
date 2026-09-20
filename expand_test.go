package rrule

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func at(y int, m time.Month, d, hh, mi int, loc *time.Location) time.Time {
	return time.Date(y, m, d, hh, mi, 0, 0, loc)
}

func rules(t *testing.T, s string) *Rule {
	t.Helper()
	r, err := ParseRule(s, time.UTC)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return r
}

func expand(t *testing.T, start time.Time, rule string, from, to time.Time) []time.Time {
	t.Helper()
	cal := &Calendar{DTStart: start, Rule: rules(t, rule)}
	out, err := cal.Between(from, to)
	if err != nil {
		t.Fatalf("between: %v", err)
	}
	return out
}

func TestDailyIntervalUTC(t *testing.T) {
	start := at(2024, time.January, 1, 9, 0, time.UTC)
	out := expand(t, start, "FREQ=DAILY;INTERVAL=2",
		at(2024, time.January, 1, 0, 0, time.UTC),
		at(2024, time.January, 7, 23, 59, time.UTC))
	wantDays := []int{1, 3, 5, 7}
	if len(out) != len(wantDays) {
		t.Fatalf("got %d occurrences %v, want %d", len(out), out, len(wantDays))
	}
	for i, d := range wantDays {
		if out[i].Day() != d || out[i].Hour() != 9 {
			t.Fatalf("occ %d = %v, want Jan %d 09:00", i, out[i], d)
		}
	}
}

func TestWeeklyByDayWithWKST(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	start := at(2024, time.March, 4, 12, 0, ny) // Monday
	out := expand(t, start, "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,FR;WKST=MO",
		at(2024, time.March, 1, 0, 0, ny),
		at(2024, time.March, 31, 23, 59, ny))
	var got []string
	for _, o := range out {
		got = append(got, o.Format("02 Mon 15:04"))
	}
	want := []string{"04 Mon 12:00", "08 Fri 12:00", "18 Mon 12:00", "22 Fri 12:00"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestMonthlyByMonthDay31(t *testing.T) {
	// 31st of the month: Feb and 30-day months emit nothing.
	start := at(2023, time.December, 31, 10, 0, time.UTC)
	out := expand(t, start, "FREQ=MONTHLY;BYMONTHDAY=31",
		at(2024, time.January, 1, 0, 0, time.UTC),
		at(2025, time.March, 1, 0, 0, time.UTC))
	var got []string
	for _, o := range out {
		got = append(got, o.Format("2006-01-02"))
	}
	want := []string{"2024-01-31", "2024-03-31", "2024-05-31",
		"2024-07-31", "2024-08-31", "2024-10-31", "2024-12-31",
		"2025-01-31"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestFebruaryLeapAndCommon(t *testing.T) {
	// Rule on the 29th, monthly: hits in every month with >=29 days.
	start := at(2024, time.January, 29, 0, 0, time.UTC)
	out := expand(t, start, "FREQ=MONTHLY;BYMONTHDAY=29",
		at(2024, time.January, 29, 0, 0, time.UTC),
		at(2025, time.March, 28, 23, 59, time.UTC))
	var got []string
	for _, o := range out {
		got = append(got, o.Format("2006-01"))
	}
	// Feb 2024 (leap) yes, Feb 2025 (common) no.
	feb2024, feb2025 := false, false
	for _, m := range got {
		if m == "2024-02" {
			feb2024 = true
		}
		if m == "2025-02" {
			feb2025 = true
		}
	}
	if !feb2024 {
		t.Fatalf("leap Feb 29 missing: %v", got)
	}
	if feb2025 {
		t.Fatalf("common Feb must not have 29th: %v", got)
	}

	// Yearly Feb 29: 2024 only within 2023..2025.
	start = at(2024, time.February, 29, 9, 0, time.UTC)
	out = expand(t, start, "FREQ=YEARLY;BYMONTHDAY=29",
		at(2023, time.January, 1, 0, 0, time.UTC),
		at(2025, time.December, 31, 0, 0, time.UTC))
	if len(out) != 1 || out[0].Format("2006-01-02") != "2024-02-29" {
		t.Fatalf("yearly Feb 29: %v", out)
	}
}

func TestMonthlyNthWeekdayAndLast(t *testing.T) {
	start := at(2024, time.March, 1, 9, 0, time.UTC)
	out := expand(t, start, "FREQ=MONTHLY;BYDAY=2MO",
		at(2024, time.March, 1, 0, 0, time.UTC),
		at(2024, time.May, 31, 23, 59, time.UTC))
	wantDays := []int{11, 8, 13}
	if len(out) != 3 {
		t.Fatalf("got %v", out)
	}
	for i, d := range wantDays {
		if out[i].Day() != d {
			t.Fatalf("got %v want days %v", out, wantDays)
		}
	}

	start = at(2024, time.March, 29, 9, 0, time.UTC)
	out = expand(t, start, "FREQ=MONTHLY;BYDAY=-1FR",
		at(2024, time.March, 1, 0, 0, time.UTC),
		at(2024, time.May, 31, 23, 59, time.UTC))
	lastFridays := []int{29, 26, 31}
	if len(out) != 3 {
		t.Fatalf("got %v", out)
	}
	for i, d := range lastFridays {
		if out[i].Day() != d || out[i].Weekday() != time.Friday {
			t.Fatalf("got %v want last Fridays %v", out, lastFridays)
		}
	}
}

func TestBySetPosNegative(t *testing.T) {
	start := at(2024, time.March, 1, 8, 0, time.UTC)
	out := expand(t, start, "FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1",
		at(2024, time.March, 1, 0, 0, time.UTC),
		at(2024, time.April, 30, 23, 59, time.UTC))
	if len(out) != 2 {
		t.Fatalf("got %v", out)
	}
	if out[0].Format("2006-01-02") != "2024-03-29" {
		t.Fatalf("march last weekday: %v", out[0])
	}
	if out[1].Format("2006-01-02") != "2024-04-30" {
		t.Fatalf("april last weekday: %v", out[1])
	}
}

func TestCountAndUntilAndValidation(t *testing.T) {
	start := at(2024, time.January, 1, 9, 0, time.UTC)
	out := expand(t, start, "FREQ=DAILY;COUNT=3",
		at(2000, time.January, 1, 0, 0, time.UTC),
		at(2030, time.January, 1, 0, 0, time.UTC))
	if len(out) != 3 {
		t.Fatalf("count: got %d", len(out))
	}

	r, err := ParseRule("FREQ=DAILY;UNTIL=20240105T090000Z", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	cal := &Calendar{DTStart: start, Rule: r}
	out, err = cal.Between(at(2024, time.January, 1, 0, 0, time.UTC),
		at(2024, time.January, 31, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 5 {
		t.Fatalf("until: got %d (%v)", len(out), out)
	}

	if _, err := ParseRule("FREQ=DAILY;COUNT=3;UNTIL=20240105T090000Z", time.UTC); err == nil {
		t.Fatal("COUNT and UNTIL together must fail")
	}
	if _, err := ParseRule("INTERVAL=2", time.UTC); err == nil {
		t.Fatal("missing FREQ must fail")
	}
}
