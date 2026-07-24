package auth

// providers.go — Pre-configured OAuth configurations for known providers.
// Phase 7: OAuth flow integration.

const (
	anthropicOAuthClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	anthropicOAuthAuthorizeURL = "https://platform.claude.com/oauth/authorize"
	anthropicOAuthTokenURL     = "https://platform.claude.com/v1/oauth/token"
	anthropicOAuthDeviceURL    = "https://platform.claude.com/oauth/device/code"
)

var anthropicOAuthScopes = []string{
	"user:profile",
	"user:inference",
	"user:sessions:claude_code",
	"user:mcp_servers",
	"user:file_upload",
}

// AnthropicOAuthConfig returns the standard OAuth PKCE configuration for
// Anthropic's console. This is used by /connect anthropic when the user
// chooses OAuth authentication.
func AnthropicOAuthConfig() OAuthConfig {
	return OAuthConfig{
		ClientID: anthropicOAuthClientID,
		AuthURL:  anthropicOAuthAuthorizeURL,
		TokenURL: anthropicOAuthTokenURL,
		Scopes:   append([]string(nil), anthropicOAuthScopes...),
	}
}

// AnthropicDeviceAuthConfig returns the Device Authorization Grant config
// for Anthropic. This is an alternative for environments where a browser
// redirect is inconvenient (e.g., remote SSH sessions).
func AnthropicDeviceAuthConfig() DeviceAuthConfig {
	return DeviceAuthConfig{
		ClientID:      anthropicOAuthClientID,
		DeviceAuthURL: anthropicOAuthDeviceURL,
		TokenURL:      anthropicOAuthTokenURL,
		Scopes:        append([]string(nil), anthropicOAuthScopes...),
	}
}
