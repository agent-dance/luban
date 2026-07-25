package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// querySkillCatalogPostCompactProvider is the cycle-free adapter between the
// compact package's attachment pipeline and the query loop's authoritative
// Manager plus context-epoch ledger.
type querySkillCatalogPostCompactProvider struct {
	query *QueryLoop
}

func (provider querySkillCatalogPostCompactProvider) PostCompactSkillAttachments(_ context.Context, _ compact.PostCompactAttachmentState) []types.Message {
	if provider.query == nil || provider.query.config.SkillManager == nil {
		return nil
	}
	var message types.Message
	err := provider.query.consumeSkillSnapshotForRun(func(snapshot skills.CatalogSnapshot) error {
		var renderErr error
		message, renderErr = postCompactCatalogSnapshotMessage(snapshot, provider.query.config.MaxContextTokens)
		return renderErr
	})
	if err != nil {
		// Optional attachment providers are best-effort. The lifecycle boundary
		// performs the same operation again and propagates an authoritative
		// snapshot failure before installing the replacement history.
		return nil
	}
	// Exact bodies are attached at the loop lifecycle boundary, which can see
	// the complete flattened replacement and therefore distinguish retained
	// bodies from compacted-away bodies without rebudgeting the retained tail.
	return []types.Message{message}
}

func (q *QueryLoop) PostCompactAttachmentProvider() compact.PostCompactAttachmentProvider {
	if q == nil {
		return nil
	}
	cfg := q.config
	if cfg.PlanState == nil &&
		cfg.SkillManager == nil &&
		cfg.BackgroundTasks == nil &&
		cfg.MCPState == nil &&
		cfg.AgentDefinitions == nil &&
		q.registry == nil {
		return nil
	}
	provider := &compact.RuntimeAttachmentProvider{
		PlanState:        cfg.PlanState,
		BackgroundTasks:  cfg.BackgroundTasks,
		MCPState:         cfg.MCPState,
		AgentDefinitions: cfg.AgentDefinitions,
		SessionID:        cfg.SessionID,
		CWD:              cfg.CWD,
		DeferredToolNames: func() []string {
			if q.registry == nil {
				return nil
			}
			tools := q.registry.DeferredTools()
			names := make([]string, 0, len(tools))
			for _, tool := range tools {
				if tool != nil {
					names = append(names, tool.Name())
				}
			}
			sort.Strings(names)
			return names
		},
		LoadedToolNames: func() []string {
			names := make([]string, 0, len(q.loadedToolNames))
			for name := range q.loadedToolNames {
				names = append(names, name)
			}
			sort.Strings(names)
			return names
		},
	}
	if cfg.SkillManager != nil {
		provider.SkillCatalog = querySkillCatalogPostCompactProvider{query: q}
	}
	return provider
}

func (q *QueryLoop) postCompactAttachmentProvider() compact.PostCompactAttachmentProvider {
	return q.PostCompactAttachmentProvider()
}

type postCompactSkillBody struct {
	ID       skills.SkillID
	Encoded  string
	Envelope visibleSkillInvocationEnvelope
	Order    int
}

const (
	postCompactSkillBodyMessageIDPrefix       = "skill-body:"
	postCompactSkillBodyToolResultMetadataKey = "luban_post_compact_skill_body"
)

