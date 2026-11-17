package healdata

import (
	"bytes"
	"cmp"
	"errors"
	"slices"
	"time"
)

// PartialDayDuration: аппарат хранит длительность за сутки, а не за отдельный фрагмент.
const PartialDayDuration Code = "partial_day_duration"

var ErrInvalidSessionGap = errors.New("healdata: перерыв сеанса должен быть от 1 до 240 минут")

// SessionPart задаёт включительный диапазон исходных минут одной записи.
// Day остаётся исходной записью: индекс, происхождение и байты не изменяются.
type SessionPart struct {
	Day                     *Day
	FirstMinute, LastMinute uint16
}

// Session объединяет близкие наблюдения одного источника. Это группировка записей,
// а не установление физиологического сна. SpanMinutes включает короткие перерывы.
type Session struct {
	Start, End                      DeviceTime
	Parts                           []SessionPart
	SpanMinutes                     int
	TherapyMinutes, RecalculatedAHI Metric
	EventCounts                     map[uint8]int
}

type SessionExclusion struct {
	Day    *Day
	Reason Code
}
type SessionSet struct {
	Sessions []Session
	Excluded []SessionExclusion
}

func sessionIdentity(d *Day) string {
	if d.Source.DeviceID != "" {
		return "device:" + d.Source.DeviceID
	}
	if d.Source.SourceID != "" {
		return "source:" + d.Source.SourceID
	}
	return ""
}
func sessionDayCompare(a, b *Day) int {
	if c := cmp.Compare(sessionIdentity(a), sessionIdentity(b)); c != 0 {
		return c
	}
	if c := a.Source.ReportDate.asTime().Compare(b.Source.ReportDate.asTime()); c != 0 {
		return c
	}
	if c := bytes.Compare(a.SHA256[:], b.SHA256[:]); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Source.SourceID, b.Source.SourceID); c != 0 {
		return c
	}
	return cmp.Compare(a.Source.Label, b.Source.Label)
}

type sessionFragment struct {
	part       SessionPart
	start, end time.Time
	identity   string
}

