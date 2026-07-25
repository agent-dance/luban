package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// Config is the OAuth-specific portion of an MCP server configuration.
type Config struct {
	ClientID              string `json:"clientId,omitempty"`
	CallbackPort          *int   `json:"callbackPort,omitempty"`
	AuthServerMetadataURL string `json:"authServerMetadataUrl,omitempty"`
	XAA                   *bool  `json:"xaa,omitempty"`
}

// ServerDescriptor is the auth domain's transport-independent view of a
// configured MCP server. OAuthCapable is decided by the transport layer.
type ServerDescriptor struct {
	Transport    string
	URL          string
	Headers      map[string]string
	OAuth        *Config
	OAuthCapable bool
}

// UnauthorizedError carries an OAuth challenge from a remote transport.
type UnauthorizedError struct {
	Challenge  *AuthChallenge
	ServerURL  string
	StatusCode int
}

// Error implements error.
func (e *UnauthorizedError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPUnauthorized)
	}
	if e.Challenge != nil && e.Challenge.ASURI != "" {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPUnauthorizedASURI, e.Challenge.ASURI)
	}
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPUnauthorized)
}

// HTTPAuthError is the narrow contract implemented by transport-layer HTTP
// errors that carry enough information for auth-state classification.
type HTTPAuthError interface {
	error
	AuthStatusCode() int
	AuthChallenge() *AuthChallenge
}

// IsRequiredError reports whether err should transition a server into the
// needs-auth state.
func IsRequiredError(err error) bool {
	if err == nil {
		return false
	}
	var unauthorized *UnauthorizedError
	if errors.As(err, &unauthorized) {
		return true
	}
	var remote HTTPAuthError
	if !errors.As(err, &remote) {
		return false
	}
	if remote.AuthStatusCode() == http.StatusUnauthorized {
		return true
	}
	challenge := remote.AuthChallenge()
	return remote.AuthStatusCode() == http.StatusForbidden &&
		challenge != nil &&
		strings.EqualFold(challenge.ErrorCode, "insufficient_scope")
}
