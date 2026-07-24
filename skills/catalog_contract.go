package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

var (
	// ErrInvalidSkillID identifies a value that is not a stable catalog ID.
	ErrInvalidSkillID = errors.New("invalid skill ID")
	// ErrInvalidSkillLocator identifies an empty or unsafe canonical locator.
	ErrInvalidSkillLocator = errors.New("invalid skill locator")
	// ErrInvalidSkillDigest identifies a digest outside the sha256:<hex> contract.
	ErrInvalidSkillDigest = errors.New("invalid skill digest")
	// ErrInvalidCatalogRevision identifies a zero catalog revision where an
	// announced or authoritative catalog value requires a real revision.
	ErrInvalidCatalogRevision = errors.New("invalid catalog revision")
	// ErrInvalidSkillRevision identifies a zero effective-skill revision.
	ErrInvalidSkillRevision = errors.New("invalid skill revision")
	// ErrInvalidVisibility identifies an unknown visibility value.
	ErrInvalidVisibility = errors.New("invalid skill visibility")
	// ErrInvalidSkillScope identifies an unknown visibility ownership scope.
	ErrInvalidSkillScope = errors.New("invalid skill scope")
)

// SkillID is the stable identity of one discovered skill. It is deliberately
// independent of the display or invocation name, because multiple sources and
// locators may legitimately publish the same name.
//
// IDs use skill:<source>:<opaque-identity>. The opaque portion is derived from
// the canonical locator by the identity helpers; consumers must not parse it.
type SkillID string

const skillIDPrefix = "skill:"

// Validate reports whether id is a stable, source-qualified skill identity.
// In particular, a bare display name is never a valid SkillID.
func (id SkillID) Validate() error {
	raw := string(id)
	if raw == "" || strings.TrimSpace(raw) != raw || !strings.HasPrefix(raw, skillIDPrefix) {
		return fmt.Errorf("%w: expected skill:<source>:<identity>", ErrInvalidSkillID)
	}
	rest := strings.TrimPrefix(raw, skillIDPrefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || hasControl(parts[1]) {
		return fmt.Errorf("%w: expected skill:<source>:<identity>", ErrInvalidSkillID)
	}
	if err := SkillSource(parts[0]).Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSkillID, err)
	}
	return nil
}

// IsValid reports whether id satisfies the stable identity contract.
func (id SkillID) IsValid() bool { return id.Validate() == nil }

// Source returns the source encoded by a valid ID.
func (id SkillID) Source() (SkillSource, bool) {
	if id.Validate() != nil {
		return "", false
	}
	rest := strings.TrimPrefix(string(id), skillIDPrefix)
	parts := strings.SplitN(rest, ":", 2)
	return SkillSource(parts[0]), true
}

// Validate reports whether source is a supported owner of skill content.
func (source SkillSource) Validate() error {
	switch source {
	case SourceProject, SourceUser, SourceManaged, SourcePlugin, SourceMCP,
		SourceBundled, SourceCommandsLegacy:
		return nil
	default:
		return fmt.Errorf("unknown skill source %q", source)
	}
}

// SkillLocator is a canonical filesystem or virtual-resource locator. Its
// canonicalization is source-specific and belongs to the identity helpers.
type SkillLocator string

// Validate rejects empty, padded, or control-character-bearing locators.
func (locator SkillLocator) Validate() error {
	raw := string(locator)
	if raw == "" || strings.TrimSpace(raw) != raw || hasControl(raw) {
		return ErrInvalidSkillLocator
	}
	return nil
}

// SkillDigest identifies the exact effective SKILL.md content injected on
// invocation. It is always a lowercase SHA-256 digest in sha256:<hex> form.
type SkillDigest string

// Validate reports whether digest has the canonical SHA-256 representation.
func (digest SkillDigest) Validate() error {
	raw := string(digest)
	if len(raw) != len("sha256:")+64 || !strings.HasPrefix(raw, "sha256:") {
		return ErrInvalidSkillDigest
	}
	for _, r := range strings.TrimPrefix(raw, "sha256:") {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return ErrInvalidSkillDigest
		}
	}
	return nil
}

