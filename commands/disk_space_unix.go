//go:build !windows

package commands

import "syscall"

func diskFreeGB(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	freeBytes := stat.Bavail * uint64(stat.Bsize) //nolint:unconvert
	return float64(freeBytes) / (1 << 30), nil
}
