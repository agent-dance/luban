package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/agent-dance/luban/auth"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
)

// connectCmd implements the /connect command for unified auth management.
//
// Usage:
//
//	/connect                — list all providers with connection status
//	/connect <provider>     — connect to a provider by entering an API key
//	/connect <provider> --delete — remove stored credentials for a provider
//	/connect <provider> --oauth  — connect using OAuth PKCE flow
//	/connect <provider> --device — connect using Device Authorization Grant
type connectCmd struct{}

func (c *connectCmd) Name() string      { return "connect" }
func (c *connectCmd) Aliases() []string { return nil }
func (c *connectCmd) Description() string {
	return builtinCommandDescription("connect")
}

func (c *connectCmd) Execute(ctx *Context, args string) error {
	if ctx.ProviderRegistry == nil {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConnectRegistryUnavailable))
		return nil
	}

	args = strings.TrimSpace(args)
	if args == "" {
		return c.listProviders(ctx)
	}

	parts := strings.Fields(args)
	providerName := provider.CanonicalProviderName(parts[0])

	// Handle flags.
	for _, flag := range parts[1:] {
		switch flag {
		case "--delete", "delete":
			return c.deleteCredentials(ctx, providerName)
		case "--oauth", "oauth":
			return c.connectOAuth(ctx, providerName)
		case "--device", "device":
			return c.connectDevice(ctx, providerName)
		}
	}

	return c.connectProvider(ctx, providerName)
}

// listProviders shows visible providers and their connection status.
func (c *connectCmd) listProviders(ctx *Context) error {
	all := ctx.ProviderRegistry.Visible()

	var sb strings.Builder
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyConnectListHeader))
	sb.WriteString(strings.Repeat("─", 50) + "\n")

	for _, info := range all {
		status := "❌"
		connection := ctx.ProviderRegistry.ConnectionState(info.Name).Localized(ctx.Language)
		detail := connection.Detail
		switch connection.State {
		case provider.ConnectionStateConnected:
			status = "✅"
		case provider.ConnectionStateLocal:
			status = "◌"
		case provider.ConnectionStateUnknown:
			status = "?"
		}
		displayName := info.DisplayName
		if displayName == "" {
			displayName = info.Name
		}
		sb.WriteString(fmt.Sprintf("  %s %-12s — %s\n", status, displayName, detail))
	}

	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyConnectListHint))
	ctx.OnEvent(sb.String())
	return nil
}

// connectProvider prompts for an API key and saves it to the credential store.
// For providers that support OAuth, suggests the --oauth flag.
func (c *connectCmd) connectProvider(ctx *Context, name string) error {
	name = provider.CanonicalProviderName(name)
	info, ok := ctx.ProviderRegistry.Get(name)
	if !ok {
		return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyConnectUnknownProviderAvailable,
			name, strings.Join(ctx.ProviderRegistry.VisibleNames(), ", ")))
	}

	connection := ctx.ProviderRegistry.ConnectionState(info.Name).Localized(ctx.Language)
	if connection.CanSelectModels {
		displayName := info.DisplayName
		if displayName == "" {
			displayName = info.Name
		}
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectReady, displayName, connection.Detail))
		return nil
	}

	// Hint about OAuth if supported.
	for _, method := range info.AuthMethods {
		if method == "oauth_pkce" {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectOAuthHint, info.DisplayName, name))
			break
		}
		if method == "device_code" {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectDeviceHint, info.DisplayName, name))
			break
		}
	}

	if !providerSupportsAuthMethod(info, "api_key") && info.EnvKey == "" {
		if connection.SetupHint != "" {
			ctx.OnEvent(connection.SetupHint + "\n")
			return nil
		}
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectInlineAPIKeyUnsupported, info.DisplayName))
		return nil
	}

	if ctx.CredentialStore == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyConnectCredentialStoreCannotSave))
	}

	// Prompt for API key.
	displayName := info.DisplayName
	if displayName == "" {
		displayName = info.Name
	}
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectAPIKeyPrompt, displayName, info.EnvKey))

	// Read the API key. In REPL mode the Confirm func is not ideal for
	// free-text input; we read directly from stdin.
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConnectCancelled))
		return nil
	}
	apiKey := strings.TrimSpace(scanner.Text())
	if apiKey == "" {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConnectNoAPIKey))
		return nil
	}

	// Save to credential store.
	entry := provider.CredentialEntry{
		Provider:   info.Name,
		AuthMethod: "api_key",
		APIKey:     apiKey,
		LastUsed:   time.Now(),
	}
	if err := ctx.CredentialStore.Set(entry); err != nil {
		return connectWrappedError(ctx.Language, i18n.KeyConnectSaveCredentialsFailed, err)
	}

	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectCredentialsSaved, displayName))
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectModelHint, info.Name))
	return nil
}

