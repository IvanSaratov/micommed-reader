package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func invoke(t *testing.T, want int, args ...string) string {
	t.Helper()
	var out, stderr bytes.Buffer
	code := Run(context.Background(), append([]string{"healdata"}, args...), &out, &stderr)
	if code != want || (want != 0 && stderr.Len() == 0) {
		t.Fatalf("%v: код %d, stderr %q, stdout %.300q", args, code, stderr.String(), out.String())
	}
	return out.String()
}
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "research", "fixtures", name+".bin"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func put(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

type member struct {
	name string
	data []byte
}

func zipInput(t *testing.T, members ...member) string {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, m := range members {
		w, err := z.CreateHeader(&zip.FileHeader{Name: m.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(m.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return put(t, t.TempDir(), "HealData.ZIP", b.Bytes())
}

func TestCommandLine(t *testing.T) {
	for _, args := range [][]string{{"inspect"}, {"inspect", "a", "b"}, {"summary", "--from", "2001-02-30", "."}, {"summary", "--from", "2001-02-05", "--to", "2001-02-04", "."}, {"export", "csv", "."}, {"summary", "--wat", "."}, {"wat"}, {"export", "wat"}} {
		invoke(t, 2, args...)
	}
	help := invoke(t, 0, "export", "--help")
	if !strings.Contains(help, "json") || !strings.Contains(help, "csv") {
		t.Fatal(help)
	}
	dir := t.TempDir()
	raw := fixture(t, "clock")
	p := put(t, dir, "20010203", raw)
	invoke(t, 2, "inspect", "--date", "2001-02-04", p)
	invoke(t, 2, "inspect", "--date", "2001-02-03", dir)
	invoke(t, 2, "inspect", put(t, t.TempDir(), "clock.bin", raw))
	invoke(t, 1, "inspect", t.TempDir())
	invoke(t, 1, "summary", filepath.Join(dir, "missing"))
	for _, path := range []string{p, dir, zipInput(t, member{"HealData/20010203", raw}, member{"HealData/ID.txt", []byte("id")})} {
		out := invoke(t, 0, "inspect", path)
		if !strings.Contains(out, "2001-02-03") || !strings.Contains(out, "HEALDATA-9312") {
			t.Fatal(out)
		}
	}
}

type testMetric struct {
	Value  *float64
	Reason string
}
type testDocument struct {
	Schema int `json:"schema_version"`
	Days   []struct {
		Date      string
		SHA       string     `json:"sha256"`
		StoredAHI testMetric `json:"stored_ahi"`
		Events    []struct{ Code, Minute, Index int }
	}
	Sources []struct{ Label string }
	Period  struct {
		Days    int        `json:"included_days"`
		Count   int        `json:"counted_events"`
		Minutes testMetric `json:"therapy_minutes"`
		AHI     testMetric `json:"recalculated_ahi"`
	}
	Diagnostics []struct{ Code, Date, Through string }
}

func exported(t *testing.T, code int, args ...string) testDocument {
	t.Helper()
	var doc testDocument
	raw := invoke(t, code, append([]string{"export", "json"}, args...)...)
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Schema != 1 {
		t.Fatalf("schema %d", doc.Schema)
	}
	return doc
}
func TestPeriodAndVersions(t *testing.T) {
	a := member{"20010203", fixture(t, "clock")}
	b := member{"20010204", fixture(t, "next-day")}
	p := zipInput(t, a, b, a)
	doc := exported(t, 0, p)
	if len(doc.Sources) != 3 || len(doc.Days) != 2 || doc.Period.Days != 2 || doc.Period.Count != 1 || *doc.Period.Minutes.Value != 31 || *doc.Period.AHI.Value != 60.0/31 || doc.Sources[0].Label == doc.Sources[1].Label {
		t.Fatalf("сводка %+v", doc)
	}
	if e := doc.Days[0].Events; len(e) != 3 || e[0].Minute != 900 || e[1].Minute != 900 || e[1].Code != 238 || e[1].Index != 1 {
		t.Fatal(e)
	}
	if !strings.Contains(invoke(t, 0, "summary", p), "31") {
		t.Fatal("минуты отсутствуют")
	}
	if d := exported(t, 0, "--from", "2001-02-04", "--to", "2001-02-04", p); len(d.Days) != 1 || d.Days[0].Date != "2001-02-04" {
		t.Fatal(d)
	}
	if d := exported(t, 0, "--from", "2001-03-01", p); len(d.Days) != 0 || d.Period.AHI.Value != nil {
		t.Fatal(d)
	}
	if d := exported(t, 3, zipInput(t, a, b, member{a.name, fixture(t, "minimum")})); len(d.Days) != 3 || d.Period.Days != 1 {
		t.Fatal(d)
	}
	if d := exported(t, 3, zipInput(t, member{"20010230", a.data}, member{"20010204", fixture(t, "truncated")})); len(d.Days) != 0 || len(d.Diagnostics) < 2 {
		t.Fatal(d)
	}
}
func TestCSVAndNoClobber(t *testing.T) {
	p := put(t, t.TempDir(), "20010203", fixture(t, "nan"))
	doc := exported(t, 0, p)
	if doc.Days[0].StoredAHI.Value != nil || doc.Days[0].StoredAHI.Reason != "non_finite_summary" {
		t.Fatal(doc)
	}
	dest := filepath.Join(t.TempDir(), "csv")
	invoke(t, 0, "export", "csv", "-o", dest, p)
	invoke(t, 1, "export", "csv", "-o", dest, p)
	for _, table := range []string{"sources", "days", "pressure", "channels", "events", "stored", "period", "diagnostics"} {
		data, err := os.ReadFile(filepath.Join(dest, table+".csv"))
		if err != nil {
			t.Fatal(err)
		}
		rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
		if err != nil || len(rows) < 1 {
			t.Fatal(table, err)
		}
		if table == "stored" && (!strings.Contains(string(data), "non_finite_summary") || !strings.Contains(string(data), "raw_bits")) {
			t.Fatal("потеря исходного значения")
		}
	}
	path := filepath.Join(t.TempDir(), "result.json")
	invoke(t, 0, "export", "json", "-o", path, p)
	invoke(t, 1, "export", "json", "-o", path, p)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != invoke(t, 0, "export", "json", p) {
		t.Fatal("нестабильный JSON", err)
	}
	original, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(original, fixture(t, "nan")) {
		t.Fatal("вход изменился")
	}
}
