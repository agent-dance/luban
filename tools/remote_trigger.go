package tools

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/auth"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	remoteTriggerBetaHeader         = "ccr-triggers-2026-01-30"
	oauthBetaHeader                 = "oauth-2025-04-20"
	remoteTriggerMaxResultSizeChars = 100_000
	defaultOAuthAPIBaseURL          = "https://api.anthropic.com"
	stagingOAuthAPIBaseURL          = "https://api-staging.anthropic.com"
	localOAuthAPIBaseURL            = "http://localhost:8000"
	stagingOAuthTokenURL            = "https://platform.staging.ant.dev/v1/oauth/token"
	stagingOAuthAuthorizeURL        = "https://platform.staging.ant.dev/oauth/authorize"
	stagingOAuthClientID            = "22422756-60c9-4084-8eb7-27705fd5cf9a"
	remoteTriggerInferenceScope     = "user:inference"
	remoteTriggerProfileScope       = "user:profile"
	remoteTriggerOAuthTokenFDEnv    = "CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR"
	remoteTriggerOAuthTokenFile     = "/home/claude/.claude/remote/.oauth_token"
)

var remoteTriggerIDPattern = regexp.MustCompile(`^[\w-]+$`)

var remoteTriggerOAuthFDCache struct {
	sync.Mutex
	descriptor string
	fileInfo   os.FileInfo
	token      string
	resolved   bool
}

var allowedRemoteTriggerCustomOAuthBaseURLs = map[string]struct{}{
	"https://beacon.claude-ai.staging.ant.dev": {},
	"https://claude.fedstart.com":              {},
	"https://claude-staging.fedstart.com":      {},
}

type remoteTriggerProfileResponse struct {
	Organization struct {
		UUID string `json:"uuid"`
	} `json:"organization"`
}

type remoteTriggerOAuthConfig struct {
	APIBaseURL  string
	OAuthConfig auth.OAuthConfig
}

// RemoteTriggerOutput is the stable programmatic result retained alongside
// the model-visible "HTTP <status>\n<json>" rendering.
type RemoteTriggerOutput struct {
	Status int    `json:"status"`
	JSON   string `json:"json"`
}

type remoteTriggerOutput = RemoteTriggerOutput

type remoteTriggerAuthState struct {
	AccessToken            string
	Scopes                 []string
	CachedOrganizationUUID string
	cacheOrganizationUUID  func(string)
}

func (t *RemoteTriggerTool) executeRemoteTriggerAPI(ctx context.Context, in RemoteTriggerInput) (types.ToolResult, error) {
	if strings.TrimSpace(in.Action) == "" {
		return toolRuntimeError(i18n.KeyToolRemoteActionRequired), nil
	}
	if in.TriggerID != "" && !remoteTriggerIDPattern.MatchString(in.TriggerID) {
		return toolRuntimeError(i18n.KeyToolRemoteTriggerIDInvalid), nil
	}

	oauthCfg, err := t.resolveRemoteTriggerOAuthConfig()
	if err != nil {
		return ErrorResponse(err), nil
	}

	authState, err := t.resolveRemoteTriggerAuthState(ctx, oauthCfg.OAuthConfig)
	if err != nil {
		return ErrorResponse(err), nil
	}
	if authState.AccessToken == "" {
		return toolRuntimeError(i18n.KeyToolRemoteNotAuthenticated), nil
	}

	orgUUID, err := t.resolveRemoteTriggerOrganizationUUID(ctx, authState, oauthCfg.APIBaseURL)
	if err != nil {
		return ErrorResponse(err), nil
	}
	if orgUUID == "" {
		return toolRuntimeError(i18n.KeyToolRemoteOrganizationMissing), nil
	}

	method, url, payload, err := buildRemoteTriggerRequest(oauthCfg.APIBaseURL, in)
	if err != nil {
		return ErrorResponse(err), nil
	}

	statusCode, respBody, err := t.doRemoteTriggerRequest(ctx, method, url, payload, authState.AccessToken, orgUUID)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolRemoteRequestFailed, err), nil
	}

	output := remoteTriggerOutput{Status: statusCode, JSON: normalizeRemoteTriggerJSON(respBody)}
	return types.ToolResult{
		Content: fmtRemoteTriggerOutput(output.Status, output.JSON),
		Data:    output,
	}, nil
}

func (t *RemoteTriggerTool) doRemoteTriggerRequest(ctx context.Context, method, url string, payload any, accessToken, orgUUID string) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRemoteEncodeBodyFailed, err))
		}
		body = bytes.NewReader(data)
	}

	// RT-01: req.WithContext(ctx) propagates cancellation to the wire.
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return 0, nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRemoteBuildRequestFailed, err))
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", remoteTriggerBetaHeader)
	req.Header.Set("x-organization-uuid", orgUUID)

	resp, err := t.remoteTriggerHTTPClient(20 * time.Second).Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRemoteReadResponseFailed, err))
	}
	return resp.StatusCode, respBody, nil
}

