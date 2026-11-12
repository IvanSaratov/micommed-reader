package healdata

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"testing"
)

func sessionDay(date Date, minutes []uint16, duration float32) *Day {
	d := &Day{Source: Source{ReportDate: date, SourceID: "one", Label: fmt.Sprint(date, minutes)}, clockKnown: true, Stored: map[FieldID]StoredFloat{"summary_0ddc": {Value: duration}}}
	d.SHA256 = sha256.Sum256([]byte(d.Source.Label))
	for _, m := range minutes {
		d.Pressure = append(d.Pressure, PressurePoint{m, 70, 80})
	}
	return d
}
func TestSessionsBoundaryAndSplit(t *testing.T) {
	var night []uint16
	for m := 840; m < 1440; m += 30 {
		night = append(night, uint16(m))
	}
	night = append(night, 1439)
	a := sessionDay(Date{2001, 2, 25}, night, 600)
	b := sessionDay(Date{2001, 2, 26}, []uint16{0, 20, 40, 840, 870}, 100)
	a.Events = []Event{{900, 102}, {900, 238}}
	got, err := GroupSessions([]*Day{b, a}, 60)
	if err != nil || len(got.Sessions) != 2 {
		t.Fatalf("%v %+v", err, got)
	}
	s := got.Sessions[0]
	if s.Start.Day != 25 || s.Start.Hour != 23 || s.End.Day != 26 || s.End.Hour != 9 || s.End.Minute != 40 || len(s.Parts) != 2 {
		t.Fatal(s)
	}
	if s.EventCounts[102] != 1 || s.EventCounts[238] != 1 || s.TherapyMinutes.Valid || s.RecalculatedAHI.Reason != PartialDayDuration {
		t.Fatal(s)
	}
	if got.Sessions[1].Start.Hour != 23 || got.Sessions[1].RecalculatedAHI.Valid {
		t.Fatal(got.Sessions[1])
	}
}
func TestSessionsGap60And61(t *testing.T) {
	d := sessionDay(Date{2001, 2, 25}, []uint16{0, 60, 121}, 30)
	s, err := GroupSessions([]*Day{d}, 60)
	if err != nil || len(s.Sessions) != 2 || s.Sessions[0].SpanMinutes != 60 {
		t.Fatal(s, err)
	}
	if _, err = GroupSessions([]*Day{d}, 0); err == nil {
		t.Fatal("нулевой порог")
	}
	if _, err = GroupSessions([]*Day{nil}, 60); err != ErrNilDay {
		t.Fatal(err)
	}
}
func TestSessionsWholeDayMetrics(t *testing.T) {
	a := sessionDay(Date{2001, 2, 25}, []uint16{1430, 1439}, 30)
	b := sessionDay(Date{2001, 2, 26}, []uint16{0, 20}, 10)
	a.Events = []Event{{1439, 102}}
	b.Events = []Event{{0, 103}, {0, 238}}
	s, err := GroupSessions([]*Day{a, b, a}, 60)
	if err != nil || len(s.Sessions) != 1 {
		t.Fatal(s, err)
	}
	got := s.Sessions[0]
	if !got.TherapyMinutes.Valid || got.TherapyMinutes.Value != 40 || !got.RecalculatedAHI.Valid || got.RecalculatedAHI.Value != 3 || len(got.Parts) != 2 {
		t.Fatal(got)
	}
	a.Diagnostics = append(a.Diagnostics, Diagnostic{Code: CounterMismatch})
	s, _ = GroupSessions([]*Day{a, b}, 60)
	if s.Sessions[0].RecalculatedAHI.Reason != CounterMismatch || !s.Sessions[0].TherapyMinutes.Valid {
		t.Fatal(s)
	}
}
func TestSessionsSourceConflictAndClock(t *testing.T) {
	a := sessionDay(Date{2001, 2, 25}, []uint16{1430, 1439}, 30)
	b := sessionDay(Date{2001, 2, 26}, []uint16{0, 20}, 10)
	b.Source.SourceID = "two"
	s, _ := GroupSessions([]*Day{a, b}, 60)
	if len(s.Sessions) != 2 {
		t.Fatal(s)
	}
	c := sessionDay(a.Source.ReportDate, []uint16{1, 2}, 5)
	s, _ = GroupSessions([]*Day{a, c, b}, 60)
	if len(s.Sessions) != 1 || len(s.Excluded) != 2 || s.Excluded[0].Reason != ConflictingVersions {
		t.Fatal(s)
	}
	a.clockKnown = false
	b.Diagnostics = []Diagnostic{{Code: TimeReversal}}
	s, _ = GroupSessions([]*Day{a, b}, 60)
	if len(s.Sessions) != 0 || len(s.Excluded) != 2 {
		t.Fatal(s)
	}
	a.clockKnown = true
	a.Source.SourceID = ""
	s, _ = GroupSessions([]*Day{a}, 60)
	if len(s.Sessions) != 0 || s.Excluded[0].Reason != MissingSource {
		t.Fatal(s)
	}
}
func TestSessionsCalendarTransitionAndStableOrder(t *testing.T) {
	a := sessionDay(Date{2001, 12, 31}, []uint16{890, 900}, 10)
	b := sessionDay(Date{2002, 1, 1}, []uint16{0, 20}, 20)
	x, _ := GroupSessions([]*Day{a, b}, 60)
	y, _ := GroupSessions([]*Day{b, a}, 60)
	if !reflect.DeepEqual(x, y) || x.Sessions[0].End.Year != 2002 || x.Sessions[0].End.Day != 1 {
		t.Fatal(x, y)
	}
	if len(a.Pressure) != 2 || a.Pressure[0].Minute != 890 {
		t.Fatal("исходные данные изменены")
	}
}

func TestSessionsUnrepresentableClock(t *testing.T) {
	d := sessionDay(Date{9999, 12, 31}, []uint16{900}, 1)
	s, err := GroupSessions([]*Day{d}, 60)
	if err != nil || len(s.Sessions) != 0 || len(s.Excluded) != 1 || s.Excluded[0].Reason != DateConflict {
		t.Fatal(s, err)
	}
}

func TestSessionsConfirmedDeviceIdentity(t *testing.T) {
	a := sessionDay(Date{2001, 2, 25}, []uint16{1439}, 1)
	b := sessionDay(Date{2001, 2, 26}, []uint16{0}, 1)
	a.Source.SourceID = "archive-a"
	b.Source.SourceID = "archive-b"
	a.Source.DeviceID = "confirmed-device"
	b.Source.DeviceID = "confirmed-device"
	same, err := GroupSessions([]*Day{a, b}, 60)
	if err != nil || len(same.Sessions) != 1 {
		t.Fatal(same, err)
	}
	b.Source.DeviceID = "another-device"
	b.Source.SourceID = a.Source.SourceID
	different, err := GroupSessions([]*Day{a, b}, 60)
	if err != nil || len(different.Sessions) != 2 {
		t.Fatal(different, err)
	}
}