// CatalogRevision monotonically identifies an authoritative catalog state.
// Revision zero is reserved for "not observed yet" cursors.
type CatalogRevision uint64

// Validate requires an announced or authoritative catalog revision.
func (revision CatalogRevision) Validate() error {
	if revision == 0 {
		return ErrInvalidCatalogRevision
	}
	return nil
}

// SkillRevision monotonically identifies the latest effective state for one
// stable SkillID. It changes when any effective catalog field changes.
type SkillRevision uint64

// Validate requires an effective skill revision.
func (revision SkillRevision) Validate() error {
	if revision == 0 {
		return ErrInvalidSkillRevision
	}
	return nil
}

// Visibility controls discovery and invocation without conflating prompt
// visibility with execution authorization.
type Visibility string

const (
	// VisibilityAuto uses the skill's effective policy defaults.
	VisibilityAuto Visibility = "auto"
	// VisibilityNameOnly exposes the name but never its description.
	VisibilityNameOnly Visibility = "name-only"
	// VisibilityManualOnly hides the skill from model discovery while retaining
	// explicit user invocation when policy permits it.
	VisibilityManualOnly Visibility = "manual-only"
	// VisibilityOff hides the skill and blocks future execution.
	VisibilityOff Visibility = "off"
)

// Validate reports whether visibility is one of the four supported states.
func (visibility Visibility) Validate() error {
	switch visibility {
	case VisibilityAuto, VisibilityNameOnly, VisibilityManualOnly, VisibilityOff:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidVisibility, visibility)
	}
}

// IsNonOff reports whether visibility can be remembered for a later binary
// re-enable operation.
func (visibility Visibility) IsNonOff() bool {
	return visibility == VisibilityAuto || visibility == VisibilityNameOnly || visibility == VisibilityManualOnly
}

// SkillScope identifies the layer that owns a visibility value. Precedence is
// managed deny, session, project, user, frontmatter, then default. A lower
// layer can never relax a managed restriction.
type SkillScope string

const (
	SkillScopeDefault     SkillScope = "default"
	SkillScopeFrontmatter SkillScope = "frontmatter"
	SkillScopeUser        SkillScope = "user"
	SkillScopeProject     SkillScope = "project"
	SkillScopeSession     SkillScope = "session"
	SkillScopeManaged     SkillScope = "managed"
)

// Validate reports whether scope is a known visibility ownership layer.
func (scope SkillScope) Validate() error {
	switch scope {
	case SkillScopeDefault, SkillScopeFrontmatter, SkillScopeUser,
		SkillScopeProject, SkillScopeSession, SkillScopeManaged:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidSkillScope, scope)
	}
}

// VisibilityOverride is the persisted or session-local value for one stable
// skill at one scope. LastNonOff is valid only while Visibility is off and lets
// a binary toggle restore name-only or manual-only rather than losing intent.
//
// JSON decoding accepts the legacy scalar form (for example, "off") as well
// as the structured form. Stores that use map keys for ID and scope attach
// those fields after decoding the scalar; MarshalJSON always emits the
// structured representation so LastNonOff can be preserved.
type VisibilityOverride struct {
	SkillID    SkillID     `json:"skill_id,omitempty"`
	Scope      SkillScope  `json:"scope,omitempty"`
	Visibility Visibility  `json:"visibility"`
	LastNonOff *Visibility `json:"last_non_off,omitempty"`
}

// Validate checks identity, ownership, and last-non-off invariants.
func (override VisibilityOverride) Validate() error {
	if err := override.SkillID.Validate(); err != nil {
		return err
	}
	if err := override.Scope.Validate(); err != nil {
		return err
	}
	if err := override.Visibility.Validate(); err != nil {
		return err
	}
	if override.LastNonOff == nil {
		return nil
	}
	if override.Visibility != VisibilityOff || !override.LastNonOff.IsNonOff() {
		return fmt.Errorf("%w: last_non_off is valid only for off overrides", ErrInvalidVisibility)
	}
	return nil
}

