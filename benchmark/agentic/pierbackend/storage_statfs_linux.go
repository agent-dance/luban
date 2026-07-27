//go:build linux

package pierbackend

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func sampleHostFilesystem(path string) (hostFilesystemSample, error) {
	handle, err := pinHostFilesystem(path)
	if err != nil {
		return hostFilesystemSample{}, err
	}
	sample, sampleErr := handle.sample()
	return sample, errors.Join(sampleErr, handle.close())
}

type unixPinnedHostFilesystem struct {
	file           *os.File
	pinnedIdentity hostFilesystemIdentity
	closed         bool
}

func pinHostFilesystem(path string) (pinnedHostFilesystem, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.IsDir() {
		_ = file.Close()
		return nil, errors.New("host storage authority is not a directory")
	}
	var fileStat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &fileStat); err != nil {
		_ = file.Close()
		return nil, err
	}
	var stat unix.Statfs_t
	if err := unix.Fstatfs(int(file.Fd()), &stat); err != nil {
		_ = file.Close()
		return nil, err
	}
	identity := makeHostFilesystemIdentity(
		uint64(fileStat.Dev), uint64(stat.Type), uint64(stat.Fsid.Val[0]), uint64(stat.Fsid.Val[1]), "",
	)
	return &unixPinnedHostFilesystem{file: file, pinnedIdentity: identity}, nil
}

func (filesystem *unixPinnedHostFilesystem) identity() string {
	return filesystem.pinnedIdentity.privateTuple
}

func (filesystem *unixPinnedHostFilesystem) sample() (hostFilesystemSample, error) {
	if filesystem == nil || filesystem.file == nil || filesystem.closed {
		return hostFilesystemSample{}, errors.New("host filesystem handle is closed")
	}
	var stat unix.Statfs_t
	if err := unix.Fstatfs(int(filesystem.file.Fd()), &stat); err != nil {
		return hostFilesystemSample{}, err
	}
	var fileStat unix.Stat_t
	if err := unix.Fstat(int(filesystem.file.Fd()), &fileStat); err != nil {
		return hostFilesystemSample{}, err
	}
	observedIdentity := makeHostFilesystemIdentity(
		uint64(fileStat.Dev), uint64(stat.Type), uint64(stat.Fsid.Val[0]), uint64(stat.Fsid.Val[1]), "",
	)
	if observedIdentity != filesystem.pinnedIdentity {
		return hostFilesystemSample{}, errors.New("pinned host filesystem volume identity changed")
	}
	blockSize := uint64(stat.Frsize)
	if blockSize == 0 {
		if stat.Bsize <= 0 {
			return hostFilesystemSample{}, errors.New("host filesystem reported no positive block size")
		}
		blockSize = uint64(stat.Bsize)
	}
	total, err := checkedFilesystemBytes(stat.Blocks, blockSize)
	if err != nil {
		return hostFilesystemSample{}, err
	}
	available, err := checkedFilesystemBytes(stat.Bavail, blockSize)
	if err != nil {
		return hostFilesystemSample{}, err
	}
	if stat.Bfree > stat.Blocks {
		return hostFilesystemSample{}, errors.New("host filesystem free blocks exceed total blocks")
	}
	used, err := checkedFilesystemBytes(stat.Blocks-stat.Bfree, blockSize)
	if err != nil {
		return hostFilesystemSample{}, err
	}
	return hostFilesystemSample{
		identity:             filesystem.pinnedIdentity.privateTuple,
		volumeIdentitySHA256: filesystem.pinnedIdentity.publicSHA256,
		filesystemType:       filesystem.pinnedIdentity.filesystemType,
		blockSizeBytes:       blockSize, totalBytes: total,
		availableBytes: available, usedBytes: used,
	}, nil
}

func (filesystem *unixPinnedHostFilesystem) close() error {
	if filesystem == nil || filesystem.file == nil || filesystem.closed {
		return nil
	}
	filesystem.closed = true
	return filesystem.file.Close()
}
