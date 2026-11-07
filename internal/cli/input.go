package cli

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	hd "github.com/IvanSaratov/micommed-reader"
)

const (
	maxEntries = 4096
	maxBytes   = 40 << 20
	daySize    = 9312
)

type problem struct {
	Kind    string     `json:"kind"`
	Code    string     `json:"code"`
	Date    string     `json:"date,omitempty"`
	Through string     `json:"through,omitempty"`
	Label   string     `json:"label,omitempty"`
	Field   hd.FieldID `json:"field,omitempty"`
	Offset  *int       `json:"offset,omitempty"`
	Index   *int       `json:"index,omitempty"`
	Detail  string     `json:"detail,omitempty"`
}

func diagnostic(d hd.Diagnostic) problem {
	p := problem{Kind: "warning", Code: string(d.Code), Field: d.Field}
	if d.Offset >= 0 {
		p.Offset = &d.Offset
	}
	if d.Index >= 0 {
		p.Index = &d.Index
	}
	return p
}

type record struct {
	date, label string
	day         *hd.Day
	issue       *problem
}
type input struct {
	id                 string
	records            []record
	services, entries  int
	candidates         int
	declared, consumed int64
	idDirectory        string
}

func dateInName(name string) (string, bool) {
	if len(name) != 8 || strings.Trim(name, "0123456789") != "" {
		return "", false
	}
	return name[:4] + "-" + name[4:6] + "-" + name[6:], true
}

func asDate(s string) hd.Date {
	if s == "" {
		return hd.Date{}
	}
	t, _ := parseDate(s)
	return hd.Date{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
}
func containerLimit() error {
	return fmt.Errorf("превышен предел входа (%d элементов, 40 МиБ содержимого)", maxEntries)
}

func readInput(ctx context.Context, name, date string) (*input, error) {
	if name == "" {
		return nil, usage("пустой путь к источнику")
	}
	// Удаляем только завершающие разделители: Lstat("ссылка/") иначе пойдёт по ссылке.
	for len(name) > len(filepath.VolumeName(name))+1 && os.IsPathSeparator(name[len(name)-1]) {
		name = name[:len(name)-1]
	}
	info, err := os.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("открытие источника: %w", err)
	}
	in := &input{}
	isZIP := strings.EqualFold(filepath.Ext(name), ".zip")
	switch {
	case info.IsDir():
		if date != "" {
			return nil, usage("--date применяется только к одиночному дневному файлу")
		}
		abs, e := filepath.Abs(name)
		if e != nil {
			return nil, e
		}
		resolved, e := filepath.EvalSymlinks(abs)
		if e != nil {
			return nil, e
		}
		in.id = fmt.Sprintf("dir:%x", sha256.Sum256([]byte(resolved)))
		err = in.folder(ctx, resolved)
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("источник должен быть обычным файлом или папкой")
	case isZIP:
		if date != "" {
			return nil, usage("--date применяется только к одиночному дневному файлу")
		}
		err = in.archive(ctx, name)
	default:
		label := filepath.Base(name)
		// Дата имени относится к суткам от 09:00. Заголовок может приходиться на следующие календарные сутки.
		fileDate, dated := dateInName(label)
		if dated && date != "" && date != fileDate {
			return nil, usage("дата имени %s расходится с --date %s", fileDate, date)
		}
		if !dated && date == "" {
			return nil, usage("укажите --date YYYY-MM-DD для файла %q", label)
		}
		if dated {
			date = fileDate
		}
		if info.Size() > maxBytes {
			return nil, containerLimit()
		}
		err = in.read(label, date, func() (io.ReadCloser, error) { return os.Open(name) })
	}
	if err != nil {
		return nil, err
	}
	if in.candidates == 0 {
		return nil, errors.New("дневные записи не найдены")
	}
	return in, nil
}

func (in *input) entry(name, label string, mode fs.FileMode, size uint64, open func() (io.ReadCloser, error)) error {
	in.entries++
	if in.entries > maxEntries || size > uint64(maxBytes-in.declared) {
		return containerLimit()
	}
	in.declared += int64(size)
	if mode.IsDir() {
		return nil
	}
	if !mode.IsRegular() {
		in.services++
		return nil
	}
	if strings.EqualFold(path.Base(name), "ID.txt") {
		dir := path.Dir(name)
		if in.idDirectory != "" && in.idDirectory != dir {
			return errors.New("найдено несколько выгрузок с ID.txt. Выберите одну папку или ZIP")
		}
		in.idDirectory = dir
	}
	date, dated := dateInName(path.Base(name))
	if !dated {
		in.services++
	}
	return in.read(label, date, open)
}

