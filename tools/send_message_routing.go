package tools

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

const teamLeadName = "team-lead"

type peerAddress struct {
	scheme string
	target string
}

type structuredSendMessage struct {
	Type      string
	RequestID string
	Approve   *bool
	Reason    string
	Feedback  string
}

var teamNameAdjectives = []string{
	"bright", "calm", "clever", "cozy", "curious", "eager", "gentle", "joyful",
	"lively", "mellow", "nimble", "polished", "radiant", "steady", "swift", "witty",
}

var teamNameNouns = []string{
	"atlas", "beacon", "brook", "cipher", "ember", "harbor", "meadow", "nexus",
	"orchard", "quill", "signal", "spruce", "star", "summit", "willow", "zephyr",
}

func parsePeerAddress(to string) peerAddress {
	switch {
	case strings.HasPrefix(to, "uds:"):
		return peerAddress{scheme: "uds", target: strings.TrimPrefix(to, "uds:")}
	case strings.HasPrefix(to, "/"):
		return peerAddress{scheme: "uds", target: to}
	case strings.HasPrefix(to, "bridge:"):
		// SM-01: bridge:<session-id> denotes a cross-machine UDS bridge peer.
		// Cross-machine prompt-injection requires explicit user consent and
		// must remain immune to bypassPermissions / auto-allow classifiers
		// (see RequiresBridgePermission).
		return peerAddress{scheme: "bridge", target: strings.TrimPrefix(to, "bridge:")}
	default:
		return peerAddress{scheme: "other", target: to}
	}
}

// RequiresBridgePermission reports whether the given recipient is a
// cross-machine bridge: address. The TS reference (SendMessageTool.ts:585-602)
// returns behavior:'ask' with classifierApprovable:false for these recipients,
// so callers must surface a user-facing prompt that bypassPermissions cannot
// auto-approve. This helper centralises the classification so future address
// schemes can extend it without touching the routing tool.
func RequiresBridgePermission(to string) (sessionID string, ok bool) {
	addr := parsePeerAddress(strings.TrimSpace(to))
	if addr.scheme != "bridge" {
		return "", false
	}
	target := strings.TrimSpace(addr.target)
	if target == "" {
		return "", false
	}
	return target, true
}

// bridgePermissionRegistry tracks bridge: session IDs the host approval flow
// has explicitly granted. SM-01 requires bridgePermissions to be granted out
// of band by a user-facing prompt; this registry is the contact surface the
// approval handler updates after the user clicks "Allow".
var bridgePermissionRegistry struct {
	mu      sync.RWMutex
	allowed map[string]struct{}
}

// GrantBridgePermission records that the user has approved sending to the
// given bridge session for the lifetime of the process. Mirrors the TS
// approval-cache; the runtime is expected to call this from the user-prompt
// resolution path. The call is idempotent.
func GrantBridgePermission(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	bridgePermissionRegistry.mu.Lock()
	defer bridgePermissionRegistry.mu.Unlock()
	if bridgePermissionRegistry.allowed == nil {
		bridgePermissionRegistry.allowed = map[string]struct{}{}
	}
	bridgePermissionRegistry.allowed[sessionID] = struct{}{}
}

// RevokeBridgePermission clears any prior grant for the given session.
func RevokeBridgePermission(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	bridgePermissionRegistry.mu.Lock()
	defer bridgePermissionRegistry.mu.Unlock()
	delete(bridgePermissionRegistry.allowed, sessionID)
}

func bridgePermissionGranted(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	bridgePermissionRegistry.mu.RLock()
	defer bridgePermissionRegistry.mu.RUnlock()
	_, ok := bridgePermissionRegistry.allowed[sessionID]
	return ok
}

func unsupportedPeerScheme(to string) string {
	to = strings.TrimSpace(to)
	if to == "" || to == "*" || strings.HasPrefix(to, "uds:") || strings.HasPrefix(to, "/") || strings.HasPrefix(to, "bridge:") {
		return ""
	}
	idx := strings.IndexByte(to, ':')
	if idx <= 0 {
		return ""
	}
	scheme := to[:idx]
	if !isSupportedAddressSchemeToken(scheme) {
		return ""
	}
	return scheme
}

