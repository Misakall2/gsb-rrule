package rrule

import "time"

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

func anyOverlap(a, b []Interval) bool {
	for _, x := range a {
		for _, y := range b {
			if x.Overlaps(y) {
				return true
			}
		}
	}
	return false
}

func firstOverlap(a, b []Interval) (Interval, Interval, bool) {
	for _, x := range a {
		for _, y := range b {
			if x.Overlaps(y) {
				return x, y, true
			}
		}
	}
	return Interval{}, Interval{}, false
}
