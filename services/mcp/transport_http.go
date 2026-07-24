package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const (
	defaultRemoteRequestTimeout = 60 * time.Second
	remoteResponseBodyLimit     = 8 << 20
)

// HTTPTransportConfig captures the inputs for an MCP Streamable HTTP
// transport.
type HTTPTransportConfig struct {
	BaseURL        string
	HTTPClient     *http.Client
	Headers        map[string]string
	HeaderProvider HeaderProvider
	Auth           TokenSource
	ServerName     string
	RequestTimeout time.Duration
	UserAgent      string
}

// HTTPTransport implements MCP Streamable HTTP on top of task_03's Transport
// interface. POST responses are decoded and pushed into Receive so the client
// owns JSON-RPC correlation.
type HTTPTransport struct {
	cfg HTTPTransportConfig

	client         *http.Client
	requestTimeout time.Duration

	urlMu   sync.RWMutex
	baseURL string

	msgCh   chan JSONRPCMessage
	errCh   chan error
	closeCh chan struct{}

	closeOnce sync.Once
	errOnce   sync.Once
	closed    atomic.Bool
}

// NewHTTPTransport constructs a Streamable HTTP transport. A nil HTTPClient
// defaults to a client with no global timeout; per-POST timeouts are enforced
// with request contexts so long-lived SSE GET streams are not affected.
func NewHTTPTransport(cfg HTTPTransportConfig) (*HTTPTransport, error) {
	baseURL, err := normalizeHTTPTransportURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRemoteRequestTimeout
	}
	return &HTTPTransport{
		cfg:            cfg,
		client:         client,
		requestTimeout: timeout,
		baseURL:        baseURL,
		msgCh:          make(chan JSONRPCMessage, 64),
		errCh:          make(chan error, 1),
		closeCh:        make(chan struct{}),
	}, nil
}

// Send posts one JSON-RPC message. If the server returns a JSON-RPC response
// directly, Send delivers it to Receive instead of decoding it here.
func (t *HTTPTransport) Send(ctx context.Context, msg JSONRPCMessage) error {
	return t.sendToCurrentURL(ctx, msg)
}

// Receive returns messages delivered by POST responses or companion streams.
func (t *HTTPTransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	if t == nil {
		return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "HTTP")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case err := <-t.errCh:
		return JSONRPCMessage{}, err
	default:
	}

	select {
	case msg := <-t.msgCh:
		return msg, nil
	case err := <-t.errCh:
		return JSONRPCMessage{}, err
	case <-ctx.Done():
		return JSONRPCMessage{}, ctx.Err()
	case <-t.closeCh:
		select {
		case err := <-t.errCh:
			return JSONRPCMessage{}, err
		default:
			return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "HTTP")
		}
	}
}

// Close is idempotent.
func (t *HTTPTransport) Close() error {
	if t == nil {
		return nil
	}
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		close(t.closeCh)
	})
	return nil
}

func (t *HTTPTransport) sendToCurrentURL(ctx context.Context, msg JSONRPCMessage) error {
	return t.sendToURL(ctx, t.currentURL(), msg)
}

func (t *HTTPTransport) sendToURL(ctx context.Context, endpoint string, msg JSONRPCMessage) error {
	if t == nil || t.client == nil {
		return newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "HTTP")
	}
	if t.closed.Load() {
		return newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "HTTP")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPHTTPOperationFailed, err, "marshal")
	}

	reqCtx := ctx
	cancel := func() {}
	if t.requestTimeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, t.requestTimeout)
	}
	defer cancel()

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
	req.Header.Set("Accept", MCPStreamableHTTPAccept)

	resp, err := t.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if reqCtx.Err() != nil {
			return reqCtx.Err()
		}
		return i18n.WrapError(i18n.KeyServicesMCPHTTPOperationFailed, err, "POST")
	}
	defer resp.Body.Close()

	if err := t.handleHTTPResponse(reqCtx, endpoint, resp); err != nil {
		return err
	}
	return nil
}

func (t *HTTPTransport) handleHTTPResponse(ctx context.Context, endpoint string, resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return &UnauthorizedError{
			Challenge:  ParseWWWAuthenticate(resp),
			ServerURL:  endpoint,
			StatusCode: resp.StatusCode,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, remoteResponseBodyLimit))
		return &RemoteHTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			ServerURL:  endpoint,
			Body:       string(raw),
			RPCError:   parseRPCError(raw),
		}
	}
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusAccepted {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return nil
	}

	mediaType := responseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == "text/event-stream" {
		return t.readSSEEvents(ctx, resp.Body)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, remoteResponseBodyLimit))
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPHTTPOperationFailed, err, "read response")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return t.deliverJSONPayload(raw)
}

func (t *HTTPTransport) deliverJSONPayload(raw []byte) error {
	var msg JSONRPCMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPHTTPOperationFailed, err, "decode response")
	}
	return t.deliver(msg)
}

func (t *HTTPTransport) deliver(msg JSONRPCMessage) error {
	if t == nil {
		return newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "HTTP")
	}
	select {
	case t.msgCh <- msg:
		return nil
	case <-t.closeCh:
		return newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "HTTP")
	}
}

func (t *HTTPTransport) fail(err error) {
	if t == nil || err == nil {
		return
	}
	t.errOnce.Do(func() {
		select {
		case t.errCh <- err:
		default:
		}
		_ = t.Close()
	})
}

func (t *HTTPTransport) headers(ctx context.Context, endpoint string) (map[string]string, error) {
	return resolveRemoteHeaders(ctx, remoteHeaderConfig{
		ServerName:     t.cfg.ServerName,
		ServerURL:      endpoint,
		UserAgent:      t.cfg.UserAgent,
		Headers:        t.cfg.Headers,
		HeaderProvider: t.cfg.HeaderProvider,
		Auth:           t.cfg.Auth,
	})
}

func (t *HTTPTransport) currentURL() string {
	t.urlMu.RLock()
	defer t.urlMu.RUnlock()
	return t.baseURL
}

func (t *HTTPTransport) setCurrentURL(raw string) error {
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

func parseRPCError(raw []byte) *RPCError {
	var envelope struct {
		Error *RPCError `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil
	}
	return envelope.Error
}
