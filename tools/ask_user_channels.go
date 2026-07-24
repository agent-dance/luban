// Package tools — askuser-channels-isenabled.
//
// Mirrors src/tools/AskUserQuestionTool/AskUserQuestionTool.tsx:135-145
// (`isEnabled` short-circuit). The TS UI disables the multiple-choice
// AskUserQuestion dialog when the harness has been routed onto a remote
// channel (Telegram, Discord, Slack relay) because the TUI dialog has no
// UI surface to render — calling the tool would just block on stdin
// forever. The Go side reproduces that signal via the KAIROS_CHANNELS
// environment variable, which the host harness sets when a remote
// channel is active.
package tools

import (
	"os"
	"strings"
	"sync/atomic"
)

// askUserChannelsActiveOverride lets tests force a value without mutating
// the process env. 0 = unset, 1 = forced active, -1 = forced inactive.
var askUserChannelsActiveOverride int32

// SetAskUserChannelsActiveForTest forces AskUserChannelsActive to return
// the given value. Pass nil/0 to clear. Tests should defer-reset.
func SetAskUserChannelsActiveForTest(active *bool) {
	if active == nil {
		atomic.StoreInt32(&askUserChannelsActiveOverride, 0)
		return
	}
	if *active {
		atomic.StoreInt32(&askUserChannelsActiveOverride, 1)
	} else {
		atomic.StoreInt32(&askUserChannelsActiveOverride, -1)
	}
}

// AskUserChannelsActive reports whether the harness has signalled that the
// user is currently on a remote channel. The signal is the KAIROS_CHANNELS
// environment variable: any non-empty, non-"0", non-"false" value enables.
func AskUserChannelsActive() bool {
	if v := atomic.LoadInt32(&askUserChannelsActiveOverride); v != 0 {
		return v == 1
	}
	val := strings.ToLower(strings.TrimSpace(os.Getenv("KAIROS_CHANNELS")))
	if val == "" {
		return false
	}
	switch val {
	case "0", "false", "off", "no":
		return false
	}
	return true
}
