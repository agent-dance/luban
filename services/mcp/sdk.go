package mcp

import (
	"context"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

// SDKMessageSender bridges MCP JSON-RPC messages from the CLI process to an
// SDK-owned in-process MCP server. Requests should return a response; pure
// notifications may return nil.
type SDKMessageSender interface {
	SendMCPMessage(ctx context.Context, serverName string, message JSONRPCMessage) (*JSONRPCMessage, error)
}

// SDKMessageSenderFunc adapts a function to SDKMessageSender.
type SDKMessageSenderFunc func(ctx context.Context, serverName string, message JSONRPCMessage) (*JSONRPCMessage, error)

// SendMCPMessage implements SDKMessageSender.
func (f SDKMessageSenderFunc) SendMCPMessage(ctx context.Context, serverName string, message JSONRPCMessage) (*JSONRPCMessage, error) {
	if f == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilSDKSender, serverName)
	}
	return f(ctx, serverName, message)
}

var sdkSenderRegistry = struct {
	sync.RWMutex
	nextToken int64
	senders   map[string]registeredSDKMessageSender
}{senders: map[string]registeredSDKMessageSender{}}

type registeredSDKMessageSender struct {
	token  int64
	sender SDKMessageSender
}

// RegisterSDKMessageSender installs an SDK control sender for serverName and
// returns a cleanup function. It is intentionally process-local: external SDK
// hosts must register a bridge before sdk configs can connect.
func RegisterSDKMessageSender(serverName string, sender SDKMessageSender) func() {
	if serverName == "" || sender == nil {
		return func() {}
	}
	sdkSenderRegistry.Lock()
	sdkSenderRegistry.nextToken++
	token := sdkSenderRegistry.nextToken
	sdkSenderRegistry.senders[serverName] = registeredSDKMessageSender{token: token, sender: sender}
	sdkSenderRegistry.Unlock()
	return func() {
		sdkSenderRegistry.Lock()
		if current, ok := sdkSenderRegistry.senders[serverName]; ok && current.token == token {
			delete(sdkSenderRegistry.senders, serverName)
		}
		sdkSenderRegistry.Unlock()
	}
}

func lookupSDKMessageSender(serverName string) (SDKMessageSender, bool) {
	sdkSenderRegistry.RLock()
	defer sdkSenderRegistry.RUnlock()
	registered, ok := sdkSenderRegistry.senders[serverName]
	return registered.sender, ok
}

// NewSDKTransport resolves the registered SDK control bridge for a config.
// When no bridge is available, it fails closed with a typed unsupported error
// instead of hanging on an absent SDK process.
func NewSDKTransport(name string, config MCPServerConfig) (Transport, error) {
	sender, ok := lookupSDKMessageSender(name)
	if !ok && config.Name != "" && config.Name != name {
		sender, ok = lookupSDKMessageSender(config.Name)
	}
	if !ok {
		return nil, &UnsupportedIntegrationError{
			ServerName: name,
			Transport:  TransportSDK,
			Reason:     mcpText(i18n.KeyMCPSDKBridgeMissing),
		}
	}
	return NewSDKControlClientTransport(name, sender), nil
}

// SDKControlClientTransport is the CLI-side SDK MCP bridge. It mirrors the TS
// SdkControlClientTransport: send wraps one MCP message in a host control call,
// and any response is delivered back to the protocol client receive loop.
type SDKControlClientTransport struct {
	serverName string
	sender     SDKMessageSender

	recvCh  chan JSONRPCMessage
	closeCh chan struct{}

	closeOnce sync.Once
	closed    bool
	mu        sync.RWMutex
}

// NewSDKControlClientTransport constructs a client-side SDK control transport.
func NewSDKControlClientTransport(serverName string, sender SDKMessageSender) *SDKControlClientTransport {
	return &SDKControlClientTransport{
		serverName: serverName,
		sender:     sender,
		recvCh:     make(chan JSONRPCMessage, 64),
		closeCh:    make(chan struct{}),
	}
}

// Send forwards one MCP message to the SDK control channel.
func (t *SDKControlClientTransport) Send(ctx context.Context, msg JSONRPCMessage) error {
	if t == nil || t.sender == nil {
		return newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "SDK control")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if t.isClosed() {
		return newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "SDK control")
	}
	response, err := t.sender.SendMCPMessage(ctx, t.serverName, msg)
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPSDKControlSend, err, t.serverName)
	}
	if response == nil {
		return nil
	}
	select {
	case t.recvCh <- *response:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closeCh:
		return newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "SDK control")
	}
}

// Receive returns responses from the SDK host.
func (t *SDKControlClientTransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	if t == nil {
		return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "SDK control")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case msg := <-t.recvCh:
		return msg, nil
	case <-ctx.Done():
		return JSONRPCMessage{}, ctx.Err()
	case <-t.closeCh:
		return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "SDK control")
	}
}

// Close is idempotent.
func (t *SDKControlClientTransport) Close() error {
	if t == nil {
		return nil
	}
	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.closed = true
		t.mu.Unlock()
		close(t.closeCh)
	})
	return nil
}

func (t *SDKControlClientTransport) isClosed() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.closed
}

// SDKControlServerTransport is the SDK-side bridge. Deliver injects a CLI
// control message into the MCP server; Send emits server responses back through
// sendMCPMessage.
type SDKControlServerTransport struct {
	sendMCPMessage func(context.Context, JSONRPCMessage) error

	recvCh  chan JSONRPCMessage
	closeCh chan struct{}

	closeOnce sync.Once
}

// NewSDKControlServerTransport constructs an SDK-side control transport.
func NewSDKControlServerTransport(sendMCPMessage func(context.Context, JSONRPCMessage) error) *SDKControlServerTransport {
	return &SDKControlServerTransport{
		sendMCPMessage: sendMCPMessage,
		recvCh:         make(chan JSONRPCMessage, 64),
		closeCh:        make(chan struct{}),
	}
}

// Deliver injects a CLI-originated MCP message into the SDK-side server.
func (t *SDKControlServerTransport) Deliver(ctx context.Context, msg JSONRPCMessage) error {
	if t == nil {
		return newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "SDK server")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case t.recvCh <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closeCh:
		return newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "SDK server")
	}
}

// Send emits a server response through the SDK host callback.
func (t *SDKControlServerTransport) Send(ctx context.Context, msg JSONRPCMessage) error {
	if t == nil || t.sendMCPMessage == nil {
		return newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "SDK server")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-t.closeCh:
		return newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "SDK server")
	default:
	}
	return t.sendMCPMessage(ctx, msg)
}

// Receive waits for the next CLI-originated message.
func (t *SDKControlServerTransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	if t == nil {
		return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "SDK server")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case msg := <-t.recvCh:
		return msg, nil
	case <-ctx.Done():
		return JSONRPCMessage{}, ctx.Err()
	case <-t.closeCh:
		return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "SDK server")
	}
}

// Close is idempotent.
func (t *SDKControlServerTransport) Close() error {
	if t == nil {
		return nil
	}
	t.closeOnce.Do(func() { close(t.closeCh) })
	return nil
}
