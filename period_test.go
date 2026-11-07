package healdata

import (
	"errors"
	"math"
	"os"
	"reflect"
	"testing"
)

func periodDay(date Date, hash byte, minutes float32, codes ...uint8) *Day {
	d := &Day{Source: Source{ReportDate: date, SourceID: "archive"}, Stored: map[FieldID]StoredFloat{"summary_0dc8": {Value: 0}, "summary_0ddc": {Value: minutes}}, Counters: map[uint8]uint16{}}
	d.SHA256[0] = hash
	for _, code := range codes {
		d.Events = append(d.Events, Event{Code: code})
		if code >= 101 && code <= 104 {
			d.Counters[code]++
		}
	}
	return d
}

func TestPeriodWeightedCoverage(t *testing.T) {
	d1 := periodDay(Date{2001, 2, 3}, 1, 30, 102, 238, 101)
	d2 := periodDay(Date{2001, 2, 4}, 2, 1)
	s, err := SummarizePeriod([]*Day{d1, d2, d1}, Date{2001, 2, 3}, Date{2001, 2, 4})
	if err != nil {
		t.Fatal(err)
	}
	if s.IncludedDays != 2 || s.CountedEvents != 1 || !s.TherapyMinutes.Valid || s.TherapyMinutes.Value != 31 || !s.RecalculatedAHI.Valid || s.RecalculatedAHI.Value != 60.0/31 {
		t.Fatalf("summary: %+v", s)
	}
	if len(s.Duplicates) != 1 || len(s.Duplicates[0].Entries) != 2 || len(s.MissingDates) != 0 || len(s.Exclusions) != 0 {
		t.Fatalf("coverage: %+v", s)
	}
}

