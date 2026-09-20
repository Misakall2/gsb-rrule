package rrule

import (
	"fmt"
	"time"
)

// Interval is a half-open [Start, End) span on the UTC time line.
// An event ending exactly when another begins does not overlap.
type Interval struct {
	Start time.Time
	End   time.Time
}

// Overlaps reports whether two half-open intervals share any time.
func (i Interval) Overlaps(j Interval) bool {
	return i.Start.Before(j.End) && j.Start.Before(i.End)
}

// OccurrenceInterval converts a single occurrence to a UTC instant
// interval.
//
// Floating occurrences have no timezone of their own, so loc is the
// resource location used to pin their wall clock to the timeline (for
// example the meeting room's zone). It is required for floating
// occurrences and ignored otherwise.
//
// An all-day occurrence occupies the whole calendar day in its own
// frame: midnight to next midnight, and dur is ignored. A timed
// occurrence uses dur; a zero-duration occurrence occupies no time and
// never overlaps anything.
func OccurrenceInterval(o Occurrence, dur time.Duration, loc *time.Location) (Interval, bool, error) {
	if o.AllDay {
		if o.Floating {
			if loc == nil {
				return Interval{}, false, fmt.Errorf("rrule: floating all-day occurrence needs a location")
			}
			start := time.Date(o.Wall.Year, o.Wall.Month, o.Wall.Day, 0, 0, 0, 0, loc)
			return Interval{Start: start.UTC(), End: start.AddDate(0, 0, 1).UTC()}, true, nil
		}
		loc2 := o.Instant.Location()
		start := time.Date(o.Wall.Year, o.Wall.Month, o.Wall.Day, 0, 0, 0, 0, loc2)
		return Interval{Start: start.UTC(), End: start.AddDate(0, 0, 1).UTC()}, true, nil
	}

	var start time.Time
	if o.Floating {
		if loc == nil {
			return Interval{}, false, fmt.Errorf("rrule: floating occurrence needs a resource location")
		}
		inst, ok := materialize(o.Wall, loc)
		if !ok {
			// The floating wall time does not exist in the resource
			// zone (spring gap): the booking cannot occur there.
			return Interval{}, false, nil
		}
		start = inst
	} else {
		start = o.Instant
	}
	if dur <= 0 {
		return Interval{Start: start.UTC(), End: start.UTC()}, true, nil
	}
	return Interval{Start: start.UTC(), End: start.Add(dur).UTC()}, true, nil
}

// Intervals expands s over the closed window [from, to] and converts
// every occurrence into a half-open UTC interval. loc is required when
// s is floating (it pins wall clocks to the resource zone) and ignored
// otherwise. Occurrences whose wall time does not exist in that zone
// (spring-forward gaps) are skipped.
func Intervals(s *Schedule, from, to time.Time, loc *time.Location) ([]Interval, error) {
	occs, err := Expand(s, from, to)
	if err != nil {
		return nil, err
	}
	var out []Interval
	for _, o := range occs {
		iv, ok, err := OccurrenceInterval(o, s.Duration, loc)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, iv)
		}
	}
	return out, nil
}

// Conflicts reports whether a and b occupy the same resource at the
// same time within the closed window [from, to].
//
// locA/locB pin floating schedules to their resource zones and are
// ignored for zoned/UTC schedules. All comparisons happen on the UTC
// time line using half-open intervals, so back-to-back bookings do not
// conflict.
func Conflicts(a, b *Schedule, from, to time.Time, locA, locB *time.Location) (bool, error) {
	// Widen the window backwards by a safe margin so that an occurrence
	// starting before the window but running into it is still checked.
	from2 := from.AddDate(-1, 0, 0)
	ia, err := Intervals(a, from2, to, locA)
	if err != nil {
		return false, err
	}
	ib, err := Intervals(b, from2, to, locB)
	if err != nil {
		return false, err
	}
	for _, x := range ia {
		for _, y := range ib {
			if x.Overlaps(y) {
				return true, nil
			}
		}
	}
	return false, nil
}

// FirstConflict returns the first pair of overlapping intervals, if any.
func FirstConflict(a, b *Schedule, from, to time.Time, locA, locB *time.Location) (Interval, Interval, bool, error) {
	from2 := from.AddDate(-1, 0, 0)
	ia, err := Intervals(a, from2, to, locA)
	if err != nil {
		return Interval{}, Interval{}, false, err
	}
	ib, err := Intervals(b, from2, to, locB)
	if err != nil {
		return Interval{}, Interval{}, false, err
	}
	for _, x := range ia {
		for _, y := range ib {
			if x.Overlaps(y) {
				return x, y, true, nil
			}
		}
	}
	return Interval{}, Interval{}, false, nil
}
