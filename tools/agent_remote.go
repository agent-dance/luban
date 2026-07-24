package tools

// agent_remote.go implements the RemoteAgentRuntime stub described by
// tasks/agent.json subtask agent-06. The TS reference (remoteSubagent.ts)
// delegates a sub-agent run to a remote runtime accessed through the OAuth
// trigger API. This Go implementation provides a small interface
// (RemoteRuntimeProvider) plus an HTTP-backed default that mirrors the
// auth/baseURL plumbing already used by RemoteTriggerTool.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// RemoteAgentSpawnRequest describes the minimal payload passed to a remote
// runtime when an Agent invocation requests isolation="remote".
type RemoteAgentSpawnRequest struct {
	AgentID             string                          `json:"agentId,omitempty"`
	AgentType           string                          `json:"agentType,omitempty"`
	Prompt              string                          `json:"prompt,omitempty"`
	Description         string                          `json:"description,omitempty"`
	Model               string                          `json:"model,omitempty"`
	ParentCWD           string                          `json:"parentCwd,omitempty"`
	Metadata            map[string]string               `json:"metadata,omitempty"`
	ProfileRestrictions *RemoteAgentProfileRestrictions `json:"profileRestrictions,omitempty"`
	AvoidPrompts        bool                            `json:"avoidPrompts"`
	// PermissionSnapshot is the immutable parent runtime policy captured when
	// the child is spawned. Remote runtimes must enforce this snapshot as-is;
	// it is not a hint or a child-selectable permission mode.
	PermissionSnapshot types.ToolRuntimeContext `json:"permissionSnapshot"`
}

// RemoteAgentProfileRestrictions is the resolved, serializable subset of an
// agent profile that constrains tool access. AllowedToolsSpecified
// distinguishes an absent allowlist from an explicitly empty allowlist.
type RemoteAgentProfileRestrictions struct {
	AllowedToolsSpecified bool     `json:"allowedToolsSpecified,omitempty"`
	AllowedToolSpecs      []string `json:"allowedToolSpecs,omitempty"`
	DisallowedToolSpecs   []string `json:"disallowedToolSpecs,omitempty"`
}

func remoteAgentProfileRestrictionsFromProfile(profile agentProfile) *RemoteAgentProfileRestrictions {
	allowed := displayAgentToolSpecs(profile.AllowedToolSpecs, profile.AllowedTools)
	disallowed := displayAgentToolSpecs(profile.DisallowedToolSpecs, profile.DisallowedTools)
	if !profile.AllowedToolsSpecified && len(disallowed) == 0 {
		return nil
	}
	return &RemoteAgentProfileRestrictions{
		AllowedToolsSpecified: profile.AllowedToolsSpecified,
		AllowedToolSpecs:      append([]string(nil), allowed...),
		DisallowedToolSpecs:   append([]string(nil), disallowed...),
	}
}

// RemoteAgentLaunch describes the response returned by the remote runtime when
// it accepts a spawn request.
type RemoteAgentLaunch struct {
	TaskID                      string `json:"taskId"`
	SessionURL                  string `json:"sessionUrl,omitempty"`
	OutputFile                  string `json:"outputFile,omitempty"`
	PermissionSnapshotEnforced  bool   `json:"permissionSnapshotEnforced,omitempty"`
	ProfileRestrictionsEnforced bool   `json:"profileRestrictionsEnforced,omitempty"`
	PromptRoutingEnforced       bool   `json:"promptRoutingEnforced,omitempty"`
}

// RemoteAgentStatus is the polled state of a remote agent run.
type RemoteAgentStatus struct {
	TaskID         string            `json:"taskId"`
	Phase          string            `json:"phase"`
	TranscriptPath string            `json:"transcriptPath,omitempty"`
	Result         *AgentCompleted   `json:"result,omitempty"`
	Error          string            `json:"error,omitempty"`
	Progress       []RemoteProgress  `json:"progress,omitempty"`
	Extra          map[string]string `json:"extra,omitempty"`
}

// RemoteProgress mirrors a single AgentProgressEvent on the wire.
type RemoteProgress struct {
	Phase        string `json:"phase"`
	MessageCount int    `json:"messageCount"`
	LatestTool   string `json:"latestTool,omitempty"`
	ElapsedMs    int64  `json:"elapsedMs"`
	TokensUsed   int    `json:"tokensUsed"`
}