func isSupportedAddressSchemeToken(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		case i > 0 && (r == '+' || r == '.' || r == '-'):
		default:
			return false
		}
	}
	return s != ""
}

func sanitizeSwarmName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallback
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '_' || r == '-':
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r):
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	sanitized := strings.Trim(b.String(), "-_")
	for sanitized != "" {
		r := rune(sanitized[0])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			break
		}
		sanitized = sanitized[1:]
	}
	if sanitized == "" {
		sanitized = fallback
	}
	if len(sanitized) > 64 {
		sanitized = strings.TrimRight(sanitized[:64], "-_")
	}
	if sanitized == "" {
		sanitized = "team"
	}
	return sanitized
}

func teamStorageName(teamName string) string {
	return sanitizeTeamStorageName(teamName, "team")
}

func sanitizeTeamStorageName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallback
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteByte('-')
	}
	sanitized := strings.Trim(b.String(), "-")
	if sanitized == "" {
		sanitized = fallback
	}
	if len(sanitized) > 64 {
		sanitized = strings.TrimRight(sanitized[:64], "-")
	}
	if sanitized == "" {
		sanitized = "team"
	}
	return sanitized
}

func teamMemberMailboxName(spec TeamAgentSpec, index int) string {
	if candidate := sanitizeSwarmName(spec.ID, ""); candidate != "" && candidate != "team" {
		return candidate
	}
	if candidate := sanitizeSwarmName(spec.Role, ""); candidate != "" && candidate != "team" {
		return candidate
	}
	return fmt.Sprintf("member-%d", index+1)
}

func currentTeamSessionID(mgr *TeamManager) string {
	return mgr.CurrentSessionID()
}

