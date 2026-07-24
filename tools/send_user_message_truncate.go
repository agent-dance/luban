// Package tools contains the legacy transient-delivery classifier shared by
// older message adapters. SendUserMessage itself no longer retries a sink: the
// TS-aligned tool is rendered locally and has no delivery callback.
package tools

import (
	"strings"
)

// isTransientSinkError reports whether an older message adapter error looks
// transient (429, channel-closed mid-frame, or a network blip).
func isTransientSinkError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	transientMarkers := []string{
		"429",
		"too many requests",
		"timeout",
		"timed out",
		"deadline exceeded",
		"connection reset",
		"connection refused",
		"broken pipe",
		"i/o timeout",
		"channel closed",
		"temporary",
		"try again",
	}
	for _, m := range transientMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}