func providerSupportsAuthMethod(info provider.ProviderInfo, method string) bool {
	for _, candidate := range info.AuthMethods {
		if candidate == method {
			return true
		}
	}
	return false
}

// RunProviderOAuthConnect runs the same OAuth flow used by "/connect <provider> --oauth".
// It is exported so non-command UI surfaces can reuse the command implementation
// without parsing a synthetic slash command string.
func RunProviderOAuthConnect(ctx *Context, name string) error {
	return (&connectCmd{}).connectOAuth(ctx, name)
}

// RunProviderDeviceConnect runs the same device auth flow used by
// "/connect <provider> --device".
func RunProviderDeviceConnect(ctx *Context, name string) error {
	return (&connectCmd{}).connectDevice(ctx, name)
}

// connectOAuth runs an OAuth PKCE flow for a provider.
func (c *connectCmd) connectOAuth(ctx *Context, name string) error {
	name = provider.CanonicalProviderName(name)
	info, ok := ctx.ProviderRegistry.Get(name)
	if !ok {
		return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyConnectUnknownProvider, name))
	}

	if ctx.CredentialStore == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyConnectCredentialStoreUnavailable))
	}

	// Check if the provider supports OAuth PKCE.
	supportsOAuth := false
	for _, method := range info.AuthMethods {
		if method == "oauth_pkce" {
			supportsOAuth = true
			break
		}
	}
	if !supportsOAuth {
		return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyConnectOAuthUnsupported, info.DisplayName))
	}

	if name == "openai" {
		return c.connectOpenAIOAuth(ctx, info)
	}

	// Get the OAuth configuration for this provider.
	oauthCfg := oauthConfigForProvider(name)
	if oauthCfg.AuthURL == "" {
		return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyConnectOAuthConfigUnavailable, info.DisplayName))
	}

	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectOAuthStarting, info.DisplayName))
	ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConnectBrowserOpening))

	// Start the OAuth flow with a URL callback.
	authURLCh := make(chan string, 1)
	authCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Launch flow in background so we can open the browser.
	type flowResult struct {
		token *auth.TokenResponse
		err   error
	}
	resultCh := make(chan flowResult, 1)
	go func() {
		tr, err := auth.StartOAuthFlowWithURL(authCtx, oauthCfg, authURLCh)
		resultCh <- flowResult{tr, err}
	}()

	// Wait for auth URL and open browser.
	select {
	case authURL := <-authURLCh:
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectOpenURL, authURL))
		_ = openBrowser(authURL, ctx.Language)
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConnectWaiting))
	case <-authCtx.Done():
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyConnectOAuthTimedOut))
	}

	// Wait for the flow to complete.
	select {
	case result := <-resultCh:
		if result.err != nil {
			return connectWrappedError(ctx.Language, i18n.KeyConnectOAuthFailed, result.err)
		}

		// Save OAuth credentials to the credential store.
		entry := provider.CredentialEntry{
			Provider:     info.Name,
			AuthMethod:   "oauth",
			AccessToken:  result.token.AccessToken,
			RefreshToken: result.token.RefreshToken,
			ExpiresAt:    result.token.ExpiresAt,
			LastUsed:     time.Now(),
		}
		if err := ctx.CredentialStore.Set(entry); err != nil {
			return connectWrappedError(ctx.Language, i18n.KeyConnectSaveOAuthCredentialsFailed, err)
		}

		// Also save to auth.Store for the OAuthHookAdapter.
		authStore, err := auth.NewStore()
		if err == nil {
			scopes := strings.Fields(result.token.Scope)
			if len(scopes) == 0 {
				scopes = append([]string(nil), oauthCfg.Scopes...)
			}
			_ = authStore.SaveCredentials(&auth.Credentials{
				Provider:     info.Name,
				AccessToken:  result.token.AccessToken,
				RefreshToken: result.token.RefreshToken,
				ExpiresAt:    result.token.ExpiresAt,
				TokenType:    result.token.TokenType,
				Scopes:       scopes,
			})
		}

		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectOAuthSuccess, info.DisplayName))
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectModelHint, info.Name))
		return nil

	case <-authCtx.Done():
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyConnectOAuthTimedOut))
	}
}

