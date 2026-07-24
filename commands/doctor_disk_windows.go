//go:build windows

package commands

import (
	"os"

	"github.com/agent-dance/luban/i18n"
	"golang.org/x/sys/windows"
)

// checkDiskSpace reports free disk space in the home directory.
func checkDiskSpace(lang i18n.Language) checkResult {
	r := checkResult{}

	home, err := os.UserHomeDir()
	if err != nil {
		home = `C:\`
	}
	path, err := windows.UTF16PtrFromString(home)
	if err != nil {
		r.ok = false
		r.message = i18n.Format(lang, i18n.KeyDoctorDiskStatError, home, err)
		return r
	}

	var freeBytesAvailable uint64
	if err := windows.GetDiskFreeSpaceEx(path, &freeBytesAvailable, nil, nil); err != nil {
		r.ok = false
		r.message = i18n.Format(lang, i18n.KeyDoctorDiskStatError, home, err)
		return r
	}
	return diskSpaceResult(freeBytesAvailable, lang)
}
