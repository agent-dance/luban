//go:build darwin || linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

// task28SurfacePTY gives the real TUI App a terminal without depending on
// another task's test harness or adding a package. go-tui requires raw-mode
// stdin during construction even though this acceptance test drives the REPL
// directly and never starts the interactive event loop.
func task28SurfacePTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	fail := func(err error) (*os.File, *os.File) {
		_ = master.Close()
		t.Fatal(err)
		return nil, nil
	}
	fd := master.Fd()
	var slavePath string
	switch runtime.GOOS {
	case "darwin":
		for _, request := range []uintptr{0x20007454, 0x20007452} { // TIOCPTYGRANT, TIOCPTYUNLK
			if ioctlErr := task28SurfaceIoctl(fd, request, 0); ioctlErr != nil {
				return fail(ioctlErr)
			}
		}
		var name [128]byte
		if ioctlErr := task28SurfaceIoctl(fd, 0x40807453, uintptr(unsafe.Pointer(&name[0]))); ioctlErr != nil { // TIOCPTYGNAME
			return fail(ioctlErr)
		}
		if end := bytes.IndexByte(name[:], 0); end >= 0 {
			slavePath = string(name[:end])
		}
	case "linux":
		var unlock int32
		if ioctlErr := task28SurfaceIoctl(fd, 0x40045431, uintptr(unsafe.Pointer(&unlock))); ioctlErr != nil { // TIOCSPTLCK
			return fail(ioctlErr)
		}
		var number uint32
		if ioctlErr := task28SurfaceIoctl(fd, 0x80045430, uintptr(unsafe.Pointer(&number))); ioctlErr != nil { // TIOCGPTN
			return fail(ioctlErr)
		}
		slavePath = "/dev/pts/" + fmt.Sprint(number)
	}
	if strings.TrimSpace(slavePath) == "" {
		return fail(errors.New("PTY slave path is empty"))
	}
	slave, err := os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		return fail(err)
	}
	return master, slave
}

func task28SurfaceIoctl(fd uintptr, request uintptr, argument uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, argument)
	if errno != 0 {
		return errno
	}
	return nil
}