func buildRemoteTriggerRequest(baseURL string, in RemoteTriggerInput) (method string, url string, payload any, err error) {
	base := strings.TrimRight(baseURL, "/") + "/v1/code/triggers"

	switch in.Action {
	case "list":
		return http.MethodGet, base, nil, nil
	case "get":
		if in.TriggerID == "" {
			return "", "", nil, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRemoteGetNeedsTriggerID))
		}
		return http.MethodGet, base + "/" + in.TriggerID, nil, nil
	case "create":
		if in.Body == nil {
			return "", "", nil, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRemoteCreateNeedsBody))
		}
		return http.MethodPost, base, in.Body, nil
	case "update":
		if in.TriggerID == "" {
			return "", "", nil, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRemoteUpdateNeedsTriggerID))
		}
		if in.Body == nil {
			return "", "", nil, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRemoteUpdateNeedsBody))
		}
		return http.MethodPost, base + "/" + in.TriggerID, in.Body, nil
	case "run":
		if in.TriggerID == "" {
			return "", "", nil, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRemoteRunNeedsTriggerID))
		}
		return http.MethodPost, base + "/" + in.TriggerID + "/run", map[string]any{}, nil
	default:
		return "", "", nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRemoteActionUnsupported, in.Action))
	}
}

func (t *RemoteTriggerTool) resolveRemoteTriggerAuthState(ctx context.Context, oauthConfig auth.OAuthConfig) (remoteTriggerAuthState, error) {
	if t.AccessTokenResolver != nil {
		token, err := t.AccessTokenResolver(ctx)
		return remoteTriggerAuthState{AccessToken: strings.TrimSpace(token), Scopes: append([]string(nil), oauthConfig.Scopes...)}, err
	}
	if isTruthyEnv(os.Getenv("LUBAN_CODE_BARE")) || isTruthyEnv(os.Getenv("DEEPSEEK_CODE_BARE")) || isTruthyEnv(os.Getenv("CLAUDE_CODE_BARE")) {
		return remoteTriggerAuthState{}, nil
	}
	if token := strings.TrimSpace(os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")); token != "" {
		return remoteTriggerAuthState{AccessToken: token, Scopes: []string{remoteTriggerInferenceScope}}, nil
	}
	if token := remoteTriggerOAuthTokenFromFileDescriptor(); token != "" {
		return remoteTriggerAuthState{AccessToken: token, Scopes: []string{remoteTriggerInferenceScope}}, nil
	}

	store, err := auth.NewStore()
	if err != nil {
		return remoteTriggerAuthState{}, nil
	}
	creds, err := store.LoadCredentials()
	if err != nil || creds == nil {
		return remoteTriggerAuthState{}, err
	}
	if !isRemoteTriggerClaudeAIProvider(creds.Provider) || !hasRemoteTriggerScope(creds.Scopes, remoteTriggerInferenceScope) {
		return remoteTriggerAuthState{}, nil
	}
	if auth.IsExpired(creds) && creds.RefreshToken != "" {
		creds, err = store.EnsureValid(ctx, oauthConfig)
		if err != nil {
			return remoteTriggerAuthState{}, err
		}
	}
	if creds == nil || strings.TrimSpace(creds.AccessToken) == "" {
		return remoteTriggerAuthState{}, nil
	}
	state := remoteTriggerAuthState{
		AccessToken:            strings.TrimSpace(creds.AccessToken),
		Scopes:                 append([]string(nil), creds.Scopes...),
		CachedOrganizationUUID: strings.TrimSpace(creds.OrganizationUUID),
	}
	state.cacheOrganizationUUID = func(orgUUID string) {
		orgUUID = strings.TrimSpace(orgUUID)
		if orgUUID == "" {
			return
		}
		updated := *creds
		updated.Scopes = append([]string(nil), creds.Scopes...)
		updated.OrganizationUUID = orgUUID
		_ = store.SaveCredentials(&updated)
	}
	return state, nil
}

// resolveRemoteTriggerAccessToken preserves the token-only integration used by
// remote Agent dispatch while keeping all token sources behind the Claude.ai
// OAuth-specific lifecycle above.
func (t *RemoteTriggerTool) resolveRemoteTriggerAccessToken(ctx context.Context, oauthConfig auth.OAuthConfig) (string, error) {
	state, err := t.resolveRemoteTriggerAuthState(ctx, oauthConfig)
	return state.AccessToken, err
}

