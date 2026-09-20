package rrule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseRule parses an RRULE value (the optional "RRULE:" prefix is
// accepted). The supported subset is FREQ, INTERVAL, BYDAY,
// BYMONTHDAY, BYMONTH, BYSETPOS, COUNT, UNTIL and WKST.
//
// COUNT and UNTIL are mutually exclusive.
//
// UNTIL may be given as UTC ("20240310T050000Z") or as a floating
// local wall time ("20240310T010000"); the caller's schedule location
// determines how a floating UNTIL is interpreted during expansion.
func ParseRule(s string) (*Rule, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "RRULE:")
	r := &Rule{Interval: 1, WKST: MO}

	var haveFreq, haveCount, haveUntil, haveWKST bool
	seen := map[string]bool{}

	for _, part := range strings.Split(s, ";") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("rrule: malformed part %q", part)
		}
		key, val := strings.ToUpper(kv[0]), kv[1]
		if seen[key] {
			return nil, fmt.Errorf("rrule: duplicate %s", key)
		}
		seen[key] = true

		switch key {
		case "FREQ":
			switch strings.ToUpper(val) {
			case "DAILY":
				r.Freq = DAILY
			case "WEEKLY":
				r.Freq = WEEKLY
			case "MONTHLY":
				r.Freq = MONTHLY
			case "YEARLY":
				r.Freq = YEARLY
			default:
				return nil, fmt.Errorf("rrule: unsupported FREQ %q", val)
			}
			haveFreq = true
		case "INTERVAL":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("rrule: bad INTERVAL %q", val)
			}
			r.Interval = n
		case "COUNT":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("rrule: bad COUNT %q", val)
			}
			r.Count = n
			haveCount = true
		case "UNTIL":
			t, err := parseUntil(val)
			if err != nil {
				return nil, err
			}
			r.Until = t
			haveUntil = true
		case "BYDAY":
			for _, item := range strings.Split(val, ",") {
				ow, err := parseByDay(strings.TrimSpace(item))
				if err != nil {
					return nil, err
				}
				r.ByDay = append(r.ByDay, ow)
			}
		case "BYMONTHDAY":
			for _, item := range strings.Split(val, ",") {
				n, err := strconv.Atoi(strings.TrimSpace(item))
				if err != nil || n < -31 || n == 0 || n > 31 {
					return nil, fmt.Errorf("rrule: bad BYMONTHDAY %q", item)
				}
				r.ByMonthDay = append(r.ByMonthDay, n)
			}
		case "BYMONTH":
			for _, item := range strings.Split(val, ",") {
				n, err := strconv.Atoi(strings.TrimSpace(item))
				if err != nil || n < 1 || n > 12 {
					return nil, fmt.Errorf("rrule: bad BYMONTH %q", item)
				}
				r.ByMonth = append(r.ByMonth, time.Month(n))
			}
		case "BYSETPOS":
			for _, item := range strings.Split(val, ",") {
				n, err := strconv.Atoi(strings.TrimSpace(item))
				if err != nil || n == 0 {
					return nil, fmt.Errorf("rrule: bad BYSETPOS %q", item)
				}
				r.BySetPos = append(r.BySetPos, n)
			}
		case "WKST":
			w, err := parseWeekday(strings.TrimSpace(val))
			if err != nil {
				return nil, err
			}
			r.WKST = w
			haveWKST = true
		default:
			return nil, fmt.Errorf("rrule: unsupported key %q", key)
		}
	}

	if !haveFreq {
		return nil, fmt.Errorf("rrule: FREQ is required")
	}
	if haveCount && haveUntil {
		return nil, fmt.Errorf("rrule: COUNT and UNTIL are mutually exclusive")
	}
	_ = haveWKST

	if r.Freq == DAILY || r.Freq == WEEKLY {
		for _, ow := range r.ByDay {
			if ow.N != 0 {
				return nil, fmt.Errorf("rrule: ordinal BYDAY %s requires MONTHLY or YEARLY", ow)
			}
		}
	}
	if r.Freq == DAILY && len(r.ByMonthDay) > 0 {
		return nil, fmt.Errorf("rrule: BYMONTHDAY requires MONTHLY or YEARLY")
	}
	return r, nil
}

func parseWeekday(s string) (Weekday, error) {
	switch strings.ToUpper(s) {
	case "MO":
		return MO, nil
	case "TU":
		return TU, nil
	case "WE":
		return WE, nil
	case "TH":
		return TH, nil
	case "FR":
		return FR, nil
	case "SA":
		return SA, nil
	case "SU":
		return SU, nil
	}
	return 0, fmt.Errorf("rrule: bad weekday %q", s)
}

func parseByDay(s string) (OrdWeekday, error) {
	i := 0
	n := 0
	neg := false
	if strings.HasPrefix(s, "+") {
		i++
	}
	if strings.HasPrefix(s, "-") {
		neg = true
		i++
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	if neg {
		n = -n
	}
	w, err := parseWeekday(s[i:])
	if err != nil {
		return OrdWeekday{}, fmt.Errorf("rrule: bad BYDAY %q", s)
	}
	return OrdWeekday{N: n, Day: w}, nil
}

func parseUntil(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	switch len(s) {
	case 8: // YYYYMMDD, floating date
		if t, err := time.ParseInLocation("20060102", s, time.UTC); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, Floating), nil
		}
	case 16: // YYYYMMDDTHHMMSSZ
		if t, err := time.Parse("20060102T150405Z", s); err == nil {
			return t, nil
		}
	case 15: // YYYYMMDDTHHMMSS, floating local wall time
		if t, err := time.Parse("20060102T150405", s); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, Floating), nil
		}
	}
	return time.Time{}, fmt.Errorf("rrule: bad UNTIL %q", s)
}
