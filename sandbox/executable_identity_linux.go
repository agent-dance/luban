//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"syscall"
)

func executableIdentity(info os.FileInfo) (string, uint32, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return "", 0, false
	}
	identity := fmt.Sprintf(
		"dev=%d;ino=%d;uid=%d;gid=%d;mode=%d;size=%d;mtime=%d.%d;ctime=%d.%d",
		stat.Dev, stat.Ino, stat.Uid, stat.Gid, stat.Mode, stat.Size,
		stat.Mtim.Sec, stat.Mtim.Nsec, stat.Ctim.Sec, stat.Ctim.Nsec,
	)
	return identity, stat.Uid, true
}

func executableOwner(info os.FileInfo) (uint32, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, false
	}
	return stat.Uid, true
}
