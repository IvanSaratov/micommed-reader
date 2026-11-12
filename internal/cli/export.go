package cli

import (
	"io"
	"os"
	"path/filepath"
)

// Имя назначения появляется только после успешной записи и закрытия файла.
func publishJSON(dest string, write func(io.Writer) error) error {
	f, err := os.CreateTemp(filepath.Dir(dest), ".healdata-json-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = write(f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return publishNew(f.Name(), dest)
}