// GroupSessions соединяет соседние наблюдения с перерывом <= maxGapMinutes.
// Дневные файлы сначала делятся на фрагменты: иначе утро и следующий вечер
// одного файла склеили бы несколько ночей цепочкой. Порядок входа несущественен.
// Подтверждённый DeviceID имеет приоритет над SourceID, как у SummarizePeriod.
func GroupSessions(days []*Day, maxGapMinutes int) (SessionSet, error) {
	result := SessionSet{Sessions: []Session{}, Excluded: []SessionExclusion{}}
	if maxGapMinutes < 1 || maxGapMinutes > 240 {
		return result, ErrInvalidSessionGap
	}
	ordered := slices.Clone(days)
	for _, d := range ordered {
		if d == nil {
			return result, ErrNilDay
		}
	}
	slices.SortFunc(ordered, sessionDayCompare)
	var fragments []sessionFragment
	for i := 0; i < len(ordered); {
		j := i + 1
		for j < len(ordered) && sessionIdentity(ordered[j]) == sessionIdentity(ordered[i]) && ordered[j].Source.ReportDate == ordered[i].Source.ReportDate {
			j++
		}
		group := ordered[i:j]
		i = j
		d := group[0]
		reason := Code("")
		switch {
		case sessionIdentity(d) == "":
			reason = MissingSource
		case !d.Source.ReportDate.valid():
			reason = MissingDate
		case group[len(group)-1].SHA256 != d.SHA256:
			reason = ConflictingVersions
		case !d.clockKnown:
			reason = DateConflict
		case d.hasDiagnostic(TimeReversal):
			reason = TimeReversal
		}
		if reason != "" {
			for _, x := range group {
				result.Excluded = append(result.Excluded, SessionExclusion{x, reason})
			}
			continue
		}
		// Битовая карта минут объединяет оси, но исходные ряды и повторы не теряются.
		var observed [1440]bool
		for _, p := range d.Pressure {
			observed[p.Minute] = true
		}
		for _, ch := range d.Channels {
			for _, p := range ch.Points {
				observed[p.Minute] = true
			}
		}
		for _, e := range d.Events {
			observed[e.Minute] = true
		}
		validClock := true
		for minute, exists := range observed {
			if exists {
				if _, ok := d.ClockAt(uint16(minute)); !ok {
					validClock = false
					break
				}
			}
		}
		if !validClock {
			result.Excluded = append(result.Excluded, SessionExclusion{d, DateConflict})
			continue
		}
		first, last := -1, -1
		appendPart := func() {
			if first < 0 {
				return
			}
			start, _ := d.ClockAt(uint16(first))
			end, _ := d.ClockAt(uint16(last))
			fragments = append(fragments, sessionFragment{SessionPart{d, uint16(first), uint16(last)}, start.asTime(), end.asTime(), sessionIdentity(d)})
		}
		for minute, exists := range observed {
			if !exists {
				continue
			}
			if first < 0 {
				first = minute
			} else if minute-last > maxGapMinutes {
				appendPart()
				first = minute
			}
			last = minute
		}
		appendPart()
		if first < 0 {
			result.Excluded = append(result.Excluded, SessionExclusion{d, EmptySeries})
		}
	}
	slices.SortFunc(fragments, func(a, b sessionFragment) int {
		if c := cmp.Compare(a.identity, b.identity); c != 0 {
			return c
		}
		if c := a.start.Compare(b.start); c != 0 {
			return c
		}
		return a.end.Compare(b.end)
	})
	var identity string
	for _, f := range fragments {
		n := len(result.Sessions)
		if n == 0 || identity != f.identity || f.start.Sub(result.Sessions[n-1].End.asTime()) > time.Duration(maxGapMinutes)*time.Minute {
			start, _ := f.part.Day.ClockAt(f.part.FirstMinute)
			result.Sessions = append(result.Sessions, Session{Start: start, Parts: []SessionPart{}, EventCounts: map[uint8]int{}})
			n++
		}
		identity = f.identity
		s := &result.Sessions[n-1]
		s.Parts = append(s.Parts, f.part)
		s.End, _ = f.part.Day.ClockAt(f.part.LastMinute)
	}
	// Суточную длительность разрешено присвоить только одному полному сеансу.
	contributions := map[*Day]int{}
	for _, s := range result.Sessions {
		for _, p := range s.Parts {
			contributions[p.Day]++
		}
	}
	for i := range result.Sessions {
		s := &result.Sessions[i]
		s.SpanMinutes = int(s.End.asTime().Sub(s.Start.asTime()) / time.Minute)
		durationReason, ahiReason := Code(""), Code("")
		var minutes float64
		for _, part := range s.Parts {
			d := part.Day
			for _, e := range d.Events {
				if e.Minute >= part.FirstMinute && e.Minute <= part.LastMinute {
					s.EventCounts[e.Code]++
				}
			}
			stats := d.Stats()
			if contributions[d] > 1 {
				durationReason = PartialDayDuration
			} else if !stats.TherapyMinutes.Valid && durationReason == "" {
				durationReason = stats.TherapyMinutes.Reason
			}
			if !stats.RecalculatedAHI.Valid && ahiReason == "" {
				ahiReason = stats.RecalculatedAHI.Reason
			}
			minutes += stats.TherapyMinutes.Value
		}
		s.TherapyMinutes = Metric{Origin: "formula_session_minutes", Reason: durationReason}
		s.RecalculatedAHI = Metric{Origin: "formula_session_ahi"}
		if durationReason != "" {
			s.RecalculatedAHI.Reason = durationReason
			continue
		}
		s.TherapyMinutes.Valid = true
		s.TherapyMinutes.Value = minutes
		if ahiReason != "" {
			s.RecalculatedAHI.Reason = ahiReason
			continue
		}
		s.RecalculatedAHI.Valid = true
		s.RecalculatedAHI.Value = 60 * float64(ahiEventCount(s.EventCounts)) / minutes
	}
	slices.SortStableFunc(result.Sessions, func(a, b Session) int {
		if c := a.Start.asTime().Compare(b.Start.asTime()); c != 0 {
			return c
		}
		return sessionDayCompare(a.Parts[0].Day, b.Parts[0].Day)
	})
	return result, nil
}
