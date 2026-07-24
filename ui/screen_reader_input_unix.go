//go:build unix

package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/agent-dance/luban/i18n"
	"golang.org/x/sys/unix"
)

func scanScreenReaderInput(input io.Reader, lines chan<- screenReaderLine, stop <-chan struct{}, done chan<- struct{}, sequence *atomic.Uint64) {
	file, ok := input.(*os.File)
	if !ok {
		scanScreenReaderInputGeneric(input, lines, stop, done, sequence)
		return
	}
	defer close(done)
	defer close(lines)
	fd := int32(file.Fd())
	pending := make([]byte, 0, 4096)
	buffer := make([]byte, 4096)
	emit := func(line screenReaderLine) bool {
		line.sequence = sequence.Add(1)
		select {
		case lines <- line:
			return true
		case <-stop:
			return false
		}
	}
	for {
		select {
		case <-stop:
			return
		default:
		}
		pollFDs := []unix.PollFd{{Fd: fd, Events: unix.POLLIN | unix.POLLHUP}}
		n, err := unix.Poll(pollFDs, 100)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			emit(screenReaderLine{err: fmt.Errorf("%s: %w", i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderPollInputFailed), err)})
			return
		}
		if n == 0 {
			continue
		}
		count, readErr := unix.Read(int(fd), buffer)
		if count > 0 {
			pending = append(pending, buffer[:count]...)
			for {
				newline := bytes.IndexByte(pending, '\n')
				if newline < 0 {
					break
				}
				line := pending[:newline]
				if len(line) > 0 && line[len(line)-1] == '\r' {
					line = line[:len(line)-1]
				}
				if !emit(screenReaderLine{text: string(line)}) {
					return
				}
				pending = pending[newline+1:]
			}
		}
		if readErr == unix.EINTR || readErr == unix.EAGAIN {
			continue
		}
		if readErr != nil {
			emit(screenReaderLine{err: fmt.Errorf("%s: %w", i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderReadInputFailed), readErr)})
			return
		}
		if count == 0 {
			if len(pending) > 0 {
				emit(screenReaderLine{text: string(bytes.TrimSuffix(pending, []byte{'\r'}))})
			}
			return
		}
	}
}