func currentTeamCWD(mgr *TeamManager) string {
	if mgr != nil {
		if cwd := mgr.CurrentCWD(); cwd != "" {
			return cwd
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func teamConfigExists(teamName string) bool {
	path, err := swarm.TeamConfigPath(teamName)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func randomTeamSlug() string {
	var bytes [2]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("team-%d", time.Now().UnixNano())
	}
	adj := teamNameAdjectives[int(bytes[0])%len(teamNameAdjectives)]
	noun := teamNameNouns[int(bytes[1])%len(teamNameNouns)]
	return sanitizeSwarmName(adj+"-"+noun, "team")
}

func uniqueTeamName(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", fmt.Errorf("team_name is required for TeamCreate")
	}
	sanitized := teamStorageName(requested)
	if !teamConfigExists(sanitized) {
		return requested, nil
	}
	for i := 0; i < 64; i++ {
		candidate := randomTeamSlug()
		if !teamConfigExists(candidate) {
			return candidate, nil
		}
	}
	return "", i18n.NewError(i18n.KeyToolRuntimeTeamUniqueNameGenerationFailed, "team")
}

func buildPersistedTeamConfig(teamName string, description string, leadAgentID string, leadSessionID string, cwd string, model string, agentSpecs []TeamAgentSpec) *swarm.TeamConfig {
	joinedAt := time.Now().UnixMilli()
	leadAgentType := "team-lead"
	if len(agentSpecs) > 0 && strings.TrimSpace(agentSpecs[0].Role) != "" {
		leadAgentType = strings.TrimSpace(agentSpecs[0].Role)
	}
	cfg := &swarm.TeamConfig{
		Name:          teamName,
		Description:   description,
		CreatedAt:     joinedAt,
		LeadAgentID:   leadAgentID,
		LeadSessionID: leadSessionID,
		Members: []swarm.TeamMember{
			{
				AgentID:       leadAgentID,
				Name:          teamLeadName,
				AgentType:     leadAgentType,
				Model:         model,
				JoinedAt:      joinedAt,
				CWD:           cwd,
				Subscriptions: []string{},
				IsActive:      true,
			},
		},
	}
	seenAgentIDs := map[string]struct{}{strings.ToLower(strings.TrimSpace(leadAgentID)): struct{}{}}
	for i, spec := range agentSpecs {
		key := strings.ToLower(strings.TrimSpace(spec.ID))
		if _, ok := seenAgentIDs[key]; ok {
			continue
		}
		seenAgentIDs[key] = struct{}{}
		cfg.Members = append(cfg.Members, swarm.TeamMember{
			AgentID:       spec.ID,
			Name:          teamMemberMailboxName(spec, i),
			AgentType:     spec.Role,
			Model:         model,
			JoinedAt:      joinedAt,
			CWD:           cwd,
			Subscriptions: []string{},
			IsActive:      true,
		})
	}
	return cfg
}

func activeNonLeadTeamMembers(info *TeamInfo) ([]string, error) {
	if info == nil {
		return nil, nil
	}
	if info.StorageName != "" {
		cfg, err := swarm.LoadTeamConfig(info.StorageName)
		if err == nil {
			var active []string
			for _, member := range cfg.Members {
				if strings.EqualFold(member.Name, teamLeadName) || !member.IsActive {
					continue
				}
				active = append(active, member.Name)
			}
			return active, nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, err
		}
	}
	return nil, nil
}

func setTeamMemberActive(teamName string, memberName string, active bool) error {
	if strings.TrimSpace(teamName) == "" || strings.TrimSpace(memberName) == "" {
		return nil
	}
	storageName := teamStorageName(teamName)
	_, err := swarm.UpdateTeamConfig(context.Background(), storageName, func(cfg *swarm.TeamConfig) error {
		for i := range cfg.Members {
			member := &cfg.Members[i]
			if !strings.EqualFold(member.Name, memberName) && !strings.EqualFold(member.AgentID, memberName) {
				continue
			}
			member.IsActive = active
			if active {
				member.Lifecycle = "active"
			} else {
				member.Lifecycle = "inactive"
			}
			return nil
		}
		return nil
	})
	return err
}

func loadActiveTeamConfig(mgr *TeamManager) (*swarm.TeamConfig, error) {
	var candidates []string
	if envTeam := strings.TrimSpace(os.Getenv("CLAUDE_CODE_TEAM_NAME")); envTeam != "" {
		candidates = append(candidates, envTeam)
		if safe := teamStorageName(envTeam); safe != envTeam {
			candidates = append(candidates, safe)
		}
	}
	if mgr != nil {
		if info := mgr.currentTeamInfo(); info != nil {
			if info.StorageName != "" {
				candidates = append(candidates, info.StorageName)
			}
			if info.Name != "" && info.Name != info.StorageName {
				candidates = append(candidates, info.Name)
			}
		}
	}

	seen := map[string]struct{}{}
	for _, teamName := range candidates {
		if teamName == "" {
			continue
		}
		if _, ok := seen[teamName]; ok {
			continue
		}
		seen[teamName] = struct{}{}
		cfg, err := swarm.LoadTeamConfig(teamName)
		if err == nil {
			return cfg, nil
		}
	}

	if mgr != nil {
		if info := mgr.currentTeamInfo(); info != nil {
			name := info.StorageName
			if name == "" {
				name = teamStorageName(info.Name)
			}
			cfg := &swarm.TeamConfig{
				Name:        name,
				Description: info.Description,
				LeadAgentID: info.LeadAgentID,
				Members: []swarm.TeamMember{
					{
						AgentID:  info.LeadAgentID,
						Name:     teamLeadName,
						IsActive: true,
					},
				},
			}
			for i, agentID := range info.Agents {
				cfg.Members = append(cfg.Members, swarm.TeamMember{
					AgentID:  agentID,
					Name:     sanitizeSwarmName(agentID, fmt.Sprintf("member-%d", i+1)),
					IsActive: true,
				})
			}
			return cfg, nil
		}
	}
	return nil, nil
}

func resolveMailboxRecipientName(cfg *swarm.TeamConfig, target string) string {
	target = strings.TrimSpace(target)
	if strings.EqualFold(target, teamLeadName) {
		return teamLeadName
	}
	for _, member := range cfg.Members {
		if strings.EqualFold(member.Name, target) || strings.EqualFold(member.AgentID, target) {
			return member.Name
		}
	}
	return target
}

func broadcastMailboxRecipients(cfg *swarm.TeamConfig, sender string) []string {
	sender = strings.TrimSpace(sender)
	seen := map[string]struct{}{}
	recipients := make([]string, 0, len(cfg.Members))
	for _, member := range cfg.Members {
		name := strings.TrimSpace(member.Name)
		if name == "" || strings.EqualFold(name, sender) {
			continue
		}
		if _, ok := seen[strings.ToLower(name)]; ok {
			continue
		}
		seen[strings.ToLower(name)] = struct{}{}
		recipients = append(recipients, name)
	}
	return recipients
}

func runtimeAgentID(mgr *TeamManager) string {
	if mgr != nil && mgr.Runtime != nil {
		if id := strings.TrimSpace(mgr.Runtime.ToolRuntimeContext().AgentID); id != "" {
			return id
		}
	}
	return strings.TrimSpace(os.Getenv("CLAUDE_CODE_AGENT_ID"))
}

func teamMemberByIdentity(cfg *swarm.TeamConfig, identities ...string) (swarm.TeamMember, bool) {
	if cfg == nil {
		return swarm.TeamMember{}, false
	}
	for _, identity := range identities {
		identity = strings.TrimSpace(identity)
		if identity == "" {
			continue
		}
		for _, member := range cfg.Members {
			if strings.EqualFold(member.Name, identity) || strings.EqualFold(member.AgentID, identity) {
				return member, true
			}
		}
	}
	return swarm.TeamMember{}, false
}

func currentMessageSenderName(cfg *swarm.TeamConfig, mgr *TeamManager, fallback string) string {
	if name := strings.TrimSpace(os.Getenv("CLAUDE_CODE_AGENT_NAME")); name != "" {
		if member, ok := teamMemberByIdentity(cfg, name); ok {
			return member.Name
		}
		return name
	}
	if member, ok := teamMemberByIdentity(cfg, runtimeAgentID(mgr)); ok {
		return member.Name
	}
	return fallback
}

func teammateColor(cfg *swarm.TeamConfig, identity string) string {
	if member, ok := teamMemberByIdentity(cfg, identity); ok {
		return strings.TrimSpace(member.Color)
	}
	return ""
}

func runtimeIsTeamLead(cfg *swarm.TeamConfig, mgr *TeamManager, activeTeam bool) bool {
	if !activeTeam || cfg == nil || strings.TrimSpace(cfg.LeadAgentID) == "" {
		return false
	}
	agentID := runtimeAgentID(mgr)
	if agentID != "" {
		return strings.EqualFold(agentID, cfg.LeadAgentID)
	}
	if name := strings.TrimSpace(os.Getenv("CLAUDE_CODE_AGENT_NAME")); name != "" {
		return strings.EqualFold(name, teamLeadName) || strings.EqualFold(name, cfg.LeadAgentID)
	}
	return true
}

func decodeStructuredSendMessage(msg any) (structuredSendMessage, bool, error) {
	switch value := msg.(type) {
	case nil:
		return structuredSendMessage{}, false, nil
	case string:
		return structuredSendMessage{}, false, nil
	case map[string]any:
		out := structuredSendMessage{}
		out.Type, _ = value["type"].(string)
		out.RequestID, _ = value["request_id"].(string)
		out.Reason, _ = value["reason"].(string)
		out.Feedback, _ = value["feedback"].(string)
		if approve, ok := coerceSemanticBool(value["approve"]); ok {
			out.Approve = &approve
		}
		return out, true, nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return structuredSendMessage{}, false, err
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return structuredSendMessage{}, false, err
		}
		return decodeStructuredSendMessage(raw)
	}
}

func coerceSemanticBool(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch v {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}

// generateMessageRequestID returns a globally-unique request ID for the given
// kind+target pair. SM-07: previous implementation used time.Now().UnixNano()
// which collides when two agents emit the same kind+target pair within the
// same nanosecond (CI / in-process fan-out). We now suffix a 64-bit
// crypto-random value to guarantee uniqueness, falling back to nanos only when
// crypto/rand is unavailable.
func generateMessageRequestID(kind, target string) string {
	suffix := newRandomRequestIDSuffix()
	return fmt.Sprintf("%s-%s-%s",
		sanitizeSwarmName(kind, "request"),
		sanitizeSwarmName(target, "target"),
		suffix)
}

func newRandomRequestIDSuffix() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand failure is extremely rare; nanos is an acceptable fallback.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 16)
	for i, x := range b {
		out[i*2] = hexDigits[x>>4]
		out[i*2+1] = hexDigits[x&0x0f]
	}
	return string(out)
}

