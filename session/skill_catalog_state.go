package session

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/agent-dance/luban/skills"
)

// SessionCatalogEntryDigest fingerprints one complete model-facing catalog
// entry. It deliberately differs from SkillDigest: summary, visibility, or
// other announced metadata can change without changing SKILL.md content.
type SessionCatalogEntryDigest string

// Validate requires the same canonical SHA-256 wire representation used by
// the catalog contracts while retaining the distinct semantic type.
func (digest SessionCatalogEntryDigest) Validate() error {
	if err := skills.SkillDigest(digest).Validate(); err != nil {
		return fmt.Errorf("invalid session catalog entry digest: %w", err)
	}
	return nil
}

// SessionLoadedSkillDigest identifies the exact skill content and rendered
// payload already present in one visible context epoch. Both values are needed:
// identical SKILL.md can render differently for different invocation args.
type SessionLoadedSkillDigest struct {
	ContentDigest skills.SkillDigest             `json:"content_digest"`
	PayloadDigest skills.InvocationPayloadDigest `json:"payload_digest"`
}

// Validate checks both the stable content and exact rendered payload digests.
func (loaded SessionLoadedSkillDigest) Validate() error {
	if err := loaded.ContentDigest.Validate(); err != nil {
		return fmt.Errorf("invalid loaded skill content digest: %w", err)
	}
	if err := loaded.PayloadDigest.Validate(); err != nil {
		return err
	}
	return nil
}

// SessionSkillsMeta is the optional skill-specific session sidecar. Overrides
// belong to the session lifetime. Announced and loaded ledgers belong only to
// ContextEpoch and must not be used after visible history is replaced.
type SessionSkillsMeta struct {
	Overrides         map[skills.SkillID]skills.VisibilityOverride `json:"overrides,omitempty"`
	ContextEpoch      uint64                                       `json:"context_epoch,omitempty"`
	AnnouncedRevision skills.CatalogRevision                       `json:"announced_revision,omitempty"`
	AnnouncedEntries  map[skills.SkillID]SessionCatalogEntryDigest `json:"announced_entries,omitempty"`
	LoadedDigests     map[skills.SkillID]SessionLoadedSkillDigest  `json:"loaded_digests,omitempty"`
}

// SessionSkillsVisibleState is evidence reconstructed from messages that are
// actually present in the current model-visible history. Reconstruction stays
// in loop/compact owners; this package only validates and reconciles it.
type SessionSkillsVisibleState struct {
	ContextEpoch      uint64
	AnnouncedRevision skills.CatalogRevision
	AnnouncedEntries  map[skills.SkillID]SessionCatalogEntryDigest
	LoadedDigests     map[skills.SkillID]SessionLoadedSkillDigest
}

// Clone returns a deep copy safe for partial metadata updates.
func (meta SessionSkillsMeta) Clone() SessionSkillsMeta {
	return SessionSkillsMeta{
		Overrides:         cloneSessionOverrides(meta.Overrides),
		ContextEpoch:      meta.ContextEpoch,
		AnnouncedRevision: meta.AnnouncedRevision,
		AnnouncedEntries:  cloneSessionCatalogEntries(meta.AnnouncedEntries),
		LoadedDigests:     cloneSessionLoadedDigests(meta.LoadedDigests),
	}
}

// Validate enforces stable IDs, session-only overrides, valid digests, and the
// rule that visible-context ledgers require a non-zero epoch.
func (meta SessionSkillsMeta) Validate() error {
	if _, err := normalizeSessionOverrides(meta.Overrides); err != nil {
		return err
	}
	if err := validateSessionCatalogLedger(meta.ContextEpoch, meta.AnnouncedRevision, meta.AnnouncedEntries); err != nil {
		return err
	}
	if len(meta.LoadedDigests) > 0 && meta.ContextEpoch == 0 {
		return errors.New("session skills: loaded digests require a context epoch")
	}
	for id, loaded := range meta.LoadedDigests {
		if err := id.Validate(); err != nil {
			return fmt.Errorf("session skills: loaded digest key: %w", err)
		}
		if err := loaded.Validate(); err != nil {
			return fmt.Errorf("session skills: loaded digest %s: %w", id, err)
		}
	}
	return nil
}

// CatalogCursor returns a defensive cursor only for the requested visible
// context epoch. A mismatch forces callers to emit a full snapshot.
func (meta SessionSkillsMeta) CatalogCursor(contextEpoch uint64) (skills.CatalogRevision, map[skills.SkillID]SessionCatalogEntryDigest, bool) {
	if contextEpoch == 0 || meta.ContextEpoch != contextEpoch || meta.AnnouncedRevision == 0 {
		return 0, nil, false
	}
	return meta.AnnouncedRevision, cloneSessionCatalogEntries(meta.AnnouncedEntries), true
}

// LoadedDigest returns an exact loaded-body record only for the requested
// visible context epoch. A mismatch forces full body injection.
func (meta SessionSkillsMeta) LoadedDigest(contextEpoch uint64, id skills.SkillID) (SessionLoadedSkillDigest, bool) {
	if contextEpoch == 0 || meta.ContextEpoch != contextEpoch {
		return SessionLoadedSkillDigest{}, false
	}
	loaded, ok := meta.LoadedDigests[id]
	return loaded, ok
}

