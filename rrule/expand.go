package rrule

import (
	"fmt"
	"time"
)

// safety bounds so a misconfigured window or rule cannot loop forever.
const maxPeriods = 200_000

// Expand returns the occurrences of s whose wall-clock start lies in
// the closed window [from, to]. The window must be non-zero and
// from <= to.
//
// The window times use the same convention as s.DTStart: UTC times for
// UTC/floating scheduling of the window for zoned schedules; for
// floating schedules the wall-clock fields are what matters. The
// returned slice is ordered by start time.
//
// DST behaviour for zoned schedules:
//
//   - On a spring-forward, a wall time that does not exist causes that
//     occurrence to be skipped (e.g. 02:30 when 02:00 jumps to 03:00).
//   - On a fall-back, an ambiguous wall time resolves to its first
//     (pre-transition) instant; a single rule instance never expands
//     twice.
func Expand(s *Schedule, from, to time.Time) ([]Occurrence, error) {
	if s == nil {
		return nil, fmt.Errorf("rrule: nil schedule")
	}
	if s.Rule == nil {
		return expandOneShot(s, from, to)
	}
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("rrule: expansion requires a finite [from, to] window")
	}
	if from.After(to) {
		return nil, fmt.Errorf("rrule: from is after to")
	}

	loc := s.DTStart.Location()
	floating := isFloating(loc)

	fromW := naiveOf(from)
	toW := naiveOf(to)

	var untilW *WallClock
	if !s.Rule.Until.IsZero() {
		uw := naiveOf(s.Rule.Until)
		untilW = &uw
	}

	ex := map[WallClock]bool{}
	for _, e := range s.ExDate {
		ex[naiveOf(e)] = true
	}
	for _, walls := range expandExRules(s, naiveOf(s.DTStart), untilW, toW) {
		for _, w := range walls {
			ex[w] = true
		}
	}

	cands := s.Rule.naiveOccurrences(naiveOf(s.DTStart), untilW)

	var out []Occurrence
	count := 0
	for _, w := range cands {
		count++
		if s.Rule.Count > 0 && count > s.Rule.Count {
			break
		}
		if ex[w] {
			continue
		}
		if w.Before(fromW) || toW.Before(w) {
			continue
		}
		occ := Occurrence{Wall: w, Floating: floating, AllDay: s.AllDay}
		if !floating {
			inst, ok := materialize(w, loc)
			if !ok {
				// Nonexistent local wall time (spring-forward gap).
				continue
			}
			occ.Instant = inst
		}
		out = append(out, occ)
	}

	// RDATEs add individual instances (subject to EXDATE).
	seen := map[WallClock]bool{}
	for _, o := range out {
		seen[o.Wall] = true
	}
	for _, r := range s.RDate {
		w := naiveOf(r)
		if ex[w] || seen[w] || w.Before(fromW) || toW.Before(w) {
			continue
		}
		occ := Occurrence{Wall: w, Floating: floating, AllDay: s.AllDay}
		if !floating {
			inst, ok := materialize(w, loc)
			if !ok {
				continue
			}
			occ.Instant = inst
		}
		out = append(out, occ)
		seen[w] = true
	}

	sortOccurrences(out)
	return out, nil
}

// expandExRules materializes the wall-clock starts of every EXRULE.
//
// Each EXRULE shares the schedule DTSTART and obeys its own COUNT and
// UNTIL (COUNT counts from DTSTART, exactly like the main rule). An
// EXRULE without either bound is clamped to the earliest of the main
// rule's UNTIL and the query window end, so exclusion generation can
// never run away; the underlying period generator is also capped by
// maxPeriods.
func expandExRules(s *Schedule, start WallClock, mainUntil *WallClock, windowEnd WallClock) [][]WallClock {
	if len(s.ExRule) == 0 {
		return nil
	}
	out := make([][]WallClock, 0, len(s.ExRule))
	for _, xr := range s.ExRule {
		if xr == nil {
			continue
		}
		var hard *WallClock
		switch {
		case xr.Count > 0 || !xr.Until.IsZero():
			// The rule carries its own terminator.
		case mainUntil != nil && mainUntil.Before(windowEnd):
			u := *mainUntil
			hard = &u
		default:
			hard = &windowEnd
		}
		var until *WallClock
		if !xr.Until.IsZero() {
			u := naiveOf(xr.Until)
			if mainUntil != nil && mainUntil.Before(u) {
				u = *mainUntil
			}
			until = &u
		} else if hard != nil {
			until = hard
		}
		walls := xr.generate(start, until, false)
		if xr.Count > 0 && len(walls) > xr.Count {
			walls = walls[:xr.Count]
		}
		out = append(out, walls)
	}
	return out
}

