package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// SSEConfig captures the inputs for a Server-Sent Events transport.
type SSEConfig struct {
	BaseURL        string
	PostURL        string
	HTTPClient     *http.Client
	Headers        map[string]string
	HeaderProvider HeaderProvider
	Auth           TokenSource
	ServerName     string
	RequestTimeout time.Duration
	UserAgent      string
}

// SSETransport implements the MCP SSE transport: a long-lived GET stream for
// server messages plus POST for client messages.
type SSETransport struct {
	*HTTPTransport

	streamURL string

	streamOnce   sync.Once
	streamCtx    context.Context
	streamCancel context.CancelFunc
	legacyID     atomic.Int64
}

// NewSSETransport constructs an SSE transport. The SSE GET is opened lazily
// from Receive so legacy CallRaw-only callers can still receive typed POST
// errors without racing the stream.
func NewSSETransport(cfg SSEConfig) (*SSETransport, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, i18n.NewError(i18n.KeyMCPSSEBaseURLRequired)
	}
	postURL := strings.TrimSpace(cfg.PostURL)
	if postURL == "" {
		postURL = cfg.BaseURL
	}
	httpTransport, err := NewHTTPTransport(HTTPTransportConfig{
		BaseURL:        postURL,
		HTTPClient:     cfg.HTTPClient,
		Headers:        cfg.Headers,
		HeaderProvider: cfg.HeaderProvider,
		Auth:           cfg.Auth,
		ServerName:     cfg.ServerName,
		RequestTimeout: cfg.RequestTimeout,
		UserAgent:      cfg.UserAgent,
	})
	if err != nil {
		return nil, err
	}
	streamURL, err := normalizeHTTPTransportURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	streamCtx, streamCancel := context.WithCancel(context.Background())
	return &SSETransport{
		HTTPTransport: httpTransport,
		streamURL:     streamURL,
		streamCtx:     streamCtx,
		streamCancel:  streamCancel,
	}, nil
}

// Receive starts the companion SSE stream once and then returns messages from
// either the GET stream or POST responses.
func (t *SSETransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	if t == nil {
		return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "SSE")
	}
	t.startStream()
	return t.HTTPTransport.Receive(ctx)
}

// Close is idempotent and cancels the long-lived GET stream.
func (t *SSETransport) Close() error {
	if t == nil {
		return nil
	}
	t.streamCancel()
	return t.HTTPTransport.Close()
}

// CallRaw preserves the pre-task_03 RawCaller compatibility path used by older
// tests and tool wiring. Transport-backed clients should use NewProtocolClient.
func (t *SSETransport) CallRaw(ctx context.Context, method string, params, out any) error {
	if t == nil {
		return i18n.NewError(i18n.KeyServicesMCPSSENotInitialized)
	}
	id := t.legacyID.Add(1)
	msg, err := NewRequestMessage(id, method, params)
	if err != nil {
		return err
	}
	if err := t.Send(ctx, msg); err != nil {
		return err
	}
	key := fmt.Sprintf("%d", id)
	for {
		resp, err := t.HTTPTransport.Receive(ctx)
		if err != nil {
			return err
		}
		if messageIDKey(resp.ID) != key {
			continue
		}
		if resp.Error != nil {
			return resp.Error
		}
		if out == nil {
			return nil
		}
		if rawOut, ok := out.(*json.RawMessage); ok {
			*rawOut = cloneRawMessage(resp.Result)
			return nil
		}
		if len(bytes.TrimSpace(resp.Result)) == 0 {
			return nil
		}
		return json.Unmarshal(resp.Result, out)
	}
}

func (t *SSETransport) startStream() {
	t.streamOnce.Do(func() {
		go func() {
			err := t.readEventStream(t.streamCtx)
			if err == nil || errors.Is(err, context.Canceled) || t.closed.Load() {
				return
			}
			t.fail(newTransportClosedError(i18n.KeyServicesMCPTransportStreamFailedReason, err, "SSE"))
		}()
	})
}

func (t *SSETransport) readEventStream(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.streamURL, nil)
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPSSEOperationFailed, err, "build GET")
	}
	headers, err := t.headers(ctx, t.streamURL)
	if err != nil {
		return err
	}
	applyRemoteHeaders(req, headers)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := t.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return i18n.WrapError(i18n.KeyServicesMCPSSEOperationFailed, err, "GET")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return &UnauthorizedError{
			Challenge:  ParseWWWAuthenticate(resp),
			ServerURL:  t.streamURL,
			StatusCode: resp.StatusCode,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, remoteResponseBodyLimit))
		return &RemoteHTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			ServerURL:  t.streamURL,
			Body:       string(raw),
			RPCError:   parseRPCError(raw),
		}
	}
	if responseMediaType(resp.Header.Get("Content-Type")) != "text/event-stream" {
		return i18n.NewError(i18n.KeyServicesMCPSSEContentType, resp.Header.Get("Content-Type"))
	}
	return t.readSSEEvents(ctx, resp.Body)
}

func (t *HTTPTransport) readSSEEvents(ctx context.Context, r io.Reader) error {
	reader := bufio.NewReader(r)
	event := sseEvent{}
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if dispatchErr := t.consumeSSELine(ctx, &event, line); dispatchErr != nil {
				return dispatchErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if event.hasData() {
					return t.dispatchSSEEvent(ctx, event)
				}
				return newTransportClosedError(i18n.KeyServicesMCPTransportEOFReason, io.EOF, "SSE stream")
			}
			return err
		}
	}
}

type sseEvent struct {
	event string
	id    string
	retry string
	data  []string
}

func (e *sseEvent) hasData() bool {
	return e != nil && len(e.data) > 0
}

func (e *sseEvent) reset() {
	e.event = ""
	e.id = ""
	e.retry = ""
	e.data = e.data[:0]
}

func (t *HTTPTransport) consumeSSELine(ctx context.Context, event *sseEvent, rawLine string) error {
	line := strings.TrimRight(rawLine, "\r\n")
	if line == "" {
		if event.hasData() {
			if err := t.dispatchSSEEvent(ctx, *event); err != nil {
				return err
			}
		}
		event.reset()
		return nil
	}
	if strings.HasPrefix(line, ":") {
		return nil
	}
	field, value, ok := strings.Cut(line, ":")
	if !ok {
		field = line
		value = ""
	} else if strings.HasPrefix(value, " ") {
		value = strings.TrimPrefix(value, " ")
	}
	switch field {
	case "event":
		event.event = value
	case "id":
		event.id = value
	case "retry":
		event.retry = value
	case "data":
		event.data = append(event.data, value)
	}
	return nil
}

func (t *HTTPTransport) dispatchSSEEvent(ctx context.Context, event sseEvent) error {
	data := strings.Join(event.data, "\n")
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return nil
	}
	if event.event == "endpoint" {
		endpoint, err := resolveSSEEndpoint(t.currentURL(), data)
		if err != nil {
			return err
		}
		return t.setCurrentURL(endpoint)
	}
	var msg JSONRPCMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPSSEOperationFailed, err, "decode JSON-RPC event")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return t.deliver(msg)
}

func resolveSSEEndpoint(base, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", i18n.NewError(i18n.KeyServicesMCPSSEEndpointMissingURL)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", i18n.WrapError(i18n.KeyServicesMCPSSEOperationFailed, err, "parse endpoint")
	}
	if parsed.IsAbs() {
		return normalizeHTTPTransportURL(parsed.String())
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	return normalizeHTTPTransportURL(baseURL.ResolveReference(parsed).String())
}