// RestoreVisibility returns the state a binary enable action should restore.
// Old scalar off records have no remembered state and therefore restore auto.
func (override VisibilityOverride) RestoreVisibility() Visibility {
	if override.Visibility == VisibilityOff && override.LastNonOff != nil && override.LastNonOff.IsNonOff() {
		return *override.LastNonOff
	}
	return VisibilityAuto
}

// UnmarshalJSON accepts both legacy scalar and structured override records.
func (override *VisibilityOverride) UnmarshalJSON(data []byte) error {
	if override == nil {
		return errors.New("nil visibility override")
	}
	var scalar Visibility
	if err := json.Unmarshal(data, &scalar); err == nil {
		if err := scalar.Validate(); err != nil {
			return err
		}
		*override = VisibilityOverride{Visibility: scalar}
		return nil
	}
	type wire VisibilityOverride
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := decoded.Visibility.Validate(); err != nil {
		return err
	}
	if decoded.LastNonOff != nil {
		if decoded.Visibility != VisibilityOff || !decoded.LastNonOff.IsNonOff() {
			return fmt.Errorf("%w: invalid last_non_off", ErrInvalidVisibility)
		}
	}
	*override = VisibilityOverride(decoded)
	return nil
}

// EffectiveSkill is an immutable-at-boundary view of one stable discovered
// skill after policy evaluation. It contains metadata only; full SKILL.md text
// is loaded and versioned separately on invocation.
type EffectiveSkill struct {
	ID                 SkillID       `json:"id"`
	Name               string        `json:"name"`
	Summary            string        `json:"summary,omitempty"`
	SummaryGenerated   bool          `json:"summary_generated,omitempty"`
	Source             SkillSource   `json:"source"`
	Locator            SkillLocator  `json:"locator"`
	Digest             SkillDigest   `json:"digest"`
	Revision           SkillRevision `json:"revision"`
	Visibility         Visibility    `json:"visibility"`
	VisibilitySource   SkillScope    `json:"visibility_source"`
	ModelVisible       bool          `json:"model_visible"`
	DescriptionVisible bool          `json:"description_visible"`
	UserInvocable      bool          `json:"user_invocable"`
	Executable         bool          `json:"executable"`
	Mutable            bool          `json:"mutable"`
	ReadOnlyReason     string        `json:"read_only_reason,omitempty"`
	ShadowedBy         SkillID       `json:"shadowed_by,omitempty"`
}

// Validate checks the value-level safety invariants shared by every registry
// producer. Detailed policy precedence belongs to the policy evaluator.
func (skill EffectiveSkill) Validate() error {
	if err := skill.ID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(skill.Name) == "" {
		return errors.New("skill name is empty")
	}
	if err := skill.Source.Validate(); err != nil {
		return err
	}
	if encodedSource, ok := skill.ID.Source(); !ok || encodedSource != skill.Source {
		return fmt.Errorf("%w: source does not match ID", ErrInvalidSkillID)
	}
	if err := skill.Locator.Validate(); err != nil {
		return err
	}
	if err := skill.Digest.Validate(); err != nil {
		return err
	}
	if err := skill.Revision.Validate(); err != nil {
		return err
	}
	if err := skill.Visibility.Validate(); err != nil {
		return err
	}
	if err := skill.VisibilitySource.Validate(); err != nil {
		return err
	}
	if skill.DescriptionVisible && !skill.ModelVisible {
		return errors.New("description cannot be visible when skill is not model-visible")
	}
	if skill.Visibility == VisibilityOff && (skill.ModelVisible || skill.DescriptionVisible || skill.UserInvocable || skill.Executable) {
		return errors.New("off skill cannot be visible, invocable, or executable")
	}
	if skill.Visibility == VisibilityNameOnly && skill.DescriptionVisible {
		return errors.New("name-only skill cannot expose its description")
	}
	if skill.Visibility == VisibilityManualOnly && skill.ModelVisible {
		return errors.New("manual-only skill cannot be model-visible")
	}
	if skill.ShadowedBy != "" {
		if err := skill.ShadowedBy.Validate(); err != nil {
			return fmt.Errorf("invalid shadowing identity: %w", err)
		}
		if skill.ShadowedBy == skill.ID {
			return errors.New("skill cannot shadow itself")
		}
		if skill.ModelVisible || skill.Executable {
			return errors.New("shadowed skill cannot be model-visible or executable")
		}
	}
	if !skill.Mutable && strings.TrimSpace(skill.ReadOnlyReason) == "" {
		return errors.New("immutable skill requires a read-only reason")
	}
	return nil
}

