package tools

import (
	"context"
	"fmt"
	"io"
	"time"
)

// StartCronPromptExecution runs a cron prompt as a background local agent task
// so TaskOutput/TaskStop observe the same runtime substrate as Bash/Agent.
// If the job's prompt is a recognised sentinel (e.g. <<autonomous-loop>>) it
// is expanded just before launching.
func StartCronPromptExecution(agentTool *AgentTool, background *BackgroundTaskManager, job *CronJob) error {
	if job == nil {
		return fmt.Errorf("cron execution dependencies are unavailable")
	}
	if cronDisabledByFeatureFlag() || !IdleGateConsult(context.Background(), job.ID) {
		return nil
	}
	if agentTool == nil || background == nil {
		return fmt.Errorf("cron execution dependencies are unavailable")
	}

	prompt, err := ResolvePrompt(job.Prompt)
	if err != nil {
		return fmt.Errorf("cron job %s: %w", job.ID, err)
	}

	description := fmt.Sprintf("cron %s", job.ID)
	_, err = background.StartAgentTask(context.Background(), prompt, description, func(ctx context.Context, _ io.Writer) (string, error) {
		summary, err := agentTool.runSubAgent(ctx, fmt.Sprintf("cron-%s-%d", job.ID, time.Now().UnixNano()), AgentInput{
			Description: description,
			Prompt:      prompt,
		}, nil)
		return summary.Output, err
	})
	return err
}
