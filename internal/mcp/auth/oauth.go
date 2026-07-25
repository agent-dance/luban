// Package auth owns transport-independent MCP OAuth behavior.
//
// Provides PKCE (RFC 7636) helpers for the OAuth handshake the MCPTool runs
// when an HTTP-mode MCP server returns 401. The intent is to keep the
// transport-agnostic auth state in one place so the tool layer (mcp_tools.go)
// can stay focused on JSON-RPC plumbing.
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// PKCEChallengeMethodS256 is the canonical PKCE challenge method (RFC 7636).
// Exposed as a constant so callers can assert on the post-fix contract.
const PKCEChallengeMethodS256 = "S256"

// PKCEPair carries a code verifier and its corresponding S256 challenge.
type PKCEPair struct {
	Verifier  string
	Challenge string
	Method    string
}

// NewPKCEPair generates a new PKCE verifier+challenge using S256 (RFC 7636).
// The verifier is a 43-character base64url-without-padding random string and
// the challenge is the base64url-without-padding SHA-256 of the verifier.
func NewPKCEPair() (PKCEPair, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return PKCEPair{}, i18n.WrapError(i18n.KeyServicesMCPPKCERandom, err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return PKCEPair{
		Verifier:  verifier,
		Challenge: challenge,
		Method:    PKCEChallengeMethodS256,
	}, nil
}

// TokenSource resolves a Bearer token for an MCP server URL. Implementations
// may consult a local token store, an OAuth refresh flow, or an injected
// resolver. Returning ("", nil) signals "no token available, send anonymous".
type TokenSource interface {
	TokenFor(ctx context.Context, serverURL string) (string, error)
}

// TokenSourceFunc is a TokenSource backed by a closure.
type TokenSourceFunc func(ctx context.Context, serverURL string) (string, error)

// TokenFor implements TokenSource.
func (f TokenSourceFunc) TokenFor(ctx context.Context, serverURL string) (string, error) {
	return f(ctx, serverURL)
}

// DefaultTokenSource returns an empty TokenSource that emits no token.
// Callers can layer their own resolver over this.
func DefaultTokenSource() TokenSource {
	return TokenSourceFunc(func(context.Context, string) (string, error) { return "", nil })
}

// AuthChallenge is the shape of a parsed WWW-Authenticate header that points
// the client at an OAuth authorization server.
type AuthChallenge struct {
	Raw                 string
	Realm               string
	ASURI               string // authorization server URI (RFC 9728 "as_uri")
	Scheme              string
	ErrorCode           string
	ErrorDescription    string
	Scope               string
	ResourceMetadataURL string
	Params              map[string]string
}

// ParseWWWAuthenticate extracts a Bearer challenge from an HTTP response.
// Returns nil when the response carries no WWW-Authenticate header or when
// the header doesn't advertise Bearer.
func ParseWWWAuthenticate(resp *http.Response) *AuthChallenge {
	if resp == nil {
		return nil
	}
	h := headerValue(resp.Header, "WWW-Authenticate")
	if h == "" {
		return nil
	}
	// Very small subset of RFC 7235 parsing: scheme followed by k="v" pairs.
	h = strings.TrimSpace(h)
	lower := strings.ToLower(h)
	idx := strings.Index(lower, "bearer")
	if idx < 0 {
		return nil
	}
	h = strings.TrimSpace(h[idx:])
	parts := strings.SplitN(h, " ", 2)
	if len(parts) == 0 {
		return nil
	}
	scheme := strings.TrimSpace(parts[0])
	if !strings.EqualFold(scheme, "Bearer") {
		return nil
	}
	out := &AuthChallenge{Raw: headerValue(resp.Header, "WWW-Authenticate"), Scheme: scheme, Params: map[string]string{}}
	if len(parts) == 2 {
		for _, kv := range splitChallengeParams(parts[1]) {
			eq := strings.Index(kv, "=")
			if eq < 0 {
				continue
			}
			k := strings.TrimSpace(kv[:eq])
			v := unquoteChallengeValue(kv[eq+1:])
			out.Params[strings.ToLower(k)] = v
			switch strings.ToLower(k) {
			case "realm":
				out.Realm = v
			case "as_uri", "authorization_uri":
				out.ASURI = v
			case "error":
				out.ErrorCode = v
			case "error_description":
				out.ErrorDescription = v
			case "scope":
				out.Scope = v
			case "resource_metadata", "resource_metadata_url":
				out.ResourceMetadataURL = v
			}
		}
	}
	return out
}

func headerValue(headers http.Header, name string) string {
	if headers == nil {
		return ""
	}
	if value := headers.Get(name); value != "" {
		return value
	}
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func splitChallengeParams(s string) []string {
	var (
		out      []string
		buf      strings.Builder
		inQuotes bool
	)
	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			buf.WriteRune(r)
		case r == ',' && !inQuotes:
			out = append(out, buf.String())
			buf.Reset()
		default:
			buf.WriteRune(r)
		}
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}

func unquoteChallengeValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		if unquoted, err := strconvUnquote(raw); err == nil {
			return unquoted
		}
		return strings.Trim(raw, `"`)
	}
	return raw
}

