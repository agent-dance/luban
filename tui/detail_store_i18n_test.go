package tui

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func useDetailStoreTestLanguage(t *testing.T, lang i18n.Language) {
	t.Helper()
	previous := i18n.DetectOrLoadLanguage()
	t.Setenv("HOME", t.TempDir())
	if err := i18n.SaveLanguage(lang); err != nil {
		t.Fatalf("SaveLanguage(%s): %v", lang.Code(), err)
	}
	t.Cleanup(func() {
		if err := i18n.SaveLanguage(previous); err != nil {
			t.Errorf("restore language: %v", err)
		}
	})
}

func TestMemoryDetailStoreErrorsUseRuntimeLanguageAndPreserveSentinels(t *testing.T) {
	useDetailStoreTestLanguage(t, i18n.LangZH)
	store := NewMemoryDetailStore()

	missing := DetailRef{
		Source: memoryDetailSource,
		Key:    "raw-logical-key",
		Size:   0,
		Digest: strings.Repeat("0", 2*32),
	}
	_, err := store.Get(missing)
	if !errors.Is(err, ErrDetailNotFound) {
		t.Fatalf("missing error did not preserve ErrDetailNotFound: %v", err)
	}
	if got := err.Error(); strings.Contains(got, "detail not found") || !strings.Contains(got, missing.Key) {
		t.Fatalf("missing error was not localized or lost logical key: %q", got)
	}

	ref, err := store.Put("digest-key", []byte("AAAA"))
	if err != nil {
		t.Fatal(err)
	}
	store.details[ref.Digest] = []byte("BBBB")
	_, err = store.Get(ref)
	if !errors.Is(err, ErrInvalidDetailRef) {
		t.Fatalf("digest error did not preserve ErrInvalidDetailRef: %v", err)
	}
	actualDigest := digestBytes([]byte("BBBB"))
	if got := err.Error(); strings.Contains(got, "invalid detail reference") || !strings.Contains(got, ref.Key) || !strings.Contains(got, actualDigest) || !strings.Contains(got, ref.Digest) {
		t.Fatalf("digest error was not localized or lost raw parameters: %q", got)
	}
}

func TestFileDetailStoreJSONCauseAndEntryRemainInspectable(t *testing.T) {
	useDetailStoreTestLanguage(t, i18n.LangZH)
	root := t.TempDir()
	store, err := NewFileDetailStore(root)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".observations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	entry := "broken-entry.json"
	path := filepath.Join(dir, entry)
	if err := os.WriteFile(path, []byte(`{"id":`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = store.LoadObservationEvidence()
	var syntaxError *json.SyntaxError
	if !errors.As(err, &syntaxError) {
		t.Fatalf("journal decode error did not preserve JSON cause: %T %v", err, err)
	}
	if got := err.Error(); strings.Contains(got, "decode observation journal") || !strings.Contains(got, entry) || !strings.Contains(got, syntaxError.Error()) {
		t.Fatalf("journal decode error was not localized or lost raw parameters: %q", got)
	}
}

func TestFileDetailStoreInvalidReferenceKeepsRawSourceDigestAndPath(t *testing.T) {
	useDetailStoreTestLanguage(t, i18n.LangZH)
	root := t.TempDir()
	store, err := NewFileDetailStore(root)
	if err != nil {
		t.Fatal(err)
	}

	badDigest := "not-a-sha256-digest"
	_, err = store.Get(DetailRef{Source: "foreign-source", Key: "raw-key", Digest: badDigest})
	if !errors.Is(err, ErrInvalidDetailRef) {
		t.Fatalf("source error did not preserve ErrInvalidDetailRef: %v", err)
	}
	if got := err.Error(); strings.Contains(got, "invalid detail reference") || !strings.Contains(got, "foreign-source") || !strings.Contains(got, fileDetailSource) {
		t.Fatalf("source error was not localized or lost sources: %q", got)
	}

	digestBytes, err := hex.DecodeString(strings.Repeat("a", 2*32))
	if err != nil {
		t.Fatal(err)
	}
	ref := DetailRef{Source: fileDetailSource, Key: "raw-key", Size: 4, Digest: hex.EncodeToString(digestBytes)}
	path, err := store.pathForRef(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("AAAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = store.Get(ref)
	if !errors.Is(err, ErrInvalidDetailRef) {
		t.Fatalf("permission error did not preserve ErrInvalidDetailRef: %v", err)
	}
	if got := err.Error(); strings.Contains(got, "permissions are not private") || !strings.Contains(got, path) || !strings.Contains(got, "0644") {
		t.Fatalf("permission error was not localized or lost path/mode: %q", got)
	}
}

func TestFileDetailStoreOSCauseAndPathUseRuntimeLanguage(t *testing.T) {
	useDetailStoreTestLanguage(t, i18n.LangZH)
	path := filepath.Join(t.TempDir(), "artifact-file")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewFileDetailStore(path)
	var pathError *os.PathError
	if !errors.As(err, &pathError) {
		t.Fatalf("artifact-root error did not preserve OS cause: %T %v", err, err)
	}
	if got := err.Error(); strings.Contains(got, "prepare artifact root") || !strings.Contains(got, path) || !strings.Contains(got, pathError.Error()) {
		t.Fatalf("artifact-root error was not localized or lost raw path/cause: %q", got)
	}
}

func TestDetailStoreEnglishNotFoundContractRemainsCompatible(t *testing.T) {
	useDetailStoreTestLanguage(t, i18n.LangEN)
	store := NewMemoryDetailStore()
	ref := DetailRef{Source: memoryDetailSource, Key: "logical-key", Digest: strings.Repeat("0", 2*32)}
	_, err := store.Get(ref)
	if got, want := err.Error(), "detail not found: logical-key"; got != want {
		t.Fatalf("Get() error = %q, want %q", got, want)
	}
	if _, err := NewFileDetailStore(""); err == nil || err.Error() != "artifact root: invalid detail reference" {
		t.Fatalf("empty artifact-root error = %q", err)
	}
}
