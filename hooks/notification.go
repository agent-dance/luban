package hooks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	gotui "github.com/grindlemire/go-tui"
)

// executeNotificationHook sends a system notification and never blocks tool
// execution (notifications are always non-blocking).
//
// Platform dispatch:
//   - macOS  → osascript "display notification"
//   - Linux  → notify-send  (falls back to terminal bell if not found)
//   - other  → terminal bell through the active terminal owner
func executeNotificationHook(ctx context.Context, hook Hook, input HookInput) HookOutput {
	// hook is unused; signature matches dispatch interface.
	_ = hook

	// N3: Cap notification delivery to 5 seconds so a hung notification process
	// never stalls the caller indefinitely.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	title := input.Title
	if title == "" {
		title = brand.DisplayName
	}
	message := input.Message
	if message == "" {
		// Fall back to a generic label derived from hook type.
		message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookNotificationDefault, string(input.Type))
	}

	var err error
	switch runtime.GOOS {
	case "darwin":
		err = sendMacOSNotification(ctx, title, message)
	case "linux":
		err = sendLinuxNotification(ctx, title, message)
	default:
		sendBellNotification()
	}

	if err != nil {
		// Notification failures are non-fatal execution failures.
		message := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookNotificationFailed, err)
		return HookOutput{ExecutionError: message}
	}
	return HookOutput{}
}

// sendMacOSNotification uses osascript to display a macOS notification.
// Title and message are passed via environment variables to prevent injection.
func sendMacOSNotification(ctx context.Context, title, message string) error {
	// N1: Use environment variables rather than inline string formatting so that
	// special characters in title/message cannot escape the AppleScript string.
	script := `display notification (system attribute "NOTIFICATION_MSG") with title (system attribute "NOTIFICATION_TITLE")`
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	cmd.Env = append(os.Environ(),
		"NOTIFICATION_MSG="+message,
		"NOTIFICATION_TITLE="+title,
	)
	// N2: Capture stderr into a buffer instead of forwarding to os.Stderr so
	// notification errors stay contained and can be returned to the caller.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, stderr.String())
		}
		return err
	}
	return nil
}

// sendLinuxNotification uses notify-send. Falls back to terminal bell if the
// binary is not available.
func sendLinuxNotification(ctx context.Context, title, message string) error {
	_, err := exec.LookPath("notify-send")
	if err != nil {
		// notify-send not available; ring the terminal bell instead.
		sendBellNotification()
		return nil
	}
	cmd := exec.CommandContext(ctx, "notify-send", title, message)
	// N2: Capture stderr into a buffer rather than forwarding to os.Stderr.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, stderr.String())
		}
		return err
	}
	return nil
}

// sendBellNotification emits the ASCII bell through the active terminal
// owner. Without an owner the notification is intentionally silent; writing
// directly to stderr would corrupt a concurrently rendered TUI frame.
func sendBellNotification() {
	_ = gotui.WriteTerminalControl([]byte{'\a'})
}
