package tasktools

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	agentruntime "github.com/agent-dance/luban/internal/agent"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
)

type backgroundManagerPort struct {
	manager *agentruntime.BackgroundTaskManager
}

func (p backgroundManagerPort) Stop(id string) (agentcontract.TaskSnapshot, error) {
	return p.manager.Stop(id)
}

func (p backgroundManagerPort) Wait(id string, timeout time.Duration) (agentcontract.TaskSnapshot, string) {
	return p.manager.Wait(id, timeout)
}

func (p backgroundManagerPort) Snapshot(id string) (agentcontract.TaskSnapshot, bool) {
	return p.manager.Snapshot(id)
}

func (p backgroundManagerPort) ReadOutput(snapshot agentcontract.TaskSnapshot, limit int64) (BackgroundOutput, error) {
	output, err := agentruntime.ReadTaskOutput(snapshot.OutputPath, limit)
	return BackgroundOutput{Content: output.Content, WasTruncated: output.WasTruncated}, err
}

func TestBackgroundToolsIntegrateThroughReceiptAdapter(t *testing.T) {
	manager := agentruntime.NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() {
		if err := manager.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	port := backgroundManagerPort{manager: manager}
	outputTool := NewTaskOutputTool(port)
	stopTool := NewTaskStopTool(port)

	completed, err := manager.StartShellTask(context.Background(), "printf output", "print", exec.Command("sh", "-c", "printf 'hello from task'"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := outputTool.Execute(context.Background(), map[string]any{"task_id": completed.ID, "block": true, "timeout": 5000})
	if err != nil || result.IsError || !strings.Contains(result.Content, "hello from task") {
		t.Fatalf("output=%#v err=%v", result, err)
	}

	running, err := manager.StartShellTask(context.Background(), "sleep", "sleep", exec.Command("sh", "-c", "sleep 30"))
	if err != nil {
		t.Fatal(err)
	}
	result, err = outputTool.Execute(context.Background(), map[string]any{"task_id": running.ID, "block": false})
	if err != nil || result.IsError || !strings.Contains(result.Content, "<retrieval_status>not_ready</retrieval_status>") {
		t.Fatalf("nonblocking output=%#v err=%v", result, err)
	}
	stopped, err := stopTool.Execute(context.Background(), map[string]any{"task_id": running.ID})
	if err != nil || stopped.IsError {
		t.Fatalf("stop=%#v err=%v", stopped, err)
	}
}