// ReconcileSessionSkillsMeta binds persisted state to visible-history evidence.
// Overrides always survive. A context-epoch mismatch clears both ledgers. In a
// matching epoch, the catalog cursor is retained only when its complete state
// exactly matches visible evidence, and loaded entries are retained one by one
// only when the exact body/payload pair is visible.
func ReconcileSessionSkillsMeta(persisted *SessionSkillsMeta, visible SessionSkillsVisibleState) (*SessionSkillsMeta, error) {
	if visible.ContextEpoch == 0 {
		return nil, errors.New("session skills: visible context epoch must be non-zero")
	}
	visibleMeta := SessionSkillsMeta{
		ContextEpoch:      visible.ContextEpoch,
		AnnouncedRevision: visible.AnnouncedRevision,
		AnnouncedEntries:  visible.AnnouncedEntries,
		LoadedDigests:     visible.LoadedDigests,
	}
	if err := visibleMeta.Validate(); err != nil {
		return nil, fmt.Errorf("session skills: invalid visible state: %w", err)
	}

	current := &SessionSkillsMeta{}
	if persisted != nil {
		normalized, err := normalizeSessionSkillsMeta(persisted)
		if err != nil {
			return nil, err
		}
		current = normalized
	}
	if current.ContextEpoch != visible.ContextEpoch {
		return &SessionSkillsMeta{
			Overrides:    cloneSessionOverrides(current.Overrides),
			ContextEpoch: visible.ContextEpoch,
		}, nil
	}

	reconciled := &SessionSkillsMeta{
		Overrides:    cloneSessionOverrides(current.Overrides),
		ContextEpoch: visible.ContextEpoch,
	}
	if current.AnnouncedRevision == visible.AnnouncedRevision &&
		reflect.DeepEqual(current.AnnouncedEntries, visible.AnnouncedEntries) {
		reconciled.AnnouncedRevision = current.AnnouncedRevision
		reconciled.AnnouncedEntries = cloneSessionCatalogEntries(current.AnnouncedEntries)
	}
	for id, loaded := range current.LoadedDigests {
		if visibleLoaded, ok := visible.LoadedDigests[id]; ok && visibleLoaded == loaded {
			if reconciled.LoadedDigests == nil {
				reconciled.LoadedDigests = make(map[skills.SkillID]SessionLoadedSkillDigest)
			}
			reconciled.LoadedDigests[id] = loaded
		}
	}
	return reconciled, nil
}

func normalizeSessionSkillsMeta(meta *SessionSkillsMeta) (*SessionSkillsMeta, error) {
	if meta == nil {
		return nil, nil
	}
	normalized := meta.Clone()
	overrides, err := normalizeSessionOverrides(normalized.Overrides)
	if err != nil {
		return nil, err
	}
	normalized.Overrides = overrides
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	return &normalized, nil
}

func normalizeSessionOverrides(input map[skills.SkillID]skills.VisibilityOverride) (map[skills.SkillID]skills.VisibilityOverride, error) {
	if input == nil {
		return nil, nil
	}
	result := make(map[skills.SkillID]skills.VisibilityOverride, len(input))
	for id, override := range input {
		if err := id.Validate(); err != nil {
			return nil, fmt.Errorf("session skills: override key: %w", err)
		}
		if override.SkillID != "" && override.SkillID != id {
			return nil, fmt.Errorf("session skills: override %s embeds a different skill ID", id)
		}
		if override.Scope != "" && override.Scope != skills.SkillScopeSession {
			return nil, fmt.Errorf("session skills: override %s has non-session scope %s", id, override.Scope)
		}
		override.SkillID = id
		override.Scope = skills.SkillScopeSession
		if err := override.Validate(); err != nil {
			return nil, fmt.Errorf("session skills: override %s: %w", id, err)
		}
		result[id] = cloneSessionOverride(override)
	}
	return result, nil
}

func validateSessionCatalogLedger(epoch uint64, revision skills.CatalogRevision, entries map[skills.SkillID]SessionCatalogEntryDigest) error {
	if revision == 0 {
		if len(entries) > 0 {
			return errors.New("session skills: announced entries require an announced revision")
		}
		return nil
	}
	if epoch == 0 {
		return errors.New("session skills: announced revision requires a context epoch")
	}
	if err := revision.Validate(); err != nil {
		return err
	}
	for id, digest := range entries {
		if err := id.Validate(); err != nil {
			return fmt.Errorf("session skills: announced entry key: %w", err)
		}
		if err := digest.Validate(); err != nil {
			return fmt.Errorf("session skills: announced entry %s: %w", id, err)
		}
	}
	return nil
}

func cloneSessionOverrides(input map[skills.SkillID]skills.VisibilityOverride) map[skills.SkillID]skills.VisibilityOverride {
	if input == nil {
		return nil
	}
	result := make(map[skills.SkillID]skills.VisibilityOverride, len(input))
	for id, override := range input {
		result[id] = cloneSessionOverride(override)
	}
	return result
}

func cloneSessionOverride(override skills.VisibilityOverride) skills.VisibilityOverride {
	if override.LastNonOff != nil {
		lastNonOff := *override.LastNonOff
		override.LastNonOff = &lastNonOff
	}
	return override
}

func cloneSessionCatalogEntries(input map[skills.SkillID]SessionCatalogEntryDigest) map[skills.SkillID]SessionCatalogEntryDigest {
	if input == nil {
		return nil
	}
	result := make(map[skills.SkillID]SessionCatalogEntryDigest, len(input))
	for id, digest := range input {
		result[id] = digest
	}
	return result
}

func cloneSessionLoadedDigests(input map[skills.SkillID]SessionLoadedSkillDigest) map[skills.SkillID]SessionLoadedSkillDigest {
	if input == nil {
		return nil
	}
	result := make(map[skills.SkillID]SessionLoadedSkillDigest, len(input))
	for id, digest := range input {
		result[id] = digest
	}
	return result
}
