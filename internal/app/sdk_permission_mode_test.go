package app

import (
	"testing"

	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/sdk"
)

func TestInitialSDKPermissionModeFollowsAllowAll(t *testing.T) {
	tests := []struct {
		name string
		opts cli.Options
		want sdk.InitialPermissionMode
	}{
		{name: "default bridge", want: sdk.InitialPermissionBridge},
		{name: "allow all", opts: cli.Options{AllowAll: true}, want: sdk.InitialPermissionFullAuto},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := initialSDKPermissionMode(test.opts); got != test.want {
				t.Fatalf("initialSDKPermissionMode() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestInitialInteractivePermissionModeDefaultsAuto(t *testing.T) {
	tests := []struct {
		name string
		opts cli.Options
		want permissions.Mode
	}{
		{name: "interactive TUI", want: permissions.ModeAllowAll},
		{name: "screen reader", opts: cli.Options{ScreenReader: true}, want: permissions.ModeAllowAll},
		{name: "print permission bridge", opts: cli.Options{Print: true}, want: permissions.ModeAskAlways},
		{name: "SDK permission bridge", opts: cli.Options{SDK: true}, want: permissions.ModeAskAlways},
		{name: "explicit allow all in print mode", opts: cli.Options{Print: true, AllowAll: true}, want: permissions.ModeAllowAll},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := initialPermissionCheckerMode(test.opts); got != test.want {
				t.Fatalf("initialPermissionCheckerMode() = %v, want %v", got, test.want)
			}
		})
	}
}
