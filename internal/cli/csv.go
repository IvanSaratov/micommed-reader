package cli

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type csvTable struct {
	f *os.File
	w *csv.Writer
}
type csvFiles struct {
	tables []csvTable
	err    error
}

func (s *csvFiles) table(dir, name, header string) *csv.Writer {
	if s.err != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, name+".csv"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		s.err = err
		return nil
	}
	w := csv.NewWriter(f)
	s.tables = append(s.tables, csvTable{f, w})
	s.err = w.Write(strings.Split(header, ","))
	return w
}
func (s *csvFiles) close() error {
	for _, t := range s.tables {
		t.w.Flush()
		if err := t.w.Error(); s.err == nil {
			s.err = err
		}
		if err := t.f.Close(); s.err == nil {
			s.err = err
		}
	}
	return s.err
}

func (r *report) writeCSV(dest string) error {
	dest = filepath.Clean(dest)
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".healdata-csv-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = r.csvTables(stage); err != nil {
		return err
	}
	return publishNew(stage, dest)
}

func (r *report) csvTables(dir string) error {
	var files csvFiles
	const key = "source_id,date,sha256,"
	sources := files.table(dir, "sources", key+"label,status")
	days := files.table(dir, "days", key+"start_device_time,end_device_time,therapy_minutes,minutes_reason,stored_ahi,stored_ahi_reason,recalculated_ahi,recalculated_ahi_reason")
	pressure := files.table(dir, "pressure", key+"index,minute,device_time,first_raw,second_raw,first_scale,second_scale,unit,meaning_evidence")
	channels := files.table(dir, "channels", key+"channel_id,index,minute,device_time,raw,scale,unit,meaning_evidence")
	events := files.table(dir, "events", key+"index,minute,device_time,code,label,meaning_evidence")
	stored := files.table(dir, "stored", key+"field,value,reason,raw_bits,unit,meaning_evidence")
	period := files.table(dir, "period", "schema_version,profile,decoder_version,from,through,included_days,counted_events,therapy_minutes,minutes_reason,recalculated_ahi,recalculated_ahi_reason")
	issues := files.table(dir, "diagnostics", "source_id,date,through,label,kind,code,field,offset,index,detail")
	if files.err != nil {
		return files.close()
	}
	for _, s := range r.sources() {
		sources.Write([]string{s.ID, s.Date, s.SHA, s.Label, s.Status})
	}
	for _, day := range r.days {
		d := dayValues(day)
		row := func(w *csv.Writer, cells ...string) {
			if files.err == nil {
				files.err = w.Write(append([]string{d.ID, d.Date, d.SHA}, cells...))
			}
		}
		row(days, d.Start, d.End, number(d.Minutes.Value), string(d.Minutes.Reason), number(d.StoredAHI.Value), string(d.StoredAHI.Reason), number(d.AHI.Value), string(d.AHI.Reason))
		for _, p := range d.Pressure {
			f, g := d.PressureFields[0], d.PressureFields[1]
			row(pressure, strconv.Itoa(p.Index), strconv.Itoa(int(p.Minute)), p.Time, strconv.Itoa(int(p.First)), strconv.Itoa(int(p.Second)), number(&f.Scale), number(&g.Scale), f.Unit, f.Evidence)
		}
		for _, ch := range d.Channels {
			for _, p := range ch.Points {
				row(channels, string(ch.ID), strconv.Itoa(p.Index), strconv.Itoa(int(p.Minute)), p.Time, strconv.Itoa(int(p.Raw)), number(&ch.Scale), ch.Unit, ch.Evidence)
			}
		}
		for _, e := range d.Events {
			row(events, strconv.Itoa(e.Index), strconv.Itoa(int(e.Minute)), e.Time, strconv.Itoa(int(e.Code)), e.Label, e.Evidence)
		}
		for _, s := range d.Stored {
			row(stored, string(s.ID), number(s.Value), string(s.Reason), s.RawBits, s.Unit, s.Evidence)
		}
		if files.err != nil {
			break
		}
	}
	p := periodValues(r.period)
	period.Write([]string{"1", r.profile, r.version, p.From, p.Through, strconv.Itoa(p.Included), strconv.Itoa(p.Count), number(p.Minutes.Value), string(p.Minutes.Reason), number(p.AHI.Value), string(p.AHI.Reason)})
	for _, d := range r.diagnostics {
		issues.Write([]string{r.id, d.Date, d.Through, d.Label, d.Kind, d.Code, string(d.Field), optionalInt(d.Offset), optionalInt(d.Index), d.Detail})
	}
	return files.close()
}
func optionalInt(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}
