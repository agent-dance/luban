package tools

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// BashExecutionMode controls the behavioural envelope around BashTool.
type BashExecutionMode string

const (
	// BashModeDefault applies normal permission gates.
	BashModeDefault BashExecutionMode = ""
	// BashModePlan blocks any command with a non-read effect.
	// TS canonical name is "acceptEdits".
	BashModePlan BashExecutionMode = "acceptEdits"
	// BashModeSafe blocks destructive commands but permits writes/network.
	// TS canonical name is "dontAsk".
	BashModeSafe BashExecutionMode = "dontAsk"
	// BashModeYolo disables all mode-level checks.
	// TS canonical name is "bypassPermissions".
	BashModeYolo BashExecutionMode = "bypassPermissions"
)

// Legacy mode names that have been renamed. ValidateCommandForMode rejects
// them with an explicit error so callers update their config.
var deprecatedModeNames = map[string]string{
	"plan": "acceptEdits",
	"safe": "dontAsk",
	"yolo": "bypassPermissions",
}

// ValidateCommandForMode returns nil when `cmd` is permitted under `mode`,
// or an error explaining the violation. The semantics may be precomputed by
// the caller; pass SemanticUnknown to recompute.
func ValidateCommandForMode(cmd string, semantics CommandSemantic, mode BashExecutionMode) error {
	if cmd == "" {
		return nil
	}
	rawMode := string(mode)
	if rawMode == "" {
		return nil
	}
	// Reject legacy names loudly.
	if canonical, ok := deprecatedModeNames[strings.ToLower(rawMode)]; ok {
		return i18n.NewError(i18n.KeyToolIndirectBashModeDeprecated, rawMode, canonical)
	}

	switch BashExecutionMode(rawMode) {
	case BashModeYolo:
		return nil
	case BashModeDefault:
		return nil
	case BashModePlan, BashModeSafe:
		// handled below
	default:
		return i18n.NewError(i18n.KeyToolIndirectBashModeUnknown, rawMode)
	}

	if semantics == SemanticUnknown {
		semantics = ClassifyCommand(cmd)
	}
	switch BashExecutionMode(rawMode) {
	case BashModePlan:
		// acceptEdits permits read-only commands only; non-reads (including
		// destructive) are blocked just like the legacy plan mode.
		if semantics != SemanticRead {
			return i18n.NewError(i18n.KeyToolIndirectBashModeNonReadForbidden, semantics.String())
		}
	case BashModeSafe:
		if semantics == SemanticDestructive {
			return i18n.NewError(i18n.KeyToolIndirectBashModeDestructive)
		}
		if warning, fire := DestructiveCommandWarning(cmd); fire && warning != "" {
			return i18n.NewError(i18n.KeyToolIndirectBashModePattern, warning)
		}
	}
	return nil
}
