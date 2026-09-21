package rrule

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// parity_cases.json is a shared rule/window matrix. parity_golden.json
// holds the exact occurrence lists produced for it by the pre-refactor
// implementation (commit ec8e856, generated via the snapshot generator
// described in testdata/README.md). This test proves the refactor
// emits, for every rule and window, the identical occurrence list:
// same wall clocks, same floating flags, same UTC instants.

type parityCase struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	DTStart string   `json:"dtstart"`
	RRule   string   `json:"rrule"`
	ExRule  string   `json:"exrule"`
	ExDate  []string `json:"exdate"`
	RDate   []string `json:"rdate"`
	AllDay  bool     `json:"allday"`
	From    string   `json:"from"`
	To      string   `json:"to"`
}

type parityOcc struct {
	Wall     string `json:"wall"`
	Instant  string `json:"instant,omitempty"`
	Zone     string `json:"zone,omitempty"`
	Floating bool   `json:"floating"`
	AllDay   bool   `json:"allday"`
}

type parityGolden struct {
	Name  string      `json:"name"`
	Error string      `json:"error,omitempty"`
	Occs  []parityOcc `json:"occs"`
}

type parityCasesFile struct {
	Cases []parityCase `json:"cases"`
}

type parityGoldenFile struct {
	Golden []parityGolden `json:"golden"`
}

func parityLoc(t *testing.T, kind string) *time.Location {
	t.Helper()
	switch kind {
	case "floating":
		return Floating
	case "utc":
		return time.UTC
	default:
		return loadZone(t, kind)
	}
}

func parityParse(t *testing.T, s string, loc *time.Location) time.Time {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02T15:04:05", s, loc)
	if err != nil {
		t.Fatalf("bad parity time %q: %v", s, err)
	}
	return tm
}

func loadParity(t *testing.T) ([]parityCase, map[string]parityGolden) {
	t.Helper()
	dir := filepath.Join("testdata")

	cb, err := os.ReadFile(filepath.Join(dir, "parity_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cf parityCasesFile
	if err := json.Unmarshal(cb, &cf); err != nil {
		t.Fatal(err)
	}

	gb, err := os.ReadFile(filepath.Join(dir, "parity_golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gf parityGoldenFile
	if err := json.Unmarshal(gb, &gf); err != nil {
		t.Fatal(err)
	}
	golden := make(map[string]parityGolden, len(gf.Golden))
	for _, g := range gf.Golden {
		golden[g.Name] = g
	}
	return cf.Cases, golden
}

func parityEncode(o Occurrence) parityOcc {
	po := parityOcc{
		Wall: fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d",
			o.Wall.Year, o.Wall.Month, o.Wall.Day,
			o.Wall.Hour, o.Wall.Minute, o.Wall.Second),
		Floating: o.Floating,
		AllDay:   o.AllDay,
	}
	if !o.Instant.IsZero() {
		po.Instant = o.Instant.UTC().Format("2006-01-02T15:04:05Z")
		po.Zone = o.Instant.Location().String()
	}
	return po
}

func TestParityWithPreRefactor(t *testing.T) {
	cases, golden := loadParity(t)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			loc := parityLoc(t, c.Kind)
			s := &Schedule{
				DTStart: parityParse(t, c.DTStart, loc),
				AllDay:  c.AllDay,
			}
			if c.RRule != "" {
				s.Rule = mustParse(t, c.RRule)
			}
			if c.ExRule != "" {
				s.ExRule = mustParse(t, c.ExRule)
			}
			for _, e := range c.ExDate {
				s.ExDate = append(s.ExDate, parityParse(t, e, loc))
			}
			for _, r := range c.RDate {
				s.RDate = append(s.RDate, parityParse(t, r, loc))
			}

			want, ok := golden[c.Name]
			if !ok {
				t.Fatalf("no golden entry for %q", c.Name)
			}

			occs, err := Expand(s, parityParse(t, c.From, loc), parityParse(t, c.To, loc))
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if gotErr != want.Error {
				t.Fatalf("error = %q, want %q", gotErr, want.Error)
			}

			got := make([]parityOcc, 0, len(occs))
			for _, o := range occs {
				got = append(got, parityEncode(o))
			}
			if len(got) != len(want.Occs) {
				t.Fatalf("occurrence count = %d, want %d\ngot:  %+v\nwant: %+v",
					len(got), len(want.Occs), got, want.Occs)
			}
			for i := range got {
				if got[i] != want.Occs[i] {
					t.Fatalf("occ %d = %+v, want %+v", i, got[i], want.Occs[i])
				}
			}
		})
	}
}
