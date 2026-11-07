package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSourceEquivalenceAndLimits(t *testing.T) {
	raw := fixture(t, "clock")
	dir := t.TempDir()
	file := put(t, dir, "rec/20010203", raw)
	z := zipInput(t, member{"rec/20010203", raw})
	for _, path := range []string{file, dir, z} {
		doc := exported(t, 0, path)
		if len(doc.Days) != 1 || doc.Days[0].SHA != fmt.Sprintf("%x", sha256.Sum256(raw)) || doc.Period.Minutes.Value == nil || *doc.Period.Minutes.Value != 30 {
			t.Fatal("несовпадение источников")
		}
	}
	put(t, dir, "one/ID.txt", nil)
	put(t, dir, "two/ID.txt", nil)
	invoke(t, 1, "inspect", dir)
	crcName := zipInput(t, member{"20010203", raw}, member{"20010204", fixture(t, "next-day")})
	b, err := os.ReadFile(crcName)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(b, raw)
	if i < 0 {
		t.Fatal("не найден блок данных")
	}
	b[i] ^= 1
	damaged := exported(t, 3, put(t, t.TempDir(), "crc.zip", b))
	if len(damaged.Days) != 1 || damaged.Days[0].Date != "2001-02-04" || !slices.ContainsFunc(damaged.Diagnostics, func(p struct{ Code, Date, Through string }) bool { return p.Code == "read_error" }) {
		t.Fatal(damaged)
	}
	entries := make([]member, 4097)
	for i := range entries {
		entries[i].name = fmt.Sprintf("note%d", i)
	}
	invoke(t, 1, "inspect", zipInput(t, entries...))
	large := put(t, t.TempDir(), "notes", nil)
	if err := os.Truncate(large, maxBytes+1); err != nil {
		t.Fatal(err)
	}
	invoke(t, 1, "inspect", filepath.Dir(large))
	in := input{id: "zip:test", consumed: maxBytes - 5}
	if err := in.read("notes", "", func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("123456")), nil }); err == nil {
		t.Fatal("превышение фактического бюджета")
	}
}

func TestCSVPreservesIndependentAxes(t *testing.T) {
	raw := fixture(t, "clock")
	binary.LittleEndian.PutUint16(raw[0x1f74+4:], 898)
	input := put(t, t.TempDir(), "20010203", raw)
	dir := t.TempDir()
	first, second := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	for _, p := range []string{first, second} {
		invoke(t, 0, "export", "csv", "-o", p, input)
	}
	files, err := os.ReadDir(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		a, err := os.ReadFile(filepath.Join(first, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(second, file.Name()))
		if err != nil || !bytes.Equal(a, b) {
			t.Fatal(file.Name(), err)
		}
		if file.Name() != "channels.csv" {
			continue
		}
		rows, err := csv.NewReader(bytes.NewReader(a)).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]string{}
		index, channel, minute := slices.Index(rows[0], "index"), slices.Index(rows[0], "channel_id"), slices.Index(rows[0], "minute")
		for _, row := range rows[1:] {
			if row[index] == "2" {
				seen[row[channel]] = row[minute]
			}
		}
		if seen["channel_5"] != "898" || seen["channel_4"] != "900" {
			t.Fatal(seen)
		}
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestCoverageAndWriteFailure(t *testing.T) {
	z := zipInput(t, member{"20010203", fixture(t, "clock")}, member{"20010205", fixture(t, "truncated")}, member{"20010230", nil})
	doc := exported(t, 3, "--from", "2001-02-03", "--to", "2001-02-06", z)
	var gaps []string
	for _, p := range doc.Diagnostics {
		if p.Code == "missing_dates" {
			gaps = append(gaps, p.Date+"/"+p.Through)
		}
	}
	if !slices.Equal(gaps, []string{"2001-02-04/2001-02-04", "2001-02-06/2001-02-06"}) {
		t.Fatal(gaps)
	}
	filtered := exported(t, 3, "--from", "2001-02-04", "--to", "2001-02-04", z)
	if len(filtered.Sources) != 1 || len(filtered.Days) != 0 {
		t.Fatal(filtered)
	}
	p := filepath.Join(t.TempDir(), "partial.json")
	failed := errors.New("сбой записи")
	if err := publishJSON(p, func(w io.Writer) error { fmt.Fprint(w, "начало"); return failed }); !errors.Is(err, failed) {
		t.Fatal(err)
	}
	files, err := os.ReadDir(filepath.Dir(p))
	if err != nil || len(files) != 0 {
		t.Fatal(files, err)
	}
	if Run(t.Context(), []string{"healdata", "export", "json", z}, failedWriter{}, io.Discard) != 1 {
		t.Fatal("ошибка вывода должна давать код 1")
	}
}

func TestReadableDiagnostics(t *testing.T) {
	p := put(t, t.TempDir(), "20010203", fixture(t, "truncated"))
	if out := invoke(t, 3, "inspect", p); !strings.Contains(out, "длина") {
		t.Fatal(out)
	}
	good := put(t, t.TempDir(), "20010203", fixture(t, "clock"))
	if out := invoke(t, 0, "inspect", good); strings.Count(out, "sample_gap") > 1 {
		t.Fatal("повтор предупреждения", out)
	}
	if out := invoke(t, 0, "summary", good); strings.Contains(out, "sample_gap") {
		t.Fatal("дублирование подробной диагностики в сводке")
	}
}
