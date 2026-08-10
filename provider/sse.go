package provider

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// sseEvent represents a single Server-Sent Event.
type sseEvent struct {
	Type string // event type (from "event:" line)
	Data string // data payload (from "data:" line)
	Err  error  // scanner/transport failure; never provider-controlled content
}

const (
	defaultSSEInitBuf = 64 * 1024       // 64KB initial buffer
	defaultSSEMaxBuf  = 4 * 1024 * 1024 // 4MB max buffer (raised from 1MB)
	// Responses gateways may aggregate a runaway function argument into one SSE
	// data line. Waiting for the generic 4 MiB scanner ceiling defeats the
	// decoded per-tool limits because the reducer cannot inspect an incomplete
	// event. Function tools are capped at 64 KiB or less, so 160 KiB preserves
	// JSON-escape headroom while terminating a degenerate line early.
	maxResponsesFunctionCallDeltaLineBytes = 160 * 1024
)

var errResponsesFunctionCallDeltaLineTooLarge = errors.New("responses function-call delta line exceeds transport bound")

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
			ch <- sseEvent{Type: "error", Data: fmt.Sprintf("SSE scanner error: %v", err), Err: err}
		}
	}()
	return ch
}

// parseResponsesSSE applies an event-aware line ceiling before the generic SSE
// decoder has buffered an entire upstream event. Other Responses events retain
// the 4 MiB compatibility ceiling, including large custom ApplyPatch payloads
// and encrypted reasoning/output-item envelopes.
func parseResponsesSSE(ctx context.Context, r io.Reader) <-chan sseEvent {
	ch := make(chan sseEvent, 64)
	go func() {
		defer close(ch)
		reader := bufio.NewReaderSize(r, defaultSSEInitBuf)
		currentType := ""
		dataLines := make([]string, 0, 1)
		send := func(event sseEvent) bool {
			select {
			case ch <- event:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for {
			line, err := readResponsesSSELine(reader, currentType)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					send(sseEvent{Type: "error", Err: err})
				}
				return
			}
			if len(line) == 0 {
				if len(dataLines) > 0 && !send(sseEvent{Type: currentType, Data: strings.Join(dataLines, "\n")}) {
					return
				}
				currentType = ""
				dataLines = dataLines[:0]
				continue
			}
			switch {
			case bytes.HasPrefix(line, []byte("event:")):
				currentType = strings.TrimSpace(string(bytes.TrimPrefix(line, []byte("event:"))))
			case bytes.HasPrefix(line, []byte("data:")):
				data := strings.TrimSpace(string(bytes.TrimPrefix(line, []byte("data:"))))
				if data == "[DONE]" {
					return
				}
				dataLines = append(dataLines, data)
			}
		}
	}()
	return ch
}

func readResponsesSSELine(reader *bufio.Reader, eventType string) ([]byte, error) {
	limit := defaultSSEMaxBuf
	if eventType == "response.function_call_arguments.delta" {
		limit = maxResponsesFunctionCallDeltaLineBytes
	}
	line := make([]byte, 0, min(defaultSSEInitBuf, limit))
	for {
		fragment, err := reader.ReadSlice('\n')
		line = append(line, fragment...)
		if eventType == "" && responsesFunctionCallDeltaTypeInDataLine(line) {
			limit = maxResponsesFunctionCallDeltaLineBytes
		}
		if len(line) > limit {
			return nil, errResponsesFunctionCallDeltaLineTooLarge
		}
		switch err {
		case nil:
			line = bytes.TrimSuffix(line, []byte{'\n'})
			line = bytes.TrimSuffix(line, []byte{'\r'})
			return line, nil
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			if len(line) == 0 {
				return nil, io.EOF
			}
			return line, nil
		default:
			return nil, err
		}
	}
}

func responsesFunctionCallDeltaTypeInDataLine(line []byte) bool {
	return bytes.Contains(line, []byte("response.function_call_arguments.delta"))
}
