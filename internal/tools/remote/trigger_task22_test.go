package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestRemoteTriggerStrictInputRejectsLegacyURLBeforeNetwork(t *testing.T) {
	var contacted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tool := &Trigger{
		HTTPClient: srv.Client(),
		AccessTokenResolver: func(context.Context) (string, error) {
			t.Fatal("access token resolver must not run for invalid input")
			return "", nil
		},
		OrganizationUUIDResolver: func(context.Context, string, string) (string, error) {
			t.Fatal("organization resolver must not run for invalid input")
			return "", nil
		},
		BaseURLResolver: func() (string, error) { return srv.URL, nil },
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "run",
		"url":    srv.URL,
		"method": "POST",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected strict input error, got success: %s", result.Content)
	}
	if contacted {
		t.Fatal("legacy url input caused network traffic")
	}
	if !strings.Contains(result.Content, `unknown field "url"`) &&
		!strings.Contains(result.Content, `unknown field "method"`) {
		t.Fatalf("strict error did not mention legacy fields: %s", result.Content)
	}
}

func TestRemoteTriggerSchemaRejectsUnknownFields(t *testing.T) {
	schema := (&Trigger{}).Schema()
	if !schema.RejectsUnknownFields() {
		t.Fatalf("RemoteTrigger schema must be strict: %#v", schema)
	}
	if _, ok := schema.Properties["url"]; ok {
		t.Fatal("RemoteTrigger schema must not expose legacy url")
	}
	if _, ok := schema.Properties["method"]; ok {
		t.Fatal("RemoteTrigger schema must not expose legacy method")
	}
}

func TestRemoteTriggerCustomOAuthURLAllowlist(t *testing.T) {
	t.Setenv("LUBAN_CODE_CUSTOM_OAUTH_URL", "https://evil.example")
	if _, err := (&Trigger{}).resolveRemoteTriggerOAuthConfig(); err == nil ||
		!strings.Contains(err.Error(), "not an approved endpoint") {
		t.Fatalf("unapproved custom OAuth URL error = %v", err)
	}
	tool := &Trigger{AccessTokenResolver: func(context.Context) (string, error) {
		t.Fatal("access token resolver must not run for an unapproved OAuth endpoint")
		return "", nil
	}}
	result, err := tool.Execute(context.Background(), map[string]any{"action": "list"})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "not an approved endpoint") {
		t.Fatalf("unapproved endpoint Execute err=%v result=%#v", err, result)
	}

	t.Setenv("LUBAN_CODE_CUSTOM_OAUTH_URL", "https://claude.fedstart.com/")
	cfg, err := (&Trigger{}).resolveRemoteTriggerOAuthConfig()
	if err != nil {
		t.Fatalf("approved custom OAuth URL: %v", err)
	}
	if cfg.APIBaseURL != "https://claude.fedstart.com" {
		t.Fatalf("API base = %q", cfg.APIBaseURL)
	}
	if cfg.OAuthConfig.TokenURL != "https://claude.fedstart.com/v1/oauth/token" {
		t.Fatalf("token URL = %q", cfg.OAuthConfig.TokenURL)
	}
}

func TestRemoteTriggerStagingOAuthConfig(t *testing.T) {
	t.Setenv("USER_TYPE", "ant")
	t.Setenv("USE_STAGING_OAUTH", "1")
	cfg, err := (&Trigger{}).resolveRemoteTriggerOAuthConfig()
	if err != nil {
		t.Fatalf("staging OAuth config: %v", err)
	}
	if cfg.APIBaseURL != stagingOAuthAPIBaseURL {
		t.Fatalf("staging API base = %q", cfg.APIBaseURL)
	}
	if cfg.OAuthConfig.TokenURL != stagingOAuthTokenURL {
		t.Fatalf("staging token URL = %q", cfg.OAuthConfig.TokenURL)
	}
}

