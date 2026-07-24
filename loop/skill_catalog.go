package loop

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// SkillCatalogContextEpoch identifies one model-visible history generation.
// Compaction, recovery, or another wholesale history replacement must install
// a new epoch so the coordinator rebuilds a full catalog snapshot instead of
// appending a delta to metadata that is no longer visible.
type SkillCatalogContextEpoch string

// Validate rejects missing or padded epoch identities.
func (epoch SkillCatalogContextEpoch) Validate() error {
	raw := string(epoch)
	if raw == "" || strings.TrimSpace(raw) != raw {
		return errors.New("skill catalog context epoch is empty or padded")
	}
	return nil
}

// SkillCatalogCursor is the narrow, persistence-safe ledger needed to plan the
// next model projection. AnnouncedSnapshot is the authoritative state at the
// last catalog developer message actually present in this context epoch.
// LedgerSnapshot is the latest authoritative snapshot observed at a sampling
// boundary.
//
// The two revisions may differ: a registry revision that changes no
// model-facing fields advances LedgerSnapshot without emitting a message. The
// next delta must still be computed from AnnouncedSnapshot so its from_revision
// matches the catalog state visible to the model.
type SkillCatalogCursor struct {
	ContextEpoch         SkillCatalogContextEpoch `json:"context_epoch"`
	AnnouncedSnapshot    skills.CatalogSnapshot   `json:"announced_snapshot"`
	LedgerSnapshot       skills.CatalogSnapshot   `json:"ledger_snapshot"`
	VisibleMessageDigest string                   `json:"visible_message_digest"`
}

// Empty reports whether no catalog projection has been planned yet.
func (cursor SkillCatalogCursor) Empty() bool {
	return cursor.ContextEpoch == "" &&
		cursor.AnnouncedSnapshot.Revision == 0 &&
		len(cursor.AnnouncedSnapshot.Skills) == 0 &&
		cursor.LedgerSnapshot.Revision == 0 &&
		len(cursor.LedgerSnapshot.Skills) == 0 &&
		cursor.VisibleMessageDigest == ""
}

// Clone returns a defensive copy of the cursor ledger.
func (cursor SkillCatalogCursor) Clone() SkillCatalogCursor {
	return SkillCatalogCursor{
		ContextEpoch:         cursor.ContextEpoch,
		AnnouncedSnapshot:    cursor.AnnouncedSnapshot.Clone(),
		LedgerSnapshot:       cursor.LedgerSnapshot.Clone(),
		VisibleMessageDigest: cursor.VisibleMessageDigest,
	}
}

// AnnouncedRevision returns the revision represented by the last visible
// catalog developer message.
func (cursor SkillCatalogCursor) AnnouncedRevision() skills.CatalogRevision {
	return cursor.AnnouncedSnapshot.Revision
}

// Validate checks a non-empty cursor without requiring its visible revision
// to equal the ledger revision; model-neutral catalog changes may separate the
// two as documented above. The all-zero cursor represents initial state.
func (cursor SkillCatalogCursor) Validate() error {
	if cursor.Empty() {
		return nil
	}
	if err := cursor.ContextEpoch.Validate(); err != nil {
		return err
	}
	if err := cursor.AnnouncedSnapshot.Validate(); err != nil {
		return fmt.Errorf("skill catalog cursor announced snapshot: %w", err)
	}
	if err := cursor.LedgerSnapshot.Validate(); err != nil {
		return fmt.Errorf("skill catalog cursor ledger: %w", err)
	}
	ledgerDelta, err := skills.DiffCatalog(cursor.AnnouncedSnapshot, cursor.LedgerSnapshot)
	if err != nil {
		return fmt.Errorf("skill catalog cursor ledger transition: %w", err)
	}
	if !ledgerDelta.Empty() {
		return errors.New("skill catalog cursor ledger contains unannounced model-facing changes")
	}
	if !validSkillCatalogMessageDigest(cursor.VisibleMessageDigest) {
		return errors.New("invalid skill catalog visible message digest")
	}
	return nil
}

// SkillCatalogPlanKind describes the optional developer message returned for a
// sampling boundary.
type SkillCatalogPlanKind string

const (
	SkillCatalogPlanNone     SkillCatalogPlanKind = "none"
	SkillCatalogPlanSnapshot SkillCatalogPlanKind = "snapshot"
	SkillCatalogPlanDelta    SkillCatalogPlanKind = "delta"
)

// SkillCatalogRebuildReason explains why a full snapshot was selected instead
// of a tail delta. Values are internal state, not user-visible copy.
type SkillCatalogRebuildReason string

