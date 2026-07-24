package sandbox

import (
	"context"
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
		Env:     []string{"FOO=bar"},
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

// TestNoopBackend_NilEnv verifies that nil Env leaves cmd.Env nil (inherit from parent).
func TestNoopBackend_NilEnv(t *testing.T) {
	ctx := context.Background()
	b := NoopBackend{}
	cmd, err := b.Command(ctx, Config{}, "echo")
	if err != nil {
		t.Fatalf("Command() error: %v", err)
	}
	if cmd.Env != nil {
		t.Errorf("cmd.Env = %v, want nil (inherit from parent)", cmd.Env)
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

	t.Run("nil input returns nil", func(t *testing.T) {
		if result := SafeEnv(nil); result != nil {
			t.Errorf("SafeEnv(nil) = %v, want nil", result)
		}
	})
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