func TestRemoteTriggerOAuthConfigPrecedenceAndClientOverride(t *testing.T) {
	t.Setenv("USER_TYPE", "ant")
	t.Setenv("USE_STAGING_OAUTH", "1")
	t.Setenv("LUBAN_CODE_CUSTOM_OAUTH_URL", "https://claude.fedstart.com/")
	t.Setenv("LUBAN_CODE_OAUTH_CLIENT_ID", "client-override")

	cfg, err := (&Trigger{}).resolveRemoteTriggerOAuthConfig()
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if cfg.APIBaseURL != "https://claude.fedstart.com" ||
		cfg.OAuthConfig.TokenURL != "https://claude.fedstart.com/v1/oauth/token" ||
		cfg.OAuthConfig.AuthURL != "https://claude.fedstart.com/oauth/authorize" {
		t.Fatalf("custom config = %#v", cfg)
	}
	if cfg.OAuthConfig.ClientID != "client-override" {
		t.Fatalf("client ID = %q", cfg.OAuthConfig.ClientID)
	}
}

func TestRemoteTrigger429NoRetryAfterBodyMutation(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
	}))
	defer srv.Close()

	tool := &Trigger{
		HTTPClient:               srv.Client(),
		AccessTokenResolver:      func(context.Context) (string, error) { return "tok", nil },
		OrganizationUUIDResolver: func(context.Context, string, string) (string, error) { return "org", nil },
		BaseURLResolver:          func() (string, error) { return srv.URL, nil },
	}
	result, err := tool.Execute(context.Background(), map[string]any{"action": "create", "body": map[string]any{}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one request for 429, got %d", calls)
	}
	if !strings.Contains(result.Content, "HTTP 429") || !strings.Contains(result.Content, `{"error":"rate_limited"}`) {
		t.Fatalf("429 response not surfaced: %s", result.Content)
	}
	if strings.Contains(result.Content, "_retry_after_seconds") {
		t.Fatalf("Retry-After was injected into body: %s", result.Content)
	}
}

func TestRemoteTriggerMutatingActionsDoNotRetryFailures(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{name: "create", input: map[string]any{"action": "create", "body": map[string]any{}}},
		{name: "update", input: map[string]any{"action": "update", "trigger_id": "trigger-1", "body": map[string]any{}}},
		{name: "run", input: map[string]any{"action": "run", "trigger_id": "trigger-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"unavailable"}`))
			}))
			defer srv.Close()

			result, err := newTask22RemoteTriggerTool(srv).Execute(context.Background(), tt.input)
			if err != nil || result.IsError {
				t.Fatalf("Execute err=%v result=%#v", err, result)
			}
			if calls != 1 {
				t.Fatalf("requests = %d, want 1", calls)
			}
			if result.Content != `HTTP 503
{"error":"unavailable"}` {
				t.Fatalf("response = %q", result.Content)
			}
		})
	}
}

func TestRemoteTriggerStrictInputMetadataAndMapper(t *testing.T) {
	tool := &Trigger{}
	if schema := tool.Schema(); !schema.RejectsUnknownFields() {
		t.Fatalf("input schema is not strict: %#v", schema)
	}
	metadata := tool.ToolMetadata(nil)
	if metadata.MaxResultSizeChars != maxResultSizeChars {
		t.Fatalf("max result size = %d", metadata.MaxResultSizeChars)
	}
	if !metadata.ConcurrencySafe {
		t.Fatal("RemoteTrigger must be concurrency safe")
	}
	discovery := registry.DiscoveryMetadata(tool)
	if !discovery.ShouldDefer || discovery.SearchHint == "" {
		t.Fatalf("RemoteTrigger discovery metadata = %#v", discovery)
	}

	block := types.MapToolResult(tool, types.ToolResult{
		Data: triggerOutput{Status: http.StatusCreated, JSON: `{"id":"x"}`},
	}, "toolu_rt")
	if block.Content != "HTTP 201\n{\"id\":\"x\"}" {
		t.Fatalf("mapped output = %q", block.Content)
	}
	if block.Metadata["maxResultSizeChars"] != "100000" {
		t.Fatalf("result metadata = %#v", block.Metadata)
	}
}

func TestRemoteTriggerReadOnlyAndClassifierMetadata(t *testing.T) {
	tool := &Trigger{}
	for _, action := range []string{"list", "get"} {
		if !tool.ToolMetadata(map[string]any{"action": action}).ReadOnly {
			t.Fatalf("%s should be read-only", action)
		}
	}
	for _, action := range []string{"create", "update", "run"} {
		if tool.ToolMetadata(map[string]any{"action": action}).ReadOnly {
			t.Fatalf("%s should not be read-only", action)
		}
	}
}

