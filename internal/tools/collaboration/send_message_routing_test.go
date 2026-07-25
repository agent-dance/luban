package collaboration

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

type retainedMessengerStub struct {
	resume  RetainedAgentResume
	handled bool
	err     error
	target  string
	prompt  string
	calls   int
}

func (stub *retainedMessengerStub) ResumeAgent(_ context.Context, target, prompt string) (RetainedAgentResume, bool, error) {
	stub.calls++
	stub.target = target
	stub.prompt = prompt
	return stub.resume, stub.handled, stub.err
}

type retainedStopperStub struct {
	target string
	calls  int
	found  bool
}

func (stub *retainedStopperStub) AbortAgent(target string) bool {
	stub.calls++
	stub.target = target
	return stub.found
}

func TestSendMessageOnlyExplicitUDSPrefixSelectsLocalSocket(t *testing.T) {
	if address := parseSendMessageAddress("uds:/tmp/peer.sock"); !address.isUDS || address.target != "/tmp/peer.sock" {
		t.Fatalf("explicit UDS address = %#v", address)
	}
	if address := parseSendMessageAddress("/tmp/peer.sock"); address.isUDS {
		t.Fatalf("bare absolute path selected UDS: %#v", address)
	}
	if scheme := unsupportedSendMessageScheme("tcp:peer"); scheme != "tcp" {
		t.Fatalf("unsupported scheme = %q", scheme)
	}
	if address := parseSendMessageAddress("uds:/tmp/peer@example.sock"); !address.isUDS || address.target != "/tmp/peer@example.sock" {
		t.Fatalf("UDS path containing @ = %#v", address)
	}
}

func TestSendMessageRetainedAgentPortNeedsNoTeamFallback(t *testing.T) {
	retained := &retainedMessengerStub{
		resume: RetainedAgentResume{Status: "running"}, handled: true,
	}
	result, err := NewSendMessageTool(nil, retained, nil).Execute(context.Background(), map[string]any{
		"to": "worker-1", "summary": "share current status", "message": "continue",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute() = %#v, error = %v", result, err)
	}
	if retained.calls != 1 || retained.target != "worker-1" || retained.prompt != "continue" {
		t.Fatalf("retained delivery = %#v", retained)
	}
	if data, ok := result.Data.(sendMessageResult); !ok || !data.Success {
		t.Fatalf("typed result = %#v (%T)", result.Data, result.Data)
	}
}

func TestSendMessageHasNoDefaultMailbox(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	result, err := NewSendMessageTool(nil, nil, nil).Execute(context.Background(), map[string]any{
		"to": "worker-1", "summary": "share current status", "message": "hello",
	})
	if err != nil || !result.IsError {
		t.Fatalf("Execute() = %#v, error = %v", result, err)
	}
	configPath, err := swarm.TeamConfigPath("default")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(configPath)); !os.IsNotExist(err) {
		t.Fatalf("default team directory was created: %v", err)
	}
}

