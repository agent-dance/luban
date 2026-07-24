//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package session

import "os"

func validateMemoryRegularFileLinkCount(f *os.File, path string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return validatePrivateRegularFileLinkCount(path, "open", info)
}