func TestRemoteTriggerOAuthEnvAndFileDescriptorSources(t *testing.T) {
	t.Run("environment token", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("LUBAN_CODE_OAUTH_TOKEN", "env-token")
		t.Setenv("LUBAN_CODE_ACCOUNT_UUID", "account")
		t.Setenv("LUBAN_CODE_USER_EMAIL", "user@example.test")
		t.Setenv("LUBAN_CODE_ORGANIZATION_UUID", "env-org")

		var gotAuth, gotOrg string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotOrg = r.Header.Get("x-organization-uuid")
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()

		result, err := newTask22ProductionAuthRemoteTriggerTool(srv).Execute(context.Background(), map[string]any{"action": "list"})
		if err != nil || result.IsError {
			t.Fatalf("env OAuth Execute err=%v result=%#v", err, result)
		}
		if gotAuth != "Bearer env-token" || gotOrg != "env-org" {
			t.Fatalf("headers auth=%q org=%q", gotAuth, gotOrg)
		}
	})

	t.Run("file descriptor token", func(t *testing.T) {
		resetRemoteTriggerOAuthFDCache()
		t.Cleanup(resetRemoteTriggerOAuthFDCache)
		t.Setenv("HOME", t.TempDir())
		t.Setenv("LUBAN_CODE_OAUTH_TOKEN", "")
		t.Setenv("LUBAN_CODE_ACCOUNT_UUID", "account")
		t.Setenv("LUBAN_CODE_USER_EMAIL", "user@example.test")
		t.Setenv("LUBAN_CODE_ORGANIZATION_UUID", "fd-org")
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatalf("Pipe: %v", err)
		}
		defer reader.Close()
		if _, err := writer.WriteString("fd-token\n"); err != nil {
			t.Fatalf("write token: %v", err)
		}
		_ = writer.Close()
		t.Setenv("LUBAN_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR", strconv.FormatUint(uint64(reader.Fd()), 10))

		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()
		result, err := newTask22ProductionAuthRemoteTriggerTool(srv).Execute(context.Background(), map[string]any{"action": "list"})
		if err != nil || result.IsError {
			t.Fatalf("FD OAuth Execute err=%v result=%#v", err, result)
		}
		if gotAuth != "Bearer fd-token" {
			t.Fatalf("authorization = %q", gotAuth)
		}
	})
}