const (
	SkillCatalogRebuildInitial         SkillCatalogRebuildReason = "initial"
	SkillCatalogRebuildEpochChanged    SkillCatalogRebuildReason = "context-epoch-changed"
	SkillCatalogRebuildHistoryMissing  SkillCatalogRebuildReason = "visible-history-missing"
	SkillCatalogRebuildHistoryMismatch SkillCatalogRebuildReason = "visible-history-mismatch"
)

// SkillCatalogCoordinatorInput contains only immutable values needed to plan
// one sampling boundary. VisibleHistory is inspected but never modified.
// CharBudget is a character budget, not a context-window token count.
type SkillCatalogCoordinatorInput struct {
	CurrentSnapshot skills.CatalogSnapshot
	PriorCursor     SkillCatalogCursor
	ContextEpoch    SkillCatalogContextEpoch
	VisibleHistory  []types.Message
	CharBudget      int
}

// SkillCatalogPlan is a pure append instruction plus its next persistence
// cursor. When Message is non-nil, the caller must insert it immediately before
// the current user input. The coordinator never rewrites VisibleHistory.
//
// Render preserves renderer budget diagnostics, including mandatory metadata
// overflow, so integration code can observe pressure without discarding stable
// names or revokes.
type SkillCatalogPlan struct {
	Kind          SkillCatalogPlanKind
	RebuildReason SkillCatalogRebuildReason
	Message       *types.Message
	Cursor        SkillCatalogCursor
	Render        skills.CatalogRenderResult
}

// HasMessage reports whether the plan contains a developer catalog message.
func (plan SkillCatalogPlan) HasMessage() bool { return plan.Message != nil }

// PlanSkillCatalog returns an initial/rebuilt snapshot, one coalesced delta, or
// no message. Registry state is only projected here; execution authorization
// remains the responsibility of the latest effective skill registry.
func PlanSkillCatalog(input SkillCatalogCoordinatorInput) (SkillCatalogPlan, error) {
	if err := input.ContextEpoch.Validate(); err != nil {
		return SkillCatalogPlan{}, err
	}
	current := input.CurrentSnapshot.Clone()
	if err := current.Validate(); err != nil {
		return SkillCatalogPlan{}, fmt.Errorf("current skill catalog snapshot: %w", err)
	}

	prior := input.PriorCursor.Clone()
	switch {
	case prior.Empty():
		if rebound, ok, err := rebindVisibleSkillCatalogSnapshot(current, input.ContextEpoch, input.VisibleHistory, input.CharBudget); err != nil {
			return SkillCatalogPlan{}, err
		} else if ok {
			return rebound, nil
		}
		return planSkillCatalogSnapshot(current, input.ContextEpoch, input.CharBudget, SkillCatalogRebuildInitial)
	case prior.ContextEpoch != input.ContextEpoch:
		return planSkillCatalogSnapshot(current, input.ContextEpoch, input.CharBudget, SkillCatalogRebuildEpochChanged)
	}

	visibleRevision, visibleDigest, visible := latestVisibleSkillCatalogState(input.VisibleHistory)
	if !visible {
		return planSkillCatalogSnapshot(current, input.ContextEpoch, input.CharBudget, SkillCatalogRebuildHistoryMissing)
	}
	if visibleRevision != prior.AnnouncedRevision() || visibleDigest != prior.VisibleMessageDigest {
		return planSkillCatalogSnapshot(current, input.ContextEpoch, input.CharBudget, SkillCatalogRebuildHistoryMismatch)
	}
	if err := prior.Validate(); err != nil {
		return SkillCatalogPlan{}, err
	}

	if _, err := skills.DiffCatalog(prior.LedgerSnapshot, current); err != nil {
		return SkillCatalogPlan{}, fmt.Errorf("advance skill catalog ledger: %w", err)
	}
	delta, err := skills.DiffCatalog(prior.AnnouncedSnapshot, current)
	if err != nil {
		return SkillCatalogPlan{}, fmt.Errorf("diff announced skill catalog: %w", err)
	}
	nextCursor := SkillCatalogCursor{
		ContextEpoch:         input.ContextEpoch,
		AnnouncedSnapshot:    prior.AnnouncedSnapshot.Clone(),
		LedgerSnapshot:       current.Clone(),
		VisibleMessageDigest: prior.VisibleMessageDigest,
	}
	rendered, err := skills.RenderCatalogDelta(delta, input.CharBudget)
	if err != nil {
		return SkillCatalogPlan{}, fmt.Errorf("render skill catalog delta: %w", err)
	}
	if delta.Empty() {
		return SkillCatalogPlan{
			Kind:   SkillCatalogPlanNone,
			Cursor: nextCursor,
			Render: rendered,
		}, nil
	}

	if rendered.Empty() {
		return SkillCatalogPlan{}, errors.New("non-empty skill catalog delta rendered no message")
	}
	message := types.DeveloperMessage(rendered.Text, types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogDelta,
		Revision: uint64(current.Revision),
	})
	nextCursor.AnnouncedSnapshot = current.Clone()
	nextCursor.VisibleMessageDigest = skillCatalogMessageDigest(message)
	return SkillCatalogPlan{
		Kind:    SkillCatalogPlanDelta,
		Message: &message,
		Cursor:  nextCursor,
		Render:  rendered,
	}, nil
}

