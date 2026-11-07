package cli

import "fmt"

var reasons = map[string]string{
	"unsupported_length":   "неподдерживаемая длина записи",
	"invalid_header":       "повреждён заголовок записи",
	"invalid_source_date":  "некорректная отчётная дата",
	"count_out_of_range":   "счётчик выходит за границы массива",
	"invalid_minute":       "минута за пределами отчётных суток",
	"missing_date":         "отчётная дата неизвестна",
	"date_conflict":        "отчётная дата расходится с заголовком",
	"empty_series":         "пустой ряд измерений",
	"series_at_capacity":   "ряд заполнен до предела",
	"events_at_capacity":   "список событий заполнен до предела",
	"counter_mismatch":     "счётчик расходится со списком событий",
	"axes_differ":          "временные метки каналов различаются",
	"time_reversal":        "нарушен порядок времени",
	"sample_gap":           "интервал между измерениями",
	"non_finite_summary":   "аппаратное значение не является конечным числом",
	"invalid_duration":     "недоступна длительность терапии",
	"profile_deviation":    "отклонение от наблюдаемого профиля",
	"duplicate_content":    "повтор записи",
	"conflicting_versions": "разные версии за одну дату",
	"no_usable_days":       "нет учтённых дней",
	"missing_summary":      "отсутствует аппаратное поле",
	"read_error":           "ошибка чтения файла",
}

func reasonText(code string) string {
	if s := reasons[code]; s != "" {
		return s
	}
	return code
}
func describeProblem(p problem) string {
	s := fmt.Sprintf("%s (%s)", reasonText(p.Code), p.Code)
	if p.Field != "" {
		s += " " + string(p.Field)
	}
	if p.Offset != nil {
		s += fmt.Sprintf(" @0x%x", *p.Offset)
	}
	if p.Index != nil {
		s += fmt.Sprintf("[%d]", *p.Index)
	}
	if p.Detail != "" {
		s += " — " + p.Detail
	}
	return s
}