func TestSendMessageUDSPlainText(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain socket test")
	}
	socketDir, err := os.MkdirTemp("", "luban-sm-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "peer@example.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan string, 1)
	acceptErr := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		defer connection.Close()
		content, err := io.ReadAll(connection)
		if err != nil {
			acceptErr <- err
			return
		}
		received <- string(content)
	}()

	result, err := NewSendMessageTool(nil, nil, nil).Execute(context.Background(), map[string]any{
		"to": "uds:" + socketPath, "message": "hello",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute() = %#v, error = %v", result, err)
	}
	select {
	case err := <-acceptErr:
		t.Fatal(err)
	case content := <-received:
		if content != "hello" {
			t.Fatalf("received %q", content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for UDS message")
	}
}

func TestSendMessageMailboxUsesStrictDurableTeam(t *testing.T) {
	manager := seedSendMessageTeam(t, "team-lead@alpha", "")
	result, err := NewSendMessageTool(manager, nil, nil).Execute(context.Background(), map[string]any{
		"to": "worker-1", "summary": "share current status", "message": "hello",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute() = %#v, error = %v", result, err)
	}
	messages := readSendMessageMailbox(t, "alpha", "worker-1")
	if len(messages) != 1 || messages[0].From != teamLeadName || messages[0].Text != "hello" {
		t.Fatalf("mailbox messages = %#v", messages)
	}
	data, ok := result.Data.(sendMessageResult)
	if !ok || data.Routing == nil || data.Routing.Target != "@worker-1" {
		t.Fatalf("typed routing = %#v (%T)", result.Data, result.Data)
	}
}

func TestSendMessageForegroundSessionUsesDurableLeadIdentity(t *testing.T) {
	manager := seedSendMessageTeam(t, "", "")
	result, err := NewSendMessageTool(manager, nil, nil).Execute(context.Background(), map[string]any{
		"to": "worker-1", "summary": "share current status", "message": "hello",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute() = %#v, error = %v", result, err)
	}
	messages := readSendMessageMailbox(t, "alpha", "worker-1")
	if len(messages) != 1 || messages[0].From != teamLeadName {
		t.Fatalf("foreground sender = %#v", messages)
	}
}

func TestSendMessageRejectsDurableOwnerMismatch(t *testing.T) {
	manager := seedSendMessageTeam(t, "team-lead@alpha", "")
	config, err := swarm.LoadTeamConfig("alpha")
	if err != nil {
		t.Fatal(err)
	}
	config.LeadSessionID = "different-session"
	if _, err := swarm.UpdateTeamConfig(context.Background(), "alpha", func(current *swarm.TeamConfig) error {
		current.LeadSessionID = config.LeadSessionID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	result, err := NewSendMessageTool(manager, nil, nil).Execute(context.Background(), map[string]any{
		"to": "worker-1", "summary": "share current status", "message": "hello",
	})
	if err != nil || !result.IsError || result.Outcome != types.ToolOutcomeFailed {
		t.Fatalf("Execute() = %#v, error = %v", result, err)
	}
	if _, err := os.Stat(sendMessageInboxPath(t, "alpha", "worker-1")); !os.IsNotExist(err) {
		t.Fatalf("owner mismatch wrote a mailbox: %v", err)
	}
}

func TestSendMessageApprovedShutdownDoesNotInferBackend(t *testing.T) {
	manager := seedSendMessageTeam(t, "worker-1@alpha", "")
	stopper := &retainedStopperStub{found: true}
	result, err := NewSendMessageTool(manager, nil, stopper).Execute(context.Background(), map[string]any{
		"to": teamLeadName,
		"message": map[string]any{
			"type": "shutdown_response", "request_id": "shutdown-1", "approve": true,
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute() = %#v, error = %v", result, err)
	}
	if stopper.calls != 0 {
		t.Fatalf("empty persisted backend inferred as in-process: %#v", stopper)
	}
	assertSendMessageMemberInactive(t, "alpha", "worker-1")
	messages := readSendMessageMailbox(t, "alpha", teamLeadName)
	var payload map[string]any
	if err := json.Unmarshal([]byte(messages[0].Text), &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["backendType"]; exists {
		t.Fatalf("backendType was inferred: %#v", payload)
	}
}

func TestSendMessageApprovedShutdownStopsPersistedInProcessAgent(t *testing.T) {
	manager := seedSendMessageTeam(t, "worker-1@alpha", "in-process")
	stopper := &retainedStopperStub{found: true}
	result, err := NewSendMessageTool(manager, nil, stopper).Execute(context.Background(), map[string]any{
		"to": teamLeadName,
		"message": map[string]any{
			"type": "shutdown_response", "request_id": "shutdown-2", "approve": true,
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute() = %#v, error = %v", result, err)
	}
	if stopper.calls != 1 || stopper.target != "worker-1" {
		t.Fatalf("stopper calls = %#v", stopper)
	}
	assertSendMessageMemberInactive(t, "alpha", "worker-1")
}

func seedSendMessageTeam(t *testing.T, runtimeAgentID, workerBackend string) *TeamManager {
	t.Helper()
	home := t.TempDir()
	projectRoot := t.TempDir()
	t.Setenv("HOME", home)
	identity := RuntimeIdentity{
		SessionID: "session-alpha", ProjectRoot: projectRoot, AgentID: runtimeAgentID,
	}
	manager := NewTeamManager(nil)
	manager.PublishRuntimeIdentity(identity)
	config := &swarm.TeamConfig{
		Name: "alpha", LeadAgentID: "team-lead@alpha", LeadSessionID: identity.SessionID,
		Members: []swarm.TeamMember{
			{
				AgentID: "team-lead@alpha", Name: teamLeadName, CWD: projectRoot,
				Subscriptions: []string{}, IsActive: true,
			},
			{
				AgentID: "worker-1@alpha", Name: "worker-1", BackendType: workerBackend,
				CWD: projectRoot, Subscriptions: []string{}, IsActive: true,
			},
		},
	}
	if err := swarm.CreateTeamConfigAs("alpha", config); err != nil {
		t.Fatal(err)
	}
	owner := teamOwnerKey{SessionID: identity.SessionID, ProjectRoot: canonicalTeamOwnerRoot(projectRoot)}
	manager.mu.Lock()
	manager.teams["team-1"] = &teamInfo{
		ID: "team-1", Name: "alpha", StorageName: "alpha",
		OwnerSessionID: owner.SessionID, OwnerProjectRoot: owner.ProjectRoot,
	}
	manager.activeByOwner[owner] = "team-1"
	manager.mu.Unlock()
	return manager
}

func sendMessageInboxPath(t *testing.T, storageName, recipient string) string {
	t.Helper()
	configPath, err := swarm.TeamConfigPath(storageName)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(configPath), "inboxes", recipient+".json")
}

func readSendMessageMailbox(t *testing.T, storageName, recipient string) []swarm.Message {
	t.Helper()
	content, err := os.ReadFile(sendMessageInboxPath(t, storageName, recipient))
	if err != nil {
		t.Fatal(err)
	}
	var messages []swarm.Message
	if err := json.Unmarshal(content, &messages); err != nil {
		t.Fatal(err)
	}
	return messages
}

func assertSendMessageMemberInactive(t *testing.T, storageName, identity string) {
	t.Helper()
	config, err := swarm.LoadTeamConfig(storageName)
	if err != nil {
		t.Fatal(err)
	}
	member, ok := teamMemberByIdentity(config, identity)
	if !ok || member.IsActive || !strings.EqualFold(member.Lifecycle, "inactive") {
		t.Fatalf("member was not durably deactivated: %#v", member)
	}
}
