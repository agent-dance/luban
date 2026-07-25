package skills

import (
	"strings"
	"testing"
)

// --- ParseArguments tests ---

func TestParseArguments_SimpleWords(t *testing.T) {
	got := parseArguments("foo bar baz")
	want := []string{"foo", "bar", "baz"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_DoubleQuoted(t *testing.T) {
	got := parseArguments(`foo "hello world" baz`)
	want := []string{"foo", "hello world", "baz"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_SingleQuoted(t *testing.T) {
	got := parseArguments("foo 'hello world' baz")
	want := []string{"foo", "hello world", "baz"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_Empty(t *testing.T) {
	got := parseArguments("")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestParseArguments_Whitespace(t *testing.T) {
	got := parseArguments("   ")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestParseArguments_MixedQuotes(t *testing.T) {
	got := parseArguments(`"hello" 'world' plain`)
	want := []string{"hello", "world", "plain"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_BackslashEscape(t *testing.T) {
	got := parseArguments(`hello\ world`)
	want := []string{"hello world"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_BackslashInDoubleQuotes(t *testing.T) {
	got := parseArguments(`"hello\"world"`)
	want := []string{`hello"world`}
	assertStrSliceEqual(t, want, got)
}

func TestParseArguments_BackslashInSingleQuotes(t *testing.T) {
	// Inside single quotes, backslash is literal (POSIX behavior)
	got := parseArguments(`'hello\world'`)
	want := []string{`hello\world`}
	assertStrSliceEqual(t, want, got)
}

// --- ParseArgumentNames tests ---

func TestParseArgumentNames_ValidNames(t *testing.T) {
	got := parseArgumentNames([]string{"foo", "bar", "baz"})
	want := []string{"foo", "bar", "baz"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArgumentNames_FiltersEmpty(t *testing.T) {
	got := parseArgumentNames([]string{"foo", "", "bar"})
	want := []string{"foo", "bar"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArgumentNames_FiltersNumeric(t *testing.T) {
	got := parseArgumentNames([]string{"foo", "0", "bar", "123"})
	want := []string{"foo", "bar"}
	assertStrSliceEqual(t, want, got)
}

func TestParseArgumentNames_Nil(t *testing.T) {
	got := parseArgumentNames(nil)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// --- SubstituteArguments tests ---

func TestSubstituteArguments_NilArgs(t *testing.T) {
	content := "Hello $ARGUMENTS"
	got := substituteArguments(content, nil, nil)
	if got != content {
		t.Errorf("expected unchanged content, got %q", got)
	}
}

func TestSubstituteArguments_EmptyArgs(t *testing.T) {
	// Empty string should replace $ARGUMENTS with empty
	empty := ""
	got := substituteArguments("Hello $ARGUMENTS!", &empty, nil)
	if got != "Hello !" {
		t.Errorf("expected 'Hello !', got %q", got)
	}
}

func TestSubstituteArguments_FullReplace(t *testing.T) {
	args := "world"
	got := substituteArguments("Hello $ARGUMENTS!", &args, nil)
	if got != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", got)
	}
}

func TestSubstituteArguments_IndexedArgs(t *testing.T) {
	args := "foo bar baz"
	got := substituteArguments("A=$ARGUMENTS[0] B=$ARGUMENTS[1] C=$ARGUMENTS[2]", &args, nil)
	if got != "A=foo B=bar C=baz" {
		t.Errorf("expected 'A=foo B=bar C=baz', got %q", got)
	}
}

func TestSubstituteArguments_ShorthandIndexed(t *testing.T) {
	args := "foo bar"
	got := substituteArguments("first=$0 second=$1.", &args, nil)
	if got != "first=foo second=bar." {
		t.Errorf("expected 'first=foo second=bar.', got %q", got)
	}
}

func TestSubstituteArguments_NamedArgs(t *testing.T) {
	args := "myfile.txt write"
	got := substituteArguments("File: $file Mode: $mode", &args, []string{"file", "mode"})
	if got != "File: myfile.txt Mode: write" {
		t.Errorf("expected 'File: myfile.txt Mode: write', got %q", got)
	}
}

func TestSubstituteArguments_NamedArgNotPartial(t *testing.T) {
	// $file should NOT match $filename
	args := "test.txt"
	got := substituteArguments("$filename and $file", &args, []string{"file"})
	if got != "$filename and test.txt" {
		t.Errorf("expected '$filename and test.txt', got %q", got)
	}
}

func TestSubstituteArguments_NamedArgNotIndexed(t *testing.T) {
	// $file should NOT match $file[0]
	args := "test.txt"
	got := substituteArguments("$file[0] and $file", &args, []string{"file"})
	if got != "$file[0] and test.txt" {
		t.Errorf("expected '$file[0] and test.txt', got %q", got)
	}
}

func TestSubstituteArguments_FallbackAppend(t *testing.T) {
	args := "some extra context"
	got := substituteArguments("Do something useful", &args, nil)
	expected := "Do something useful\n\nARGUMENTS: some extra context"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSubstituteArguments_NoFallbackWhenEmptyArgs(t *testing.T) {
	// Empty args + no placeholder => no append (TS: args truthy check)
	args := ""
	got := substituteArguments("Do something useful", &args, nil)
	if got != "Do something useful" {
		t.Errorf("expected unchanged content, got %q", got)
	}
}

func TestSubstituteArguments_OutOfRangeIndex(t *testing.T) {
	args := "only"
	got := substituteArguments("A=$ARGUMENTS[0] B=$ARGUMENTS[1]", &args, nil)
	if got != "A=only B=" {
		t.Errorf("expected 'A=only B=', got %q", got)
	}
}

func TestSubstituteArguments_MixedPlaceholders(t *testing.T) {
	args := "alpha beta"
	content := "Named: $x, Indexed: $ARGUMENTS[1], Short: $0, Full: $ARGUMENTS"
	got := substituteArguments(content, &args, []string{"x"})
	expected := "Named: alpha, Indexed: beta, Short: alpha, Full: alpha beta"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSubstituteArguments_AdjacentShorthand(t *testing.T) {
	// $0$1 — both should be replaced ($ is not a word char)
	args := "aa bb"
	got := substituteArguments("$0$1", &args, nil)
	if got != "aabb" {
		t.Errorf("expected 'aabb', got %q", got)
	}
}

func TestSubstituteArguments_UnmatchedShorthandStillAppendsArguments(t *testing.T) {
	// $0abc should NOT be replaced (followed by word char 'a')
	args := "xx"
	got := substituteArguments("$0abc", &args, nil)
	if got != "$0abc\n\nARGUMENTS: xx" {
		t.Errorf("unexpected substitution result %q", got)
	}
}

// --- SubstituteVariables tests ---

func TestSubstituteVariables_SkillDir(t *testing.T) {
	got := substituteVariables("Run ${LUBAN_SKILL_DIR}/script.sh", "/path/to/skill", "")
	if got != "Run /path/to/skill/script.sh" {
		t.Errorf("expected 'Run /path/to/skill/script.sh', got %q", got)
	}
}

func TestSubstituteVariables_SessionID(t *testing.T) {
	got := substituteVariables("Session: ${LUBAN_SESSION_ID}", "", "abc-123")
	if got != "Session: abc-123" {
		t.Errorf("expected 'Session: abc-123', got %q", got)
	}
}

func TestSubstituteVariables_Both(t *testing.T) {
	got := substituteVariables(
		"Dir: ${LUBAN_SKILL_DIR}, Session: ${LUBAN_SESSION_ID}",
		"/my/skill", "sess-42",
	)
	if got != "Dir: /my/skill, Session: sess-42" {
		t.Errorf("unexpected result: %q", got)
	}
}

func TestSubstituteVariables_EmptySkillDir(t *testing.T) {
	content := "Dir: ${LUBAN_SKILL_DIR}"
	got := substituteVariables(content, "", "sess-1")
	if got != content {
		t.Errorf("expected unchanged content when skillDir is empty, got %q", got)
	}
}

func TestSubstituteVariables_MultipleOccurrences(t *testing.T) {
	got := substituteVariables(
		"${LUBAN_SKILL_DIR}/a and ${LUBAN_SKILL_DIR}/b",
		"/sk", "",
	)
	if got != "/sk/a and /sk/b" {
		t.Errorf("unexpected: %q", got)
	}
}

// --- PrepareSkillContent tests ---

func TestPrepareSkillContent_FullPipeline(t *testing.T) {
	skill := &Skill{
		Content:  "Use $file at ${LUBAN_SKILL_DIR}/scripts",
		SkillDir: "/workspace/.luban-code/skills/my-skill",
		ArgNames: []string{"file"},
	}
	args := "test.go"
	got := PrepareSkillContent(skill, &args, "sess-99")

	// Step 1: base dir header prepended
	if !strings.HasPrefix(got, "Base directory for this skill: /workspace/.luban-code/skills/my-skill") {
		t.Error("expected base dir header prefix")
	}
	// Step 2: $file replaced
	if !strings.Contains(got, "Use test.go at") {
		t.Error("expected $file to be replaced")
	}
	// Step 3: ${LUBAN_SKILL_DIR} replaced
	if strings.Contains(got, "${LUBAN_SKILL_DIR}") {
		t.Error("expected ${LUBAN_SKILL_DIR} to be replaced")
	}
	if !strings.Contains(got, "/workspace/.luban-code/skills/my-skill/scripts") {
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
		Content: "No dir skill ${LUBAN_SESSION_ID}",
	}
	got := PrepareSkillContent(skill, nil, "sess-42")
	if got != "No dir skill sess-42" {
		t.Errorf("expected session ID replaced, got %q", got)
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
