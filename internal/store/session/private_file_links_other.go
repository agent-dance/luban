//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package session

import (
	"io/fs"
)

// FileInfo does not expose a portable link count. Platforms without the
// POSIX Stat_t contract retain the existing regular-file and identity checks.
func validatePrivateRegularFileLinkCount(_, _ string, _ fs.FileInfo) error {
	return nil
}
