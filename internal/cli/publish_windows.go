package cli

import (
	"golang.org/x/sys/windows"
	"os"
)

func publishNew(stage, dest string) error {
	from, err := windows.UTF16PtrFromString(stage)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(dest)
	if err != nil {
		return err
	}
	if err = windows.MoveFile(from, to); err != nil {
		return &os.LinkError{Op: "публикация", Old: stage, New: dest, Err: err}
	}
	return nil
}
