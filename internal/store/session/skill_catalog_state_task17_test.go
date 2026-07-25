package session

import (
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

const (
	sessionSkillID      skills.SkillID = "skill:project:/repo/.agents/skills/review"
	sessionOtherSkillID skills.SkillID = "skill:user:/home/me/.agents/skills/explain"

	sessionEntryDigestA SessionCatalogEntryDigest      = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sessionEntryDigestB SessionCatalogEntryDigest      = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	sessionContentA     skills.SkillDigest             = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	sessionContentB     skills.SkillDigest             = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	sessionPayloadA     skills.InvocationPayloadDigest = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	sessionPayloadB     skills.InvocationPayloadDigest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
)

func TestSessionSkillsRoundTripResumeAndPartialMetaMerge(t *testing.T) {
	directory := t.TempDir()
	store := NewFileStore(directory)
	const sessionID = "skills-round-trip"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("first")}); err != nil {
		t.Fatal(err)
	}
	want := sessionSkillsMetaFixture(7)
	if err := store.SaveMeta(sessionID, SessionMeta{Skills: want}); err != nil {
		t.Fatal(err)
	}

	// Mutating the caller-owned value after SaveMeta must not affect disk state.
	want.Overrides[sessionSkillID] = skills.VisibilityOverride{}
	want.AnnouncedEntries[sessionSkillID] = sessionEntryDigestB
	want.LoadedDigests[sessionSkillID] = SessionLoadedSkillDigest{}

	fresh := NewFileStore(directory)
	resumed, err := fresh.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	assertSessionSkillsFixture(t, resumed.Skills, 7)

	if err := fresh.SaveMeta(sessionID, SessionMeta{Title: "kept skills"}); err != nil {
		t.Fatal(err)
	}
	partial, err := fresh.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Title != "kept skills" {
		t.Fatalf("title = %q", partial.Title)
	}
	assertSessionSkillsFixture(t, partial.Skills, 7)

	// Transcript saves derive ordinary metadata but preserve the sidecar state.
	if err := fresh.Save(sessionID, []types.Message{
		types.UserMessage("first"),
		types.AssistantMessage("second"),
	}); err != nil {
		t.Fatal(err)
	}
	afterTranscriptSave, err := fresh.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	assertSessionSkillsFixture(t, afterTranscriptSave.Skills, 7)

	// GetMeta must not expose map or LastNonOff pointer storage across reads.
	afterTranscriptSave.Skills.AnnouncedEntries[sessionSkillID] = sessionEntryDigestB
	*afterTranscriptSave.Skills.Overrides[sessionSkillID].LastNonOff = skills.VisibilityAuto
	again, err := fresh.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	assertSessionSkillsFixture(t, again.Skills, 7)
}

