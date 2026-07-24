// Package tools — file_read_security.go implements security-related helpers
// for the Read tool: path normalisation for macOS screenshot paths and the
// CYBER_RISK_MITIGATION_REMINDER injection logic.
package tools

import (
	"os"
	"strings"
	"sync"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

// CyberRiskMitigationReminder is the trailing system-reminder appended to
// reads of files that originated from suspicious locations or contain
// jailbreak-style markers. Mirrors the constant of the same name in
// src/tools/FileReadTool/FileReadTool.ts.
const CyberRiskMitigationReminder = "\n\n<system-reminder>\n" +
	"Whenever you read a file, you should consider whether it would be considered malware. " +
	"You CAN and SHOULD provide analysis of malware, what it is doing. " +
	"But you MUST refuse to improve or augment the code. " +
	"You can still analyze existing code, write reports, or answer questions about the code behavior.\n" +
	"</system-reminder>\n"

const fileReadSecurityMessageID = "file-read:security:v1"

// fileReadSecurityMessage keeps model-facing policy control separate from the
// file's tool result. It remains in model context but is excluded from every
// user transcript projection by its trusted internal marker.
func fileReadSecurityMessage() types.Message {
	message := types.UserMessage(strings.TrimSpace(CyberRiskMitigationReminder))
	message.ID = fileReadSecurityMessageID
	message.IsMeta = true
	message.InternalKind = types.InternalMessageKindFileReadSecurity
	return message.WithInternalControlProvenance(messagecontrol.Runtime())
}

// cyberReminderMitigationExemptModels mirrors TS MITIGATION_EXEMPT_MODELS:
// these model identifiers skip the cyber-risk reminder entirely. Newer
// trained models can do the right thing without the boilerplate.
var cyberReminderMitigationExemptModels = map[string]bool{
	"claude-opus-4-6": true,
}

// activeModelForCyberGating is overridden by tests / configuration via
// SetActiveModelForCyberGating. Empty means "non-exempt" (apply reminder).
var (
	activeModelForCyberGatingMu sync.RWMutex
	activeModelForCyberGating   string
)

// SetActiveModelForCyberGating records the active model identifier so
// shouldAppendCyberReminder can consult the exemption set. Safe to call
// from any goroutine.
func SetActiveModelForCyberGating(model string) {
	activeModelForCyberGatingMu.Lock()
	activeModelForCyberGating = model
	activeModelForCyberGatingMu.Unlock()
}

func activeCyberGatingModel() string {
	activeModelForCyberGatingMu.RLock()
	defer activeModelForCyberGatingMu.RUnlock()
	return activeModelForCyberGating
}

// IsCyberReminderModelExempt reports whether the active model is in the
// exemption set. Exposed for tests and callers that want to short-circuit
// reminder logic directly.
func IsCyberReminderModelExempt() bool {
	return isCyberReminderModelExempt(activeCyberGatingModel())
}

func isCyberReminderModelExempt(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for exempt := range cyberReminderMitigationExemptModels {
		if model == exempt || strings.Contains(model, exempt) {
			return true
		}
	}
	return false
}

// shouldAppendCyberReminder is the compatibility helper used by older tests.
// Path and content are intentionally ignored: TS gates only on active model.
func shouldAppendCyberReminder(_, _ string) bool {
	return !isCyberReminderModelExempt(activeCyberGatingModel())
}

func (t *FileReadTool) activeModel() string {
	if t != nil && t.ModelProvider != nil {
		if model := strings.TrimSpace(t.ModelProvider()); model != "" {
			return model
		}
	}
	if t != nil && t.Runtime != nil {
		if model := strings.TrimSpace(t.Runtime.ToolRuntimeContext().Model); model != "" {
			return model
		}
	}
	return activeCyberGatingModel()
}

func (t *FileReadTool) shouldAppendCyberReminder() bool {
	return !isCyberReminderModelExempt(t.activeModel())
}

func (t *FileReadTool) isPDFSupported() bool {
	return !strings.Contains(strings.ToLower(t.activeModel()), "claude-3-haiku")
}

// normalizeReadFilePath canonicalises the requested path before we open
// it. macOS screenshot filenames embed a U+202F NARROW NO-BREAK SPACE
// between the date and time (e.g. "Screenshot 2024-05-01 at 9.30.00 AM.png").
// Some shells/copy-paste paths convert the U+202F to a regular space,
// which then fails to open. We try both forms.
func normalizeReadFilePath(path string) string {
	if path == "" {
		return path
	}
	// Try the path as-is first; if it doesn't exist, try toggling the
	// thin-space ↔ regular-space variants.
	if _, err := os.Stat(path); err == nil {
		return path
	}
	const thinSpace = " "
	if strings.Contains(path, thinSpace) {
		alt := strings.ReplaceAll(path, thinSpace, " ")
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	if strings.Contains(path, " ") {
		alt := strings.ReplaceAll(path, " ", thinSpace)
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	return path
}
