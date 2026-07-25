package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/protocol"
)

// SSEConfig captures the inputs for a Server-Sent Events transport.
type SSEConfig struct {
	BaseURL string
	Headers map[string]string
	Auth    mcpauth.TokenSource
}

// sseTransport implements the MCP SSE transport: a long-lived GET stream for
// server messages plus POST for client messages.
type sseTransport struct {
	*httpTransport

	streamURL string

	streamOnce   sync.Once
	streamCtx    context.Context
	streamCancel context.CancelFunc
}

// NewSSETransport constructs an SSE transport. The SSE GET is opened lazily
// from Receive.
func NewSSETransport(cfg SSEConfig) (*sseTransport, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, i18n.NewError(i18n.KeyMCPSSEBaseURLRequired)
	}
	httpTransport, err := NewHTTPTransport(HTTPTransportConfig(cfg))
	if err != nil {
		return nil, err
	}
	streamURL, err := normalizeHTTPTransportURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	streamCtx, streamCancel := context.WithCancel(context.Background())
	return &sseTransport{
		httpTransport: httpTransport,
		streamURL:     streamURL,
		streamCtx:     streamCtx,
		streamCancel:  streamCancel,
	}, nil
}

// Receive starts the companion SSE stream once and then returns messages from
// either the GET stream or POST responses.
func (t *sseTransport) Receive(ctx context.Context) (protocol.JSONRPCMessage, error) {
	if t == nil {
		return protocol.JSONRPCMessage{}, NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "SSE")
	}
	t.startStream()
	return t.httpTransport.Receive(ctx)
}

// Close is idempotent and cancels the long-lived GET stream.
func (t *sseTransport) Close() error {
	if t == nil {
		return nil
	}
	t.streamCancel()
	return t.httpTransport.Close()
}

func (t *sseTransport) startStream() {
	t.streamOnce.Do(func() {
		go func() {
			err := t.readEventStream(t.streamCtx)
			if err == nil || errors.Is(err, context.Canceled) || t.closed.Load() {
				return
			}
			t.fail(NewTransportClosedError(i18n.KeyServicesMCPTransportStreamFailedReason, err, "SSE"))
		}()
	})
}

func (t *sseTransport) readEventStream(ctx context.Context) error {
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
		return &mcpauth.UnauthorizedError{
			Challenge:  mcpauth.ParseWWWAuthenticate(resp),
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

func (t *httpTransport) readSSEEvents(ctx context.Context, r io.Reader) error {
	return t.readSSEEventsObserved(ctx, r, nil)
}

func (t *httpTransport) readSSEEventsObserved(ctx context.Context, r io.Reader, delivered func()) error {
	reader := bufio.NewReader(r)
	event := sseEvent{}
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if dispatchErr := t.consumeSSELine(ctx, &event, line, delivered); dispatchErr != nil {
				return dispatchErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if event.hasData() {
					return t.dispatchSSEEvent(ctx, event, delivered)
				}
				return NewTransportClosedError(i18n.KeyServicesMCPTransportEOFReason, io.EOF, "SSE stream")
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

func (t *httpTransport) consumeSSELine(ctx context.Context, event *sseEvent, rawLine string, delivered func()) error {
	line := strings.TrimRight(rawLine, "\r\n")
	if line == "" {
		if event.hasData() {
			if err := t.dispatchSSEEvent(ctx, *event, delivered); err != nil {
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

func (t *httpTransport) dispatchSSEEvent(ctx context.Context, event sseEvent, delivered func()) error {
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
	var msg protocol.JSONRPCMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPSSEOperationFailed, err, "decode JSON-RPC event")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if err := t.deliver(msg); err != nil {
		return err
	}
	if delivered != nil {
		delivered()
	}
	return nil
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
