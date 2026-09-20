// Package rrule implements a small RFC 5545 style recurrence subset:
// FREQ (DAILY/WEEKLY/MONTHLY/YEARLY), INTERVAL, BYDAY, BYMONTHDAY,
// BYMONTH, BYSETPOS, COUNT/UNTIL, EXDATE and RDATE, together with
// timezone handling (TZID, UTC and floating wall-clock times) and
// resource overlap detection.
package rrule

import (
	"strconv"
	"time"
)

// Freq is the recurrence frequency.
type Freq int

const (
	DAILY Freq = iota
	WEEKLY
	MONTHLY
	YEARLY
)

func (f Freq) String() string {
	switch f {
	case DAILY:
		return "DAILY"
	case WEEKLY:
		return "WEEKLY"
	case MONTHLY:
		return "MONTHLY"
	case YEARLY:
		return "YEARLY"
	}
	return "UNKNOWN"
}

// Weekday follows the RFC 5545 numbering: MO=1 ... SU=7.
type Weekday int

const (
	MO Weekday = 1
	TU Weekday = 2
	WE Weekday = 3
	TH Weekday = 4
	FR Weekday = 5
	SA Weekday = 6
	SU Weekday = 7
)

// OrdWeekday is a BYDAY element. N is the optional ordinal within the
// period (e.g. 2 in 2MO, -1 in -1FR); 0 means no ordinal.
type OrdWeekday struct {
	N   int
	Day Weekday
}

func (o OrdWeekday) String() string {
	if o.N == 0 {
		return o.Day.String()
	}
	return strconv.Itoa(o.N) + o.Day.String()
}

// Std returns the matching time.Weekday.
func (w Weekday) Std() time.Weekday {
	if w == SU {
		return time.Sunday
	}
	return time.Weekday(w)
}

func (w Weekday) String() string {
	return [...]string{"", "MO", "TU", "WE", "TH", "FR", "SA", "SU"}[w]
}

func fromStd(d time.Weekday) Weekday {
	if d == time.Sunday {
		return SU
	}
	return Weekday(d)
}

// Floating is the sentinel location for timezone-less "floating"
// times: the wall clock is kept as written and only gets an instant
// when pinned to a resource zone. It is a zero-offset location with an
// empty name, distinguishable from time.UTC by identity.
var Floating = time.FixedZone("", 0)

func isFloating(loc *time.Location) bool { return loc == Floating }

// Rule is the parsed RRULE. Location carries the TZID semantics:
// time.UTC means a UTC rule, time.Local or any other loaded location a
// zoned rule, and nil a floating (timezone-less) rule.
type Rule struct {
	Freq       Freq
	Interval   int
	ByDay      []OrdWeekday
	ByMonthDay []int // negative values count from the end of the month
	ByMonth    []time.Month
	BySetPos   []int // negative values count from the end of the candidate set
	Count      int   // 0 means unset
	Until      time.Time
	WKST       Weekday
}

// Schedule ties DTSTART, RRULE, EXDATE and RDATE together.
//
// Every time.Time carries the schedule's timezone convention:
// time.UTC for UTC schedules, a loaded *time.Location for TZID
// schedules, and the Floating sentinel for floating schedules.
// ExDate/RDate wall-clock fields are matched directly, so
// callers may pass them in any location for zoned schedules.
type Schedule struct {
	DTStart  time.Time
	Rule     *Rule // nil means a one-shot event
	ExDate   []time.Time
	RDate    []time.Time
	AllDay   bool          // date-only DTSTART; each occurrence occupies a whole day
	Duration time.Duration // duration of a timed occurrence
}

// Occurrence is a single materialized instance.
//
// Floating schedules leave Instant unset (Instant.IsZero()) and only
// carry Wall, meaning "this local wall-clock time in whatever zone the
// observer uses". Zoned and UTC schedules always set Instant.
type Occurrence struct {
	Wall     WallClock
	Instant  time.Time
	Floating bool
	AllDay   bool
}

// WallClock is a location-independent calendar date and clock time.
type WallClock struct {
	Year   int
	Month  time.Month
	Day    int
	Hour   int
	Minute int
	Second int
}

func naiveOf(t time.Time) WallClock {
	return WallClock{
		Year: t.Year(), Month: t.Month(), Day: t.Day(),
		Hour: t.Hour(), Minute: t.Minute(), Second: t.Second(),
	}
}

func (w WallClock) asTime() time.Time {
	return time.Date(w.Year, w.Month, w.Day, w.Hour, w.Minute, w.Second, 0, time.UTC)
}

// Before reports whether w is earlier than v in wall-clock order.
func (w WallClock) Before(v WallClock) bool { return w.asTime().Before(v.asTime()) }

// Equal reports wall-clock equality.
func (w WallClock) Equal(v WallClock) bool { return w.asTime().Equal(v.asTime()) }

// AddDate returns the wall clock shifted by the given date parts,
// keeping the clock time.
func (w WallClock) AddDate(years, months, days int) WallClock {
	t := time.Date(w.Year, w.Month, w.Day, w.Hour, w.Minute, w.Second, 0, time.UTC).
		AddDate(years, months, days)
	return naiveOf(t)
}

func (w WallClock) midnight() WallClock {
	return WallClock{Year: w.Year, Month: w.Month, Day: w.Day}
}

func (w WallClock) withClock(h, mi, s int) WallClock {
	w.Hour, w.Minute, w.Second = h, mi, s
	return w
}
