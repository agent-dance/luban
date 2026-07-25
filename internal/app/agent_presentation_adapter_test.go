package app

import "testing"

func TestAgentBackgroundPresentationPortPreservesNil(t *testing.T) {
	if got := agentBackgroundPresentationPort(nil); got != nil {
		t.Fatalf("nil manager produced a non-nil presentation port: %#v", got)
	}
}
