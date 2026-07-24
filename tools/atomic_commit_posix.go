//go:build !windows

package tools

import "os"

// replaceFileAtomically commits a same-directory temporary file over path in
// one filesystem rename operation. Callers must not add a remove-then-rename
// fallback: that would delete a concurrent winner and expose an absent target.
func replaceFileAtomically(temporaryPath, targetPath string) error {
	return os.Rename(temporaryPath, targetPath)
}

// publishFileAtomicallyNoReplace adds targetPath only if it is still absent.
// The temporary hard link is removed by the caller after the directory entry
// has been made durable.
func publishFileAtomicallyNoReplace(temporaryPath, targetPath string) error {
	return os.Link(temporaryPath, targetPath)
}