func strconvUnquote(raw string) (string, error) {
	var out strings.Builder
	escaped := false
	for i := 1; i < len(raw)-1; i++ {
		ch := raw[i]
		if escaped {
			out.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		out.WriteByte(ch)
	}
	if escaped {
		return "", errors.New("unterminated escape")
	}
	return out.String(), nil
}

func authorizeURL(asURI, clientID, redirectURI string, pkce PKCEPair, scopes []string, state string) (string, error) {
	u, err := url.Parse(asURI)
	if err != nil {
		return "", i18n.WrapError(i18n.KeyServicesMCPPKCEParseASURI, err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", pkce.Challenge)
	q.Set("code_challenge_method", pkce.Method)
	if state != "" {
		q.Set("state", state)
	}
	if len(scopes) > 0 {
		q.Set("scope", strings.Join(scopes, " "))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

const (
	defaultOAuthRequestTimeout = 30 * time.Second
	defaultOAuthFlowTimeout    = 5 * time.Minute
	oauthResponseBodyLimit     = 8 << 20
)

var invalidGrantAliases = map[string]struct{}{
	"invalid_grant":         {},
	"invalid_refresh_token": {},
	"expired_refresh_token": {},
	"token_expired":         {},
}

// AuthorizationServerMetadata is the RFC 8414 subset needed for MCP OAuth.
type AuthorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint,omitempty"`
	RevocationEndpoint                string   `json:"revocation_endpoint,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported,omitempty"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported,omitempty"`
	ClientIDMetadataDocumentSupported bool     `json:"client_id_metadata_document_supported,omitempty"`
	Scope                             string   `json:"scope,omitempty"`
	DefaultScope                      string   `json:"default_scope,omitempty"`
}

type protectedResourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
}

// OAuthError is a structured OAuth endpoint error.
type OAuthError struct {
	ErrorCode        string
	ErrorDescription string
	StatusCode       int
}

func (e *OAuthError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPOAuthErrorGeneric)
	}
	if e.ErrorDescription != "" {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPOAuthErrorDetail, e.ErrorCode, e.ErrorDescription)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPOAuthError, e.ErrorCode)
}

func isInvalidGrantError(err error) bool {
	var oauthErr *OAuthError
	if errors.As(err, &oauthErr) {
		_, ok := invalidGrantAliases[oauthErr.ErrorCode]
		return ok
	}
	return false
}

// OAuthManager owns MCP OAuth discovery, flow state, token persistence, and
// TokenSource integration.
type OAuthManager struct {
	HTTPClient         *http.Client
	Store              TokenStore
	NeedsAuth          *NeedsAuthCache
	AuthRequestTimeout time.Duration
	FlowTimeout        time.Duration
	Now                func() time.Time

	mu    sync.Mutex
	flows map[string]*OAuthFlow
}

