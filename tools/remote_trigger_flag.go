// Package tools — remote_trigger_flag.go provides the env-var
// kill-switch for the RemoteTrigger tool (RT-03).
package tools

import (
	"os"
	"strings"
)

// isRemoteTriggerDisabled reports whether the RemoteTrigger tool has been
// disabled by the operator via the CLAUDE_CODE_DISABLE_REMOTE_TRIGGER env
// var. Mirrors the TS featureFlag('remote_trigger_disabled') gate.
//
// Truthy values: "1", "true", "yes", "on" (case-insensitive). Empty / unset
// means enabled — matches the TS default.
func isRemoteTriggerDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_CODE_DISABLE_REMOTE_TRIGGER")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