func (q *QueryLoop) postCompactSkillAttachmentsWithSnapshot(
	original, replacement []types.Message,
	snapshot skills.CatalogSnapshot,
	sourceEpoch uint64,
	targetEpoch uint64,
) ([]types.Message, error) {
	// A persisted transcript may retain older compact epochs before the latest
	// boundary. Those messages are not model-visible in the source epoch and
	// therefore cannot authorize body reattachment.
	original = compact.GetMessagesAfterCompactBoundaryForScope(original, q.internalControlScope)

	var attachments []types.Message
	if !latestPostCompactCatalogIsCurrent(replacement, snapshot, q.config.MaxContextTokens) {
		message, renderErr := postCompactCatalogSnapshotMessage(snapshot, q.config.MaxContextTokens)
		if renderErr != nil {
			return nil, renderErr
		}
		attachments = append(attachments, q.sealRuntimeControlMessage(message))
	}

	// original is the loop-owned, currently visible source history. It may carry
	// an explicit user invocation emitted by the runtime before provenance IDs
	// were assigned. replacement is compactor/provider output and must already
	// be provenance-bound by the sanitizer before it can count as retained.
	latestOriginal := latestAuthorizedPostCompactSkillBodies(original, snapshot, sourceEpoch, true, true, q.internalControlScope)
	visible := latestAuthorizedPostCompactSkillBodies(replacement, snapshot, sourceEpoch, false, false, q.internalControlScope)
	visibleExact := make(map[string]struct{}, len(visible))
	for _, candidate := range visible {
		visibleExact[candidate.Encoded] = struct{}{}
	}

	candidates := make([]postCompactSkillBody, 0, len(latestOriginal))
	for _, candidate := range latestOriginal {
		if _, retained := visibleExact[candidate.Encoded]; retained {
			continue
		}
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Order != candidates[j].Order {
			return candidates[i].Order > candidates[j].Order
		}
		return candidates[i].ID < candidates[j].ID
	})

	remaining := compact.PostCompactSkillBodyBudgetBytes
	selected := make([]postCompactSkillBody, 0, min(len(candidates), compact.PostCompactMaxSkillBodies))
	for _, candidate := range candidates {
		if len(selected) >= compact.PostCompactMaxSkillBodies {
			break
		}
		size := len(candidate.Encoded)
		if size > remaining {
			continue
		}
		remaining -= size
		selected = append(selected, candidate)
	}
	// The selected set is recency-prioritized, while emitted order follows the
	// original conversation so multiple retained bodies remain deterministic.
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Order != selected[j].Order {
			return selected[i].Order < selected[j].Order
		}
		return selected[i].ID < selected[j].ID
	})
	for _, candidate := range selected {
		message, messageErr := newPostCompactSkillBodyMessage(candidate.Encoded, targetEpoch)
		if messageErr != nil {
			return nil, messageErr
		}
		attachments = append(attachments, q.sealRuntimeControlMessage(message))
	}
	return attachments, nil
}

// preparePostCompactSkillHistory appends any current snapshot/body attachments
// not already present.
func (q *QueryLoop) preparePostCompactSkillHistory(original, replacement []types.Message) ([]types.Message, error) {
	if q == nil || q.config.SkillManager == nil {
		return replacement, nil
	}
	var prepared []types.Message
	sourceEpoch := q.currentSkillCatalogEpoch()
	targetEpoch := q.nextSkillCatalogEpoch()
	err := q.consumeSkillSnapshotForRun(func(snapshot skills.CatalogSnapshot) error {
		var prepareErr error
		prepared, prepareErr = q.preparePostCompactSkillHistoryWithSnapshot(original, replacement, snapshot, sourceEpoch, targetEpoch)
		return prepareErr
	})
	return prepared, err
}

func (q *QueryLoop) preparePostCompactSkillHistoryWithSnapshot(
	original, replacement []types.Message,
	snapshot skills.CatalogSnapshot,
	sourceEpoch uint64,
	targetEpoch uint64,
) ([]types.Message, error) {
	trustedOriginal := latestAuthorizedPostCompactSkillBodies(
		compact.GetMessagesAfterCompactBoundaryForScope(original, q.internalControlScope), snapshot, sourceEpoch, true, true, q.internalControlScope,
	)
	cleaned := sanitizeInstalledPostCompactSkillHistory(
		replacement, snapshot, sourceEpoch, targetEpoch, q.internalControlScope, trustedOriginal, q.sealRuntimeControlMessage,
	)
	attachments, err := q.postCompactSkillAttachmentsWithSnapshot(original, cleaned, snapshot, sourceEpoch, targetEpoch)
	if err != nil {
		return nil, err
	}
	insertAt := trailingPlainUserInputIndex(cleaned)
	if insertAt < 0 {
		return retargetPostCompactSkillBodyMessages(
			append(cleaned, attachments...), sourceEpoch, targetEpoch, q.internalControlScope, q.sealRuntimeControlMessage,
		), nil
	}
	prepared := make([]types.Message, 0, len(cleaned)+len(attachments))
	prepared = append(prepared, cleaned[:insertAt]...)
	prepared = append(prepared, attachments...)
	prepared = append(prepared, cleaned[insertAt:]...)
	return retargetPostCompactSkillBodyMessages(
		prepared, sourceEpoch, targetEpoch, q.internalControlScope, q.sealRuntimeControlMessage,
	), nil
}