func TestRemoteTriggerOAuthFileDescriptorSourceIsConcurrentSafe(t *testing.T) {
	resetRemoteTriggerOAuthFDCache()
	t.Cleanup(resetRemoteTriggerOAuthFDCache)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LUBAN_CODE_OAUTH_TOKEN", "")
	t.Setenv("LUBAN_CODE_ACCOUNT_UUID", "account")
	t.Setenv("LUBAN_CODE_USER_EMAIL", "user@example.test")
	t.Setenv("LUBAN_CODE_ORGANIZATION_UUID", "fd-org")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	defer reader.Close()
	if _, err := writer.WriteString("shared-fd-token\n"); err != nil {
		t.Fatalf("write token: %v", err)
	}
	_ = writer.Close()
	t.Setenv(remoteTriggerOAuthTokenFDEnv, strconv.FormatUint(uint64(reader.Fd()), 10))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer shared-fd-token" {
			t.Errorf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	tool := newTask22ProductionAuthRemoteTriggerTool(srv)

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan string, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := tool.Execute(context.Background(), map[string]any{"action": "list"})
			if err != nil || result.IsError {
				errs <- fmt.Sprintf("err=%v result=%#v", err, result)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for failure := range errs {
		t.Error(failure)
	}
}

func TestRemoteTriggerStoredOAuthScopeAndOrganizationLifecycle(t *testing.T) {
	t.Run("cached organization skips profile", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeRemoteTriggerStoredCredentials(t, home, map[string]any{
			"provider":          "anthropic",
			"access_token":      "stored-token",
			"token_type":        "Bearer",
			"scopes":            []string{"user:inference"},
			"organization_uuid": "cached-org",
		})

		var profileCalls, triggerCalls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/oauth/profile":
				profileCalls++
				_, _ = w.Write([]byte(`{"organization":{"uuid":"profile-org"}}`))
			case "/v1/code/triggers":
				triggerCalls++
				if got := r.Header.Get("x-organization-uuid"); got != "cached-org" {
					t.Fatalf("organization = %q", got)
				}
				_, _ = w.Write([]byte(`[]`))
			default:
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
		}))
		defer srv.Close()
		result, err := newTask22ProductionAuthRemoteTriggerTool(srv).Execute(context.Background(), map[string]any{"action": "list"})
		if err != nil || result.IsError {
			t.Fatalf("stored OAuth Execute err=%v result=%#v", err, result)
		}
		if profileCalls != 0 || triggerCalls != 1 {
			t.Fatalf("profile calls=%d trigger calls=%d", profileCalls, triggerCalls)
		}
	})

	t.Run("missing profile scope skips profile and fails predictably", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeRemoteTriggerStoredCredentials(t, home, map[string]any{
			"provider":     "anthropic",
			"access_token": "stored-token",
			"token_type":   "Bearer",
			"scopes":       []string{"user:inference"},
		})

		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			_, _ = w.Write([]byte(`{"organization":{"uuid":"must-not-be-used"}}`))
		}))
		defer srv.Close()
		result, err := newTask22ProductionAuthRemoteTriggerTool(srv).Execute(context.Background(), map[string]any{"action": "list"})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !result.IsError || !strings.Contains(result.Content, "Unable to resolve organization UUID") {
			t.Fatalf("missing profile scope result = %#v", result)
		}
		if calls != 0 {
			t.Fatalf("missing profile scope made %d network calls", calls)
		}
	})

	t.Run("profile organization is cached", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeRemoteTriggerStoredCredentials(t, home, map[string]any{
			"provider":     "anthropic",
			"access_token": "stored-token",
			"token_type":   "Bearer",
			"scopes":       []string{"user:profile", "user:inference"},
		})

		var profileCalls, triggerCalls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/oauth/profile":
				profileCalls++
				_, _ = w.Write([]byte(`{"organization":{"uuid":"profile-org"}}`))
			case "/v1/code/triggers":
				triggerCalls++
				if got := r.Header.Get("x-organization-uuid"); got != "profile-org" {
					t.Fatalf("organization = %q", got)
				}
				_, _ = w.Write([]byte(`[]`))
			default:
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
		}))
		defer srv.Close()
		result, err := newTask22ProductionAuthRemoteTriggerTool(srv).Execute(context.Background(), map[string]any{"action": "list"})
		if err != nil || result.IsError {
			t.Fatalf("stored OAuth Execute err=%v result=%#v", err, result)
		}
		if profileCalls != 1 || triggerCalls != 1 {
			t.Fatalf("profile calls=%d trigger calls=%d", profileCalls, triggerCalls)
		}
		data, err := os.ReadFile(filepath.Join(home, ".luban-code", ".credentials.json"))
		if err != nil {
			t.Fatalf("read cached credentials: %v", err)
		}
		if !strings.Contains(string(data), `"organization_uuid": "profile-org"`) {
			t.Fatalf("profile organization was not cached: %s", data)
		}
	})

	t.Run("unrelated provider credentials are rejected", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeRemoteTriggerStoredCredentials(t, home, map[string]any{
			"provider":          "deepseek",
			"access_token":      "wrong-provider-token",
			"token_type":        "Bearer",
			"scopes":            []string{"user:profile", "user:inference"},
			"organization_uuid": "wrong-org",
		})
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
		}))
		defer srv.Close()
		result, err := newTask22ProductionAuthRemoteTriggerTool(srv).Execute(context.Background(), map[string]any{"action": "list"})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !result.IsError || !strings.Contains(result.Content, "authenticated Anthropic account") {
			t.Fatalf("unrelated credentials result = %#v", result)
		}
		if calls != 0 {
			t.Fatalf("unrelated credentials made %d network calls", calls)
		}
	})

	t.Run("noncanonical provider alias is rejected", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeRemoteTriggerStoredCredentials(t, home, map[string]any{
			"provider":          "claude.ai",
			"access_token":      "legacy-alias-token",
			"token_type":        "Bearer",
			"scopes":            []string{"user:profile", "user:inference"},
			"organization_uuid": "legacy-alias-org",
		})
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
		}))
		defer srv.Close()
		result, err := newTask22ProductionAuthRemoteTriggerTool(srv).Execute(context.Background(), map[string]any{"action": "list"})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !result.IsError || !strings.Contains(result.Content, "authenticated Anthropic account") {
			t.Fatalf("noncanonical alias result = %#v", result)
		}
		if calls != 0 {
			t.Fatalf("noncanonical alias made %d network calls", calls)
		}
	})

	t.Run("missing inference scope is rejected", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeRemoteTriggerStoredCredentials(t, home, map[string]any{
			"provider":          "anthropic",
			"access_token":      "profile-only-token",
			"token_type":        "Bearer",
			"scopes":            []string{"user:profile"},
			"organization_uuid": "cached-org",
		})
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
		}))
		defer srv.Close()
		result, err := newTask22ProductionAuthRemoteTriggerTool(srv).Execute(context.Background(), map[string]any{"action": "list"})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !result.IsError || !strings.Contains(result.Content, "authenticated Anthropic account") {
			t.Fatalf("profile-only credentials result = %#v", result)
		}
		if calls != 0 {
			t.Fatalf("profile-only credentials made %d network calls", calls)
		}
	})

	t.Run("bare mode ignores OAuth sources", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("LUBAN_CODE_BARE", "1")
		t.Setenv("LUBAN_CODE_OAUTH_TOKEN", "must-not-be-used")
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
		}))
		defer srv.Close()
		result, err := newTask22ProductionAuthRemoteTriggerTool(srv).Execute(context.Background(), map[string]any{"action": "list"})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !result.IsError || !strings.Contains(result.Content, "authenticated Anthropic account") {
			t.Fatalf("bare mode result = %#v", result)
		}
		if calls != 0 {
			t.Fatalf("bare mode made %d network calls", calls)
		}
	})
}

