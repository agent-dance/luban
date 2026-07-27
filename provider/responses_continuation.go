package provider

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const responsesReasoningContextAllTurns = "all_turns"

const (
	responsesContinuationLineageHeader = "X-Luban-Stateless-Lineage"
	responsesContinuationEpochHeader   = "X-Luban-Stateless-Epoch"
	responsesContinuationResetHeader   = "X-Luban-Stateless-Reset"
)

func responsesContinuationProtocol(semantics ResponsesSemantics, lite bool) string {
	suffix := "standard"
	if lite {
		suffix = "lite"
	}
	return "responses/" + string(semantics) + "/" + suffix
}

// convertMessagesToResponsesAPIForRequest is the production stateless path.
// Provider-native output items are replayed from their private raw ledger;
// semantic blocks remain only the execution/UI projection.
func convertMessagesToResponsesAPIForRequest(params Params, prevResponseID string, semantics ResponsesSemantics, lite bool) ([]any, error) {
	if prevResponseID != "" {
		return convertNewMessagesToResponsesAPIForRequest(params, semantics, lite)
	}
	return convertAllMessagesToResponsesAPIForRequest(params, semantics, lite)
}

func convertAllMessagesToResponsesAPIForRequest(params Params, semantics ResponsesSemantics, lite bool) ([]any, error) {
	callKinds, err := responsesToolCallKinds(params.Messages)
	if err != nil {
		return nil, err
	}
	var input []any
	for _, message := range params.Messages {
		items, err := convertMessageToResponsesAPIForRequest(params, message, semantics, lite, callKinds)
		if err != nil {
			return nil, err
		}
		input = append(input, items...)
	}
	return input, nil
}

func convertNewMessagesToResponsesAPIForRequest(params Params, semantics ResponsesSemantics, lite bool) ([]any, error) {
	callKinds, err := responsesToolCallKinds(params.Messages)
	if err != nil {
		return nil, err
	}
	lastAssistant := -1
	for index := len(params.Messages) - 1; index >= 0; index-- {
		if params.Messages[index].Role == types.RoleAssistant {
			lastAssistant = index
			break
		}
	}
	var input []any
	for _, message := range params.Messages[lastAssistant+1:] {
		items, err := convertMessageToResponsesAPIForRequest(params, message, semantics, lite, callKinds)
		if err != nil {
			return nil, err
		}
		input = append(input, items...)
	}
	return input, nil
}

func convertMessageToResponsesAPIForRequest(params Params, message types.Message, semantics ResponsesSemantics, lite bool, callKinds map[string]types.ToolDefinitionType) ([]any, error) {
	role := message.Role
	if role == types.RoleDeveloper && !params.isTrustedDeveloperMessage(message) {
		role = types.RoleUser
	}
	switch role {
	case types.RoleUser:
		return convertUserMessageToResponsesAPIWithCallKinds(message, callKinds)
	case types.RoleDeveloper:
		return convertDeveloperMessageToResponsesAPI(message), nil
	case types.RoleAssistant:
		return convertAssistantMessageToResponsesAPIForRequest(message, params.Model, semantics, lite)
	default:
		return nil, nil
	}
}

func responsesToolCallKinds(messages []types.Message) (map[string]types.ToolDefinitionType, error) {
	kinds := make(map[string]types.ToolDefinitionType)
	for _, message := range messages {
		for _, block := range message.Content {
			toolUse, ok := block.(types.ToolUseBlock)
			if !ok {
				continue
			}
			if toolUse.ID == "" || toolUse.Name == "" {
				return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
			}
			if existing, duplicate := kinds[toolUse.ID]; duplicate && existing != toolUse.ToolType {
				return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
			}
			if toolUse.ToolType == types.ToolDefinitionTypeCustom {
				patch, ok := toolUse.Input["patch"].(string)
				if toolUse.RawInput == "" || !ok || patch != toolUse.RawInput {
					return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
				}
			}
			kinds[toolUse.ID] = toolUse.ToolType
		}
	}
	for _, message := range messages {
		for _, block := range message.Content {
			result, ok := block.(types.ToolResultBlock)
			if !ok || result.ToolType != types.ToolDefinitionTypeCustom {
				continue
			}
			if kinds[result.ToolUseID] != types.ToolDefinitionTypeCustom {
				return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
			}
		}
	}
	return kinds, nil
}