func (t *RemoteTriggerTool) resolveRemoteTriggerOrganizationUUID(ctx context.Context, state remoteTriggerAuthState, baseURL string) (string, error) {
	if t.OrganizationUUIDResolver != nil {
		return t.OrganizationUUIDResolver(ctx, state.AccessToken, baseURL)
	}
	if state.CachedOrganizationUUID != "" {
		return state.CachedOrganizationUUID, nil
	}
	if orgUUID := remoteTriggerOrganizationUUIDFromEnvironment(); orgUUID != "" {
		return orgUUID, nil
	}
	if !hasRemoteTriggerScope(state.Scopes, remoteTriggerProfileScope) {
		return "", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/oauth/profile", nil)
	if err != nil {
		return "", nil
	}
	req.Header.Set("Authorization", "Bearer "+state.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", oauthBetaHeader)

	resp, err := t.remoteTriggerHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		return "", nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil
	}

	var profile remoteTriggerProfileResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&profile); err != nil {
		return "", nil
	}
	orgUUID := strings.TrimSpace(profile.Organization.UUID)
	if orgUUID != "" && state.cacheOrganizationUUID != nil {
		state.cacheOrganizationUUID(orgUUID)
	}
	return orgUUID, nil
}

func (t *RemoteTriggerTool) resolveRemoteTriggerOAuthConfig() (remoteTriggerOAuthConfig, error) {
	oauthConfig := auth.AnthropicOAuthConfig()
	if t.BaseURLResolver != nil {
		base, err := t.BaseURLResolver()
		return remoteTriggerOAuthConfig{APIBaseURL: strings.TrimRight(strings.TrimSpace(base), "/"), OAuthConfig: oauthConfig}, err
	}

	apiBaseURL := defaultOAuthAPIBaseURL
	if strings.EqualFold(strings.TrimSpace(os.Getenv("USER_TYPE")), "ant") {
		if isTruthyEnv(os.Getenv("USE_LOCAL_OAUTH")) {
			base := strings.TrimRight(strings.TrimSpace(os.Getenv("CLAUDE_LOCAL_OAUTH_API_BASE")), "/")
			if base == "" {
				base = localOAuthAPIBaseURL
			}
			oauthConfig.TokenURL = base + "/v1/oauth/token"
			consoleBase := strings.TrimRight(strings.TrimSpace(os.Getenv("CLAUDE_LOCAL_OAUTH_CONSOLE_BASE")), "/")
			if consoleBase == "" {
				consoleBase = "http://localhost:3000"
			}
			oauthConfig.AuthURL = consoleBase + "/oauth/authorize"
			oauthConfig.ClientID = stagingOAuthClientID
			apiBaseURL = base
		} else if isTruthyEnv(os.Getenv("USE_STAGING_OAUTH")) {
			oauthConfig.TokenURL = stagingOAuthTokenURL
			oauthConfig.AuthURL = stagingOAuthAuthorizeURL
			oauthConfig.ClientID = stagingOAuthClientID
			apiBaseURL = stagingOAuthAPIBaseURL
		}
	}

	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("CLAUDE_CODE_CUSTOM_OAUTH_URL")), "/"); base != "" {
		if _, ok := allowedRemoteTriggerCustomOAuthBaseURLs[base]; !ok {
			return remoteTriggerOAuthConfig{}, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRemoteOAuthEndpointInvalid))
		}
		apiBaseURL = base
		oauthConfig.AuthURL = base + "/oauth/authorize"
		oauthConfig.TokenURL = base + "/v1/oauth/token"
	}
	if clientID := strings.TrimSpace(os.Getenv("CLAUDE_CODE_OAUTH_CLIENT_ID")); clientID != "" {
		oauthConfig.ClientID = clientID
	}
	return remoteTriggerOAuthConfig{APIBaseURL: apiBaseURL, OAuthConfig: oauthConfig}, nil
}

