package tui

import (
	"strings"
)

// logicalActivitiesForPresentation removes duplicate presentation rows for a
// single logical agent while leaving every non-agent activity intact. Retained
// background agents are correlated with their Agent tool wrapper only through
// explicit runtime identity (agent ID or parent tool-use ID); presentation
// labels are deliberately not identities.
func logicalActivitiesForPresentation(activities []Activity) []Activity {
	latestAgents := make(map[string]Activity)
	anonymousAgents := make([]Activity, 0)
	result := make([]Activity, 0, len(activities))
	for _, activity := range activities {
		if activity.Kind != ActivityAgent {
			result = append(result, activity)
			continue
		}
		id := strings.TrimSpace(activity.ID)
		if id == "" {
			anonymousAgents = append(anonymousAgents, activity)
			continue
		}
		current, exists := latestAgents[id]
		if !exists || presentationActivityIsLater(activity, current) {
			latestAgents[id] = activity
		}
	}

	retainedParents := make(map[string]struct{})
	for _, activity := range latestAgents {
		if parentToolUseID, ok := retainedAgentParentToolUseID(activity); ok {
			retainedParents[parentToolUseID] = struct{}{}
		}
	}
	for id, activity := range latestAgents {
		if parentToolUseID, isWrapper := strings.CutPrefix(id, "tool:"); isWrapper {
			if _, shadowed := retainedParents[strings.TrimSpace(parentToolUseID)]; shadowed {
				continue
			}
		}
		result = append(result, activity)
	}
	result = append(result, anonymousAgents...)
	sortActivitiesByWorkAndActor(result)
	return result
}

func presentationActivityIsLater(candidate, current Activity) bool {
	if activityRunIsLater(candidate, current) {
		return true
	}
	if activityRunIsLater(current, candidate) {
		return false
	}
	if candidate.Sequence != current.Sequence {
		return candidate.Sequence > current.Sequence
	}
	if candidate.SourceSequence != current.SourceSequence {
		return candidate.SourceSequence > current.SourceSequence
	}
	return false
}

func retainedAgentParentToolUseID(activity Activity) (string, bool) {
	retainedAgentID, retained := strings.CutPrefix(strings.TrimSpace(activity.ID), "background:")
	if !retained {
		return "", false
	}
	retainedAgentID = strings.TrimSpace(retainedAgentID)
	agentID := strings.TrimSpace(activity.Progress.AgentID)
	parentToolUseID := strings.TrimSpace(activity.Progress.ParentToolUseID)
	if retainedAgentID == "" || agentID == "" || parentToolUseID == "" || retainedAgentID != agentID {
		return "", false
	}
	return parentToolUseID, true
}