// RemoteRuntimeProvider abstracts the remote runtime backend so tests can swap
// in a fake without HTTP plumbing. Implementations are expected to be safe for
// concurrent use.
type RemoteRuntimeProvider interface {
	Spawn(ctx context.Context, req RemoteAgentSpawnRequest) (RemoteAgentLaunch, error)
	Poll(ctx context.Context, taskID string) (RemoteAgentStatus, error)
	Cleanup(taskID string) error
}

// RemotePermissionSnapshotEnforcer is an explicit capability declaration for
// remote runtimes. A provider that does not implement this interface, or that
// returns false, cannot safely receive sub-agent work because the local
// runtime cannot enforce permissions after handing execution off.
type RemotePermissionSnapshotEnforcer interface {
	EnforcesPermissionSnapshot() bool
}

// RemoteProfileRestrictionsEnforcer declares that a remote runtime applies
// resolved profile restrictions in addition to the inherited parent policy.
type RemoteProfileRestrictionsEnforcer interface {
	EnforcesProfileRestrictions() bool
}

// RemoteFailClosedPromptEnforcer declares that an unattended remote runtime
// denies permission asks instead of presenting or blocking on a prompt.
type RemoteFailClosedPromptEnforcer interface {
	EnforcesFailClosedPrompts() bool
}

func requireRemotePermissionSnapshotEnforcement(provider RemoteRuntimeProvider) error {
	enforcer, ok := provider.(RemotePermissionSnapshotEnforcer)
	if !ok || !enforcer.EnforcesPermissionSnapshot() {
		return i18n.NewError(i18n.KeyToolAgentRemoteParentPermissionSnapshotRequired)
	}
	return nil
}

func requireRemoteProfileRestrictionsEnforcement(provider RemoteRuntimeProvider, restrictions *RemoteAgentProfileRestrictions) error {
	if restrictions == nil {
		return nil
	}
	enforcer, ok := provider.(RemoteProfileRestrictionsEnforcer)
	if !ok || !enforcer.EnforcesProfileRestrictions() {
		return i18n.NewError(i18n.KeyToolAgentRemoteProfileRestrictionsRequired)
	}
	return nil
}

func requireRemoteFailClosedPromptEnforcement(provider RemoteRuntimeProvider) error {
	enforcer, ok := provider.(RemoteFailClosedPromptEnforcer)
	if !ok || !enforcer.EnforcesFailClosedPrompts() {
		return i18n.NewError(i18n.KeyToolAgentRemoteFailClosedPromptsRequired)
	}
	return nil
}

// HTTPRemoteRuntime is the default RemoteRuntimeProvider. It speaks the
// /v1/code/agents endpoint using the same OAuth plumbing as RemoteTriggerTool.
type HTTPRemoteRuntime struct {
	Provider provider.Provider
	BaseURL  string
	OrgUUID  string
	// AccessToken is optional; when empty the runtime uses oauthAccessTokenForRemoteAgents.
	AccessToken          string
	AccessTokenExpiresAt time.Time
	// AccessTokenResolver may refresh a token before expiry. When nil, the
	// existing RemoteTrigger OAuth credential store is used.
	AccessTokenResolver func(context.Context) (string, error)
	TokenRefreshSkew    time.Duration
	HTTPClient          *http.Client
	// SpawnPath/PollPath/CleanupPath override the default REST routes.
	SpawnPath   string
	PollPath    string // expects %s placeholder for task ID
	CleanupPath string // expects %s placeholder for task ID
	tokenMu     sync.Mutex
}

// EnforcesPermissionSnapshot declares that the HTTP wire contract carries the
// captured parent policy to the remote agent runtime for enforcement.
func (*HTTPRemoteRuntime) EnforcesPermissionSnapshot() bool { return true }

func (*HTTPRemoteRuntime) EnforcesFailClosedPrompts() bool { return true }

// EnforcesProfileRestrictions declares support for the profileRestrictions
// portion of the remote agent wire contract.
func (*HTTPRemoteRuntime) EnforcesProfileRestrictions() bool { return true }

// remoteAgentBetaHeader gates the remote subagent endpoint at the API edge.
const remoteAgentBetaHeader = "ccr-subagents-2026-02-15"