// stripPostCompactSkillProviderAttachments removes skill projection messages
// specifically from the CompactionResult.Attachments segment. MessagesToKeep
// is deliberately untouched: an exact body in the preserved tail is real
// model-visible evidence and must not be subjected to the reattachment budget.
// The lifecycle later rebuilds the current snapshot and lost bodies from the
// authoritative Manager plus pre-replacement visible history.
func stripPostCompactSkillProviderAttachments(result *compact.CompactionResult) bool {
	if result == nil {
		return false
	}
	// PreparedMessages is written only after this sanitization boundary. A
	// compactor-supplied override would otherwise bypass segment provenance.
	changed := result.PreparedMessages != nil
	result.PreparedMessages = nil
	if len(result.Attachments) == 0 {
		return changed
	}
	filtered := make([]types.Message, 0, len(result.Attachments))
	for _, message := range result.Attachments {
		if isPostCompactSkillProviderAttachment(message) {
			continue
		}
		filtered = append(filtered, message)
	}
	changed = changed || len(filtered) != len(result.Attachments)
	result.Attachments = filtered
	return changed
}

func isPostCompactSkillProviderAttachment(message types.Message) bool {
	if message.IsTrustedDeveloperMessage() && message.DeveloperMetadata != nil {
		switch message.DeveloperMetadata.Kind {
		case types.DeveloperMessageKindSkillCatalogSnapshot, types.DeveloperMessageKindSkillCatalogDelta:
			return true
		}
	}
	if message.Role != types.RoleUser || message.IsInternalRuntimeMessage() {
		return false
	}
	for _, block := range message.Content {
		switch typed := block.(type) {
		case types.TextBlock:
			if looksLikeSkillInvocationEnvelope(typed.Text) {
				return true
			}
		case types.ToolResultBlock:
			if looksLikeSkillInvocationEnvelope(typed.TextContent()) {
				return true
			}
		}
	}
	return false
}

func looksLikeSkillInvocationEnvelope(encoded string) bool {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return false
	}
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(trimmed), &header); err == nil && header.Type == "skill_invocation" {
		return true
	}
	// Fail closed for truncated/otherwise malformed provider attachments that
	// still carry the protocol discriminator.
	return strings.Contains(trimmed, `"type"`) && strings.Contains(trimmed, `"skill_invocation"`)
}

// installPostCompactVisibleHistory is the compaction/recovery epoch fence. It
// clears Responses chaining through task_19's installVisibleHistory helper,
// preserves SessionID/PromptCacheKey, and rebuilds context-bound ledgers only
// from messages that are actually installed.
func (q *QueryLoop) installPostCompactVisibleHistory(original, replacement []types.Message) ([]types.Message, error) {
	if q == nil || q.config.SkillManager == nil {
		installed := q.installVisibleHistory(replacement)
		return installed, nil
	}
	var installed []types.Message
	sourceEpoch := q.currentSkillCatalogEpoch()
	targetEpoch := q.nextSkillCatalogEpoch()
	err := q.consumeSkillSnapshotForRun(func(snapshot skills.CatalogSnapshot) error {
		prepared, prepareErr := q.preparePostCompactSkillHistoryWithSnapshot(original, replacement, snapshot, sourceEpoch, targetEpoch)
		if prepareErr != nil {
			return prepareErr
		}
		installed = q.installVisibleHistory(prepared)
		if q.currentSkillCatalogEpoch() != targetEpoch {
			return i18n.NewError(i18n.KeyLoopPostCompactSkillCatalogEpochChanged)
		}
		return q.rebuildPostCompactSkillStateWithSnapshot(installed, snapshot)
	})
	return installed, err
}