// NewOAuthManager constructs an OAuth manager with safe defaults.
func NewOAuthManager(store TokenStore, cache *NeedsAuthCache) *OAuthManager {
	if store == nil {
		store = defaultOAuthTokenStore()
	}
	if cache == nil {
		cache = defaultNeedsAuthCache
	}
	return &OAuthManager{
		HTTPClient:         http.DefaultClient,
		Store:              store,
		NeedsAuth:          cache,
		AuthRequestTimeout: defaultOAuthRequestTimeout,
		FlowTimeout:        defaultOAuthFlowTimeout,
		Now:                time.Now,
		flows:              make(map[string]*OAuthFlow),
	}
}

func (m *OAuthManager) now() time.Time {
	if m != nil && m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *OAuthManager) httpClient() *http.Client {
	if m != nil && m.HTTPClient != nil {
		return m.HTTPClient
	}
	return http.DefaultClient
}

func (m *OAuthManager) store() TokenStore {
	if m != nil && m.Store != nil {
		return m.Store
	}
	return defaultOAuthTokenStore()
}

// TokenFor implements TokenSource. It returns a stored access token, refreshing
// it when it is expired or within the TS parity five-minute refresh window.
func (m *OAuthManager) TokenFor(ctx context.Context, serverURL string) (string, error) {
	if strings.TrimSpace(serverURL) == "" {
		return "", nil
	}
	key, creds, ok, err := m.store().LoadByServerURL(ctx, serverURL)
	if err != nil || !ok {
		return "", err
	}
	if strings.TrimSpace(creds.AccessToken) == "" {
		return "", nil
	}
	now := m.now()
	if creds.ExpiresAt.IsZero() || creds.ExpiresAt.Sub(now) > 5*time.Minute {
		return creds.AccessToken, nil
	}
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return "", nil
	}
	refreshed, err := m.refreshCredentials(ctx, key, creds)
	if err != nil {
		if isInvalidGrantError(err) {
			_ = m.store().Clear(ctx, key)
			return "", nil
		}
		return "", err
	}
	if refreshed.AccessToken == "" {
		return "", nil
	}
	return refreshed.AccessToken, nil
}

func (m *OAuthManager) refreshCredentials(ctx context.Context, serverKey string, creds StoredOAuthCredentials) (StoredOAuthCredentials, error) {
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return StoredOAuthCredentials{}, nil
	}
	if strings.TrimSpace(creds.ClientID) == "" {
		return StoredOAuthCredentials{}, i18n.NewError(i18n.KeyServicesMCPOAuthRefreshClientIDMissing)
	}
	metadata, err := m.discoverMetadataForRefresh(ctx, creds)
	if err != nil {
		return StoredOAuthCredentials{}, err
	}
	tokens, err := m.refreshToken(ctx, metadata.TokenEndpoint, creds)
	if err != nil {
		if isInvalidGrantError(err) {
			_ = m.store().Clear(ctx, serverKey)
		}
		return StoredOAuthCredentials{}, err
	}
	next := creds
	next.AccessToken = tokens.AccessToken
	if tokens.RefreshToken != "" {
		next.RefreshToken = tokens.RefreshToken
	}
	next.TokenType = tokens.TokenType
	if next.TokenType == "" {
		next.TokenType = "Bearer"
	}
	if tokens.Scope != "" {
		next.Scope = tokens.Scope
	}
	expiresIn := tokens.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	next.ExpiresAt = m.now().Add(time.Duration(expiresIn) * time.Second)
	next.UpdatedAt = m.now()
	if next.DiscoveryState.AuthorizationServerURL == "" {
		next.DiscoveryState.AuthorizationServerURL = metadata.Issuer
	}
	if err := m.store().Save(ctx, serverKey, next); err != nil {
		return StoredOAuthCredentials{}, err
	}
	if m.NeedsAuth != nil {
		m.NeedsAuth.Clear(serverKey)
	}
	return next, nil
}

