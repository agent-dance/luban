package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/agent-dance/luban/i18n"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/internal/runtime/skillauthority"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

type sendMessageAddress struct {
	isUDS  bool
	target string
}

func parseSendMessageAddress(to string) sendMessageAddress {
	if strings.HasPrefix(to, "uds:") {
		return sendMessageAddress{isUDS: true, target: strings.TrimPrefix(to, "uds:")}
	}
	return sendMessageAddress{target: to}
}

func unsupportedSendMessageScheme(to string) string {
	to = strings.TrimSpace(to)
	if to == "" || to == "*" || strings.HasPrefix(to, "uds:") {
		return ""
	}
	separator := strings.IndexByte(to, ':')
	if separator <= 0 {
		return ""
	}
	scheme := to[:separator]
	if !isAddressSchemeToken(scheme) {
		return ""
	}
	return scheme
}

func isAddressSchemeToken(value string) bool {
	for index, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case index > 0 && char >= '0' && char <= '9':
		case index > 0 && (char == '+' || char == '.' || char == '-'):
		default:
			return false
		}
	}
	return value != ""
}

func (t *SendMessageTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t != nil && t.manager != nil {
		authority, authorityErr := skillauthority.Capture(ctx, t.manager.skills)
		if authorityErr != nil {
			return sendMessageError(authorityErr), nil
		}
		identity := t.manager.runtimeIdentitySnapshot()
		if authorityErr := authority.ValidateRuntime(types.ToolRuntimeContext{
			SessionID: identity.SessionID, ProjectRoot: identity.ProjectRoot,
		}); authorityErr != nil {
			return sendMessageError(authorityErr), nil
		}
	}

	lang := i18n.DetectOrLoadLanguage()
	if rawMessage, exists := input["message"]; exists {
		switch rawMessage.(type) {
		case string, map[string]any:
		default:
			return sendMessageRuntimeError(i18n.KeyToolSendMessageInputMessageInvalid), nil
		}
	}
	in, err := types.DecodeStrictToolInput[sendMessageInput](input)
	if err != nil {
		return sendMessageError(i18n.WrapError(i18n.KeyToolRuntimeInvalidInput, err)), nil
	}
	in.To = strings.TrimSpace(in.To)
	if in.To == "" {
		return sendMessageRuntimeError(i18n.KeyToolSendMessageToRequired), nil
	}
	if scheme := unsupportedSendMessageScheme(in.To); scheme != "" {
		return sendMessageRuntimeErrorf(i18n.KeyToolSendMessageAddressSchemeUnsupported, scheme), nil
	}
	address := parseSendMessageAddress(in.To)
	if address.isUDS && strings.TrimSpace(address.target) == "" {
		return sendMessageRuntimeError(i18n.KeyToolSendMessageAddressTargetRequired), nil
	}
	// A local peer must always use the explicit uds: transport. Bare absolute
	// paths are never interpreted as sockets.
	if !address.isUDS && strings.HasPrefix(in.To, "/") {
		return sendMessageRuntimeError(i18n.KeyToolSendMessageBareRecipientRequired), nil
	}
	if !address.isUDS && strings.Contains(in.To, "@") {
		return sendMessageRuntimeError(i18n.KeyToolSendMessageBareRecipientRequired), nil
	}

	content, plain := in.Message.(string)
	structured, isStructured, decodeErr := decodeStructuredSendMessage(in.Message)
	if decodeErr != nil {
		return sendMessageError(decodeErr), nil
	}
	if !plain && !isStructured {
		return sendMessageRuntimeError(i18n.KeyToolSendMessageInputMessageInvalid), nil
	}
	if plain && !address.isUDS && strings.TrimSpace(in.Summary) == "" {
		return sendMessageRuntimeError(i18n.KeyToolSendMessageSummaryRequired), nil
	}
	if isStructured && in.To == "*" {
		return sendMessageRuntimeError(i18n.KeyToolSendMessageStructuredBroadcastUnsupported), nil
	}
	if isStructured && address.isUDS {
		return sendMessageRuntimeError(i18n.KeyToolSendMessageStructuredCrossSessionUnsupported), nil
	}
	if isStructured && structured.Type == "shutdown_response" && !strings.EqualFold(in.To, teamLeadName) {
		return sendMessageRuntimeErrorf(i18n.KeyToolSendMessageStructuredShutdownResponseTarget, teamLeadName), nil
	}
	if isStructured && structured.Type == "shutdown_response" && structured.Approve != nil && !*structured.Approve && strings.TrimSpace(structured.Reason) == "" {
		return sendMessageRuntimeError(i18n.KeyToolSendMessageStructuredShutdownRejectReasonRequired), nil
	}

	if address.isUDS {
		return t.sendToUnixSocket(ctx, address.target, content, in.Summary)
	}
	if plain && in.To != "*" && t.retained != nil {
		resume, handled, resumeErr := t.retained.ResumeAgent(ctx, in.To, content)
		if handled {
			if resumeErr != nil {
				return sendMessageResponse(sendMessageResult{
					Success: false,
					Message: i18n.WrapError(i18n.KeyToolSendMessageAgentResumeFailed, resumeErr, in.To).Error(),
				})
			}
			if strings.EqualFold(strings.TrimSpace(resume.Status), "running") {
				return sendMessageResponse(sendMessageResult{
					Success: true,
					Message: i18n.Format(lang, i18n.KeyToolSendMessageDeliveryQueued, in.To),
				})
			}
			return sendMessageResponse(sendMessageResult{
				Success: true,
				Message: i18n.Format(
					lang,
					i18n.KeyToolSendMessageAgentResumed,
					in.To,
					resume.Status,
					resume.OutputPath,
				),
			})
		}
	}

	team, config, resolveErr := t.resolveDurableTeam(ctx)
	if resolveErr != nil {
		return resolveErrResult(resolveErr), nil
	}
	if plain {
		return t.executeMailboxMessage(ctx, team, config, in, content)
	}
	return t.executeStructuredMailboxMessage(ctx, team, config, in, structured)
}