func (t *SendMessageTool) sendToUnixSocket(ctx context.Context, socketPath string, content string, summary string) (types.ToolResult, error) {
	if strings.TrimSpace(socketPath) == "" {
		return sendMessageResponse(SendMessageResult{Success: false, Message: toolRuntimeText(i18n.KeyToolLegacyCAddressTargetEmpty)})
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return sendMessageResponse(SendMessageResult{Success: false, Message: toolRuntimeFormat(i18n.KeyToolLegacyCUDSSendFailed, socketPath, err)})
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(content)); err != nil {
		return sendMessageResponse(SendMessageResult{Success: false, Message: toolRuntimeFormat(i18n.KeyToolLegacyCUDSSendFailed, socketPath, err)})
	}
	preview := summary
	if preview == "" {
		preview = content
		if len(preview) > 50 {
			preview = preview[:50]
		}
	}
	return sendMessageResponse(SendMessageResult{Success: true, Message: toolRuntimeFormat(i18n.KeyToolLegacyCUDSSent, preview, socketPath)})
}

func (t *SendMessageTool) executeMailboxMessage(ctx context.Context, cfg *swarm.TeamConfig, in SendMessageInput, content string) (types.ToolResult, error) {
	mb, err := swarm.NewMailbox(cfg.Name)
	if err != nil {
		return swarmErrorResponse(err), nil
	}

	senderName := currentMessageSenderName(cfg, t.Manager, teamLeadName)
	senderColor := teammateColor(cfg, senderName)
	recipientName := resolveMailboxRecipientName(cfg, in.To)
	if in.To == "*" {
		recipients := broadcastMailboxRecipients(cfg, senderName)
		if len(recipients) == 0 {
			return sendMessageResponse(sendMessageBroadcastResult(toolRuntimeText(i18n.KeyToolLegacyCNoBroadcastRecipients), nil, nil))
		}
		var deliveryErrs []error
		for _, recipient := range recipients {
			if err := sendWithRetry(ctx, mb, recipient, swarm.Message{
				From:    senderName,
				Text:    content,
				Color:   senderColor,
				Summary: in.Summary,
			}); err != nil {
				if ctx.Err() != nil {
					return ErrorResponse(ctx.Err()), nil
				}
				deliveryErrs = append(deliveryErrs, fmt.Errorf("%s: %w", recipient, err))
			}
		}
		if len(deliveryErrs) > 0 {
			return swarmErrorResponse(errors.Join(deliveryErrs...)), nil
		}
		message := toolRuntimeFormat(i18n.KeyToolLegacyCBroadcastSent, len(recipients), strings.Join(recipients, ", "))
		return sendMessageResponse(sendMessageBroadcastResult(message, recipients, &MessageRouting{
			Sender: senderName, SenderColor: senderColor, Target: "@team", Summary: in.Summary, Content: content,
		}))
	}

	if err := sendWithRetry(ctx, mb, recipientName, swarm.Message{
		From:    senderName,
		Text:    content,
		Color:   senderColor,
		Summary: in.Summary,
	}); err != nil {
		return swarmErrorResponse(err), nil
	}
	return sendMessageResponse(SendMessageResult{
		Success: true,
		Message: toolRuntimeFormat(i18n.KeyToolLegacyCInboxSent, recipientName),
		Routing: &MessageRouting{
			Sender: senderName, SenderColor: senderColor, Target: "@" + recipientName,
			TargetColor: teammateColor(cfg, recipientName), Summary: in.Summary, Content: content,
		},
	})
}

