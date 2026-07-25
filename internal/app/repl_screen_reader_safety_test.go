package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
)

type lockedScreenReaderOutput struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	updated chan struct{}
}

func (o *lockedScreenReaderOutput) Write(value []byte) (int, error) {
	o.mu.Lock()
	n, err := o.buffer.Write(value)
	o.mu.Unlock()
	select {
	case o.updated <- struct{}{}:
	default:
	}
	return n, err
}

func (o *lockedScreenReaderOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}

func TestScreenReaderDecisionRecorderRejectsWrongSessionWithoutWriting(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	for _, sessionID := range []string{"active", "other"} {
		if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage(sessionID)}); err != nil {
			t.Fatal(err)
		}
	}
	activeSessionID := "active"
	cfg := TUIREPLConfig{
		Repo: repo, SessionID: &activeSessionID, SessionProjectDir: &projectDir,
		SessionTransitionMu: &sync.Mutex{},
	}
	recorder := screenReaderDecisionRecorder(cfg)
	record := ui.ScreenReaderDecisionRecord{
		Prompt: permissions.PromptRequest{DecisionID: "wrong-session", SessionID: "other", ExecutionSessionID: "agent-session"},
		Response: permissions.PromptResponse{
			DecisionID: "wrong-session", Decision: permissions.DecisionAllowOnce,
			Outcome: permissions.PromptOutcomeApproved, Choice: "allow_once",
		},
		ResolvedAt: time.Now(),
	}
	if err := recorder(record); err == nil || !strings.Contains(err.Error(), "belongs to session") {
		t.Fatalf("wrong-session recorder error = %v", err)
	}
	for _, sessionID := range []string{"active", "other"} {
		meta, _, err := repo.GetMeta(sessionID, projectDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(meta.Decisions) != 0 {
			t.Fatalf("wrong-session decision was written to %s: %+v", sessionID, meta.Decisions)
		}
	}

	record.Prompt.SessionID = ""
	if err := recorder(record); err == nil || !strings.Contains(err.Error(), "no session identity") {
		t.Fatalf("missing-session recorder error = %v", err)
	}
	record.Prompt.SessionID = "active"
	if err := recorder(record); err != nil {
		t.Fatalf("matching-session recorder: %v", err)
	}
	meta, _, err := repo.GetMeta("active", projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Decisions) != 1 || meta.Decisions[0].DecisionID != "wrong-session" || meta.Decisions[0].ExecutionSessionID != "agent-session" {
		t.Fatalf("matching decision persistence = %+v", meta.Decisions)
	}
}

func TestScreenReaderWrongSessionApprovalFailsClosed(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	for _, sessionID := range []string{"active", "stale"} {
		if err := repo.Save(sessionID, projectDir, nil); err != nil {
			t.Fatal(err)
		}
	}
	activeSessionID := "active"
	cfg := TUIREPLConfig{
		Repo: repo, SessionID: &activeSessionID, SessionProjectDir: &projectDir,
		SessionTransitionMu: &sync.Mutex{},
	}
	reader, writer := io.Pipe()
	defer writer.Close()
	output := &lockedScreenReaderOutput{updated: make(chan struct{}, 1)}
	renderer := ui.NewScreenReaderRenderer(output, reader)
	renderer.SetDecisionRecorder(screenReaderDecisionRecorder(cfg))
	responses := make(chan permissions.PromptResponse, 1)
	go func() {
		responses <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "stale-approval", SessionID: "stale", Choices: []string{"allow_once", "reject"},
		})
	}()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for !strings.Contains(output.String(), "Decision choice: ") {
		select {
		case <-output.updated:
		case <-timer.C:
			t.Fatalf("decision prompt was not rendered:\n%s", output.String())
		}
	}
	if _, err := io.WriteString(writer, "decision stale-approval 1\n"); err != nil {
		t.Fatal(err)
	}
	response := <-responses
	if response.Outcome != permissions.PromptOutcomeRejected || response.Decision != permissions.DecisionDeny {
		t.Fatalf("wrong-session approval escaped fail-closed boundary: %#v", response)
	}
	if !strings.Contains(output.String(), "approval blocked") {
		t.Fatalf("missing fail-closed receipt:\n%s", output.String())
	}
}
