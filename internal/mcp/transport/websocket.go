package transport

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/protocol"
	"golang.org/x/net/http/httpguts"
)

const (
	webSocketGUID               = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	webSocketDefaultHandshake   = 30 * time.Second
	webSocketOpcodeContinuation = 0x0
	webSocketOpcodeText         = 0x1
	webSocketOpcodeBinary       = 0x2
	webSocketOpcodeClose        = 0x8
	webSocketOpcodePing         = 0x9
	webSocketOpcodePong         = 0xA
	webSocketMaxFramePayload    = 16 << 20
)

// WebSocketTransportConfig captures the inputs for an MCP WebSocket transport.
type WebSocketTransportConfig struct {
	URL     string
	Headers map[string]string
	Auth    mcpauth.TokenSource
}

// webSocketTransport implements MCP over RFC 6455 text frames using the "mcp"
// subprotocol. It is intentionally small to avoid adding a dependency.
type webSocketTransport struct {
	conn net.Conn
	br   *bufio.Reader

	readMu  sync.Mutex
	writeMu sync.Mutex

	closeOnce sync.Once
}

// NewWebSocketTransport dials and handshakes a WebSocket MCP transport.
func NewWebSocketTransport(ctx context.Context, cfg WebSocketTransportConfig) (*webSocketTransport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	wsURL, err := normalizeWebSocketURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	handshakeCtx, cancel := context.WithTimeout(ctx, webSocketDefaultHandshake)
	defer cancel()

	parsed, err := url.Parse(wsURL)
	if err != nil {
		return nil, err
	}
	headers, err := resolveRemoteHeaders(handshakeCtx, remoteHeaderConfig{
		ServerURL: wsURL,
		Headers:   cfg.Headers,
		Auth:      cfg.Auth,
	})
	if err != nil {
		return nil, err
	}

	conn, br, err := dialWebSocket(handshakeCtx, parsed, headers)
	if err != nil {
		return nil, err
	}
	return &webSocketTransport{
		conn: conn,
		br:   br,
	}, nil
}

// Send writes one JSON-RPC message as a WebSocket text frame.
func (t *webSocketTransport) Send(ctx context.Context, msg protocol.JSONRPCMessage) error {
	if t == nil || t.conn == nil {
		return NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "WebSocket")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPWebSocketOperationFailed, err, "marshal")
	}
	return t.writeFrameWithContext(ctx, webSocketOpcodeText, data, true)
}

// Receive reads and decodes one JSON-RPC message from the WebSocket.
func (t *webSocketTransport) Receive(ctx context.Context) (protocol.JSONRPCMessage, error) {
	if t == nil || t.conn == nil || t.br == nil {
		return protocol.JSONRPCMessage{}, NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "WebSocket")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		payload, opcode, err := t.readDataFrameWithContext(ctx)
		if err != nil {
			return protocol.JSONRPCMessage{}, err
		}
		switch opcode {
		case webSocketOpcodeText, webSocketOpcodeBinary:
			var msg protocol.JSONRPCMessage
			if err := json.Unmarshal(payload, &msg); err != nil {
				return protocol.JSONRPCMessage{}, i18n.WrapError(i18n.KeyServicesMCPWebSocketOperationFailed, err, "decode")
			}
			return msg, nil
		}
	}
}

// Close is idempotent.
func (t *webSocketTransport) Close() error {
	if t == nil {
		return nil
	}
	return t.closeConnection()
}

func (t *webSocketTransport) closeConnection() error {
	if t == nil {
		return nil
	}
	var err error
	t.closeOnce.Do(func() {
		if t.conn != nil {
			err = t.conn.Close()
		}
	})
	return err
}

