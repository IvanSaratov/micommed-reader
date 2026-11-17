package report

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	hd "github.com/IvanSaratov/micommed-reader/pkg/healdata"
)

const (
	MaxEntries = 4096
	MaxBytes   = 40 << 20
	daySize    = 9312
)

type Problem struct {
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

func diagnostic(d hd.Diagnostic) Problem {
	p := Problem{Kind: "warning", Code: string(d.Code), Field: d.Field}
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
	issue       *Problem
}
type Input struct {
	id                 string
	records            []record
	services, entries  int
	candidates         int
	declared, consumed int64
	idDirectory        string
}

func DateInName(name string) (string, bool) {
	if len(name) != 8 || strings.Trim(name, "0123456789") != "" {
		return "", false
	}
	return name[:4] + "-" + name[4:6] + "-" + name[6:], true
}

func asDate(s string) hd.Date {
	if s == "" {
		return hd.Date{}
	}
	t, _ := ParseDate(s)
	return hd.Date{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
}
func LimitError() error {
	return fmt.Errorf("превышен предел входа (%d элементов, 40 МиБ содержимого)", MaxEntries)
}

func (in *Input) Add(name, label string, mode fs.FileMode, size uint64, open func() (io.ReadCloser, error)) error {
	in.entries++
	if in.entries > MaxEntries || size > uint64(MaxBytes-in.declared) {
		return LimitError()
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
	date, dated := DateInName(path.Base(name))
	if !dated {
		in.services++
	}
	return in.ReadDay(label, date, open)
}

// Даже служебный файл дочитываем до EOF для проверки CRC и общего бюджета.
// В памяти остаётся максимум одна дневная запись плюс байт для проверки длины.
func (in *Input) ReadDay(label, date string, open func() (io.ReadCloser, error)) error {
	r := record{date: date, label: label}
	single := in.id == ""
	if date != "" {
		in.candidates++
	}
	f, err := open()
	var raw []byte
	if err == nil {
		budget := MaxBytes - in.consumed + 1
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
		if in.consumed > MaxBytes {
			return LimitError()
		}
		if digest != nil {
			in.id = fmt.Sprintf("file:%x", digest.Sum(nil))
		}
	}
	if single && err != nil {
		return fmt.Errorf("чтение дневного файла: %w", err)
	}
	if date != "" {
		if _, e := ParseDate(date); e != nil {
			r.date = ""
			r.issue = &Problem{Code: "invalid_source_date", Detail: "некорректная дата в имени: " + date}
		}
	}
	switch {
	case err != nil:
		r.issue = &Problem{Code: "read_error", Detail: err.Error()}
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

func NewInput(id string) *Input { return &Input{id: id} }
func (in *Input) Finish() error {
	if in.candidates == 0 {
		return errors.New("дневные записи не найдены")
	}
	return nil
}
func (in *Input) ReadZIP(ctx context.Context, f io.ReaderAt, size int64, progress func(int, int)) error {
	if err := checkZipIndex(f, size); err != nil {
		return fmt.Errorf("чтение каталога ZIP: %w", err)
	}
	z, err := zip.NewReader(f, size)
	if err != nil {
		return fmt.Errorf("чтение ZIP: %w", err)
	}
	if len(z.File) > MaxEntries {
		return LimitError()
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.NewSectionReader(f, 0, size)); err != nil {
		return err
	}
	in.id = fmt.Sprintf("zip:%x", h.Sum(nil))
	for i, m := range z.File {
		if progress != nil {
			progress(i, len(z.File))
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Номер отличает записи с одинаковым путём. Имя используется только как метка.
		label := fmt.Sprintf("%s#%d", m.Name, i+1)
		if err := in.Add(m.Name, label, m.Mode(), m.UncompressedSize64, m.Open); err != nil {
			return err
		}
	}
	return in.Finish()
}

func ParseDate(s string) (time.Time, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err == nil && t.Year() == 0 {
		err = errors.New("недопустимый год")
	}
	return t, err
}
func (r *Report) Partial() bool { return r.partial }
