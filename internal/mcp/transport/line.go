package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/mcp/protocol"
)

// lineTransport is a newline-delimited JSON-RPC transport. It is intentionally
// small and transport-neutral; stdio process lifecycle remains in
// transport_stdio.go.
type lineTransport struct {
	r      *bufio.Reader
	w      io.Writer
	closer io.Closer

	readMu  sync.Mutex
	writeMu sync.Mutex
}

// newLineTransport wraps an io.Reader/io.Writer pair as a JSON-RPC message
// transport. The optional closer is called from Close.
func newLineTransport(r io.Reader, w io.Writer, closer io.Closer) *lineTransport {
	return &lineTransport{
		r:      bufio.NewReader(r),
		w:      w,
		closer: closer,
	}
}

// Send writes one newline-delimited JSON-RPC message.
func (t *lineTransport) Send(ctx context.Context, msg protocol.JSONRPCMessage) error {
	if t == nil || t.w == nil {
		return NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "line")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPLineOperationFailed, err, "marshal")
	}
	data = append(data, '\n')

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	done := make(chan error, 1)
	go func() {
		_, err := t.w.Write(data)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return i18n.WrapError(i18n.KeyServicesMCPLineOperationFailed, err, "write")
		}
		return nil
	}
}

// Receive reads one newline-delimited JSON-RPC message.
func (t *lineTransport) Receive(ctx context.Context) (protocol.JSONRPCMessage, error) {
	if t == nil || t.r == nil {
		return protocol.JSONRPCMessage{}, NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "line")
	}

	t.readMu.Lock()
	defer t.readMu.Unlock()

	type readResult struct {
		line []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		line, err := t.r.ReadBytes('\n')
		done <- readResult{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		return protocol.JSONRPCMessage{}, ctx.Err()
	case result := <-done:
		if result.err != nil {
			if errors.Is(result.err, io.EOF) || errors.Is(result.err, io.ErrClosedPipe) {
				return protocol.JSONRPCMessage{}, NewTransportClosedError(i18n.KeyServicesMCPTransportEOFReason, result.err, "line transport")
			}
			return protocol.JSONRPCMessage{}, i18n.WrapError(i18n.KeyServicesMCPLineOperationFailed, result.err, "read")
		}
		var msg protocol.JSONRPCMessage
		if err := json.Unmarshal(bytes.TrimSpace(result.line), &msg); err != nil {
			return protocol.JSONRPCMessage{}, i18n.WrapError(i18n.KeyServicesMCPLineOperationFailed, err, "decode")
		}
		return msg, nil
	}
}

// Close closes the underlying closer when one was provided.
func (t *lineTransport) Close() error {
	if t == nil || t.closer == nil {
		return nil
	}
	return t.closer.Close()
}
