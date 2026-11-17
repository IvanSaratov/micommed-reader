package healdata

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"time"
)

const daySize = 9312

// DecoderVersion указывает версию декодера, в том числе для отчётов об ошибках разбора.
const DecoderVersion = "0.1.0"

// Раскладка относится только к исследованному профилю. Один дескриптор задаёт
// и границы чтения, и смещения, доступные для проверки происхождения точки.
type arrayLayout struct {
	id                                      FieldID
	count, values, minutes, width, capacity int
}

var pressureLayout = arrayLayout{"pressure_1", 0, 0x0008, 0x0648, 2, 400}

const secondPressureOffset = 0x0328

var eventLayout = arrayLayout{"events", 0x0968, 0x096a, 0x0a00, 1, 150}
var channelLayouts = [...]arrayLayout{
	{"channel_1", 0x0f18, 0x0f20, 0x104c, 1, 290},
	{"channel_2", 0x12f4, 0x12f8, 0x153c, 2, 290},
	{"channel_3", 0x1780, 0x1782, 0x18a4, 1, 290},
	{"channel_4", 0x1ae8, 0x1aea, 0x1c0c, 1, 290},
	{"channel_5", 0x1e50, 0x1e52, 0x1f74, 1, 290},
}

func word(b []byte, off int) uint16 { return binary.LittleEndian.Uint16(b[off:]) }

func (l arrayLayout) check(b []byte) error {
	n := int(word(b, l.count))
	if n > l.capacity {
		return invalid(CountOutOfRange, l.id, l.count, -1)
	}
	for i := 0; i < n; i++ {
		off := l.minutes + 2*i
		if word(b, off) >= 1440 {
			return invalid(InvalidMinute, l.id, off, i)
		}
	}
	return nil
}

// DecodeDay разбирает одну распакованную дневную запись. При структурной ошибке
// возвращает nil и *DecodeError. Предупреждения о надёжности отдельных значений
// находятся в Day.Diagnostics и позволяют читать остальные поля.
func DecodeDay(data []byte, source Source) (*Day, error) {
	if len(data) != daySize {
		return nil, invalid(UnsupportedLength, "record", -1, -1)
	}
	if source.ReportDate != (Date{}) && !source.ReportDate.valid() {
		return nil, invalid(InvalidSourceDate, "report_date", -1, -1)
	}
	start, err := readStamp(data, 0x0b30, "start")
	if err != nil {
		return nil, err
	}
	end, err := readStamp(data, 0x0b38, "end")
	if err != nil {
		return nil, err
	}
	if end.asTime().Before(start.asTime()) {
		return nil, invalid(InvalidHeader, "end", 0x0b38, -1)
	}
	if err := pressureLayout.check(data); err != nil {
		return nil, err
	}
	if err := eventLayout.check(data); err != nil {
		return nil, err
	}
	for _, l := range channelLayouts {
		if err := l.check(data); err != nil {
			return nil, err
		}
	}

	d := &Day{
		Source: source, SHA256: sha256.Sum256(data), Start: start, End: end,
		Profile: "HEALDATA-9312/v0.2", DecoderVersion: DecoderVersion, raw: bytes.Clone(data),
	}
	if source.ReportDate == (Date{}) {
		d.warn(MissingDate, "report_date", -1, -1)
	} else {
		origin := source.ReportDate.asTime().Add(9 * time.Hour)
		if start.asTime().Before(origin) || !end.asTime().Before(origin.AddDate(0, 0, 1)) {
			d.warn(DateConflict, "report_date", -1, -1)
		} else {
			d.clockKnown = true
		}
	}
	d.readSeries()
	d.readEvents()
	d.readSummaries()
	return d, nil
}
