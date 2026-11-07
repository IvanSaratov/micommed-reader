# RC-C60 Reader

`healdata` — библиотека на Go и утилита командной строки для локального чтения дневных записей RC-C60. Утилита открывает файл, папку или ZIP, показывает сводку терапии и экспортирует измерения в JSON и CSV. Поддерживаемый профиль HEALDATA содержит 9 312 байт на день. Трактовки полей и единиц основаны на исследовании формата. [Описание формата](docs/format.md)

## Установка

Требуется Go 1.26 или новее.

```sh
go install github.com/IvanSaratov/micommed-reader/cmd/healdata@latest
```

Чтобы собрать утилиту из клона репозитория, выполните `go build -o ./healdata ./cmd/healdata`. Для проверки сборки на синтетической записи выполните следующую команду из корня проекта.

```sh
./healdata inspect --date 2001-02-03 research/fixtures/clock.bin
```

## Команды

```sh
healdata inspect ./HealData/
healdata summary ./HealData.zip
healdata summary --from 2001-02-03 --to 2001-02-04 ./HealData/
healdata export json -o report.json ./HealData.zip
healdata export csv -o report-csv ./HealData.zip
```

`inspect` показывает состав и качество записей. У штатного дневного файла имя имеет вид `YYYYMMDD`. Для переименованного файла укажите `--date`.

`summary` показывает суточный AHI аппарата и оценку по событиям. AHI периода рассчитывается по сумме событий и минут за учтённые дни. Точный дубликат учитывается один раз. Если за дату имеются разные версии, обе попадают в экспорт, а дата исключается из сводного AHI.

Оба экспорта принимают включительные границы `--from` и `--to`. JSON без `-o` выводится в stdout. CSV создаёт новую папку с таблицами `sources`, `days`, `pressure`, `channels`, `events`, `stored`, `period` и `diagnostics`. Измерения связаны с источниками через `source_id`, `date` и `sha256`. Пропущенное значение сопровождается причиной. Время указано по часам аппарата.

ZIP читается без извлечения на диск. Пределы для выгрузки — 4 096 элементов и 40 МиБ распакованных данных. Каталог записей ZIP также ограничен 40 МиБ. Код завершения `0` означает успех, `1` — ошибку ввода или вывода, `2` — ошибку аргументов, `3` — готовый результат с повреждёнными или исключёнными записями. Подробности каждой команды доступны через `--help`.

## Библиотека Go

```sh
go get github.com/IvanSaratov/micommed-reader
```

Импортируйте пакет как `healdata "github.com/IvanSaratov/micommed-reader"`. В следующем фрагменте `raw` содержит распакованные байты одного дня.

```go
day, err := healdata.DecodeDay(raw, healdata.Source{
    ReportDate: healdata.Date{Year: 2001, Month: 2, Day: 3},
    SourceID:   "synthetic-A",
})
if err != nil {
    return err
}
stats := day.Stats()
if stats.RecalculatedAHI.Valid {
    fmt.Println(stats.RecalculatedAHI.Value)
}
```

`ClockAt` переводит минуту отчётных суток в показание часов устройства. Предупреждения доступны в `day.Diagnostics`. Для расчёта по нескольким датам вызовите `SummarizePeriod(days, from, through)`, передав записи одного устройства или источника. Результат содержит покрытие, пропущенные даты и причины исключений. [Исполняемый пример](contract_test.go)

Проверка библиотеки и CLI на синтетических примерах — `go test ./...`. Примеры в `research/fixtures/` воспроизводятся командой `python3 research/stage1/make_fixtures.py`.
