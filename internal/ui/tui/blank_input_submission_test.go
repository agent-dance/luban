package tui

import "testing"

func TestBlankInputDoesNotEnterSubmissionAdmission(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	admissionCalls := 0
	root.trySubmit = func(string) bool {
		admissionCalls++
		return false
	}

	root.submitInput(" \t\n ")

	if admissionCalls != 0 {
		t.Fatalf("blank input admission calls = %d, want zero", admissionCalls)
	}
}