func TestRemoteTriggerOutputBodyVariantsAndLargeResult(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "json", body: `{ "ok": true }`, want: "HTTP 200\n{\"ok\":true}"},
		{name: "non json", body: `plain text`, want: "HTTP 200\n\"plain text\""},
		{name: "non json whitespace", body: " \nplain text \t", want: "HTTP 200\n\" \\nplain text \\t\""},
		{name: "empty", body: ``, want: "HTTP 200\n\"\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			result, err := newTask22RemoteTriggerTool(srv).Execute(context.Background(), map[string]any{"action": "list"})
			if err != nil || result.IsError {
				t.Fatalf("Execute err=%v result=%#v", err, result)
			}
			if result.Content != tt.want {
				t.Fatalf("content = %q, want %q", result.Content, tt.want)
			}
		})
	}

	largeValue := strings.Repeat("x", maxResultSizeChars+1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"value": largeValue})
	}))
	defer srv.Close()
	tool := newTask22RemoteTriggerTool(srv)
	result, err := tool.Execute(context.Background(), map[string]any{"action": "list"})
	if err != nil || result.IsError {
		t.Fatalf("large Execute err=%v result=%#v", err, result)
	}
	if !strings.Contains(result.Content, largeValue) {
		t.Fatalf("large response was truncated before result storage: len=%d", len(result.Content))
	}
	block := types.MapToolResult(tool, result, "toolu_large")
	if block.Metadata["maxResultSizeChars"] != "100000" {
		t.Fatalf("large result metadata = %#v", block.Metadata)
	}
}

func newTask22RemoteTriggerTool(srv *httptest.Server) *Trigger {
	return &Trigger{
		HTTPClient:               srv.Client(),
		AccessTokenResolver:      func(context.Context) (string, error) { return "token", nil },
		OrganizationUUIDResolver: func(context.Context, string, string) (string, error) { return "org", nil },
		BaseURLResolver:          func() (string, error) { return srv.URL, nil },
	}
}

func newTask22ProductionAuthRemoteTriggerTool(srv *httptest.Server) *Trigger {
	return &Trigger{
		HTTPClient:      srv.Client(),
		BaseURLResolver: func() (string, error) { return srv.URL, nil },
	}
}

func writeRemoteTriggerStoredCredentials(t *testing.T, home string, value map[string]any) {
	t.Helper()
	if _, ok := value["expires_at"]; !ok {
		value["expires_at"] = time.Time{}.Format(time.RFC3339Nano)
	}
	writeRemoteTriggerJSONFile(t, filepath.Join(home, ".luban-code", ".credentials.json"), value)
}

func writeRemoteTriggerJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func resetRemoteTriggerOAuthFDCache() {
	remoteTriggerOAuthFDCache.Lock()
	remoteTriggerOAuthFDCache.descriptor = ""
	remoteTriggerOAuthFDCache.fileInfo = nil
	remoteTriggerOAuthFDCache.token = ""
	remoteTriggerOAuthFDCache.resolved = false
	remoteTriggerOAuthFDCache.Unlock()
}
