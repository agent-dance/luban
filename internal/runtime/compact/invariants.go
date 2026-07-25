package compact

import "github.com/agent-dance/luban/types"

// AdjustIndexToPreserveAPIInvariants moves startIndex to a safer kept-tail
// boundary. It preserves provider-required tool_use/tool_result pairs and all
// assistant fragments that share an assistant message ID with the kept tail.
func AdjustIndexToPreserveAPIInvariants(messages []types.Message, startIndex int) int {
	if !supportedCompactionRoleLayout(messages) {
		return 0
	}
	if startIndex <= 0 || startIndex >= len(messages) {
		return startIndex
	}

	adjusted := adjustTailStartForDeveloperBoundary(messages, startIndex)

	resultIDs := toolResultIDsInRange(messages, adjusted)
	if len(resultIDs) > 0 {
		keptToolUses := toolUseIDSet(messages[adjusted:])
		needed := make(map[string]struct{})
		for _, id := range resultIDs {
			if id == "" {
				continue
			}
			if _, ok := keptToolUses[id]; !ok {
				needed[id] = struct{}{}
			}
		}

		for i := adjusted - 1; i >= 0 && len(needed) > 0; i-- {
			found := false
			for _, use := range messages[i].GetToolUses() {
				if _, ok := needed[use.ID]; ok {
					delete(needed, use.ID)
					found = true
				}
			}
			if found {
				adjusted = i
			}
		}
	}

	assistantIDs := assistantIDSet(messages[adjusted:])
	if len(assistantIDs) > 0 {
		for i := adjusted - 1; i >= 0; i-- {
			if messages[i].Role != types.RoleAssistant || messages[i].ID == "" {
				continue
			}
			if _, ok := assistantIDs[messages[i].ID]; ok {
				adjusted = i
			}
		}
	}

	return skipUnpairedLeadingToolResults(messages, adjusted)
}

// AdjustHeadEndToPreserveAPIInvariants extends a preserved head so it does not
// split an assistant tool_use from its following user tool_result.
func AdjustHeadEndToPreserveAPIInvariants(messages []types.Message, headEnd int) int {
	if !supportedCompactionRoleLayout(messages) {
		return len(messages)
	}
	if headEnd <= 0 || headEnd >= len(messages) {
		return headEnd
	}
	needed := toolUseIDSet(messages[:headEnd])
	if len(needed) == 0 {
		return adjustHeadEndForDeveloperBoundary(messages, headEnd)
	}
	for _, msg := range messages[:headEnd] {
		for _, block := range msg.Content {
			if result, ok := block.(types.ToolResultBlock); ok {
				delete(needed, result.ToolUseID)
			}
		}
	}
	if len(needed) == 0 {
		return adjustHeadEndForDeveloperBoundary(messages, headEnd)
	}
	adjusted := headEnd
	for i := headEnd; i < len(messages) && len(needed) > 0; i++ {
		adjusted = i + 1
		for _, block := range messages[i].Content {
			if result, ok := block.(types.ToolResultBlock); ok {
				delete(needed, result.ToolUseID)
			}
		}
	}
	return adjustHeadEndForDeveloperBoundary(messages, adjusted)
}

func supportedCompactionRoleLayout(messages []types.Message) bool {
	toolUses := make(map[string]struct{})
	toolResults := make(map[string]struct{})
	for i := 0; i < len(messages); i++ {
		message := messages[i]
		role := effectiveCompactionRole(message)
		for _, block := range message.Content {
			switch typed := block.(type) {
			case types.ToolUseBlock:
				if role != types.RoleAssistant || typed.ID == "" {
					return false
				}
				if _, duplicate := toolUses[typed.ID]; duplicate {
					return false
				}
				toolUses[typed.ID] = struct{}{}
			case types.ToolResultBlock:
				if role != types.RoleUser || typed.ToolUseID == "" {
					return false
				}
				if _, duplicate := toolResults[typed.ToolUseID]; duplicate {
					return false
				}
				toolResults[typed.ToolUseID] = struct{}{}
			}
		}
		switch role {
		case types.RoleUser, types.RoleAssistant:
			continue
		case types.RoleDeveloper:
			for i < len(messages) && effectiveCompactionRole(messages[i]) == types.RoleDeveloper {
				if !validCatalogDeveloperMessage(messages[i]) {
					return false
				}
				i++
			}
			if i >= len(messages) || messages[i].Role != types.RoleUser {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validCatalogDeveloperMessage(message types.Message) bool {
	return message.IsTrustedDeveloperMessage()
}

func effectiveCompactionRole(message types.Message) types.Role {
	if message.Role == types.RoleDeveloper && !validCatalogDeveloperMessage(message) {
		return types.RoleUser
	}
	return message.Role
}

func adjustTailStartForDeveloperBoundary(messages []types.Message, start int) int {
	if start <= 0 || start >= len(messages) {
		return start
	}
	if effectiveCompactionRole(messages[start]) != types.RoleDeveloper &&
		!(effectiveCompactionRole(messages[start]) == types.RoleUser && effectiveCompactionRole(messages[start-1]) == types.RoleDeveloper) {
		return start
	}
	for start > 0 && effectiveCompactionRole(messages[start-1]) == types.RoleDeveloper {
		start--
	}
	return start
}

func adjustHeadEndForDeveloperBoundary(messages []types.Message, headEnd int) int {
	if headEnd <= 0 || headEnd >= len(messages) || effectiveCompactionRole(messages[headEnd-1]) != types.RoleDeveloper {
		return headEnd
	}
	for headEnd < len(messages) && effectiveCompactionRole(messages[headEnd]) == types.RoleDeveloper {
		headEnd++
	}
	if headEnd < len(messages) && messages[headEnd].Role == types.RoleUser {
		return headEnd + 1
	}
	return len(messages)
}

func toolResultIDsInRange(messages []types.Message, start int) []string {
	var ids []string
	for i := start; i < len(messages); i++ {
		if effectiveCompactionRole(messages[i]) != types.RoleUser {
			continue
		}
		for _, block := range messages[i].Content {
			if result, ok := block.(types.ToolResultBlock); ok {
				ids = append(ids, result.ToolUseID)
			}
		}
	}
	return ids
}

func toolUseIDSet(messages []types.Message) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, msg := range messages {
		if effectiveCompactionRole(msg) != types.RoleAssistant {
			continue
		}
		for _, use := range msg.GetToolUses() {
			if use.ID != "" {
				ids[use.ID] = struct{}{}
			}
		}
	}
	return ids
}

func assistantIDSet(messages []types.Message) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, msg := range messages {
		if msg.Role == types.RoleAssistant && msg.ID != "" {
			ids[msg.ID] = struct{}{}
		}
	}
	return ids
}

func skipUnpairedLeadingToolResults(messages []types.Message, start int) int {
	if start < 0 {
		start = 0
	}
	for start < len(messages) && messageHasToolResultBlock(messages[start]) {
		start++
	}
	return start
}

func messageHasToolResultBlock(msg types.Message) bool {
	if effectiveCompactionRole(msg) != types.RoleUser {
		return false
	}
	for _, block := range msg.Content {
		if _, ok := block.(types.ToolResultBlock); ok {
			return true
		}
	}
	return false
}
