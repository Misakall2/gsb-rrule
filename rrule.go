package rrule

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Frequency is the FREQ part of an RRULE.
type Frequency int

const (
	Daily Frequency = iota
	Weekly
	Monthly
	Yearly
)

func (f Frequency) String() string {
	switch f {
	case Daily:
		return "DAILY"
	case Weekly:
		return "WEEKLY"
	case Monthly:
		return "MONTHLY"
	case Yearly:
		return "YEARLY"
	}
	return "UNKNOWN"
}

// Weekday is a day of the week, matching time.Weekday numbering (Sunday = 0).
type Weekday struct {
	Day time.Weekday
	// Ord is the BYDAY ordinal qualifier, e.g. 2 for "2MO", -1 for "-1FR".
	// Zero means "every MO".
	Ord int
}

// AmbiguousTime selects which instant a wall-clock time maps to when clocks
// fall back and the wall time happens twice.
type AmbiguousTime int

const (
	// FirstAmbiguous keeps the earlier instant (summer offset). The rule then
	// emits one occurrence, never two.
	FirstAmbiguous AmbiguousTime = iota
	// SecondAmbiguous keeps the later instant (winter offset).
	SecondAmbiguous
)

// Rule is a parsed RFC 5545 RRULE in the supported subset.
//
// Supported: FREQ (DAILY/WEEKLY/MONTHLY/YEARLY), INTERVAL, BYDAY,
// BYMONTHDAY, BYSETPOS, COUNT, UNTIL, WKST.
type Rule struct {
	Frequency Frequency
	Interval  int
	ByDay     []Weekday
	// ByMonthDay is 1..31 from the start or negative (-1 = last day).
	ByMonthDay []int
	BySetPos   []int
	Count      int // 0 means no COUNT
	Until      time.Time
	HasUntil   bool
	Wkst       time.Weekday // week start; default Monday

	// Ambiguous controls fall-back repeats. The zero value (FirstAmbiguous)
	// emits a single occurrence.
	Ambiguous AmbiguousTime
}

var (
	errBadRule = errors.New("rrule: invalid rule")
)

// ParseRule parses an RRULE value such as
// "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE;COUNT=10".
//
// UNTIL, if present, may be a UTC DATE-TIME (Z suffix) or a DATE. A DATE or
// naive DATE-TIME is interpreted in untilLoc, which may be nil for floating
// comparison.
func ParseRule(s string, untilLoc *time.Location) (*Rule, error) {
	r := &Rule{Interval: 1, Wkst: time.Monday}
	seenFreq := false
	var byday, bymonthday, bysetpos string
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("%w: %q", errBadRule, part)
		}
		key := strings.ToUpper(strings.TrimSpace(kv[0]))
		val := strings.TrimSpace(kv[1])
		switch key {
		case "FREQ":
			switch strings.ToUpper(val) {
			case "DAILY":
				r.Frequency = Daily
			case "WEEKLY":
				r.Frequency = Weekly
			case "MONTHLY":
				r.Frequency = Monthly
			case "YEARLY":
				r.Frequency = Yearly
			default:
				return nil, fmt.Errorf("%w: unsupported FREQ %q", errBadRule, val)
			}
			seenFreq = true
		case "INTERVAL":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("%w: INTERVAL must be a positive integer", errBadRule)
			}
			r.Interval = n
		case "COUNT":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("%w: COUNT must be a positive integer", errBadRule)
			}
			r.Count = n
		case "UNTIL":
			t, err := parseUntil(val, untilLoc)
			if err != nil {
				return nil, err
			}
			r.Until = t
			r.HasUntil = true
		case "BYDAY":
			byday = val
		case "BYMONTHDAY":
			bymonthday = val
		case "BYSETPOS":
			bysetpos = val
		case "WKST":
			switch strings.ToUpper(val) {
			case "MO":
				r.Wkst = time.Monday
			case "TU":
				r.Wkst = time.Tuesday
			case "WE":
				r.Wkst = time.Wednesday
			case "TH":
				r.Wkst = time.Thursday
			case "FR":
				r.Wkst = time.Friday
			case "SA":
				r.Wkst = time.Saturday
			case "SU":
				r.Wkst = time.Sunday
			default:
				return nil, fmt.Errorf("%w: WKST %q", errBadRule, val)
			}
		default:
			return nil, fmt.Errorf("%w: unsupported part %q", errBadRule, key)
		}
	}
	if !seenFreq {
		return nil, fmt.Errorf("%w: missing FREQ", errBadRule)
	}
	if r.Count > 0 && r.HasUntil {
		return nil, fmt.Errorf("%w: COUNT and UNTIL are mutually exclusive", errBadRule)
	}
	if byday != "" {
		days, err := parseByDay(byday)
		if err != nil {
			return nil, err
		}
		r.ByDay = days
	}
	if bymonthday != "" {
		days, err := parseIntList(bymonthday)
		if err != nil {
			return nil, fmt.Errorf("%w: bad BYMONTHDAY", errBadRule)
		}
		for _, d := range days {
			if d < -31 || d == 0 || d > 31 {
				return nil, fmt.Errorf("%w: BYMONTHDAY out of range: %d", errBadRule, d)
			}
		}
		r.ByMonthDay = days
	}
	if bysetpos != "" {
		pos, err := parseIntList(bysetpos)
		if err != nil {
			return nil, fmt.Errorf("%w: bad BYSETPOS", errBadRule)
		}
		r.BySetPos = pos
	}
	return r, nil
}

