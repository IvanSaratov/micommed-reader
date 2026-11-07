package healdata

import (
	"math"
	"os"
	"testing"
)

func statsFixture(t *testing.T, name string) *Day {
	t.Helper()
	raw, err := os.ReadFile("research/fixtures/" + name + ".bin")
	if err != nil {
		t.Fatal(err)
	}
	d, err := DecodeDay(raw, Source{ReportDate: Date{2001, 2, 3}, SourceID: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDayStatsValues(t *testing.T) {
	d := statsFixture(t, "clock")
	s := d.Stats()
	for name, metric := range map[string]Metric{"stored": s.StoredAHI, "calculated": s.RecalculatedAHI, "difference": s.AbsDifference, "minutes": s.TherapyMinutes} {
		if !metric.Valid || metric.Reason != "" || metric.Origin == "" {
			t.Errorf("%s: %+v", name, metric)
		}
	}
	if s.StoredAHI.Value != 2 || s.RecalculatedAHI.Value != 2 || s.AbsDifference.Value != 0 || s.TherapyMinutes.Value != 30 {
		t.Fatalf("wrong metrics: %+v", s)
	}
	if s.EventCounts[102] != 1 || s.EventCounts[238] != 1 || s.EventCounts[101] != 1 {
		t.Fatalf("counts: %v", s.EventCounts)
	}
	s.EventCounts[102] = 999
	if d.Stats().EventCounts[102] != 1 {
		t.Fatal("returned counts alias later results")
	}
}

func TestDayStatsInvalidDependencies(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		mutate                      func(*Day)
		stored, minutes, calculated bool
		reason                      Code
	}{
		{"nan stored", func(d *Day) { d.Stored["summary_0dc8"] = StoredFloat{Value: float32(math.NaN())} }, false, true, true, NonFiniteSummary},
		{"missing stored", func(d *Day) { delete(d.Stored, "summary_0dc8") }, false, true, true, MissingSummary},
		{"zero minutes", func(d *Day) { d.Stored["summary_0ddc"] = StoredFloat{Value: 0} }, true, false, false, InvalidDuration},
		{"negative minutes", func(d *Day) { d.Stored["summary_0ddc"] = StoredFloat{Value: -1} }, true, false, false, InvalidDuration},
		{"nan minutes", func(d *Day) { d.Stored["summary_0ddc"] = StoredFloat{Value: float32(math.NaN())} }, true, false, false, NonFiniteSummary},
		{"infinite minutes", func(d *Day) { d.Stored["summary_0ddc"] = StoredFloat{Value: float32(math.Inf(1))} }, true, false, false, NonFiniteSummary},
		{"missing minutes", func(d *Day) { delete(d.Stored, "summary_0ddc") }, true, false, false, MissingSummary},
		{"duration warning", func(d *Day) { d.Diagnostics = append(d.Diagnostics, Diagnostic{Code: InvalidDuration}) }, true, false, false, InvalidDuration},
		{"mismatch", func(d *Day) { d.Diagnostics = append(d.Diagnostics, Diagnostic{Code: CounterMismatch}) }, true, true, false, CounterMismatch},
		{"capacity", func(d *Day) { d.Diagnostics = append(d.Diagnostics, Diagnostic{Code: EventsAtCapacity}) }, true, true, false, EventsAtCapacity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := statsFixture(t, "clock")
			tc.mutate(d)
			s := d.Stats()
			if s.StoredAHI.Valid != tc.stored || s.TherapyMinutes.Valid != tc.minutes || s.RecalculatedAHI.Valid != tc.calculated || s.AbsDifference.Valid {
				t.Fatalf("dependencies: %+v", s)
			}
			var failed Metric
			if !tc.minutes {
				failed = s.TherapyMinutes
			} else if !tc.stored {
				failed = s.StoredAHI
			} else {
				failed = s.RecalculatedAHI
			}
			if failed.Reason != tc.reason || s.AbsDifference.Reason != tc.reason {
				t.Fatalf("reason: %+v", s)
			}
		})
	}
}

func TestDayStatsMeasuredZero(t *testing.T) {
	d := statsFixture(t, "minimum")
	s := d.Stats()
	if !s.RecalculatedAHI.Valid || s.RecalculatedAHI.Value != 0 || !s.StoredAHI.Valid || s.StoredAHI.Value != 0 {
		t.Fatalf("available zero: %+v", s)
	}
}

func TestDayStatsDateDoesNotChangeIndependentEstimate(t *testing.T) {
	d := statsFixture(t, "clock")
	d.Diagnostics = append(d.Diagnostics, Diagnostic{Code: DateConflict})
	s := d.Stats()
	if !s.RecalculatedAHI.Valid || s.RecalculatedAHI.Value != 2 {
		t.Fatalf("calendar conflict changed event estimate: %+v", s)
	}
}

func TestUnavailableAHILeavesNaNOnlyInRawField(t *testing.T) {
	d := mustDay(t, "nan")
	s := d.Stats()
	if s.StoredAHI.Valid || s.StoredAHI.Reason != NonFiniteSummary || s.StoredAHI.Value != 0 || !s.RecalculatedAHI.Valid || s.RecalculatedAHI.Value != 2 || !math.IsNaN(float64(d.Stored["summary_0dc8"].Value)) {
		t.Fatal("NaN должен сохраниться в исходной сводке; доступность результата задаётся Valid и Reason")
	}
}
