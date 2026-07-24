package mcp

import (
	"encoding/json"
	"hash/fnv"
	"html"
	"regexp"
	"strings"
	"sync"
)

const (
	ChannelNotificationMethod           = "notifications/claude/channel"
	ChannelPermissionNotificationMethod = "notifications/claude/channel/permission"
	ChannelPermissionRequestMethod      = "notifications/claude/channel/permission_request"
)

var PermissionReplyPattern = regexp.MustCompile(`(?i)^\s*(y|yes|n|no)\s+([a-km-z]{5})\s*$`)

const channelRequestAlphabet = "abcdefghijkmnopqrstuvwxyz"

var channelRequestAvoidSubstrings = []string{
	"fuck", "shit", "cunt", "cock", "dick", "twat", "piss", "crap",
	"bitch", "whore", "ass", "tit", "cum", "fag", "dyke", "nig",
	"kike", "rape", "nazi", "damn", "poo", "pee", "wank", "anus",
}

// ChannelPermissionResponse is emitted when a channel server resolves a
// structured permission reply.
type ChannelPermissionResponse struct {
	Behavior   string
	FromServer string
}

// ChannelPermissionCallbacks owns pending channel permission responses.
type ChannelPermissionCallbacks struct {
	mu      sync.Mutex
	pending map[string]func(ChannelPermissionResponse)
}

func NewChannelPermissionCallbacks() *ChannelPermissionCallbacks {
	return &ChannelPermissionCallbacks{pending: make(map[string]func(ChannelPermissionResponse))}
}

func (c *ChannelPermissionCallbacks) OnResponse(requestID string, handler func(ChannelPermissionResponse)) func() {
	if c == nil || handler == nil {
		return func() {}
	}
	key := strings.ToLower(requestID)
	c.mu.Lock()
	if c.pending == nil {
		c.pending = make(map[string]func(ChannelPermissionResponse))
	}
	c.pending[key] = handler
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(c.pending, key)
		c.mu.Unlock()
	}
}

func (c *ChannelPermissionCallbacks) Resolve(requestID, behavior, fromServer string) bool {
	if c == nil {
		return false
	}
	key := strings.ToLower(requestID)
	c.mu.Lock()
	handler := c.pending[key]
	delete(c.pending, key)
	c.mu.Unlock()
	if handler == nil {
		return false
	}
	handler(ChannelPermissionResponse{Behavior: behavior, FromServer: fromServer})
	return true
}

// ShortPermissionRequestID produces the phone-friendly five-letter ID used by
// channel permission prompts.
func ShortPermissionRequestID(toolUseID string) string {
	candidate := hashPermissionRequestID(toolUseID)
	for salt := 0; salt < 10; salt++ {
		if !containsAvoidedSubstring(candidate) {
			return candidate
		}
		candidate = hashPermissionRequestID(toolUseID + ":" + string(rune('0'+salt)))
	}
	return candidate
}

func hashPermissionRequestID(input string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(input))
	n := h.Sum32()
	var b strings.Builder
	for i := 0; i < 5; i++ {
		b.WriteByte(channelRequestAlphabet[n%uint32(len(channelRequestAlphabet))])
		n = n / uint32(len(channelRequestAlphabet))
	}
	return b.String()
}

func containsAvoidedSubstring(value string) bool {
	for _, bad := range channelRequestAvoidSubstrings {
		if strings.Contains(value, bad) {
			return true
		}
	}
	return false
}

// ChannelPermissionRequestParams is the outbound CC-to-channel permission
// request payload.
type ChannelPermissionRequestParams struct {
	RequestID    string `json:"request_id"`
	ToolName     string `json:"tool_name"`
	Description  string `json:"description"`
	InputPreview string `json:"input_preview"`
}

func BuildChannelPermissionRequest(toolUseID, toolName, description string, input any) ChannelPermissionRequestParams {
	return ChannelPermissionRequestParams{
		RequestID:    ShortPermissionRequestID(toolUseID),
		ToolName:     toolName,
		Description:  description,
		InputPreview: TruncateForChannelPreview(input),
	}
}

func TruncateForChannelPreview(input any) string {
	data, err := json.Marshal(input)
	if err != nil {
		return "(unserializable)"
	}
	const max = 200
	if len(data) <= max {
		return string(data)
	}
	return string(data[:max]) + "..."
}

type ChannelAllowlistEntry struct {
	Marketplace string `json:"marketplace"`
	Plugin      string `json:"plugin"`
}

type ChannelEntry struct {
	Kind        string
	Name        string
	Marketplace string
	Dev         bool
}

type ChannelGateOptions struct {
	ChannelsEnabled        bool
	HasClaudeAIOAuth       bool
	Subscription           string
	ManagedChannelsEnabled *bool
	AllowedChannelPlugins  []ChannelAllowlistEntry
	SessionChannels        []ChannelEntry
}

