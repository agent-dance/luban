package remote

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteTriggerTool_CreateUsesOAuthAPI(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotVersion, gotBeta, gotOrg, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("anthropic-version")
		gotBeta = r.Header.Get("anthropic-beta")
		gotOrg = r.Header.Get("x-organization-uuid")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"trigger-123"}`))
	}))
	defer srv.Close()

	tool := &Trigger{
		HTTPClient: srv.Client(),
		AccessTokenResolver: func(context.Context) (string, error) {
			return "token-123", nil
		},
		OrganizationUUIDResolver: func(context.Context, string, string) (string, error) {
			return "org-123", nil
		},
		BaseURLResolver: func() (string, error) {
			return srv.URL, nil
		},
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "create",
		"body":   map[string]any{},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/code/triggers" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer token-123" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("anthropic-version = %q", gotVersion)
	}
	if gotBeta != remoteTriggerBetaHeader {
		t.Fatalf("anthropic-beta = %q", gotBeta)
	}
	if gotOrg != "org-123" {
		t.Fatalf("x-organization-uuid = %q", gotOrg)
	}
	if strings.TrimSpace(gotBody) != "{}" {
		t.Fatalf("body = %q", gotBody)
	}

	decoded := parseHTTPTextResult(t, result.Content)
	if decoded["status"] != float64(http.StatusCreated) {
		t.Fatalf("status = %v", decoded["status"])
	}
	if decoded["json"] != `{"id":"trigger-123"}` {
		t.Fatalf("json = %v", decoded["json"])
	}
}

// parseHTTPTextResult parses the canonical "HTTP <code>\n<body>" envelope
// emitted by Trigger into a map mirroring the legacy JSON shape so
// existing assertions remain readable.
func parseHTTPTextResult(t *testing.T, content string) map[string]any {
	t.Helper()
	idx := strings.IndexByte(content, '\n')
	if !strings.HasPrefix(content, "HTTP ") || idx < 0 {
		t.Fatalf("expected HTTP-text envelope, got %q", content)
	}
	codeStr := strings.TrimPrefix(content[:idx], "HTTP ")
	codeStr = strings.TrimSpace(codeStr)
	body := content[idx+1:]
	var code float64
	if _, err := fmt.Sscanf(codeStr, "%f", &code); err != nil {
		t.Fatalf("invalid status %q: %v", codeStr, err)
	}
	// The body emitted by normalizeRemoteTriggerJSON is the raw JSON. The
	// legacy unit-test schema expected `json` to be the raw JSON STRING,
	// so wrap as needed.
	out := map[string]any{"status": code, "json": body}
	return out
}

func TestRemoteTriggerTool_ResolvesOrganizationFromProfile(t *testing.T) {
	var sawProfile, sawList bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/oauth/profile":
			sawProfile = true
			if r.Header.Get("Authorization") != "Bearer token-xyz" {
				t.Fatalf("profile auth header = %q", r.Header.Get("Authorization"))
			}
			if r.Header.Get("anthropic-beta") != oauthBetaHeader {
				t.Fatalf("profile beta header = %q", r.Header.Get("anthropic-beta"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"organization":{"uuid":"org-from-profile"}}`))
		case "/v1/code/triggers":
			sawList = true
			if r.Header.Get("x-organization-uuid") != "org-from-profile" {
				t.Fatalf("list org header = %q", r.Header.Get("x-organization-uuid"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	t.Setenv("LUBAN_CODE_ORGANIZATION_UUID", "")
	t.Setenv("OAUTH_ORGANIZATION_UUID", "")

	tool := &Trigger{
		HTTPClient: srv.Client(),
		AccessTokenResolver: func(context.Context) (string, error) {
			return "token-xyz", nil
		},
		BaseURLResolver: func() (string, error) {
			return srv.URL, nil
		},
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "list",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if !sawProfile {
		t.Fatal("expected profile lookup")
	}
	if !sawList {
		t.Fatal("expected trigger list call")
	}
}

func TestRemoteTriggerTool_RequiresAction(t *testing.T) {
	tool := &Trigger{}

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error, got success: %s", result.Content)
	}
	if !strings.Contains(result.Content, "action is required") {
		t.Fatalf("unexpected error: %s", result.Content)
	}
}
