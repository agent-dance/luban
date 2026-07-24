package loop

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// skillLoadedLedgerCapability binds a tool execution to this QueryLoop and to
// the exact model-visible history captured at the execution boundary. Cloning
// here (rather than closing over the caller's slice) makes the capability safe
// to invoke concurrently after the query goroutine advances its local state.
func (q *QueryLoop) skillLoadedLedgerCapability(messages []types.Message) func(skills.SkillID) SkillLoadedLedgerState {
	visible := cloneMessages(messages)
	return func(id skills.SkillID) SkillLoadedLedgerState {
		return q.ResolveSkillLoadedLedger(visible, id)
	}
}

// ResolveSkillLoadedLedger reconciles one stable skill ID against the exact
// model-visible message snapshot supplied by the invocation boundary. It never
// calls Manager: SkillTool invokes this method while ResolveLatest holds the
// Manager read transaction, where recursive Manager access could deadlock
// behind a queued writer.
//
// Callers must pass committed visible history: model calls use the immutable
// ToolExecutionContext messages and explicit-user calls use an idle
// QueryLoop.Messages snapshot. A pending UI ToolResult must not be included.
func (q *QueryLoop) ResolveSkillLoadedLedger(messages []types.Message, id skills.SkillID) SkillLoadedLedgerState {
	if q == nil {
		return SkillLoadedLedgerState{}
	}

	q.skillCatalogMu.Lock()
	q.ensureSkillCatalogEpochLocked()
	epoch := q.skillCatalogEpoch
	cursor := q.skillCatalogCursor.Clone()
	q.skillCatalogMu.Unlock()

	state := SkillLoadedLedgerState{ContextEpoch: epoch}
	if err := id.Validate(); err != nil {
		return state
	}

	segment := compact.GetMessagesAfterCompactBoundaryForScope(messages, q.internalControlScope, true)
	cursorCurrent := visibleSkillCatalogCursorCurrent(segment, cursor, epoch, q.internalControlScope)
	candidate, found := latestVisibleSkillBodyForID(segment, id, epoch, q.internalControlScope)
	if found && (!cursor.Empty() && !cursorCurrent ||
		!visibleSkillBodyAuthorizedByCursor(candidate, cursor)) {
		found = false
	}

	q.skillCatalogMu.Lock()
	defer q.skillCatalogMu.Unlock()
	q.ensureSkillCatalogEpochLocked()
	if q.skillCatalogEpoch != epoch || !reflect.DeepEqual(q.skillCatalogCursor, cursor) {
		return SkillLoadedLedgerState{ContextEpoch: q.skillCatalogEpoch}
	}
	if !found {
		delete(q.loadedSkillDigests, id)
		return state
	}

	entry := SkillLoadedLedgerEntry{
		ContentDigest: candidate.Envelope.Skill.Digest,
		PayloadDigest: candidate.Envelope.PayloadDigest,
	}
	q.loadedSkillDigests[id] = entry
	state.LoadedContextEpoch = epoch
	state.ContentDigest = entry.ContentDigest
	state.PayloadDigest = entry.PayloadDigest
	return state
}

// VisibleSkillCatalogState returns the current runtime ledger after a
// conservative, read-only reconstruction from committed visible history.
// Missing or mismatched catalog evidence clears the cursor so the next sample
// emits a full snapshot. Exact bodies are rebuilt even when the live ledger has
// not consumed them yet, which lets explicit-user envelopes persist on save.
func (q *QueryLoop) VisibleSkillCatalogState(messages []types.Message) SkillCatalogRuntimeState {
	if q == nil {
		return SkillCatalogRuntimeState{}
	}
	q.skillCatalogMu.Lock()
	q.ensureSkillCatalogEpochLocked()
	persisted := q.skillCatalogStateLocked()
	q.skillCatalogMu.Unlock()

	reconciled := q.ReconcileVisibleSkillCatalogState(messages, persisted)
	if !persisted.Cursor.Empty() && reconciled.Cursor.Empty() {
		// A cursor mismatch means the caller did not supply the model-visible
		// history represented by this runtime. Do not manufacture loaded evidence
		// from a different or partial transcript.
		reconciled.LoadedDigests = nil
		return q.confirmVisibleSkillCatalogState(persisted, reconciled)
	}
	segment := compact.GetMessagesAfterCompactBoundaryForScope(messages, q.internalControlScope, true)
	for id, candidate := range latestStrictVisibleSkillBodies(segment, persisted.ContextEpoch, q.internalControlScope) {
		if !reconciled.Cursor.Empty() && !visibleSkillBodyAuthorizedByCursor(candidate, reconciled.Cursor) {
			continue
		}
		if reconciled.LoadedDigests == nil {
			reconciled.LoadedDigests = make(map[skills.SkillID]SkillLoadedLedgerEntry)
		}
		reconciled.LoadedDigests[id] = SkillLoadedLedgerEntry{
			ContentDigest: candidate.Envelope.Skill.Digest,
			PayloadDigest: candidate.Envelope.PayloadDigest,
		}
	}
	return q.confirmVisibleSkillCatalogState(persisted, reconciled)
}

