package i18n

// Semantic keys for the core TUI shell. The translations are initially shared
// with the legacy catalog while existing copy is migrated incrementally.
const (
	KeyInputPlaceholder        Key = "tui.input.placeholder"
	KeyPastedText              Key = "tui.input.pasted_text"
	KeyPromptHistoryNotSaved   Key = "tui.input.history_not_saved"
	KeyUserPrefix              Key = "tui.message.user_prefix"
	KeyImageAttachment         Key = "tui.message.image_attachment"
	KeyErrorPrefix             Key = "common.error_prefix"
	KeyModeAuto                Key = "mode.auto"
	KeyModeAsk                 Key = "mode.ask"
	KeyModePlan                Key = "mode.plan"
	KeyModeLabel               Key = "mode.label"
	KeyWebSearchCount          Key = "status.web_search_count"
	KeyShowAllEvidence         Key = "evidence.show_all"
	KeyGoalPrefix              Key = "goal.prefix"
	KeyGoalPausedPrefix        Key = "goal.paused_prefix"
	KeySlashCommandsTitle      Key = "slash_commands.title"
	KeyPermissionAllowOnce     Key = "permission.allow_once"
	KeyPermissionAlwaysAllow   Key = "permission.always_allow"
	KeyPermissionExecute       Key = "permission.execute"
	KeyPermissionStayInPlan    Key = "permission.stay_in_plan"
	KeyPermissionReject        Key = "permission.reject"
	KeyRiskMedium              Key = "risk.medium"
	KeyRiskHigh                Key = "risk.high"
	KeyPermissionDecision      Key = "permission.decision"
	KeyPlanDecision            Key = "plan.decision"
	KeyDecisionActor           Key = "decision.actor"
	KeyDecisionAgentSession    Key = "decision.agent_session"
	KeyDecisionAction          Key = "decision.action"
	KeyDecisionTarget          Key = "decision.target"
	KeyDecisionImpact          Key = "decision.impact"
	KeyDecisionRisk            Key = "decision.risk"
	KeyDecisionScope           Key = "decision.scope"
	KeyDecisionInput           Key = "decision.input"
	KeyDecisionAfterApproval   Key = "decision.after_approval"
	KeyGoodbye                 Key = "common.goodbye"
	KeyForkNoConversationTurns Key = "fork.no_conversation_turns"
)

func init() {
	registerLegacySemantic(map[Key]string{
		KeyInputPlaceholder:        "Type a message... (Ctrl+D to exit)",
		KeyPastedText:              "[Pasted text #%d +%d lines]",
		KeyPromptHistoryNotSaved:   "⚠ Prompt history not saved",
		KeyUserPrefix:              "You: ",
		KeyImageAttachment:         "📷 [Image #%d] (%s)",
		KeyErrorPrefix:             "Error: ",
		KeyModeAuto:                "Auto",
		KeyModeAsk:                 "Ask",
		KeyModePlan:                "Plan",
		KeyModeLabel:               "%s mode",
		KeyWebSearchCount:          "%d web search",
		KeyShowAllEvidence:         "show all evidence",
		KeyGoalPrefix:              "Goal: ",
		KeyGoalPausedPrefix:        "Goal paused: ",
		KeySlashCommandsTitle:      "Slash Commands — Up/Down move, Tab complete, Enter run, Esc close",
		KeyPermissionAllowOnce:     "Allow once",
		KeyPermissionAlwaysAllow:   "Always allow",
		KeyPermissionExecute:       "Execute",
		KeyPermissionStayInPlan:    "Stay in Plan",
		KeyPermissionReject:        "Reject",
		KeyRiskMedium:              "medium",
		KeyRiskHigh:                "high",
		KeyPermissionDecision:      "Permission Decision",
		KeyPlanDecision:            "Plan Decision",
		KeyDecisionActor:           "Actor: %s (%s)  Work: %s",
		KeyDecisionAgentSession:    "Agent execution session: ",
		KeyDecisionAction:          "Action: ",
		KeyDecisionTarget:          "Target: ",
		KeyDecisionImpact:          "Impact: ",
		KeyDecisionRisk:            "Risk: ",
		KeyDecisionScope:           "Scope: ",
		KeyDecisionInput:           "Input: ",
		KeyDecisionAfterApproval:   "After approval: permission mode ",
		KeyGoodbye:                 "Goodbye!",
		KeyForkNoConversationTurns: "No conversation turns are available to fork.",
	})
}

func registerLegacySemantic(entries map[Key]string) {
	for key, legacyKey := range entries {
		semanticTranslations[key] = translations[legacyKey]
	}
}
