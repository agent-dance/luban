package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/ui"
)

func TestScreenReaderMCPCommandUsesLiveBackendAndLinearOutput(t *testing.T) {
	manager := svcmcp.NewManager()
	manager.AddConfig("live-screen-reader", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "true"})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, strings.NewReader(""))
	sessionID, cwd := "session", t.TempDir()
	cfg := TUIREPLConfig{Engine: screenReaderLifecycleEngine{}, SessionID: &sessionID, CWD: &cwd, MCPBackend: manager}

	handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), "/mcp")
	if err != nil || !handled || exit {
		t.Fatalf("handle /mcp = handled %t exit %t err %v", handled, exit, err)
	}
	text := output.String()
	if !strings.Contains(text, "live-screen-reader  state=pending") {
		t.Fatalf("screen-reader /mcp did not use live backend:\n%s", text)
	}
	if strings.Count(text, "live-screen-reader  state=pending") != 1 {
		t.Fatalf("screen-reader /mcp duplicated the server row:\n%s", text)
	}
	if strings.ContainsAny(text, "\x1b\r") {
		t.Fatalf("screen-reader /mcp emitted terminal control: %q", text)
	}
}

func TestScreenReaderDoctorCommandUsesLiveBackend(t *testing.T) {
	manager := svcmcp.NewManager()
	manager.AddConfig("live-doctor", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "true"})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, strings.NewReader(""))
	sessionID, cwd := "session", t.TempDir()
	cfg := TUIREPLConfig{Engine: screenReaderLifecycleEngine{}, SessionID: &sessionID, CWD: &cwd, MCPBackend: manager}

	handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), "/doctor")
	if err != nil || !handled || exit {
		t.Fatalf("handle /doctor = handled %t exit %t err %v", handled, exit, err)
	}
	if text := output.String(); !strings.Contains(text, "MCP: 1 server(s) configured: 0 connected, 1 pending") {
		t.Fatalf("screen-reader /doctor did not use live backend:\n%s", text)
	} else if strings.Count(text, "MCP: 1 server(s) configured: 0 connected, 1 pending") != 1 {
		t.Fatalf("screen-reader /doctor duplicated the diagnostic row:\n%s", text)
	}
}
