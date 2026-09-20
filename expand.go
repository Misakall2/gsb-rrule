package rrule

import (
	"fmt"
	"sort"
	"time"
)

// Calendar binds a DTSTART to an RRULE plus EXDATE/RDATE exceptions.
//
// The location of DTSTART decides the mode of every occurrence:
//   - a real zone (for example from time.LoadLocation("America/New_York"))
//     makes the rule a TZID rule;
//   - time.UTC makes it a UTC rule;
//   - Floating makes it a zone-less wall-clock rule.
//
// EXDATE/RDATE values must use the same mode (absolute or floating) as
// DTSTART.
type Calendar struct {
	DTStart time.Time
	Rule    *Rule
	ExDates []time.Time
	RDates  []time.Time
}

const maxPeriods = 1_000_000

// Between expands the rule and returns every occurrence inside the closed
// window [from, to].
//
// A window is mandatory even when the rule has neither COUNT nor UNTIL, so an
// unbounded rule can never loop forever. Window endpoints use the same mode
// as DTSTART: absolute instants for zoned/UTC rules, wall-clock times in the
// Floating location for floating rules.
func (c *Calendar) Between(from, to time.Time) ([]time.Time, error) {
	if c.Rule == nil {
		return nil, fmt.Errorf("rrule: calendar has no rule")
	}
	r := c.Rule
	if r.Interval <= 0 {
		r.Interval = 1
	}
	if to.Before(from) {
		return nil, fmt.Errorf("rrule: empty window: to before from")
	}
	if r.HasUntil && modesDiffer(c.DTStart, r.Until) {
		return nil, fmt.Errorf("rrule: UNTIL time zone mode must match DTSTART")
	}
	for _, ex := range c.ExDates {
		if modesDiffer(c.DTStart, ex) {
			return nil, fmt.Errorf("rrule: EXDATE time zone mode must match DTSTART")
		}
	}

	loc := c.DTStart.Location()
	hh, mi, ss := c.DTStart.Hour(), c.DTStart.Minute(), c.DTStart.Second()
	sy0, sm0, sd0 := c.DTStart.Date()

	emitted := map[string]bool{}
	var out []time.Time
	count := 0

	add := func(t time.Time) bool {
		key := instKey(t)
		if emitted[key] {
			return true
		}
		if r.HasUntil && afterTime(t, r.Until) {
			return false
		}
		if r.Count > 0 && count >= r.Count {
			return false
		}
		emitted[key] = true
		count++
		if !beforeTime(t, from) && !afterTime(t, to) && !isExcluded(c.ExDates, t) {
			out = append(out, t)
		}
		return true
	}

	// DTSTART anchors the sequence. If its own wall time falls inside a
	// spring-forward gap, the anchor simply does not exist.
	var anchor time.Time
	if t0, gap := buildWall(sy0, sm0, sd0, hh, mi, ss, loc, r.Ambiguous); !gap {
		anchor = t0
	}
	anchorValid := !anchor.IsZero()

	limitYear := maxYear(to)

	for k := 0; k < maxPeriods; k++ {
		dates := periodDates(r, c.DTStart, k)
		var cands []time.Time
		for _, d := range dates {
			t, gap := buildWall(d.year, d.month, d.day, hh, mi, ss, loc, r.Ambiguous)
			if gap {
				continue
			}
			cands = append(cands, t)
		}
		cands = applySetPos(r, cands)
		sortTimes(cands)
		stop := false
		for _, t := range cands {
			// The recurrence starts at DTSTART; earlier candidates inside
			// period 0 (e.g. a BYDAY before DTSTART's own date) are not
			// occurrences.
			if anchorValid && beforeTime(t, anchor) {
				continue
			}
			if !add(t) {
				stop = true
				break
			}
		}
		if stop {
			break
		}
		first := firstDate(r, c.DTStart, k+1)
		if first.year > limitYear+1 {
			break
		}
		ft, _ := buildWall(first.year, first.month, first.day, 0, 0, 0, loc, FirstAmbiguous)
		if r.HasUntil && ft.After(r.Until) {
			break
		}
		if r.Count == 0 && !r.HasUntil && afterTime(ft, to) {
			break
		}
	}

	for _, rd := range c.RDates {
		if modesDiffer(c.DTStart, rd) {
			return nil, fmt.Errorf("rrule: RDATE time zone mode must match DTSTART")
		}
		if !beforeTime(rd, from) && !afterTime(rd, to) && !isExcluded(c.ExDates, rd) {
			out = append(out, rd)
		}
	}

	sortTimes(out)
	return dedupe(out), nil
}

