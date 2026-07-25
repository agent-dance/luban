//go:build windows

package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync/atomic"

	"github.com/agent-dance/luban/i18n"
	"golang.org/x/sys/windows"
)

var cancelScreenReaderSynchronousIO = windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelSynchronousIo")

func scanScreenReaderInput(input io.Reader, lines chan<- screenReaderLine, stop <-chan struct{}, done chan<- struct{}, sequence *atomic.Uint64) {
	file, ok := input.(*os.File)
	if !ok {
		scanScreenReaderInputGeneric(input, lines, stop, done, sequence)
		return
	}
	defer close(done)
	defer close(lines)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	process := windows.CurrentProcess()
	var thread windows.Handle
	if err := windows.DuplicateHandle(process, windows.CurrentThread(), process, &thread, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		emitScreenReaderWindowsLine(lines, stop, sequence, screenReaderLine{err: fmt.Errorf("%s: %w", i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderDuplicateHandleFailed), err)})
		return
	}
	defer windows.CloseHandle(thread)
	readLoopDone := make(chan struct{})
	cancelWorkerDone := make(chan struct{})
	go func() {
		defer close(cancelWorkerDone)
		cancelScreenReaderReadUntilDone(stop, readLoopDone, func() {
			_, _, _ = cancelScreenReaderSynchronousIO.Call(uintptr(thread))
			_ = windows.CancelIoEx(windows.Handle(file.Fd()), nil)
		})
	}()
	defer func() {
		close(readLoopDone)
		<-cancelWorkerDone
	}()

	pending := make([]byte, 0, 4096)
	buffer := make([]byte, 4096)
	for {
		count, readErr := file.Read(buffer)
		if count > 0 {
			pending = append(pending, buffer[:count]...)
			for {
				newline := bytes.IndexByte(pending, '\n')
				if newline < 0 {
					break
				}
				line := bytes.TrimSuffix(pending[:newline], []byte{'\r'})
				if !emitScreenReaderWindowsLine(lines, stop, sequence, screenReaderLine{text: string(line)}) {
					return
				}
				pending = pending[newline+1:]
			}
		}
		if readErr == nil {
			continue
		}
		select {
		case <-stop:
			return
		default:
		}
		if errors.Is(readErr, io.EOF) {
			if len(pending) > 0 {
				emitScreenReaderWindowsLine(lines, stop, sequence, screenReaderLine{text: string(bytes.TrimSuffix(pending, []byte{'\r'}))})
			}
			return
		}
		emitScreenReaderWindowsLine(lines, stop, sequence, screenReaderLine{err: fmt.Errorf("%s: %w", i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderReadInputFailed), readErr)})
		return
	}
}

func emitScreenReaderWindowsLine(lines chan<- screenReaderLine, stop <-chan struct{}, sequence *atomic.Uint64, line screenReaderLine) bool {
	line.sequence = sequence.Add(1)
	select {
	case lines <- line:
		return true
	case <-stop:
		return false
	}
}
