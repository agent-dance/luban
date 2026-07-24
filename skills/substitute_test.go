package skills

import (
	"strings"
	"testing"
)

func strPtr(s string) *string { return &s }

// --- ParseArguments tests ---

func TestParseArguments_SimpleWords(t *testing.T) {
	got := ParseArguments("foo bar baz")
	want := []string{"foo", "bar", "baz"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_DoubleQuoted(t *testing.T) {
	got := ParseArguments(`foo "hello world" baz`)
	want := []string{"foo", "hello world", "baz"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_SingleQuoted(t *testing.T) {
	got := ParseArguments("foo 'hello world' baz")
	want := []string{"foo", "hello world", "baz"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_Empty(t *testing.T) {
	got := ParseArguments("")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestParseArguments_Whitespace(t *testing.T) {
	got := ParseArguments("   ")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestParseArguments_MixedQuotes(t *testing.T) {
	got := ParseArguments(`"hello" 'world' plain`)
	want := []string{"hello", "world", "plain"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_BackslashEscape(t *testing.T) {
	got := ParseArguments(`hello\ world`)
	want := []string{"hello world"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_BackslashInDoubleQuotes(t *testing.T) {
	got := ParseArguments(`"hello\"world"`)
	want := []string{`hello"world`}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_BackslashInSingleQuotes(t *testing.T) {
	// Inside single quotes, backslash is literal (POSIX behavior)
	got := ParseArguments(`'hello\world'`)
	want := []string{`hello\world`}
	assertStrSliceEqual(t, want, got)
}

// --- ParseArgumentNames tests ---

func TestParseArgumentNames_ValidNames(t *testing.T) {
	got := ParseArgumentNames([]string{"foo", "bar", "baz"})
	want := []string{"foo", "bar", "baz"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArgumentNames_FiltersEmpty(t *testing.T) {
	got := ParseArgumentNames([]string{"foo", "", "bar"})
	want := []string{"foo", "bar"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArgumentNames_FiltersNumeric(t *testing.T) {
	got := ParseArgumentNames([]string{"foo", "0", "bar", "123"})
	want := []string{"foo", "bar"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArgumentNames_Nil(t *testing.T) {
	got := ParseArgumentNames(nil)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// --- SubstituteArguments tests ---

func TestSubstituteArguments_NilArgs(t *testing.T) {
	content := "Hello $ARGUMENTS"
	got := SubstituteArguments(content, nil, true, nil)
	if got != content {
		t.Errorf("expected unchanged content, got %q", got)
	}
}

func TestSubstituteArguments_EmptyArgs(t *testing.T) {
	// Empty string should replace $ARGUMENTS with empty
	empty := ""
	got := SubstituteArguments("Hello $ARGUMENTS!", &empty, true, nil)
	if got != "Hello !" {
		t.Errorf("expected 'Hello !', got %q", got)
	}
}

func TestSubstituteArguments_FullReplace(t *testing.T) {
	args := "world"
	got := SubstituteArguments("Hello $ARGUMENTS!", &args, true, nil)
	if got != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", got)
	}
}

func TestSubstituteArguments_IndexedArgs(t *testing.T) {
	args := "foo bar baz"
	got := SubstituteArguments("A=$ARGUMENTS[0] B=$ARGUMENTS[1] C=$ARGUMENTS[2]", &args, true, nil)
	if got != "A=foo B=bar C=baz" {
		t.Errorf("expected 'A=foo B=bar C=baz', got %q", got)
	}
}

func TestSubstituteArguments_ShorthandIndexed(t *testing.T) {
	args := "foo bar"
	got := SubstituteArguments("first=$0 second=$1.", &args, true, nil)
	if got != "first=foo second=bar." {
		t.Errorf("expected 'first=foo second=bar.', got %q", got)
	}
}

func TestSubstituteArguments_NamedArgs(t *testing.T) {
	args := "myfile.txt write"
	got := SubstituteArguments("File: $file Mode: $mode", &args, true, []string{"file", "mode"})
	if got != "File: myfile.txt Mode: write" {
		t.Errorf("expected 'File: myfile.txt Mode: write', got %q", got)
	}
}

func TestSubstituteArguments_NamedArgNotPartial(t *testing.T) {
	// $file should NOT match $filename
	args := "test.txt"
	got := SubstituteArguments("$filename and $file", &args, true, []string{"file"})
	if got != "$filename and test.txt" {
		t.Errorf("expected '$filename and test.txt', got %q", got)
	}
}

func TestSubstituteArguments_NamedArgNotIndexed(t *testing.T) {
	// $file should NOT match $file[0]
	args := "test.txt"
	got := SubstituteArguments("$file[0] and $file", &args, true, []string{"file"})
	if got != "$file[0] and test.txt" {
		t.Errorf("expected '$file[0] and test.txt', got %q", got)
	}
}

func TestSubstituteArguments_FallbackAppend(t *testing.T) {
	args := "some extra context"
	got := SubstituteArguments("Do something useful", &args, true, nil)
	expected := "Do something useful\n\nARGUMENTS: some extra context"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSubstituteArguments_NoFallbackWhenDisabled(t *testing.T) {
	args := "some extra context"
	got := SubstituteArguments("Do something useful", &args, false, nil)
	if got != "Do something useful" {
		t.Errorf("expected unchanged content, got %q", got)
	}
}

func TestSubstituteArguments_NoFallbackWhenEmptyArgs(t *testing.T) {
	// Empty args + no placeholder => no append (TS: args truthy check)
	args := ""
	got := SubstituteArguments("Do something useful", &args, true, nil)
	if got != "Do something useful" {
		t.Errorf("expected unchanged content, got %q", got)
	}
}

func TestSubstituteArguments_OutOfRangeIndex(t *testing.T) {
	args := "only"
	got := SubstituteArguments("A=$ARGUMENTS[0] B=$ARGUMENTS[1]", &args, false, nil)
	if got != "A=only B=" {
		t.Errorf("expected 'A=only B=', got %q", got)
	}
}

func TestSubstituteArguments_MixedPlaceholders(t *testing.T) {
	args := "alpha beta"
	content := "Named: $x, Indexed: $ARGUMENTS[1], Short: $0, Full: $ARGUMENTS"
	got := SubstituteArguments(content, &args, true, []string{"x"})
	expected := "Named: alpha, Indexed: beta, Short: alpha, Full: alpha beta"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSubstituteArguments_AdjacentShorthand(t *testing.T) {
	// $0$1 — both should be replaced ($ is not a word char)
	args := "aa bb"
	got := SubstituteArguments("$0$1", &args, false, nil)
	if got != "aabb" {
		t.Errorf("expected 'aabb', got %q", got)
	}
}

func TestSubstituteArguments_ShorthandNotFollowedByWord(t *testing.T) {
	// $0abc should NOT be replaced (followed by word char 'a')
	args := "xx"
	got := SubstituteArguments("$0abc", &args, false, nil)
	if got != "$0abc" {
		t.Errorf("expected '$0abc', got %q", got)
	}
}

// --- SubstituteVariables tests ---

func TestSubstituteVariables_SkillDir(t *testing.T) {
	got := SubstituteVariables("Run ${CLAUDE_SKILL_DIR}/script.sh", "/path/to/skill", "")
	if got != "Run /path/to/skill/script.sh" {
		t.Errorf("expected 'Run /path/to/skill/script.sh', got %q", got)
	}
}

func TestSubstituteVariables_SessionID(t *testing.T) {
	got := SubstituteVariables("Session: ${CLAUDE_SESSION_ID}", "", "abc-123")
	if got != "Session: abc-123" {
		t.Errorf("expected 'Session: abc-123', got %q", got)
	}
}

func TestSubstituteVariables_Both(t *testing.T) {
	got := SubstituteVariables(
		"Dir: ${CLAUDE_SKILL_DIR}, Session: ${CLAUDE_SESSION_ID}",
		"/my/skill", "sess-42",
	)
	if got != "Dir: /my/skill, Session: sess-42" {
		t.Errorf("unexpected result: %q", got)
	}
}

func TestSubstituteVariables_EmptySkillDir(t *testing.T) {
	content := "Dir: ${CLAUDE_SKILL_DIR}"
	got := SubstituteVariables(content, "", "sess-1")
	if got != content {
		t.Errorf("expected unchanged content when skillDir is empty, got %q", got)
	}
}

func TestSubstituteVariables_MultipleOccurrences(t *testing.T) {
	got := SubstituteVariables(
		"${CLAUDE_SKILL_DIR}/a and ${CLAUDE_SKILL_DIR}/b",
		"/sk", "",
	)
	if got != "/sk/a and /sk/b" {
		t.Errorf("unexpected: %q", got)
	}
}

// --- PrepareSkillContent tests ---

func TestPrepareSkillContent_FullPipeline(t *testing.T) {
	skill := &Skill{
		Content:  "Use $file at ${CLAUDE_SKILL_DIR}/scripts",
		SkillDir: "/home/user/.claude/skills/my-skill",
		ArgNames: []string{"file"},
	}
	args := "test.go"
	got := PrepareSkillContent(skill, &args, "sess-99")

	// Step 1: base dir header prepended
	if !strings.HasPrefix(got, "Base directory for this skill: /home/user/.claude/skills/my-skill") {
		t.Error("expected base dir header prefix")
	}
	// Step 2: $file replaced
	if !strings.Contains(got, "Use test.go at") {
		t.Error("expected $file to be replaced")
	}
	// Step 3: ${CLAUDE_SKILL_DIR} replaced
	if strings.Contains(got, "${CLAUDE_SKILL_DIR}") {
		t.Error("expected ${CLAUDE_SKILL_DIR} to be replaced")
	}
	if !strings.Contains(got, "/home/user/.claude/skills/my-skill/scripts") {
		t.Error("expected skill dir in scripts path")
	}
}

func TestPrepareSkillContent_NoArgs(t *testing.T) {
	skill := &Skill{
		Content: "Simple skill content",
	}
	got := PrepareSkillContent(skill, nil, "sess-1")
	if got != "Simple skill content" {
		t.Errorf("expected unchanged content, got %q", got)
	}
}

func TestPrepareSkillContent_NoBaseDir(t *testing.T) {
	skill := &Skill{
		Content: "No dir skill ${CLAUDE_SESSION_ID}",
	}
	got := PrepareSkillContent(skill, nil, "sess-42")
	if got != "No dir skill sess-42" {
		t.Errorf("expected session ID replaced, got %q", got)
	}
}

// --- GenerateProgressiveArgumentHint tests ---

func TestGenerateProgressiveArgumentHint(t *testing.T) {
	tests := []struct {
		name      string
		argNames  []string
		typedArgs []string
		want      string
	}{
		{"all remaining", []string{"file", "mode", "output"}, nil, "[file] [mode] [output]"},
		{"some remaining", []string{"file", "mode"}, []string{"x"}, "[mode]"},
		{"all filled", []string{"file"}, []string{"x"}, ""},
		{"overfilled", []string{"file"}, []string{"x", "y"}, ""},
		{"empty names", nil, nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateProgressiveArgumentHint(tt.argNames, tt.typedArgs)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

// --- HasShellCommands tests ---

func TestHasShellCommands_BlockPattern(t *testing.T) {
	content := "Before\n```!\necho hello\n```\nAfter"
	if !HasShellCommands(content) {
		t.Error("expected shell commands detected for block pattern")
	}
}

func TestHasShellCommands_InlinePattern(t *testing.T) {
	content := "Run this: !`echo hello` now"
	if !HasShellCommands(content) {
		t.Error("expected shell commands detected for inline pattern")
	}
}

func TestHasShellCommands_None(t *testing.T) {
	content := "Just a normal skill with no shell commands"
	if HasShellCommands(content) {
		t.Error("expected no shell commands detected")
	}
}

func TestHasShellCommands_FalsePositive(t *testing.T) {
	// Regular backticks without ! prefix shouldn't match
	content := "Use `echo hello` in your terminal"
	if HasShellCommands(content) {
		t.Error("expected no shell commands for regular backtick code")
	}
}

// --- Helper ---

func assertStrSliceEqual(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("length mismatch: want %v, got %v", want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("index %d: want %q, got %q", i, want[i], got[i])
		}
	}
}