func dialWebSocket(ctx context.Context, parsed *url.URL, headers map[string]string) (net.Conn, *bufio.Reader, error) {
	if err := validateWebSocketHandshakeHeaders(headers); err != nil {
		return nil, nil, err
	}
	host := parsed.Host
	address := host
	if _, _, err := net.SplitHostPort(address); err != nil {
		switch parsed.Scheme {
		case "wss":
			address = net.JoinHostPort(host, "443")
		default:
			address = net.JoinHostPort(host, "80")
		}
	}

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, nil, i18n.WrapError(i18n.KeyServicesMCPWebSocketOperationFailed, err, "dial")
	}
	rawConn := conn
	cancelCloseDone := make(chan struct{})
	stopCancelClose := context.AfterFunc(ctx, func() {
		_ = rawConn.Close()
		close(cancelCloseDone)
	})
	cancelCloseActive := true
	stopAndWaitForCancelClose := func() {
		if stopCancelClose() {
			return
		}
		<-cancelCloseDone
	}
	defer func() {
		if cancelCloseActive {
			stopAndWaitForCancelClose()
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		defer conn.SetDeadline(time.Time{})
	}
	if parsed.Scheme == "wss" {
		tlsConfig := &tls.Config{ServerName: parsed.Hostname()}
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, nil, i18n.WrapError(i18n.KeyServicesMCPWebSocketOperationFailed, err, "TLS handshake")
		}
		conn = tlsConn
	}

	key, err := newWebSocketKey()
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	path := parsed.RequestURI()
	if path == "" {
		path = "/"
	}
	var req bytes.Buffer
	fmt.Fprintf(&req, "GET %s HTTP/1.1\r\n", path)
	fmt.Fprintf(&req, "Host: %s\r\n", parsed.Host)
	req.WriteString("Upgrade: websocket\r\n")
	req.WriteString("Connection: Upgrade\r\n")
	fmt.Fprintf(&req, "Sec-WebSocket-Key: %s\r\n", key)
	req.WriteString("Sec-WebSocket-Version: 13\r\n")
	req.WriteString("Sec-WebSocket-Protocol: mcp\r\n")
	for name, value := range headers {
		if strings.EqualFold(name, "Host") ||
			strings.EqualFold(name, "Upgrade") ||
			strings.EqualFold(name, "Connection") ||
			strings.EqualFold(name, "Sec-WebSocket-Key") ||
			strings.EqualFold(name, "Sec-WebSocket-Version") ||
			strings.EqualFold(name, "Sec-WebSocket-Protocol") {
			continue
		}
		fmt.Fprintf(&req, "%s: %s\r\n", name, value)
	}
	req.WriteString("\r\n")
	if _, err := conn.Write(req.Bytes()); err != nil {
		_ = conn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		return nil, nil, i18n.WrapError(i18n.KeyServicesMCPWebSocketOperationFailed, err, "handshake write")
	}

	br := bufio.NewReader(conn)
	httpReq := &http.Request{Method: http.MethodGet, URL: parsed}
	resp, err := http.ReadResponse(br, httpReq)
	if err != nil {
		_ = conn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		return nil, nil, i18n.WrapError(i18n.KeyServicesMCPWebSocketOperationFailed, err, "handshake read")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		_ = conn.Close()
		return nil, nil, &mcpauth.UnauthorizedError{
			Challenge:  mcpauth.ParseWWWAuthenticate(resp),
			ServerURL:  parsed.String(),
			StatusCode: resp.StatusCode,
		}
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, remoteResponseBodyLimit))
		_ = conn.Close()
		return nil, nil, &RemoteHTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			ServerURL:  parsed.String(),
			Body:       string(raw),
			RPCError:   parseRPCError(raw),
		}
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		_ = conn.Close()
		return nil, nil, i18n.NewError(i18n.KeyServicesMCPWebSocketMissingUpgrade)
	}
	if !headerHasToken(resp.Header.Get("Connection"), "upgrade") {
		_ = conn.Close()
		return nil, nil, i18n.NewError(i18n.KeyServicesMCPWebSocketMissingConnection)
	}
	if got, want := resp.Header.Get("Sec-WebSocket-Accept"), webSocketAccept(key); got != want {
		_ = conn.Close()
		return nil, nil, i18n.NewError(i18n.KeyServicesMCPWebSocketAcceptMismatch)
	}
	if protocol := strings.TrimSpace(resp.Header.Get("Sec-WebSocket-Protocol")); protocol != "" && !strings.EqualFold(protocol, "mcp") {
		_ = conn.Close()
		return nil, nil, i18n.NewError(i18n.KeyServicesMCPWebSocketSubprotocol, protocol)
	}
	stopAndWaitForCancelClose()
	cancelCloseActive = false
	if ctxErr := ctx.Err(); ctxErr != nil {
		_ = conn.Close()
		return nil, nil, ctxErr
	}
	return conn, br, nil
}

func validateWebSocketHandshakeHeaders(headers map[string]string) error {
	for name, value := range headers {
		if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
			return i18n.NewError(i18n.KeyServicesMCPWebSocketHeaderInvalid)
		}
	}
	return nil
}

func (t *webSocketTransport) readDataFrameWithContext(ctx context.Context) ([]byte, byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	stopCancelClose := context.AfterFunc(ctx, func() { _ = t.closeConnection() })
	defer stopCancelClose()

	t.readMu.Lock()
	defer t.readMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	payload, opcode, err := t.readDataFrame()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, 0, ctxErr
	}
	return payload, opcode, err
}

