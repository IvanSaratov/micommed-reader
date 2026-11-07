package cli

import (
	"golang.org/x/sys/unix"
	"os"
)

func publishNew(stage, dest string) error {
	err := unix.Renameat2(unix.AT_FDCWD, stage, unix.AT_FDCWD, dest, unix.RENAME_NOREPLACE)
	if err != nil {
		return &os.LinkError{Op: "публикация", Old: stage, New: dest, Err: err}
	}
	return nil
}