func (m *OAuthManager) discoverMetadataForRefresh(ctx context.Context, creds StoredOAuthCredentials) (*AuthorizationServerMetadata, error) {
	if creds.DiscoveryState.AuthorizationServerURL != "" {
		return m.discoverAuthorizationServerMetadata(ctx, creds.DiscoveryState.AuthorizationServerURL)
	}
	return m.DiscoverAuthServerMetadata(ctx, creds.ServerName, creds.ServerURL, "", creds.DiscoveryState.ResourceMetadataURL)
}

// DiscoverAuthServerMetadata implements RFC 9728 protected-resource discovery
// with path-aware RFC 8414 fallback.
func (m *OAuthManager) DiscoverAuthServerMetadata(ctx context.Context, serverName, serverURL, configuredMetadataURL, resourceMetadataURL string) (*AuthorizationServerMetadata, error) {
	if configuredMetadataURL != "" {
		if err := validateConfiguredMetadataURL(configuredMetadataURL); err != nil {
			return nil, err
		}
		return m.fetchAuthorizationServerMetadata(ctx, configuredMetadataURL)
	}

	if resourceMetadataURL != "" {
		if prm, err := m.fetchProtectedResourceMetadata(ctx, resourceMetadataURL); err == nil {
			if meta, err := m.firstAuthorizationServerMetadata(ctx, prm); err == nil && meta != nil {
				return meta, nil
			}
		}
	}

	var discoveryErr error
	for _, candidate := range protectedResourceMetadataCandidates(serverURL) {
		prm, err := m.fetchProtectedResourceMetadata(ctx, candidate)
		if err != nil {
			discoveryErr = err
			continue
		}
		meta, err := m.firstAuthorizationServerMetadata(ctx, prm)
		if err == nil && meta != nil {
			return meta, nil
		}
		discoveryErr = err
	}

	meta, err := m.discoverAuthorizationServerMetadata(ctx, serverURL)
	if err == nil && meta != nil {
		return meta, nil
	}
	if discoveryErr != nil {
		return nil, discoveryErr
	}
	return nil, err
}

func (m *OAuthManager) discoverAuthorizationServerMetadata(ctx context.Context, issuer string) (*AuthorizationServerMetadata, error) {
	var lastErr error
	for _, candidate := range authorizationServerMetadataCandidates(issuer) {
		meta, err := m.fetchAuthorizationServerMetadata(ctx, candidate)
		if err == nil {
			return meta, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = i18n.NewError(i18n.KeyServicesMCPOAuthMetadataCandidatesEmpty)
	}
	return nil, lastErr
}

func (m *OAuthManager) firstAuthorizationServerMetadata(ctx context.Context, prm *protectedResourceMetadata) (*AuthorizationServerMetadata, error) {
	if prm == nil || len(prm.AuthorizationServers) == 0 {
		return nil, i18n.NewError(i18n.KeyServicesMCPProtectedMetadataServersEmpty)
	}
	return m.discoverAuthorizationServerMetadata(ctx, prm.AuthorizationServers[0])
}

func (m *OAuthManager) fetchProtectedResourceMetadata(ctx context.Context, endpoint string) (*protectedResourceMetadata, error) {
	var out protectedResourceMetadata
	if err := m.fetchJSON(ctx, endpoint, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (m *OAuthManager) fetchAuthorizationServerMetadata(ctx context.Context, endpoint string) (*AuthorizationServerMetadata, error) {
	var out AuthorizationServerMetadata
	if err := m.fetchJSON(ctx, endpoint, &out); err != nil {
		return nil, err
	}
	if out.TokenEndpoint == "" || out.AuthorizationEndpoint == "" {
		return nil, i18n.NewError(i18n.KeyServicesMCPOAuthMetadataEndpointsMissing)
	}
	if out.Issuer == "" {
		out.Issuer = endpoint
	}
	return &out, nil
}

func (m *OAuthManager) fetchJSON(ctx context.Context, endpoint string, out any) error {
	reqCtx, cancel := context.WithTimeout(ctx, timeoutOrDefault(m.AuthRequestTimeout, defaultOAuthRequestTimeout))
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := m.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, oauthResponseBodyLimit))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return i18n.NewError(i18n.KeyServicesMCPOAuthGETRejected, endpoint, resp.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPOAuthDecodeJSON, err, endpoint)
	}
	return nil
}

