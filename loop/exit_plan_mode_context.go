package loop

import (
	"strings"

	"github.com/agent-dance/luban/types"
)

func planModeContextRestart(results []types.ToolResultBlock) (string, bool) {
	for _, result := range results {
		if result.IsError || result.Metadata["clearContext"] != "true" || result.Metadata["restartExecution"] != "true" {
			continue
		}
		content := strings.TrimSpace(result.TextContent())
		if content == "" {
			content = "The user approved exiting plan mode. Continue implementing the approved plan."
		}
		return content, true
	}
	return "", false
}
