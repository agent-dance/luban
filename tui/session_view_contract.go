package tui

// SessionViewOwnership classifies every AppState surface reachable from the
// Root component. The registry is intentionally executable policy: a new Root
// read must be classified before tests accept it, preventing session-owned
// display state from silently bypassing DurableSessionView/checkpoint design.
type SessionViewOwnership string

const (
	SessionViewDurable       SessionViewOwnership = "durable"
	SessionViewRenderContext SessionViewOwnership = "render_context"
	SessionViewTransient     SessionViewOwnership = "transient"
	SessionViewIdentity      SessionViewOwnership = "identity"
	SessionViewMutation      SessionViewOwnership = "mutation"
	SessionViewRevision      SessionViewOwnership = "revision"
)

var rootSessionViewAccessContract = func() map[string]SessionViewOwnership {
	contract := make(map[string]SessionViewOwnership)
	register := func(owner SessionViewOwnership, names ...string) {
		for _, name := range names {
			if _, duplicate := contract[name]; duplicate {
				panic("duplicate Root session-view ownership: " + name)
			}
			contract[name] = owner
		}
	}
	register(SessionViewDurable,
		"ActiveSessionInteraction", "ActiveSessionUsage", "ActivityFocus", "ActivitySnapshot", "ActivityViewOffset",
		"CumulativeCost", "DecisionHistory", "DecisionReceipt", "ExpandedView", "GetActivity", "GetObservation", "Goal",
		"LastAssistantText", "MaxTokens", "Messages", "Mode", "Model", "PendingImageSelected", "PendingImages",
		"PinnedObservationSnapshot", "Provider", "ReadDetail", "SessionCacheCreateTokens", "SessionCacheReadAtCompact",
		"SessionCacheReadTokens", "SessionCompactionCount", "SessionCompletedRoundInputTokens", "SessionCompletedRoundOutputTokens",
		"SessionCostKnown", "SessionHasCompacted", "SessionInputTokens", "SessionInputTokensAtCompact", "SessionOutputTokens",
		"SessionRoundUsageKnown", "SessionTotalCacheCreateTokens", "SessionTotalCacheReadTokens", "SessionTotalInputTokens",
		"SessionTotalOutputTokens", "SessionUsageKnown", "SessionWebSearchRequests", "TaskViewItems", "ToolSegmentExpansion",
		"TranscriptShowAll", "UsedTokens", "toolSegmentExpansionOverride",
	)
	register(SessionViewRenderContext,
		"ContextEstimateComplete", "ContextMeasurement", "ContextWindowK", "Language", "ModelCanSeeImages", "ModelCostCurrency", "ModelCostIn", "ModelCostOut", "ProvStatus", "ReasoningEffort", "TermWidth", "Tools",
	)
	register(SessionViewTransient,
		"AskUserDraft", "DecisionReq", "DecisionResp", "DecisionSelected", "ForkPicker", "HasActiveQuery", "LLMCall", "ModelPicker", "PermReq", "PermResp", "PermSelected", "QueuedInputCount", "SessionPicker", "SkillsMenu", "TryCancelQuery",
	)
	register(SessionViewIdentity, "SessionID")
	register(SessionViewMutation,
		"AcknowledgeActivity", "AddPendingImage", "AppendMessage", "CycleObservationDisclosure", "RemovePendingImage",
		"RelocalizeToolPresentations",
		"SetExpandedView", "SetInteractionAnchor", "SetInteractionCursor", "SetInteractionEditor",
		"SetInteractionScroll", "SetInteractionSlash", "SetToolSegmentExpanded", "SetTranscriptShowAll", "bindBatch",
	)
	register(SessionViewRevision, "ActivityRevision", "InteractionRevision", "TaskListRevision", "ViewRevision")
	return contract
}()
