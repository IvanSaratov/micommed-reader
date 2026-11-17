package healdata

import (
	"encoding/binary"
	"math"
)

func (d *Day) readSummaries() {
	d.Stored = make(map[FieldID]StoredFloat, len(summaries))
	for _, f := range summaries {
		id := summaryID(f.offset)
		bits := binary.LittleEndian.Uint32(d.raw[f.offset:])
		v := math.Float32frombits(bits)
		d.Stored[id] = StoredFloat{Bits: bits, Value: v}
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			d.warn(NonFiniteSummary, id, f.offset, -1)
		}
	}
	// Аппарат хранит дробные минуты. Допуск в две минуты учитывает округление
	// границ заголовка по правилам исследовательского профиля.
	m := float64(d.Stored["summary_0ddc"].Value)
	limit := d.End.asTime().Sub(d.Start.asTime()).Minutes() + 2
	if !(m > 0 && m <= limit) {
		d.warn(InvalidDuration, "summary_0ddc", 0x0ddc, -1)
	}
	d.checkObservedRelations()
}

// Эти равенства наблюдались на исследованном корпусе. Нарушение равенства
// заслуживает внимания, но не отменяет независимо прочитанные исходные числа.
func (d *Day) checkObservedRelations() {
	for i, p := range d.Pressure {
		if p.FirstRaw != p.SecondRaw {
			d.warn(ProfileDeviation, "pressure_2", secondPressureOffset+2*i, i)
			break
		}
	}
	for off := 0xdb8; off <= 0xdc4; off += 4 {
		if d.Stored[summaryID(off)].Bits != d.Stored[summaryID(off-16)].Bits {
			d.warn(ProfileDeviation, summaryID(off), off, -1)
		}
	}
	minutes := d.Stored["summary_0ddc"].Value
	if minutes > 0 && !d.hasDiagnostic(InvalidDuration) {
		numerator := d.Stored["summary_0de4"].Value
		want := math.Float32bits(numerator / minutes)
		if want != d.Stored["summary_0e38"].Bits {
			d.warn(ProfileDeviation, "summary_0e38", 0x0e38, -1)
		}
	}
}