func (t *SendMessageTool) resolveDurableTeam(ctx context.Context) (teamSnapshot, *swarm.TeamConfig, error) {
	if t == nil || t.manager == nil {
		return teamSnapshot{}, nil, i18n.NewError(i18n.KeyToolSendMessageTeamContextRequired)
	}
	team, ok := t.manager.currentTeamSnapshot()
	if !ok {
		return teamSnapshot{}, nil, i18n.NewError(i18n.KeyToolSendMessageTeamContextRequired)
	}
	if strings.TrimSpace(team.ID) == "" || strings.TrimSpace(team.Name) == "" ||
		strings.TrimSpace(team.StorageName) == "" || strings.TrimSpace(team.Owner.SessionID) == "" ||
		strings.TrimSpace(team.Owner.ProjectRoot) == "" {
		return teamSnapshot{}, nil, errInvalidDurableTeam
	}
	if exec, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok && exec.IsRuntimeOwned() {
		if strings.TrimSpace(exec.SessionID) != team.Owner.SessionID ||
			canonicalSendMessageRoot(exec.ProjectRoot) != team.Owner.ProjectRoot {
			return teamSnapshot{}, nil, errInvalidDurableTeam
		}
	}
	config, err := swarm.LoadTeamConfig(team.StorageName)
	if err != nil {
		return teamSnapshot{}, nil, i18n.NewError(i18n.KeyToolSendMessageTeamMissing, team.Name)
	}
	if !validDurableTeamConfig(team, config) {
		return teamSnapshot{}, nil, errInvalidDurableTeam
	}
	return team, config, nil
}

var errInvalidDurableTeam = errors.New("invalid durable team identity")

func validDurableTeamConfig(team teamSnapshot, config *swarm.TeamConfig) bool {
	if config == nil || strings.TrimSpace(config.Name) == "" || config.Name != team.Name ||
		strings.TrimSpace(config.LeadAgentID) == "" || config.LeadSessionID != team.Owner.SessionID ||
		len(config.Members) == 0 {
		return false
	}
	lead, ok := teamMemberByIdentity(config, config.LeadAgentID)
	if !ok || !strings.EqualFold(lead.Name, teamLeadName) || strings.TrimSpace(lead.CWD) == "" {
		return false
	}
	return canonicalSendMessageRoot(lead.CWD) == team.Owner.ProjectRoot
}

func resolveErrResult(err error) types.ToolResult {
	if _, localized := i18n.DescribeSemanticError(err); localized {
		return sendMessageError(err)
	}
	return types.ToolResult{
		Content: swarm.UserFacingError(i18n.DetectOrLoadLanguage(), err),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}
}

func canonicalSendMessageRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	root = filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = filepath.Clean(resolved)
	}
	return root
}

