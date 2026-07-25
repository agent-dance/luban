package toolbase

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

type inputFixture struct {
	Name string `json:"name"`
}

func TestParseInputAcceptsDeclaredFields(t *testing.T) {
	parsed, err := ParseInput[inputFixture](map[string]any{"name": "value"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "value" {
		t.Fatalf("name = %q, want value", parsed.Name)
	}
}

func TestParseStrictInputRejectsUnknownFields(t *testing.T) {
	_, err := ParseStrictInput[inputFixture](map[string]any{"name": "value", "unknown": true})
	if err == nil {
		t.Fatal("strict input accepted an unknown field")
	}
}

func TestParseStrictInputOrErrorReturnsFailedOutcome(t *testing.T) {
	_, result := ParseStrictInputOrError[inputFixture](map[string]any{"unknown": true})
	if result == nil || !result.IsError || result.Outcome != types.ToolOutcomeFailed {
		t.Fatalf("result = %#v, want failed tool outcome", result)
	}
}
