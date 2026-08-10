package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoopBackend verifies that NoopBackend passes through commands unchanged.
func TestNoopBackend(t *testing.T) {
	ctx := context.Background()
	b := NoopBackend{}

	if b.Name() != "none" {
		t.Errorf("Name() = %q, want %q", b.Name(), "none")
	}
	if !b.Available() {
		t.Error("Available() = false, want true")
	}
	workDir := t.TempDir()

	cfg := Config{
		WorkDir: workDir,
		Environment: captureEnvironment(nil, NewEnvironmentPolicy(nil, map[string]string{
			"FOO": "bar",
		})),
	}
	cmd, err := b.Command(ctx, cfg, "echo", "hello")
	if err != nil {
		t.Fatalf("Command() error: %v", err)
	}
	if cmd.Path == "" {
		t.Error("cmd.Path is empty")
	}
	if cmd.Dir != workDir {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, workDir)
	}
	if len(cmd.Env) != 1 || cmd.Env[0] != "FOO=bar" {
		t.Errorf("cmd.Env = %v, want [FOO=bar]", cmd.Env)
	}
}

func TestNoopBackendIsNotRealSandboxCapability(t *testing.T) {
	if IsRealBackend(NoopBackend{}) {
		t.Fatal("NoopBackend was accepted as real sandbox authority")
	}
}

// TestNoopBackendZeroEnvironment verifies that a zero snapshot never leaves
// cmd.Env nil, which would inherit every parent credential through os/exec.
func TestNoopBackendZeroEnvironment(t *testing.T) {
	t.Setenv("LUBAN_TEST_API_KEY", "noop-secret-sentinel")
	ctx := context.Background()
	b := NoopBackend{}
	cmd, err := b.Command(ctx, Config{}, "echo")
	if err != nil {
		t.Fatalf("Command() error: %v", err)
	}
	if cmd.Env == nil {
		t.Fatal("cmd.Env is nil and would inherit the parent environment")
	}
	for _, entry := range cmd.Env {
		if strings.Contains(entry, "noop-secret-sentinel") {
			t.Fatal("NoopBackend leaked a filtered credential")
		}
	}
}

// TestNoopBackend_NoWorkDir verifies WorkDir is not set when empty.
func TestNoopBackend_NoWorkDir(t *testing.T) {
	ctx := context.Background()
	b := NoopBackend{}
	cmd, err := b.Command(ctx, Config{}, "true")
	if err != nil {
		t.Fatalf("Command() error: %v", err)
	}
	if cmd.Dir != "" {
		t.Errorf("cmd.Dir = %q, want empty string", cmd.Dir)
	}
}

// TestDetect verifies that Detect always returns a non-nil, Available() Backend.
func TestDetect(t *testing.T) {
	b := Detect()
	if b == nil {
		t.Fatal("Detect() returned nil")
	}
	if b.Name() == "" {
		t.Error("Detect() returned a backend with empty Name()")
	}
	if !b.Available() {
		t.Errorf("Detect() returned backend %q that reports Available()=false", b.Name())
	}
}

// TestValidatePaths verifies path validation logic.
func TestValidatePaths(t *testing.T) {
	t.Run("valid absolute paths", func(t *testing.T) {
		if err := validatePaths([]string{t.TempDir(), t.TempDir()}); err != nil {
			t.Errorf("unexpected error for valid paths: %v", err)
		}
	})

	t.Run("empty path rejected", func(t *testing.T) {
		if err := validatePaths([]string{""}); err == nil {
			t.Error("expected error for empty path, got nil")
		}
	})

	t.Run("relative path rejected", func(t *testing.T) {
		if err := validatePaths([]string{"relative/path"}); err == nil {
			t.Error("expected error for relative path, got nil")
		}
	})

	t.Run("path with newline rejected", func(t *testing.T) {
		if err := validatePaths([]string{filepath.Join(t.TempDir(), "foo\nbar")}); err == nil {
			t.Error("expected error for path with newline, got nil")
		}
	})

	t.Run("path with carriage return rejected", func(t *testing.T) {
		if err := validatePaths([]string{filepath.Join(t.TempDir(), "foo\rbar")}); err == nil {
			t.Error("expected error for path with carriage return, got nil")
		}
	})

	t.Run("path with tab rejected", func(t *testing.T) {
		if err := validatePaths([]string{filepath.Join(t.TempDir(), "foo\tbar")}); err == nil {
			t.Error("expected error for path with tab, got nil")
		}
	})

	t.Run("nil/empty slice ok", func(t *testing.T) {
		if err := validatePaths(nil); err != nil {
			t.Errorf("unexpected error for nil paths: %v", err)
		}
		if err := validatePaths([]string{}); err != nil {
			t.Errorf("unexpected error for empty paths: %v", err)
		}
	})
}

