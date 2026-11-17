package healdata

import (
	"errors"
	"testing"
)

func TestDeviceClock(t *testing.T) {
	d := mustDay(t, "clock")
	for minute, want := range map[uint16]DeviceTime{
		0: {Date{2001, 2, 3}, 9, 0, 0}, 899: {Date{2001, 2, 3}, 23, 59, 0},
		900: {Date{2001, 2, 4}, 0, 0, 0}, 1439: {Date{2001, 2, 4}, 8, 59, 0},
	} {
		if got, ok := d.ClockAt(minute); !ok || got != want {
			t.Errorf("minute %d: %v, %v", minute, got, ok)
		}
	}
	if _, ok := d.ClockAt(1440); ok {
		t.Error("accepted 1440")
	}
	for _, tc := range []struct {
		date Date
		code Code
	}{{Date{}, MissingDate}, {Date{2001, 2, 4}, DateConflict}} {
		d, err := DecodeDay(fixture(t, "clock"), Source{ReportDate: tc.date})
		if err != nil || !d.hasDiagnostic(tc.code) {
			t.Fatal("expected warning", tc, err)
		}
		if _, ok := d.ClockAt(0); ok {
			t.Fatal("invented clock")
		}
	}
	for _, date := range []Date{{2001, 2, 29}, {2000, 2, 31}, {2000, 13, 1}} {
		_, err := DecodeDay(fixture(t, "minimum"), Source{ReportDate: date})
		var de *DecodeError
		if !errors.As(err, &de) || de.Code != InvalidSourceDate {
			t.Fatal(date, err)
		}
	}
}