func (q *QueryLoop) confirmVisibleSkillCatalogState(snapshot, reconstructed SkillCatalogRuntimeState) SkillCatalogRuntimeState {
	q.skillCatalogMu.Lock()
	defer q.skillCatalogMu.Unlock()
	q.ensureSkillCatalogEpochLocked()
	if q.skillCatalogEpoch != snapshot.ContextEpoch || !reflect.DeepEqual(q.skillCatalogCursor, snapshot.Cursor) {
		return SkillCatalogRuntimeState{ContextEpoch: q.skillCatalogEpoch}
	}
	return reconstructed
}

// ReconcileVisibleSkillCatalogState is retained for source compatibility, but
// without an authoritative message-control scope it cannot safely restore
// cursor or loaded-body evidence. Runtime/resume callers must use the
// QueryLoop method below after installing the exact session scope.
func ReconcileVisibleSkillCatalogState(messages []types.Message, persisted SkillCatalogRuntimeState) SkillCatalogRuntimeState {
	_ = messages
	reconciled := SkillCatalogRuntimeState{ContextEpoch: persisted.ContextEpoch}
	if persisted.Validate() != nil {
		return reconciled
	}
	return reconciled
}

// ReconcileVisibleSkillCatalogState validates persisted evidence against this
// loop's exact session/generation authority. Unbound and foreign process HMACs
// cannot become loaded ledger or catalog cursor state.
func (q *QueryLoop) ReconcileVisibleSkillCatalogState(messages []types.Message, persisted SkillCatalogRuntimeState) SkillCatalogRuntimeState {
	reconciled := SkillCatalogRuntimeState{ContextEpoch: persisted.ContextEpoch}
	if q == nil || !q.internalControlScope.Bound() || persisted.Validate() != nil {
		return reconciled
	}

	segment := compact.GetMessagesAfterCompactBoundaryForScope(messages, q.internalControlScope, false)
	if visibleSkillCatalogCursorCurrent(segment, persisted.Cursor, persisted.ContextEpoch, q.internalControlScope) {
		reconciled.Cursor = persisted.Cursor.Clone()
	}

	candidates := latestStrictVisibleSkillBodies(segment, persisted.ContextEpoch, q.internalControlScope)
	for id, entry := range persisted.LoadedDigests {
		candidate, found := candidates[id]
		if !found || candidate.Envelope.Skill.Digest != entry.ContentDigest ||
			candidate.Envelope.PayloadDigest != entry.PayloadDigest {
			continue
		}
		if !reconciled.Cursor.Empty() && !visibleSkillBodyAuthorizedByCursor(candidate, reconciled.Cursor) {
			continue
		}
		if reconciled.LoadedDigests == nil {
			reconciled.LoadedDigests = make(map[skills.SkillID]SkillLoadedLedgerEntry)
		}
		reconciled.LoadedDigests[id] = entry
	}
	return reconciled
}

func visibleSkillCatalogCursorCurrent(
	messages []types.Message,
	cursor SkillCatalogCursor,
	epoch uint64,
	scope messagecontrol.Scope,
) bool {
	if cursor.Empty() {
		return false
	}
	if cursor.Validate() != nil || cursor.ContextEpoch != skillCatalogContextEpoch(epoch) {
		return false
	}
	revision, digest, found := latestVisibleSkillCatalogState(messages, scope)
	return found && revision == cursor.AnnouncedRevision() && digest == cursor.VisibleMessageDigest
}

func visibleSkillBodyAuthorizedByCursor(candidate postCompactSkillBody, cursor SkillCatalogCursor) bool {
	if cursor.Empty() {
		return true
	}
	row, found := cursor.LedgerSnapshot.Find(candidate.ID)
	if !found {
		return false
	}
	envelope := candidate.Envelope.Skill
	return row.ID == envelope.ID && row.Name == envelope.Name && row.Revision == envelope.Revision &&
		row.Digest == envelope.Digest && row.Source == envelope.Source && row.Locator == envelope.Locator &&
		row.Executable && row.Visibility != skills.VisibilityOff && row.ShadowedBy == ""
}

func latestVisibleSkillBodyForID(
	messages []types.Message,
	id skills.SkillID,
	epoch uint64,
	scope messagecontrol.Scope,
) (postCompactSkillBody, bool) {
	candidate, found := latestStrictVisibleSkillBodies(messages, epoch, scope)[id]
	return candidate, found
}