func TestPeriodFixturePair(t *testing.T) {
	d1 := statsFixture(t, "clock")
	raw, err := os.ReadFile("research/fixtures/next-day.bin")
	if err != nil {
		t.Fatal(err)
	}
	d2, err := DecodeDay(raw, Source{ReportDate: Date{2001, 2, 4}, SourceID: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := SummarizePeriod([]*Day{d2, d1, d1}, Date{2001, 2, 3}, Date{2001, 2, 4})
	if err != nil {
		t.Fatal(err)
	}
	if s.IncludedDays != 2 || s.CountedEvents != 1 || s.TherapyMinutes.Value != 31 || !s.RecalculatedAHI.Valid || s.RecalculatedAHI.Value != 60.0/31 {
		t.Fatalf("fixture pair: %+v", s)
	}
}

func TestPeriodUsesOnlyDependenciesOfEventEstimate(t *testing.T) {
	d := periodDay(Date{2001, 2, 3}, 1, 60, 102, 103, 104, 238)
	d.Stored["summary_0dc8"] = StoredFloat{Value: float32(math.NaN())}
	s, err := SummarizePeriod([]*Day{d}, d.Source.ReportDate, d.Source.ReportDate)
	if err != nil {
		t.Fatal(err)
	}
	if s.IncludedDays != 1 || s.CountedEvents != 3 || !s.RecalculatedAHI.Valid || s.RecalculatedAHI.Value != 3 {
		t.Fatalf("independent estimate: %+v", s)
	}
}

func TestPeriodMissingVersusExcluded(t *testing.T) {
	bad := periodDay(Date{2001, 2, 4}, 2, 0)
	s, err := SummarizePeriod([]*Day{periodDay(Date{2001, 2, 2}, 1, 30), bad}, Date{2001, 2, 1}, Date{2001, 2, 6})
	if err != nil {
		t.Fatal(err)
	}
	want := []DateRange{{From: Date{2001, 2, 1}, Through: Date{2001, 2, 1}}, {From: Date{2001, 2, 3}, Through: Date{2001, 2, 3}}, {From: Date{2001, 2, 5}, Through: Date{2001, 2, 6}}}
	if !reflect.DeepEqual(s.MissingDates, want) || len(s.Exclusions) != 1 || s.Exclusions[0].Date != bad.Source.ReportDate || s.Exclusions[0].Reason != InvalidDuration {
		t.Fatalf("coverage: %+v", s)
	}
}

func TestPeriodEmpty(t *testing.T) {
	s, err := SummarizePeriod(nil, Date{2000, 2, 28}, Date{2000, 3, 1})
	if err != nil {
		t.Fatal(err)
	}
	if s.IncludedDays != 0 || s.RecalculatedAHI.Valid || s.RecalculatedAHI.Reason != NoUsableDays || s.TherapyMinutes.Valid || !reflect.DeepEqual(s.MissingDates, []DateRange{{From: Date{2000, 2, 28}, Through: Date{2000, 3, 1}}}) {
		t.Fatalf("empty: %+v", s)
	}
}

func TestPeriodConflictAndStableOrdering(t *testing.T) {
	a := periodDay(Date{2001, 2, 3}, 1, 30, 102)
	b := periodDay(Date{2001, 2, 3}, 2, 30, 102)
	c := periodDay(Date{2001, 2, 4}, 3, 20)
	a.Source.Label = "z"
	b.Source.Label = "b"
	c.Source.Label = "a"
	copyA := *a
	copyA.Source.Label = "a"
	inputs := [][]*Day{{a, b, c, &copyA}, {&copyA, c, b, a}, {b, a, &copyA, c}}
	var first PeriodSummary
	for i, input := range inputs {
		s, err := SummarizePeriod(input, Date{2001, 2, 3}, Date{2001, 2, 5})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = s
		} else if !reflect.DeepEqual(s, first) {
			t.Fatalf("input order changes result: %+v versus %+v", s, first)
		}
	}
	if first.IncludedDays != 1 || first.CountedEvents != 0 || first.TherapyMinutes.Value != 20 || len(first.Conflicts) != 1 || first.Conflicts[0].Reason != ConflictingVersions || len(first.Exclusions) != 1 || len(first.Duplicates) != 1 || len(first.MissingDates) != 1 {
		t.Fatalf("conflict: %+v", first)
	}
	entries := first.Conflicts[0].Entries
	if len(entries) != 3 || entries[0].Source.Label != "a" || entries[0].SHA256[0] != 1 || entries[1].Source.Label != "z" || entries[1].SHA256[0] != 1 || entries[2].Source.Label != "b" || entries[2].SHA256[0] != 2 {
		t.Fatalf("version lost its origins: %+v", entries)
	}
}

func TestPeriodExclusionReasons(t *testing.T) {
	for _, code := range []Code{CounterMismatch, EventsAtCapacity, DateConflict, InvalidDuration} {
		t.Run(string(code), func(t *testing.T) {
			d := periodDay(Date{2001, 2, 3}, 1, 30, 102)
			d.Diagnostics = append(d.Diagnostics, Diagnostic{Code: code})
			s, err := SummarizePeriod([]*Day{d}, d.Source.ReportDate, d.Source.ReportDate)
			if err != nil || s.IncludedDays != 0 || s.CountedEvents != 0 || s.TherapyMinutes.Valid || len(s.Exclusions) != 1 || s.Exclusions[0].Reason != code || len(s.MissingDates) != 0 {
				t.Fatalf("exclusion: %+v, %v", s, err)
			}
		})
	}
}

func TestPeriodIdentity(t *testing.T) {
	from, through := Date{2001, 2, 3}, Date{2001, 2, 4}
	for _, tc := range []struct {
		name                               string
		device1, source1, device2, source2 string
		mixed                              bool
	}{
		{"two devices", "one", "x", "two", "x", true},
		{"same device", "one", "x", "one", "y", false},
		{"two sources", "", "x", "", "y", true},
		{"separate namespaces", "x", "", "", "x", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := periodDay(from, 1, 30), periodDay(through, 2, 30)
			a.Source.DeviceID, a.Source.SourceID = tc.device1, tc.source1
			b.Source.DeviceID, b.Source.SourceID = tc.device2, tc.source2
			_, err := SummarizePeriod([]*Day{a, b}, from, through)
			if errors.Is(err, ErrMixedSources) != tc.mixed || (!tc.mixed && err != nil) {
				t.Fatalf("identity: %v", err)
			}
		})
	}
	noID := periodDay(from, 1, 30)
	noID.Source.SourceID = ""
	noDate := periodDay(Date{}, 2, 30)
	out := periodDay(Date{2001, 2, 5}, 3, 30)
	out.Source.DeviceID = "other"
	s, err := SummarizePeriod([]*Day{noID, noDate, out}, from, through)
	if err != nil || len(s.Exclusions) != 2 || s.Exclusions[0].Reason != MissingDate || s.Exclusions[1].Reason != MissingSource || !reflect.DeepEqual(s.MissingDates, []DateRange{{From: through, Through: through}}) {
		t.Fatalf("unknown and outside: %+v, %v", s, err)
	}
}

func TestPeriodArguments(t *testing.T) {
	for _, tc := range []struct{ from, through Date }{
		{Date{}, Date{2001, 2, 3}}, {Date{2001, 2, 31}, Date{2001, 3, 3}}, {Date{2001, 2, 4}, Date{2001, 2, 3}},
		{Date{-1, 1, 1}, Date{2001, 2, 3}}, {Date{2001, 2, 3}, Date{10000, 1, 1}},
	} {
		if _, err := SummarizePeriod(nil, tc.from, tc.through); !errors.Is(err, ErrInvalidPeriod) {
			t.Fatalf("range %v…%v: %v", tc.from, tc.through, err)
		}
	}
	if _, err := SummarizePeriod([]*Day{nil}, Date{2001, 2, 3}, Date{2001, 2, 4}); !errors.Is(err, ErrNilDay) {
		t.Fatalf("nil: %v", err)
	}
}