func (t *SendMessageTool) sendToUnixSocket(ctx context.Context, socketPath, content, summary string) (types.ToolResult, error) {
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return sendMessageResponse(sendMessageResult{
			Success: false,
			Message: i18n.WrapError(i18n.KeyToolSendMessageUDSSendFailed, err, socketPath).Error(),
		})
	}
	defer connection.Close()
	if _, err := connection.Write([]byte(content)); err != nil {
		return sendMessageResponse(sendMessageResult{
			Success: false,
			Message: i18n.WrapError(i18n.KeyToolSendMessageUDSSendFailed, err, socketPath).Error(),
		})
	}
	preview := strings.TrimSpace(summary)
	if preview == "" {
		preview = content
		if len(preview) > 50 {
			preview = preview[:50]
		}
	}
	return sendMessageResponse(sendMessageResult{
		Success: true,
		Message: sendMessageRuntimeFormat(i18n.KeyToolSendMessageUDSSent, preview, socketPath),
	})
}

func (t *SendMessageTool) executeMailboxMessage(
	ctx context.Context,
	team teamSnapshot,
	config *swarm.TeamConfig,
	in sendMessageInput,
	content string,
) (types.ToolResult, error) {
	mailbox, err := swarm.NewMailbox(team.StorageName)
	if err != nil {
		return sendMessageSwarmError(err), nil
	}
	sender, ok := t.currentSender(ctx, team, config)
	if !ok || !sender.IsActive {
		return sendMessageSwarmError(errInvalidDurableTeam), nil
	}
	if in.To == "*" {
		recipients := broadcastMailboxRecipients(config, sender.Name)
		if len(recipients) == 0 {
			return sendMessageResponse(sendMessageBroadcastResult(
				sendMessageRuntimeText(i18n.KeyToolSendMessageNoBroadcastRecipients), nil, nil,
			))
		}
		var deliveryErrors []error
		for _, recipient := range recipients {
			if err := sendMailboxWithRetry(ctx, mailbox, recipient, swarm.Message{
				From: sender.Name, Text: content, Color: sender.Color, Summary: in.Summary,
			}); err != nil {
				if ctx.Err() != nil {
					return sendMessageSwarmError(ctx.Err()), nil
				}
				deliveryErrors = append(deliveryErrors, err)
			}
		}
		if len(deliveryErrors) > 0 {
			return sendMessageSwarmError(errors.Join(deliveryErrors...)), nil
		}
		message := sendMessageRuntimeFormat(
			i18n.KeyToolSendMessageBroadcastSent, len(recipients), strings.Join(recipients, ", "),
		)
		return sendMessageResponse(sendMessageBroadcastResult(message, recipients, &messageRouting{
			Sender: sender.Name, SenderColor: sender.Color, Target: "@team", Summary: in.Summary, Content: content,
		}))
	}

	recipient, ok := activeTeamMemberByIdentity(config, in.To)
	if !ok {
		return sendMessageSwarmError(errInvalidDurableTeam), nil
	}
	if err := sendMailboxWithRetry(ctx, mailbox, recipient.Name, swarm.Message{
		From: sender.Name, Text: content, Color: sender.Color, Summary: in.Summary,
	}); err != nil {
		return sendMessageSwarmError(err), nil
	}
	return sendMessageResponse(sendMessageResult{
		Success: true,
		Message: sendMessageRuntimeFormat(i18n.KeyToolSendMessageInboxSent, recipient.Name),
		Routing: &messageRouting{
			Sender: sender.Name, SenderColor: sender.Color, Target: "@" + recipient.Name,
			TargetColor: recipient.Color, Summary: in.Summary, Content: content,
		},
	})
}

