//go:build darwin

package sandbox

import (
	"context"
	"strings"
	"testing"
)

// TestSeatbeltProfile verifies profile string generation without running sandbox-exec.
func TestSeatbeltProfile(t *testing.T) {
	b := SeatbeltBackend{}

	if b.Name() != "sandbox-exec" {
		t.Errorf("Name() = %q, want %q", b.Name(), "sandbox-exec")
	}

	t.Run("deny default header", func(t *testing.T) {
		p := buildSeatbeltProfile(Config{})
		mustContain(t, p, "(version 1)")
		mustContain(t, p, "(deny default)")
		mustContain(t, p, "(allow process-exec)")
		mustContain(t, p, "(allow process-fork)")
		// F12: must NOT have unrestricted mach-lookup
		mustNotContain(t, p, "(allow mach-lookup)")
		mustContain(t, p, `(allow mach-lookup (global-name "com.apple.SecurityServer"))`)
	})

	t.Run("base system paths always present", func(t *testing.T) {
		p := buildSeatbeltProfile(Config{})
		for _, path := range []string{"/usr", "/bin", "/Library", "/System", "/private/etc", "/dev"} {
			if !strings.Contains(p, path) {
				t.Errorf("profile missing base path %q", path)
			}
		}
		mustContain(t, p, `(allow file-read* (literal "/"))`)
	})

	t.Run("read-only paths", func(t *testing.T) {
		cfg := Config{ReadOnlyPaths: []string{"/home/user/src", "/opt/tools"}}
		p := buildSeatbeltProfile(cfg)
		mustContain(t, p, `(allow file-read* (subpath "/home/user/src"))`)
		mustContain(t, p, `(allow file-read* (subpath "/opt/tools"))`)
		// Should NOT grant write.
		mustNotContain(t, p, `(allow file-write* (subpath "/home/user/src"))`)
	})

	t.Run("read-write paths", func(t *testing.T) {
		cfg := Config{ReadWritePaths: []string{"/home/user/out"}}
		p := buildSeatbeltProfile(cfg)
		mustContain(t, p, `(allow file-read* (subpath "/home/user/out"))`)
		mustContain(t, p, `(allow file-write* (subpath "/home/user/out"))`)
	})

	t.Run("host filesystem read-write", func(t *testing.T) {
		p := buildSeatbeltProfile(Config{ReadWritePaths: []string{"/"}})
		mustContain(t, p, `(allow file-read* (subpath "/"))`)
		mustContain(t, p, `(allow file-write* (subpath "/"))`)
	})

	t.Run("temp paths always writable", func(t *testing.T) {
		p := buildSeatbeltProfile(Config{})
		mustContain(t, p, `(allow file-write* (subpath "/tmp"))`)
		mustContain(t, p, `(allow file-write* (subpath "/private/var/folders"))`)
	})

	t.Run("no network when AllowedDomains empty", func(t *testing.T) {
		p := buildSeatbeltProfile(Config{AllowedDomains: []string{}})
		// localhost is always allowed
		mustContain(t, p, `(allow network* (remote ip "localhost:*"))`)
		// broad (allow network*) must NOT appear
		lines := strings.Split(p, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "(allow network*)" {
				t.Error("profile must not have unrestricted (allow network*) when AllowedDomains is empty")
			}
		}
	})

	t.Run("allow all network with wildcard domain", func(t *testing.T) {
		p := buildSeatbeltProfile(Config{AllowedDomains: []string{"*"}})
		mustContain(t, p, "(allow network*)")
	})

	// F3: specific domains must NOT silently upgrade to allow-all; network is denied.
	t.Run("specific domains deny network (phase 2)", func(t *testing.T) {
		p := buildSeatbeltProfile(Config{AllowedDomains: []string{"example.com"}})
		lines := strings.Split(p, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "(allow network*)" {
				t.Error("profile must not have unrestricted (allow network*) for specific domain list — network should be denied (Phase 2)")
			}
		}
		// localhost is still allowed
		mustContain(t, p, `(allow network* (remote ip "localhost:*"))`)
	})

	t.Run("path quoting escapes special chars", func(t *testing.T) {
		// A path with a double-quote in it (unusual but must be handled).
		quoted := seatbeltQuote(`/home/user/my"dir`)
		if !strings.Contains(quoted, `\"`) {
			t.Errorf("seatbeltQuote did not escape double-quote: %s", quoted)
		}
		// Must start and end with double quotes.
		if !strings.HasPrefix(quoted, `"`) || !strings.HasSuffix(quoted, `"`) {
			t.Errorf("seatbeltQuote output not wrapped in quotes: %s", quoted)
		}
	})

	// F2: seatbeltQuote must strip control characters.
	t.Run("path quoting strips newlines", func(t *testing.T) {
		quoted := seatbeltQuote("/tmp/foo\nbar")
		if strings.Contains(quoted, "\n") {
			t.Errorf("seatbeltQuote did not strip newline: %s", quoted)
		}
		// Resulting value should contain both path segments joined.
		mustContain(t, quoted, "/tmp/foo")
		mustContain(t, quoted, "bar")
	})

	t.Run("path quoting strips carriage return", func(t *testing.T) {
		quoted := seatbeltQuote("/tmp/foo\rbar")
		if strings.Contains(quoted, "\r") {
			t.Errorf("seatbeltQuote did not strip carriage return: %s", quoted)
		}
	})

	t.Run("path quoting strips tab", func(t *testing.T) {
		quoted := seatbeltQuote("/tmp/foo\tbar")
		if strings.Contains(quoted, "\t") {
			t.Errorf("seatbeltQuote did not strip tab: %s", quoted)
		}
	})

	t.Run("path quoting strips null byte", func(t *testing.T) {
		quoted := seatbeltQuote("/tmp/foo\x00bar")
		if strings.Contains(quoted, "\x00") {
			t.Errorf("seatbeltQuote did not strip null byte: %s", quoted)
		}
	})

	t.Run("path quoting handles parentheses", func(t *testing.T) {
		// Parentheses are valid in paths and should pass through unchanged.
		quoted := seatbeltQuote("/home/user/my(dir)")
		mustContain(t, quoted, "(dir)")
	})
}

