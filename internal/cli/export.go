package cli

import (
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	hd "github.com/IvanSaratov/micommed-reader"
)

// Указатель сохраняет различие между измеренным нулём и недоступным значением в обоих форматах.
type measure struct {
	Value  *float64   `json:"value"`
	Reason hd.Code    `json:"reason,omitempty"`
	Origin hd.FieldID `json:"origin,omitempty"`
}

func metric(m hd.Metric) measure {
	v := measure{Origin: m.Origin, Reason: m.Reason}
	if m.Valid {
		v.Value = &m.Value
	}
	return v
}
func number(n *float64) string {
	if n == nil {
		return ""
	}
	return strconv.FormatFloat(*n, 'g', -1, 64)
}

type periodData struct {
	From     string  `json:"from"`
	Through  string  `json:"through"`
	Included int     `json:"included_days"`
	Count    int     `json:"counted_events"`
	Minutes  measure `json:"therapy_minutes"`
	AHI      measure `json:"recalculated_ahi"`
}

func periodValues(p hd.PeriodSummary) periodData {
	return periodData{dateText(p.From), dateText(p.Through), p.IncludedDays, p.CountedEvents, metric(p.TherapyMinutes), metric(p.RecalculatedAHI)}
}

type sourceData struct {
	ID     string `json:"source_id"`
	Date   string `json:"date"`
	Label  string `json:"label"`
	SHA    string `json:"sha256,omitempty"`
	Status string `json:"status"`
}

func (r *report) sources() []sourceData {
	list := make([]sourceData, 0, len(r.records))
	for _, e := range r.records {
		s := sourceData{ID: r.id, Date: e.date, Label: e.label, Status: "ok"}
		if e.day != nil {
			s.SHA = fmt.Sprintf("%x", e.day.SHA256)
		} else {
			s.Status = e.issue.Code
		}
		list = append(list, s)
	}
	return list
}

type field struct {
	ID       hd.FieldID `json:"field"`
	Unit     string     `json:"unit"`
	Scale    float64    `json:"scale"`
	Evidence string     `json:"meaning_evidence"`
}

func fieldValues(id hd.FieldID) field {
	f, _ := hd.DescribeField(id)
	return field{id, f.Unit, f.Scale, f.MeaningEvidence}
}

type storedField struct {
	field
	measure
	RawBits string `json:"raw_bits,omitempty"`
}
type sample struct {
	Index  int    `json:"index"`
	Minute uint16 `json:"minute"`
	Time   string `json:"device_time,omitempty"`
}

func at(d *hd.Day, i int, m uint16) sample {
	s := sample{Index: i, Minute: m}
	if t, ok := d.ClockAt(m); ok {
		s.Time = clockText(t)
	}
	return s
}

type pressure struct {
	sample
	First  uint16 `json:"first_raw"`
	Second uint16 `json:"second_raw"`
}
type point struct {
	sample
	Raw uint16 `json:"raw"`
}
type channel struct {
	field
	Points []point `json:"points"`
}
type event struct {
	sample
	Code     uint8  `json:"code"`
	Label    string `json:"label,omitempty"`
	Evidence string `json:"meaning_evidence"`
}
type dayData struct {
	ID             string        `json:"source_id"`
	Date           string        `json:"date"`
	SHA            string        `json:"sha256"`
	Start          string        `json:"start_device_time"`
	End            string        `json:"end_device_time"`
	Minutes        measure       `json:"therapy_minutes"`
	StoredAHI      measure       `json:"stored_ahi"`
	AHI            measure       `json:"recalculated_ahi"`
	PressureFields [2]field      `json:"pressure_fields"`
	Pressure       []pressure    `json:"pressure"`
	Channels       []channel     `json:"channels"`
	Events         []event       `json:"events"`
	Stored         []storedField `json:"stored"`
}

func dayValues(d *hd.Day) dayData {
	stats := d.Stats()
	v := dayData{ID: d.Source.SourceID, Date: dateText(d.Source.ReportDate), SHA: fmt.Sprintf("%x", d.SHA256),
		Start: clockText(d.Start), End: clockText(d.End), Minutes: metric(stats.TherapyMinutes),
		StoredAHI: metric(stats.StoredAHI), AHI: metric(stats.RecalculatedAHI),
		PressureFields: [2]field{fieldValues("pressure_1"), fieldValues("pressure_2")},
		Pressure:       make([]pressure, 0, len(d.Pressure)), Events: make([]event, 0, len(d.Events))}
	for i, p := range d.Pressure {
		v.Pressure = append(v.Pressure, pressure{at(d, i, p.Minute), p.FirstRaw, p.SecondRaw})
	}
	for _, ch := range d.Channels {
		c := channel{fieldValues(ch.ID), make([]point, 0, len(ch.Points))}
		for i, p := range ch.Points {
			c.Points = append(c.Points, point{at(d, i, p.Minute), p.Raw})
		}
		v.Channels = append(v.Channels, c)
	}
	for i, e := range d.Events {
		info, ok := hd.DescribeEvent(e.Code)
		if !ok {
			info.MeaningEvidence = "U"
		}
		v.Events = append(v.Events, event{at(d, i, e.Minute), e.Code, info.Label, info.MeaningEvidence})
	}
	for _, id := range slices.Sorted(maps.Keys(d.Stored)) {
		s := d.Stored[id]
		n := float64(s.Value)
		m := measure{Value: &n}
		if math.IsNaN(n) || math.IsInf(n, 0) {
			m = measure{Reason: hd.NonFiniteSummary}
		}
		v.Stored = append(v.Stored, storedField{fieldValues(id), m, fmt.Sprintf("%08x", s.Bits)})
	}
	for _, code := range slices.Sorted(maps.Keys(d.Counters)) {
		n := float64(d.Counters[code])
		id := hd.FieldID(fmt.Sprintf("counter_%d", code))
		v.Stored = append(v.Stored, storedField{fieldValues(id), measure{Value: &n}, ""})
	}
	return v
}

// Имя назначения появляется только после успешной записи и закрытия файла.
func publishJSON(dest string, write func(io.Writer) error) error {
	f, err := os.CreateTemp(filepath.Dir(dest), ".healdata-json-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = write(f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return publishNew(f.Name(), dest)
}