var weekdayToken = map[string]time.Weekday{
	"SU": time.Sunday,
	"MO": time.Monday,
	"TU": time.Tuesday,
	"WE": time.Wednesday,
	"TH": time.Thursday,
	"FR": time.Friday,
	"SA": time.Saturday,
}

func parseByDay(s string) ([]Weekday, error) {
	var out []Weekday
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if len(tok) < 2 {
			return nil, fmt.Errorf("%w: bad BYDAY %q", errBadRule, tok)
		}
		dayTok := tok[len(tok)-2:]
		day, ok := weekdayToken[strings.ToUpper(dayTok)]
		if !ok {
			return nil, fmt.Errorf("%w: bad BYDAY %q", errBadRule, tok)
		}
		ord := 0
		if len(tok) > 2 {
			n, err := strconv.Atoi(tok[:len(tok)-2])
			if err != nil || n == 0 {
				return nil, fmt.Errorf("%w: bad BYDAY ordinal %q", errBadRule, tok)
			}
			ord = n
		}
		out = append(out, Weekday{Day: day, Ord: ord})
	}
	return out, nil
}

func parseIntList(s string) ([]int, error) {
	var out []int
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		n, err := strconv.Atoi(tok)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func parseUntil(s string, loc *time.Location) (time.Time, error) {
	t, err := ParseDateIn(s, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: bad UNTIL: %v", errBadRule, err)
	}
	return t, nil
}

// String renders the rule back in RRULE form, useful for tests and logging.
func (r *Rule) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "FREQ=%s", r.Frequency)
	if r.Interval != 1 {
		fmt.Fprintf(&b, ";INTERVAL=%d", r.Interval)
	}
	if len(r.ByDay) > 0 {
		toks := make([]string, len(r.ByDay))
		for i, d := range r.ByDay {
			if d.Ord != 0 {
				toks[i] = fmt.Sprintf("%d%s", d.Ord, dayToken(d.Day))
			} else {
				toks[i] = dayToken(d.Day)
			}
		}
		fmt.Fprintf(&b, ";BYDAY=%s", strings.Join(toks, ","))
	}
	if len(r.ByMonthDay) > 0 {
		toks := make([]string, len(r.ByMonthDay))
		for i, d := range r.ByMonthDay {
			toks[i] = strconv.Itoa(d)
		}
		fmt.Fprintf(&b, ";BYMONTHDAY=%s", strings.Join(toks, ","))
	}
	if len(r.BySetPos) > 0 {
		toks := make([]string, len(r.BySetPos))
		for i, p := range r.BySetPos {
			toks[i] = strconv.Itoa(p)
		}
		fmt.Fprintf(&b, ";BYSETPOS=%s", strings.Join(toks, ","))
	}
	if r.Count > 0 {
		fmt.Fprintf(&b, ";COUNT=%d", r.Count)
	}
	if r.HasUntil {
		fmt.Fprintf(&b, ";UNTIL=%s", r.Until.UTC().Format("20060102T150405Z"))
	}
	if r.Wkst != time.Monday {
		fmt.Fprintf(&b, ";WKST=%s", dayToken(r.Wkst))
	}
	return b.String()
}

func dayToken(d time.Weekday) string {
	for tok, wd := range weekdayToken {
		if wd == d {
			return tok
		}
	}
	return ""
}