func convertAssistantMessageToResponsesAPIForRequest(message types.Message, model string, semantics ResponsesSemantics, lite bool) ([]any, error) {
	continuation, valid := message.ValidatedProviderContinuation()
	if message.HasProviderContinuation() && !valid {
		return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
	}
	if continuation == nil {
		if messageHasOpenAIEncryptedReasoning(message) {
			// A cipher without its complete sibling output items is not an
			// admissible stateless history. Never fall back to lossy reconstruction.
			return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
		}
		return convertAssistantMessageToResponsesAPIForModel(message, model), nil
	}

	protocol := responsesContinuationProtocol(semantics, lite)
	if continuation.Protocol != protocol ||
		strings.TrimSpace(continuation.RequestedModel) != strings.TrimSpace(model) ||
		strings.TrimSpace(continuation.ServedModel) != strings.TrimSpace(model) ||
		continuation.ReasoningContext != responsesReasoningContextAllTurns ||
		(continuation.ResponseStatus != "completed" && continuation.ResponseStatus != "incomplete") {
		return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
	}

	items := append([]types.ProviderContinuationItem(nil), continuation.Items...)
	sort.Slice(items, func(left, right int) bool { return items[left].OutputIndex < items[right].OutputIndex })
	result := make([]any, 0, len(items))
	for index, item := range items {
		if item.OutputIndex != index {
			return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
		}
		raw := item.RawJSON()
		var envelope map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &envelope) != nil {
			return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
		}
		itemType, _ := envelope["type"].(string)
		if itemType == "" || !validResponsesContinuationItem(envelope) {
			return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
		}
		result = append(result, raw)
	}
	return result, nil
}

func messageHasOpenAIEncryptedReasoning(message types.Message) bool {
	for _, block := range message.Content {
		switch thinking := block.(type) {
		case types.ThinkingBlock:
			if thinking.Signature != "" && thinking.SignatureKind == types.ThinkingSignatureOpenAIEncryptedReasoning {
				return true
			}
		case *types.ThinkingBlock:
			if thinking != nil && thinking.Signature != "" && thinking.SignatureKind == types.ThinkingSignatureOpenAIEncryptedReasoning {
				return true
			}
		}
	}
	return false
}

func buildResponsesContinuation(output []json.RawMessage, requestModel, servedModel, responseStatus string, semantics ResponsesSemantics, lite bool) (*types.ProviderContinuation, error) {
	if semantics != ResponsesSemanticsOpenAIPublic && semantics != ResponsesSemanticsOpenAICodex {
		return nil, nil
	}
	if strings.TrimSpace(requestModel) == "" || strings.TrimSpace(servedModel) != strings.TrimSpace(requestModel) ||
		(responseStatus != "completed" && responseStatus != "incomplete") {
		return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
	}
	continuation := &types.ProviderContinuation{
		Protocol:         responsesContinuationProtocol(semantics, lite),
		RequestedModel:   requestModel,
		ServedModel:      servedModel,
		ReasoningContext: responsesReasoningContextAllTurns,
		ResponseStatus:   responseStatus,
		Items:            make([]types.ProviderContinuationItem, 0, len(output)),
	}
	for index, raw := range output {
		var item map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &item) != nil {
			return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
		}
		itemType, _ := item["type"].(string)
		if itemType == "" {
			return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
		}
		if !validResponsesContinuationItem(item) {
			return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
		}
		continuation.Items = append(continuation.Items, types.NewProviderContinuationItem(index, raw))
	}
	return continuation, nil
}

func validResponsesContinuationItem(item map[string]any) bool {
	itemType, _ := item["type"].(string)
	switch itemType {
	case "reasoning":
		id, _ := item["id"].(string)
		encrypted, _ := item["encrypted_content"].(string)
		_, summaryPresent := item["summary"].([]any)
		if id == "" || encrypted == "" || !summaryPresent {
			return false
		}
	case "function_call":
		id, _ := item["id"].(string)
		callID, _ := item["call_id"].(string)
		name, _ := item["name"].(string)
		arguments, argumentsPresent := item["arguments"].(string)
		status, _ := item["status"].(string)
		if id == "" || callID == "" || name == "" || !argumentsPresent || arguments == "" || status == "" {
			return false
		}
	case "custom_tool_call":
		id, _ := item["id"].(string)
		callID, _ := item["call_id"].(string)
		name, _ := item["name"].(string)
		input, inputPresent := item["input"].(string)
		status, _ := item["status"].(string)
		if id == "" || callID == "" || name == "" || !inputPresent || input == "" || status != "completed" {
			return false
		}
	case "message":
		id, _ := item["id"].(string)
		role, _ := item["role"].(string)
		status, _ := item["status"].(string)
		_, contentPresent := item["content"].([]any)
		if id == "" || role != "assistant" || status == "" || !contentPresent {
			return false
		}
	default:
		return false
	}
	return true
}
