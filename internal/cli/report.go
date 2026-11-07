package cli

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	hd "github.com/IvanSaratov/micommed-reader"
)

type report struct {
	id                                     string
	records                                []record
	days                                   []*hd.Day
	period                                 hd.PeriodSummary
	diagnostics                            []problem
	service, broken, duplicates, conflicts int
	partial                                bool
	profile, version                       string
}

func dateText(d hd.Date) string {
	if d.Year == 0 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}
func clockText(t hd.DeviceTime) string {
	return fmt.Sprintf("%sT%02d:%02d:%02d", dateText(t.Date), t.Hour, t.Minute, t.Second)
}

func makeReport(in *input, from, to string) (*report, error) {
	r := &report{id: in.id, version: hd.DecoderVersion, service: in.services, diagnostics: []problem{}}
	slices.SortStableFunc(in.records, func(a, b record) int {
		if d := cmp.Compare(a.date, b.date); d != 0 {
			return d
		}
		return cmp.Compare(a.label, b.label)
	})
	var first, last string
	for _, entry := range in.records {
		if entry.day != nil {
			r.profile = entry.day.Profile
		}
		if entry.date != "" {
			if first == "" {
				first = entry.date
			}
			last = entry.date
		}
	}
	if from == "" {
		from = first
		if to != "" && (from == "" || to < from) {
			from = to
		}
	}
	if to == "" {
		to = max(last, from)
	}
	var all []*hd.Day
	// Для рядов нужен один экземпляр версии, для SummarizePeriod — каждое её происхождение.
	type key struct {
		date   string
		digest [32]byte
	}
	seen := map[key]bool{}
	damaged := map[string]bool{}
	for _, entry := range in.records {
		if entry.date != "" && (entry.date < from || entry.date > to) {
			continue
		}
		r.records = append(r.records, entry)
		if entry.issue != nil {
			r.diagnostics = append(r.diagnostics, *entry.issue)
			r.broken++
			damaged[entry.date] = true
			continue
		}
		d := entry.day
		all = append(all, d)
		k := key{entry.date, d.SHA256}
		if seen[k] {
			continue
		}
		seen[k] = true
		r.days = append(r.days, d)
		for _, diag := range d.Diagnostics {
			p := diagnostic(diag)
			p.Date, p.Label = entry.date, entry.label
			r.diagnostics = append(r.diagnostics, p)
		}
	}
	if from != "" {
		period, err := hd.SummarizePeriod(all, asDate(from), asDate(to))
		if err != nil {
			return nil, err
		}
		r.period = period
	} else {
		r.period.TherapyMinutes.Reason = hd.NoUsableDays
		r.period.RecalculatedAHI.Reason = hd.NoUsableDays
	}
	for _, dup := range r.period.Duplicates {
		r.duplicates += len(dup.Entries) - 1
		r.diagnostics = append(r.diagnostics, problem{Kind: "duplicate", Code: string(dup.Reason), Date: dateText(dup.Date), Label: dup.Entries[0].Source.Label})
	}
	r.conflicts = len(r.period.Conflicts)
	for _, ex := range r.period.Exclusions {
		r.diagnostics = append(r.diagnostics, problem{Kind: "exclusion", Code: string(ex.Reason), Date: dateText(ex.Date), Detail: "дата исключена из сводки"})
	}
	// Ядро видит только декодированные дни. Повреждённый файл уже имеет свою причину
	// в диагностике, поэтому исключаем его дату из интервалов отсутствующих файлов.
	for _, gap := range r.period.MissingDates {
		start, end := dateText(gap.From), dateText(gap.Through)
		for _, entry := range r.records {
			d := entry.date
			if !damaged[d] || d < start || d > end {
				continue
			}
			t, _ := parseDate(d)
			if start < d {
				r.addGap(start, t.AddDate(0, 0, -1).Format(time.DateOnly))
			}
			if d == end {
				start = ""
				break
			}
			start = t.AddDate(0, 0, 1).Format(time.DateOnly)
		}
		if start != "" {
			r.addGap(start, end)
		}
	}
	r.partial = r.broken > 0 || len(r.period.Exclusions) > 0
	return r, nil
}
func (r *report) addGap(from, to string) {
	r.diagnostics = append(r.diagnostics, problem{Kind: "gap", Code: "missing_dates", Date: from, Through: to})
}
func metricText(m hd.Metric) string {
	if !m.Valid {
		return "н/д (" + string(m.Reason) + ")"
	}
	return strconv.FormatFloat(m.Value, 'f', -1, 64)
}

func (r *report) writeText(w io.Writer, cmd string) error {
	var b bytes.Buffer
	switch cmd {
	case "inspect":
		for _, e := range r.records {
			fmt.Fprintf(&b, "%s  %q", e.date, e.label)
			if e.issue != nil {
				fmt.Fprintf(&b, "  %s\n", describeProblem(*e.issue))
				continue
			}
			d := e.day
			n := 0
			for _, s := range d.Channels {
				n += len(s.Points)
			}
			fmt.Fprintf(&b, "  точек давления %d, точек каналов %d, событий %d", len(d.Pressure), n, len(d.Events))
			counts := map[hd.Code]int{}
			for _, diag := range d.Diagnostics {
				counts[diag.Code]++
			}
			var codes []string
			for _, code := range slices.Sorted(maps.Keys(counts)) {
				codes = append(codes, fmt.Sprintf("%s (%s × %d)", reasonText(string(code)), code, counts[code]))
			}
			if len(codes) > 0 {
				fmt.Fprint(&b, "  ", strings.Join(codes, ", "))
			}
			fmt.Fprintln(&b)
			if len(r.days) == 1 {
				fmt.Fprintf(&b, "  %s  %s … %s\n", d.Profile, clockText(d.Start), clockText(d.End))
			}
		}
		fmt.Fprintf(&b, "Прочитано %d, служебных %d, повреждённых %d, повторов %d, конфликтных дат %d\n", len(r.records)-r.broken, r.service, r.broken, r.duplicates, r.conflicts)
	case "summary":
		fmt.Fprintln(&b, "Дата        Минуты терапии   AHI аппарата   AHI по событиям   Файл")
		for _, d := range r.days {
			s := d.Stats()
			fmt.Fprintf(&b, "%s  %s  %s  %s  %q\n", dateText(d.Source.ReportDate), metricText(s.TherapyMinutes), metricText(s.StoredAHI), metricText(s.RecalculatedAHI), d.Source.Label)
		}
		p := r.period
		fmt.Fprintf(&b, "Период %s … %s\nУчтено дней %d, минут терапии %s, событий для AHI %d, AHI по событиям %s\n", dateText(p.From), dateText(p.Through), p.IncludedDays, metricText(p.TherapyMinutes), p.CountedEvents, metricText(p.RecalculatedAHI))
	}
	for _, p := range r.diagnostics {
		if p.Code == "missing_dates" {
			fmt.Fprintf(&b, "Нет записей: %s … %s\n", p.Date, p.Through)
			continue
		}
		if p.Kind == "exclusion" || p.Kind == "duplicate" || (p.Kind == "error" && cmd == "summary") {
			fmt.Fprintf(&b, "%s  %s", p.Date, describeProblem(p))
			if p.Label != "" {
				fmt.Fprintf(&b, "  %q", p.Label)
			}
			fmt.Fprintln(&b)
		}
	}
	_, err := b.WriteTo(w)
	return err
}