// CatalogSnapshot is a complete immutable-at-boundary registry view at one
// revision. Callers must treat Skills as read-only; NewCatalogSnapshot and
// Clone defensively copy it.
type CatalogSnapshot struct {
	Revision CatalogRevision  `json:"revision"`
	Skills   []EffectiveSkill `json:"skills"`
}

// NewCatalogSnapshot validates, copies, and deterministically orders skills by
// stable ID.
func NewCatalogSnapshot(revision CatalogRevision, skills []EffectiveSkill) (CatalogSnapshot, error) {
	snapshot := CatalogSnapshot{Revision: revision, Skills: append([]EffectiveSkill(nil), skills...)}
	sort.Slice(snapshot.Skills, func(i, j int) bool { return snapshot.Skills[i].ID < snapshot.Skills[j].ID })
	if err := snapshot.Validate(); err != nil {
		return CatalogSnapshot{}, err
	}
	return snapshot, nil
}

// Clone returns a defensive copy suitable for crossing a registry boundary.
func (snapshot CatalogSnapshot) Clone() CatalogSnapshot {
	return CatalogSnapshot{Revision: snapshot.Revision, Skills: append([]EffectiveSkill(nil), snapshot.Skills...)}
}

// Validate checks the snapshot revision, entries, unique IDs, and canonical
// ordering.
func (snapshot CatalogSnapshot) Validate() error {
	if err := snapshot.Revision.Validate(); err != nil {
		return err
	}
	var previous SkillID
	for index, skill := range snapshot.Skills {
		if err := skill.Validate(); err != nil {
			return fmt.Errorf("skill %d: %w", index, err)
		}
		if index > 0 && skill.ID <= previous {
			return errors.New("snapshot skill IDs must be unique and sorted")
		}
		previous = skill.ID
	}
	return nil
}

// Find returns a copy of the skill with id, if present.
func (snapshot CatalogSnapshot) Find(id SkillID) (EffectiveSkill, bool) {
	index := sort.Search(len(snapshot.Skills), func(i int) bool { return snapshot.Skills[i].ID >= id })
	if index >= len(snapshot.Skills) || snapshot.Skills[index].ID != id {
		return EffectiveSkill{}, false
	}
	return snapshot.Skills[index], true
}

// CatalogUpsertReason explains why the latest state must be announced.
type CatalogUpsertReason string

const (
	CatalogUpsertAdded     CatalogUpsertReason = "added"
	CatalogUpsertUpdated   CatalogUpsertReason = "updated"
	CatalogUpsertReenabled CatalogUpsertReason = "re-enabled"
)

func (reason CatalogUpsertReason) validate() error {
	switch reason {
	case CatalogUpsertAdded, CatalogUpsertUpdated, CatalogUpsertReenabled:
		return nil
	default:
		return fmt.Errorf("invalid catalog upsert reason %q", reason)
	}
}

// CatalogRevokeReason explains why a previously announced skill must no
// longer be considered usable.
type CatalogRevokeReason string

const (
	CatalogRevokeDisabled       CatalogRevokeReason = "disabled"
	CatalogRevokeDeleted        CatalogRevokeReason = "deleted"
	CatalogRevokeVisibility     CatalogRevokeReason = "visibility-changed"
	CatalogRevokePermissionLost CatalogRevokeReason = "permission-lost"
	CatalogRevokeShadowed       CatalogRevokeReason = "shadowed"
)

func (reason CatalogRevokeReason) validate() error {
	switch reason {
	case CatalogRevokeDisabled, CatalogRevokeDeleted, CatalogRevokeVisibility,
		CatalogRevokePermissionLost, CatalogRevokeShadowed:
		return nil
	default:
		return fmt.Errorf("invalid catalog revoke reason %q", reason)
	}
}

