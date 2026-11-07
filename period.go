package healdata

import (
	"bytes"
	"errors"
	"sort"
)

var (
	// ErrMixedSources запрещает объединять разные контексты происхождения.
	ErrMixedSources = errors.New("healdata: разные устройства или источники периода")
	// ErrInvalidPeriod означает невозможные либо переставленные границы периода.
	ErrInvalidPeriod = errors.New("healdata: недопустимый диапазон дат")
	// ErrNilDay означает nil вместо дневной записи, в том числе за пределами периода.
	ErrNilDay = errors.New("healdata: nil вместо дневной записи")
)

const (
	// MissingSource означает отсутствие DeviceID и устойчивого SourceID.
	MissingSource Code = "missing_source"
	// DuplicateContent отмечает повтор одних байтов за ту же дату.
	DuplicateContent Code = "duplicate_content"
	// ConflictingVersions исключает дату с разными отпечатками в одном контексте.
	ConflictingVersions Code = "conflicting_versions"
	// NoUsableDays означает отсутствие общего покрытия событий и длительности.
	NoUsableDays Code = "no_usable_days"
)

// DateRange задаёт включительный диапазон календарных дат.
type DateRange struct {
	From    Date
	Through Date
}

// PeriodEntry связывает одно происхождение с отпечатком его байтов.
type PeriodEntry struct {
	Source Source
	SHA256 [32]byte
}

// PeriodRecord сохраняет причины и все происхождения повторов, конфликтов
// либо исключений. Entries сохраняет повторные импорты и отсортирован
// по отпечатку и источнику.
type PeriodRecord struct {
	Date    Date
	Entries []PeriodEntry
	Reason  Code
}

// PeriodSummary содержит оценку по согласованному покрытию событий и времени.
// MissingDates не включает даты с исключёнными входными записями.
// Конфликт даты присутствует и в Conflicts, и в Exclusions.
type PeriodSummary struct {
	From            Date
	Through         Date
	IncludedDays    int
	CountedEvents   int
	TherapyMinutes  Metric
	RecalculatedAHI Metric
	MissingDates    []DateRange
	Duplicates      []PeriodRecord
	Conflicts       []PeriodRecord
	Exclusions      []PeriodRecord
}

// SummarizePeriod считает период from…through включительно в одном контексте.
// Подтверждённый DeviceID имеет приоритет над SourceID; пространства имён
// устройства и источника различны. Записи вне периода пропускаются, но nil
// всегда считается ошибкой аргумента. Результат не зависит от порядка входа.
func SummarizePeriod(days []*Day, from, through Date) (PeriodSummary, error) {
	s := PeriodSummary{From: from, Through: through,
		TherapyMinutes:  Metric{Origin: "formula_period_minutes", Reason: NoUsableDays},
		RecalculatedAHI: Metric{Origin: "formula_period_ahi", Reason: NoUsableDays},
	}
	if !from.valid() || !through.valid() || dateLess(through, from) {
		return PeriodSummary{}, ErrInvalidPeriod
	}
	for _, day := range days {
		if day == nil {
			return PeriodSummary{}, ErrNilDay
		}
	}
	byDate := make(map[Date][]*Day)
	present := make(map[Date]bool)
	var context, contextKind string
	for _, day := range days {
		date := day.Source.ReportDate
		if !date.valid() {
			s.Exclusions = append(s.Exclusions, periodRecord(date, []*Day{day}, MissingDate))
			continue
		}
		if dateLess(date, from) || dateLess(through, date) {
			continue
		}
		present[date] = true
		kind, id := "source", day.Source.SourceID
		if day.Source.DeviceID != "" {
			kind, id = "device", day.Source.DeviceID
		}
		if id == "" {
			s.Exclusions = append(s.Exclusions, periodRecord(date, []*Day{day}, MissingSource))
			continue
		}
		if contextKind != "" && (contextKind != kind || context != id) {
			return PeriodSummary{}, ErrMixedSources
		}
		contextKind, context = kind, id
		byDate[date] = append(byDate[date], day)
	}
	for _, date := range sortedDates(byDate) {
		entries := byDate[date]
		versions := make(map[[32]byte][]*Day)
		for _, day := range entries {
			versions[day.SHA256] = append(versions[day.SHA256], day)
		}
		for _, version := range versions {
			if len(version) > 1 {
				s.Duplicates = append(s.Duplicates, periodRecord(date, version, DuplicateContent))
			}
		}
		if len(versions) > 1 {
			record := periodRecord(date, entries, ConflictingVersions)
			s.Conflicts = append(s.Conflicts, record)
			// Независимые срезы не позволяют изменению одной категории менять другую.
			s.Exclusions = append(s.Exclusions, periodRecord(date, entries, ConflictingVersions))
			continue
		}
		day := entries[0]
		stats := day.Stats()
		reason := Code("")
		if day.hasDiagnostic(DateConflict) {
			reason = DateConflict
		} else if !stats.RecalculatedAHI.Valid {
			reason = stats.RecalculatedAHI.Reason
		}
		if reason != "" {
			s.Exclusions = append(s.Exclusions, periodRecord(date, entries, reason))
			continue
		}
		s.IncludedDays++
		s.CountedEvents += ahiEventCount(stats.EventCounts)
		s.TherapyMinutes.Value += stats.TherapyMinutes.Value
	}
	if s.IncludedDays > 0 {
		s.TherapyMinutes.Valid, s.TherapyMinutes.Reason = true, ""
		s.RecalculatedAHI.Valid, s.RecalculatedAHI.Reason = true, ""
		s.RecalculatedAHI.Value = 60 * float64(s.CountedEvents) / s.TherapyMinutes.Value
	}
	s.MissingDates = missingRanges(present, from, through)
	for _, records := range [][]PeriodRecord{s.Duplicates, s.Conflicts, s.Exclusions} {
		sort.Slice(records, func(i, j int) bool { return recordLess(records[i], records[j]) })
	}
	return s, nil
}

