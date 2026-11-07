package healdata_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/IvanSaratov/micommed-reader"
)

func FuzzDecodeDay(f *testing.F) {
	seeds, err := filepath.Glob("research/fixtures/*.bin")
	if err != nil || len(seeds) == 0 {
		f.Fatal("синтетические файлы недоступны", err)
	}
	for _, path := range seeds {
		b, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := healdata.DecodeDay(b, healdata.Source{ReportDate: healdata.Date{Year: 2001, Month: 2, Day: 3}})
		if err != nil {
			if d != nil {
				t.Fatal("запись при структурной ошибке")
			}
			return
		}
		if d == nil || len(b) != 9312 || len(d.Pressure) > 400 || len(d.Events) > 150 {
			t.Fatal("нарушены границы профиля")
		}
		for _, ch := range d.Channels {
			if len(ch.Points) > 290 {
				t.Fatal("переполнение канала")
			}
			for _, p := range ch.Points {
				if p.Minute >= 1440 {
					t.Fatal("недопустимая минута")
				}
			}
		}
		stats := d.Stats()
		if stats.RecalculatedAHI.Valid && !stats.TherapyMinutes.Valid {
			t.Fatal("индекс без знаменателя")
		}
	})
}