func expandOneShot(s *Schedule, from, to time.Time) ([]Occurrence, error) {
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("rrule: expansion requires a finite [from, to] window")
	}
	loc := s.DTStart.Location()
	floating := isFloating(loc)
	w := naiveOf(s.DTStart)
	ex := map[WallClock]bool{}
	for _, e := range s.ExDate {
		if naiveOf(e).Equal(w) {
			return nil, nil
		}
	}
	for _, walls := range expandExRules(s, w, nil, naiveOf(to)) {
		for _, exw := range walls {
			ex[exw] = true
		}
	}
	if ex[w] {
		return nil, nil
	}
	fw, tw := naiveOf(from), naiveOf(to)
	if w.Before(fw) || tw.Before(w) {
		return nil, nil
	}
	occ := Occurrence{Wall: w, Floating: floating, AllDay: s.AllDay}
	if !floating {
		inst, ok := materialize(w, loc)
		if !ok {
			return nil, nil
		}
		occ.Instant = inst
	}
	return []Occurrence{occ}, nil
}

// materialize converts a wall clock into an instant in loc. The bool
// is false when the wall time does not exist (a DST spring gap).
// Ambiguous fall-back times resolve to the first (earlier) instant.
func materialize(w WallClock, loc *time.Location) (time.Time, bool) {
	t := time.Date(w.Year, w.Month, w.Day, w.Hour, w.Minute, w.Second, 0, loc)
	// t is always a valid instant; detect gaps by re-reading the wall.
	rt := t.In(loc)
	if rt.Hour() != w.Hour || rt.Minute() != w.Minute || rt.Second() != w.Second ||
		rt.Day() != w.Day || rt.Month() != w.Month || rt.Year() != w.Year {
		return time.Time{}, false
	}
	return t, true
}

func sortOccurrences(o []Occurrence) {
	// small-slice insertion sort; occurrence counts can be large for
	// DAILY rules but windows stay bounded in practice.
	for i := 1; i < len(o); i++ {
		for j := i; j > 0 && o[j].Wall.Before(o[j-1].Wall); j-- {
			o[j], o[j-1] = o[j-1], o[j]
		}
	}
}

// naiveOccurrences generates the full ordered wall-clock start times
// (including DTSTART) up to and including until.
func (r *Rule) naiveOccurrences(start WallClock, until *WallClock) []WallClock {
	return r.generate(start, until, true)
}

// generate builds the wall-clock starts up to and including until.
// includeStart forces DTSTART to be the first element, matching RFC
// 5545 for RRULE; EXRULE generation passes false because an occurrence
// is only excluded when the rule's BYxxx parts actually generate it.
func (r *Rule) generate(start WallClock, until *WallClock, includeStart bool) []WallClock {
	clockH, clockM, clockS := start.Hour, start.Minute, start.Second

	gen := r.periodGenerator(start)

	var out []WallClock
	for p := 0; p < maxPeriods; p++ {
		days := gen(p)
		if len(days) == 0 {
			continue
		}

		var picks []WallClock
		if len(r.BySetPos) > 0 {
			ordered := make([]WallClock, 0, len(days))
			for _, d := range days {
				ordered = append(ordered, d.withClock(clockH, clockM, clockS))
			}
			for _, pos := range r.BySetPos {
				idx := pos
				if idx < 0 {
					idx = len(ordered) + idx + 1
				}
				idx--
				if idx >= 0 && idx < len(ordered) {
					picks = append(picks, ordered[idx])
				}
			}
		} else {
			for _, d := range days {
				picks = append(picks, d.withClock(clockH, clockM, clockS))
			}
		}

		for _, w := range picks {
			if w.Before(start) {
				continue
			}
			if until != nil && until.Before(w) {
				return out
			}
			out = append(out, w)
		}
	}

	// RFC 5545: for RRULE DTSTART is always the first instance, even
	// when the BYxxx parts would not generate it, as long as UNTIL
	// permits. EXRULE keeps only what the rule itself generates.
	if includeStart && (until == nil || !until.Before(start)) {
		for i, w := range out {
			if w.Equal(start) {
				if i > 0 {
					copy(out[1:i+1], out[0:i])
					out[0] = start
				}
				return out
			}
		}
		out = append([]WallClock{start}, out...)
	}
	return out
}

