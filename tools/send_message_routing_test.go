package tools

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/swarm"
)

func createMailboxTeam(t *testing.T, mgr *TeamManager, teamName string, agents []any) {
	t.Helper()
	res, err := NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{
		"team_name": teamName,
	})
	if err != nil {
		t.Fatalf("TeamCreate returned error: %v", err)
	}
	if res.IsError {
		t.Fatalf("TeamCreate failed: %s", res.Content)
	}
	if len(agents) == 0 {
		return
	}
	storageName := teamStorageName(teamName)
	cfg, err := swarm.LoadTeamConfig(storageName)
	if err != nil {
		t.Fatalf("LoadTeamConfig: %v", err)
	}
	for _, raw := range agents {
		spec, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("unexpected test agent spec %#v", raw)
		}
		id, _ := spec["id"].(string)
		role, _ := spec["role"].(string)
		cfg.Members = append(cfg.Members, swarm.TeamMember{
			AgentID:       id,
			Name:          id,
			AgentType:     role,
			CWD:           os.Getenv("HOME"),
			Subscriptions: []string{},
			IsActive:      true,
		})
	}
	if err := swarm.SaveTeamConfigAs(storageName, cfg); err != nil {
		t.Fatalf("SaveTeamConfigAs: %v", err)
	}
}

func readMailboxMessages(t *testing.T, teamName string, recipient string) []swarm.Message {
	t.Helper()
	mb, err := swarm.NewMailbox(teamStorageName(teamName))
	if err != nil {
		t.Fatalf("NewMailbox: %v", err)
	}
	msgs, err := mb.Read(context.Background(), recipient)
	if err != nil {
		t.Fatalf("Mailbox.Read: %v", err)
	}
	return msgs
}

func TestSendMessage_TeamMailboxDirect(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "alpha", []any{
		map[string]any{"id": "worker-1", "role": "worker"},
	})

	res, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to":      "worker-1",
		"summary": "Share the latest status",
		"message": "hello teammate",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}

	payload := decodeJSONResult(t, res.Content)
	if payload["message"] != "Message sent to worker-1's inbox" {
		t.Fatalf("expected mailbox delivery message, got: %s", res.Content)
	}
	msgs := readMailboxMessages(t, "alpha", "worker-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 mailbox message, got %d", len(msgs))
	}
	if msgs[0].From != teamLeadName {
		t.Fatalf("expected sender %q, got %q", teamLeadName, msgs[0].From)
	}
	if msgs[0].Text != "hello teammate" {
		t.Fatalf("expected text %q, got %q", "hello teammate", msgs[0].Text)
	}
	if msgs[0].Summary != "Share the latest status" {
		t.Fatalf("expected summary to round-trip, got %q", msgs[0].Summary)
	}
}

func TestSendMessage_ShutdownRequestWritesMailboxMessage(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "alpha", []any{
		map[string]any{"id": "worker-1", "role": "worker"},
	})

	res, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to": "worker-1",
		"message": map[string]any{
			"type":   "shutdown_request",
			"reason": "work is complete",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}

	payload := decodeJSONResult(t, res.Content)
	requestID, _ := payload["request_id"].(string)
	if requestID == "" {
		t.Fatalf("expected request_id in response, got: %s", res.Content)
	}

	msgs := readMailboxMessages(t, "alpha", "worker-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 mailbox message, got %d", len(msgs))
	}
	var structured map[string]any
	if err := json.Unmarshal([]byte(msgs[0].Text), &structured); err != nil {
		t.Fatalf("failed to decode structured mailbox payload %q: %v", msgs[0].Text, err)
	}
	if structured["type"] != "shutdown_request" {
		t.Fatalf("expected shutdown_request payload, got %#v", structured)
	}
	if structured["requestId"] != requestID {
		t.Fatalf("expected requestId %q, got %#v", requestID, structured["requestId"])
	}
	if structured["from"] != teamLeadName {
		t.Fatalf("expected sender %q, got %#v", teamLeadName, structured["from"])
	}
	if structured["reason"] != "work is complete" {
		t.Fatalf("expected reason to round-trip, got %#v", structured["reason"])
	}
}

func TestSendMessage_ShutdownResponseMustTargetTeamLead(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "alpha", []any{
		map[string]any{"id": "worker-1", "role": "worker"},
	})

	res, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to": "worker-1",
		"message": map[string]any{
			"type":       "shutdown_response",
			"request_id": "shutdown-worker-1-1",
			"approve":    true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected tool error, got success: %s", res.Content)
	}
	if !strings.Contains(res.Content, `shutdown_response must be sent to "team-lead"`) {
		t.Fatalf("unexpected error message: %s", res.Content)
	}
}

