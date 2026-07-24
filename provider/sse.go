package provider

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// sseEvent represents a single Server-Sent Event.
type sseEvent struct {
	Type string // event type (from "event:" line)
	Data string // data payload (from "data:" line)
}

const (
	defaultSSEInitBuf = 64 * 1024       // 64KB initial buffer
	defaultSSEMaxBuf  = 4 * 1024 * 1024 // 4MB max buffer (raised from 1MB)
)

// parseSSE reads Server-Sent Events from a reader using the default buffer size.
// Yields events through the returned channel. Closes channel on EOF or error.
func parseSSE(r io.Reader) <-chan sseEvent {
	return parseSSEWithBuffer(r, defaultSSEMaxBuf)
}

// parseSSEWithBuffer reads Server-Sent Events from a reader with a configurable
// maximum scanner buffer size. When the scanner encounters a line exceeding
// maxBufSize, it sends an error event (type "error") instead of silently terminating.
func parseSSEWithBuffer(r io.Reader, maxBufSize int) <-chan sseEvent {
	ch := make(chan sseEvent, 64)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(r)
		initBuf := defaultSSEInitBuf
		if initBuf > maxBufSize {
			initBuf = maxBufSize
		}
		scanner.Buffer(make([]byte, initBuf), maxBufSize)

		var currentType string
		var dataLines []string

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				// Empty line = event boundary
				if len(dataLines) > 0 {
					ch <- sseEvent{
						Type: currentType,
						Data: strings.Join(dataLines, "\n"),
					}
				}
				currentType = ""
				dataLines = nil
				continue
			}

			if strings.HasPrefix(line, "event:") {
				currentType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				data := strings.TrimPrefix(line, "data:")
				data = strings.TrimSpace(data)
				if data == "[DONE]" {
					return
				}
				dataLines = append(dataLines, data)
			}
			// ignore "id:", "retry:", comments starting with ":"
		}

		if err := scanner.Err(); err != nil {
			ch <- sseEvent{Type: "error", Data: fmt.Sprintf("SSE scanner error: %v", err)}
		}
	}()
	return ch
}
