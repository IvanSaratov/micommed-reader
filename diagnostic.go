package healdata

import "fmt"

// Code — стабильный идентификатор причины. Потребители сравнивают коды, а не текст ошибки.
type Code string

// Структурные ошибки, предупреждения о покрытии и расхождения с исследованным профилем.
const (
	UnsupportedLength Code = "unsupported_length"
	InvalidHeader     Code = "invalid_header"
	InvalidSourceDate Code = "invalid_source_date"
	CountOutOfRange   Code = "count_out_of_range"
	InvalidMinute     Code = "invalid_minute"
	MissingDate       Code = "missing_date"
	DateConflict      Code = "date_conflict"
	EmptySeries       Code = "empty_series"
	SeriesAtCapacity  Code = "series_at_capacity"
	EventsAtCapacity  Code = "events_at_capacity"
	CounterMismatch   Code = "counter_mismatch"
	AxesDiffer        Code = "axes_differ"
	TimeReversal      Code = "time_reversal"
	SampleGap         Code = "sample_gap"
	NonFiniteSummary  Code = "non_finite_summary"
	InvalidDuration   Code = "invalid_duration"
	ProfileDeviation  Code = "profile_deviation"
)

// Diagnostic привязывает причину к полю, байту и элементу массива.
// Неприменимые Offset и Index равны −1; смещения даны от начала дневного файла.
type Diagnostic struct {
	Code          Code
	Field         FieldID
	Offset, Index int
}

// DecodeError означает, что структуру записи небезопасно использовать. Поддерживает errors.As.
type DecodeError struct{ Diagnostic }

// Error возвращает краткое описание структурной ошибки.
func (e *DecodeError) Error() string {
	location := string(e.Field)
	if e.Offset >= 0 {
		location += fmt.Sprintf(" @0x%04x", e.Offset)
	}
	if e.Index >= 0 {
		location += fmt.Sprintf(" [%d]", e.Index)
	}
	return fmt.Sprintf("healdata: %s (%s)", e.Code, location)
}

func invalid(code Code, field FieldID, offset, index int) error {
	return &DecodeError{Diagnostic{code, field, offset, index}}
}
