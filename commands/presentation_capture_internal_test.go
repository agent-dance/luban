package commands

import "strings"

func captureCompletedCommand(output *strings.Builder) func(CommandPresentation) {
	return func(event CommandPresentation) {
		if event.State == CommandStateCompleted {
			output.WriteString(event.Result)
		}
	}
}
