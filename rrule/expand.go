package rrule

import (
	"fmt"
	"time"
)

// Expand returns the occurrences of s whose wall-clock start lies in
// the closed window [from, to]. The window must be non-zero and
// from <= to.
//
// The window times use the same convention as s.DTStart: UTC times for
// UTC/floating scheduling of the window for zoned schedules; for
// floating schedules the wall-clock fields are what matters. The
// returned slice is ordered by start time.
//
// DST behaviour for zoned schedules:
//
//   - On a spring-forward, a wall time that does not exist causes that
//     occurrence to be skipped (e.g. 02:30 when 02:00 jumps to 03:00).
//   - On a fall-back, an ambiguous wall time resolves to its first
//     (pre-transition) instant; a single rule instance never expands
//     twice.
func Expand(s *Schedule, from, to time.Time) ([]Occurrence, error) {
	if s == nil {
		return nil, fmt.Errorf("rrule: nil schedule")
	}
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("rrule: expansion requires a finite [from, to] window")
	}
	if from.After(to) {
		return nil, fmt.Errorf("rrule: from is after to")
	}

	clock := clockFor(s.DTStart)
	fromW := naiveOf(from)
	toW := naiveOf(to)

	if s.Rule == nil {
		return expandOneShot(s, fromW, toW, clock)
	}

	untilW := clock.untilWall(s.Rule)
	ex := s.exceptionWalls(toW, clock)
	cands := s.Rule.generateWalls(naiveOf(s.DTStart), untilW, true)

	var out []Occurrence
	count := 0
	for _, w := range cands {
		occ, ok := clock.occurrence(w, s.AllDay)
		if !ok {
			// A nonexistent local wall time does not consume COUNT.
			continue
		}
		count++
		if s.Rule.Count > 0 && count > s.Rule.Count {
			break
		}
		if ex[w] {
			continue
		}
		if w.Before(fromW) || toW.Before(w) {
			continue
		}
		out = append(out, occ)
	}

	// RDATEs add individual instances (subject to EXDATE).
	seen := map[WallClock]bool{}
	for _, o := range out {
		seen[o.Wall] = true
	}
	for _, r := range s.RDate {
		w := naiveOf(r)
		if ex[w] || seen[w] || w.Before(fromW) || toW.Before(w) {
			continue
		}
		occ, ok := clock.occurrence(w, s.AllDay)
		if !ok {
			continue
		}
		out = append(out, occ)
		seen[w] = true
	}

	sortOccurrences(out)
	return out, nil
}

func expandOneShot(s *Schedule, fromW, toW WallClock, clock scheduleClock) ([]Occurrence, error) {
	w := naiveOf(s.DTStart)
	if s.exceptionWalls(toW, clock)[w] {
		return nil, nil
	}
	if w.Before(fromW) || toW.Before(w) {
		return nil, nil
	}
	occ, ok := clock.occurrence(w, s.AllDay)
	if !ok {
		return nil, nil
	}
	return []Occurrence{occ}, nil
}

func sortOccurrences(o []Occurrence) {
	// small-slice insertion sort; occurrence counts can be large for
	// DAILY rules but windows stay bounded in practice.
	for i := 1; i < len(o); i++ {
		for j := i; j > 0 && o[j].Wall.Before(o[j-1].Wall); j-- {
			o[j], o[j-1] = o[j-1], o[j]
		}
	}
}
