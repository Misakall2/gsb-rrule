package rrule

import (
	"fmt"
	"time"
)

type scheduleClock struct {
	loc      *time.Location
	floating bool
}

func clockFor(t time.Time) scheduleClock {
	loc := t.Location()
	return scheduleClock{loc: loc, floating: isFloating(loc)}
}

func pinnedClock(loc *time.Location) scheduleClock {
	return scheduleClock{loc: loc}
}

func (c scheduleClock) occurrence(w WallClock, allDay bool) (Occurrence, bool) {
	occ := Occurrence{Wall: w, Floating: c.floating, AllDay: allDay}
	if c.floating {
		return occ, true
	}
	inst, ok := c.materialize(w)
	if !ok {
		return Occurrence{}, false
	}
	occ.Instant = inst
	return occ, true
}

func (c scheduleClock) exists(w WallClock) bool {
	if c.floating {
		return true
	}
	_, ok := c.materialize(w)
	return ok
}

func (c scheduleClock) untilWall(r *Rule) *WallClock {
	if r == nil || r.Until.IsZero() {
		return nil
	}
	if r.Until.Location() == time.UTC && c.loc != time.UTC && !c.floating {
		u := naiveOf(r.Until.In(c.loc))
		return &u
	}
	u := naiveOf(r.Until)
	return &u
}

func (c scheduleClock) windowTime(t time.Time, resource *time.Location) time.Time {
	if !c.floating {
		return t.In(c.loc)
	}
	loc := resource
	if loc == nil {
		loc = c.loc
	}
	it := t.In(loc)
	return time.Date(it.Year(), it.Month(), it.Day(), it.Hour(), it.Minute(), it.Second(), 0, Floating)
}

// materialize converts a wall clock into an instant in c.loc. The bool
// is false when the wall time does not exist (a DST spring gap).
// Ambiguous fall-back times resolve to the first (earlier) instant.
func (c scheduleClock) materialize(w WallClock) (time.Time, bool) {
	t := time.Date(w.Year, w.Month, w.Day, w.Hour, w.Minute, w.Second, 0, c.loc)
	// t is always a valid instant; detect gaps by re-reading the wall.
	rt := t.In(c.loc)
	if rt.Hour() != w.Hour || rt.Minute() != w.Minute || rt.Second() != w.Second ||
		rt.Day() != w.Day || rt.Month() != w.Month || rt.Year() != w.Year {
		return time.Time{}, false
	}
	return t, true
}

func occurrenceInterval(o Occurrence, dur time.Duration, resource *time.Location) (Interval, bool, error) {
	if o.AllDay {
		if o.Floating {
			if resource == nil {
				return Interval{}, false, fmt.Errorf("rrule: floating all-day occurrence needs a location")
			}
			start := time.Date(o.Wall.Year, o.Wall.Month, o.Wall.Day, 0, 0, 0, 0, resource)
			return Interval{Start: start.UTC(), End: start.AddDate(0, 0, 1).UTC()}, true, nil
		}
		loc := o.Instant.Location()
		start := time.Date(o.Wall.Year, o.Wall.Month, o.Wall.Day, 0, 0, 0, 0, loc)
		return Interval{Start: start.UTC(), End: start.AddDate(0, 0, 1).UTC()}, true, nil
	}

	var start time.Time
	if o.Floating {
		if resource == nil {
			return Interval{}, false, fmt.Errorf("rrule: floating occurrence needs a resource location")
		}
		inst, ok := pinnedClock(resource).materialize(o.Wall)
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
