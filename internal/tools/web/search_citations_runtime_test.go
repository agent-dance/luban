package web

import (
	"strings"
	"testing"
)

func TestSourcesReminderIsPresent(t *testing.T) {
	if strings.TrimSpace(SourcesReminder()) == "" {
		t.Fatal("SourcesReminder must not be empty")
	}
}
