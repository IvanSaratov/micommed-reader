package healdata

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestStored(t *testing.T) {
	d := mustDay(t, "nan")
	ahi, ok := d.Stored["summary_0dc8"]
	if !ok || !math.IsNaN(float64(ahi.Value)) || ahi.Bits != binary.LittleEndian.Uint32(d.raw[0xdc8:]) || !d.hasDiagnostic(NonFiniteSummary) {
		t.Fatal("NaN bits or diagnostics")
	}
	b := fixture(t, "clock")
	binary.LittleEndian.PutUint32(b[0xdb8:], 0x3fc00000)
	binary.LittleEndian.PutUint32(b[0xe38:], 0x3e800000)
	d, err := DecodeDay(b, testSource())
	if err != nil || len(d.Stored) != 23 || d.Stored["summary_0db8"].Value != 1.5 || d.Stored["summary_0e38"].Value != 0.25 {
		t.Fatal("stored groups", err)
	}
	for _, duration := range []float32{0, -1, float32(math.Inf(1)), 9999} {
		binary.LittleEndian.PutUint32(b[0xddc:], math.Float32bits(duration))
		d, err = DecodeDay(b, testSource())
		if err != nil || !d.hasDiagnostic(InvalidDuration) {
			t.Fatalf("duration=%g, err=%v", duration, err)
		}
	}
}

func TestFieldInfo(t *testing.T) {
	p, ok := DescribeField("pressure_1")
	if !ok || p.Offset != 8 || p.Scale != 0.1 || p.MeaningEvidence != "C+H" || p.StructureEvidence != "S" || p.Source == "" {
		t.Fatal(p)
	}
	fifth, ok := DescribeField("channel_5")
	if !ok || fifth.Offset+fifth.Stride*289 != 0x1f73 || fifth.TimeOffset+fifth.TimeStride*289 != 0x21b6 || fifth.Unit != "" || fifth.MeaningEvidence != "U" {
		t.Fatal(fifth)
	}
	if _, ok := DescribeField("summary_ffff"); ok {
		t.Fatal("invented field")
	}
	info, ok := DescribeField("summary_0dfc")
	if !ok || info.Offset != 0xdfc || info.RawType != "f32" || info.Unit == "" {
		t.Fatal(info)
	}
}

func TestEventInfo(t *testing.T) {
	if _, ok := DescribeEvent(238); ok {
		t.Fatal("invented meaning")
	}
	for _, c := range []uint8{100, 101, 102, 103, 104, 150, 151} {
		info, ok := DescribeEvent(c)
		if !ok || info.Label == "" || info.MeaningEvidence == "" || info.Source == "" {
			t.Error(c, info)
		}
	}
}

func TestFieldInfoUnknownPurpose(t *testing.T) {
	duration, _ := DescribeField("summary_0de0")
	ratio, _ := DescribeField("summary_0e38")
	if duration.MeaningEvidence != "U" || ratio.Unit != "" {
		t.Fatal("назначение второй длительности и единицы отношения ещё не подтверждены")
	}
}
