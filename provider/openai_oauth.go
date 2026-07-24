package provider

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const (
	openAIOAuthClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAIOAuthIssuer       = "https://auth.openai.com"
	openAIOAuthAuthURL      = openAIOAuthIssuer + "/oauth/authorize"
	openAIOAuthOriginator   = "codex_cli_rs"
	openAIOAuthDefaultPort  = 1455
	openAIOAuthCallbackPath = "/auth/callback"
)

var openAIOAuthScopes = []string{
	"openid",
	"profile",
	"email",
	"offline_access",
	"api.connectors.read",
	"api.connectors.invoke",
}

var openAIOAuthTokenURL = openAIOAuthIssuer + "/oauth/token"
var openAIOAuthHTTPClient = &http.Client{Timeout: 30 * time.Second}

type openAITokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

type OpenAIOAuthResult struct {
	APIKey              string
	APIKeyExchangeError string
	AccessToken         string
	RefreshToken        string
	ExpiresAt           time.Time
	AccountID           string
	PlanType            string
	AccountIsFedRAMP    bool
}

// StartOpenAIOAuthFlow runs the Codex-style OpenAI browser OAuth flow and
// returns the ChatGPT OAuth tokens plus an API-key exchange result when the
// auth server makes one available.
func StartOpenAIOAuthFlow(ctx context.Context, authURLOut chan<- string) (*OpenAIOAuthResult, error) {
	verifier, challenge, err := openAIGeneratePKCE()
	if err != nil {
		return nil, err
	}
	state, err := openAIRandomState()
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", openAIOAuthDefaultPort))
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthStartCallbackServer, err)
	}
	defer ln.Close()

	redirectURI := fmt.Sprintf("http://localhost:%d%s", ln.Addr().(*net.TCPAddr).Port, openAIOAuthCallbackPath)
	codeCh := make(chan string, 1)
	handlerErrCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(openAIOAuthCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		lang := i18n.DetectOrLoadLanguage()
		if gotState := q.Get("state"); gotState != state {
			http.Error(w, i18n.Text(lang, i18n.KeyOAuthInvalidState), http.StatusBadRequest)
			handlerErrCh <- i18n.NewError(i18n.KeyAuthOAuthCallbackStateMismatch, gotState)
			return
		}
		if errParam := q.Get("error"); errParam != "" {
			desc := q.Get("error_description")
			http.Error(w, i18n.Format(lang, i18n.KeyOAuthAuthorizationDenied, errParam), http.StatusBadRequest)
			handlerErrCh <- i18n.NewError(i18n.KeyAuthOAuthAuthorizationError, errParam, desc)
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, i18n.Text(lang, i18n.KeyOAuthMissingCode), http.StatusBadRequest)
			handlerErrCh <- i18n.NewError(i18n.KeyAuthOAuthCallbackMissingCode)
			return
		}
		fmt.Fprintln(w, i18n.Text(lang, i18n.KeyOAuthAuthorizationSuccess))
		codeCh <- code
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	authURL, err := buildOpenAIAuthorizeURL(challenge, state, redirectURI)
	if err != nil {
		return nil, err
	}
	if authURLOut != nil {
		select {
		case authURLOut <- authURL:
		default:
		}
	}

	var code string
	select {
	case <-ctx.Done():
		return nil, i18n.WrapError(i18n.KeyAuthOAuthFlowCancelled, ctx.Err())
	case err := <-handlerErrCh:
		return nil, err
	case code = <-codeCh:
	}

	tr, err := exchangeOpenAIAuthorizationCode(ctx, code, verifier, redirectURI)
	if err != nil {
		return nil, err
	}
	return openAIResultFromTokenResponse(ctx, tr)
}