// ensurePostCompactSkillState is used after manual compaction has already
// crossed installVisibleHistory in query.go. It adds a missing current snapshot
// defensively, then reconstructs both cursor and loaded-body evidence.
func (q *QueryLoop) ensurePostCompactSkillState(messages []types.Message) ([]types.Message, error) {
	if q == nil || q.config.SkillManager == nil {
		return messages, nil
	}
	var prepared []types.Message
	epoch := q.currentSkillCatalogEpoch()
	err := q.consumeSkillSnapshotForRun(func(snapshot skills.CatalogSnapshot) error {
		var prepareErr error
		// messages is the already-installed output of the first lifecycle
		// sanitizer. Reusing it as trusted source avoids treating the public
		// provenance marker as an authentication token during cleanup.
		prepared, prepareErr = q.preparePostCompactSkillHistoryWithSnapshot(messages, messages, snapshot, epoch, epoch)
		if prepareErr != nil {
			return prepareErr
		}
		return q.rebuildPostCompactSkillStateWithSnapshot(prepared, snapshot)
	})
	return prepared, err
}

func (q *QueryLoop) consumeSkillSnapshotForRun(consume func(skills.CatalogSnapshot) error) error {
	if q == nil || q.config.SkillManager == nil {
		return nil
	}
	q.skillRunGenerationMu.RLock()
	generation := q.skillRunGeneration
	q.skillRunGenerationMu.RUnlock()
	if generation == 0 {
		generation = q.config.SkillProjectGeneration
	}
	if generation != 0 {
		return q.config.SkillManager.ConsumeSnapshotAtGeneration(q.config.SessionID, generation, consume)
	}
	// Manual compaction runs outside a sampling lease. Bind the current project
	// authority first, then hold that exact generation through the callback.
	binding, err := q.config.SkillManager.SnapshotBinding(q.config.SessionID)
	if err != nil {
		return err
	}
	return q.config.SkillManager.ConsumeSnapshotAtGeneration(q.config.SessionID, binding.ProjectGeneration, consume)
}

func (q *QueryLoop) rebuildPostCompactSkillStateWithSnapshot(messages []types.Message, snapshot skills.CatalogSnapshot) error {
	catalogMessage, found := latestCurrentPostCompactCatalog(messages, snapshot, q.config.MaxContextTokens)
	if !found {
		return i18n.NewError(i18n.KeyLoopPostCompactSkillCatalogMissing)
	}
	bodies := latestAuthorizedPostCompactSkillBodies(
		messages, snapshot, q.currentSkillCatalogEpoch(), false, false, q.internalControlScope,
	)

	q.skillCatalogMu.Lock()
	q.ensureSkillCatalogEpochLocked()
	epoch := q.skillCatalogEpoch
	q.skillCatalogCursor = SkillCatalogCursor{
		ContextEpoch:         skillCatalogContextEpoch(epoch),
		AnnouncedSnapshot:    snapshot.Clone(),
		LedgerSnapshot:       snapshot.Clone(),
		VisibleMessageDigest: skillCatalogMessageDigest(catalogMessage),
	}
	q.loadedSkillDigests = make(map[skills.SkillID]SkillLoadedLedgerEntry, len(bodies))
	for id, candidate := range bodies {
		q.loadedSkillDigests[id] = SkillLoadedLedgerEntry{
			ContentDigest: candidate.Envelope.Skill.Digest,
			PayloadDigest: candidate.Envelope.PayloadDigest,
		}
	}
	q.skillCatalogMu.Unlock()
	return nil
}