func planSkillCatalogSnapshot(
	current skills.CatalogSnapshot,
	epoch SkillCatalogContextEpoch,
	charBudget int,
	reason SkillCatalogRebuildReason,
) (SkillCatalogPlan, error) {
	rendered, err := skills.RenderCatalogSnapshot(current, charBudget)
	if err != nil {
		return SkillCatalogPlan{}, fmt.Errorf("render skill catalog snapshot: %w", err)
	}
	message := types.DeveloperMessage(rendered.Text, types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
		Revision: uint64(current.Revision),
	})
	return SkillCatalogPlan{
		Kind:          SkillCatalogPlanSnapshot,
		RebuildReason: reason,
		Message:       &message,
		Cursor: SkillCatalogCursor{
			ContextEpoch:         epoch,
			AnnouncedSnapshot:    current.Clone(),
			LedgerSnapshot:       current.Clone(),
			VisibleMessageDigest: skillCatalogMessageDigest(message),
		},
		Render: rendered,
	}, nil
}

func rebindVisibleSkillCatalogSnapshot(
	current skills.CatalogSnapshot,
	epoch SkillCatalogContextEpoch,
	messages []types.Message,
	charBudget int,
) (SkillCatalogPlan, bool, error) {
	for index := len(messages) - 1; index >= 0; index-- {
		visible := messages[index]
		if !visible.IsTrustedDeveloperMessage() || visible.DeveloperMetadata == nil {
			continue
		}
		switch visible.DeveloperMetadata.Kind {
		case types.DeveloperMessageKindSkillCatalogDelta:
			return SkillCatalogPlan{}, false, nil
		case types.DeveloperMessageKindSkillCatalogSnapshot:
			rendered, err := skills.RenderCatalogSnapshot(current, charBudget)
			if err != nil {
				return SkillCatalogPlan{}, false, fmt.Errorf("render restored skill catalog snapshot: %w", err)
			}
			want := types.DeveloperMessage(rendered.Text, types.DeveloperMessageMetadata{
				Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
				Revision: uint64(current.Revision),
			})
			if !sameSkillCatalogMessage(visible, want) {
				return SkillCatalogPlan{}, false, nil
			}
			return SkillCatalogPlan{
				Kind: SkillCatalogPlanNone,
				Cursor: SkillCatalogCursor{
					ContextEpoch:         epoch,
					AnnouncedSnapshot:    current.Clone(),
					LedgerSnapshot:       current.Clone(),
					VisibleMessageDigest: skillCatalogMessageDigest(visible),
				},
				Render: rendered,
			}, true, nil
		}
	}
	return SkillCatalogPlan{}, false, nil
}

func sameSkillCatalogMessage(got, want types.Message) bool {
	if !got.IsTrustedDeveloperMessage() ||
		got.Role != want.Role || got.IsMeta != want.IsMeta || got.GetText() != want.GetText() {
		return false
	}
	if got.DeveloperMetadata == nil || want.DeveloperMetadata == nil {
		return false
	}
	return *got.DeveloperMetadata == *want.DeveloperMetadata
}

func latestVisibleSkillCatalogState(messages []types.Message, scopes ...messagecontrol.Scope) (skills.CatalogRevision, string, bool) {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		trusted := message.IsTrustedDeveloperMessage()
		if len(scopes) == 1 {
			trusted = message.IsTrustedDeveloperMessageForScope(scopes[0], false)
		}
		if !trusted || message.DeveloperMetadata == nil {
			continue
		}
		switch message.DeveloperMetadata.Kind {
		case types.DeveloperMessageKindSkillCatalogSnapshot, types.DeveloperMessageKindSkillCatalogDelta:
			return skills.CatalogRevision(message.DeveloperMetadata.Revision), skillCatalogMessageDigest(message), true
		}
	}
	return 0, "", false
}

func skillCatalogMessageDigest(message types.Message) string {
	metadata := message.DeveloperMetadata
	if metadata == nil {
		return ""
	}
	payload := string(message.Role) + "\x00" + strconv.FormatBool(message.IsMeta) + "\x00" + string(metadata.Kind) + "\x00" + strconv.FormatUint(metadata.Revision, 10) + "\x00" + message.GetText()
	digest := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("sha256:%x", digest)
}

func validSkillCatalogMessageDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, char := range strings.TrimPrefix(value, "sha256:") {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
