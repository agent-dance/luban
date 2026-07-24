//go:build !windows

package commands

import (
	"os"
	"syscall"

	"github.com/agent-dance/luban/i18n"
)

// checkDiskSpace reports free disk space in the home directory.
func checkDiskSpace(lang i18n.Language) checkResult {
	r := checkResult{}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "/"
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(home, &stat); err != nil {
		r.ok = false
		r.message = i18n.Format(lang, i18n.KeyDoctorDiskStatError, home, err)
		return r
	}

	freeBytes := stat.Bavail * uint64(stat.Bsize) //nolint:unconvert
	return diskSpaceResult(freeBytes, lang)
}
