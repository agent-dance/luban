package mcp

import (
	"context"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

// UnsupportedIntegrationError marks specialized integrations whose external
// host/runtime is not available in the current Go process.
type UnsupportedIntegrationError struct {
	ServerName string
	Transport  TransportType
	Reason     string
}

// Error implements error.
func (e *UnsupportedIntegrationError) Error() string {
	if e == nil {
		return mcpText(i18n.KeyMCPIntegrationUnsupported)
	}
	reason := e.Reason
	if reason == "" {
		reason = mcpText(i18n.KeyMCPIntegrationUnavailable)
	}
	if e.ServerName == "" {
		return mcpFormat(i18n.KeyMCPIntegrationUnsupportedTransport, e.Transport, reason)
	}
	return mcpFormat(i18n.KeyMCPIntegrationUnsupportedServer, e.ServerName, e.Transport, reason)
}

// InProcessMCPServer is the minimal contract for bundled MCP servers that can
// run inside the Go process.
type InProcessMCPServer interface {
	Connect(ctx context.Context, transport Transport) error
	Close() error
}

// InProcessServerFactory constructs a bundled in-process MCP server.
type InProcessServerFactory func(ctx context.Context, name string, config MCPServerConfig) (InProcessMCPServer, error)

var inProcessFactories = struct {
	sync.RWMutex
	nextToken int64
	factories map[string]registeredInProcessFactory
}{factories: map[string]registeredInProcessFactory{}}

type registeredInProcessFactory struct {
	token   int64
	factory InProcessServerFactory
}

// RegisterInProcessServerFactory installs an in-process server factory for a
// normalized server name and returns a cleanup function.
func RegisterInProcessServerFactory(name string, factory InProcessServerFactory) func() {
	normalized := NormalizeNameForMCP(name)
	if normalized == "" || factory == nil {
		return func() {}
	}
	inProcessFactories.Lock()
	inProcessFactories.nextToken++
	token := inProcessFactories.nextToken
	inProcessFactories.factories[normalized] = registeredInProcessFactory{token: token, factory: factory}
	inProcessFactories.Unlock()
	return func() {
		inProcessFactories.Lock()
		if current, ok := inProcessFactories.factories[normalized]; ok && current.token == token {
			delete(inProcessFactories.factories, normalized)
		}
		inProcessFactories.Unlock()
	}
}

func lookupInProcessFactory(name string) (InProcessServerFactory, bool) {
	inProcessFactories.RLock()
	defer inProcessFactories.RUnlock()
	registered, ok := inProcessFactories.factories[NormalizeNameForMCP(name)]
	return registered.factory, ok
}

// MaybeNewInProcessTransport starts a registered in-process server for known
// bundled MCP server names. It returns handled=false for ordinary stdio
// servers so the default factory can continue to subprocess transport.
func MaybeNewInProcessTransport(ctx context.Context, name string, config MCPServerConfig) (transport Transport, handled bool, err error) {
	if !IsBundledInProcessServer(name) {
		return nil, false, nil
	}
	factory, ok := lookupInProcessFactory(name)
	if !ok {
		return nil, true, &UnsupportedIntegrationError{
			ServerName: name,
			Transport:  config.Type,
			Reason:     mcpText(i18n.KeyMCPIntegrationUnavailableInBuild),
		}
	}
	server, err := factory(ctx, name, config)
	if err != nil {
		return nil, true, err
	}
	if server == nil {
		return nil, true, &UnsupportedIntegrationError{
			ServerName: name,
			Transport:  config.Type,
			Reason:     mcpText(i18n.KeyMCPIntegrationFactoryNil),
		}
	}
	clientTransport, serverTransport := CreateLinkedTransportPair()
	if err := server.Connect(ctx, serverTransport); err != nil {
		_ = clientTransport.Close()
		_ = serverTransport.Close()
		_ = server.Close()
		return nil, true, err
	}
	return &managedInProcessTransport{Transport: clientTransport, server: server}, true, nil
}

// IsBundledInProcessServer reports the TS bundled MCP server names that Go can
// either run through a registered factory or fail closed with a clear message.
func IsBundledInProcessServer(name string) bool {
	normalized := NormalizeNameForMCP(name)
	return normalized == "claude-in-chrome" || normalized == "computer-use"
}

// CreateLinkedTransportPair creates a client/server in-process transport pair.
func CreateLinkedTransportPair() (Transport, Transport) {
	a := newInProcessTransport()
	b := newInProcessTransport()
	a.peer = b
	b.peer = a
	return a, b
}

type inProcessTransport struct {
	peer *inProcessTransport

	in      chan JSONRPCMessage
	closeCh chan struct{}

	closeOnce sync.Once
	mu        sync.RWMutex
	closed    bool
}

func newInProcessTransport() *inProcessTransport {
	return &inProcessTransport{
		in:      make(chan JSONRPCMessage, 64),
		closeCh: make(chan struct{}),
	}
}

func (t *inProcessTransport) Send(ctx context.Context, msg JSONRPCMessage) error {
	if t == nil || t.peer == nil {
		return newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "in-process")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if t.isClosed() || t.peer.isClosed() {
		return newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "in-process")
	}
	select {
	case t.peer.in <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closeCh:
		return newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "in-process")
	case <-t.peer.closeCh:
		return newTransportClosedError(i18n.KeyServicesMCPTransportPeerClosedReason, ErrTransportClosed, "in-process")
	}
}

func (t *inProcessTransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	if t == nil {
		return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "in-process")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case msg := <-t.in:
		return msg, nil
	case <-ctx.Done():
		return JSONRPCMessage{}, ctx.Err()
	case <-t.closeCh:
		return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, ErrTransportClosed, "in-process")
	}
}

func (t *inProcessTransport) Close() error {
	if t == nil {
		return nil
	}
	t.closeSelf()
	if t.peer != nil {
		t.peer.closeSelf()
	}
	return nil
}

func (t *inProcessTransport) closeSelf() {
	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.closed = true
		t.mu.Unlock()
		close(t.closeCh)
	})
}

func (t *inProcessTransport) isClosed() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.closed
}

type managedInProcessTransport struct {
	Transport
	server InProcessMCPServer
	once   sync.Once
}

func (t *managedInProcessTransport) Close() error {
	if t == nil {
		return nil
	}
	var err error
	t.once.Do(func() {
		err = t.Transport.Close()
		if t.server != nil {
			if serverErr := t.server.Close(); err == nil {
				err = serverErr
			}
		}
	})
	return err
}
