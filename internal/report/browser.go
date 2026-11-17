package report

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"

	hd "github.com/IvanSaratov/micommed-reader/pkg/healdata"
)

// Index содержит только сведения для навигации. Ряды запрашиваются для одного дня.
type Index struct {
	Sources     []sourceData      `json:"sources"`
	Period      periodData        `json:"period"`
	Diagnostics []Problem         `json:"diagnostics"`
	Partial     bool              `json:"partial"`
	Reasons     map[string]string `json:"reasons"`
}

func (r *Report) Index() Index {
	return Index{r.sources(), periodValues(r.period), r.diagnostics, r.partial, reasons}
}

type DayView struct {
	dayData
	Diagnostics []Problem                   `json:"diagnostics"`
	Fields      map[hd.FieldID]hd.FieldInfo `json:"fields"`
	EventCounts map[uint8]int               `json:"event_counts"`
}

func (r *Report) Day(date, sha string) (DayView, error) {
	for _, d := range r.days {
		if dateText(d.Source.ReportDate) != date || fmt.Sprintf("%x", d.SHA256) != sha {
			continue
		}
		v := DayView{dayData: dayValues(d), Diagnostics: []Problem{}, Fields: map[hd.FieldID]hd.FieldInfo{}, EventCounts: d.Stats().EventCounts}
		for _, p := range r.diagnostics {
			if p.Date == date && (p.Label == "" || p.Label == d.Source.Label) {
				v.Diagnostics = append(v.Diagnostics, p)
			}
		}
		for _, id := range []hd.FieldID{"pressure_1", "pressure_2", "channel_1", "channel_2", "channel_3", "channel_4", "channel_5", "summary_0dc8", "summary_0ddc"} {
			v.Fields[id], _ = hd.DescribeField(id)
		}
		return v, nil
	}
	return DayView{}, errors.New("версия дня не найдена")
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// ZIP формируется по таблицам. В памяти не накапливается несжатая CSV-выгрузка.
func (r *Report) WriteCSVZip(w io.Writer) error {
	z := zip.NewWriter(w)
	for _, table := range []string{"sources", "days", "pressure", "channels", "events", "stored", "period", "diagnostics"} {
		name := table + ".csv"
		dst, err := z.Create(name)
		if err != nil {
			return err
		}
		err = r.WriteCSVTables(func(current string) (io.WriteCloser, error) {
			if current == name {
				return nopWriteCloser{dst}, nil
			}
			return nopWriteCloser{io.Discard}, nil
		})
		if err != nil {
			return err
		}
	}
	return z.Close()
}