func dateLess(a, b Date) bool {
	if a.Year != b.Year {
		return a.Year < b.Year
	}
	if a.Month != b.Month {
		return a.Month < b.Month
	}
	return a.Day < b.Day
}

func sortedDates[T any](entries map[Date]T) []Date {
	dates := make([]Date, 0, len(entries))
	for date := range entries {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool { return dateLess(dates[i], dates[j]) })
	return dates
}

func periodRecord(date Date, days []*Day, reason Code) PeriodRecord {
	r := PeriodRecord{Date: date, Reason: reason}
	for _, day := range days {
		r.Entries = append(r.Entries, PeriodEntry{Source: day.Source, SHA256: day.SHA256})
	}
	sort.Slice(r.Entries, func(i, j int) bool {
		if cmp := bytes.Compare(r.Entries[i].SHA256[:], r.Entries[j].SHA256[:]); cmp != 0 {
			return cmp < 0
		}
		return sourceLess(r.Entries[i].Source, r.Entries[j].Source)
	})
	return r
}

func sourceLess(a, b Source) bool {
	if a.DeviceID != b.DeviceID {
		return a.DeviceID < b.DeviceID
	}
	if a.SourceID != b.SourceID {
		return a.SourceID < b.SourceID
	}
	if a.Label != b.Label {
		return a.Label < b.Label
	}
	return dateLess(a.ReportDate, b.ReportDate)
}

func recordLess(a, b PeriodRecord) bool {
	if a.Date != b.Date {
		return dateLess(a.Date, b.Date)
	}
	if a.Reason != b.Reason {
		return a.Reason < b.Reason
	}
	for i := 0; i < len(a.Entries) && i < len(b.Entries); i++ {
		if cmp := bytes.Compare(a.Entries[i].SHA256[:], b.Entries[i].SHA256[:]); cmp != 0 {
			return cmp < 0
		}
		if a.Entries[i].Source != b.Entries[i].Source {
			return sourceLess(a.Entries[i].Source, b.Entries[i].Source)
		}
	}
	return len(a.Entries) < len(b.Entries)
}

func missingRanges(present map[Date]bool, from, through Date) []DateRange {
	var ranges []DateRange
	next := from
	for _, date := range sortedDates(present) {
		if dateLess(next, date) {
			ranges = append(ranges, DateRange{From: next, Through: dateFromTime(date.asTime().AddDate(0, 0, -1))})
		}
		next = dateFromTime(date.asTime().AddDate(0, 0, 1))
	}
	if !dateLess(through, next) {
		ranges = append(ranges, DateRange{From: next, Through: through})
	}
	return ranges
}
