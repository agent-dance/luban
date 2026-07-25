package mcp

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	"github.com/agent-dance/luban/internal/mcp/protocol"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
)

type mcpProtocolRawCaller interface {
	CallRaw(context.Context, string, any, any) error
}

const mcpProtocolTestVersion = "2024-11-05"

type mcpProtocolTestTransport struct {
	raw       mcpProtocolRawCaller
	responses chan protocol.JSONRPCMessage
	closed    chan struct{}
}

func newMCPProtocolTestClient(t testing.TB, raw mcpProtocolRawCaller) *mcptransport.Client {
	t.Helper()
	transport := &mcpProtocolTestTransport{raw: raw, responses: make(chan protocol.JSONRPCMessage, 8), closed: make(chan struct{})}
	client, err := mcptransport.NewClient(context.Background(), transport)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func (t *mcpProtocolTestTransport) Send(ctx context.Context, message protocol.JSONRPCMessage) error {
	if message.Method == "notifications/initialized" {
		return nil
	}
	if message.Method == "initialize" {
		result, _ := json.Marshal(map[string]any{
			"protocolVersion": mcpProtocolTestVersion,
			"capabilities":    catalog.ServerCapabilities{},
			"serverInfo":      catalog.ServerInfo{Name: "test", Version: "1"},
		})
		return t.respond(ctx, protocol.JSONRPCMessage{JSONRPC: protocol.JSONRPCVersion, ID: protocol.CloneRawMessage(message.ID), Result: result})
	}
	var params any = map[string]any{}
	if len(message.Params) > 0 {
		if err := json.Unmarshal(message.Params, &params); err != nil {
			return err
		}
	}
	var result json.RawMessage
	if err := t.raw.CallRaw(ctx, message.Method, params, &result); err != nil {
		return err
	}
	return t.respond(ctx, protocol.JSONRPCMessage{JSONRPC: protocol.JSONRPCVersion, ID: protocol.CloneRawMessage(message.ID), Result: result})
}

func (t *mcpProtocolTestTransport) respond(ctx context.Context, response protocol.JSONRPCMessage) error {
	select {
	case t.responses <- response:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closed:
		return io.EOF
	}
}

func (t *mcpProtocolTestTransport) Receive(ctx context.Context) (protocol.JSONRPCMessage, error) {
	select {
	case response := <-t.responses:
		return response, nil
	case <-ctx.Done():
		return protocol.JSONRPCMessage{}, ctx.Err()
	case <-t.closed:
		return protocol.JSONRPCMessage{}, io.EOF
	}
}

func (t *mcpProtocolTestTransport) Close() error {
	select {
	case <-t.closed:
	default:
		close(t.closed)
	}
	return nil
}