func (q *QueryLoop) currentSkillCatalogEpoch() uint64 {
	q.skillCatalogMu.Lock()
	defer q.skillCatalogMu.Unlock()
	q.ensureSkillCatalogEpochLocked()
	return q.skillCatalogEpoch
}

func (q *QueryLoop) nextSkillCatalogEpoch() uint64 {
	epoch := q.currentSkillCatalogEpoch() + 1
	if epoch == 0 {
		return 1
	}
	return epoch
}

func newPostCompactSkillBodyMessage(encoded string, contextEpoch uint64) (types.Message, error) {
	if contextEpoch == 0 {
		return types.Message{}, i18n.NewError(i18n.KeyLoopPostCompactSkillBodyEpochMissing)
	}
	candidate, err := decodePostCompactSkillBody(encoded, 0)
	if err != nil {
		return types.Message{}, err
	}
	message := types.UserMessage(encoded)
	message.ID = fmt.Sprintf("%s%d:%s", postCompactSkillBodyMessageIDPrefix, contextEpoch, candidate.Envelope.PayloadDigest)
	message.InternalKind = types.InternalMessageKindSkillInvocation
	return message.WithInternalControlProvenance(messagecontrol.Runtime()), nil
}

func validPostCompactSkillBodyMessageProvenance(message types.Message, candidate postCompactSkillBody, expectedEpoch uint64) bool {
	return message.IsTrustedSkillInvocationMessage() && validPostCompactSkillBodyProvenanceValue(message.ID, candidate, expectedEpoch)
}

func validPostCompactSkillToolResultProvenance(result types.ToolResultBlock, candidate postCompactSkillBody, expectedEpoch uint64) bool {
	return validPostCompactSkillBodyProvenanceValue(result.Metadata[postCompactSkillBodyToolResultMetadataKey], candidate, expectedEpoch)
}

func validPostCompactSkillBodyProvenanceValue(value string, candidate postCompactSkillBody, expectedEpoch uint64) bool {
	if expectedEpoch == 0 || !strings.HasPrefix(value, postCompactSkillBodyMessageIDPrefix) {
		return false
	}
	rest := strings.TrimPrefix(value, postCompactSkillBodyMessageIDPrefix)
	separator := strings.IndexByte(rest, ':')
	if separator <= 0 {
		return false
	}
	epoch, err := strconv.ParseUint(rest[:separator], 10, 64)
	if err != nil || epoch != expectedEpoch {
		return false
	}
	return rest[separator+1:] == string(candidate.Envelope.PayloadDigest)
}

func bindPostCompactSkillToolResultProvenance(result types.ToolResultBlock, candidate postCompactSkillBody, epoch uint64) types.ToolResultBlock {
	metadata := make(map[string]string, len(result.Metadata)+1)
	for key, value := range result.Metadata {
		metadata[key] = value
	}
	metadata[postCompactSkillBodyToolResultMetadataKey] = fmt.Sprintf(
		"%s%d:%s", postCompactSkillBodyMessageIDPrefix, epoch, candidate.Envelope.PayloadDigest,
	)
	result.Metadata = metadata
	return result
}

