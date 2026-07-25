package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/brand"
)

func TestFileTokenStorePersistsLocalFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := NewFileTokenStore(path)
	key := "srv|abc"
	want := StoredOAuthCredentials{ServerName: "srv", ServerURL: "https://mcp.example.test", AccessToken: "token"}
	if err := store.Save(context.Background(), key, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok, err := store.Load(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("Load ok=%v err=%v", ok, err)
	}
	if got.AccessToken != "token" {
		t.Fatalf("AccessToken = %q", got.AccessToken)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("token store permissions too open: %s", info.Mode().Perm())
	}
}

func TestServerKeyChangesWithSecurityRelevantConfig(t *testing.T) {
	base := ServerDescriptor{Transport: "http", URL: "https://mcp.example.test", Headers: map[string]string{"X-Tenant": "one"}}
	first := ServerKey("repo", base)
	base.Headers["X-Tenant"] = "two"
	second := ServerKey("repo", base)
	if first == second {
		t.Fatal("credential key did not change with header configuration")
	}
}

func TestDefaultOAuthTokenStoreUsesCurrentBrandDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	store, ok := defaultOAuthTokenStore().(*FileTokenStore)
	if !ok {
		t.Fatalf("default store type = %T", defaultOAuthTokenStore())
	}
	want := filepath.Join(home, brand.ConfigDirName, "mcp_oauth_tokens.json")
	if store.path != want {
		t.Fatalf("default store path = %q, want %q", store.path, want)
	}
}
