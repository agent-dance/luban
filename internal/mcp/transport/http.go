package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/i18n"
	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/protocol"
)

const (
	defaultRemoteRequestTimeout = 60 * time.Second
	remoteResponseBodyLimit     = 8 << 20
)

// HTTPTransportConfig captures the inputs for an MCP Streamable HTTP
// transport.
type HTTPTransportConfig struct {
	BaseURL string
	Headers map[string]string
	Auth    mcpauth.TokenSource
}

// httpTransport implements MCP Streamable HTTP on top of task_03's Transport
// interface. POST responses are decoded and pushed into Receive so the client
// owns JSON-RPC correlation.
type httpTransport struct {
	cfg HTTPTransportConfig

	client         *http.Client
	requestTimeout time.Duration
	runCtx         context.Context
	runCancel      context.CancelCauseFunc

	urlMu   sync.RWMutex
	baseURL string

	msgCh   chan protocol.JSONRPCMessage
	errCh   chan error
	closeCh chan struct{}

	closeOnce sync.Once
	errOnce   sync.Once
	closed    atomic.Bool
	closeMu   sync.RWMutex
	closeErr  error

	pumpMu     sync.Mutex
	pumpClosed bool
	pumpWG     sync.WaitGroup
}

// NewHTTPTransport constructs a Streamable HTTP transport. Per-POST timeouts
// are enforced with request contexts so long-lived SSE GET streams are not
// affected.
func NewHTTPTransport(cfg HTTPTransportConfig) (*httpTransport, error) {
	baseURL, err := normalizeHTTPTransportURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	runCtx, runCancel := context.WithCancelCause(context.Background())
	return &httpTransport{
		cfg:            cfg,
		client:         http.DefaultClient,
		requestTimeout: defaultRemoteRequestTimeout,
		runCtx:         runCtx,
		runCancel:      runCancel,
		baseURL:        baseURL,
		msgCh:          make(chan protocol.JSONRPCMessage, 64),
		errCh:          make(chan error, 1),
		closeCh:        make(chan struct{}),
	}, nil
}

// Send posts one JSON-RPC message. If the server returns a JSON-RPC response
// directly, Send delivers it to Receive instead of decoding it here.
func (t *httpTransport) Send(ctx context.Context, msg protocol.JSONRPCMessage) error {
	return t.sendToCurrentURL(ctx, msg)
}

// Receive returns messages delivered by POST responses or companion streams.
func (t *httpTransport) Receive(ctx context.Context) (protocol.JSONRPCMessage, error) {
	if t == nil {
		return protocol.JSONRPCMessage{}, NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "HTTP")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case err := <-t.errCh:
		return protocol.JSONRPCMessage{}, err
	default:
	}

	select {
	case msg := <-t.msgCh:
		return msg, nil
	case err := <-t.errCh:
		return protocol.JSONRPCMessage{}, err
	case <-ctx.Done():
		return protocol.JSONRPCMessage{}, ctx.Err()
	case <-t.closeCh:
		select {
		case err := <-t.errCh:
			return protocol.JSONRPCMessage{}, err
		default:
			return protocol.JSONRPCMessage{}, NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, t.closedCause(), "HTTP")
		}
	}
}

// Close is idempotent.
func (t *httpTransport) Close() error {
	if t == nil {
		return nil
	}
	t.shutdown(ErrTransportClosed)
	t.pumpWG.Wait()
	return nil
}

func (t *httpTransport) shutdown(cause error) {
	if cause == nil {
		cause = ErrTransportClosed
	}
	t.closeOnce.Do(func() {
		t.closeMu.Lock()
		t.closeErr = cause
		t.closeMu.Unlock()
		t.closed.Store(true)
		close(t.closeCh)
		t.pumpMu.Lock()
		t.pumpClosed = true
		t.pumpMu.Unlock()
		if t.runCancel != nil {
			t.runCancel(cause)
		}
	})
}

func (t *httpTransport) closedCause() error {
	if t == nil {
		return ErrTransportClosed
	}
	t.closeMu.RLock()
	cause := t.closeErr
	t.closeMu.RUnlock()
	if cause == nil {
		return ErrTransportClosed
	}
	return cause
}

func (t *httpTransport) sendToCurrentURL(ctx context.Context, msg protocol.JSONRPCMessage) error {
	return t.sendToURL(ctx, t.currentURL(), msg)
}

