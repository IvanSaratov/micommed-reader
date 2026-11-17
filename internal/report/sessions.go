package report

import (
	"errors"
	"fmt"
	hd "github.com/IvanSaratov/micommed-reader/pkg/healdata"
	"time"
)

type sessionSummary struct {
	ID             int           `json:"id"`
	Start          string        `json:"start_device_time"`
	End            string        `json:"end_device_time"`
	SpanMinutes    int           `json:"span_minutes"`
	TherapyMinutes measure       `json:"therapy_minutes"`
	AHI            measure       `json:"recalculated_ahi"`
	EventCounts    map[uint8]int `json:"event_counts"`
}
type SessionIndex struct {
	GapMinutes int              `json:"gap_minutes"`
	Sessions   []sessionSummary `json:"sessions"`
	Issues     []Problem        `json:"issues"`
}
type SessionReport struct {
	set   hd.SessionSet
	index SessionIndex
}

func (r *Report) Sessions(gap int) (*SessionReport, error) {
	set, err := hd.GroupSessions(r.days, gap)
	if err != nil {
		return nil, err
	}
	out := &SessionReport{set: set, index: SessionIndex{GapMinutes: gap, Sessions: []sessionSummary{}, Issues: []Problem{}}}
	for i, s := range set.Sessions {
		out.index.Sessions = append(out.index.Sessions, sessionSummary{i, clockText(s.Start), clockText(s.End), s.SpanMinutes, metric(s.TherapyMinutes), metric(s.RecalculatedAHI), s.EventCounts})
	}
	for _, x := range set.Excluded {
		out.index.Issues = append(out.index.Issues, Problem{Kind: "exclusion", Code: string(x.Reason), Date: dateText(x.Day.Source.ReportDate), Label: x.Day.Source.Label})
	}
	for _, x := range r.records {
		if x.issue != nil {
			out.index.Issues = append(out.index.Issues, *x.issue)
		}
	}
	return out, nil
}
func (s *SessionReport) Index() SessionIndex { return s.index }

type sessionSample struct {
	Index       int    `json:"index"`
	Minute      int    `json:"minute"`
	Time        string `json:"device_time"`
	SourceIndex int    `json:"source_index"`
	SourceDate  string `json:"source_date"`
	SHA         string `json:"source_sha256"`
	BreakBefore bool   `json:"break_before,omitempty"`
}
type sessionPressure struct {
	sessionSample
	First  uint16 `json:"first_raw"`
	Second uint16 `json:"second_raw"`
}
type sessionPoint struct {
	sessionSample
	Raw uint16 `json:"raw"`
}
type sessionChannel struct {
	field
	Points []sessionPoint `json:"points"`
}
type sessionEvent struct {
	sessionSample
	Code     uint8  `json:"code"`
	Label    string `json:"label,omitempty"`
	Evidence string `json:"meaning_evidence"`
}
type SessionView struct {
	sessionSummary
	PressureFields [2]field                    `json:"pressure_fields"`
	Pressure       []sessionPressure           `json:"pressure"`
	Channels       []sessionChannel            `json:"channels"`
	Events         []sessionEvent              `json:"events"`
	Fields         map[hd.FieldID]hd.FieldInfo `json:"fields"`
	Diagnostics    []Problem                   `json:"diagnostics"`
}

func deviceMoment(t hd.DeviceTime) time.Time {
	return time.Date(t.Year, time.Month(t.Month), t.Day, t.Hour, t.Minute, t.Second, 0, time.UTC)
}
func (s *SessionReport) View(id int) (SessionView, error) {
	if id < 0 || id >= len(s.set.Sessions) {
		return SessionView{}, errors.New("сеанс не найден")
	}
	session := s.set.Sessions[id]
	view := SessionView{sessionSummary: s.index.Sessions[id], PressureFields: [2]field{fieldValues("pressure_1"), fieldValues("pressure_2")}, Pressure: []sessionPressure{}, Channels: []sessionChannel{}, Events: []sessionEvent{}, Fields: map[hd.FieldID]hd.FieldInfo{}, Diagnostics: []Problem{}}
	for i := 1; i <= 5; i++ {
		view.Channels = append(view.Channels, sessionChannel{fieldValues(hd.FieldID(fmt.Sprintf("channel_%d", i))), []sessionPoint{}})
	}
	for _, id := range []hd.FieldID{"pressure_1", "pressure_2", "channel_1", "channel_2", "channel_3", "channel_4", "channel_5"} {
		view.Fields[id], _ = hd.DescribeField(id)
	}
	for _, part := range session.Parts {
		d := part.Day
		includes := func(m uint16) bool { return m >= part.FirstMinute && m <= part.LastMinute }
		// Новая ось только для отображения. Исходный индекс и происхождение передаются отдельно.
		at := func(index, original int, m uint16) sessionSample {
			stamp, _ := d.ClockAt(m)
			return sessionSample{Index: index, Minute: int(deviceMoment(stamp).Sub(deviceMoment(session.Start)) / time.Minute), Time: clockText(stamp), SourceIndex: original, SourceDate: dateText(d.Source.ReportDate), SHA: fmt.Sprintf("%x", d.SHA256)}
		}
		for i, p := range d.Pressure {
			if includes(p.Minute) {
				view.Pressure = append(view.Pressure, sessionPressure{at(len(view.Pressure), i, p.Minute), p.FirstRaw, p.SecondRaw})
			}
		}
		for c, ch := range d.Channels {
			for i, p := range ch.Points {
				if includes(p.Minute) {
					points := &view.Channels[c].Points
					sample := at(len(*points), i, p.Minute)
					if len(*points) > 0 && sample.Minute-(*points)[len(*points)-1].Minute > 10 {
						sample.BreakBefore = true
					}
					*points = append(*points, sessionPoint{sample, p.Raw})
				}
			}
		}
		for i, e := range d.Events {
			if includes(e.Minute) {
				info, ok := hd.DescribeEvent(e.Code)
				if !ok {
					info.MeaningEvidence = "U"
				}
				view.Events = append(view.Events, sessionEvent{at(len(view.Events), i, e.Minute), e.Code, info.Label, info.MeaningEvidence})
			}
		}
	}
	return view, nil
}
