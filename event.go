package rrule

import "time"

// Event is an occupied span of a resource.
//
// Timed events carry a real location or UTC; their boundaries are absolute
// instants. AllDay events are calendar-day spans: Start is a date at
// midnight (its location is used when it is floating) and End is optional.
// When End is zero the event occupies the single day of Start; otherwise it
// occupies every date from Start through End inclusive.
//
// A floating timed event (location Floating) is anchored to an observer
// location before overlap comparison, so floating and absolute events share one
// comparison implementation.
type Event struct {
	Resource string
	Start    time.Time
	End      time.Time
	AllDay   bool
}

// Interval is a half-open absolute span [Start, End).
//
// Boundary semantics: one event ending exactly when another begins does NOT
// overlap (back-to-back meetings are allowed).
type Interval struct {
	Start time.Time
	End   time.Time
}

// NewTimedEvent builds a timed [start, end) event.
func NewTimedEvent(resource string, start, end time.Time) Event {
	return Event{Resource: resource, Start: start, End: end}
}

// NewAllDayEvent builds an all-day event. With no end it is one day;
// otherwise it spans from start through end inclusive (both are midnight
// values in the relevant location).
func NewAllDayEvent(resource string, start time.Time, end ...time.Time) Event {
	e := Event{Resource: resource, Start: start, AllDay: true}
	if len(end) > 0 {
		e.End = end[0]
	}
	return e
}

// Interval converts the event to a half-open absolute interval.
//
// observer anchors floating times. For all-day floating events it supplies the
// calendar whose midnights are used; it may be nil, in which case UTC is
// used.
func (e Event) Interval(observer *time.Location) Interval {
	if e.AllDay {
		return e.allDayInterval(observer)
	}
	start := anchorInstant(e.Start, observer)
	end := anchorInstant(e.End, observer)
	if !end.After(start) {
		end = start
	}
	return Interval{Start: start.UTC(), End: end.UTC()}
}

func (e Event) allDayInterval(observer *time.Location) Interval {
	loc := e.Start.Location()
	if loc == Floating {
		if observer != nil {
			loc = observer
		} else {
			loc = time.UTC
		}
	}
	y, m, d := e.Start.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	lastDay := start
	if !e.End.IsZero() {
		ey, em, ed := e.End.Date()
		lastDay = time.Date(ey, em, ed, 0, 0, 0, 0, loc)
	}
	if lastDay.Before(start) {
		lastDay = start
	}
	end := lastDay.AddDate(0, 0, 1)
	return Interval{Start: start.UTC(), End: end.UTC()}
}

func anchorInstant(t time.Time, observer *time.Location) time.Time {
	if t.Location() != Floating {
		return t
	}
	loc := observer
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := t.Date()
	return time.Date(y, m, d, t.Hour(), t.Minute(), t.Second(), 0, loc)
}

// Overlaps reports whether two events occupy the same resource at overlapping
// times. Different resources never overlap. observer anchors floating events.
//
// Intervals are half-open, so touching endpoints (e.g. 10:00 and 10:00)
// do not count as an overlap.
func Overlaps(a, b Event, observer *time.Location) bool {
	if a.Resource != b.Resource {
		return false
	}
	return IntervalsOverlap(a.Interval(observer), b.Interval(observer))
}

// IntervalsOverlap reports whether two half-open absolute intervals overlap.
func IntervalsOverlap(a, b Interval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}

// Conflicts returns every event in events that overlaps ev, grouped on the
// same resource rule.
func Conflicts(ev Event, events []Event, observer *time.Location) []Event {
	var out []Event
	for _, other := range events {
		if Overlaps(ev, other, observer) {
			out = append(out, other)
		}
	}
	return out
}
