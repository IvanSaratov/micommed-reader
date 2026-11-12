package report

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"testing"
)

func TestSharedZIPAndDay(t *testing.T) {
	raw, err := os.ReadFile("../../research/fixtures/clock.bin")
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for range 2 {
		f, err := z.Create("20010203")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	in := NewInput("")
	if err = in.ReadZIP(context.Background(), bytes.NewReader(b.Bytes()), int64(b.Len()), nil); err != nil {
		t.Fatal(err)
	}
	r, err := Build(in, "", "")
	if err != nil {
		t.Fatal(err)
	}
	index := r.Index()
	if len(index.Sources) != 2 || index.Period.Included != 1 {
		t.Fatal(index)
	}
	d, err := r.Day("2001-02-03", index.Sources[0].SHA)
	if err != nil || len(d.Events) != 3 || d.Events[1].Code != 238 || *d.Minutes.Value != 30 {
		t.Fatal(err, d)
	}
	var out bytes.Buffer
	if err = r.WriteCSVZip(&out); err != nil {
		t.Fatal(err)
	}
	zipOut, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil || len(zipOut.File) != 8 {
		t.Fatal(err)
	}
	for _, f := range zipOut.File {
		rd, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.ReadAll(rd)
		rd.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestActualReadLimit(t *testing.T) {
	in := Input{id: "test", consumed: MaxBytes - 5}
	if err := in.ReadDay("notes", "", func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader([]byte("123456"))), nil }); err == nil {
		t.Fatal("фактический бюджет не проверен")
	}
}

func TestSessionViewPreservesPointsAcrossFiles(t *testing.T) {
	in := NewInput("synthetic")
	for _, f := range []struct{ name, fixture string }{{"20010203", "clock"}, {"20010204", "next-day"}} {
		raw, err := os.ReadFile("../../research/fixtures/" + f.fixture + ".bin")
		if err != nil {
			t.Fatal(err)
		}
		err = in.Add(f.name, f.name, 0, uint64(len(raw)), func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil })
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := Build(in, "", "")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := r.Sessions(60)
	if err != nil || len(sessions.Index().Sessions) != 2 {
		t.Fatal(err, sessions)
	}
	first, err := sessions.View(0)
	if err != nil || len(first.Pressure) != 4 || first.Pressure[2].Minute != 5 || first.Pressure[3].SourceIndex != 3 || first.Events[1].Code != 238 {
		t.Fatal(err, first)
	}
	next, err := sessions.View(1)
	if err != nil || len(next.Pressure) != 2 || next.Pressure[1].Minute != 1 || next.Pressure[1].SourceDate != "2001-02-04" {
		t.Fatal(err, next)
	}
	if _, err = sessions.View(-1); err == nil {
		t.Fatal("отрицательный индекс")
	}
	if _, err = sessions.View(2); err == nil {
		t.Fatal("индекс вне набора")
	}
}
