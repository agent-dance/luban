//go:build windows

package tools

import (
	"os"

	"golang.org/x/sys/windows"
)

// replaceFileAtomically uses the Win32 same-volume rename primitive with
// replace and write-through semantics. The temporary file is created beside
// the target, so copying across volumes is neither necessary nor permitted.
// If Windows cannot replace the target (for example, because another process
// denied delete sharing), the operation fails closed and leaves the target
// untouched.
func replaceFileAtomically(temporaryPath, targetPath string) error {
	return moveFileAtomically(temporaryPath, targetPath,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH, "replace")
}

// publishFileAtomicallyNoReplace omits MOVEFILE_REPLACE_EXISTING, making an
// existing destination an error decided by the filesystem in the same rename
// operation that publishes the temporary file.
func publishFileAtomicallyNoReplace(temporaryPath, targetPath string) error {
	return moveFileAtomically(temporaryPath, targetPath, windows.MOVEFILE_WRITE_THROUGH, "publish")
}

func moveFileAtomically(temporaryPath, targetPath string, flags uint32, operation string) error {
	from, err := windows.UTF16PtrFromString(temporaryPath)
	if err != nil {
		return &os.LinkError{Op: operation, Old: temporaryPath, New: targetPath, Err: err}
	}
	to, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return &os.LinkError{Op: operation, Old: temporaryPath, New: targetPath, Err: err}
	}
	if err := windows.MoveFileEx(from, to, flags); err != nil {
		return &os.LinkError{Op: operation, Old: temporaryPath, New: targetPath, Err: err}
	}
	return nil
}
