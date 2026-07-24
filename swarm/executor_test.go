package swarm

import (
	"os/exec"
	"strings"
	"testing"
)

// ---- shellQuote ----

func TestShellQuote_EmptyString(t *testing.T) {
	got, err := shellQuote("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "''"
	if got != want {
		t.Errorf("shellQuote(%q) = %q, want %q", "", got, want)
	}
}

func TestShellQuote_SimpleString(t *testing.T) {
	got, err := shellQuote("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "'hello'"
	if got != want {
		t.Errorf("shellQuote(%q) = %q, want %q", "hello", got, want)
	}
}

func TestShellQuote_SingleQuotes(t *testing.T) {
	got, err := shellQuote("it's")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "'it'\\''s'"
	if got != want {
		t.Errorf("shellQuote(%q) = %q, want %q", "it's", got, want)
	}
}

func TestShellQuote_StringWithSpaces(t *testing.T) {
	got, err := shellQuote("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "'hello world'"
	if got != want {
		t.Errorf("shellQuote(%q) = %q, want %q", "hello world", got, want)
	}
}

func TestShellQuote_SpecialChars(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"$VAR", "'$VAR'"},
		{"`cmd`", "'`cmd`'"},
		{"a;b", "'a;b'"},
	}
	for _, tc := range cases {
		got, err := shellQuote(tc.in)
		if err != nil {
			t.Fatalf("shellQuote(%q): unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestShellQuote_RejectsNewlines(t *testing.T) {
	_, err := shellQuote("hello\nworld")
	if err == nil {
		t.Error("expected error for string with newline, got nil")
	}
}

func TestShellQuote_RejectsCarriageReturn(t *testing.T) {
	_, err := shellQuote("hello\rworld")
	if err == nil {
		t.Error("expected error for string with carriage return, got nil")
	}
}

// ---- buildModelFlag ----

func TestBuildModelFlag_Empty(t *testing.T) {
	got, err := buildModelFlag("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("buildModelFlag(%q) = %q, want %q", "", got, "")
	}
}

func TestBuildModelFlag_NonEmpty(t *testing.T) {
	got, err := buildModelFlag("claude-3-opus")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := " --model 'claude-3-opus'"
	if got != want {
		t.Errorf("buildModelFlag(%q) = %q, want %q", "claude-3-opus", got, want)
	}
}

// ---- buildPermFlags ----

func TestBuildPermFlags_AllowAll(t *testing.T) {
	got, err := buildPermFlags(true, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := " --allow-all"
	if got != want {
		t.Errorf("buildPermFlags(true, nil) = %q, want %q", got, want)
	}
}

func TestBuildPermFlags_AllowAllIgnoresPerms(t *testing.T) {
	got, err := buildPermFlags(true, []string{"--allow-tool Bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := " --allow-all"
	if got != want {
		t.Errorf("buildPermFlags(true, perms) = %q, want %q", got, want)
	}
}

func TestBuildPermFlags_EmptyPerms(t *testing.T) {
	got, err := buildPermFlags(false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("buildPermFlags(false, nil) = %q, want %q", got, "")
	}
}

func TestBuildPermFlags_CustomPerms(t *testing.T) {
	got, err := buildPermFlags(false, []string{"--allow-tool Bash", "--allow-tool Read"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Values are shell-quoted by buildPermFlags
	want := " --allow-tool 'Bash' --allow-tool 'Read'"
	if got != want {
		t.Errorf("buildPermFlags(false, perms) = %q, want %q", got, want)
	}
}

func TestBuildPermFlags_RejectsUnknownFlags(t *testing.T) {
	_, err := buildPermFlags(false, []string{"; rm -rf /"})
	if err == nil {
		t.Error("expected error for unknown flag, got nil")
	}
}

func TestBuildPermFlags_RejectsInvalidFlagName(t *testing.T) {
	_, err := buildPermFlags(false, []string{"--unknown-flag value"})
	if err == nil {
		t.Error("expected error for unrecognized flag name, got nil")
	}
}

func TestBuildPermFlags_RejectsAllowAllInPerms(t *testing.T) {
	// --allow-all must be set via AllowAll field, not permissions array
	_, err := buildPermFlags(false, []string{"--allow-all"})
	if err == nil {
		t.Error("expected error for --allow-all in permissions array (privilege escalation), got nil")
	}
}

// ---- buildCommand ----

func TestBuildCommand_BasicArgs(t *testing.T) {
	cmd, err := buildCommand("/bin/ccg", "/work", "alice", "myteam", "", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd == "" {
		t.Fatal("buildCommand returned empty string")
	}
	if !strings.Contains(cmd, "env -i") {
		t.Errorf("expected env -i in: %s", cmd)
	}
	if !strings.Contains(cmd, "sh -c") {
		t.Errorf("expected sh -c in: %s", cmd)
	}
	if !strings.Contains(cmd, "--agent-id") {
		t.Errorf("expected --agent-id in: %s", cmd)
	}
	if !strings.Contains(cmd, "alice") {
		t.Errorf("expected alice in: %s", cmd)
	}
	if !strings.Contains(cmd, "--team-name") {
		t.Errorf("expected --team-name in: %s", cmd)
	}
	if !strings.Contains(cmd, "myteam") {
		t.Errorf("expected myteam in: %s", cmd)
	}
}

func TestBuildCommand_WithModel(t *testing.T) {
	cmd, err := buildCommand("/bin/ccg", "/work", "bob", "team1", "claude-3-haiku", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cmd, "--model") {
		t.Errorf("expected --model in: %s", cmd)
	}
	if !strings.Contains(cmd, "claude-3-haiku") {
		t.Errorf("expected claude-3-haiku in: %s", cmd)
	}
}

func TestBuildCommand_AllowAllTrue(t *testing.T) {
	cmd, err := buildCommand("/bin/ccg", "/work", "carol", "team1", "", true, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cmd, "--allow-all") {
		t.Errorf("expected --allow-all in: %s", cmd)
	}
}

func TestBuildCommand_AllowAllFalse(t *testing.T) {
	cmd, err := buildCommand("/bin/ccg", "/work", "dave", "team1", "", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(cmd, "--allow-all") {
		t.Errorf("buildCommand with allowAll=false should not include --allow-all, got: %s", cmd)
	}
}

func TestBuildCommand_CustomPermissions(t *testing.T) {
	cmd, err := buildCommand("/bin/ccg", "/work", "eve", "team1", "", false, []string{"--allow-tool Bash", "--allow-tool Read"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cmd, "--allow-tool") {
		t.Errorf("expected --allow-tool in: %s", cmd)
	}
	if !strings.Contains(cmd, "Bash") {
		t.Errorf("expected Bash in: %s", cmd)
	}
}

func TestBuildCommand_RejectsInvalidPerms(t *testing.T) {
	_, err := buildCommand("/bin/ccg", "/work", "evil", "team1", "", false, []string{"--allow-tool Bash", "; rm -rf /"})
	if err == nil {
		t.Error("expected error for invalid permission flag")
	}
}

func TestBuildCommand_RejectsEmptyBinary(t *testing.T) {
	_, err := buildCommand("", "/work", "alice", "team1", "", false, nil)
	if err == nil {
		t.Error("expected error for empty binary")
	}
}

func TestBuildCommand_RejectsEmptyCwd(t *testing.T) {
	_, err := buildCommand("/bin/ccg", "", "alice", "team1", "", false, nil)
	if err == nil {
		t.Error("expected error for empty cwd")
	}
}

func TestBuildCommand_RejectsNewlineInCwd(t *testing.T) {
	_, err := buildCommand("/bin/ccg", "/work\n/evil", "alice", "team1", "", false, nil)
	if err == nil {
		t.Error("expected error for newline in cwd")
	}
}

func TestShellQuote_RoundTrip(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required to validate POSIX shell quoting round-trip: %v", err)
	}
	cases := []string{
		"hello",
		"hello world",
		"it's a test",
		"$HOME",
		"`whoami`",
		"a;b&&c|d",
		"path/with spaces/and'quotes",
	}
	for _, input := range cases {
		quoted, err := shellQuote(input)
		if err != nil {
			t.Fatalf("shellQuote(%q): %v", input, err)
		}
		// Use sh -c printf to verify the quoted string round-trips correctly.
		cmd := exec.Command("sh", "-c", "printf '%s' "+quoted)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("sh -c for %q: %v", input, err)
		}
		if string(out) != input {
			t.Errorf("shellQuote roundtrip: input=%q, quoted=%s, got=%q", input, quoted, string(out))
		}
	}
}