func TestSendMessage_ShutdownApprovalMarksMemberInactive(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "alpha", []any{
		map[string]any{"id": "worker-1", "role": "worker"},
	})
	t.Setenv("CLAUDE_CODE_AGENT_NAME", "worker-1")

	res, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to": "team-lead",
		"message": map[string]any{
			"type":       "shutdown_response",
			"request_id": "shutdown-worker-1-1",
			"approve":    true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}

	cfg, err := swarm.LoadTeamConfig("alpha")
	if err != nil {
		t.Fatalf("LoadTeamConfig: %v", err)
	}
	for _, member := range cfg.Members {
		if member.Name == "worker-1" {
			if member.IsActive {
				t.Fatalf("expected worker-1 to be marked inactive after shutdown approval")
			}
			return
		}
	}
	t.Fatal("expected worker-1 to exist in team config")
}

func TestSendMessage_UDSPlainText(t *testing.T) {
	mgr := newTestManager(t)
	socketPath := filepath.Join(t.TempDir(), "peer.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen(unix): %v", err)
	}
	defer listener.Close()

	received := make(chan string, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		defer conn.Close()
		data, err := io.ReadAll(conn)
		if err != nil {
			acceptErr <- err
			return
		}
		received <- string(data)
	}()

	res, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to":      "uds:" + socketPath,
		"message": "hello via uds",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}

	payload := decodeJSONResult(t, res.Content)
	if payload["success"] != true {
		t.Fatalf("expected success payload, got: %s", res.Content)
	}
	select {
	case err := <-acceptErr:
		t.Fatalf("uds listener error: %v", err)
	case got := <-received:
		if got != "hello via uds" {
			t.Fatalf("expected payload %q, got %q", "hello via uds", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for uds payload")
	}
}

func TestSendMessage_RejectsUnsupportedAddressScheme(t *testing.T) {
	mgr := newTestManager(t)
	res, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to":      "tcp:remote-peer",
		"message": "hello remote",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected tool error for unsupported address scheme, got success: %s", res.Content)
	}
	if !strings.Contains(res.Content, `unsupported SendMessage address scheme "tcp"`) {
		t.Fatalf("unexpected unsupported scheme message: %s", res.Content)
	}
}

func TestSendMessage_RejectsQualifiedTeammateNames(t *testing.T) {
	mgr := newTestManager(t)
	res, err := NewSendMessageTool(mgr).Execute(context.Background(), map[string]any{
		"to":      "team-lead@alpha",
		"message": "hello",
		"summary": "Send a quick note",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected tool error, got success: %s", res.Content)
	}
	if !strings.Contains(res.Content, `to must be a bare teammate name or "*"`) {
		t.Fatalf("unexpected validation message: %s", res.Content)
	}
}

func TestTeamCreate_PersistsTeamConfig(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "alpha", []any{
		map[string]any{"id": "worker-1", "role": "worker"},
		map[string]any{"id": "worker-2", "role": "reviewer"},
	})

	path := filepath.Join(os.Getenv("HOME"), brand.ConfigDirName, "teams", "alpha", "team.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected persisted team file at %q: %v", path, err)
	}
	cfg, err := swarm.LoadTeamConfig("alpha")
	if err != nil {
		t.Fatalf("LoadTeamConfig: %v", err)
	}
	if cfg.LeadAgentID != "team-lead@alpha" {
		t.Fatalf("expected lead agent id %q, got %q", "team-lead@alpha", cfg.LeadAgentID)
	}
	if cfg.LeadSessionID != "sess-test" {
		t.Fatalf("expected lead session id %q, got %q", "sess-test", cfg.LeadSessionID)
	}
	if len(cfg.Members) != 3 {
		t.Fatalf("expected lead plus 2 members, got %d", len(cfg.Members))
	}
	if cfg.Members[0].Name != teamLeadName {
		t.Fatalf("expected first member to be %q, got %q", teamLeadName, cfg.Members[0].Name)
	}
	if cfg.Members[0].CWD != os.Getenv("HOME") {
		t.Fatalf("expected lead member cwd %q, got %q", os.Getenv("HOME"), cfg.Members[0].CWD)
	}
	if cfg.Members[1].CWD != os.Getenv("HOME") || cfg.Members[2].CWD != os.Getenv("HOME") {
		t.Fatalf("expected teammate cwd values to inherit team cwd, got %#v", cfg.Members)
	}
}
