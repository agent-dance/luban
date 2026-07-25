package transport

import (
	"context"
	"errors"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/mcp/protocol"
)

// Transport is the minimal message transport required by Client.
type Transport interface {
	Send(context.Context, protocol.JSONRPCMessage) error
	Receive(context.Context) (protocol.JSONRPCMessage, error)
	Close() error
}

// ErrTransportClosed is the sentinel used when a transport close rejects
// pending MCP calls.
var ErrTransportClosed = i18n.NewError(i18n.KeyServicesMCPTransportClosed)

// TransportClosedError carries the concrete close/receive cause while still
// matching ErrTransportClosed via errors.Is.
type TransportClosedError struct {
	reason     string
	err        error
	reasonKey  i18n.Key
	reasonArgs []any
	reasonFunc func() string
}

// Error implements error.
func (e *TransportClosedError) Error() string {
	if e == nil {
		return ErrTransportClosed.Error()
	}
	reason := e.reason
	if e.reasonFunc != nil {
		reason = e.reasonFunc()
	} else if e.reasonKey != "" {
		reason = i18n.Format(i18n.DetectOrLoadLanguage(), e.reasonKey, e.reasonArgs...)
	}
	if reason == "" {
		return ErrTransportClosed.Error()
	}
	if e.err != nil && !errors.Is(e.err, ErrTransportClosed) {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPTransportClosedReasonCause, reason, e.err)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPTransportClosedReason, reason)
}

func newTransportClosedErrorFunc(reason func() string, cause error) *TransportClosedError {
	return &TransportClosedError{err: cause, reasonFunc: reason}
}

// NewTransportClosedError constructs a localized transport close error.
func NewTransportClosedError(reasonKey i18n.Key, cause error, reasonArgs ...any) *TransportClosedError {
	return &TransportClosedError{
		err:        cause,
		reasonKey:  reasonKey,
		reasonArgs: append([]any(nil), reasonArgs...),
	}
}

// Unwrap returns the concrete transport cause.
func (e *TransportClosedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// Is makes errors.Is(err, ErrTransportClosed) true even when Err wraps a
// lower-level IO or context error.
func (e *TransportClosedError) Is(target error) bool {
	return target == ErrTransportClosed
}
