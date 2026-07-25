//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package compact

import "io/fs"

func validateResultPrivateRegularFileLinkCount(_ string, _ fs.FileInfo) error {
	return nil
}
