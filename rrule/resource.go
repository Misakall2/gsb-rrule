package rrule

import (
	"fmt"
	"sort"
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

// touches reports whether j starts at or before i ends. Two spans that
// merely meet (i.End == j.Start) touch but do not overlap; busy-time
// merging treats them as one continuous block.
func (i Interval) touches(j Interval) bool {
	return !j.Start.After(i.End)
}

// MergeBusy unions overlapping or adjacent intervals into ordered,
// disjoint busy blocks. Adjacent means one block ends exactly when the
// next begins; they become a single block even though Overlaps is
// false for them, matching the half-open boundary convention. Zero
// duration intervals carry no occupancy and are dropped.
func MergeBusy(ivs []Interval) []Interval {
	if len(ivs) == 0 {
		return nil
	}
	sorted := make([]Interval, 0, len(ivs))
	for _, iv := range ivs {
		if iv.End.After(iv.Start) {
			sorted = append(sorted, iv)
		}
	}
	if len(sorted) == 0 {
		return nil
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Start.Equal(sorted[j].Start) {
			return sorted[i].End.Before(sorted[j].End)
		}
		return sorted[i].Start.Before(sorted[j].Start)
	})
	merged := []Interval{sorted[0]}
	for _, iv := range sorted[1:] {
		cur := &merged[len(merged)-1]
		if cur.touches(iv) {
			if iv.End.After(cur.End) {
				cur.End = iv.End
			}
			continue
		}
		merged = append(merged, iv)
	}
	return merged
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

// Booking is one schedule placed on a resource. Loc pins a floating
// schedule's wall clocks to the resource zone and is ignored for
// zoned or UTC schedules.
type Booking struct {
	Schedule *Schedule
	Loc      *time.Location
}

// BusyIntervals expands every booking on one resource over the closed
// occurrence window [from, to] and returns the merged busy time on the
// UTC time line: overlapping blocks are united, and blocks that merely
// meet end-to-start are also united (see MergeBusy). The result is
// ordered, disjoint and clipped to [from, to).
//
// Occurrences starting up to a year before from are considered so that
// an event already in progress at the start of the window still marks
// the room busy. All-day blocks, timed events and floating events
// pinned via Booking.Loc compare on equal footing, using the same
// half-open boundary rules as Conflicts.
func BusyIntervals(bookings []Booking, from, to time.Time) ([]Interval, error) {
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("rrule: busy intervals require a finite [from, to] window")
	}
	if from.After(to) {
		return nil, fmt.Errorf("rrule: from is after to")
	}
	from2 := from.AddDate(-1, 0, 0)
	var all []Interval
	for _, b := range bookings {
		if b.Schedule == nil {
			continue
		}
		ivs, err := Intervals(b.Schedule, from2, to, b.Loc)
		if err != nil {
			return nil, err
		}
		for _, iv := range ivs {
			all = append(all, clipInterval(iv, from, to))
		}
	}
	return MergeBusy(all), nil
}

// clipInterval restricts iv to the half-open window [from, to). A
// span ending at or before from, or starting at or after to, becomes
// a zero interval which MergeBusy drops.
func clipInterval(iv Interval, from, to time.Time) Interval {
	if iv.Start.Before(from) {
		iv.Start = from
	}
	if iv.End.After(to) {
		iv.End = to
	}
	return iv
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
