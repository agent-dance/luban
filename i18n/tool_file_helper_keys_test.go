package i18n

import (
	"errors"
	"strings"
	"testing"
)

type toolFileHelperTestCause struct{}

func (*toolFileHelperTestCause) Error() string { return "raw-cause-42" }

func TestToolFileHelperKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolFileHelperKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolFileHelperEnglishContract(t *testing.T) {
	cases := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolFileHelperStatFailed, []any{errors.New("raw-cause")}, "failed to stat file: raw-cause"},
		{KeyToolFileHelperResolveSymlinkFailed, []any{"/tmp/link", errors.New("raw-cause")}, `failed to resolve symlink "/tmp/link": raw-cause`},
		{KeyToolFileHelperPathIsDirectory, []any{"/tmp/path"}, `path "/tmp/path" is a directory, not a file`},
		{KeyToolFileHelperReadFailed, []any{errors.New("raw-cause")}, "failed to read file: raw-cause"},
		{KeyToolFileHelperWriteTargetOutsideAllowed, []any{errors.New("raw-cause")}, "write target changed outside allowed directories: raw-cause"},
		{KeyToolFileHelperCreatedAfterCheck, nil, "file was created after it was checked; read it before writing"},
		{KeyToolFileHelperRecheckWriteTargetFailed, []any{errors.New("raw-cause")}, "failed to recheck write target: raw-cause"},
		{KeyToolFileHelperSymlinkTargetChanged, nil, "symlink target changed after it was read; read it again before writing"},
		{KeyToolFileHelperWriteTargetReplaced, nil, "file was replaced after it was read; read it again before writing"},
		{KeyToolFileHelperEditTargetReplacedBefore, nil, "file was replaced before it could be read; read it again before editing"},
		{KeyToolFileHelperEditTargetChangedWhileRead, nil, "file changed while it was being read; read it again before editing"},
		{KeyToolFileHelperEditTargetOutsideAllowed, []any{errors.New("raw-cause")}, "edit target changed outside allowed directories: raw-cause"},
		{KeyToolFileHelperRecheckEditTargetFailed, []any{errors.New("raw-cause")}, "failed to recheck edit target: raw-cause"},
		{KeyToolFileHelperEditThroughSymlink, []any{"/tmp/link"}, `refusing to edit through symlink "/tmp/link"`},
		{KeyToolFileHelperEditTargetReplacedAfter, nil, "file was replaced after it was read; read it again before editing"},
		{KeyToolFileHelperEditTargetChangedAfter, nil, "file changed after it was read; read it again before editing"},
		{KeyToolFileHelperPathOutsideAllowed, []any{"/tmp/raw"}, "path is outside allowed directories (resolved to /tmp/raw)"},
		{KeyToolFileHelperVerifyFDPathFailed, []any{errors.New("raw-cause")}, "cannot verify fd path: raw-cause"},
	}

	for _, tc := range cases {
		if got := Format(LangEN, tc.key, tc.args...); got != tc.want {
			t.Errorf("Format(LangEN, %s) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestToolFileHelperErrorsUseActiveLanguageAndPreserveCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := &toolFileHelperTestCause{}
	err := WrapError(KeyToolFileHelperResolveSymlinkFailed, cause, "/tmp/link")
	if !errors.Is(err, cause) {
		t.Fatal("file helper error did not preserve its underlying cause")
	}
	var typedCause *toolFileHelperTestCause
	if !errors.As(err, &typedCause) || typedCause != cause {
		t.Fatal("file helper error did not preserve its typed underlying cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	if english != `failed to resolve symlink "/tmp/link": raw-cause-42` {
		t.Fatalf("English compatibility changed: %q", english)
	}
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese || !strings.Contains(chinese, "raw-cause-42") {
		t.Fatalf("runtime localization failed: en=%q zh=%q", english, chinese)
	}
}
