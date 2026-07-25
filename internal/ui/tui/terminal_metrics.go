package tui

import (
	"github.com/agent-dance/luban/observability"
	gotui "github.com/grindlemire/go-tui"
)

func init() {
	// The nested terminal module cannot import the host runtime. This bounded
	// observer bridges owner-channel rejections without exposing control bytes.
	gotui.InstallTerminalControlObserver(func(reason gotui.TerminalControlRejection) {
		switch reason {
		case gotui.TerminalControlNoOwner:
			observability.RecordTerminalControlRejected(observability.TerminalWriteNoOwner)
		case gotui.TerminalControlUnavailable:
			observability.RecordTerminalControlRejected(observability.TerminalWriteUnavailable)
		default:
			observability.RecordTerminalControlRejected(observability.TerminalWriteFailure)
		}
	})
}