func (c *connectCmd) connectOpenAIOAuth(ctx *Context, info provider.ProviderInfo) error {
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectOAuthStarting, info.DisplayName))
	ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConnectBrowserOpening))

	authURLCh := make(chan string, 1)
	authCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	type flowResult struct {
		result *provider.OpenAIOAuthResult
		err    error
	}
	resultCh := make(chan flowResult, 1)
	go func() {
		res, err := provider.StartOpenAIOAuthFlow(authCtx, authURLCh)
		resultCh <- flowResult{result: res, err: err}
	}()

	select {
	case authURL := <-authURLCh:
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectOpenURL, authURL))
		_ = openBrowser(authURL, ctx.Language)
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConnectWaiting))
	case <-authCtx.Done():
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyConnectOAuthTimedOut))
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			return connectWrappedError(ctx.Language, i18n.KeyConnectOAuthFailed, result.err)
		}

		entry := provider.CredentialEntry{
			Provider:                "openai",
			AuthMethod:              "oauth",
			APIKey:                  result.result.APIKey,
			AccessToken:             result.result.AccessToken,
			RefreshToken:            result.result.RefreshToken,
			ExpiresAt:               result.result.ExpiresAt,
			AccountID:               result.result.AccountID,
			ChatGPTPlanType:         result.result.PlanType,
			ChatGPTAccountIsFedRAMP: result.result.AccountIsFedRAMP,
			LastUsed:                time.Now(),
		}
		if err := ctx.CredentialStore.Set(entry); err != nil {
			return connectWrappedError(ctx.Language, i18n.KeyConnectSaveOAuthCredentialsFailed, err)
		}

		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectOAuthSuccess, info.DisplayName))
		if result.result.APIKey != "" {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConnectOpenAITokensWithAPIKey))
		} else {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConnectOpenAITokensCodex))
			if result.result.APIKeyExchangeError != "" {
				ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConnectOpenAIAPIKeyUnavailable))
			}
		}
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectModelHint, "openai"))
		return nil
	case <-authCtx.Done():
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyConnectOAuthTimedOut))
	}
}

