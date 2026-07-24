//go:build windows

package session

import (
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func validateMemoryRegularFileLinkCount(f *os.File, path string) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return err
	}
	if info.NumberOfLinks != 1 {
		return &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return nil
}
