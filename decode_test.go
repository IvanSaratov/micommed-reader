package healdata

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("research", "fixtures", name+".bin"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testSource() Source { return Source{ReportDate: Date{2001, 2, 3}, SourceID: "synthetic-A"} }

func mustDay(t testing.TB, name string) *Day {
	t.Helper()
	d, err := DecodeDay(fixture(t, name), testSource())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDecodeEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name   string
		code   Code
		offset int
	}{
		{"truncated", UnsupportedLength, -1}, {"bad-count", CountOutOfRange, 0},
		{"bad-event-count", CountOutOfRange, 0x968}, {"bad-channel-count", CountOutOfRange, 0x1e50},
		{"bad-clock", InvalidMinute, 0x648}, {"bad-date", InvalidHeader, 0xb32}, {"foreign-zero", InvalidHeader, 0xb30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := DecodeDay(fixture(t, tc.name), testSource())
			var de *DecodeError
			if d != nil || !errors.As(err, &de) || de.Code != tc.code || de.Offset != tc.offset {
				t.Fatalf("day=%v, err=%v", d != nil, err)
			}
		})
	}
	d := mustDay(t, "minimum")
	if d.Profile == "" || d.DecoderVersion == "" || d.Source != testSource() {
		t.Fatal("missing provenance")
	}
}

func TestOwnership(t *testing.T) {
	b := fixture(t, "clock")
	hash := sha256.Sum256(b)
	d, err := DecodeDay(b, testSource())
	if err != nil {
		t.Fatal(err)
	}
	b[8]++
	d.RawBytes()[9]++
	if sha256.Sum256(d.RawBytes()) != hash || d.SHA256 != hash {
		t.Fatal("input or result aliases stored bytes")
	}
}
