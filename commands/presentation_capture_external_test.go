package commands_test

import (
	"strings"

	"github.com/agent-dance/luban/commands"
)

func captureCompletedCommand(output *strings.Builder) func(commands.CommandPresentation) {
	return func(event commands.CommandPresentation) {
		if event.State == commands.CommandStateCompleted {
			output.WriteString(event.Result)
		}
	}
}
