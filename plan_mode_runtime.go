package main

import (
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/tools"
)

func bindPlanModePermissionDispatcher(scope *tools.RuntimeScope, state *tools.PlanState, checker *permissions.Checker) error {
	if scope == nil || checker == nil {
		return nil
	}
	scope.SetPermissionModeDispatcher(
		func() string { return permissionCheckerModeName(checker.Mode()) },
		func(mode string) error { return setPermissionCheckerMode(checker, mode) },
	)
	if state != nil && state.IsActive() {
		if err := scope.TransitionPermissionMode("plan"); err != nil {
			return rootRuntimeWrap(i18n.KeyRootPlanModeRestore, err)
		}
	}
	return nil
}

func permissionCheckerModeName(mode permissions.Mode) string {
	switch mode {
	case permissions.ModeAllowAll:
		return "bypassPermissions"
	case permissions.ModeRuleBased:
		return "ruleBased"
	default:
		return "default"
	}
}

func setPermissionCheckerMode(checker *permissions.Checker, mode string) error {
	switch mode {
	case "auto", "acceptEdits", "bypassPermissions":
		// This is either an explicit UI transition or restoration of a mode the
		// user selected before model-initiated plan entry.
		return checker.SetModeFromUser(permissions.ModeAllowAll)
	case "ruleBased":
		return checker.SetMode(permissions.ModeRuleBased)
	case "default", "dontAsk", "bubble", "plan":
		return checker.SetMode(permissions.ModeAskAlways)
	default:
		return rootRuntimeError(i18n.KeyRootPlanModeUnsupported, mode)
	}
}