func (t *httpTransport) sendToURL(ctx context.Context, endpoint string, msg protocol.JSONRPCMessage) error {
	if t == nil || t.client == nil {
		return NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "HTTP")
	}
	if t.closed.Load() {
		return NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, t.closedCause(), "HTTP")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPHTTPOperationFailed, err, "marshal")
	}

	reqCtx, cancel := context.WithCancelCause(t.runCtx)
	stopCaller := context.AfterFunc(ctx, func() {
		cancel(ctx.Err())
	})
	var timeout *time.Timer
	if t.requestTimeout > 0 {
		timeout = time.AfterFunc(t.requestTimeout, func() {
			cancel(context.DeadlineExceeded)
		})
	}
	detachRequest := func() {
		stopCaller()
		if timeout != nil {
			timeout.Stop()
		}
	}
	finishRequest := func() {
		detachRequest()
		cancel(context.Canceled)
	}
	requestOwned := false
	defer func() {
		if !requestOwned {
			finishRequest()
		}
	}()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPHTTPOperationFailed, err, "build POST")
	}
	headers, err := t.headers(reqCtx, endpoint)
	if err != nil {
		return err
	}
	applyRemoteHeaders(req, headers)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", streamableHTTPAccept)

	resp, err := t.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cause := context.Cause(reqCtx); cause != nil {
			if t.closed.Load() {
				return NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, cause, "HTTP")
			}
			return cause
		}
		return i18n.WrapError(i18n.KeyServicesMCPHTTPOperationFailed, err, "POST")
	}
	responseOwned := false
	defer func() {
		if !responseOwned {
			_ = resp.Body.Close()
		}
	}()

	streamResponse, err := t.handleHTTPResponse(endpoint, resp)
	if err != nil {
		return err
	}
	if !streamResponse {
		return nil
	}

	first, err := t.startResponsePump(reqCtx, resp.Body, finishRequest)
	if err != nil {
		return err
	}
	requestOwned = true
	responseOwned = true
	if len(bytes.TrimSpace(msg.ID)) == 0 {
		detachRequest()
		return nil
	}

	select {
	case firstErr := <-first:
		if firstErr != nil {
			if t.closed.Load() {
				return NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, firstErr, "HTTP")
			}
			return firstErr
		}
		detachRequest()
		return nil
	case <-ctx.Done():
		cancel(ctx.Err())
		return ctx.Err()
	case <-t.closeCh:
		return NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, t.closedCause(), "HTTP")
	}
}

func (t *httpTransport) handleHTTPResponse(endpoint string, resp *http.Response) (bool, error) {
	if resp.StatusCode == http.StatusUnauthorized {
		return false, &mcpauth.UnauthorizedError{
			Challenge:  mcpauth.ParseWWWAuthenticate(resp),
			ServerURL:  endpoint,
			StatusCode: resp.StatusCode,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, remoteResponseBodyLimit))
		return false, &RemoteHTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			ServerURL:  endpoint,
			Body:       string(raw),
			RPCError:   parseRPCError(raw),
		}
	}
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusAccepted {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return false, nil
	}

	mediaType := responseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == "text/event-stream" {
		return true, nil
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, remoteResponseBodyLimit))
	if err != nil {
		return false, i18n.WrapError(i18n.KeyServicesMCPHTTPOperationFailed, err, "read response")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return false, nil
	}
	return false, t.deliverJSONPayload(raw)
}

func (t *httpTransport) startResponsePump(
	ctx context.Context,
	body io.ReadCloser,
	finish func(),
) (<-chan error, error) {
	first := make(chan error, 1)
	t.pumpMu.Lock()
	if t.pumpClosed || t.closed.Load() {
		t.pumpMu.Unlock()
		return nil, NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, t.closedCause(), "HTTP")
	}
	t.pumpWG.Add(1)
	t.pumpMu.Unlock()

	go func() {
		defer t.pumpWG.Done()
		defer finish()
		defer body.Close()

		var firstOnce sync.Once
		delivered := false
		signalFirst := func(err error) {
			firstOnce.Do(func() {
				first <- err
			})
		}
		err := t.readSSEEventsObserved(ctx, body, func() {
			delivered = true
			signalFirst(nil)
		})
		if err == nil || (delivered && errors.Is(err, io.EOF)) {
			signalFirst(nil)
			return
		}
		if cause := context.Cause(ctx); cause != nil {
			signalFirst(cause)
			return
		}
		signalFirst(err)
		if delivered && !t.closed.Load() {
			t.fail(NewTransportClosedError(i18n.KeyServicesMCPTransportStreamFailedReason, err, "HTTP"))
		}
	}()
	return first, nil
}

func (t *httpTransport) deliverJSONPayload(raw []byte) error {
	var msg protocol.JSONRPCMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPHTTPOperationFailed, err, "decode response")
	}
	return t.deliver(msg)
}

func (t *httpTransport) deliver(msg protocol.JSONRPCMessage) error {
	if t == nil {
		return NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "HTTP")
	}
	select {
	case t.msgCh <- msg:
		return nil
	case <-t.closeCh:
		return NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, t.closedCause(), "HTTP")
	}
}

func (t *httpTransport) fail(err error) {
	if t == nil || err == nil {
		return
	}
	t.errOnce.Do(func() {
		select {
		case t.errCh <- err:
		default:
		}
		t.shutdown(err)
	})
}

func (t *httpTransport) headers(ctx context.Context, endpoint string) (map[string]string, error) {
	return resolveRemoteHeaders(ctx, remoteHeaderConfig{
		ServerURL: endpoint,
		Headers:   t.cfg.Headers,
		Auth:      t.cfg.Auth,
	})
}

func (t *httpTransport) currentURL() string {
	t.urlMu.RLock()
	defer t.urlMu.RUnlock()
	return t.baseURL
}

func (t *httpTransport) setCurrentURL(raw string) error {
	normalized, err := normalizeHTTPTransportURL(raw)
	if err != nil {
		return err
	}
	t.urlMu.Lock()
	t.baseURL = normalized
	t.urlMu.Unlock()
	return nil
}

func normalizeHTTPTransportURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", i18n.NewError(i18n.KeyMCPHTTPBaseURLRequired)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", i18n.NewError(i18n.KeyMCPHTTPURLInvalid, raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", i18n.NewError(i18n.KeyMCPHTTPSchemeInvalid, parsed.Scheme)
	}
	return parsed.String(), nil
}

func responseMediaType(contentType string) string {
	if contentType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	}
	return strings.ToLower(mediaType)
}

func parseRPCError(raw []byte) *protocol.RPCError {
	var envelope struct {
		Error *protocol.RPCError `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil
	}
	return envelope.Error
}