// TestSeatbeltValidation verifies that Command() rejects invalid paths before
// building the profile.
func TestSeatbeltValidation(t *testing.T) {
	t.Run("relative ReadOnlyPath rejected", func(t *testing.T) {
		b := SeatbeltBackend{}
		cfg := Config{ReadOnlyPaths: []string{"relative/path"}}
		_, err := b.Command(nil, cfg, "echo")
		if err == nil {
			t.Error("expected error for relative ReadOnlyPath, got nil")
		}
	})

	t.Run("relative ReadWritePath rejected", func(t *testing.T) {
		b := SeatbeltBackend{}
		cfg := Config{ReadWritePaths: []string{"relative/path"}}
		_, err := b.Command(nil, cfg, "echo")
		if err == nil {
			t.Error("expected error for relative ReadWritePath, got nil")
		}
	})

	t.Run("path with newline rejected", func(t *testing.T) {
		b := SeatbeltBackend{}
		cfg := Config{ReadWritePaths: []string{"/tmp/foo\nbar"}}
		_, err := b.Command(nil, cfg, "echo")
		if err == nil {
			t.Error("expected error for path containing newline, got nil")
		}
	})
}

// TestSeatbeltAvailable checks Available() without asserting its value
// (sandbox-exec may or may not be present depending on macOS version/SIP).
func TestSeatbeltAvailable(t *testing.T) {
	b := SeatbeltBackend{}
	avail := b.Available()
	t.Logf("SeatbeltBackend.Available() = %v", avail)
}

func TestSeatbeltCommandUsesPreparedAbsoluteExecutableCapability(t *testing.T) {
	b := SeatbeltBackend{}
	capability, ok := b.SandboxCapability()
	if !ok {
		t.Skip("sandbox-exec trusted capability unavailable")
	}
	cmd, err := b.Command(context.Background(), Config{}, "echo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != capability.ExecutablePath || cmd.Args[0] != capability.ExecutablePath {
		t.Fatalf("command executable = path %q argv0 %q, capability %q", cmd.Path, cmd.Args[0], capability.ExecutablePath)
	}
}
