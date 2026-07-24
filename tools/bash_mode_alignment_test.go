package tools

// Alignment red tests for bash_mode_validation.go.
//
// Audit ref: alignment_audit.md P1-4 — bash_mode_validation.go currently
// uses the legacy mode names plan/safe/yolo. The TS reference uses
// acceptEdits / bypassPermissions / dontAsk (see permissionMode.ts in the
// reference repo). These tests pin the desired names so the gap is observable.
//
// Each test calls ValidateCommandForMode with the TS-canonical mode string
// and expects the validator to recognise it. The current implementation
// silently passes any unknown mode (the switch falls through to a no-op),
// which means tests must check semantic behaviour, not just "is recognised".

import (
	"strings"
	"testing"
)

//  1. The TS-canonical "acceptEdits" mode should permit read-only commands and
//     behave like Plan mode for non-reads. Today BashModePlan == "plan", so
//     passing "acceptEdits" lands in BashModeDefault path which never blocks.
func TestBashAlignment_Mode_AcceptEdits_BlocksDestructive(t *testing.T) {
	// TS ref: permissionMode.ts — "acceptEdits" must reject destructive ops.
	err := ValidateCommandForMode("rm -rf /tmp/foo", SemanticDestructive, BashExecutionMode("acceptEdits"))
	if err == nil {
		t.Errorf("acceptEdits mode should reject destructive command, got nil error")
	}
}

//  2. "bypassPermissions" should be recognised as the TS-canonical name for
//     yolo-equivalent mode (no checks). Currently the switch falls through to
//     BashModeDefault, which is also no-op, so the only way to detect the gap
//     is to confirm the constant string value matches.
func TestBashAlignment_Mode_BypassPermissions_IsCanonicalConstant(t *testing.T) {
	// Audit P1-4: BashModeYolo string should be "bypassPermissions" not "yolo".
	if string(BashModeYolo) != "bypassPermissions" {
		t.Errorf("BashModeYolo should equal \"bypassPermissions\" (TS canonical), got %q", string(BashModeYolo))
	}
}

//  3. "dontAsk" is the TS-canonical name for safe-mode-equivalent. The Go
//     constant is "safe"; this pins the rename.
func TestBashAlignment_Mode_DontAsk_IsCanonicalConstant(t *testing.T) {
	// Audit P1-4: BashModeSafe string should be "dontAsk" not "safe".
	if string(BashModeSafe) != "dontAsk" {
		t.Errorf("BashModeSafe should equal \"dontAsk\" (TS canonical), got %q", string(BashModeSafe))
	}
}

//  4. The legacy "plan" name should NOT be the canonical constant. TS uses
//     "acceptEdits". Locking the rename in both directions.
func TestBashAlignment_Mode_AcceptEdits_IsCanonicalConstant(t *testing.T) {
	if string(BashModePlan) != "acceptEdits" {
		t.Errorf("BashModePlan should equal \"acceptEdits\" (TS canonical), got %q", string(BashModePlan))
	}
}

//  5. Unknown / legacy mode names should produce a clear error (or at least be
//     explicitly rejected). Currently ValidateCommandForMode silently treats
//     anything unrecognised as default = permit. This is a spec violation:
//     a typo'd mode name should fail loudly.
func TestBashAlignment_Mode_LegacyPlanName_IsRejected(t *testing.T) {
	// After rename, the legacy name "plan" must not be silently accepted —
	// it should error so callers update their config.
	err := ValidateCommandForMode("rm -rf /tmp/foo", SemanticDestructive, BashExecutionMode("plan"))
	if err == nil {
		t.Errorf("legacy mode name \"plan\" should be rejected (renamed to acceptEdits), got nil")
		return
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown") &&
		!strings.Contains(strings.ToLower(err.Error()), "deprecated") &&
		!strings.Contains(strings.ToLower(err.Error()), "renamed") {
		// Loose contract — just want SOME indication it's no longer valid.
		t.Errorf("error for legacy \"plan\" should mention unknown/deprecated/renamed, got %q", err.Error())
	}
}

//  6. Sanity: default mode permits any benign command. Locks the false-positive
//     contract so the rename doesn't accidentally block ordinary commands.
func TestBashAlignment_Mode_NegativeDefaultPermitsBenign(t *testing.T) {
	if err := ValidateCommandForMode("ls -la", SemanticRead, BashModeDefault); err != nil {
		t.Errorf("default mode should permit benign read, got error: %v", err)
	}
}
