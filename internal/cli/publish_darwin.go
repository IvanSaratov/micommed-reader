package cli

import (
	"golang.org/x/sys/unix"
	"os"
)

// RENAME_EXCL проверяет назначение и переносит готовый результат одним системным вызовом.
func publishNew(stage, dest string) error {
	err := unix.RenameatxNp(unix.AT_FDCWD, stage, unix.AT_FDCWD, dest, unix.RENAME_EXCL)
	if err != nil {
		return &os.LinkError{Op: "публикация", Old: stage, New: dest, Err: err}
	}
	return nil
}
