package input

import (
	"errors"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestClipboardCauseErrorPreservesCause(t *testing.T) {
	cause := errors.New("clipboard backend unavailable")
	err := clipboardCauseError(i18n.KeyAuxClipboardCreateTemp, cause, cause)
	if !errors.Is(err, cause) {
		t.Fatalf("localized clipboard error lost cause: %v", err)
	}
	if err.Error() == cause.Error() {
		t.Fatalf("clipboard error omitted semantic copy: %v", err)
	}
}