func (t *SendMessageTool) executeStructuredMailboxMessage(
	ctx context.Context,
	team teamSnapshot,
	config *swarm.TeamConfig,
	in sendMessageInput,
	message structuredSendMessage,
) (types.ToolResult, error) {
	mailbox, err := swarm.NewMailbox(team.StorageName)
	if err != nil {
		return sendMessageSwarmError(err), nil
	}
	sender, ok := t.currentSender(ctx, team, config)
	if !ok || !sender.IsActive {
		return sendMessageSwarmError(errInvalidDurableTeam), nil
	}
	recipient, ok := activeTeamMemberByIdentity(config, in.To)
	if !ok {
		return sendMessageSwarmError(errInvalidDurableTeam), nil
	}
	now := time.Now().UTC().Format(time.RFC3339)

	switch message.Type {
	case "shutdown_request":
		requestID := generateShutdownRequestID(recipient.Name)
		payload := map[string]any{
			"type": "shutdown_request", "requestId": requestID, "from": sender.Name, "timestamp": now,
		}
		if strings.TrimSpace(message.Reason) != "" {
			payload["reason"] = message.Reason
		}
		encoded, err := jsonMessagePayload(payload)
		if err != nil {
			return sendMessageSwarmError(err), nil
		}
		if err := sendMailboxWithRetry(ctx, mailbox, recipient.Name, swarm.Message{
			From: sender.Name, Text: encoded, Color: sender.Color,
		}); err != nil {
			return sendMessageSwarmError(err), nil
		}
		return sendMessageResponse(sendMessageResult{
			Success:   true,
			Message:   sendMessageRuntimeFormat(i18n.KeyToolSendMessageShutdownRequestSent, recipient.Name, requestID),
			RequestID: requestID,
			Target:    recipient.Name,
		})

	case "shutdown_response":
		if !strings.EqualFold(recipient.Name, teamLeadName) {
			return sendMessageRuntimeErrorf(i18n.KeyToolSendMessageStructuredShutdownResponseTarget, teamLeadName), nil
		}
		if message.RequestID == "" {
			return sendMessageRuntimeErrorf(i18n.KeyToolSendMessageStructuredFieldRequired, "shutdown_response", "request_id"), nil
		}
		if message.Approve == nil {
			return sendMessageRuntimeErrorf(i18n.KeyToolSendMessageStructuredFieldRequired, "shutdown_response", "approve"), nil
		}

		payload := map[string]any{
			"requestId": message.RequestID, "from": sender.Name, "timestamp": now,
		}
		var responseMessage string
		if *message.Approve {
			if strings.EqualFold(sender.BackendType, "in-process") && t.stopper == nil {
				return sendMessageSwarmError(errInvalidDurableTeam), nil
			}
			payload["type"] = "shutdown_approved"
			if paneID := strings.TrimSpace(sender.TmuxPaneID); paneID != "" {
				payload["paneId"] = paneID
			}
			if backend := strings.TrimSpace(sender.BackendType); backend != "" {
				payload["backendType"] = backend
			}
			responseMessage = sendMessageRuntimeFormat(i18n.KeyToolSendMessageShutdownApproved, teamLeadName, sender.Name)
		} else {
			payload["type"] = "shutdown_rejected"
			payload["reason"] = message.Reason
			responseMessage = sendMessageRuntimeFormat(i18n.KeyToolSendMessageShutdownRejected, message.Reason)
		}
		encoded, err := jsonMessagePayload(payload)
		if err != nil {
			return sendMessageSwarmError(err), nil
		}
		if err := sendMailboxWithRetry(ctx, mailbox, recipient.Name, swarm.Message{
			From: sender.Name, Text: encoded, Color: sender.Color,
		}); err != nil {
			return sendMessageSwarmError(err), nil
		}
		if *message.Approve {
			persisted, err := deactivateTeamMember(ctx, team.StorageName, sender.AgentID, sender.Name)
			if err != nil {
				return sendMessageSwarmError(err), nil
			}
			if strings.EqualFold(strings.TrimSpace(persisted.BackendType), "in-process") && t.stopper != nil {
				t.stopper.AbortAgent(persisted.Name)
			}
		}
		return sendMessageResponse(sendMessageResult{
			Success: true, Message: responseMessage, RequestID: message.RequestID,
		})
	}

	return sendMessageRuntimeErrorf(i18n.KeyToolSendMessageStructuredTypeUnsupported, message.Type), nil
}

func (t *SendMessageTool) currentSender(ctx context.Context, team teamSnapshot, config *swarm.TeamConfig) (swarm.TeamMember, bool) {
	if execution, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok && execution.IsRuntimeOwned() {
		actorID := strings.TrimSpace(execution.ActorID)
		if member, found := teamMemberByIdentity(config, actorID); found {
			return member, true
		}
		if actorID == "assistant" && strings.TrimSpace(execution.SessionID) == team.Owner.SessionID {
			return teamMemberByIdentity(config, config.LeadAgentID)
		}
		return swarm.TeamMember{}, false
	}
	if t == nil || t.manager == nil {
		return swarm.TeamMember{}, false
	}
	identity := t.manager.runtimeIdentitySnapshot()
	if agentID := strings.TrimSpace(identity.AgentID); agentID != "" {
		if member, found := teamMemberByIdentity(config, agentID); found {
			return member, true
		}
	}
	if strings.TrimSpace(identity.SessionID) == team.Owner.SessionID {
		return teamMemberByIdentity(config, config.LeadAgentID)
	}
	return swarm.TeamMember{}, false
}