// TestSafeEnv verifies environment variable filtering.
func TestSafeEnv(t *testing.T) {
	t.Run("keeps PATH and HOME", func(t *testing.T) {
		env := []string{"PATH=/usr/bin", "HOME=/root", "USER=alice"}
		filtered := SafeEnv(env)
		if !contains(filtered, "PATH=/usr/bin") {
			t.Error("SafeEnv dropped PATH")
		}
		if !contains(filtered, "HOME=/root") {
			t.Error("SafeEnv dropped HOME")
		}
		if !contains(filtered, "USER=alice") {
			t.Error("SafeEnv dropped USER")
		}
	})

	t.Run("strips KEY variables", func(t *testing.T) {
		env := []string{"AWS_ACCESS_KEY=abc123", "API_KEY=secret", "MYKEY=val"}
		filtered := SafeEnv(env)
		for _, e := range filtered {
			k, _, _ := strings.Cut(e, "=")
			upper := strings.ToUpper(k)
			if strings.Contains(upper, "KEY") {
				t.Errorf("SafeEnv kept KEY variable: %s", e)
			}
		}
	})

	t.Run("strips TOKEN variables", func(t *testing.T) {
		env := []string{"GITHUB_TOKEN=ghp_abc", "AUTH_TOKEN=xyz", "PATH=/usr/bin"}
		filtered := SafeEnv(env)
		for _, e := range filtered {
			k, _, _ := strings.Cut(e, "=")
			upper := strings.ToUpper(k)
			if strings.Contains(upper, "TOKEN") {
				t.Errorf("SafeEnv kept TOKEN variable: %s", e)
			}
		}
		if !contains(filtered, "PATH=/usr/bin") {
			t.Error("SafeEnv dropped PATH")
		}
	})

	t.Run("strips SECRET variables", func(t *testing.T) {
		env := []string{"MY_SECRET=shhh", "DB_PASSWORD=pass", "PATH=/usr/bin"}
		filtered := SafeEnv(env)
		for _, e := range filtered {
			k, _, _ := strings.Cut(e, "=")
			upper := strings.ToUpper(k)
			if strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") {
				t.Errorf("SafeEnv kept secret variable: %s", e)
			}
		}
	})

	t.Run("strips CREDENTIAL and AUTH variables", func(t *testing.T) {
		env := []string{"GIT_CREDENTIALS=tok", "ANTHROPIC_AUTH_TOKEN=tok2", "HOME=/root"}
		filtered := SafeEnv(env)
		for _, e := range filtered {
			k, _, _ := strings.Cut(e, "=")
			upper := strings.ToUpper(k)
			if strings.Contains(upper, "CREDENTIAL") || strings.Contains(upper, "AUTH") {
				t.Errorf("SafeEnv kept credential/auth variable: %s", e)
			}
		}
		if !contains(filtered, "HOME=/root") {
			t.Error("SafeEnv dropped HOME")
		}
	})

	t.Run("keeps LC_ prefix variables", func(t *testing.T) {
		env := []string{"LC_ALL=en_US.UTF-8", "LC_TIME=C", "LANG=en_US.UTF-8"}
		filtered := SafeEnv(env)
		if !contains(filtered, "LC_ALL=en_US.UTF-8") {
			t.Error("SafeEnv dropped LC_ALL")
		}
		if !contains(filtered, "LANG=en_US.UTF-8") {
			t.Error("SafeEnv dropped LANG")
		}
	})

	t.Run("keeps XDG_ prefix variables", func(t *testing.T) {
		env := []string{"XDG_RUNTIME_DIR=/run/user/1000", "XDG_DATA_HOME=/home/user/.local"}
		filtered := SafeEnv(env)
		if !contains(filtered, "XDG_RUNTIME_DIR=/run/user/1000") {
			t.Error("SafeEnv dropped XDG_RUNTIME_DIR")
		}
	})

	t.Run("keeps common compiler settings", func(t *testing.T) {
		env := []string{
			"GOFLAGS=-mod=readonly", "GOMODCACHE=/cache/go", "CGO_ENABLED=1",
			"CARGO_TARGET_DIR=/cache/cargo", "RUSTFLAGS=-Cdebuginfo=0",
			"CC=clang", "CXX=clang++", "CMAKE_GENERATOR=Ninja",
			"JAVA_HOME=/opt/jdk", "GRADLE_USER_HOME=/cache/gradle",
			"NODE_OPTIONS=--max-old-space-size=4096", "PYTHONPATH=/workspace/lib",
		}
		filtered := SafeEnv(env)
		for _, entry := range env {
			if !contains(filtered, entry) {
				t.Errorf("SafeEnv dropped build setting %q", entry)
			}
		}
	})

	t.Run("keeps offline build settings", func(t *testing.T) {
		env := []string{
			"CARGO_NET_OFFLINE=true", "GOPROXY=off", "GOSUMDB=off",
			"NPM_CONFIG_OFFLINE=true", "PNPM_CONFIG_OFFLINE=true", "YARN_ENABLE_NETWORK=0",
			"PIP_NO_INDEX=1", "UV_OFFLINE=1", "MAVEN_ARGS=--offline",
			"MVNW_REPOURL=http://127.0.0.1:9",
		}
		filtered := SafeEnv(env)
		for _, entry := range env {
			if !contains(filtered, entry) {
				t.Errorf("SafeEnv dropped offline build setting %q", entry)
			}
		}
	})

	t.Run("strips secret shaped names inside safe prefixes", func(t *testing.T) {
		env := []string{
			"NPM_CONFIG_AUTH_TOKEN=npm-secret",
			"npm_config_password=npm-password",
			"XDG_CLIENT_SECRET=xdg-secret",
			"XDG_HTTP_AUTHORIZATION=authorization-secret",
			"CMAKE_API_KEY=cmake-secret",
			"CMAKE_SIGNING_KEY=cmake-key-secret",
		}
		filtered := SafeEnv(env)
		if len(filtered) != 0 {
			t.Fatalf("SafeEnv kept secret-shaped prefixed values: %v", filtered)
		}
	})

	t.Run("nil input returns explicit empty environment", func(t *testing.T) {
		if result := SafeEnv(nil); result == nil || len(result) != 0 {
			t.Errorf("SafeEnv(nil) = %#v, want non-nil empty environment", result)
		}
	})
}

