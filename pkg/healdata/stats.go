package healdata

import "math"

// MissingSummary означает отсутствие сохранённого поля, нужного для расчёта.
const MissingSummary Code = "missing_summary"

// Metric — значение с явным признаком доступности и источником.
// При Valid=false причину указывает Reason, а Value равен нулю. Исходные биты хранятся в Day.Stored.
type Metric struct {
	Value  float64
	Valid  bool
	Reason Code
	Origin FieldID
}

// DayStats разделяет сохранённый индекс и оценку по меткам событий.
// EventCounts содержит все коды, включая коды без известной трактовки.
type DayStats struct {
	StoredAHI       Metric
	RecalculatedAHI Metric
	AbsDifference   Metric
	TherapyMinutes  Metric
	EventCounts     map[uint8]int
}

// Stats вычисляет показатели без изменения записи. Для оценки AHI используются
// только коды 102, 103, 104 и положительная конечная длительность из 0x0ddc.
// Проблема сохранённого AHI не закрывает независимую оценку по событиям.
func (d *Day) Stats() DayStats {
	s := DayStats{
		StoredAHI:      storedMetric(d, "summary_0dc8"),
		TherapyMinutes: storedMetric(d, "summary_0ddc"),
		EventCounts:    make(map[uint8]int),
	}
	for _, event := range d.Events {
		s.EventCounts[event.Code]++
	}
	if s.TherapyMinutes.Valid && (s.TherapyMinutes.Value <= 0 || d.hasDiagnostic(InvalidDuration)) {
		s.TherapyMinutes = Metric{Origin: s.TherapyMinutes.Origin, Reason: InvalidDuration}
	}
	s.RecalculatedAHI = Metric{Origin: "formula_ahi"}
	switch {
	case !s.TherapyMinutes.Valid:
		s.RecalculatedAHI.Reason = s.TherapyMinutes.Reason
	case d.hasDiagnostic(CounterMismatch):
		s.RecalculatedAHI.Reason = CounterMismatch
	case d.hasDiagnostic(EventsAtCapacity):
		s.RecalculatedAHI.Reason = EventsAtCapacity
	default:
		s.RecalculatedAHI.Valid = true
		s.RecalculatedAHI.Value = 60 * float64(ahiEventCount(s.EventCounts)) / s.TherapyMinutes.Value
	}
	s.AbsDifference = Metric{Origin: "formula_ahi_abs_difference"}
	switch {
	case !s.StoredAHI.Valid:
		s.AbsDifference.Reason = s.StoredAHI.Reason
	case !s.RecalculatedAHI.Valid:
		s.AbsDifference.Reason = s.RecalculatedAHI.Reason
	default:
		s.AbsDifference.Valid = true
		s.AbsDifference.Value = math.Abs(s.StoredAHI.Value - s.RecalculatedAHI.Value)
	}
	return s
}

func storedMetric(d *Day, field FieldID) Metric {
	m := Metric{Origin: field}
	stored, ok := d.Stored[field]
	if !ok {
		m.Reason = MissingSummary
		return m
	}
	v := float64(stored.Value)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		m.Reason = NonFiniteSummary
		return m
	}
	m.Value = v
	m.Valid = true
	return m
}

func ahiEventCount(counts map[uint8]int) int {
	return counts[102] + counts[103] + counts[104]
}