func (t *RemoteTriggerTool) remoteTriggerHTTPClient(timeout time.Duration) *http.Client {
	if t.HTTPClient != nil {
		return t.HTTPClient
	}
	// RT-06: honour HTTPS_PROXY + custom CA bundle (SSL_CERT_FILE /
	// NODE_EXTRA_CA_CERTS). Without these, enterprise TLS-inspecting
	// proxies cause every call to fail with x509 errors.
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if rootPool := loadCustomCABundle(); rootPool != nil {
		transport.TLSClientConfig = &tls.Config{RootCAs: rootPool}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

// loadCustomCABundle returns the system roots merged with any extra CA
// certificates pointed at by SSL_CERT_FILE or NODE_EXTRA_CA_CERTS. Returns
// nil if neither is set or both fail to load — in which case the http
// package's default root pool is used.
func loadCustomCABundle() *x509.CertPool {
	candidates := []string{
		strings.TrimSpace(os.Getenv("SSL_CERT_FILE")),
		strings.TrimSpace(os.Getenv("NODE_EXTRA_CA_CERTS")),
	}
	hasCandidate := false
	for _, p := range candidates {
		if p != "" {
			hasCandidate = true
			break
		}
	}
	if !hasCandidate {
		return nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		pool.AppendCertsFromPEM(data)
	}
	return pool
}

func normalizeRemoteTriggerJSON(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return `""`
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err == nil {
		return compact.String()
	}
	// Axios preserves the original response string when JSON parsing fails.
	// Keep surrounding whitespace and newlines before applying jsonStringify.
	encoded, err := json.Marshal(string(body))
	if err == nil {
		return string(encoded)
	}
	return mustJSON(string(body))
}

func hasRemoteTriggerScope(scopes []string, wanted string) bool {
	for _, scope := range scopes {
		if strings.TrimSpace(scope) == wanted {
			return true
		}
	}
	return false
}

func isRemoteTriggerClaudeAIProvider(providerName string) bool {
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "", "anthropic", "claude", "claude.ai", "firstparty", "first-party":
		return true
	default:
		return false
	}
}

func remoteTriggerOrganizationUUIDFromEnvironment() string {
	accountUUID := strings.TrimSpace(os.Getenv("CLAUDE_CODE_ACCOUNT_UUID"))
	email := strings.TrimSpace(os.Getenv("CLAUDE_CODE_USER_EMAIL"))
	orgUUID := strings.TrimSpace(os.Getenv("CLAUDE_CODE_ORGANIZATION_UUID"))
	if accountUUID == "" || email == "" || orgUUID == "" {
		return ""
	}
	return orgUUID
}

func remoteTriggerOAuthTokenFromFileDescriptor() string {
	if rawFD := strings.TrimSpace(os.Getenv(remoteTriggerOAuthTokenFDEnv)); rawFD != "" {
		if token, ok := readRemoteTriggerOAuthTokenFromFD(rawFD); ok && token != "" {
			persistRemoteTriggerOAuthToken(token)
			return token
		}
	}
	data, err := os.ReadFile(remoteTriggerOAuthTokenFile)
	if err != nil || len(data) > 1<<20 {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readRemoteTriggerOAuthTokenFromFD(rawFD string) (string, bool) {
	fd, err := strconv.ParseUint(rawFD, 10, 32)
	if err != nil {
		return "", false
	}

	file, closeFile, err := openRemoteTriggerOAuthFD(uintptr(fd))
	if err != nil {
		return "", false
	}
	if closeFile {
		defer file.Close()
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return "", false
	}

	remoteTriggerOAuthFDCache.Lock()
	defer remoteTriggerOAuthFDCache.Unlock()
	if remoteTriggerOAuthFDCache.resolved &&
		remoteTriggerOAuthFDCache.descriptor == rawFD &&
		remoteTriggerOAuthFDCache.fileInfo != nil &&
		os.SameFile(remoteTriggerOAuthFDCache.fileInfo, fileInfo) {
		return remoteTriggerOAuthFDCache.token, true
	}

	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	token := ""
	if err == nil && len(data) <= 1<<20 {
		token = strings.TrimSpace(string(data))
	}
	remoteTriggerOAuthFDCache.descriptor = rawFD
	remoteTriggerOAuthFDCache.fileInfo = fileInfo
	remoteTriggerOAuthFDCache.token = token
	remoteTriggerOAuthFDCache.resolved = true
	return token, err == nil
}

func openRemoteTriggerOAuthFD(fd uintptr) (*os.File, bool, error) {
	var path string
	switch runtime.GOOS {
	case "darwin", "freebsd":
		path = fmt.Sprintf("/dev/fd/%d", fd)
	case "linux":
		path = fmt.Sprintf("/proc/self/fd/%d", fd)
	}
	if path != "" {
		file, err := os.Open(path)
		return file, true, err
	}
	file := os.NewFile(fd, "remote-trigger-oauth-token")
	if file == nil {
		return nil, false, fmt.Errorf("invalid OAuth token file descriptor %d", fd)
	}
	// os.NewFile takes ownership by default. This descriptor is inherited and
	// owned by the process launcher, so prevent the wrapper finalizer closing it.
	runtime.SetFinalizer(file, nil)
	return file, false, nil
}

func persistRemoteTriggerOAuthToken(token string) {
	if !isTruthyEnv(os.Getenv("CLAUDE_CODE_REMOTE")) || strings.TrimSpace(token) == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(remoteTriggerOAuthTokenFile), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(remoteTriggerOAuthTokenFile, []byte(token), 0o600)
}

func fmtRemoteTriggerOutput(status int, jsonBody string) string {
	return fmt.Sprintf("HTTP %d\n%s", status, jsonBody)
}

func isTruthyEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
