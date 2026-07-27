//go:build !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package pierbackend

import "errors"

func sampleHostFilesystem(string) (hostFilesystemSample, error) {
	return hostFilesystemSample{}, errors.New("host filesystem Statfs observation is unsupported on this platform")
}

func pinHostFilesystem(string) (pinnedHostFilesystem, error) {
	return nil, errors.New("host filesystem Statfs observation is unsupported on this platform")
}