// ResolveCredentialConfig builds provider config from the attached credential
// store. OpenAI OAuth entries use the ChatGPT Codex backend, matching Codex's
// ChatGPT-login path; API-key entries use the public OpenAI API.
func ResolveCredentialConfig(r *ProviderRegistry, providerName string) (Config, error) {
	cfg := Config{CacheRoutingPreference: ParseCacheRoutingPreference(os.Getenv("LUBAN_CODE_CACHE_ROUTING_MODE"))}
	if r == nil {
		return cfg, nil
	}

	cs := r.CredentialStoreRef()
	if cs == nil {
		return cfg, nil
	}

	lookupName := CanonicalProviderName(providerName)
	var entry CredentialEntry
	found := false
	for _, candidate := range CredentialLookupNames(providerName) {
		if e, ok := cs.Get(candidate); ok {
			entry = e
			lookupName = CanonicalProviderName(candidate)
			found = true
			break
		}
	}
	if !found {
		return cfg, nil
	}

	if lookupName == "openai" && entry.AuthMethod == "oauth" {
		needsRefresh := entry.AccessToken == "" || (!entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt))
		if needsRefresh {
			refreshed, err := RefreshOpenAIOAuthCredential(context.Background(), entry)
			if err != nil {
				return cfg, err
			}
			if err := cs.Set(refreshed); err != nil {
				return cfg, i18n.WrapError(i18n.KeyAuthOAuthSaveRefreshedCredentials, err)
			}
			entry = refreshed
		}
		oauthConfig, err := openAIOAuthConfigFromEntry(entry)
		if err != nil {
			return Config{}, err
		}
		oauthConfig.CacheRoutingPreference = cfg.CacheRoutingPreference
		return oauthConfig, nil
	} else {
		switch entry.AuthMethod {
		case "api_key", "env":
			cfg.APIKey = entry.APIKey
		case "oauth":
			if lookupName == "openai" {
				cfg.APIKey = entry.AccessToken
				cfg.Headers = mergeHeaders(cfg.Headers, openAICodexHeaders())
			} else if lookupName == "anthropic" {
				if hook := r.OAuthHookRef(); hook != nil {
					token, err := hook.LoadAccessToken(context.Background())
					if err != nil {
						return cfg, err
					}
					if token != "" {
						cfg.AuthToken = token
					}
				}
				if cfg.AuthToken == "" {
					cfg.AuthToken = entry.AccessToken
				}
			} else {
				cfg.APIKey = entry.AccessToken
			}
		}
	}

	if entry.BaseURL != "" {
		cfg.BaseURL = entry.BaseURL
	}
	return cfg, nil
}

