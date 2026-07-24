package hooks

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskCreatedTask26ConfigFilenameAndCommandBlocking(t *testing.T) {
	settings := []byte(`{"hooks":{"TaskCreated":[{"hooks":[{"type":"command","command":"exit 2"}]}]}}`)
	runner, err := LoadConfigData(settings, "task26-settings")
	if err != nil {
		t.Fatalf("LoadConfigData: %v", err)
	}
	_, err = runner.RunBlocking(context.Background(), HookTaskCreated, HookInput{TaskID: "7", TaskSubject: "blocked"})
	if err == nil {
		t.Fatal("TaskCreated command exit 2 did not block")
	}
	var blocking *BlockingError
	if !strings.Contains(err.Error(), "TaskCreated") || !errors.As(err, &blocking) {
		t.Fatalf("TaskCreated block was not typed: %T %v", err, err)
	}

	for _, filename := range []string{"task-created-a.sh", "taskcreated-b.sh"} {
		if got := hookTypeFromFilename(filename); got != HookTaskCreated {
			t.Errorf("hookTypeFromFilename(%q) = %q", filename, got)
		}
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "task-created-capture.sh")
	capture := filepath.Join(dir, "payload.json")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat > \""+capture+"\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	outputs := loaded.Run(context.Background(), HookTaskCreated, HookInput{TaskID: "9", TaskSubject: "capture"})
	if len(outputs) != 1 {
		t.Fatalf("TaskCreated filename hook outputs = %d", len(outputs))
	}
	body, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `"hook_event_name":"TaskCreated"`) || !strings.Contains(text, `"task_id":"9"`) {
		t.Fatalf("captured native TaskCreated payload = %s", body)
	}
}

func TestTaskCreatedTask26HTTPStructuredBlock(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	var payload string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		payload = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"block":true,"system_reminder":"http policy"}`)
	}))
	defer server.Close()

	runner := NewRunner([]Hook{{Type: HookTaskCreated, Kind: HookKindHTTP, URL: server.URL, Timeout: 2}})
	_, err := runner.RunBlocking(context.Background(), HookTaskCreated, HookInput{
		TaskID: "17", TaskSubject: "HTTP task", TeamName: "team-a", TeammateName: "worker-a",
	})
	if err == nil || !strings.Contains(err.Error(), "http policy") {
		t.Fatalf("HTTP TaskCreated hook did not block: %v", err)
	}
	for _, field := range []string{`"hook_event_name":"TaskCreated"`, `"task_id":"17"`, `"team_name":"team-a"`, `"teammate_name":"worker-a"`} {
		if !strings.Contains(payload, field) {
			t.Errorf("HTTP payload missing %s: %s", field, payload)
		}
	}
}
