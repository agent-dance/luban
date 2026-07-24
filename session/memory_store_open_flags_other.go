//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package session

func memoryStoreNonblockFlag() int {
	return 0
}