func TestEnvironmentPolicyExplicitDelegationAndOverride(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"OPENAI_API_KEY=host-secret",
		"CUSTOM_BUILD_ROOT=/host/build",
	}
	policy := NewEnvironmentPolicy(
		[]string{"OPENAI_API_KEY", "CUSTOM_BUILD_ROOT"},
		map[string]string{"CUSTOM_BUILD_ROOT": "/override/build", "EMPTY_VALUE": ""},
	)
	snapshot := captureEnvironment(parent, policy)
	entries := environmentSnapshotEntries(snapshot)
	if !contains(entries, "OPENAI_API_KEY=host-secret") {
		t.Fatal("explicit allowlist did not delegate the requested variable")
	}
	if !contains(entries, "CUSTOM_BUILD_ROOT=/override/build") {
		t.Fatal("explicit override did not take precedence")
	}
	if !contains(entries, "EMPTY_VALUE=") {
		t.Fatal("explicit empty override was dropped")
	}
	if strings.Contains(policy.Fingerprint(), "host-secret") || strings.Contains(policy.Fingerprint(), "/override/build") {
		t.Fatal("environment policy fingerprint disclosed a raw value")
	}
}

func environmentSnapshotEntries(snapshot EnvironmentSnapshot) []string {
	command := &exec.Cmd{}
	snapshot.Apply(command)
	return command.Env
}

func TestEnvironmentSnapshotFingerprintBindsResolvedValues(t *testing.T) {
	policy := NewEnvironmentPolicy([]string{"CUSTOM_BUILD_ROOT"}, nil)
	first := captureEnvironment([]string{"CUSTOM_BUILD_ROOT=/one"}, policy)
	second := captureEnvironment([]string{"CUSTOM_BUILD_ROOT=/two"}, policy)
	if first.Fingerprint() == second.Fingerprint() {
		t.Fatal("resolved environment fingerprints did not bind the delegated value")
	}
	if strings.Contains(first.Fingerprint(), "/one") || strings.Contains(second.Fingerprint(), "/two") {
		t.Fatal("resolved environment fingerprint disclosed a raw value")
	}
}

func TestEnvironmentAuthorityDebugAndJSONFormattingRedactsValues(t *testing.T) {
	const secret = "formatting-secret-sentinel"
	policy := NewEnvironmentPolicy(nil, map[string]string{"CUSTOM_BUILD_ROOT": secret})
	snapshot := captureEnvironment(nil, policy)
	encoded, err := json.Marshal(struct {
		Policy   EnvironmentPolicy   `json:"policy"`
		Snapshot EnvironmentSnapshot `json:"snapshot"`
	}{Policy: policy, Snapshot: snapshot})
	if err != nil {
		t.Fatalf("marshal environment authority: %v", err)
	}
	config := Config{Environment: snapshot}
	surfaces := fmt.Sprintf("%v\n%+v\n%#v\n%+v\n%#v\n%s", policy, snapshot, snapshot, config, config, encoded)
	if strings.Contains(surfaces, secret) {
		t.Fatal("environment authority formatting disclosed an override value")
	}
}

// contains is a helper for TestSafeEnv.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// mustContain is a test helper used by platform-specific test files.
func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("missing %q in:\n%s", needle, haystack)
	}
}

// mustNotContain is a test helper used by platform-specific test files.
func mustNotContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Errorf("must not contain %q in:\n%s", needle, haystack)
	}
}
