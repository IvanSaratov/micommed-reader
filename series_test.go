package healdata

import (
	"encoding/binary"
	"testing"
)

func TestSeries(t *testing.T) {
	d := mustDay(t, "minimum")
	if len(d.Pressure) != 1 || d.Pressure[0].FirstRaw != 0 || len(d.Events) != 0 {
		t.Fatal("count/tail interpretation")
	}
	for _, ch := range d.Channels {
		if len(ch.Points) != 1 || ch.Points[0].Raw != 0 {
			t.Fatal(ch.ID)
		}
	}
	d = mustDay(t, "at-capacity")
	if len(d.Pressure) != 400 || d.Pressure[399].FirstRaw != 429 || len(d.Channels[1].Points) != 290 || d.Channels[1].Points[289].Raw != 1289 || d.Channels[4].Points[289].Raw != 89 || !d.hasDiagnostic(SeriesAtCapacity) {
		t.Fatal("last active sample")
	}
	b := fixture(t, "clock")
	binary.LittleEndian.PutUint16(b[0x1f74+4:], 898)
	d, err := DecodeDay(b, testSource())
	if err != nil || !d.hasDiagnostic(AxesDiffer) || !d.hasDiagnostic(TimeReversal) || !d.hasDiagnostic(SampleGap) {
		t.Fatal("independent axes", err)
	}
	if d.Channels[4].Points[2].Minute != 898 || d.Channels[3].Points[2].Minute != 900 {
		t.Fatal("reordered samples")
	}
	d = mustDay(t, "unobserved-empty")
	if !d.hasDiagnostic(EmptySeries) || len(d.Pressure) != 0 || len(d.Channels[0].Points) != 4 {
		t.Fatal("empty pressure series")
	}
}
func TestEvents(t *testing.T) {
	d := mustDay(t, "clock")
	if len(d.Events) != 3 || d.Events[0] != (Event{900, 102}) || d.Events[1] != (Event{900, 238}) || d.Events[2] != (Event{1439, 101}) || d.Counters[102] != 1 {
		t.Fatal("event ordering or count")
	}
	d = mustDay(t, "at-capacity")
	if !d.hasDiagnostic(EventsAtCapacity) || len(d.Events) != 150 || d.Events[149].Minute != 149 {
		t.Fatal("events capacity")
	}
	d = mustDay(t, "counter-mismatch")
	if !d.hasDiagnostic(CounterMismatch) || d.Counters[102] != 2 || len(d.Events) != 3 {
		t.Fatal("counter mismatch is partial")
	}
}