// CatalogUpsert replaces any older model-facing state for Skill.ID. When
// multiple changes happen before a sampling boundary, latest-revision-wins and
// only the final upsert or revoke for that ID is retained.
type CatalogUpsert struct {
	Skill  EffectiveSkill      `json:"skill"`
	Reason CatalogUpsertReason `json:"reason"`
}

// Validate checks an upsert value.
func (upsert CatalogUpsert) Validate() error {
	if err := upsert.Reason.validate(); err != nil {
		return err
	}
	return upsert.Skill.Validate()
}

// CatalogRevoke removes a stable ID from future model use. A revoke is an
// authorization signal for future execution; it cannot erase metadata or full
// skill text already present in prompt history. Sensitive revocation therefore
// requires a new or scrubbed context.
type CatalogRevoke struct {
	ID       SkillID             `json:"id"`
	Name     string              `json:"name,omitempty"`
	Source   SkillSource         `json:"source"`
	Locator  SkillLocator        `json:"locator"`
	Revision SkillRevision       `json:"revision"`
	Reason   CatalogRevokeReason `json:"reason"`
}

// Validate checks a revoke's stable ownership and reason metadata.
func (revoke CatalogRevoke) Validate() error {
	if err := revoke.ID.Validate(); err != nil {
		return err
	}
	if err := revoke.Source.Validate(); err != nil {
		return err
	}
	if encodedSource, ok := revoke.ID.Source(); !ok || encodedSource != revoke.Source {
		return fmt.Errorf("%w: source does not match ID", ErrInvalidSkillID)
	}
	if err := revoke.Locator.Validate(); err != nil {
		return err
	}
	if err := revoke.Revision.Validate(); err != nil {
		return err
	}
	return revoke.Reason.validate()
}

// CatalogDelta is the coalesced append-only model projection between two
// authoritative revisions. For each ID, only its state at ToRevision appears.
// Older conversation messages are never edited or removed.
type CatalogDelta struct {
	FromRevision CatalogRevision `json:"from_revision"`
	ToRevision   CatalogRevision `json:"to_revision"`
	Upserts      []CatalogUpsert `json:"upserts,omitempty"`
	Revokes      []CatalogRevoke `json:"revokes,omitempty"`
}

// NewCatalogDelta copies, sorts, and validates one coalesced delta.
func NewCatalogDelta(from, to CatalogRevision, upserts []CatalogUpsert, revokes []CatalogRevoke) (CatalogDelta, error) {
	delta := CatalogDelta{
		FromRevision: from,
		ToRevision:   to,
		Upserts:      append([]CatalogUpsert(nil), upserts...),
		Revokes:      append([]CatalogRevoke(nil), revokes...),
	}
	sort.Slice(delta.Upserts, func(i, j int) bool { return delta.Upserts[i].Skill.ID < delta.Upserts[j].Skill.ID })
	sort.Slice(delta.Revokes, func(i, j int) bool { return delta.Revokes[i].ID < delta.Revokes[j].ID })
	if err := delta.Validate(); err != nil {
		return CatalogDelta{}, err
	}
	return delta, nil
}

// Clone returns a defensive copy of the delta event slices.
func (delta CatalogDelta) Clone() CatalogDelta {
	return CatalogDelta{
		FromRevision: delta.FromRevision,
		ToRevision:   delta.ToRevision,
		Upserts:      append([]CatalogUpsert(nil), delta.Upserts...),
		Revokes:      append([]CatalogRevoke(nil), delta.Revokes...),
	}
}

// Empty reports whether the delta has no model-facing events. Revisions may
// still advance when a registry change does not affect the model projection.
func (delta CatalogDelta) Empty() bool { return len(delta.Upserts) == 0 && len(delta.Revokes) == 0 }

