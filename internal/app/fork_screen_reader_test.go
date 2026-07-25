package app

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type forkScreenReaderEngine struct {
	engine.Engine
	sessions engine.SessionManager
}

func (e *forkScreenReaderEngine) Sessions() engine.SessionManager { return e.sessions }
func (e *forkScreenReaderEngine) Provider() provider.Provider     { return screenReaderLifecycleProvider{} }

func TestScreenReaderForkListsNewestFirstAndSelectsByNumber(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "source"
	messages := []types.Message{
		types.UserMessage("first question"), types.AssistantMessage("first answer"),
		types.UserMessage("second question"), types.AssistantMessage("second answer"),
	}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	manager := engine.NewRepositorySessionManager(repo, func() string { return projectDir })
	eng := &forkScreenReaderEngine{sessions: manager}
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	openedID := ""
	cfg := TUIREPLConfig{
		Engine: eng, Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd,
		SessionTransitionMu: &sync.Mutex{},
		OpenSessionTerminal: func(_ context.Context, forkID, _ string, _, _ string) error {
			openedID = forkID
			return nil
		},
	}

	handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), "/fork")
	if err != nil || !handled || exit {
		t.Fatalf("list fork points: handled=%t exit=%t err=%v", handled, exit, err)
	}
	text := output.String()
	newest := bytes.Index([]byte(text), []byte("1. second question"))
	older := bytes.Index([]byte(text), []byte("2. first question"))
	if newest < 0 || older <= newest {
		t.Fatalf("fork list is not newest-first:\n%s", text)
	}

	handled, exit, err = handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), "/fork 2")
	if err != nil || !handled || exit || openedID == "" {
		t.Fatalf("select fork point: handled=%t exit=%t opened=%q err=%v", handled, exit, openedID, err)
	}
	forked, ref, err := repo.LoadByID(openedID, projectDir)
	if err != nil || ref.ProjectDir != projectDir {
		t.Fatalf("load fork: ref=%+v err=%v", ref, err)
	}
	if len(forked) != 2 || forked[0].GetText() != "first question" || forked[1].GetText() != "first answer" {
		t.Fatalf("selected fork transcript = %#v", forked)
	}
}

func TestScreenReaderForkEmptyHistoryEmitsWarningWithoutError(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "empty-source"
	if err := repo.Save(sessionID, projectDir, nil); err != nil {
		t.Fatal(err)
	}
	eng := &forkScreenReaderEngine{sessions: engine.NewRepositorySessionManager(repo, func() string { return projectDir })}
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	cfg := TUIREPLConfig{Engine: eng, Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd}

	if err := forkScreenReaderSession(context.Background(), cfg, renderer, []string{"/fork"}); err != nil {
		t.Fatalf("empty fork history returned an error: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "Warning:") || strings.Contains(got, "Error:") {
		t.Fatalf("empty fork history output = %q, want warning without error", got)
	}
}