// dayDate is a calendar date in the rule's zone.
type dayDate struct {
	year  int
	month time.Month
	day   int
}

// firstDate is the earliest possible candidate date of period k.
func firstDate(r *Rule, start time.Time, k int) dayDate {
	y, m, d := start.Date()
	switch r.Frequency {
	case Daily:
		return addDays(dayDate{y, m, d}, k*r.Interval)
	case Weekly:
		ws := weekStart(dayDate{y, m, d}, r.Wkst)
		return addDays(ws, k*7*r.Interval)
	case Monthly:
		m2 := time.Month(int(m-1) + k*r.Interval + 1)
		return dayDate{y, m2, 1}
	case Yearly:
		return dayDate{y + k*r.Interval, m, 1}
	}
	return dayDate{y, m, d}
}

// periodDates returns every candidate date within period k, before BYSETPOS.
func periodDates(r *Rule, start time.Time, k int) []dayDate {
	sy, sm, sd := start.Date()
	switch r.Frequency {
	case Daily:
		d := addDays(dayDate{sy, sm, sd}, k*r.Interval)
		if len(r.ByDay) == 0 {
			return []dayDate{d}
		}
		for _, wd := range r.ByDay {
			if wd.Ord == 0 && weekdayOf(d) == wd.Day {
				return []dayDate{d}
			}
		}
		return nil

	case Weekly:
		ws := weekStart(dayDate{sy, sm, sd}, r.Wkst)
		base := addDays(ws, k*7*r.Interval)
		days := r.ByDay
		if len(days) == 0 {
			days = []Weekday{{Day: start.Weekday()}}
		}
		var out []dayDate
		for _, wd := range days {
			if wd.Ord != 0 {
				continue
			}
			off := (int(wd.Day) - int(r.Wkst) + 7) % 7
			out = append(out, addDays(base, off))
		}
		return out

	case Monthly:
		m := time.Month(int(sm-1) + k*r.Interval + 1)
		y := sy + int((m-1)/12)
		m = ((m-1)%12+12)%12 + 1
		return monthCandidates(r, start, y, m, sd)

	case Yearly:
		y := sy + k*r.Interval
		return monthCandidates(r, start, y, sm, sd)
	}
	return nil
}

// monthCandidates builds candidates for one month. For YEARLY the scope is the
// DTSTART month; BYMONTHDAY/BYDAY are interpreted inside that month.
func monthCandidates(r *Rule, start time.Time, y int, m time.Month, startDay int) []dayDate {
	n := daysInMonth(y, m)
	var out []dayDate

	addDay := func(d int) {
		if d >= 1 && d <= n {
			out = append(out, dayDate{y, m, d})
		}
	}

	for _, md := range r.ByMonthDay {
		if md > 0 {
			addDay(md)
		} else {
			addDay(n + md + 1)
		}
	}

	for _, wd := range r.ByDay {
		if wd.Ord == 0 {
			for d := 1; d <= n; d++ {
				if time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Weekday() == wd.Day {
					addDay(d)
				}
			}
			continue
		}
		if d, ok := nthWeekday(y, m, n, wd.Day, wd.Ord); ok {
			addDay(d)
		}
	}

	if len(r.ByMonthDay) == 0 && len(r.ByDay) == 0 {
		addDay(startDay)
	}
	return dedupeDates(out)
}

