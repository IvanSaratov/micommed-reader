package cli

import (
	"io"
	"os"
	"path/filepath"

	"github.com/IvanSaratov/micommed-reader/internal/report"
)

func writeCSV(dest string, r *report.Report) error {
	dest = filepath.Clean(dest)
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".healdata-csv-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = csvTables(stage, r); err != nil {
		return err
	}
	return publishNew(stage, dest)
}

func csvTables(dir string, r *report.Report) error {
	return r.WriteCSVTables(func(name string) (io.WriteCloser, error) {
		return os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	})
}