// sanitizeInstalledPostCompactSkillHistory discards prior catalog projections
// and standalone envelope-like messages that are malformed, unprovenanced, or
// no longer authorized by the transaction snapshot. Paired ToolResult bodies
// are preserved to maintain provider invariants; ledger reconstruction still
// validates them against the same snapshot.
func sanitizeInstalledPostCompactSkillHistory(
	messages []types.Message,
	snapshot skills.CatalogSnapshot,
	expectedEpoch uint64,
	targetEpoch uint64,
	controlScope messagecontrol.Scope,
	trustedOriginal map[skills.SkillID]postCompactSkillBody,
	seal func(types.Message) types.Message,
) []types.Message {
	latest := latestAuthorizedPostCompactSkillBodies(messages, snapshot, expectedEpoch, false, false, controlScope)
	allowedStandalone := make(map[string]struct{}, len(latest))
	allowedPaired := make(map[string]struct{}, len(latest))
	for id, candidate := range latest {
		trusted, trustedMatch := trustedOriginal[id]
		if trustedMatch && trusted.Encoded == candidate.Encoded {
			allowedStandalone[candidate.Encoded] = struct{}{}
			allowedPaired[candidate.Encoded] = struct{}{}
			continue
		}
	}
	out := make([]types.Message, 0, len(messages))
	for _, message := range messages {
		if message.IsTrustedDeveloperMessage() && message.DeveloperMetadata != nil {
			switch message.DeveloperMetadata.Kind {
			case types.DeveloperMessageKindSkillCatalogSnapshot, types.DeveloperMessageKindSkillCatalogDelta:
				continue
			}
		}
		if message.Role == types.RoleUser && !message.IsInternalRuntimeMessage() && len(message.Content) == 1 {
			if text, ok := message.Content[0].(types.TextBlock); ok && looksLikeSkillInvocationEnvelope(text.Text) {
				candidate, err := decodePostCompactSkillBody(text.Text, 0)
				if err != nil {
					continue
				}
				if _, allowed := allowedStandalone[candidate.Encoded]; !allowed {
					continue
				}
				bound, bindErr := newPostCompactSkillBodyMessage(candidate.Encoded, targetEpoch)
				if bindErr != nil {
					continue
				}
				message = seal(bound)
			}
		}
		if message.Role == types.RoleUser && !message.IsInternalRuntimeMessage() {
			content := append([]types.ContentBlock(nil), message.Content...)
			changed := false
			for index, block := range content {
				result, ok := block.(types.ToolResultBlock)
				if !ok || !looksLikeSkillInvocationEnvelope(result.TextContent()) {
					continue
				}
				candidate, decodeErr := decodePostCompactSkillBody(result.Content, 0)
				_, allowed := allowedPaired[candidate.Encoded]
				if decodeErr == nil && allowed {
					content[index] = bindPostCompactSkillToolResultProvenance(result, candidate, expectedEpoch)
					changed = true
					continue
				}
				// Preserve the tool_result block and ID for provider pairing, but
				// remove untrusted envelope bytes and receipts so it cannot rebuild
				// loaded-body evidence.
				result.Content = ""
				result.ContentBlocks = nil
				result.Metadata = nil
				content[index] = result
				changed = true
			}
			if changed {
				message.Content = content
			}
		}
		out = append(out, message)
	}
	return out
}

// retargetPostCompactSkillBodyMessages binds every trusted standalone body to
// the epoch being installed. This prevents a transcript marker from one
// replacement generation from being replayed as loaded-body evidence in a
// later generation.
func retargetPostCompactSkillBodyMessages(
	messages []types.Message,
	sourceEpoch, targetEpoch uint64,
	controlScope messagecontrol.Scope,
	seal func(types.Message) types.Message,
) []types.Message {
	if targetEpoch == 0 {
		return messages
	}
	out := append([]types.Message(nil), messages...)
	for index, message := range out {
		if message.Role != types.RoleUser || message.IsInternalRuntimeMessage() {
			continue
		}
		if len(message.Content) == 1 && message.IsTrustedSkillInvocationMessageForScope(controlScope) {
			if text, ok := message.Content[0].(types.TextBlock); ok {
				candidate, err := decodePostCompactSkillBody(text.Text, 0)
				if err == nil && (validPostCompactSkillBodyMessageProvenance(message, candidate, sourceEpoch) ||
					validPostCompactSkillBodyMessageProvenance(message, candidate, targetEpoch)) {
					if retargeted, retargetErr := newPostCompactSkillBodyMessage(candidate.Encoded, targetEpoch); retargetErr == nil {
						out[index] = seal(retargeted)
						continue
					}
				}
			}
		}
		content := append([]types.ContentBlock(nil), message.Content...)
		changed := false
		for blockIndex, block := range content {
			result, ok := block.(types.ToolResultBlock)
			if !ok {
				continue
			}
			candidate, err := decodePostCompactSkillBody(result.Content, 0)
			if err != nil || (!validPostCompactSkillToolResultProvenance(result, candidate, sourceEpoch) &&
				!validPostCompactSkillToolResultProvenance(result, candidate, targetEpoch)) {
				continue
			}
			content[blockIndex] = bindPostCompactSkillToolResultProvenance(result, candidate, targetEpoch)
			changed = true
		}
		if changed {
			message.Content = content
			out[index] = message
		}
	}
	return out
}

