package i18n

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestTUIDetailStoreCatalogCoversEveryLanguageAndFormatsParameters(t *testing.T) {
	cause := errors.New("raw-os-json-cause")
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyTUIDetailStoreNotFound, nil, "detail not found"},
		{KeyTUIDetailStoreInvalidReference, nil, "invalid detail reference"},
		{KeyTUIDetailStoreNotFoundKey, []any{"logical-key"}, "detail not found: logical-key"},
		{KeyTUIDetailStoreRetainedIntegrity, []any{"logical-key", 3, 4}, `invalid detail reference: retained detail "logical-key" failed integrity check (size 3, expected 4)`},
		{KeyTUIDetailStoreRetainedDigest, []any{"logical-key", "actual-digest", "expected-digest"}, `invalid detail reference: retained detail "logical-key" digest "actual-digest" differs from reference "expected-digest"`},
		{KeyTUIDetailStoreArtifactRootEmpty, nil, "artifact root: invalid detail reference"},
		{KeyTUIDetailStoreResolveArtifactRoot, []any{"/artifact-root", cause}, `resolve artifact root "/artifact-root": raw-os-json-cause`},
		{KeyTUIDetailStorePrepareArtifactRoot, []any{"/artifact-root", cause}, `prepare artifact root "/artifact-root": raw-os-json-cause`},
		{KeyTUIDetailStorePrepareShard, []any{"/artifact-root/shard", cause}, `prepare detail shard "/artifact-root/shard": raw-os-json-cause`},
		{KeyTUIDetailStoreCreateTemporary, []any{"/artifact-root/shard", cause}, `create detail temporary file in "/artifact-root/shard": raw-os-json-cause`},
		{KeyTUIDetailStoreSecureTemporary, []any{"/artifact-root/.detail-temp", cause}, `secure detail temporary file "/artifact-root/.detail-temp": raw-os-json-cause`},
		{KeyTUIDetailStoreWrite, []any{"/artifact-root/.detail-temp", cause}, `write detail "/artifact-root/.detail-temp": raw-os-json-cause`},
		{KeyTUIDetailStoreSync, []any{"/artifact-root/.detail-temp", cause}, `sync detail "/artifact-root/.detail-temp": raw-os-json-cause`},
		{KeyTUIDetailStoreClose, []any{"/artifact-root/.detail-temp", cause}, `close detail "/artifact-root/.detail-temp": raw-os-json-cause`},
		{KeyTUIDetailStorePublish, []any{"/artifact-root/.detail-temp", "/artifact-root/value.detail", cause}, `publish detail "/artifact-root/.detail-temp" as "/artifact-root/value.detail": raw-os-json-cause`},
		{KeyTUIDetailStoreSyncDirectory, []any{"/artifact-root/shard", cause}, `sync detail directory "/artifact-root/shard": raw-os-json-cause`},
		{KeyTUIDetailStoreJournalInvalid, nil, "journal observation: invalid detail reference"},
		{KeyTUIDetailStoreJournalReference, []any{"observation-7", cause}, "journal observation observation-7: raw-os-json-cause"},
		{KeyTUIDetailStoreEncodeJournal, []any{"observation-7", cause}, "encode observation journal for observation-7: raw-os-json-cause"},
		{KeyTUIDetailStorePrepareJournal, []any{"/artifact-root/.observations", cause}, `prepare observation journal "/artifact-root/.observations": raw-os-json-cause`},
		{KeyTUIDetailStorePublishJournal, []any{"/artifact-root/.observations/entry.json", cause}, `publish observation journal "/artifact-root/.observations/entry.json": raw-os-json-cause`},
		{KeyTUIDetailStoreSyncJournal, []any{"/artifact-root/.observations", cause}, `sync observation journal "/artifact-root/.observations": raw-os-json-cause`},
		{KeyTUIDetailStoreReadJournal, []any{"/artifact-root/.observations", cause}, `read observation journal "/artifact-root/.observations": raw-os-json-cause`},
		{KeyTUIDetailStoreInspectJournalEntry, []any{"entry.json", "/artifact-root/.observations/entry.json", cause}, `inspect observation journal entry entry.json at "/artifact-root/.observations/entry.json": raw-os-json-cause`},
		{KeyTUIDetailStoreJournalEntryInvalid, []any{"entry.json", "/artifact-root/.observations/entry.json"}, `observation journal entry entry.json at "/artifact-root/.observations/entry.json": invalid detail reference`},
		{KeyTUIDetailStoreReadJournalEntry, []any{"entry.json", "/artifact-root/.observations/entry.json", cause}, `read observation journal entry entry.json at "/artifact-root/.observations/entry.json": raw-os-json-cause`},
		{KeyTUIDetailStoreDecodeJournalEntry, []any{"entry.json", cause}, "decode observation journal entry entry.json: raw-os-json-cause"},
		{KeyTUIDetailStoreValidateJournal, []any{"observation-7", cause}, "validate observation journal observation-7: raw-os-json-cause"},
		{KeyTUIDetailStoreInspectDetail, []any{"/artifact-root/value.detail", cause}, `inspect detail "/artifact-root/value.detail": raw-os-json-cause`},
		{KeyTUIDetailStoreDetailNotRegular, []any{"/artifact-root/value.detail"}, `invalid detail reference: detail "/artifact-root/value.detail" is not a regular file`},
		{KeyTUIDetailStoreDetailPermissions, []any{"/artifact-root/value.detail", "0644"}, `invalid detail reference: detail "/artifact-root/value.detail" permissions 0644 are not private`},
		{KeyTUIDetailStoreOpenDetail, []any{"/artifact-root/value.detail", cause}, `open detail "/artifact-root/value.detail": raw-os-json-cause`},
		{KeyTUIDetailStoreStatDetail, []any{"/artifact-root/value.detail", cause}, `stat detail "/artifact-root/value.detail": raw-os-json-cause`},
		{KeyTUIDetailStoreDetailChanged, []any{"/artifact-root/value.detail"}, `invalid detail reference: detail "/artifact-root/value.detail" changed while opening`},
		{KeyTUIDetailStoreDetailSize, []any{"/artifact-root/value.detail", int64(3), 4}, `invalid detail reference: detail "/artifact-root/value.detail" size 3 differs from reference 4`},
		{KeyTUIDetailStoreReadDetail, []any{"/artifact-root/value.detail", cause}, `read detail "/artifact-root/value.detail": raw-os-json-cause`},
		{KeyTUIDetailStoreDetailIntegrity, []any{"/artifact-root/value.detail", 3, 4}, `invalid detail reference: detail "/artifact-root/value.detail" failed integrity check (size 3, expected 4)`},
		{KeyTUIDetailStoreDetailDigest, []any{"/artifact-root/value.detail", "actual-digest", "expected-digest"}, `invalid detail reference: detail "/artifact-root/value.detail" digest "actual-digest" differs from reference "expected-digest"`},
		{KeyTUIDetailStoreRelativizePath, []any{"/artifact-root/value.detail", "/artifact-root", cause}, `validate detail path "/artifact-root/value.detail" against artifact root "/artifact-root": raw-os-json-cause`},
		{KeyTUIDetailStorePathEscapesRoot, []any{"/elsewhere/value.detail", "/artifact-root"}, `invalid detail reference: detail path "/elsewhere/value.detail" escapes artifact root "/artifact-root"`},
		{KeyTUIDetailStoreLogicalKeyInvalid, []any{""}, `logical key "": invalid detail reference`},
		{KeyTUIDetailStoreSourceMismatch, []any{"memory", "file"}, `invalid detail reference: source "memory" does not match store "file"`},
		{KeyTUIDetailStoreReferenceMalformed, []any{"logical-key", -1}, `invalid detail reference: malformed key "logical-key" or size -1`},
		{KeyTUIDetailStoreDigestMalformed, []any{"bad-digest"}, `invalid detail reference: malformed digest "bad-digest"`},
		{KeyTUIDetailStorePathNotRealDirectory, []any{"/artifact-root"}, `path "/artifact-root" is not a real directory`},
	}

	if len(tests) != len(tuiDetailStoreKeys) {
		t.Fatalf("detail-store cases = %d, keys = %d", len(tests), len(tuiDetailStoreKeys))
	}
	for _, test := range tests {
		if got := Format(LangEN, test.key, test.args...); got != test.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", test.key, got, test.want)
		}
		for _, lang := range AllLanguages() {
			got := Format(lang, test.key, test.args...)
			if got == "" || strings.HasPrefix(got, "[") || strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) = %q", lang.Code(), test.key, got)
			}
			for _, arg := range test.args {
				raw := fmt.Sprint(arg)
				if raw != "" && !strings.Contains(got, raw) {
					t.Errorf("Format(%s, %q) omitted raw parameter %q: %q", lang.Code(), test.key, raw, got)
				}
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

type tuiDetailStoreTypedCause struct{ marker string }

func (e *tuiDetailStoreTypedCause) Error() string { return "raw-detail-cause-" + e.marker }

func TestTUIDetailStoreErrorsDeferLanguageAndPreserveCauses(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	invalidReference := NewError(KeyTUIDetailStoreInvalidReference)
	internal := WrapInternalError(KeyTUIDetailStoreLogicalKeyInvalid, invalidReference, "raw-logical-key")
	if !errors.Is(internal, invalidReference) {
		t.Fatal("internal reference error did not preserve errors.Is")
	}

	cause := &tuiDetailStoreTypedCause{marker: "42"}
	external := WrapError(KeyTUIDetailStoreResolveArtifactRoot, cause, "/raw/artifact-root")
	var typed *tuiDetailStoreTypedCause
	if !errors.As(external, &typed) || typed.marker != "42" {
		t.Fatalf("external error did not preserve errors.As: %#v", typed)
	}

	detectedLanguageCache.Store(int32(LangEN))
	englishInternal := internal.Error()
	englishExternal := external.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chineseInternal := internal.Error()
	chineseExternal := external.Error()
	if englishInternal != `logical key "raw-logical-key": invalid detail reference` {
		t.Fatalf("English internal error = %q", englishInternal)
	}
	if englishExternal != `resolve artifact root "/raw/artifact-root": raw-detail-cause-42` {
		t.Fatalf("English external error = %q", englishExternal)
	}
	if chineseInternal == englishInternal || strings.Contains(chineseInternal, "invalid detail reference") || !strings.Contains(chineseInternal, "raw-logical-key") {
		t.Fatalf("internal runtime localization failed: en=%q zh=%q", englishInternal, chineseInternal)
	}
	if chineseExternal == englishExternal || !strings.Contains(chineseExternal, "/raw/artifact-root") || !strings.Contains(chineseExternal, cause.Error()) {
		t.Fatalf("external runtime localization failed: en=%q zh=%q", englishExternal, chineseExternal)
	}
}
