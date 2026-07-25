package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestToolAgentAdjacentKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolAgentAdjacentKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolAgentAdjacentEnglishContract(t *testing.T) {
	cause := errors.New("raw-cause-42")
	tests := map[Key]struct {
		args []any
		want string
	}{
		KeyToolAgentMCPManagerMissingDetail:          {want: "no MCP manager configured"},
		KeyToolAgentMCPManagerNotConfigured:          {want: "WaitForMCPReadiness: no MCP manager configured"},
		KeyToolAgentMCPServerNotConfiguredDetail:     {want: "server is not configured"},
		KeyToolAgentMCPRequiredServersNotConfigured:  {args: []any{"github, slack"}, want: "Agent error: required MCP servers are not configured: github, slack"},
		KeyToolAgentMCPReadinessFailed:               {args: []any{"github: raw-detail"}, want: "Agent error: MCP readiness failed for: github: raw-detail"},
		KeyToolAgentMCPReadinessTimedOutWithCause:    {args: []any{cause}, want: "readiness timed out: raw-cause-42"},
		KeyToolAgentMCPReadinessTimedOut:             {want: "readiness timed out"},
		KeyToolAgentPluginConfigDirectoryUnavailable: {want: "could not resolve LUBAN Code config directory for plugin agents"},
	}

	if len(tests) != len(toolAgentAdjacentKeys) {
		t.Fatalf("English compatibility cases = %d, keys = %d", len(tests), len(toolAgentAdjacentKeys))
	}
	for _, key := range toolAgentAdjacentKeys {
		test, ok := tests[key]
		if !ok {
			t.Errorf("missing English compatibility case for %s", key)
			continue
		}
		if got := Format(LangEN, key, test.args...); got != test.want {
			t.Errorf("Format(LangEN, %s) = %q, want %q", key, got, test.want)
		}
	}
}

func TestToolAgentAdjacentParameterizedTranslationsPreserveRawValues(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
		raw  []string
	}{
		{KeyToolAgentMCPRequiredServersNotConfigured, []any{"raw-server-23"}, []string{"raw-server-23"}},
		{KeyToolAgentMCPReadinessFailed, []any{"raw-detail-29"}, []string{"raw-detail-29"}},
	}
	for _, test := range tests {
		for _, lang := range AllLanguages() {
			got := Format(lang, test.key, test.args...)
			for _, raw := range test.raw {
				if !strings.Contains(got, raw) {
					t.Errorf("Format(%s, %s) omitted %q: %q", lang.Code(), test.key, raw, got)
				}
			}
		}
	}
}
