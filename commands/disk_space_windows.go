//go:build windows

package commands

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func diskFreeGB(path string) (float64, error) {
	root := filepath.VolumeName(path) + `\`
	if root == `\` {
		root = path
	}
	ptr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, err
	}
	var freeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &freeBytes, nil, nil); err != nil {
		return 0, err
	}
	return float64(freeBytes) / (1 << 30), nil
}
