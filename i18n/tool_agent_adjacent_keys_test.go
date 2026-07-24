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

func TestToolAgentAdjacentEnglishCompatibility(t *testing.T) {
	cause := errors.New("raw-cause-42")
	tests := map[Key]struct {
		args []any
		want string
	}{
		KeyToolAgentRemoteParentPermissionSnapshotRequired:  {want: "Agent error: remote runtime must explicitly declare and enforce the parent permission snapshot"},
		KeyToolAgentRemoteProfileRestrictionsRequired:       {want: "Agent error: remote runtime must explicitly declare and enforce resolved profile restrictions"},
		KeyToolAgentRemoteFailClosedPromptsRequired:         {want: "Agent error: remote runtime must explicitly declare fail-closed permission prompt handling"},
		KeyToolAgentRemoteAuthenticationRequired:            {want: `Agent error: isolation="remote" requires an authenticated claude.ai session`},
		KeyToolAgentRemoteEncodeSpawnFailed:                 {args: []any{cause}, want: "encode remote agent spawn: raw-cause-42"},
		KeyToolAgentRemoteBuildSpawnRequestFailed:           {args: []any{cause}, want: "build remote agent spawn request: raw-cause-42"},
		KeyToolAgentRemoteSpawnRequestFailed:                {args: []any{cause}, want: "remote agent spawn failed: raw-cause-42"},
		KeyToolAgentRemoteSpawnRejected:                     {args: []any{503, "raw-body-17"}, want: "remote agent spawn returned 503: raw-body-17"},
		KeyToolAgentRemoteReadSpawnResponseFailed:           {args: []any{cause}, want: "read remote agent spawn response: raw-cause-42"},
		KeyToolAgentRemoteDecodeSpawnResponseFailed:         {args: []any{cause}, want: "decode remote agent spawn response: raw-cause-42"},
		KeyToolAgentRemoteTaskIDMissing:                     {want: "remote agent spawn returned no taskId"},
		KeyToolAgentRemotePermissionSnapshotUnacknowledged:  {want: "remote agent runtime did not acknowledge parent permission snapshot enforcement"},
		KeyToolAgentRemotePromptRoutingUnacknowledged:       {want: "remote agent runtime did not acknowledge fail-closed prompt routing"},
		KeyToolAgentRemoteProfileRestrictionsUnacknowledged: {want: "remote agent runtime did not acknowledge profile restriction enforcement"},
		KeyToolAgentMCPManagerMissingDetail:                 {want: "no MCP manager configured"},
		KeyToolAgentMCPManagerNotConfigured:                 {want: "WaitForMCPReadiness: no MCP manager configured"},
		KeyToolAgentMCPServerNotConfiguredDetail:            {want: "server is not configured"},
		KeyToolAgentMCPRequiredServersNotConfigured:         {args: []any{"github, slack"}, want: "Agent error: required MCP servers are not configured: github, slack"},
		KeyToolAgentMCPReadinessFailed:                      {args: []any{"github: raw-detail"}, want: "Agent error: MCP readiness failed for: github: raw-detail"},
		KeyToolAgentMCPReadinessTimedOutWithCause:           {args: []any{cause}, want: "readiness timed out: raw-cause-42"},
		KeyToolAgentMCPReadinessTimedOut:                    {want: "readiness timed out"},
		KeyToolAgentPluginConfigDirectoryUnavailable:        {want: "could not resolve LUBAN Code config directory for plugin agents"},
		KeyToolAgentPluginPermissionModeUnsupported:         {args: []any{"/tmp/reviewer.md", cause}, want: `plugin agent "/tmp/reviewer.md": raw-cause-42`},
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

func TestToolAgentAdjacentErrorsUseRuntimeLanguageAndPreserveCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := errors.New("raw-cause-42")
	err := WrapError(KeyToolAgentRemoteSpawnRequestFailed, cause)
	if !errors.Is(err, cause) {
		t.Fatal("semantic Agent error did not preserve its cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese {
		t.Fatalf("runtime language did not change Agent error: %q", english)
	}
	if !strings.Contains(chinese, "raw-cause-42") {
		t.Fatalf("localized Agent error omitted raw cause: %q", chinese)
	}
	if !errors.Is(err, cause) {
		t.Fatal("localized Agent error no longer preserves its cause")
	}
}

func TestToolAgentAdjacentParameterizedTranslationsPreserveRawValues(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
		raw  []string
	}{
		{KeyToolAgentRemoteSpawnRejected, []any{599, "raw-body-17"}, []string{"599", "raw-body-17"}},
		{KeyToolAgentMCPRequiredServersNotConfigured, []any{"raw-server-23"}, []string{"raw-server-23"}},
		{KeyToolAgentMCPReadinessFailed, []any{"raw-detail-29"}, []string{"raw-detail-29"}},
		{KeyToolAgentPluginPermissionModeUnsupported, []any{"/raw/path-31", "raw-cause-37"}, []string{"/raw/path-31", "raw-cause-37"}},
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
