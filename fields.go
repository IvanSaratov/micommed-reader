package healdata

import "fmt"

const fieldMap = "docs/format.md"

// FieldInfo описывает, как найти исходные байты и насколько обоснованы их
// единицы. Для массива адрес элемента равен Offset + i*Stride; минута читается
// как u16 по TimeOffset + i*TimeStride. Отсутствующие смещения равны −1.
type FieldInfo struct {
	ID                                                  FieldID
	Label, RawType                                      string
	Offset, Stride, CountOffset, TimeOffset, TimeStride int
	Scale                                               float64
	Unit                                                string
	// S — структура, C — косвенное свидетельство в ПО, H — гипотеза,
	// U — неизвестно, V — официальная сверка. Статусы смысла могут сочетаться.
	StructureEvidence, MeaningEvidence, Source string
}

// EventInfo описывает предполагаемое значение числового кода для данного профиля.
type EventInfo struct{ Label, MeaningEvidence, Source string }

// DescribeEvent возвращает подпись известного кода. false оставляет трактовку открытой.
func DescribeEvent(code uint8) (EventInfo, bool) {
	var label, evidence string
	switch code {
	case 100:
		label, evidence = "Храп", "H"
	case 101:
		label, evidence = "Ограничение потока", "C"
	case 102:
		label, evidence = "Обструктивное апноэ", "C"
	case 103:
		label, evidence = "Центральное апноэ", "C"
	case 104:
		label, evidence = "Гипопноэ", "C"
	case 150:
		label, evidence = "Конец большой утечки", "C+H"
	case 151:
		label, evidence = "Начало большой утечки", "C+H"
	default:
		return EventInfo{}, false
	}
	return EventInfo{label, evidence, fieldMap + "#коды-событий-и-счётчики"}, true
}

type meaning struct{ label, unit, evidence string }

// В этом профиле группа 0x0db8…0x0dc4 повторяет сводки давления.
// Её сохраняем отдельно: совпадение чисел не устанавливает назначение полей.
var summaries = []struct {
	offset int
	meaning
}{
	{0x0da8, meaning{"Предполагаемый P95 давления", "см вод. ст.", "H"}},
	{0x0dac, meaning{"Предполагаемый P90 давления", "см вод. ст.", "H"}},
	{0x0db0, meaning{"Предполагаемое среднее давление", "см вод. ст.", "H"}},
	{0x0db4, meaning{"Предполагаемый максимум давления", "см вод. ст.", "H"}},
	{0x0db8, meaning{"Повтор первой сводки", "", "U"}},
	{0x0dbc, meaning{"Повтор второй сводки", "", "U"}},
	{0x0dc0, meaning{"Повтор третьей сводки", "", "U"}},
	{0x0dc4, meaning{"Повтор четвёртой сводки", "", "U"}},
	{0x0dc8, meaning{"Сохранённый AHI", "событий/ч", "H"}},
	{0x0dcc, meaning{"Индекс апноэ", "событий/ч", "H"}},
	{0x0dd0, meaning{"Индекс гипопноэ", "событий/ч", "H"}},
	{0x0dd4, meaning{"Индекс центрального апноэ", "событий/ч", "H"}},
	{0x0dd8, meaning{"Индекс обструктивного апноэ", "событий/ч", "H"}},
	{0x0ddc, meaning{"Предполагаемая длительность использования", "мин", "H"}},
	{0x0de0, meaning{"Вторая предполагаемая длительность", "мин", "U"}},
	{0x0de4, meaning{"Числитель доли времени состояния", "мин", "H"}},
	{0x0de8, meaning{"Неизвестная сводка", "", "U"}},
	{0x0dec, meaning{"Предполагаемая средняя утечка", "л/мин", "H"}},
	{0x0df0, meaning{"Неизвестная сводка", "", "U"}},
	{0x0df4, meaning{"Предполагаемый средний дыхательный объём", "мл", "H"}},
	{0x0df8, meaning{"Предполагаемая средняя минутная вентиляция", "л/мин", "H"}},
	{0x0dfc, meaning{"Предполагаемая средняя частота дыхания", "1/мин", "H"}},
	{0x0e38, meaning{"Предполагаемая доля времени состояния", "", "H"}},
}

func summaryID(offset int) FieldID { return FieldID(fmt.Sprintf("summary_%04x", offset)) }

// Метаданные строятся один раз; значения возвращаются по копии.
var fieldCatalog = buildCatalog()

func buildCatalog() map[FieldID]FieldInfo {
	result := make(map[FieldID]FieldInfo)
	scalar := func(id FieldID, offset int, typ string, m meaning) FieldInfo {
		return FieldInfo{ID: id, Offset: offset, RawType: typ, Label: m.label, Unit: m.unit, Scale: 1,
			CountOffset: -1, TimeOffset: -1, StructureEvidence: "S", MeaningEvidence: m.evidence, Source: fieldMap}
	}
	array := func(l arrayLayout, m meaning, scale float64) {
		typ := "u16"
		if l.width == 1 {
			typ = "u8"
		}
		info := scalar(l.id, l.values, typ, m)
		info.Stride, info.CountOffset, info.TimeOffset, info.TimeStride = l.width, l.count, l.minutes, 2
		info.Scale = scale
		result[l.id] = info
	}
	pressure := meaning{"Предполагаемое давление", "см вод. ст.", "C+H"}
	array(pressureLayout, pressure, 0.1)
	second := pressureLayout
	second.id, second.values = "pressure_2", secondPressureOffset
	array(second, pressure, 0.1)
	channels := [len(channelLayouts)]meaning{
		{"Предполагаемая утечка", "л/мин", "H"},
		{"Предполагаемый дыхательный объём", "мл", "H"},
		{"Предполагаемая минутная вентиляция", "л/мин", "H"},
		{"Предполагаемая частота дыхания", "1/мин", "H"},
		{"Пятый канал", "", "U"},
	}
	for i, l := range channelLayouts {
		array(l, channels[i], 1)
	}
	array(eventLayout, meaning{"Код события", "", "S"}, 1)
	for _, f := range summaries {
		id := summaryID(f.offset)
		result[id] = scalar(id, f.offset, "f32", f.meaning)
	}
	for _, f := range tallyFields {
		result[f.id] = scalar(f.id, f.offset, "u16", meaning{fmt.Sprintf("Счётчик кода %d", f.code), "события", "S"})
	}
	result["start"] = scalar("start", 0xb30, "u16+5u8", meaning{"Начало записи", "часы аппарата", "S"})
	result["end"] = scalar("end", 0xb38, "u16+5u8", meaning{"Конец записи", "часы аппарата", "S"})
	return result
}

// DescribeField возвращает схему и основания трактовки поля для HEALDATA-9312.
// Пустая единица и статус U обозначают неизвестный смысл исходного числа.
func DescribeField(id FieldID) (FieldInfo, bool) { f, ok := fieldCatalog[id]; return f, ok }