func TestSessionSkillsReconcileVisibleStateAndEpochMismatch(t *testing.T) {
	persisted := sessionSkillsMetaFixture(11)
	exactVisible := SessionSkillsVisibleState{
		ContextEpoch:      11,
		AnnouncedRevision: persisted.AnnouncedRevision,
		AnnouncedEntries:  persisted.AnnouncedEntries,
		LoadedDigests:     persisted.LoadedDigests,
	}
	reconciled, err := ReconcileSessionSkillsMeta(persisted, exactVisible)
	if err != nil {
		t.Fatal(err)
	}
	assertSessionSkillsFixture(t, reconciled, 11)
	if revision, entries, ok := reconciled.CatalogCursor(11); !ok || revision != 19 || entries[sessionSkillID] != sessionEntryDigestA {
		t.Fatalf("catalog cursor = %d %#v %v", revision, entries, ok)
	}
	if _, _, ok := reconciled.CatalogCursor(12); ok {
		t.Fatal("different epoch reused announced cursor")
	}
	if loaded, ok := reconciled.LoadedDigest(11, sessionSkillID); !ok || loaded.ContentDigest != sessionContentA {
		t.Fatalf("loaded digest = %#v %v", loaded, ok)
	}
	if _, ok := reconciled.LoadedDigest(12, sessionSkillID); ok {
		t.Fatal("different epoch reused loaded body")
	}

	// Missing catalog evidence invalidates the entire cursor. Loaded evidence is
	// retained entry-by-entry only when both digests match.
	missingCatalog, err := ReconcileSessionSkillsMeta(persisted, SessionSkillsVisibleState{
		ContextEpoch:  11,
		LoadedDigests: map[skills.SkillID]SessionLoadedSkillDigest{sessionSkillID: persisted.LoadedDigests[sessionSkillID]},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := missingCatalog.CatalogCursor(11); ok || missingCatalog.AnnouncedRevision != 0 || len(missingCatalog.AnnouncedEntries) != 0 {
		t.Fatalf("missing catalog evidence retained cursor: %#v", missingCatalog)
	}
	if _, ok := missingCatalog.LoadedDigest(11, sessionSkillID); !ok {
		t.Fatal("exact visible loaded body was discarded")
	}
	if _, ok := missingCatalog.LoadedDigest(11, sessionOtherSkillID); ok {
		t.Fatal("absent loaded body remained reusable")
	}

	wrongPayload, err := ReconcileSessionSkillsMeta(persisted, SessionSkillsVisibleState{
		ContextEpoch: 11,
		LoadedDigests: map[skills.SkillID]SessionLoadedSkillDigest{
			sessionSkillID: {ContentDigest: sessionContentA, PayloadDigest: sessionPayloadB},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := wrongPayload.LoadedDigest(11, sessionSkillID); ok {
		t.Fatal("different rendered payload reused loaded body")
	}

	newEpoch, err := ReconcileSessionSkillsMeta(persisted, SessionSkillsVisibleState{
		ContextEpoch:      12,
		AnnouncedRevision: persisted.AnnouncedRevision,
		AnnouncedEntries:  persisted.AnnouncedEntries,
		LoadedDigests:     persisted.LoadedDigests,
	})
	if err != nil {
		t.Fatal(err)
	}
	if newEpoch.ContextEpoch != 12 || newEpoch.AnnouncedRevision != 0 || len(newEpoch.AnnouncedEntries) != 0 || len(newEpoch.LoadedDigests) != 0 {
		t.Fatalf("epoch mismatch retained visible ledger: %#v", newEpoch)
	}
	if got := newEpoch.Overrides[sessionSkillID]; got.Visibility != skills.VisibilityOff || got.RestoreVisibility() != skills.VisibilityManualOnly {
		t.Fatalf("epoch mismatch lost session override: %#v", got)
	}
}

func TestSessionSkillsMetaRejectsInvalidStateWithoutOverwrite(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "invalid-skills-state"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("seed")}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMeta(sessionID, SessionMeta{Skills: sessionSkillsMetaFixture(3)}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.metaPath(sessionID))
	if err != nil {
		t.Fatal(err)
	}

	invalid := sessionSkillsMetaFixture(3)
	invalid.Overrides[sessionSkillID] = skills.VisibilityOverride{
		SkillID: sessionSkillID, Scope: skills.SkillScopeProject, Visibility: skills.VisibilityOff,
	}
	if err := store.SaveMeta(sessionID, SessionMeta{Skills: invalid}); err == nil {
		t.Fatal("non-session override unexpectedly persisted")
	}
	after, err := os.ReadFile(store.metaPath(sessionID))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("rejected skills metadata changed sidecar")
	}

	invalidDigest := sessionSkillsMetaFixture(3)
	invalidDigest.AnnouncedEntries[sessionSkillID] = "not-a-digest"
	if err := invalidDigest.Validate(); err == nil {
		t.Fatal("invalid announced digest unexpectedly valid")
	}
	noEpoch := sessionSkillsMetaFixture(0)
	if err := noEpoch.Validate(); err == nil {
		t.Fatal("visible ledgers without context epoch unexpectedly valid")
	}
	if _, err := ReconcileSessionSkillsMeta(nil, SessionSkillsVisibleState{}); err == nil {
		t.Fatal("zero visible context epoch unexpectedly reconciled")
	}
}

func TestSessionSkillsOverridesRemainIsolatedAcrossResume(t *testing.T) {
	directory := t.TempDir()
	store := NewFileStore(directory)
	for _, sessionID := range []string{"session-a", "session-b"} {
		if err := store.Save(sessionID, []types.Message{types.UserMessage(sessionID)}); err != nil {
			t.Fatal(err)
		}
	}
	a := sessionSkillsMetaFixture(21)
	b := sessionSkillsMetaFixture(22)
	b.Overrides = map[skills.SkillID]skills.VisibilityOverride{
		sessionOtherSkillID: {
			SkillID: sessionOtherSkillID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityNameOnly,
		},
	}
	if err := store.SaveMeta("session-a", SessionMeta{Skills: a}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMeta("session-b", SessionMeta{Skills: b}); err != nil {
		t.Fatal(err)
	}

	fresh := NewFileStore(directory)
	metaA, err := fresh.GetMeta("session-a")
	if err != nil {
		t.Fatal(err)
	}
	metaB, err := fresh.GetMeta("session-b")
	if err != nil {
		t.Fatal(err)
	}
	if metaA.Skills.ContextEpoch != 21 || len(metaA.Skills.Overrides) != 1 || metaA.Skills.Overrides[sessionSkillID].Visibility != skills.VisibilityOff {
		t.Fatalf("session A state = %#v", metaA.Skills)
	}
	if metaB.Skills.ContextEpoch != 22 || len(metaB.Skills.Overrides) != 1 || metaB.Skills.Overrides[sessionOtherSkillID].Visibility != skills.VisibilityNameOnly {
		t.Fatalf("session B state = %#v", metaB.Skills)
	}
}

func TestSessionSkillsPartialMetaUpdatesAreRaceSafe(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "skills-meta-race"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("race")}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsByUpdate := make(chan error, 3)
	updates := []SessionMeta{
		{Title: "updated title"},
		{Usage: &SessionUsageMeta{InputTokens: 42}},
		{Skills: sessionSkillsMetaFixture(31)},
	}
	var wait sync.WaitGroup
	for _, update := range updates {
		update := update
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errorsByUpdate <- store.SaveMeta(sessionID, update)
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByUpdate)
	for err := range errorsByUpdate {
		if err != nil {
			t.Fatalf("concurrent SaveMeta: %v", err)
		}
	}
	meta, err := store.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "updated title" || meta.Usage == nil || meta.Usage.InputTokens != 42 {
		t.Fatalf("partial fields were lost: %#v", meta)
	}
	assertSessionSkillsFixture(t, meta.Skills, 31)
}

func sessionSkillsMetaFixture(epoch uint64) *SessionSkillsMeta {
	remembered := skills.VisibilityManualOnly
	return &SessionSkillsMeta{
		Overrides: map[skills.SkillID]skills.VisibilityOverride{
			sessionSkillID: {
				SkillID: sessionSkillID, Scope: skills.SkillScopeSession,
				Visibility: skills.VisibilityOff, LastNonOff: &remembered,
			},
		},
		ContextEpoch:      epoch,
		AnnouncedRevision: 19,
		AnnouncedEntries: map[skills.SkillID]SessionCatalogEntryDigest{
			sessionSkillID:      sessionEntryDigestA,
			sessionOtherSkillID: sessionEntryDigestB,
		},
		LoadedDigests: map[skills.SkillID]SessionLoadedSkillDigest{
			sessionSkillID:      {ContentDigest: sessionContentA, PayloadDigest: sessionPayloadA},
			sessionOtherSkillID: {ContentDigest: sessionContentB, PayloadDigest: sessionPayloadB},
		},
	}
}

func assertSessionSkillsFixture(t *testing.T, got *SessionSkillsMeta, epoch uint64) {
	t.Helper()
	if got == nil {
		t.Fatal("session skills metadata is nil")
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("session skills metadata invalid: %v", err)
	}
	if got.ContextEpoch != epoch || got.AnnouncedRevision != 19 {
		t.Fatalf("epoch/revision = %d/%d, want %d/19", got.ContextEpoch, got.AnnouncedRevision, epoch)
	}
	override := got.Overrides[sessionSkillID]
	if override.SkillID != sessionSkillID || override.Scope != skills.SkillScopeSession ||
		override.Visibility != skills.VisibilityOff || override.RestoreVisibility() != skills.VisibilityManualOnly {
		t.Fatalf("session override = %#v", override)
	}
	if got.AnnouncedEntries[sessionSkillID] != sessionEntryDigestA || got.AnnouncedEntries[sessionOtherSkillID] != sessionEntryDigestB {
		t.Fatalf("announced entries = %#v", got.AnnouncedEntries)
	}
	if loaded := got.LoadedDigests[sessionSkillID]; loaded.ContentDigest != sessionContentA || loaded.PayloadDigest != sessionPayloadA {
		t.Fatalf("loaded digest = %#v", loaded)
	}
}
