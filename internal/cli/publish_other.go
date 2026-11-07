//go:build !darwin && !linux && !windows

package cli

import (
	"os"
	"path/filepath"
)

// На остальных системах резервируем имя каталога и публикуем готовые файлы по одному.
func publishNew(stage, dest string) error {
	info, err := os.Stat(stage)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.Link(stage, dest)
	}
	files, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	if err = os.Mkdir(dest, 0700); err != nil {
		return err
	}
	var published []string
	for _, f := range files {
		name := filepath.Join(dest, f.Name())
		if err = os.Link(filepath.Join(stage, f.Name()), name); err != nil {
			for _, p := range published {
				os.Remove(p)
			}
			os.Remove(dest)
			return err
		}
		published = append(published, name)
	}
	return nil
}