// Validate checks revision monotonicity, event values, stable-ID uniqueness,
// and deterministic ordering. The all-zero delta is a valid initial no-op.
func (delta CatalogDelta) Validate() error {
	if delta.FromRevision == 0 && delta.ToRevision == 0 && delta.Empty() {
		return nil
	}
	if delta.FromRevision == 0 || delta.ToRevision == 0 || delta.ToRevision < delta.FromRevision {
		return errors.New("invalid catalog delta revisions")
	}
	if !delta.Empty() && delta.ToRevision == delta.FromRevision {
		return errors.New("catalog events require a newer revision")
	}
	seen := make(map[SkillID]struct{}, len(delta.Upserts)+len(delta.Revokes))
	var previous SkillID
	for index, upsert := range delta.Upserts {
		if err := upsert.Validate(); err != nil {
			return fmt.Errorf("upsert %d: %w", index, err)
		}
		id := upsert.Skill.ID
		if index > 0 && id <= previous {
			return errors.New("upserts must be unique and sorted")
		}
		seen[id] = struct{}{}
		previous = id
	}
	previous = ""
	for index, revoke := range delta.Revokes {
		if err := revoke.Validate(); err != nil {
			return fmt.Errorf("revoke %d: %w", index, err)
		}
		if index > 0 && revoke.ID <= previous {
			return errors.New("revokes must be unique and sorted")
		}
		if _, exists := seen[revoke.ID]; exists {
			return errors.New("skill ID cannot be both upserted and revoked")
		}
		seen[revoke.ID] = struct{}{}
		previous = revoke.ID
	}
	return nil
}

// ProjectVisibilityToggleRequest is the compare-and-swap input used by the
// project-persistent interactive toggle. ExpectedRevision is the revision the
// UI observed; stale rows must be rejected instead of targeting a replacement
// with the same display name.
type ProjectVisibilityToggleRequest struct {
	SessionID        string          `json:"session_id"`
	SkillID          SkillID         `json:"skill_id"`
	ExpectedRevision CatalogRevision `json:"expected_revision"`
}

// Validate checks the mutation target and observed revision.
func (request ProjectVisibilityToggleRequest) Validate() error {
	if strings.TrimSpace(request.SessionID) == "" || strings.TrimSpace(request.SessionID) != request.SessionID {
		return errors.New("session ID is empty or padded")
	}
	if err := request.SkillID.Validate(); err != nil {
		return err
	}
	return request.ExpectedRevision.Validate()
}

// ProjectVisibilityToggleOutcome reports the transaction truth rather than
// assuming that persistence, live apply, and compensating rollback all share
// one failure mode.
type ProjectVisibilityToggleOutcome string

const (
	ProjectVisibilityToggleCommitted ProjectVisibilityToggleOutcome = "committed"
	ProjectVisibilityToggleRejected  ProjectVisibilityToggleOutcome = "rejected"
	ProjectVisibilityToggleDegraded  ProjectVisibilityToggleOutcome = "degraded-refresh-required"
)

func (outcome ProjectVisibilityToggleOutcome) validate() error {
	switch outcome {
	case ProjectVisibilityToggleCommitted, ProjectVisibilityToggleRejected, ProjectVisibilityToggleDegraded:
		return nil
	default:
		return fmt.Errorf("invalid project visibility toggle outcome %q", outcome)
	}
}

// ProjectVisibilityToggleReason provides a typed, UI-independent explanation
// for a rejected or degraded transaction.
type ProjectVisibilityToggleReason string

const (
	ProjectVisibilityToggleReasonNone                 ProjectVisibilityToggleReason = ""
	ProjectVisibilityToggleReasonStaleRevision        ProjectVisibilityToggleReason = "stale-revision"
	ProjectVisibilityToggleReasonUnknownSkill         ProjectVisibilityToggleReason = "unknown-skill"
	ProjectVisibilityToggleReasonReadOnly             ProjectVisibilityToggleReason = "read-only"
	ProjectVisibilityToggleReasonSessionOverride      ProjectVisibilityToggleReason = "session-override"
	ProjectVisibilityToggleReasonPersistenceFailed    ProjectVisibilityToggleReason = "persistence-failed"
	ProjectVisibilityToggleReasonLiveApplyRolledBack  ProjectVisibilityToggleReason = "live-apply-failed-rolled-back"
	ProjectVisibilityToggleReasonRollbackFailed       ProjectVisibilityToggleReason = "rollback-failed"
	ProjectVisibilityToggleReasonAuthoritativeRefresh ProjectVisibilityToggleReason = "authoritative-refresh-failed"
)