func (h *HTTPRemoteRuntime) httpClient() *http.Client {
	if h.HTTPClient != nil {
		return h.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (h *HTTPRemoteRuntime) baseURL() string {
	base := strings.TrimSpace(h.BaseURL)
	if base == "" {
		base = defaultOAuthAPIBaseURL
	}
	return strings.TrimRight(base, "/")
}

func (h *HTTPRemoteRuntime) accessToken(ctx context.Context) (string, error) {
	h.tokenMu.Lock()
	defer h.tokenMu.Unlock()
	skew := h.TokenRefreshSkew
	if skew <= 0 {
		skew = 5 * time.Minute
	}
	token := strings.TrimSpace(h.AccessToken)
	if token != "" && (h.AccessTokenExpiresAt.IsZero() || h.AccessTokenExpiresAt.After(time.Now().Add(skew))) {
		return token, nil
	}
	var (
		refreshed string
		err       error
	)
	if h.AccessTokenResolver != nil {
		refreshed, err = h.AccessTokenResolver(ctx)
	} else {
		refreshed, err = oauthAccessTokenForRemoteAgents(ctx, h.Provider)
	}
	if err != nil {
		return "", err
	}
	refreshed = strings.TrimSpace(refreshed)
	if refreshed != "" {
		h.AccessToken = refreshed
		// The default OAuth resolver already enforces expiry. Treat the returned
		// value as current until the next runtime request explicitly supplies a
		// new expiry or clears the cache.
		h.AccessTokenExpiresAt = time.Time{}
	}
	return refreshed, nil
}

func (h *HTTPRemoteRuntime) decorate(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", remoteAgentBetaHeader)
	if strings.TrimSpace(h.OrgUUID) != "" {
		req.Header.Set("x-organization-uuid", h.OrgUUID)
	}
}

// Spawn implements RemoteRuntimeProvider.
func (h *HTTPRemoteRuntime) Spawn(ctx context.Context, req RemoteAgentSpawnRequest) (RemoteAgentLaunch, error) {
	token, err := h.accessToken(ctx)
	if err != nil {
		return RemoteAgentLaunch{}, err
	}
	if token == "" {
		return RemoteAgentLaunch{}, i18n.NewError(i18n.KeyToolAgentRemoteAuthenticationRequired)
	}
	path := h.SpawnPath
	if path == "" {
		path = "/v1/code/agents"
	}
	url := h.baseURL() + path
	body, err := json.Marshal(req)
	if err != nil {
		return RemoteAgentLaunch{}, i18n.WrapError(i18n.KeyToolAgentRemoteEncodeSpawnFailed, err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return RemoteAgentLaunch{}, i18n.WrapError(i18n.KeyToolAgentRemoteBuildSpawnRequestFailed, err)
	}
	h.decorate(httpReq, token)
	resp, err := h.httpClient().Do(httpReq)
	if err != nil {
		return RemoteAgentLaunch{}, i18n.WrapError(i18n.KeyToolAgentRemoteSpawnRequestFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return RemoteAgentLaunch{}, i18n.NewError(i18n.KeyToolAgentRemoteSpawnRejected, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return RemoteAgentLaunch{}, i18n.WrapError(i18n.KeyToolAgentRemoteReadSpawnResponseFailed, err)
	}
	var launch RemoteAgentLaunch
	if err := json.Unmarshal(raw, &launch); err != nil {
		return RemoteAgentLaunch{}, i18n.WrapError(i18n.KeyToolAgentRemoteDecodeSpawnResponseFailed, err)
	}
	if strings.TrimSpace(launch.TaskID) == "" {
		return RemoteAgentLaunch{}, i18n.NewError(i18n.KeyToolAgentRemoteTaskIDMissing)
	}
	if !launch.PermissionSnapshotEnforced {
		_ = h.Cleanup(launch.TaskID)
		return RemoteAgentLaunch{}, i18n.NewError(i18n.KeyToolAgentRemotePermissionSnapshotUnacknowledged)
	}
	if req.AvoidPrompts && !launch.PromptRoutingEnforced {
		_ = h.Cleanup(launch.TaskID)
		return RemoteAgentLaunch{}, i18n.NewError(i18n.KeyToolAgentRemotePromptRoutingUnacknowledged)
	}
	if req.ProfileRestrictions != nil && !launch.ProfileRestrictionsEnforced {
		_ = h.Cleanup(launch.TaskID)
		return RemoteAgentLaunch{}, i18n.NewError(i18n.KeyToolAgentRemoteProfileRestrictionsUnacknowledged)
	}
	return launch, nil
}

// Poll implements RemoteRuntimeProvider.
func (h *HTTPRemoteRuntime) Poll(ctx context.Context, taskID string) (RemoteAgentStatus, error) {
	token, err := h.accessToken(ctx)
	if err != nil {
		return RemoteAgentStatus{}, err
	}
	if strings.TrimSpace(taskID) == "" {
		return RemoteAgentStatus{}, fmt.Errorf("taskID is required")
	}
	pathTpl := h.PollPath
	if pathTpl == "" {
		pathTpl = "/v1/code/agents/%s"
	}
	url := h.baseURL() + fmt.Sprintf(pathTpl, taskID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return RemoteAgentStatus{}, fmt.Errorf("build remote agent poll request: %w", err)
	}
	h.decorate(req, token)
	resp, err := h.httpClient().Do(req)
	if err != nil {
		return RemoteAgentStatus{}, fmt.Errorf("remote agent poll failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return RemoteAgentStatus{}, fmt.Errorf("remote agent poll returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return RemoteAgentStatus{}, fmt.Errorf("read remote agent poll response: %w", err)
	}
	var status RemoteAgentStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return RemoteAgentStatus{}, fmt.Errorf("decode remote agent poll response: %w", err)
	}
	if strings.TrimSpace(status.TaskID) == "" {
		status.TaskID = taskID
	}
	return status, nil
}

// Cleanup implements RemoteRuntimeProvider. Best-effort: a 404/410 is treated as success.
func (h *HTTPRemoteRuntime) Cleanup(taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return nil
	}
	token, err := h.accessToken(context.Background())
	if err != nil {
		return err
	}
	pathTpl := h.CleanupPath
	if pathTpl == "" {
		pathTpl = "/v1/code/agents/%s"
	}
	url := h.baseURL() + fmt.Sprintf(pathTpl, taskID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("build remote agent cleanup request: %w", err)
	}
	h.decorate(req, token)
	resp, err := h.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("remote agent cleanup failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil
	}
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("remote agent cleanup returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// PollUntilTerminal repeatedly polls the remote runtime, emitting any progress
// events into the supplied emitter, and returns the final RemoteAgentStatus
// once it transitions to a terminal phase. It honours ctx cancellation and
// the supplied poll interval (defaulting to 2s).
func PollUntilTerminal(
	ctx context.Context,
	provider RemoteRuntimeProvider,
	launch RemoteAgentLaunch,
	emitter *AgentProgressEmitter,
	pollInterval time.Duration,
) (RemoteAgentStatus, error) {
	if provider == nil {
		return RemoteAgentStatus{}, fmt.Errorf("PollUntilTerminal: provider is nil")
	}
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	emitted := 0
	for {
		status, err := provider.Poll(ctx, launch.TaskID)
		if err != nil {
			return RemoteAgentStatus{}, err
		}
		if emitter != nil && len(status.Progress) > emitted {
			for _, ev := range status.Progress[emitted:] {
				emitter.Emit(AgentProgressEvent{
					Phase:        AgentProgressPhase(ev.Phase),
					MessageCount: ev.MessageCount,
					LatestTool:   ev.LatestTool,
					ElapsedMs:    ev.ElapsedMs,
					TokensUsed:   ev.TokensUsed,
				})
			}
			emitted = len(status.Progress)
		}
		switch strings.ToLower(strings.TrimSpace(status.Phase)) {
		case "completed", "error", "aborted":
			return status, nil
		}
		select {
		case <-ctx.Done():
			return RemoteAgentStatus{}, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// oauthAccessTokenForRemoteAgents tries to read an OAuth bearer token from the
// existing RemoteTrigger plumbing. Returns "" without an error when no token
// is configured so callers can branch to a clear "no provider" message.
func oauthAccessTokenForRemoteAgents(ctx context.Context, p provider.Provider) (string, error) {
	_ = p
	tool := &RemoteTriggerTool{}
	oauthCfg, err := tool.resolveRemoteTriggerOAuthConfig()
	if err != nil {
		return "", err
	}
	token, err := tool.resolveRemoteTriggerAccessToken(ctx, oauthCfg.OAuthConfig)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}