// sendWithRetry wraps swarm.Mailbox.Send in an exponential-backoff retry loop.
// SM-04: TS uses proper-lockfile with retries:{retries:10,minTimeout:5,
// maxTimeout:100}. We mirror that: up to 10 retries with backoff growing from
// 5ms to 100ms. Only lock-contention-shaped errors are retried; permanent
// errors (validation, marshal) propagate immediately.
func sendWithRetry(ctx context.Context, mb *swarm.Mailbox, recipient string, msg swarm.Message) error {
	const maxRetries = 10
	const minBackoff = 5 * time.Millisecond
	const maxBackoff = 100 * time.Millisecond
	if strings.TrimSpace(msg.ID) == "" {
		msg.ID = swarm.NewMessageID()
	}
	if strings.TrimSpace(msg.Timestamp) == "" {
		msg.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	var lastErr error
	backoff := minBackoff
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := mb.Send(ctx, recipient, msg)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isMailboxRetryable(err) {
			return err
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
	return fmt.Errorf("mailbox send to %s exhausted %d retries: %w", recipient, maxRetries, lastErr)
}

// isMailboxRetryable reports whether the error from swarm.Mailbox.Send looks
// like a transient lock contention or temporary I/O hiccup that benefits from
// retry. Validation / marshal errors always return false so callers don't
// loop forever on a permanent failure.
func isMailboxRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "lock"),
		strings.Contains(msg, "locked"),
		strings.Contains(msg, "temporarily unavailable"),
		strings.Contains(msg, "would block"),
		strings.Contains(msg, "resource temporarily"),
		strings.Contains(msg, "context deadline exceeded"):
		return true
	}
	return false
}

