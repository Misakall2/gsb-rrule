package rrule

import "time"

// instKey is a canonical identity for an occurrence. Absolute (zoned or UTC)
// times compare by instant; floating times compare by wall clock.
func instKey(t time.Time) string {
	if t.Location() == Floating {
		return "F " + t.Format("20060102T150405")
	}
	return "U " + time.Unix(0, t.UnixNano()).UTC().Format("20060102T150405.000000000")
}

// modesDiffer reports whether one value is floating and the other absolute.
func modesDiffer(a, b time.Time) bool {
	return (a.Location() == Floating) != (b.Location() == Floating)
}

func beforeTime(a, b time.Time) bool {
	if a.Location() == Floating || b.Location() == Floating {
		return wallBefore(a, b)
	}
	return a.Before(b)
}

func afterTime(a, b time.Time) bool {
	if a.Location() == Floating || b.Location() == Floating {
		return wallBefore(b, a)
	}
	return a.After(b)
}

func wallBefore(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	if ay != by {
		return ay < by
	}
	if am != bm {
		return am < bm
	}
	if ad != bd {
		return ad < bd
	}
	ah := a.Hour()*3600 + a.Minute()*60 + a.Second()
	bh := b.Hour()*3600 + b.Minute()*60 + b.Second()
	return ah < bh
}

// isExcluded reports whether t matches an EXDATE. Matching is by instant
// for absolute rules and by wall clock for floating rules.
func isExcluded(exdates []time.Time, t time.Time) bool {
	key := instKey(t)
	for _, ex := range exdates {
		if instKey(ex) == key {
			return true
		}
	}
	return false
}
