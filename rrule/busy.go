package rrule

import (
	"fmt"
	"sort"
	"time"
)

// BusyIntervals expands every schedule sharing one resource over the
// closed occurrence-start window [from, to] and merges the
// occupied spans into busy blocks on the UTC time line. Spans that
// overlap or touch (one ends exactly when another begins) become a
// single block, so the result is ordered, disjoint and non-adjacent.
//
// from and to are absolute UTC instants defining the clipping window.
// loc pins floating schedules to the resource zone (exactly as in
// Intervals/Conflicts) and is required if any schedule floats.
// Zoned/UTC schedules keep their own TZIDs; all-day blocks occupy
// midnight-to-midnight in their own frame, matching the overlap rules.
//
// An occurrence starting before the window but still busy inside it
// (long Duration or an all-day block) is included, which is why the
// internal expansion is widened backwards.
func BusyIntervals(schedules []*Schedule, from, to time.Time, loc *time.Location) ([]Interval, error) {
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("rrule: busy intervals require a finite [from, to] window")
	}
	if from.After(to) {
		return nil, fmt.Errorf("rrule: from is after to")
	}

	var spans []Interval
	from2 := from.AddDate(-1, 0, 0)
	for _, s := range schedules {
		ivs, err := scheduleIntervalsIn(s, from2, to, loc)
		if err != nil {
			return nil, err
		}
		spans = append(spans, ivs...)
	}

	return mergeIntervals(spans, from, to), nil
}

// scheduleIntervalsIn adapts the absolute UTC window to the schedule's
// own convention before expanding: wall-clock fields for floating
// schedules, the same instant re-expressed in the schedule zone for
// zoned ones. Floating schedules additionally need loc here.
func scheduleIntervalsIn(s *Schedule, fromUTC, toUTC time.Time, loc *time.Location) ([]Interval, error) {
	if s == nil {
		return nil, fmt.Errorf("rrule: nil schedule")
	}
	floating := isFloating(s.DTStart.Location())
	if floating && loc == nil {
		return nil, fmt.Errorf("rrule: floating schedule needs a resource location")
	}

	var fw, tw time.Time
	if floating {
		fw = wallTimeIn(fromUTC, loc)
		tw = wallTimeIn(toUTC, loc)
	} else {
		zone := s.DTStart.Location()
		fw = fromUTC.In(zone)
		tw = toUTC.In(zone)
	}
	return Intervals(s, fw, tw, loc)
}

func wallTimeIn(t time.Time, loc *time.Location) time.Time {
	it := t.In(loc)
	return time.Date(it.Year(), it.Month(), it.Day(), it.Hour(), it.Minute(), it.Second(), 0, Floating)
}

// mergeIntervals sorts the spans, merges overlaps and touches, then
// clips the resulting blocks to the closed occurrence-start window.
// Zero-duration spans occupy no time and never merge.
func mergeIntervals(spans []Interval, from, to time.Time) []Interval {
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start.Equal(spans[j].Start) {
			return spans[i].End.Before(spans[j].End)
		}
		return spans[i].Start.Before(spans[j].Start)
	})

	var merged []Interval
	for _, sp := range spans {
		if !sp.End.After(sp.Start) {
			continue
		}
		if n := len(merged); n > 0 && !sp.Start.After(merged[n-1].End) {
			if sp.End.After(merged[n-1].End) {
				merged[n-1].End = sp.End
			}
			continue
		}
		merged = append(merged, sp)
	}

	out := make([]Interval, 0, len(merged))
	for _, m := range merged {
		if !m.End.After(from) || m.Start.After(to) {
			continue
		}
		if m.Start.Before(from) {
			m.Start = from
		}
		if m.End.After(to) {
			m.End = to
		}
		out = append(out, m)
	}
	return out
}
