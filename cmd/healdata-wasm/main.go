//go:build js && wasm

package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall/js"

	"github.com/IvanSaratov/micommed-reader/internal/report"
)

var current *report.Input
var complete *report.Report
var grouped *report.SessionReport

type blobReader struct {
	file int
	size int64
}

func (b blobReader) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, errors.New("некорректное смещение файла")
	}
	if off >= b.size {
		return 0, io.EOF
	}
	// FileReaderSync работает только в воркере. JS кэширует один блок выбранного файла.
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("не удалось прочитать файл: %v", v)
		}
	}()
	available := min(int64(len(p)), b.size-off)
	data := js.Global().Call("healdataRead", b.file, float64(off), int(available))
	n = js.CopyBytesToGo(p, data)
	if n < len(p) {
		err = io.EOF
	}
	return
}

type request struct {
	Action string `json:"action"`
	Files  []struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		Modified int64  `json:"modified"`
	} `json:"files"`
	Date      string `json:"date"`
	SHA       string `json:"sha"`
	From      string `json:"from"`
	Through   string `json:"through"`
	Format    string `json:"format"`
	Gap       int    `json:"gap"`
	SessionID int    `json:"session_id"`
}
type reply struct {
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

func main() {
	handler := js.FuncOf(func(_ js.Value, args []js.Value) (out any) {
		defer func() {
			if v := recover(); v != nil {
				out = encode(reply{Error: fmt.Sprint(v)})
			}
		}()
		if len(args) != 1 {
			return encode(reply{Error: "неверный запрос"})
		}
		var req request
		if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
			return encode(reply{Error: "неверный запрос"})
		}
		value, err := dispatch(req)
		if err != nil {
			return encode(reply{Error: err.Error()})
		}
		return encode(reply{Value: value})
	})
	js.Global().Set("healdataCall", handler)
	js.Global().Call("healdataReady")
	select {}
}
func encode(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"не удалось подготовить результат"}`
	}
	return string(b)
}
func dispatch(req request) (any, error) {
	if req.Action == "open" {
		return open(req)
	}
	if current == nil {
		return nil, errors.New("сначала откройте выгрузку")
	}
	if req.Action == "day" {
		return complete.Day(req.Date, req.SHA)
	}
	if req.Action == "sessions" {
		next, err := complete.Sessions(req.Gap)
		if err != nil {
			return nil, err
		}
		grouped = next
		return grouped.Index(), nil
	}
	if req.Action == "session" {
		if grouped == nil {
			return nil, errors.New("сначала подготовьте сеансы")
		}
		return grouped.View(req.SessionID)
	}
	for _, s := range []string{req.From, req.Through} {
		if s != "" {
			if _, err := report.ParseDate(s); err != nil {
				return nil, errors.New("укажите корректные даты периода")
			}
		}
	}
	if req.From != "" && req.Through != "" && req.From > req.Through {
		return nil, errors.New("начало периода позже окончания")
	}
	r, err := report.Build(current, req.From, req.Through)
	if err != nil {
		return nil, err
	}
	switch req.Action {
	case "period":
		return r.Index(), nil
	case "export":
		out := &downloadWriter{}
		buffered := bufio.NewWriterSize(out, 64<<10)
		switch req.Format {
		case "json":
			err = r.WriteJSON(buffered)
		case "csv":
			err = r.WriteCSVZip(buffered)
		default:
			err = errors.New("неизвестный формат экспорта")
		}
		if err == nil {
			err = buffered.Flush()
		}
		return map[string]int64{"bytes": out.total}, err
	default:
		return nil, errors.New("неизвестная команда")
	}
}
func open(req request) (any, error) {
	current, complete = nil, nil
	grouped = nil
	if len(req.Files) == 0 {
		return nil, errors.New("выберите файл или ZIP")
	}
	if len(req.Files) > report.MaxEntries {
		return nil, report.LimitError()
	}
	var total int64
	for _, f := range req.Files {
		if f.Size < 0 {
			return nil, errors.New("некорректный размер файла")
		}
		if strings.EqualFold(extension(f.Name), ".zip") && len(req.Files) != 1 {
			return nil, errors.New("откройте один ZIP или несколько дневных файлов")
		}
		total += f.Size
	}
	first := req.Files[0]
	in := report.NewInput("")
	if strings.EqualFold(extension(first.Name), ".zip") {
		if req.Date != "" {
			return nil, errors.New("дата одиночного дня не применяется к ZIP")
		}
		err := in.ReadZIP(context.Background(), blobReader{0, first.Size}, first.Size, func(done, count int) { js.Global().Call("healdataProgress", done, count) })
		if err != nil {
			return nil, err
		}
	} else {
		if total > report.MaxBytes {
			return nil, report.LimitError()
		}
		if req.Date != "" {
			if len(req.Files) != 1 {
				return nil, errors.New("дату можно указать только для одного файла")
			}
			if _, err := report.ParseDate(req.Date); err != nil {
				return nil, errors.New("укажите корректную дату отчётного дня")
			}
		}
		if len(req.Files) > 1 {
			// Один выбор файлов задаёт один контекст. Имена и размеры образуют его устойчивую метку.
			manifest, _ := json.Marshal(req.Files)
			in = report.NewInput(fmt.Sprintf("selection:%x", sha256.Sum256(manifest)))
		}
		for i, f := range req.Files {
			b := blobReader{i, f.Size}
			openFile := func() (io.ReadCloser, error) { return io.NopCloser(io.NewSectionReader(b, 0, f.Size)), nil }
			if len(req.Files) == 1 {
				date, ok := report.DateInName(f.Name)
				if ok && req.Date != "" && date != req.Date {
					return nil, fmt.Errorf("дата имени %s расходится с выбранной датой %s", date, req.Date)
				}
				if !ok {
					date = req.Date
				}
				if date == "" {
					return nil, errors.New("укажите дату отчётного дня для переименованного файла")
				}
				if err := in.ReadDay(f.Name, date, openFile); err != nil {
					return nil, err
				}
			} else {
				if err := in.Add(f.Name, f.Name, 0, uint64(f.Size), openFile); err != nil {
					return nil, err
				}
			}
			js.Global().Call("healdataProgress", i+1, len(req.Files))
		}
		if err := in.Finish(); err != nil {
			return nil, err
		}
	}
	r, err := report.Build(in, "", "")
	if err != nil {
		return nil, err
	}
	current, complete = in, r
	return r.Index(), nil
}
func extension(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return ""
	}
	return name[i:]
}

type downloadWriter struct{ total int64 }

func (w *downloadWriter) Write(p []byte) (int, error) {
	// Ограничиваем временный Blob браузера. При превышении пользователь выбирает меньший период.
	if w.total+int64(len(p)) > 128<<20 {
		return 0, errors.New("экспорт превышает 128 МиБ. Выберите меньший период")
	}
	data := js.Global().Get("Uint8Array").New(len(p))
	js.CopyBytesToJS(data, p)
	js.Global().Call("healdataChunk", data)
	w.total += int64(len(p))
	return len(p), nil
}
