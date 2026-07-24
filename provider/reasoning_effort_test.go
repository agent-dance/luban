package provider

import "testing"

func TestDefaultReasoningEffort(t *testing.T) {
	tests := []struct {
		name    string
		efforts []string
		want    string
	}{
		{name: "empty", want: ""},
		{name: "prefers medium", efforts: []string{"low", "medium", "high"}, want: "medium"},
		{name: "provider first tier", efforts: []string{"max"}, want: "max"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DefaultReasoningEffort(test.efforts); got != test.want {
				t.Fatalf("DefaultReasoningEffort(%v) = %q, want %q", test.efforts, got, test.want)
			}
		})
	}
}

func TestReasoningEffortForRequest(t *testing.T) {
	tests := map[string]string{
		"ultra":  "max",
		"ULTRA":  "max",
		" max ":  "max",
		"xhigh":  "xhigh",
		"custom": "custom",
	}
	for input, want := range tests {
		if got := reasoningEffortForRequest(input); got != want {
			t.Fatalf("reasoningEffortForRequest(%q) = %q, want %q", input, got, want)
		}
	}
}
