package rrule

import (
	"fmt"
	"time"
)

// This file is the single place that interprets the timezone
// convention of a Schedule: UTC, a loaded TZID zone, or the Floating
// sentinel. Wall-clock expansion never touches *time.Location; every
// conversion between a WallClock and an absolute instant happens here.

// Floating is the sentinel location for timezone-less "floating"
// times: the wall clock is kept as written and only gets an instant
// when pinned to a resource zone. It is a zero-offset location with an
// empty name, distinguishable from time.UTC by identity.
var Floating = time.FixedZone("", 0)

func isFloating(loc *time.Location) bool { return loc == Floating }

// materialize converts a wall clock into an instant in loc. The bool
// is false when the wall time does not exist (a DST spring gap).
// Ambiguous fall-back times resolve to the first (earlier) instant.
// It never mutates global state: time.Date does the zone lookup.
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

// occurrenceAt pins a generated wall clock to the timeline for a
// schedule living in loc. Floating schedules carry no instant. A
// nonexistent wall time in a zoned schedule (spring-forward gap)
// returns ok=false so the caller skips that occurrence without
// consuming COUNT.
func occurrenceAt(w WallClock, loc *time.Location, floating, allDay bool) (Occurrence, bool) {
	o := Occurrence{Wall: w, Floating: floating, AllDay: allDay}
	if floating {
		return o, true
	}
	inst, ok := materialize(w, loc)
	if !ok {
		return Occurrence{}, false
	}
	o.Instant = inst
	return o, true
}

// ruleUntil interprets an RRULE UNTIL against the schedule zone.
// A UTC UNTIL ("...Z") on a zoned schedule is compared on the wall
// clock of that zone; a floating UNTIL keeps its written wall clock;
// nil means UNTIL is unset.
func ruleUntil(r *Rule, loc *time.Location) *WallClock {
	if r == nil || r.Until.IsZero() {
		return nil
	}
	if r.Until.Location() == time.UTC && loc != time.UTC && !isFloating(loc) {
		u := naiveOf(r.Until.In(loc))
		return &u
	}
	u := naiveOf(r.Until)
	return &u
}

// windowInSchedule re-expresses an absolute UTC window in the
// schedule's own convention: wall-clock fields tagged Floating for
// floating schedules (resourceLoc is the zone the wall fields are read
// in and must be non-nil), or the same instant in the schedule zone
// for zoned/UTC ones.
func windowInSchedule(scheduleLoc *time.Location, fromUTC, toUTC time.Time, resourceLoc *time.Location) (time.Time, time.Time) {
	if isFloating(scheduleLoc) {
		return wallTimeIn(fromUTC, resourceLoc), wallTimeIn(toUTC, resourceLoc)
	}
	return fromUTC.In(scheduleLoc), toUTC.In(scheduleLoc)
}

func wallTimeIn(t time.Time, loc *time.Location) time.Time {
	it := t.In(loc)
	return time.Date(it.Year(), it.Month(), it.Day(), it.Hour(), it.Minute(), it.Second(), 0, Floating)
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
