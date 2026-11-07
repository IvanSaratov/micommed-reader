package healdata_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/IvanSaratov/micommed-reader"
)

func TestPublicContract(t *testing.T) {
	const dir = "research/fixtures"
	manifest, err := os.ReadFile(filepath.Join(dir, "expectations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases struct {
		Cases []struct {
			File, Date, Error       string
			Points, Samples, Events int
			FirstRaw                int    `json:"first_raw"`
			LastRaw                 int    `json:"last_raw"`
			LastTime                string `json:"last_time"`
		}
	}
	if err = json.Unmarshal(manifest, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases.Cases) != 14 {
		t.Fatal("ожидаются все 14 исходных примеров")
	}
	// Манифест описывает также отказы Python-прототипа. У Go три из них
	// сохраняют доступные поля и выдают предупреждение.
	fatal := map[string]healdata.Code{"truncated.bin": healdata.UnsupportedLength, "bad-count.bin": healdata.CountOutOfRange,
		"bad-event-count.bin": healdata.CountOutOfRange, "bad-channel-count.bin": healdata.CountOutOfRange,
		"bad-clock.bin": healdata.InvalidMinute, "bad-date.bin": healdata.InvalidHeader, "foreign-zero.bin": healdata.InvalidHeader}
	partial := map[string]healdata.Code{"nan.bin": healdata.NonFiniteSummary, "counter-mismatch.bin": healdata.CounterMismatch, "unobserved-empty.bin": healdata.EmptySeries}
	for _, c := range cases.Cases {
		t.Run(c.File, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, c.File))
			if err != nil {
				t.Fatal(err)
			}
			stamp, err := time.Parse("20060102", c.Date)
			if err != nil {
				t.Fatal(err)
			}
			date := healdata.Date{Year: stamp.Year(), Month: int(stamp.Month()), Day: stamp.Day()}
			day, err := healdata.DecodeDay(raw, healdata.Source{ReportDate: date, SourceID: "fixtures"})
			if code, ok := fatal[c.File]; ok {
				var de *healdata.DecodeError
				if day != nil || !errors.As(err, &de) || de.Code != code {
					t.Fatalf("структурная ошибка: %v", err)
				}
				return
			}
			if err != nil || day == nil {
				t.Fatalf("чтение дня: %v", err)
			}
			if want, ok := partial[c.File]; ok {
				for _, diag := range day.Diagnostics {
					if diag.Code == want {
						return
					}
				}
				t.Fatal("отсутствует ожидаемое предупреждение", want)
			}
			if c.Error != "" || len(day.Pressure) != c.Points || len(day.Events) != c.Events {
				t.Fatal("число точек или событий")
			}
			if int(day.Pressure[0].FirstRaw) != c.FirstRaw || int(day.Pressure[c.Points-1].FirstRaw) != c.LastRaw {
				t.Fatal("границы ряда давления")
			}
			for _, series := range day.Channels {
				if len(series.Points) != c.Samples {
					t.Fatal("число точек канала", series.ID)
				}
				v, ok := day.ClockAt(series.Points[c.Samples-1].Minute)
				got := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d", v.Year, v.Month, v.Day, v.Hour, v.Minute, v.Second)
				if !ok || got != c.LastTime {
					t.Fatal("конечная отметка", got)
				}
			}
		})
	}
}

func ExampleDecodeDay() {
	raw, err := os.ReadFile("research/fixtures/clock.bin")
	if err != nil {
		panic(err)
	}
	day, err := healdata.DecodeDay(raw, healdata.Source{
		ReportDate: healdata.Date{Year: 2001, Month: 2, Day: 3}, SourceID: "synthetic-A",
	})
	if err != nil {
		panic(err)
	}
	stats := day.Stats()
	fmt.Println(day.Pressure[0].FirstRaw, day.Pressure[0].SecondRaw)
	fmt.Println(stats.StoredAHI.Value, stats.RecalculatedAHI.Value)
	// Output:
	// 71 101
	// 2 2
}