func (reason ProjectVisibilityToggleReason) validate() error {
	switch reason {
	case ProjectVisibilityToggleReasonNone,
		ProjectVisibilityToggleReasonStaleRevision,
		ProjectVisibilityToggleReasonUnknownSkill,
		ProjectVisibilityToggleReasonReadOnly,
		ProjectVisibilityToggleReasonSessionOverride,
		ProjectVisibilityToggleReasonPersistenceFailed,
		ProjectVisibilityToggleReasonLiveApplyRolledBack,
		ProjectVisibilityToggleReasonRollbackFailed,
		ProjectVisibilityToggleReasonAuthoritativeRefresh:
		return nil
	default:
		return fmt.Errorf("invalid project visibility toggle reason %q", reason)
	}
}

// ProjectVisibilityToggleResult contains the authoritative state after a full
// persist/apply/compensate transaction. Skill is nil only when the stable ID no
// longer exists. Snapshot is always suitable for a non-optimistic UI redraw.
type ProjectVisibilityToggleResult struct {
	Outcome          ProjectVisibilityToggleOutcome `json:"outcome"`
	Reason           ProjectVisibilityToggleReason  `json:"reason,omitempty"`
	RequestedSkillID SkillID                        `json:"requested_skill_id"`
	ObservedRevision CatalogRevision                `json:"observed_revision"`
	CurrentRevision  CatalogRevision                `json:"current_revision"`
	Skill            *EffectiveSkill                `json:"skill,omitempty"`
	Snapshot         CatalogSnapshot                `json:"snapshot"`
}

// RefreshRequired reports whether the result is explicitly degraded and must
// not be presented as rolled back or committed.
func (result ProjectVisibilityToggleResult) RefreshRequired() bool {
	return result.Outcome == ProjectVisibilityToggleDegraded
}

// Validate checks result consistency without inventing success for a partial
// transaction.
func (result ProjectVisibilityToggleResult) Validate() error {
	if err := result.Outcome.validate(); err != nil {
		return err
	}
	if err := result.Reason.validate(); err != nil {
		return err
	}
	if err := result.RequestedSkillID.Validate(); err != nil {
		return err
	}
	if err := result.ObservedRevision.Validate(); err != nil {
		return err
	}
	if err := result.CurrentRevision.Validate(); err != nil {
		return err
	}
	if err := result.Snapshot.Validate(); err != nil {
		return err
	}
	if result.Snapshot.Revision != result.CurrentRevision {
		return errors.New("result revision does not match authoritative snapshot")
	}
	if result.Skill != nil {
		if err := result.Skill.Validate(); err != nil {
			return err
		}
		if result.Skill.ID != result.RequestedSkillID {
			return errors.New("result skill does not match requested stable ID")
		}
		if snapshotSkill, ok := result.Snapshot.Find(result.Skill.ID); !ok || snapshotSkill != *result.Skill {
			return errors.New("result skill does not match authoritative snapshot")
		}
	}
	switch result.Outcome {
	case ProjectVisibilityToggleCommitted:
		if result.Reason != ProjectVisibilityToggleReasonNone || result.Skill == nil {
			return errors.New("committed toggle requires a current skill and no failure reason")
		}
	case ProjectVisibilityToggleRejected:
		if result.Reason == ProjectVisibilityToggleReasonNone ||
			result.Reason == ProjectVisibilityToggleReasonRollbackFailed ||
			result.Reason == ProjectVisibilityToggleReasonAuthoritativeRefresh {
			return errors.New("rejected toggle requires a non-degraded reason")
		}
	case ProjectVisibilityToggleDegraded:
		if result.Reason != ProjectVisibilityToggleReasonRollbackFailed &&
			result.Reason != ProjectVisibilityToggleReasonAuthoritativeRefresh {
			return errors.New("degraded toggle requires a refresh-related reason")
		}
	}
	return nil
}

func hasControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}