// periodGenerator returns, for period ordinal n (0 based at DTSTART),
// the ordered list of candidate dates (midnight wall clocks) in it.
func (r *Rule) periodGenerator(start WallClock) func(n int) []WallClock {
	switch r.Freq {
	case DAILY:
		return func(n int) []WallClock {
			d := start.AddDate(0, 0, n*r.Interval)
			if len(r.ByDay) > 0 && !matchesWeekday(d, r.ByDay) {
				return nil
			}
			if !matchesMonths(d, r.ByMonth) {
				return nil
			}
			return []WallClock{d.midnight()}
		}
	case WEEKLY:
		return func(n int) []WallClock {
			periodStart := weekStart(start, r.WKST).AddDate(0, 0, n*7*r.Interval)
			days := r.ByDay
			if len(days) == 0 {
				days = []OrdWeekday{{Day: fromStd(start.asTime().Weekday())}}
			}
			var out []WallClock
			for i := 0; i < 7; i++ {
				d := periodStart.AddDate(0, 0, i)
				if d.Before(start.midnight()) {
					continue
				}
				if matchesWeekday(d, days) && matchesMonths(d, r.ByMonth) {
					out = append(out, d)
				}
			}
			return out
		}
	case MONTHLY:
		return func(n int) []WallClock {
			first := time.Date(start.Year, start.Month, 1, 0, 0, 0, 0, time.UTC).
				AddDate(0, n*r.Interval, 0)
			return monthCandidates(first.Year(), first.Month(), start, r)
		}
	default: // YEARLY
		return func(n int) []WallClock {
			base := time.Date(start.Year, 1, 1, 0, 0, 0, 0, time.UTC).
				AddDate(n*r.Interval, 0, 0)
			months := r.ByMonth
			if len(months) == 0 {
				months = []time.Month{start.Month}
			}
			var out []WallClock
			for _, m := range months {
				out = append(out, monthCandidates(base.Year(), m, start, r)...)
			}
			return out
		}
	}
}

func monthCandidates(year int, month time.Month, start WallClock, r *Rule) []WallClock {
	daysIn := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	var days []int
	for _, md := range r.ByMonthDay {
		d := md
		if d < 0 {
			d = daysIn + d + 1
		}
		if d >= 1 && d <= daysIn {
			days = append(days, d)
		}
	}
	if len(r.ByDay) > 0 {
		for d := 1; d <= daysIn; d++ {
			w := WallClock{Year: year, Month: month, Day: d}
			if ordinalDayMatches(w, r.ByDay) {
				days = append(days, d)
			}
		}
	}
	if len(r.ByMonthDay) == 0 && len(r.ByDay) == 0 {
		// start.Day may overflow the month (e.g. Jan 31 -> Feb); skip then.
		if start.Day > daysIn {
			days = nil
		} else {
			days = []int{start.Day}
		}
	}

	uniq := map[int]bool{}
	for i := 1; i < len(days); i++ {
		for j := i; j > 0 && days[j] < days[j-1]; j-- {
			days[j], days[j-1] = days[j-1], days[j]
		}
	}
	var out []WallClock
	for _, d := range days {
		if d < 1 || d > daysIn || uniq[d] {
			continue
		}
		uniq[d] = true
		w := WallClock{Year: year, Month: month, Day: d}
		if w.Before(start.midnight()) {
			continue
		}
		out = append(out, w)
	}
	return out
}

func ordinalDayMatches(w WallClock, by []OrdWeekday) bool {
	dow := fromStd(w.asTime().Weekday())
	daysIn := time.Date(w.Year, w.Month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	for _, ow := range by {
		if ow.Day != dow {
			continue
		}
		if ow.N == 0 {
			return true
		}
		n := ow.N
		if n > 0 && (w.Day-1)/7+1 == n {
			return true
		}
		if n < 0 && (daysIn-w.Day)/7+1 == -n {
			return true
		}
	}
	return false
}

func matchesWeekday(w WallClock, by []OrdWeekday) bool {
	dow := fromStd(w.asTime().Weekday())
	for _, ow := range by {
		if ow.Day == dow {
			return true
		}
	}
	return false
}

func matchesMonths(w WallClock, months []time.Month) bool {
	if len(months) == 0 {
		return true
	}
	for _, m := range months {
		if w.Month == m {
			return true
		}
	}
	return false
}

func weekStart(start WallClock, wkst Weekday) WallClock {
	mid := start.midnight()
	dow := int(fromStd(mid.asTime().Weekday()))
	ws := int(wkst)
	delta := (dow - ws + 7) % 7
	return mid.AddDate(0, 0, -delta)
}
