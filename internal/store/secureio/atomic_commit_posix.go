//go:build !windows

package secureio

import "os"

// ReplaceFileAtomically commits a same-directory temporary file over path in
// one filesystem rename operation. Callers must not add a remove-then-rename
// fallback: that would delete a concurrent winner and expose an absent target.
func ReplaceFileAtomically(temporaryPath, targetPath string) error {
	return os.Rename(temporaryPath, targetPath)
}

// PublishFileAtomicallyNoReplace adds targetPath only if it is still absent.
// The temporary hard link is removed by the caller after the directory entry
// has been made durable.
func PublishFileAtomicallyNoReplace(temporaryPath, targetPath string) error {
	return os.Link(temporaryPath, targetPath)
}