// connectDevice runs an RFC 8628 Device Authorization Grant flow.
func (c *connectCmd) connectDevice(ctx *Context, name string) error {
	name = provider.CanonicalProviderName(name)
	info, ok := ctx.ProviderRegistry.Get(name)
	if !ok {
		return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyConnectUnknownProvider, name))
	}

	if ctx.CredentialStore == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyConnectCredentialStoreUnavailable))
	}

	// Get the device auth configuration for this provider.
	deviceCfg := deviceAuthConfigForProvider(name)
	if deviceCfg.DeviceAuthURL == "" {
		return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyConnectDeviceUnavailable, info.DisplayName))
	}

	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectDeviceStarting, info.DisplayName))

	authCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	tr, err := auth.StartDeviceAuthFlow(authCtx, deviceCfg, func(dcr auth.DeviceCodeResponse) {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectUserCode, dcr.UserCode))
		if dcr.VerificationURIComplete != "" {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectOpen, dcr.VerificationURIComplete))
			_ = openBrowser(dcr.VerificationURIComplete, ctx.Language)
		} else if dcr.VerificationURI != "" {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectOpen, dcr.VerificationURI))
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectEnterCode, dcr.UserCode))
			_ = openBrowser(dcr.VerificationURI, ctx.Language)
		}
		ctx.OnEvent("\n" + i18n.Text(ctx.Language, i18n.KeyConnectWaiting))
	})
	if err != nil {
		return connectWrappedError(ctx.Language, i18n.KeyConnectDeviceFailed, err)
	}

	// Save OAuth credentials to the credential store.
	entry := provider.CredentialEntry{
		Provider:     info.Name,
		AuthMethod:   "oauth",
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    tr.ExpiresAt,
		LastUsed:     time.Now(),
	}
	if err := ctx.CredentialStore.Set(entry); err != nil {
		return connectWrappedError(ctx.Language, i18n.KeyConnectSaveCredentialsFailed, err)
	}

	// Also save to auth.Store for the OAuthHookAdapter.
	authStore, storeErr := auth.NewStore()
	if storeErr == nil {
		scopes := strings.Fields(tr.Scope)
		if len(scopes) == 0 {
			scopes = append([]string(nil), deviceCfg.Scopes...)
		}
		_ = authStore.SaveCredentials(&auth.Credentials{
			Provider:     info.Name,
			AccessToken:  tr.AccessToken,
			RefreshToken: tr.RefreshToken,
			ExpiresAt:    tr.ExpiresAt,
			TokenType:    tr.TokenType,
			Scopes:       scopes,
		})
	}

	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectDeviceSuccess, info.DisplayName))
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectModelHint, info.Name))
	return nil
}

// deleteCredentials removes stored credentials for a provider.
func (c *connectCmd) deleteCredentials(ctx *Context, name string) error {
	name = provider.CanonicalProviderName(name)
	_, ok := ctx.ProviderRegistry.Get(name)
	if !ok {
		return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyConnectUnknownProvider, name))
	}

	if ctx.CredentialStore == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyConnectCredentialStoreUnavailable))
	}

	for _, lookupName := range provider.CredentialLookupNames(name) {
		if err := ctx.CredentialStore.Delete(lookupName); err != nil {
			return connectWrappedError(ctx.Language, i18n.KeyConnectDeleteCredentialsFailed, err)
		}
	}

	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConnectCredentialsRemoved, name))
	return nil
}

func connectWrappedError(lang i18n.Language, key i18n.Key, err error) error {
	return fmt.Errorf("%s %w", i18n.Text(lang, key), err)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// oauthConfigForProvider returns the OAuth PKCE config for a known provider.
func oauthConfigForProvider(name string) auth.OAuthConfig {
	switch provider.CanonicalProviderName(name) {
	case "anthropic":
		return auth.AnthropicOAuthConfig()
	default:
		return auth.OAuthConfig{} // not configured
	}
}

// deviceAuthConfigForProvider returns the Device Auth config for a known provider.
func deviceAuthConfigForProvider(name string) auth.DeviceAuthConfig {
	switch provider.CanonicalProviderName(name) {
	case "anthropic":
		return auth.AnthropicDeviceAuthConfig()
	default:
		return auth.DeviceAuthConfig{} // not configured
	}
}

// openBrowser attempts to open a URL in the system default browser.
func openBrowser(url string, lang i18n.Language) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("%s", i18n.Format(lang, i18n.KeyConnectUnsupportedOS, runtime.GOOS))
	}
	return cmd.Start()
}