// applySetPos reduces the sorted period candidates using BYSETPOS.
func applySetPos(r *Rule, cands []time.Time) []time.Time {
	if len(r.BySetPos) == 0 {
		return cands
	}
	sortTimes(cands)
	var out []time.Time
	for _, p := range r.BySetPos {
		idx := p
		if p < 0 {
			idx = len(cands) + p + 1
		}
		if idx >= 1 && idx <= len(cands) {
			out = append(out, cands[idx-1])
		}
	}
	return out
}

// buildWall constructs an occurrence from a date plus shared clock parts.
//
// gap is true when the wall time does not exist because clocks sprang
// forward. On a fall-back repeat the wall time exists twice; the
// AmbiguousTime option selects one instant, so the rule never emits twice for
// one wall time.
func buildWall(y int, m time.Month, d, hh, mi, ss int, loc *time.Location, amb AmbiguousTime) (time.Time, bool) {
	if loc == nil || loc == Floating {
		return time.Date(y, m, d, hh, mi, ss, 0, Floating), false
	}
	t := time.Date(y, m, d, hh, mi, ss, 0, loc)
	if t.Hour() != hh || t.Minute() != mi || t.Second() != ss {
		// Spring-forward gap: Go normalized forward over the missing time.
		return time.Time{}, true
	}
	// Detect a repeated wall time by comparing with the wall time two hours
	// away. During a fall-back the offset changes by the DTA (typically 1h);
	// the instant one DTA away maps back onto the same wall clock.
	_, offAt := t.Zone()
	for _, delta := range []time.Duration{time.Hour, 2 * time.Hour} {
		alt := t.Add(delta)
		if alt.Hour() == hh && alt.Minute() == mi && alt.Second() == ss {
			_, offAlt := alt.Zone()
			if offAlt != offAt {
				if amb == SecondAmbiguous {
					return alt, false
				}
				return t, false
			}
		}
	}
	return t, false
}

// nthWeekday resolves ordinals such as 2 (second Monday) or -1 (last
// Friday) within a month.
func nthWeekday(y int, m time.Month, n int, wd time.Weekday, ord int) (int, bool) {
	if ord > 0 {
		count := 0
		for d := 1; d <= n; d++ {
			if time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Weekday() == wd {
				count++
				if count == ord {
					return d, true
				}
			}
		}
		return 0, false
	}
	count := 0
	for d := n; d >= 1; d-- {
		if time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Weekday() == wd {
			count++
			if count == -ord {
				return d, true
			}
		}
	}
	return 0, false
}

func weekStart(d dayDate, wkst time.Weekday) dayDate {
	off := (int(weekdayOf(d)) - int(wkst) + 7) % 7
	return addDays(d, -off)
}

func weekdayOf(d dayDate) time.Weekday {
	return time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC).Weekday()
}

func addDays(d dayDate, n int) dayDate {
	t := time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n)
	return dayDate{t.Year(), t.Month(), t.Day()}
}

func daysInMonth(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func dedupeDates(ds []dayDate) []dayDate {
	seen := map[dayDate]bool{}
	var out []dayDate
	for _, d := range ds {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].year != out[j].year {
			return out[i].year < out[j].year
		}
		if out[i].month != out[j].month {
			return out[i].month < out[j].month
		}
		return out[i].day < out[j].day
	})
	return out
}

func sortTimes(ts []time.Time) {
	sort.Slice(ts, func(i, j int) bool { return ts[i].Before(ts[j]) })
}

func dedupe(ts []time.Time) []time.Time {
	seen := map[string]bool{}
	var out []time.Time
	for _, t := range ts {
		k := instKey(t)
		if !seen[k] {
			seen[k] = true
			out = append(out, t)
		}
	}
	return out
}

func maxYear(t time.Time) int {
	if t.Location() == Floating {
		return t.Year()
	}
	return t.UTC().Year()
}
