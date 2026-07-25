package app

import (
	"time"

	agentruntime "github.com/agent-dance/luban/internal/agent"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	tooltasks "github.com/agent-dance/luban/internal/tools/tasks"
)

// taskBackgroundAdapter keeps task tools dependent on their consumer-owned
// runtime port while the application owns the concrete agent task manager.
type taskBackgroundAdapter struct {
	manager *agentruntime.BackgroundTaskManager
}

func newTaskBackgroundAdapter(manager *agentruntime.BackgroundTaskManager) tooltasks.BackgroundTasks {
	if manager == nil {
		return nil
	}
	return &taskBackgroundAdapter{manager: manager}
}

func (a *taskBackgroundAdapter) Stop(id string) (agentcontract.TaskSnapshot, error) {
	return a.manager.Stop(id)
}

func (a *taskBackgroundAdapter) Wait(id string, timeout time.Duration) (agentcontract.TaskSnapshot, string) {
	return a.manager.Wait(id, timeout)
}

func (a *taskBackgroundAdapter) Snapshot(id string) (agentcontract.TaskSnapshot, bool) {
	return a.manager.Snapshot(id)
}

func (a *taskBackgroundAdapter) ReadOutput(snapshot agentcontract.TaskSnapshot, maxBytes int64) (tooltasks.BackgroundOutput, error) {
	output, err := agentruntime.ReadTaskOutput(snapshot.OutputPath, maxBytes)
	if err != nil {
		return tooltasks.BackgroundOutput{}, err
	}
	return tooltasks.BackgroundOutput{
		Content:      output.Content,
		WasTruncated: output.WasTruncated,
	}, nil
}