// RefreshOpenAIOAuthCredential refreshes an OpenAI OAuth credential and
// exchanges the returned id_token for a usable OpenAI API key.
func RefreshOpenAIOAuthCredential(ctx context.Context, entry CredentialEntry) (CredentialEntry, error) {
	if entry.RefreshToken == "" {
		return entry, i18n.NewError(i18n.KeyAuthOAuthRefreshTokenRequired)
	}

	payload := map[string]string{
		"client_id":     openAIOAuthClientID,
		"grant_type":    "refresh_token",
		"refresh_token": entry.RefreshToken,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return entry, i18n.WrapError(i18n.KeyAuthOAuthEncodeRefreshRequest, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIOAuthTokenURL, strings.NewReader(string(body)))
	if err != nil {
		return entry, i18n.WrapError(i18n.KeyAuthOAuthBuildRefreshRequest, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := openAIOAuthHTTPClient.Do(req)
	if err != nil {
		return entry, i18n.WrapError(i18n.KeyAuthOAuthRefreshRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return entry, i18n.NewError(i18n.KeyAuthOAuthRefreshEndpointRejected, resp.StatusCode, string(body))
	}

	var tr openAITokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return entry, i18n.WrapError(i18n.KeyAuthOAuthDecodeRefreshResponse, err)
	}
	if tr.RefreshToken == "" {
		tr.RefreshToken = entry.RefreshToken
	}

	result, err := openAIResultFromTokenResponse(ctx, &tr)
	if err != nil {
		return entry, err
	}
	if tr.IDToken == "" {
		result.APIKey = firstNonEmpty(result.APIKey, entry.APIKey)
		result.AccountID = entry.AccountID
		result.PlanType = entry.ChatGPTPlanType
		result.AccountIsFedRAMP = entry.ChatGPTAccountIsFedRAMP
	}

	entry.APIKey = result.APIKey
	entry.AccessToken = result.AccessToken
	entry.RefreshToken = result.RefreshToken
	entry.ExpiresAt = result.ExpiresAt
	entry.AccountID = result.AccountID
	entry.ChatGPTPlanType = result.PlanType
	entry.ChatGPTAccountIsFedRAMP = result.AccountIsFedRAMP
	entry.LastUsed = time.Now()
	return entry, nil
}

func openAIOAuthConfigFromEntry(entry CredentialEntry) (Config, error) {
	accessToken := strings.TrimSpace(entry.AccessToken)
	if accessToken == "" {
		return Config{}, i18n.NewError(i18n.KeyAuthOAuthChatGPTAccessTokenNeeded)
	}

	headers := openAICodexHeaders()
	if accountID := strings.TrimSpace(entry.AccountID); accountID != "" {
		headers["ChatGPT-Account-ID"] = accountID
	}
	if entry.ChatGPTAccountIsFedRAMP {
		headers["X-OpenAI-Fedramp"] = "true"
	}

	return Config{
		AuthToken:             accessToken,
		BaseURL:               openAIChatGPTCodexBaseURL,
		Headers:               headers,
		UserScopedPromptCache: true,
	}, nil
}

func openAIOAuthRefreshHandler(r *ProviderRegistry, target *ResponsesProvider) func() bool {
	if r == nil || target == nil {
		return nil
	}
	var refreshMu sync.Mutex
	return func() bool {
		refreshMu.Lock()
		defer refreshMu.Unlock()

		cs := r.CredentialStoreRef()
		if cs == nil {
			return false
		}
		entry, ok := cs.Get("openai")
		if !ok || entry.AuthMethod != "oauth" || strings.TrimSpace(entry.RefreshToken) == "" {
			return false
		}
		refreshed, err := RefreshOpenAIOAuthCredential(context.Background(), entry)
		if err != nil {
			return false
		}
		if err := cs.Set(refreshed); err != nil {
			return false
		}
		cfg, err := openAIOAuthConfigFromEntry(refreshed)
		if err != nil {
			return false
		}
		target.ApplyCredentialConfig(cfg)
		return true
	}
}

func openAIResultFromTokenResponse(ctx context.Context, tr *openAITokenResponse) (*OpenAIOAuthResult, error) {
	if tr == nil {
		return nil, i18n.NewError(i18n.KeyAuthOAuthMissingTokenResponse)
	}
	if tr.AccessToken == "" {
		return nil, i18n.NewError(i18n.KeyAuthOAuthTokenMissingAccessToken)
	}
	apiKey := ""
	apiKeyExchangeError := ""
	if tr.IDToken != "" {
		exchanged, err := exchangeOpenAIIDTokenForAPIKey(ctx, tr.IDToken)
		if err == nil {
			apiKey = exchanged
		} else {
			apiKeyExchangeError = err.Error()
		}
	}
	result := &OpenAIOAuthResult{
		APIKey:              apiKey,
		APIKeyExchangeError: apiKeyExchangeError,
		AccessToken:         tr.AccessToken,
		RefreshToken:        tr.RefreshToken,
	}
	if claims, err := parseOpenAIChatGPTClaims(tr.IDToken); err == nil {
		result.AccountID = claims.AccountID
		result.PlanType = claims.PlanType
		result.AccountIsFedRAMP = claims.AccountIsFedRAMP
	}
	if tr.ExpiresIn > 0 {
		result.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return result, nil
}

type openAIChatGPTClaims struct {
	AccountID        string
	PlanType         string
	AccountIsFedRAMP bool
}

type openAIJWTClaims struct {
	Auth struct {
		PlanType         string `json:"chatgpt_plan_type"`
		AccountID        string `json:"chatgpt_account_id"`
		AccountIsFedRAMP bool   `json:"chatgpt_account_is_fedramp"`
	} `json:"https://api.openai.com/auth"`
}

func parseOpenAIChatGPTClaims(jwt string) (openAIChatGPTClaims, error) {
	jwt = strings.TrimSpace(jwt)
	if jwt == "" {
		return openAIChatGPTClaims{}, i18n.NewError(i18n.KeyProviderOpenAIOAuthIDTokenEmpty)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 || parts[1] == "" {
		return openAIChatGPTClaims{}, i18n.NewError(i18n.KeyProviderOpenAIOAuthIDTokenFormatInvalid)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return openAIChatGPTClaims{}, i18n.WrapError(i18n.KeyProviderOpenAIOAuthIDTokenPayloadDecodeFailed, err)
	}
	var claims openAIJWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return openAIChatGPTClaims{}, i18n.WrapError(i18n.KeyProviderOpenAIOAuthIDTokenPayloadParseFailed, err)
	}
	return openAIChatGPTClaims{
		AccountID:        claims.Auth.AccountID,
		PlanType:         claims.Auth.PlanType,
		AccountIsFedRAMP: claims.Auth.AccountIsFedRAMP,
	}, nil
}

func exchangeOpenAIAuthorizationCode(ctx context.Context, code, verifier, redirectURI string) (*openAITokenResponse, error) {
	vals := url.Values{}
	vals.Set("grant_type", "authorization_code")
	vals.Set("code", code)
	vals.Set("redirect_uri", redirectURI)
	vals.Set("client_id", openAIOAuthClientID)
	vals.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIOAuthTokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthBuildTokenRequest, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := openAIOAuthHTTPClient.Do(req)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthTokenRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, i18n.NewError(i18n.KeyAuthOAuthTokenEndpointRejected, resp.StatusCode, string(body))
	}

	var tr openAITokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthDecodeTokenResponse, err)
	}
	return &tr, nil
}

func exchangeOpenAIIDTokenForAPIKey(ctx context.Context, idToken string) (string, error) {
	vals := url.Values{}
	vals.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
	vals.Set("client_id", openAIOAuthClientID)
	vals.Set("requested_token", "openai-api-key")
	vals.Set("subject_token", idToken)
	vals.Set("subject_token_type", "urn:ietf:params:oauth:token-type:id_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIOAuthTokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return "", i18n.WrapError(i18n.KeyProviderOpenAIOAuthAPIKeyExchangeRequestBuildFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := openAIOAuthHTTPClient.Do(req)
	if err != nil {
		return "", i18n.WrapError(i18n.KeyProviderOpenAIOAuthAPIKeyExchangeRequestFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", i18n.NewError(i18n.KeyProviderOpenAIOAuthAPIKeyExchangeRejected, resp.StatusCode, string(body))
	}

	var tr openAITokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", i18n.WrapError(i18n.KeyProviderOpenAIOAuthAPIKeyExchangeResponseDecodeFailed, err)
	}
	if tr.AccessToken == "" {
		return "", i18n.NewError(i18n.KeyProviderOpenAIOAuthAPIKeyExchangeMissingAccessToken)
	}
	return tr.AccessToken, nil
}

func buildOpenAIAuthorizeURL(challenge, state, redirectURI string) (string, error) {
	u, err := url.Parse(openAIOAuthAuthURL)
	if err != nil {
		return "", i18n.WrapError(i18n.KeyAuthOAuthInvalidAuthorizationURL, err, openAIOAuthAuthURL)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", openAIOAuthClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("scope", strings.Join(openAIOAuthScopes, " "))
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("originator", openAIOAuthOriginator)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func openAIGeneratePKCE() (string, string, error) {
	b := make([]byte, 96)
	if _, err := rand.Read(b); err != nil {
		return "", "", i18n.WrapError(i18n.KeyAuthOAuthGenerateVerifier, err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	return verifier, challenge, nil
}

func openAIRandomState() (string, error) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", i18n.WrapError(i18n.KeyAuthOAuthGenerateState, err)
	}
	return base64.RawURLEncoding.EncodeToString(stateBytes), nil
}
