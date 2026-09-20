package rrule

import (
	"fmt"
	"time"
)

// Floating is a sentinel location for values without a time zone.
//
// A floating time describes a wall-clock time (for example 09:00 local) that
// is not tied to any offset. When it has to be compared with absolute
// instants, it is anchored in an observer location.
var Floating = time.FixedZone("FLOATING", 0)

// IsFloating reports whether t carries no time zone information.
func IsFloating(t time.Time) bool {
	return t.Location() == Floating
}

var dateLayouts = []string{"20060102", "20060102T150405", "20060102T150405Z"}

// ParseDate parses an RFC 5545 DATE (YYYYMMDD) or DATE-TIME.
//
// DATE-TIME values ending in Z are interpreted as UTC. Everything else is
// floating: use ParseDateIn to parse it against a TZID location instead.
func ParseDate(s string) (time.Time, error) {
	return parseDate(s, nil)
}

// ParseDateIn parses an RFC 5545 DATE or DATE-TIME against loc.
//
// A UTC (Z suffixed) value is returned in UTC and loc is ignored. A plain DATE
// is returned as a midnight value in loc. A naive DATE-TIME is interpreted in
// loc; pass Floating to keep it time-zone-less.
func ParseDateIn(s string, loc *time.Location) (time.Time, error) {
	return parseDate(s, loc)
}

func parseDate(s string, loc *time.Location) (time.Time, error) {
	switch len(s) {
	case len("20060102"):
		t, err := time.Parse("20060102", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("rrule: bad date %q: %w", s, err)
		}
		if loc == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Floating), nil
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc), nil
	default:
		if len(s) != len("20060102T150405") && len(s) != len("20060102T150405Z") {
			return time.Time{}, fmt.Errorf("rrule: unrecognized date %q", s)
		}
		if s[len(s)-1] == 'Z' {
			t, err := time.Parse("20060102T150405Z", s)
			if err != nil {
				return time.Time{}, fmt.Errorf("rrule: bad date-time %q: %w", s, err)
			}
			return t, nil
		}
		t, err := time.Parse("20060102T150405", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("rrule: bad date-time %q: %w", s, err)
		}
		if loc == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, Floating), nil
		}
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc), nil
	}
}