func (t *SendMessageTool) executeStructuredMailboxMessage(ctx context.Context, cfg *swarm.TeamConfig, in SendMessageInput, msg structuredSendMessage, activeTeam bool) (types.ToolResult, error) {
	mb, err := swarm.NewMailbox(cfg.Name)
	if err != nil {
		return swarmErrorResponse(err), nil
	}

	senderName := currentMessageSenderName(cfg, t.Manager, teamLeadName)
	senderColor := teammateColor(cfg, senderName)
	recipientName := resolveMailboxRecipientName(cfg, in.To)
	now := time.Now().UTC().Format(time.RFC3339)

	switch msg.Type {
	case "shutdown_request":
		requestID := generateMessageRequestID("shutdown", recipientName)
		payload := map[string]any{
			"type":      "shutdown_request",
			"requestId": requestID,
			"from":      senderName,
			"timestamp": now,
		}
		if strings.TrimSpace(msg.Reason) != "" {
			payload["reason"] = msg.Reason
		}
		if err := sendWithRetry(ctx, mb, recipientName, swarm.Message{From: senderName, Text: mustJSON(payload), Color: senderColor}); err != nil {
			return swarmErrorResponse(err), nil
		}
		return sendMessageResponse(SendMessageResult{Success: true, Message: toolRuntimeFormat(i18n.KeyToolLegacyCShutdownRequestSent, recipientName, requestID), RequestID: requestID, Target: recipientName})

	case "shutdown_response":
		senderName = currentMessageSenderName(cfg, t.Manager, "teammate")
		senderColor = teammateColor(cfg, senderName)
		if recipientName != teamLeadName {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCShutdownResponseTarget, teamLeadName)), nil
		}
		if msg.RequestID == "" {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCShutdownRequestIDRequired)), nil
		}
		if msg.Approve == nil {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCShutdownApproveRequired)), nil
		}
		var payload map[string]any
		var responseMessage string
		if *msg.Approve {
			payload = map[string]any{
				"type":      "shutdown_approved",
				"requestId": msg.RequestID,
				"from":      senderName,
				"timestamp": now,
			}
			if member, ok := teamMemberByIdentity(cfg, runtimeAgentID(t.Manager), senderName); ok {
				if paneID := strings.TrimSpace(member.TmuxPaneID); paneID != "" {
					payload["paneId"] = paneID
				}
				backendType := strings.TrimSpace(member.BackendType)
				if backendType == "" && t.Manager != nil && t.Manager.Background != nil {
					if _, local := t.Manager.Background.ResolveAgentTarget(senderName); local {
						backendType = "in-process"
					}
				}
				if backendType != "" {
					payload["backendType"] = backendType
				}
			}
			responseMessage = toolRuntimeFormat(i18n.KeyToolLegacyCShutdownApproved, teamLeadName, senderName)
		} else {
			if strings.TrimSpace(msg.Reason) == "" {
				return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCShutdownRejectReason)), nil
			}
			payload = map[string]any{
				"type":      "shutdown_rejected",
				"requestId": msg.RequestID,
				"from":      senderName,
				"reason":    msg.Reason,
				"timestamp": now,
			}
			responseMessage = toolRuntimeFormat(i18n.KeyToolLegacyCShutdownRejected, msg.Reason)
		}
		if err := sendWithRetry(ctx, mb, teamLeadName, swarm.Message{From: senderName, Text: mustJSON(payload), Color: senderColor}); err != nil {
			return swarmErrorResponse(err), nil
		}
		if *msg.Approve {
			if activeTeam {
				if err := setTeamMemberActive(cfg.Name, senderName, false); err != nil {
					return swarmErrorResponse(err), nil
				}
			}
			// SM-09: actually halt the in-process teammate. The TS reference
			// finds the LocalAgentTask via findTeammateTaskByAgentId and calls
			// abortController.abort() so the agent exits its event loop in-
			// process. Go's BackgroundTaskManager owns the running session;
			// AbortAgent below cancels its run context so the loop unwinds at
			// the next safe point. We tolerate "not running" gracefully —
			// out-of-process teammates fall back to setTeamMemberActive
			// already and rejoin via inbox-poll suppression.
			if t.Manager != nil && t.Manager.Background != nil {
				_ = t.Manager.Background.AbortAgent(senderName)
			}
		}
		return sendMessageResponse(SendMessageResult{Success: true, Message: responseMessage, RequestID: msg.RequestID})

	case "plan_approval_response":
		if msg.RequestID == "" {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCPlanRequestIDRequired)), nil
		}
		if msg.Approve == nil {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCPlanApproveRequired)), nil
		}
		if !runtimeIsTeamLead(cfg, t.Manager, activeTeam) {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCPlanLeadOnly)), nil
		}
		senderName = teamLeadName
		feedback := strings.TrimSpace(msg.Feedback)
		if !*msg.Approve && feedback == "" {
			feedback = toolRuntimeText(i18n.KeyToolLegacyCPlanNeedsRevision)
		}
		payload := map[string]any{
			"type":      "plan_approval_response",
			"requestId": msg.RequestID,
			"approved":  *msg.Approve,
			"timestamp": now,
		}
		if !*msg.Approve {
			payload["feedback"] = feedback
		}
		if err := sendWithRetry(ctx, mb, recipientName, swarm.Message{From: senderName, Text: mustJSON(payload), Color: teammateColor(cfg, senderName)}); err != nil {
			return swarmErrorResponse(err), nil
		}
		// In-process teammates share this process, so consume the same authenticated
		// response immediately. Out-of-process teammates consume the mailbox copy;
		// an absent local pending entry is therefore expected and harmless.
		resolution, resolutionErr := ResolveTeammatePlanApprovalResponse(msg.RequestID, senderName, *msg.Approve, feedback)
		if resolutionErr != nil && !errors.Is(resolutionErr, ErrPlanApprovalNotPending) {
			return ErrorResponse(resolutionErr), nil
		}
		if resolutionErr == nil {
			DefaultInflightTable().Resolve(msg.RequestID)
			if t.Manager != nil && t.Manager.Background != nil {
				t.Manager.Background.ApplyAgentPlanApproval(resolution)
			}
		}
		if *msg.Approve {
			return sendMessageResponse(SendMessageResult{Success: true, Message: toolRuntimeFormat(i18n.KeyToolLegacyCPlanApproved, recipientName), RequestID: msg.RequestID})
		}
		return sendMessageResponse(SendMessageResult{Success: true, Message: toolRuntimeFormat(i18n.KeyToolLegacyCPlanRejected, recipientName, feedback), RequestID: msg.RequestID})
	}

	return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCUnsupportedStructuredType, msg.Type)), nil
}