type ChannelGateResult struct {
	Action string
	Kind   string
	Reason string
}

func GateChannelServer(serverName string, capabilities ServerCapabilities, pluginSource string, opts ChannelGateOptions) ChannelGateResult {
	if !hasExperimentalCapability(capabilities, "claude/channel") {
		return ChannelGateResult{Action: "skip", Kind: "capability", Reason: "server did not declare claude/channel capability"}
	}
	if !opts.ChannelsEnabled {
		return ChannelGateResult{Action: "skip", Kind: "disabled", Reason: "channels feature is not currently available"}
	}
	if !opts.HasClaudeAIOAuth {
		return ChannelGateResult{Action: "skip", Kind: "auth", Reason: "channels requires claude.ai authentication"}
	}
	managed := opts.Subscription == "team" || opts.Subscription == "enterprise"
	if managed && (opts.ManagedChannelsEnabled == nil || !*opts.ManagedChannelsEnabled) {
		return ChannelGateResult{Action: "skip", Kind: "policy", Reason: "channels not enabled by org policy"}
	}
	entry, ok := FindChannelEntry(serverName, opts.SessionChannels)
	if !ok {
		return ChannelGateResult{Action: "skip", Kind: "session", Reason: "server not in --channels list for this session"}
	}
	if entry.Kind == "plugin" {
		_, actualMarketplace := ParsePluginIdentifier(pluginSource)
		if actualMarketplace != entry.Marketplace {
			if actualMarketplace == "" {
				actualMarketplace = "an unknown source"
			}
			return ChannelGateResult{Action: "skip", Kind: "marketplace", Reason: "channel plugin marketplace does not match installed plugin source: " + actualMarketplace}
		}
		if !entry.Dev && !channelPluginAllowed(entry.Name, entry.Marketplace, opts.AllowedChannelPlugins) {
			return ChannelGateResult{Action: "skip", Kind: "allowlist", Reason: "plugin is not on the approved channels allowlist"}
		}
	} else if !entry.Dev {
		return ChannelGateResult{Action: "skip", Kind: "allowlist", Reason: "server entries are not on the approved channels allowlist"}
	}
	return ChannelGateResult{Action: "register"}
}

func FindChannelEntry(serverName string, channels []ChannelEntry) (ChannelEntry, bool) {
	parts := strings.Split(serverName, ":")
	for _, entry := range channels {
		if entry.Kind == "server" && serverName == entry.Name {
			return entry, true
		}
		if entry.Kind == "plugin" && len(parts) >= 2 && parts[0] == "plugin" && parts[1] == entry.Name {
			return entry, true
		}
	}
	return ChannelEntry{}, false
}

func ParsePluginIdentifier(source string) (name string, marketplace string) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", ""
	}
	parts := strings.Split(source, "@")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return source, ""
}

func channelPluginAllowed(name, marketplace string, entries []ChannelAllowlistEntry) bool {
	for _, entry := range entries {
		if entry.Plugin == name && entry.Marketplace == marketplace {
			return true
		}
	}
	return false
}

// FilterPermissionRelayConnections returns connected channel servers that are
// explicitly allowed and opted into permission relay.
func FilterPermissionRelayConnections(connections []MCPServerConnection, isInAllowlist func(string) bool) []MCPServerConnection {
	out := make([]MCPServerConnection, 0, len(connections))
	for _, conn := range connections {
		if conn.Type != MCPStateConnected {
			continue
		}
		if isInAllowlist != nil && !isInAllowlist(conn.Name) {
			continue
		}
		if !hasExperimentalCapability(conn.Capabilities, "claude/channel") ||
			!hasExperimentalCapability(conn.Capabilities, "claude/channel/permission") {
			continue
		}
		out = append(out, conn)
	}
	return out
}

func hasExperimentalCapability(capabilities ServerCapabilities, key string) bool {
	raw, ok := capabilities["experimental"]
	if !ok {
		return false
	}
	experimental, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	value, ok := experimental[key]
	if !ok {
		return false
	}
	if b, ok := value.(bool); ok {
		return b
	}
	return value != nil
}

func WrapChannelMessage(serverName, content string, meta map[string]string) string {
	var attrs strings.Builder
	for key, value := range meta {
		if safeChannelMetaKey(key) {
			attrs.WriteString(" ")
			attrs.WriteString(key)
			attrs.WriteString(`="`)
			attrs.WriteString(html.EscapeString(value))
			attrs.WriteString(`"`)
		}
	}
	return `<channel source="` + html.EscapeString(serverName) + `"` + attrs.String() + ">\n" + content + "\n</channel>"
}

func safeChannelMetaKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if i == 0 {
			if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
				return false
			}
			continue
		}
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
