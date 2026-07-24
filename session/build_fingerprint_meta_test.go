package session

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/types"
)

func TestSessionMetaTracksFirstAndLastWriterBuild(t *testing.T) {
	store := NewFileStore(t.TempDir())
	first := sessionTestFingerprint("v1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false, 1)
	second := sessionTestFingerprint("v2", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", true, 2)
	current := first
	store.writerFingerprint = func() buildinfo.Fingerprint { return current }

	if err := store.Save("build-session", []types.Message{types.UserMessage("hello")}); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	current = second
	if err := store.SaveMeta("build-session", SessionMeta{Title: "renamed"}); err != nil {
		t.Fatalf("second writer save: %v", err)
	}

	meta, err := store.GetMeta("build-session")
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	if meta.FirstWriterBuild == nil || !reflect.DeepEqual(*meta.FirstWriterBuild, first) {
		t.Fatalf("first writer = %#v, want %#v", meta.FirstWriterBuild, first)
	}
	if meta.LastWriterBuild == nil || !reflect.DeepEqual(*meta.LastWriterBuild, second) {
		t.Fatalf("last writer = %#v, want %#v", meta.LastWriterBuild, second)
	}

	// A normal transcript save is also a writer event and must retain the first
	// writer while stamping the latest process identity.
	current = first
	if err := store.Save("build-session", []types.Message{types.UserMessage("next")}); err != nil {
		t.Fatalf("transcript rewrite: %v", err)
	}
	meta, err = store.GetMeta("build-session")
	if err != nil {
		t.Fatalf("get rewritten metadata: %v", err)
	}
	if meta.FirstWriterBuild == nil || !reflect.DeepEqual(*meta.FirstWriterBuild, first) ||
		meta.LastWriterBuild == nil || !reflect.DeepEqual(*meta.LastWriterBuild, first) {
		t.Fatalf("writer history after transcript rewrite = first %#v last %#v", meta.FirstWriterBuild, meta.LastWriterBuild)
	}
}

func TestSessionBuildFingerprintJSONAndLegacyMetadata(t *testing.T) {
	fingerprint := sessionTestFingerprint("v1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false, 1)
	encoded, err := json.Marshal(SessionMeta{ID: "roundtrip", FirstWriterBuild: &fingerprint, LastWriterBuild: &fingerprint})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	var decoded SessionMeta
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if decoded.FirstWriterBuild == nil || decoded.LastWriterBuild == nil ||
		!reflect.DeepEqual(*decoded.FirstWriterBuild, fingerprint) || !reflect.DeepEqual(*decoded.LastWriterBuild, fingerprint) {
		t.Fatalf("fingerprint round trip = first %#v last %#v", decoded.FirstWriterBuild, decoded.LastWriterBuild)
	}
	for _, field := range []string{`"revision"`, `"dirty"`, `"process_start"`, `"executable"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("session writer fingerprint omitted %s: %s", field, encoded)
		}
	}

	var legacy SessionMeta
	if err := json.Unmarshal([]byte(`{"id":"legacy","title":"old"}`), &legacy); err != nil {
		t.Fatalf("unmarshal legacy metadata: %v", err)
	}
	if legacy.FirstWriterBuild != nil || legacy.LastWriterBuild != nil {
		t.Fatalf("legacy metadata fabricated writer builds: %#v", legacy)
	}
}

func TestLegacySessionSidecarGetsFirstAndLastKnownWriterOnNextSave(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	fingerprint := sessionTestFingerprint("v2", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", false, 2)
	store.writerFingerprint = func() buildinfo.Fingerprint { return fingerprint }
	if err := writePrivateFileAtomic(dir, store.sessionPath("legacy"), ".legacy-transcript-*", nil); err != nil {
		t.Fatalf("write legacy transcript: %v", err)
	}
	legacy := []byte(`{"id":"legacy","title":"old"}`)
	if err := writePrivateFileAtomic(dir, store.metaPath("legacy"), ".legacy-meta-*", legacy); err != nil {
		t.Fatalf("write legacy sidecar: %v", err)
	}

	if err := store.SaveMeta("legacy", SessionMeta{Title: "updated"}); err != nil {
		t.Fatalf("upgrade legacy sidecar: %v", err)
	}
	meta, err := store.GetMeta("legacy")
	if err != nil {
		t.Fatalf("load upgraded sidecar: %v", err)
	}
	if meta.FirstWriterBuild == nil || meta.LastWriterBuild == nil ||
		!reflect.DeepEqual(*meta.FirstWriterBuild, fingerprint) || !reflect.DeepEqual(*meta.LastWriterBuild, fingerprint) {
		t.Fatalf("upgraded writer fingerprints = first %#v last %#v", meta.FirstWriterBuild, meta.LastWriterBuild)
	}
}

func sessionTestFingerprint(version, revision string, dirty bool, hour int) buildinfo.Fingerprint {
	buildTime := time.Date(2026, 7, 17, hour, 0, 0, 0, time.UTC)
	started := time.Date(2026, 7, 18, hour, 0, 0, 0, time.UTC)
	return buildinfo.Fingerprint{
		Version: version, Revision: revision, Dirty: &dirty, BuildTime: &buildTime,
		ProcessStart: started, Executable: "/opt/luban-code",
	}
}
