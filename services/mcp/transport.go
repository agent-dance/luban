package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

// JSONRPCVersion is the protocol version marker used by MCP transports.
const JSONRPCVersion = "2.0"

// JSONRPCMessage is the transport-neutral JSON-RPC 2.0 envelope used by the
// services-layer client. Result and Params stay raw so MCP result envelopes are
// not flattened or schema-stripped by the transport layer.
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error object returned by an MCP server.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements error.
func (e *RPCError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPJSONRPCError)
	}
	if e.Message == "" {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPJSONRPCErrorCode, e.Code)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPJSONRPCErrorDetail, e.Code, e.Message)
}

// Transport reads and writes JSON-RPC messages for a single MCP connection.
// Concrete transports may be stdio line framing, streamable HTTP/SSE,
// WebSocket, or in-process pairs; the client only depends on this contract.
type Transport interface {
	Send(context.Context, JSONRPCMessage) error
	Receive(context.Context) (JSONRPCMessage, error)
	Close() error
}

// ErrTransportClosed is the sentinel used when a transport close rejects
// pending MCP calls.
var ErrTransportClosed = i18n.NewError(i18n.KeyServicesMCPTransportClosed)

// TransportClosedError carries the concrete close/receive cause while still
// matching ErrTransportClosed via errors.Is.
type TransportClosedError struct {
	Reason string
	Err    error

	reasonKey  i18n.Key
	reasonArgs []any
	reasonFunc func() string
}

// Error implements error.
func (e *TransportClosedError) Error() string {
	if e == nil {
		return ErrTransportClosed.Error()
	}
	reason := e.Reason
	if e.reasonFunc != nil {
		reason = e.reasonFunc()
	} else if e.reasonKey != "" {
		reason = i18n.Format(i18n.DetectOrLoadLanguage(), e.reasonKey, e.reasonArgs...)
	}
	if reason == "" {
		return ErrTransportClosed.Error()
	}
	if e.Err != nil && !errors.Is(e.Err, ErrTransportClosed) {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPTransportClosedReasonCause, reason, e.Err)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPTransportClosedReason, reason)
}

func newTransportClosedErrorFunc(reason func() string, cause error) *TransportClosedError {
	return &TransportClosedError{Err: cause, reasonFunc: reason}
}

func newTransportClosedError(reasonKey i18n.Key, cause error, reasonArgs ...any) *TransportClosedError {
	return &TransportClosedError{
		Err:        cause,
		reasonKey:  reasonKey,
		reasonArgs: append([]any(nil), reasonArgs...),
	}
}

// Unwrap returns the concrete transport cause.
func (e *TransportClosedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Is makes errors.Is(err, ErrTransportClosed) true even when Err wraps a
// lower-level IO or context error.
func (e *TransportClosedError) Is(target error) bool {
	return target == ErrTransportClosed
}

// NewRequestMessage builds a JSON-RPC request envelope.
func NewRequestMessage(id int64, method string, params any) (JSONRPCMessage, error) {
	if method == "" {
		return JSONRPCMessage{}, errors.New("services/mcp: JSON-RPC request missing method")
	}
	idRaw, err := json.Marshal(id)
	if err != nil {
		return JSONRPCMessage{}, err
	}
	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      idRaw,
		Method:  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return JSONRPCMessage{}, fmt.Errorf("services/mcp: marshal request params: %w", err)
		}
		msg.Params = raw
	}
	return msg, nil
}

// NewNotificationMessage builds a JSON-RPC notification envelope.
func NewNotificationMessage(method string, params any) (JSONRPCMessage, error) {
	if method == "" {
		return JSONRPCMessage{}, errors.New("services/mcp: JSON-RPC notification missing method")
	}
	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return JSONRPCMessage{}, fmt.Errorf("services/mcp: marshal notification params: %w", err)
		}
		msg.Params = raw
	}
	return msg, nil
}

// NewResultMessage builds a JSON-RPC response with a result payload.
func NewResultMessage(id json.RawMessage, result any) (JSONRPCMessage, error) {
	if len(bytes.TrimSpace(id)) == 0 {
		return JSONRPCMessage{}, errors.New("services/mcp: JSON-RPC response missing id")
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return JSONRPCMessage{}, fmt.Errorf("services/mcp: marshal response result: %w", err)
	}
	return JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      cloneRawMessage(id),
		Result:  raw,
	}, nil
}

// NewErrorMessage builds a JSON-RPC error response.
func NewErrorMessage(id json.RawMessage, code int, message string, data any) (JSONRPCMessage, error) {
	if len(bytes.TrimSpace(id)) == 0 {
		return JSONRPCMessage{}, errors.New("services/mcp: JSON-RPC error response missing id")
	}
	rpcErr := &RPCError{Code: code, Message: message}
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return JSONRPCMessage{}, fmt.Errorf("services/mcp: marshal error data: %w", err)
		}
		rpcErr.Data = raw
	}
	return JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      cloneRawMessage(id),
		Error:   rpcErr,
	}, nil
}

// LineTransport is a newline-delimited JSON-RPC transport. It is intentionally
// small and transport-neutral; stdio process lifecycle remains in
// transport_stdio.go.
type LineTransport struct {
	r      *bufio.Reader
	w      io.Writer
	closer io.Closer

	readMu  sync.Mutex
	writeMu sync.Mutex
}

// NewLineTransport wraps an io.Reader/io.Writer pair as a JSON-RPC message
// transport. The optional closer is called from Close.
func NewLineTransport(r io.Reader, w io.Writer, closer io.Closer) *LineTransport {
	return &LineTransport{
		r:      bufio.NewReader(r),
		w:      w,
		closer: closer,
	}
}

// Send writes one newline-delimited JSON-RPC message.
func (t *LineTransport) Send(ctx context.Context, msg JSONRPCMessage) error {
	if t == nil || t.w == nil {
		return newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "line")
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
func (t *LineTransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	if t == nil || t.r == nil {
		return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "line")
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
		return JSONRPCMessage{}, ctx.Err()
	case result := <-done:
		if result.err != nil {
			if errors.Is(result.err, io.EOF) || errors.Is(result.err, io.ErrClosedPipe) {
				return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportEOFReason, result.err, "line transport")
			}
			return JSONRPCMessage{}, i18n.WrapError(i18n.KeyServicesMCPLineOperationFailed, result.err, "read")
		}
		var msg JSONRPCMessage
		if err := json.Unmarshal(bytes.TrimSpace(result.line), &msg); err != nil {
			return JSONRPCMessage{}, i18n.WrapError(i18n.KeyServicesMCPLineOperationFailed, err, "decode")
		}
		return msg, nil
	}
}

// Close closes the underlying closer when one was provided.
func (t *LineTransport) Close() error {
	if t == nil || t.closer == nil {
		return nil
	}
	return t.closer.Close()
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	cp := make(json.RawMessage, len(raw))
	copy(cp, raw)
	return cp
}