func (in *input) folder(ctx context.Context, name string) error {
	root, err := os.OpenRoot(name)
	if err != nil {
		return err
	}
	defer root.Close()
	// Читаем каталог порциями: ограничение числа элементов действует и на сам обход.
	pending := []string{"."}
	for len(pending) > 0 {
		dir := pending[0]
		pending = pending[1:]
		children, err := in.readDirectory(ctx, root, dir)
		if err != nil {
			return fmt.Errorf("обход папки %q: %w", dir, err)
		}
		pending = append(pending, children...)
	}
	return nil
}

func (in *input) readDirectory(ctx context.Context, root *os.Root, dir string) ([]string, error) {
	f, err := root.Open(filepath.FromSlash(dir))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var children []string
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch, readErr := f.ReadDir(64)
		for _, entry := range batch {
			p := path.Join(dir, entry.Name())
			info, err := root.Lstat(filepath.FromSlash(p))
			if err != nil {
				return nil, err
			}
			var size uint64
			if info.Mode().IsRegular() {
				size = uint64(info.Size())
			}
			if err := in.entry(p, p, info.Mode(), size, func() (io.ReadCloser, error) { return root.Open(filepath.FromSlash(p)) }); err != nil {
				return nil, err
			}
			if info.IsDir() {
				children = append(children, p)
			}
		}
		if readErr == io.EOF {
			return children, nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}

}

func (in *input) archive(ctx context.Context, name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if err := checkZipIndex(f, stat.Size()); err != nil {
		return fmt.Errorf("чтение каталога ZIP: %w", err)
	}
	z, err := zip.NewReader(f, stat.Size())
	if err != nil {
		return fmt.Errorf("чтение ZIP: %w", err)
	}
	if len(z.File) > maxEntries {
		return containerLimit()
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	in.id = fmt.Sprintf("zip:%x", h.Sum(nil))
	for i, m := range z.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Номер отличает записи с одинаковым путём. Имя используется только как метка.
		label := fmt.Sprintf("%s#%d", m.Name, i+1)
		if err := in.entry(m.Name, label, m.Mode(), m.UncompressedSize64, m.Open); err != nil {
			return err
		}
	}
	return nil
}

// Даже служебный файл дочитываем до EOF для проверки CRC и общего бюджета.
// В памяти остаётся максимум одна дневная запись плюс байт для проверки длины.
func (in *input) read(label, date string, open func() (io.ReadCloser, error)) error {
	r := record{date: date, label: label}
	single := in.id == ""
	if date != "" {
		in.candidates++
	}
	f, err := open()
	var raw []byte
	if err == nil {
		budget := maxBytes - in.consumed + 1
		limited := &io.LimitedReader{R: f, N: budget}
		var reader io.Reader = limited
		var digest hash.Hash
		if single {
			digest = sha256.New()
			reader = io.TeeReader(limited, digest)
		}
		if date != "" {
			raw, err = io.ReadAll(io.LimitReader(reader, daySize+1))
		}
		if err == nil {
			_, err = io.Copy(io.Discard, reader)
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		in.consumed += budget - limited.N
		if in.consumed > maxBytes {
			return containerLimit()
		}
		if digest != nil {
			in.id = fmt.Sprintf("file:%x", digest.Sum(nil))
		}
	}
	if single && err != nil {
		return fmt.Errorf("чтение дневного файла: %w", err)
	}
	if date != "" {
		if _, e := parseDate(date); e != nil {
			r.date = ""
			r.issue = &problem{Code: "invalid_source_date", Detail: "некорректная дата в имени: " + date}
		}
	}
	switch {
	case err != nil:
		r.issue = &problem{Code: "read_error", Detail: err.Error()}
	case r.issue != nil:
	case date == "":
		return nil
	default:
		day, e := hd.DecodeDay(raw, hd.Source{ReportDate: asDate(date), SourceID: in.id, Label: label})
		if e != nil {
			var de *hd.DecodeError
			if !errors.As(e, &de) {
				return e
			}
			p := diagnostic(de.Diagnostic)
			r.issue = &p
		} else {
			r.day = day
		}
	}
	if r.issue != nil {
		r.issue.Kind = "error"
		r.issue.Label = label
		r.issue.Date = r.date
	}
	in.records = append(in.records, r)
	return nil
}
