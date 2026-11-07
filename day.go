// Package healdata читает дневные записи HEALDATA-9312 в памяти.
// Трактовки полей относятся к исследованному варианту формата RC-C60.
package healdata

import "bytes"

// FieldID обозначает расположение поля независимо от его предполагаемого смысла.
type FieldID string

// Source задаёт происхождение распакованной записи. ReportDate — дата отчётного
// дня по часам устройства. DeviceID указывают только для подтверждённого устройства.
type Source struct {
	ReportDate                Date
	SourceID, DeviceID, Label string
}

// PressurePoint хранит общую минуту и оба исходных отсчёта предполагаемого давления.
type PressurePoint struct{ Minute, FirstRaw, SecondRaw uint16 }

// Point сохраняет исходное значение и его минуту, в том числе при повторе времени.
type Point struct{ Minute, Raw uint16 }

// Series содержит один канал с собственной временной осью.
type Series struct {
	ID     FieldID
	Points []Point
}

// Event хранит исходный код события, в том числе неизвестный.
type Event struct {
	Minute uint16
	Code   uint8
}

// StoredFloat сохраняет биты аппарата даже для NaN и бесконечности.
type StoredFloat struct {
	Bits  uint32
	Value float32
}

// Day — результат DecodeDay. Срезы и карты принадлежат вызывающей стороне.
// Методам статистики следует передавать записи без ручных изменений.
type Day struct {
	Source                  Source
	SHA256                  [32]byte
	Profile, DecoderVersion string
	Start, End              DeviceTime
	Pressure                []PressurePoint
	Channels                [5]Series
	Events                  []Event
	Counters                map[uint8]uint16
	Stored                  map[FieldID]StoredFloat
	Diagnostics             []Diagnostic

	raw        []byte
	clockKnown bool
}

// RawBytes возвращает копию всей записи, включая неизвестные поля и хвосты.
func (d *Day) RawBytes() []byte { return bytes.Clone(d.raw) }

func (d *Day) hasDiagnostic(code Code) bool {
	for _, issue := range d.Diagnostics {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func (d *Day) warn(code Code, field FieldID, offset, index int) {
	d.Diagnostics = append(d.Diagnostics, Diagnostic{code, field, offset, index})
}