func protectedResourceMetadataCandidates(serverURL string) []string {
	u, err := url.Parse(serverURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	origin := u.Scheme + "://" + u.Host
	path := strings.TrimRight(u.EscapedPath(), "/")
	candidates := []string{}
	if path != "" {
		candidates = append(candidates, origin+"/.well-known/oauth-protected-resource"+path)
	}
	candidates = append(candidates, origin+"/.well-known/oauth-protected-resource")
	return dedupeStrings(candidates)
}

func authorizationServerMetadataCandidates(issuer string) []string {
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	if strings.Contains(u.Path, "/.well-known/oauth-authorization-server") {
		return []string{u.String()}
	}
	origin := u.Scheme + "://" + u.Host
	path := strings.TrimRight(u.EscapedPath(), "/")
	candidates := []string{}
	if path != "" {
		candidates = append(candidates, origin+"/.well-known/oauth-authorization-server"+path)
	}
	candidates = append(candidates, origin+"/.well-known/oauth-authorization-server")
	return dedupeStrings(candidates)
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, value := range in {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func validateConfiguredMetadataURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return i18n.NewError(i18n.KeyMCPOAuthMetadataURLInvalid, raw)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1") {
		return nil
	}
	return i18n.NewError(i18n.KeyMCPOAuthMetadataHTTPS)
}

// OAuthFlowOptions controls StartOAuthFlow.
type OAuthFlowOptions struct {
	CallbackPort        int
	Scopes              []string
	ResourceMetadataURL string
	SkipBrowserOpen     bool
	OnAuthorizationURL  func(string)
	OpenBrowser         func(context.Context, string) error
	Timeout             time.Duration
}

// OAuthFlow is an in-flight authorization-code flow. The auth URL is returned
// immediately; Done resolves when the callback is exchanged and tokens are saved.
type OAuthFlow struct {
	ServerName       string
	ServerKey        string
	AuthorizationURL string
	RedirectURI      string
	State            string

	done   chan error
	server *http.Server
	close  context.CancelFunc
}

// Done returns a channel that receives the terminal flow result.
func (f *OAuthFlow) Done() <-chan error {
	if f == nil {
		ch := make(chan error)
		close(ch)
		return ch
	}
	return f.done
}

// Close cancels the flow and closes the callback listener.
func (f *OAuthFlow) Close() {
	if f == nil {
		return
	}
	if f.close != nil {
		f.close()
	}
	if f.server != nil {
		_ = f.server.Close()
	}
}

// StartOAuthFlow starts an MCP OAuth authorization-code + PKCE flow.
func (m *OAuthManager) StartOAuthFlow(ctx context.Context, serverName string, cfg ServerDescriptor, opts OAuthFlowOptions) (*OAuthFlow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !cfg.OAuthCapable {
		return nil, i18n.NewError(i18n.KeyMCPOAuthUnsupportedTransport, cfg.Transport)
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, i18n.NewError(i18n.KeyMCPOAuthServerURLRequired)
	}
	serverKey := ServerKey(serverName, cfg)
	cachedNeedsAuth, _ := LookupNeedsAuth(m.NeedsAuth, serverName, cfg)
	if opts.ResourceMetadataURL == "" {
		opts.ResourceMetadataURL = cachedNeedsAuth.ResourceMetadataURL
	}
	metadataURL := ""
	if cfg.OAuth != nil {
		metadataURL = cfg.OAuth.AuthServerMetadataURL
		if opts.CallbackPort == 0 && cfg.OAuth.CallbackPort != nil {
			opts.CallbackPort = *cfg.OAuth.CallbackPort
		}
	}
	metadata, err := m.DiscoverAuthServerMetadata(ctx, serverName, cfg.URL, metadataURL, opts.ResourceMetadataURL)
	if err != nil {
		return nil, err
	}

	pkce, err := NewPKCEPair()
	if err != nil {
		return nil, err
	}
	state, err := randomURLSafe(32)
	if err != nil {
		return nil, err
	}
	ln, err := listenOAuthCallback(opts.CallbackPort)
	if err != nil {
		return nil, err
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)

	clientInfo, err := m.oauthClientInformation(ctx, serverKey, serverName, cfg, metadata, redirectURI)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	scopes := opts.Scopes
	if len(scopes) == 0 && cachedNeedsAuth.Scope != "" {
		scopes = strings.Fields(cachedNeedsAuth.Scope)
	}
	if len(scopes) == 0 {
		scopes = scopesFromMetadata(metadata)
	}
	authURL, err := authorizeURL(metadata.AuthorizationEndpoint, clientInfo.ClientID, redirectURI, pkce, scopes, state)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}

	flowCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	flow := &OAuthFlow{
		ServerName:       serverName,
		ServerKey:        serverKey,
		AuthorizationURL: authURL,
		RedirectURI:      redirectURI,
		State:            state,
		done:             done,
		close:            cancel,
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code, err := parseOAuthCallback(r.URL, state)
		if err != nil {
			http.Error(w, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyOAuthAuthenticationError), http.StatusBadRequest)
			select {
			case errCh <- err:
			default:
			}
			return
		}
		_, _ = io.WriteString(w, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyOAuthAuthenticationSuccess))
		select {
		case codeCh <- code:
		default:
		}
	})
	server := &http.Server{Handler: mux}
	flow.server = server
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	m.setActiveFlow(serverKey, flow)
	if opts.OnAuthorizationURL != nil {
		opts.OnAuthorizationURL(authURL)
	}
	if !opts.SkipBrowserOpen && opts.OpenBrowser != nil {
		go func() { _ = opts.OpenBrowser(flowCtx, authURL) }()
	}

	go m.finishOAuthFlow(flowCtx, flow, cfg, metadata, clientInfo, pkce, codeCh, errCh, timeoutOrDefault(opts.Timeout, timeoutOrDefault(m.FlowTimeout, defaultOAuthFlowTimeout)))
	return flow, nil
}

