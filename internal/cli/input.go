package cli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/IvanSaratov/micommed-reader/internal/report"
)

func readInput(ctx context.Context, name, date string) (*report.Input, error) {
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
	in := report.NewInput("")
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
		in = report.NewInput(fmt.Sprintf("dir:%x", sha256.Sum256([]byte(resolved))))
		err = folder(ctx, resolved, in)
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("источник должен быть обычным файлом или папкой")
	case isZIP:
		if date != "" {
			return nil, usage("--date применяется только к одиночному дневному файлу")
		}
		err = archive(ctx, name, in)
	default:
		label := filepath.Base(name)
		// Дата имени относится к суткам от 09:00. Заголовок может приходиться на следующие календарные сутки.
		fileDate, dated := report.DateInName(label)
		if dated && date != "" && date != fileDate {
			return nil, usage("дата имени %s расходится с --date %s", fileDate, date)
		}
		if !dated && date == "" {
			return nil, usage("укажите --date YYYY-MM-DD для файла %q", label)
		}
		if dated {
			date = fileDate
		}
		if info.Size() > report.MaxBytes {
			return nil, report.LimitError()
		}
		err = in.ReadDay(label, date, func() (io.ReadCloser, error) { return os.Open(name) })
	}
	if err != nil {
		return nil, err
	}
	if err := in.Finish(); err != nil {
		return nil, err
	}
	return in, nil
}

func folder(ctx context.Context, name string, in *report.Input) error {
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
		children, err := readDirectory(ctx, root, dir, in)
		if err != nil {
			return fmt.Errorf("обход папки %q: %w", dir, err)
		}
		pending = append(pending, children...)
	}
	return nil
}

func readDirectory(ctx context.Context, root *os.Root, dir string, in *report.Input) ([]string, error) {
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
			if err := in.Add(p, p, info.Mode(), size, func() (io.ReadCloser, error) { return root.Open(filepath.FromSlash(p)) }); err != nil {
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

func archive(ctx context.Context, name string, in *report.Input) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return in.ReadZIP(ctx, f, info.Size(), nil)
}
