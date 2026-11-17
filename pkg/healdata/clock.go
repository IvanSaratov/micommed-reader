package healdata

import "time"

// Date — календарная дата без часового пояса. Нулевое значение обозначает неизвестную дату.
type Date struct{ Year, Month, Day int }

func (d Date) asTime() time.Time {
	// UTC служит здесь только для календарной арифметики без переходов летнего времени.
	return time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC)
}

func dateFromTime(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, int(m), d}
}

func (d Date) valid() bool {
	return d.Year >= 1 && d.Year <= 9999 && d.Month >= 1 && d.Month <= 12 &&
		d.Day >= 1 && d.Day <= 31 && dateFromTime(d.asTime()) == d
}

// DeviceTime — показание часов устройства с точностью до секунды, без привязки к UTC.
type DeviceTime struct {
	Date
	Hour, Minute, Second int
}

func (v DeviceTime) asTime() time.Time {
	return time.Date(v.Year, time.Month(v.Month), v.Day, v.Hour, v.Minute, v.Second, 0, time.UTC)
}

// ClockAt переводит минуту от 09:00 в показание часов аппарата. false означает,
// что опорная дата неизвестна, противоречит заголовку или минута за пределами суток.
func (d *Day) ClockAt(minute uint16) (DeviceTime, bool) {
	if !d.clockKnown || minute >= 1440 {
		return DeviceTime{}, false
	}
	instant := d.Source.ReportDate.asTime().Add((540 + time.Duration(minute)) * time.Minute)
	date := dateFromTime(instant)
	if !date.valid() {
		return DeviceTime{}, false
	}
	return DeviceTime{date, instant.Hour(), instant.Minute(), 0}, true
}

func readStamp(b []byte, base int, field FieldID) (DeviceTime, error) {
	v := DeviceTime{
		Date{int(word(b, base)), int(b[base+2]), int(b[base+3])},
		int(b[base+4]), int(b[base+5]), int(b[base+6]),
	}
	var delta int
	switch {
	case v.Year < 1 || v.Year > 9999:
		delta = 0
	case v.Month < 1 || v.Month > 12:
		delta = 2
	case !v.Date.valid():
		delta = 3
	case v.Hour > 23:
		delta = 4
	case v.Minute > 59:
		delta = 5
	case v.Second > 59:
		delta = 6
	default:
		return v, nil
	}
	return DeviceTime{}, invalid(InvalidHeader, field, base+delta, -1)
}