func (m *OAuthManager) finishOAuthFlow(ctx context.Context, flow *OAuthFlow, cfg ServerDescriptor, metadata *AuthorizationServerMetadata, clientInfo OAuthClientInformation, pkce PKCEPair, codeCh <-chan string, errCh <-chan error, timeout time.Duration) {
	defer func() {
		if flow.server != nil {
			_ = flow.server.Close()
		}
		m.clearActiveFlow(flow.ServerKey, flow)
		close(flow.done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var code string
	select {
	case <-ctx.Done():
		flow.done <- ctx.Err()
		return
	case <-timer.C:
		flow.done <- i18n.NewError(i18n.KeyServicesMCPOAuthTimeout)
		return
	case err := <-errCh:
		flow.done <- err
		return
	case code = <-codeCh:
	}
	tokens, err := m.exchangeAuthorizationCode(ctx, metadata.TokenEndpoint, clientInfo, code, pkce.Verifier, flow.RedirectURI)
	if err != nil {
		flow.done <- err
		return
	}
	now := m.now()
	creds := newStoredOAuthCredentials(flow.ServerName, cfg, tokens, now)
	creds.ClientID = clientInfo.ClientID
	creds.ClientSecret = clientInfo.ClientSecret
	creds.DiscoveryState.AuthorizationServerURL = metadata.Issuer
	creds.DiscoveryState.ResourceMetadataURL = flowResourceMetadataURL(cfg, m.NeedsAuth, flow.ServerName)
	if err := m.store().Save(ctx, flow.ServerKey, creds); err != nil {
		flow.done <- err
		return
	}
	if m.NeedsAuth != nil {
		m.NeedsAuth.Clear(flow.ServerKey)
	}
	flow.done <- nil
}

func flowResourceMetadataURL(cfg ServerDescriptor, cache *NeedsAuthCache, serverName string) string {
	if cache == nil {
		return ""
	}
	state, ok := LookupNeedsAuth(cache, serverName, cfg)
	if !ok {
		return ""
	}
	return state.ResourceMetadataURL
}

func (m *OAuthManager) setActiveFlow(serverKey string, flow *OAuthFlow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.flows == nil {
		m.flows = make(map[string]*OAuthFlow)
	}
	m.flows[serverKey] = flow
}

func (m *OAuthManager) clearActiveFlow(serverKey string, flow *OAuthFlow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.flows[serverKey] == flow {
		delete(m.flows, serverKey)
	}
}

// ActiveOAuthFlow returns the in-flight flow for a server, if any.
func (m *OAuthManager) ActiveOAuthFlow(serverName string, cfg ServerDescriptor) (*OAuthFlow, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	flow, ok := m.flows[ServerKey(serverName, cfg)]
	return flow, ok
}

func listenOAuthCallback(port int) (net.Listener, error) {
	addr := "127.0.0.1:0"
	if port > 0 {
		addr = fmt.Sprintf("127.0.0.1:%d", port)
	}
	return net.Listen("tcp", addr)
}

func parseOAuthCallback(u *url.URL, wantState string) (string, error) {
	query := u.Query()
	if got := query.Get("state"); subtle.ConstantTimeCompare([]byte(got), []byte(wantState)) != 1 {
		return "", i18n.NewError(i18n.KeyServicesMCPOAuthStateMismatch)
	}
	if oauthError := query.Get("error"); oauthError != "" {
		return "", &OAuthError{ErrorCode: oauthError, ErrorDescription: query.Get("error_description")}
	}
	code := query.Get("code")
	if code == "" {
		return "", i18n.NewError(i18n.KeyServicesMCPOAuthCallbackCodeMissing)
	}
	return code, nil
}

func (m *OAuthManager) oauthClientInformation(ctx context.Context, serverKey, serverName string, cfg ServerDescriptor, metadata *AuthorizationServerMetadata, redirectURI string) (OAuthClientInformation, error) {
	if stored, ok, err := m.store().Load(ctx, serverKey); err != nil {
		return OAuthClientInformation{}, err
	} else if ok && stored.ClientID != "" {
		return OAuthClientInformation{ClientID: stored.ClientID, ClientSecret: stored.ClientSecret}, nil
	}
	if cfg.OAuth != nil && cfg.OAuth.ClientID != "" {
		return OAuthClientInformation{ClientID: cfg.OAuth.ClientID}, nil
	}
	if metadata.RegistrationEndpoint == "" {
		return OAuthClientInformation{}, i18n.NewError(i18n.KeyServicesMCPOAuthRegistrationUnavailable)
	}
	return m.registerOAuthClient(ctx, serverKey, serverName, cfg, metadata.RegistrationEndpoint, redirectURI)
}

func (m *OAuthManager) registerOAuthClient(ctx context.Context, serverKey, serverName string, cfg ServerDescriptor, endpoint, redirectURI string) (OAuthClientInformation, error) {
	body := map[string]any{
		"client_name":                fmt.Sprintf("%s (%s)", brand.DisplayName, serverName),
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	var out struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := m.postJSON(ctx, endpoint, body, &out); err != nil {
		return OAuthClientInformation{}, err
	}
	if out.ClientID == "" {
		return OAuthClientInformation{}, i18n.NewError(i18n.KeyServicesMCPOAuthRegistrationClientID)
	}
	creds := StoredOAuthCredentials{
		ServerName:   serverName,
		ServerURL:    cfg.URL,
		ClientID:     out.ClientID,
		ClientSecret: out.ClientSecret,
		UpdatedAt:    m.now(),
	}
	if err := m.store().Save(ctx, serverKey, creds); err != nil {
		return OAuthClientInformation{}, err
	}
	return OAuthClientInformation{ClientID: out.ClientID, ClientSecret: out.ClientSecret}, nil
}

func (m *OAuthManager) exchangeAuthorizationCode(ctx context.Context, tokenEndpoint string, clientInfo OAuthClientInformation, code, verifier, redirectURI string) (OAuthTokens, error) {
	vals := url.Values{}
	vals.Set("grant_type", "authorization_code")
	vals.Set("code", code)
	vals.Set("redirect_uri", redirectURI)
	vals.Set("client_id", clientInfo.ClientID)
	vals.Set("code_verifier", verifier)
	if clientInfo.ClientSecret != "" {
		vals.Set("client_secret", clientInfo.ClientSecret)
	}
	return m.postTokenForm(ctx, tokenEndpoint, vals)
}

func (m *OAuthManager) refreshToken(ctx context.Context, tokenEndpoint string, creds StoredOAuthCredentials) (OAuthTokens, error) {
	vals := url.Values{}
	vals.Set("grant_type", "refresh_token")
	vals.Set("refresh_token", creds.RefreshToken)
	vals.Set("client_id", creds.ClientID)
	if creds.ClientSecret != "" {
		vals.Set("client_secret", creds.ClientSecret)
	}
	return m.postTokenForm(ctx, tokenEndpoint, vals)
}

func (m *OAuthManager) postJSON(ctx context.Context, endpoint string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeoutOrDefault(m.AuthRequestTimeout, defaultOAuthRequestTimeout))
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := m.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respRaw, err := io.ReadAll(io.LimitReader(resp.Body, oauthResponseBodyLimit))
	if err != nil {
		return err
	}
	if oauthErr := parseOAuthErrorResponse(resp.StatusCode, respRaw); oauthErr != nil {
		return oauthErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return i18n.NewError(i18n.KeyServicesMCPOAuthPOSTRejected, endpoint, resp.StatusCode)
	}
	return json.Unmarshal(respRaw, out)
}

func (m *OAuthManager) postTokenForm(ctx context.Context, endpoint string, vals url.Values) (OAuthTokens, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeoutOrDefault(m.AuthRequestTimeout, defaultOAuthRequestTimeout))
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, strings.NewReader(vals.Encode()))
	if err != nil {
		return OAuthTokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := m.httpClient().Do(req)
	if err != nil {
		return OAuthTokens{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, oauthResponseBodyLimit))
	if err != nil {
		return OAuthTokens{}, err
	}
	if oauthErr := parseOAuthErrorResponse(resp.StatusCode, raw); oauthErr != nil {
		return OAuthTokens{}, oauthErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return OAuthTokens{}, i18n.NewError(i18n.KeyServicesMCPOAuthTokenEndpointRejected, resp.StatusCode)
	}
	var tokens OAuthTokens
	if err := json.Unmarshal(raw, &tokens); err != nil {
		return OAuthTokens{}, err
	}
	if tokens.AccessToken == "" {
		return OAuthTokens{}, i18n.NewError(i18n.KeyServicesMCPOAuthAccessTokenMissing)
	}
	if tokens.TokenType == "" {
		tokens.TokenType = "Bearer"
	}
	return tokens, nil
}

func parseOAuthErrorResponse(status int, raw []byte) *OAuthError {
	var body struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.Error == "" {
		return nil
	}
	normalized := body.Error
	if _, ok := invalidGrantAliases[normalized]; ok {
		normalized = "invalid_grant"
	}
	if status >= 200 && status < 300 {
		return &OAuthError{ErrorCode: normalized, ErrorDescription: body.ErrorDescription, StatusCode: http.StatusBadRequest}
	}
	return &OAuthError{ErrorCode: normalized, ErrorDescription: body.ErrorDescription, StatusCode: status}
}

func scopesFromMetadata(metadata *AuthorizationServerMetadata) []string {
	if metadata == nil {
		return nil
	}
	if strings.TrimSpace(metadata.Scope) != "" {
		return strings.Fields(metadata.Scope)
	}
	if strings.TrimSpace(metadata.DefaultScope) != "" {
		return strings.Fields(metadata.DefaultScope)
	}
	if len(metadata.ScopesSupported) > 0 {
		return append([]string(nil), metadata.ScopesSupported...)
	}
	return nil
}

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func timeoutOrDefault(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
