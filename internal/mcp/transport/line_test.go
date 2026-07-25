package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/mcp/protocol"
)

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func TestLineTransportSendsAndReceivesJSONRPCMessages(t *testing.T) {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()
	transport := newLineTransport(serverToClientR, clientToServerW, closerFunc(func() error {
		_ = clientToServerW.Close()
		_ = serverToClientR.Close()
		return nil
	}))
	defer transport.Close() //nolint:errcheck

	req, err := protocol.NewRequestMessage(12, "tools/list", map[string]any{"cursor": "abc"})
	if err != nil {
		t.Fatalf("NewRequestMessage: %v", err)
	}
	sendErr := make(chan error, 1)
	go func() {
		sendErr <- transport.Send(context.Background(), req)
	}()

	var serverGot protocol.JSONRPCMessage
	if err := json.NewDecoder(clientToServerR).Decode(&serverGot); err != nil {
		t.Fatalf("server decode: %v", err)
	}
	if serverGot.JSONRPC != protocol.JSONRPCVersion || serverGot.Method != "tools/list" || string(serverGot.ID) != "12" {
		t.Fatalf("sent request mismatch: %#v", serverGot)
	}
	select {
	case err := <-sendErr:
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send did not complete")
	}

	resp, err := protocol.NewResultMessage(serverGot.ID, map[string]any{
		"tools": []map[string]any{{"name": "read"}},
	})
	if err != nil {
		t.Fatalf("NewResultMessage: %v", err)
	}
	encodeErr := make(chan error, 1)
	go func() {
		encodeErr <- json.NewEncoder(serverToClientW).Encode(resp)
	}()
	clientGot, err := transport.Receive(context.Background())
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	select {
	case err := <-encodeErr:
		if err != nil {
			t.Fatalf("server encode: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server encode did not complete")
	}
	if string(clientGot.ID) != "12" || len(clientGot.Result) == 0 {
		t.Fatalf("received response mismatch: %#v", clientGot)
	}
}

func TestLineTransportCloseUsesTypedClosedError(t *testing.T) {
	reader, writer := io.Pipe()
	transport := newLineTransport(reader, writer, closerFunc(func() error {
		_ = writer.Close()
		_ = reader.Close()
		return nil
	}))
	if err := transport.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := transport.Receive(context.Background())
	if !errors.Is(err, ErrTransportClosed) {
		t.Fatalf("Receive error = %T %[1]v, want ErrTransportClosed", err)
	}
}
