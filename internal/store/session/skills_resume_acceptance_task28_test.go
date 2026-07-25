package session

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestSkillsResumeAcceptanceTrustsOnlyCurrentVisibleEpoch(t *testing.T) {
	id := skills.SkillID("skill:project:resume")
	last := skills.VisibilityManualOnly
	entry := SessionCatalogEntryDigest("sha256:" + strings.Repeat("a", 64))
	loaded := SessionLoadedSkillDigest{
		ContentDigest: skills.SkillDigest("sha256:" + strings.Repeat("b", 64)),
		PayloadDigest: skills.InvocationPayloadDigest("sha256:" + strings.Repeat("c", 64)),
	}
	persisted := &SessionSkillsMeta{
		Overrides: map[skills.SkillID]skills.VisibilityOverride{id: {
			SkillID: id, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityOff, LastNonOff: &last,
		}},
		ContextEpoch:      7,
		AnnouncedRevision: 9,
		AnnouncedEntries:  map[skills.SkillID]SessionCatalogEntryDigest{id: entry},
		LoadedDigests:     map[skills.SkillID]SessionLoadedSkillDigest{id: loaded},
	}
	storeDir := t.TempDir()
	store := NewFileStore(storeDir)
	if err := store.Save("resume-session", []types.Message{types.UserMessage("visible history")}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMeta("resume-session", SessionMeta{Skills: persisted}); err != nil {
		t.Fatal(err)
	}
	// Simulate a fresh process: no pointer or in-memory map from the writer is
	// reused by the reader that feeds engine resume reconciliation.
	fresh := NewFileStore(storeDir)
	reloadedMeta, err := fresh.GetMeta("resume-session")
	if err != nil {
		t.Fatal(err)
	}
	if reloadedMeta.Skills == nil {
		t.Fatal("fresh sidecar load omitted skills metadata")
	}
	persisted = reloadedMeta.Skills

	matching, err := ReconcileSessionSkillsMeta(persisted, SessionSkillsVisibleState{
		ContextEpoch:      7,
		AnnouncedRevision: 9,
		AnnouncedEntries:  map[skills.SkillID]SessionCatalogEntryDigest{id: entry},
		// The body was compacted out even though the catalog remains visible.
		LoadedDigests: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if matching.AnnouncedRevision != 9 || matching.AnnouncedEntries[id] != entry || len(matching.LoadedDigests) != 0 {
		t.Fatalf("matching visible state=%#v", matching)
	}
	if got := matching.Overrides[id]; got.Visibility != skills.VisibilityOff || got.LastNonOff == nil || *got.LastNonOff != last {
		t.Fatalf("matching override=%#v", got)
	}

	newEpoch, err := ReconcileSessionSkillsMeta(persisted, SessionSkillsVisibleState{ContextEpoch: 8})
	if err != nil {
		t.Fatal(err)
	}
	if newEpoch.ContextEpoch != 8 || newEpoch.AnnouncedRevision != 0 || len(newEpoch.AnnouncedEntries) != 0 || len(newEpoch.LoadedDigests) != 0 {
		t.Fatalf("new epoch retained invisible ledger=%#v", newEpoch)
	}
	if got := newEpoch.Overrides[id]; got.Visibility != skills.VisibilityOff || got.LastNonOff == nil || *got.LastNonOff != last {
		t.Fatalf("new epoch lost session override=%#v", got)
	}
	if _, _, ok := newEpoch.CatalogCursor(8); ok {
		t.Fatal("new epoch exposed a cursor before a full catalog rebuild")
	}
	if _, ok := newEpoch.LoadedDigest(8, id); ok {
		t.Fatal("new epoch trusted a compacted-away skill body")
	}
}

func TestSkillsResumeAcceptanceRejectsSidecarAheadOfVisibleHistory(t *testing.T) {
	id := skills.SkillID("skill:user:resume")
	persistedEntry := SessionCatalogEntryDigest("sha256:" + strings.Repeat("d", 64))
	visibleEntry := SessionCatalogEntryDigest("sha256:" + strings.Repeat("e", 64))
	persisted := &SessionSkillsMeta{
		ContextEpoch:      4,
		AnnouncedRevision: 12,
		AnnouncedEntries:  map[skills.SkillID]SessionCatalogEntryDigest{id: persistedEntry},
	}
	reconciled, err := ReconcileSessionSkillsMeta(persisted, SessionSkillsVisibleState{
		ContextEpoch:      4,
		AnnouncedRevision: 10,
		AnnouncedEntries:  map[skills.SkillID]SessionCatalogEntryDigest{id: visibleEntry},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.AnnouncedRevision != 0 || len(reconciled.AnnouncedEntries) != 0 {
		t.Fatalf("sidecar ahead of history remained authoritative=%#v", reconciled)
	}
}
