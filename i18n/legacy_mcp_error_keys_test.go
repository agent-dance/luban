package i18n

import (
	"errors"
	"strings"
	"testing"
)

type legacyMCPTestCause struct{}

func (*legacyMCPTestCause) Error() string { return "raw-cause-42" }

func legacyMCPErrorCases() []struct {
	key  Key
	args []any
	want string
	raw  []string
} {
	cause := errors.New("raw-cause-42")
	return []struct {
		key  Key
		args []any
		want string
		raw  []string
	}{
		{KeyLegacyMCPStdinPipe, []any{cause}, "stdin pipe: raw-cause-42", []string{"raw-cause-42"}},
		{KeyLegacyMCPStdoutPipe, []any{cause}, "stdout pipe: raw-cause-42", []string{"raw-cause-42"}},
		{KeyLegacyMCPStartServer, []any{cause}, "start MCP server: raw-cause-42", []string{"raw-cause-42"}},
		{KeyLegacyMCPInitialize, []any{cause}, "MCP initialize: raw-cause-42", []string{"raw-cause-42"}},
		{KeyLegacyMCPToolReturnedError, []any{"remote-tool-output-42\nline-2"}, "tool error: remote-tool-output-42\nline-2", []string{"remote-tool-output-42\nline-2"}},
		{KeyLegacyMCPNilClient, nil, "mcp: nil client", nil},
		{KeyLegacyMCPSSEInitialize, []any{cause}, "SSE MCP initialize: raw-cause-42", []string{"raw-cause-42"}},
		{KeyLegacyMCPHTTPPost, []any{cause}, "HTTP POST: raw-cause-42", []string{"raw-cause-42"}},
		{KeyLegacyMCPHTTPStatus, []any{418, "raw-http-body-42"}, "HTTP 418: raw-http-body-42", []string{"418", "raw-http-body-42"}},
		{KeyLegacyMCPSSEClientClosed, nil, "SSE client closed", nil},
		{KeyLegacyMCPSSEGetStatus, []any{503}, "SSE GET returned 503", []string{"503"}},
		{KeyLegacyMCPDecodeRPCEnvelope, []any{cause}, "decode RPC envelope: raw-cause-42", []string{"raw-cause-42"}},
		{KeyLegacyMCPRPCError, []any{-32001, "raw-rpc-message-42"}, "RPC error -32001: raw-rpc-message-42", []string{"-32001", "raw-rpc-message-42"}},
		{KeyLegacyMCPServerAlreadyRunning, []any{"server-42", 4242}, "MCP server \"server-42\" is already running (pid 4242)", []string{"server-42", "4242"}},
		{KeyLegacyMCPStartNamedServer, []any{"server-42", cause}, "start MCP server \"server-42\": raw-cause-42", []string{"server-42", "raw-cause-42"}},
		{KeyLegacyMCPServerNotFound, []any{"server-42"}, "MCP server \"server-42\" not found", []string{"server-42"}},
		{KeyLegacyMCPStopDuringRestart, []any{"server-42", cause}, "stop during restart of \"server-42\": raw-cause-42", []string{"server-42", "raw-cause-42"}},
		{KeyLegacyMCPStartDuringRestart, []any{"server-42", cause}, "start during restart of \"server-42\": raw-cause-42", []string{"server-42", "raw-cause-42"}},
		{KeyLegacyMCPNamedServerError, []any{"server-42", cause}, "server-42: raw-cause-42", []string{"server-42", "raw-cause-42"}},
		{KeyLegacyMCPHealthServerNotFound, []any{"server-42"}, "server \"server-42\" not found", []string{"server-42"}},
		{KeyLegacyMCPServerNotRunning, []any{"server-42"}, "server \"server-42\" is not running", []string{"server-42"}},
		{KeyLegacyMCPProcessNotAlive, []any{"server-42", cause}, "server \"server-42\" process not alive: raw-cause-42", []string{"server-42", "raw-cause-42"}},
		{KeyLegacyMCPHealthCheckFailed, []any{3}, "health check failed after 3 consecutive pings", []string{"3"}},
		{KeyLegacyMCPReconnectAttempt, []any{4, cause}, "reconnect attempt 4: raw-cause-42", []string{"4", "raw-cause-42"}},
		{KeyLegacyMCPServerDisappeared, []any{"server-42"}, "server \"server-42\" disappeared", []string{"server-42"}},
		{KeyLegacyMCPProcessUnexpectedExit, nil, "process exited unexpectedly", nil},
	}
}

func TestLegacyMCPErrorKeysCoverEveryLanguageAndPreserveRawValues(t *testing.T) {
	cases := legacyMCPErrorCases()
	if len(cases) != len(legacyMCPErrorKeys) {
		t.Fatalf("focused cases = %d, catalog keys = %d", len(cases), len(legacyMCPErrorKeys))
	}

	for _, tc := range cases {
		for _, lang := range AllLanguages() {
			got := Format(lang, tc.key, tc.args...)
			if got == "" || got == "["+string(tc.key)+"]" {
				t.Errorf("%s is missing for %s: %q", tc.key, lang.Code(), got)
			}
			if strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %s) has an invalid expansion: %q", lang.Code(), tc.key, got)
			}
			for _, raw := range tc.raw {
				if !strings.Contains(got, raw) {
					t.Errorf("Format(%s, %s) lost raw value %q: %q", lang.Code(), tc.key, raw, got)
				}
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyMCPErrorEnglishCompatibility(t *testing.T) {
	for _, tc := range legacyMCPErrorCases() {
		if got := Format(LangEN, tc.key, tc.args...); got != tc.want {
			t.Errorf("Format(LangEN, %s) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestLegacyMCPWrappedErrorsUseRuntimeLanguageAndPreserveCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := &legacyMCPTestCause{}
	err := WrapError(KeyLegacyMCPStartNamedServer, cause, "server-42")
	if !errors.Is(err, cause) {
		t.Fatal("legacy MCP error did not preserve its underlying cause")
	}
	var typedCause *legacyMCPTestCause
	if !errors.As(err, &typedCause) || typedCause != cause {
		t.Fatal("legacy MCP error did not preserve its typed underlying cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	if english != "start MCP server \"server-42\": raw-cause-42" {
		t.Fatalf("English compatibility changed: %q", english)
	}
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese || !strings.Contains(chinese, "server-42") || !strings.Contains(chinese, "raw-cause-42") {
		t.Fatalf("runtime localization lost raw diagnostics: en=%q zh=%q", english, chinese)
	}
}