func latestStrictVisibleSkillBodies(
	messages []types.Message,
	epoch uint64,
	scope messagecontrol.Scope,
) map[skills.SkillID]postCompactSkillBody {
	type toolUseEvidence struct {
		selector     string
		messageIndex int
		count        int
	}
	toolUses := make(map[string]toolUseEvidence)
	resultCounts := make(map[string]int)

	for messageIndex, message := range messages {
		if message.IsInternalRuntimeMessage() {
			continue
		}
		if message.Role == types.RoleAssistant {
			for _, use := range message.GetToolUses() {
				if strings.TrimSpace(use.ID) == "" {
					continue
				}
				evidence := toolUses[use.ID]
				evidence.count++
				if evidence.count == 1 && use.Name == "Skill" {
					evidence.messageIndex = messageIndex
					evidence.selector, _ = use.Input["skill"].(string)
					evidence.selector = strings.TrimSpace(evidence.selector)
				}
				toolUses[use.ID] = evidence
			}
		}
		if message.Role == types.RoleUser {
			for _, block := range message.Content {
				if result, ok := block.(types.ToolResultBlock); ok && strings.TrimSpace(result.ToolUseID) != "" {
					resultCounts[result.ToolUseID]++
				}
			}
		}
	}

	latest := make(map[skills.SkillID]postCompactSkillBody)
	order := 0
	for messageIndex, message := range messages {
		if message.Role != types.RoleUser || message.IsInternalRuntimeMessage() {
			continue
		}
		if len(message.Content) == 1 {
			if text, ok := message.Content[0].(types.TextBlock); ok {
				candidate, err := decodeCanonicalVisibleSkillBody(text.Text, order)
				if err == nil && validVisibleStandaloneSkillBody(message, candidate, epoch, scope) {
					latest[candidate.ID] = candidate
					order++
				}
			}
		}
		for _, block := range message.Content {
			result, ok := block.(types.ToolResultBlock)
			if !ok || result.IsError || result.Outcome != types.ToolOutcomeSucceeded ||
				len(result.ContentBlocks) != 0 || resultCounts[result.ToolUseID] != 1 {
				continue
			}
			evidence, paired := toolUses[result.ToolUseID]
			if !paired || evidence.count != 1 || evidence.selector == "" || evidence.messageIndex >= messageIndex {
				continue
			}
			candidate, err := decodeCanonicalVisibleSkillBody(result.Content, order)
			if err != nil || !skillSelectorMatchesEnvelope(evidence.selector, candidate) ||
				!validVisibleSkillToolResult(result, candidate, epoch) {
				continue
			}
			latest[candidate.ID] = candidate
			order++
		}
	}
	return latest
}

func validVisibleStandaloneSkillBody(
	message types.Message,
	candidate postCompactSkillBody,
	epoch uint64,
	scope messagecontrol.Scope,
) bool {
	if strings.HasPrefix(message.ID, postCompactSkillBodyMessageIDPrefix) {
		return validPostCompactSkillBodyMessageProvenance(message, candidate, epoch) &&
			message.IsTrustedSkillInvocationMessageForScope(scope, false)
	}
	return message.IsTrustedSkillInvocationMessageForScope(scope, false)
}

func validVisibleSkillToolResult(result types.ToolResultBlock, candidate postCompactSkillBody, epoch uint64) bool {
	if validPostCompactSkillToolResultProvenance(result, candidate, epoch) {
		return true
	}
	receipt, found, err := skills.DecodeSkillExecutionReceiptMetadata(result.Metadata)
	if err != nil || !found || receipt.ContextEpoch != epoch {
		return false
	}
	return validateVisibleSkillInvocationEnvelope(candidate.Encoded, receipt) == nil
}

func decodeCanonicalVisibleSkillBody(encoded string, order int) (postCompactSkillBody, error) {
	candidate, err := decodePostCompactSkillBody(encoded, order)
	if err != nil {
		return postCompactSkillBody{}, err
	}
	canonical, err := json.Marshal(candidate.Envelope)
	if err != nil || string(canonical) != encoded {
		return postCompactSkillBody{}, errNonCanonicalVisibleSkillBody
	}
	canonicalLocator, err := skills.CanonicalSkillLocator(candidate.Envelope.Skill.Source, string(candidate.Envelope.Skill.Locator))
	if err != nil || canonicalLocator != candidate.Envelope.Skill.Locator {
		return postCompactSkillBody{}, errInvalidVisibleSkillIdentity
	}
	derivedID, err := skills.ComputeSkillID(candidate.Envelope.Skill.Source, canonicalLocator)
	if err != nil || derivedID != candidate.ID {
		return postCompactSkillBody{}, errInvalidVisibleSkillIdentity
	}
	return candidate, nil
}

type visibleSkillBodyError string

func (err visibleSkillBodyError) Error() string { return string(err) }

const (
	errNonCanonicalVisibleSkillBody visibleSkillBodyError = "non-canonical visible skill body"
	errInvalidVisibleSkillIdentity  visibleSkillBodyError = "invalid visible skill identity"
)
