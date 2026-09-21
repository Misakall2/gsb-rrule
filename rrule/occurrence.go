package rrule

import "time"

// This file is the timezone-free occurrence engine: it turns a Rule
// into an ordered list of wall-clock starts. No value here ever
// carries a real *time.Location; times are built in time.UTC purely as
// a calendar arithmetic container (see WallClock). DST gaps, COUNT
// consumption across gaps and window filtering are deliberately NOT
// handled here; see expand.go and zone.go.

// safety bound so a rule without COUNT/UNTIL cannot loop forever.
const maxPeriods = 200_000

// generateWalls generates the full ordered wall-clock start times up
// to and including until. When anchor is true (a main RRULE),
// DTSTART is always the first instance per RFC 5545 even when the
// BYxxx parts would not generate it. EXRULE expansion passes false:
// it produces only instances the rule itself generates, which makes
// its own COUNT count the removed instances rather than DTSTART.
func (r *Rule) generateWalls(start WallClock, until *WallClock, anchor bool) []WallClock {
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

	// Without BYSETPOS, DTSTART is always the first instance, even when
	// the BYxxx parts would not generate it, as long as UNTIL permits.
	// BYSETPOS explicitly selects from the rule-generated candidate set,
	// so it must not be bypassed by anchoring.
	if anchor && len(r.BySetPos) == 0 && (until == nil || !until.Before(start)) {
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