func teamMemberByIdentity(config *swarm.TeamConfig, identities ...string) (swarm.TeamMember, bool) {
	if config == nil {
		return swarm.TeamMember{}, false
	}
	for _, identity := range identities {
		identity = strings.TrimSpace(identity)
		if identity == "" {
			continue
		}
		for _, member := range config.Members {
			if strings.EqualFold(member.Name, identity) || strings.EqualFold(member.AgentID, identity) {
				return member, true
			}
		}
	}
	return swarm.TeamMember{}, false
}

func activeTeamMemberByIdentity(config *swarm.TeamConfig, identity string) (swarm.TeamMember, bool) {
	member, ok := teamMemberByIdentity(config, identity)
	return member, ok && member.IsActive
}

func broadcastMailboxRecipients(config *swarm.TeamConfig, sender string) []string {
	seen := make(map[string]struct{})
	recipients := make([]string, 0, len(config.Members))
	for _, member := range config.Members {
		name := strings.TrimSpace(member.Name)
		key := strings.ToLower(name)
		if name == "" || !member.IsActive || strings.EqualFold(name, sender) {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		recipients = append(recipients, name)
	}
	return recipients
}

func deactivateTeamMember(ctx context.Context, storageName string, identities ...string) (swarm.TeamMember, error) {
	var updated swarm.TeamMember
	found := false
	_, err := swarm.UpdateTeamConfig(ctx, storageName, func(config *swarm.TeamConfig) error {
		for index := range config.Members {
			member := &config.Members[index]
			for _, identity := range identities {
				if !strings.EqualFold(member.Name, identity) && !strings.EqualFold(member.AgentID, identity) {
					continue
				}
				member.IsActive = false
				member.Lifecycle = "inactive"
				updated = *member
				found = true
				return nil
			}
		}
		return errInvalidDurableTeam
	})
	if err != nil {
		return swarm.TeamMember{}, err
	}
	if !found {
		return swarm.TeamMember{}, errInvalidDurableTeam
	}
	return updated, nil
}

func generateShutdownRequestID(target string) string {
	return fmt.Sprintf("shutdown-%s-%s", sanitizeRequestIDPart(target), swarm.NewMessageID())
}

func sanitizeRequestIDPart(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		switch {
		case unicode.IsLetter(char) || unicode.IsDigit(char):
			builder.WriteRune(char)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}
	sanitized := strings.Trim(builder.String(), "-")
	if sanitized == "" {
		return "target"
	}
	return sanitized
}

func jsonMessagePayload(payload map[string]any) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func sendMailboxWithRetry(ctx context.Context, mailbox *swarm.Mailbox, recipient string, message swarm.Message) error {
	const maxRetries = 10
	const minBackoff = 5 * time.Millisecond
	const maxBackoff = 100 * time.Millisecond
	if strings.TrimSpace(message.ID) == "" {
		message.ID = swarm.NewMessageID()
	}
	if strings.TrimSpace(message.Timestamp) == "" {
		message.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	backoff := minBackoff
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := mailbox.Send(ctx, recipient, message); err == nil {
			return nil
		} else {
			lastErr = err
			if !mailboxErrorRetryable(err) {
				return err
			}
		}
		if attempt == maxRetries {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	return fmt.Errorf("mailbox send retries exhausted: %w", lastErr)
}

func mailboxErrorRetryable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "lock") ||
		strings.Contains(message, "temporarily unavailable") ||
		strings.Contains(message, "would block") ||
		strings.Contains(message, "resource temporarily") ||
		strings.Contains(message, "context deadline exceeded")
}

func sendMessageSwarmError(err error) types.ToolResult {
	return types.ToolResult{
		Content: swarm.UserFacingError(i18n.DetectOrLoadLanguage(), err),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}
}

func sendMessageRuntimeError(key i18n.Key) types.ToolResult {
	return types.ToolResult{
		Content: sendMessageRuntimeText(key), IsError: true, Outcome: types.ToolOutcomeFailed,
	}
}

func sendMessageRuntimeErrorf(key i18n.Key, args ...any) types.ToolResult {
	return types.ToolResult{
		Content: sendMessageRuntimeFormat(key, args...), IsError: true, Outcome: types.ToolOutcomeFailed,
	}
}