func postCompactCatalogSnapshotMessage(snapshot skills.CatalogSnapshot, maxContextTokens int) (types.Message, error) {
	rendered, err := skills.RenderCatalogSnapshot(snapshot, skills.GetCharBudget(maxContextTokens))
	if err != nil {
		return types.Message{}, err
	}
	return types.DeveloperMessage(rendered.Text, types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
		Revision: uint64(snapshot.Revision),
	}), nil
}

func latestPostCompactCatalogIsCurrent(messages []types.Message, snapshot skills.CatalogSnapshot, maxContextTokens int) bool {
	_, found := latestCurrentPostCompactCatalog(messages, snapshot, maxContextTokens)
	return found
}

func latestCurrentPostCompactCatalog(messages []types.Message, snapshot skills.CatalogSnapshot, maxContextTokens int) (types.Message, bool) {
	want, err := postCompactCatalogSnapshotMessage(snapshot, maxContextTokens)
	if err != nil {
		return types.Message{}, false
	}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if !message.IsTrustedDeveloperMessage() || message.DeveloperMetadata == nil {
			continue
		}
		switch message.DeveloperMetadata.Kind {
		case types.DeveloperMessageKindSkillCatalogSnapshot, types.DeveloperMessageKindSkillCatalogDelta:
			return message, samePostCompactCatalogMessage(message, want)
		}
	}
	return types.Message{}, false
}

func samePostCompactCatalogMessage(got, want types.Message) bool {
	return sameSkillCatalogMessage(got, want)
}

func latestAuthorizedPostCompactSkillBodies(
	messages []types.Message,
	snapshot skills.CatalogSnapshot,
	standaloneEpoch uint64,
	allowUnprovenancedStandalone bool,
	allowUnprovenancedPaired bool,
	controlScopes ...messagecontrol.Scope,
) map[skills.SkillID]postCompactSkillBody {
	latest := latestStrictPostCompactSkillBodies(
		messages, standaloneEpoch, allowUnprovenancedStandalone, allowUnprovenancedPaired, controlScopes...,
	)
	for id, candidate := range latest {
		row, found := snapshot.Find(id)
		if !found || row.Digest != candidate.Envelope.Skill.Digest ||
			row.Name != candidate.Envelope.Skill.Name || row.Source != candidate.Envelope.Skill.Source ||
			row.Locator != candidate.Envelope.Skill.Locator || !row.Executable ||
			row.Visibility == skills.VisibilityOff || row.ShadowedBy != "" {
			delete(latest, id)
		}
	}
	return latest
}

// postCompactSkillBodyProjectionLost detects the one aggregate-budget case
// that must become a real lifecycle replacement: an exact body visible before
// provider projection is absent or superseded afterwards. Ordinary local
// bookkeeping removal and Anthropic cache_edits do not cross the epoch fence.
func postCompactSkillBodyProjectionLost(before, after []types.Message) bool {
	visibleBefore := latestStrictPostCompactSkillBodies(before, 0, true, true)
	if len(visibleBefore) == 0 {
		return false
	}
	visibleAfter := latestStrictPostCompactSkillBodies(after, 0, true, true)
	for id, candidate := range visibleBefore {
		retained, found := visibleAfter[id]
		if !found || retained.Encoded != candidate.Encoded {
			return true
		}
	}
	return false
}