func (t *webSocketTransport) readDataFrame() ([]byte, byte, error) {
	var message bytes.Buffer
	var messageOpcode byte
	for {
		payload, opcode, fin, err := readWebSocketFrame(t.br)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil, 0, NewTransportClosedError(i18n.KeyServicesMCPTransportEOFReason, err, "WebSocket")
			}
			return nil, 0, err
		}
		switch opcode {
		case webSocketOpcodeText, webSocketOpcodeBinary:
			message.Reset()
			messageOpcode = opcode
			message.Write(payload)
			if fin {
				return message.Bytes(), messageOpcode, nil
			}
		case webSocketOpcodeContinuation:
			if messageOpcode == 0 {
				return nil, 0, i18n.NewError(i18n.KeyServicesMCPWebSocketContinuation)
			}
			message.Write(payload)
			if fin {
				return message.Bytes(), messageOpcode, nil
			}
		case webSocketOpcodePing:
			if err := t.writeFrameWithContext(context.Background(), webSocketOpcodePong, payload, true); err != nil {
				return nil, 0, err
			}
		case webSocketOpcodePong:
			continue
		case webSocketOpcodeClose:
			return nil, 0, NewTransportClosedError(i18n.KeyServicesMCPTransportCloseFrameReason, ErrTransportClosed)
		default:
			return nil, 0, i18n.NewError(i18n.KeyServicesMCPWebSocketOpcode, opcode)
		}
		if message.Len() > webSocketMaxFramePayload {
			return nil, 0, i18n.NewError(i18n.KeyServicesMCPWebSocketMessageTooLarge)
		}
	}
}

func (t *webSocketTransport) writeFrameWithContext(ctx context.Context, opcode byte, payload []byte, mask bool) error {
	if t == nil || t.conn == nil {
		return NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "WebSocket")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stopCancelClose := context.AfterFunc(ctx, func() { _ = t.closeConnection() })
	defer stopCancelClose()

	frame := buildWebSocketFrame(opcode, payload, mask)
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := t.conn.Write(frame)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			return NewTransportClosedError(i18n.KeyServicesMCPTransportWriteClosedReason, err)
		}
		return i18n.WrapError(i18n.KeyServicesMCPWebSocketOperationFailed, err, "write")
	}
	return nil
}

func normalizeWebSocketURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", i18n.NewError(i18n.KeyMCPWebSocketURLRequired)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", i18n.NewError(i18n.KeyMCPWebSocketURLInvalid, raw)
	}
	switch parsed.Scheme {
	case "ws", "wss":
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", i18n.NewError(i18n.KeyMCPWebSocketSchemeInvalid, parsed.Scheme)
	}
	return parsed.String(), nil
}

func newWebSocketKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", i18n.WrapError(i18n.KeyServicesMCPWebSocketOperationFailed, err, "key random")
	}
	return base64.StdEncoding.EncodeToString(b[:]), nil
}

func webSocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + webSocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerHasToken(header, want string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), want) {
			return true
		}
	}
	return false
}

func readWebSocketFrame(r *bufio.Reader) ([]byte, byte, bool, error) {
	first, err := r.ReadByte()
	if err != nil {
		return nil, 0, false, err
	}
	second, err := r.ReadByte()
	if err != nil {
		return nil, 0, false, err
	}
	fin := first&0x80 != 0
	opcode := first & 0x0f
	masked := second&0x80 != 0
	length := uint64(second & 0x7f)
	switch length {
	case 126:
		var buf [2]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, 0, false, err
		}
		length = uint64(binary.BigEndian.Uint16(buf[:]))
	case 127:
		var buf [8]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, 0, false, err
		}
		length = binary.BigEndian.Uint64(buf[:])
	}
	if length > webSocketMaxFramePayload {
		return nil, 0, false, i18n.NewError(i18n.KeyServicesMCPWebSocketFrameTooLarge)
	}
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(r, maskKey[:]); err != nil {
			return nil, 0, false, err
		}
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, 0, false, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return payload, opcode, fin, nil
}

func buildWebSocketFrame(opcode byte, payload []byte, mask bool) []byte {
	var out bytes.Buffer
	out.WriteByte(0x80 | (opcode & 0x0f))
	length := len(payload)
	maskBit := byte(0)
	if mask {
		maskBit = 0x80
	}
	switch {
	case length < 126:
		out.WriteByte(maskBit | byte(length))
	case length <= math.MaxUint16:
		out.WriteByte(maskBit | 126)
		var buf [2]byte
		binary.BigEndian.PutUint16(buf[:], uint16(length))
		out.Write(buf[:])
	default:
		out.WriteByte(maskBit | 127)
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(length))
		out.Write(buf[:])
	}
	var maskKey [4]byte
	if mask {
		if _, err := rand.Read(maskKey[:]); err != nil {
			copy(maskKey[:], []byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
		}
		out.Write(maskKey[:])
	}
	if !mask {
		out.Write(payload)
		return out.Bytes()
	}
	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ maskKey[i%4]
	}
	out.Write(masked)
	return out.Bytes()
}