func latestStrictPostCompactSkillBodies(
	messages []types.Message,
	standaloneEpoch uint64,
	allowUnprovenancedEvidence bool,
	allowUnprovenancedPaired bool,
	controlScopes ...messagecontrol.Scope,
) map[skills.SkillID]postCompactSkillBody {
	type toolUseEvidence struct {
		selector     string
		messageIndex int
		unique       bool
	}
	skillToolUses := make(map[string]toolUseEvidence)
	for messageIndex, message := range messages {
		if message.Role != types.RoleAssistant || message.IsInternalRuntimeMessage() {
			continue
		}
		for _, use := range message.GetToolUses() {
			if use.Name != "Skill" || strings.TrimSpace(use.ID) == "" {
				continue
			}
			selector, selectorOK := use.Input["skill"].(string)
			selector = strings.TrimSpace(selector)
			selectorOK = selectorOK && selector != ""
			if existing, duplicate := skillToolUses[use.ID]; duplicate {
				existing.unique = false
				skillToolUses[use.ID] = existing
				continue
			}
			skillToolUses[use.ID] = toolUseEvidence{
				selector: selector, messageIndex: messageIndex, unique: selectorOK,
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
				candidate, decodeErr := decodePostCompactSkillBody(text.Text, order)
				authorized := validPostCompactSkillBodyMessageProvenance(message, candidate, standaloneEpoch)
				if authorized && len(controlScopes) == 1 {
					authorized = message.IsTrustedSkillInvocationMessageForScope(controlScopes[0])
				}
				if !authorized && allowUnprovenancedEvidence && !message.HasInternalControlProvenance() {
					authorized = true
				}
				if decodeErr == nil && authorized {
					latest[candidate.ID] = candidate
					order++
				}
			}
		}
		for _, block := range message.Content {
			result, ok := block.(types.ToolResultBlock)
			if !ok || result.IsError || result.Outcome != types.ToolOutcomeSucceeded || len(result.ContentBlocks) != 0 {
				continue
			}
			evidence, paired := skillToolUses[result.ToolUseID]
			if !paired || !evidence.unique || evidence.messageIndex >= messageIndex {
				continue
			}
			candidate, decodeErr := decodePostCompactSkillBody(result.Content, order)
			if decodeErr != nil || (!allowUnprovenancedPaired &&
				!validPostCompactSkillToolResultProvenance(result, candidate, standaloneEpoch)) ||
				!skillSelectorMatchesEnvelope(evidence.selector, candidate) {
				continue
			}
			latest[candidate.ID] = candidate
			order++
		}
	}
	return latest
}

func skillSelectorMatchesEnvelope(selector string, candidate postCompactSkillBody) bool {
	selector = strings.TrimSpace(selector)
	if selector == string(candidate.ID) {
		return true
	}
	selector = strings.TrimPrefix(selector, "/")
	return selector != "" && selector == candidate.Envelope.Skill.Name
}

func decodePostCompactSkillBody(encoded string, order int) (postCompactSkillBody, error) {
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var envelope visibleSkillInvocationEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return postCompactSkillBody{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return postCompactSkillBody{}, i18n.NewError(i18n.KeyLoopPostCompactSkillEnvelopeTrailing)
		}
		return postCompactSkillBody{}, err
	}
	if envelope.Kind != skills.InvocationEnvelopeFull && envelope.Kind != skills.InvocationEnvelopeSuperseding {
		return postCompactSkillBody{}, i18n.NewError(i18n.KeyLoopPostCompactSkillEnvelopeNoBody)
	}
	receipt := skills.SkillExecutionReceipt{
		ContextEpoch:            1, // structural validation only; the old epoch is intentionally discarded
		SkillID:                 envelope.Skill.ID,
		ContentDigest:           envelope.Skill.Digest,
		InvocationPayloadDigest: envelope.PayloadDigest,
		InvocationEnvelopeKind:  envelope.Kind,
	}
	if err := validateVisibleSkillInvocationEnvelope(encoded, receipt); err != nil {
		return postCompactSkillBody{}, err
	}
	return postCompactSkillBody{
		ID: envelope.Skill.ID, Encoded: encoded, Envelope: envelope, Order: order,
	}, nil
}

var _ compact.SkillCatalogPostCompactProvider = querySkillCatalogPostCompactProvider{}
